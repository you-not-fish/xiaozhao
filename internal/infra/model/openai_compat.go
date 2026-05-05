package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

// OpenAICompatibleConfig 是国际 OpenAI、以及所有对齐 OpenAI Chat Completions
// 协议的国内供应商（DeepSeek / 通义 / Moonshot / 智谱 / Qwen 等）共用的配置。
type OpenAICompatibleConfig struct {
	Name    string        // 供应商标识，例如 "openai" / "deepseek"
	BaseURL string        // 例如 https://api.openai.com/v1
	APIKey  string        // Bearer token
	Timeout time.Duration // 单次请求超时
}

// NewOpenAICompatible 构造 OpenAI-compatible 的 Provider。
func NewOpenAICompatible(cfg OpenAICompatibleConfig) *OpenAICompatible {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &OpenAICompatible{
		cfg: cfg,
		hc: &http.Client{
			Timeout: 0, // 流式调用不要在 http.Client 层设置超时，由 ctx 控制
		},
	}
}

// OpenAICompatible 走 /chat/completions 协议 + SSE 流的 Provider 实现。
type OpenAICompatible struct {
	cfg OpenAICompatibleConfig
	hc  *http.Client
}

func (p *OpenAICompatible) Name() string { return p.cfg.Name }

// chatRequestBody 是发送给供应商的 JSON 请求体；仅包含 MVP 需要的字段。
type chatRequestBody struct {
	Model       string          `json:"model"`
	Messages    []messageBody   `json:"messages"`
	Tools       []toolBody      `json:"tools,omitempty"`
	Stream      bool            `json:"stream"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	ToolChoice  string          `json:"tool_choice,omitempty"` // "auto" 让模型自行选择
	StreamUsage *streamOptsBody `json:"stream_options,omitempty"`
}

type streamOptsBody struct {
	IncludeUsage bool `json:"include_usage"`
}

type messageBody struct {
	Role       string            `json:"role"`
	Content    string            `json:"content,omitempty"`
	Name       string            `json:"name,omitempty"`
	ToolCalls  []apiToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type apiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function apiToolFunction `json:"function"`
}

type apiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolBody struct {
	Type     string       `json:"type"` // 固定 "function"
	Function toolFuncBody `json:"function"`
}

type toolFuncBody struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Chat 发起一次流式调用。
func (p *OpenAICompatible) Chat(ctx context.Context, req ChatRequest) (Stream, error) {
	body := chatRequestBody{
		Model:       req.Model,
		Stream:      true,
		MaxTokens:   req.MaxTokens,
		StreamUsage: &streamOptsBody{IncludeUsage: true},
	}
	if len(req.Tools) > 0 {
		body.ToolChoice = "auto"
		body.Tools = make([]toolBody, 0, len(req.Tools))
		for _, t := range req.Tools {
			body.Tools = append(body.Tools, toolBody{
				Type: "function",
				Function: toolFuncBody{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			})
		}
	}
	body.Messages = make([]messageBody, 0, len(req.Messages))
	for _, m := range req.Messages {
		mb := messageBody{
			Role:       string(m.Role),
			Content:    m.Content,
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			mb.ToolCalls = make([]apiToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				mb.ToolCalls = append(mb.ToolCalls, apiToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: apiToolFunction{
						Name:      tc.Name,
						Arguments: string(tc.Arguments),
					},
				})
			}
		}
		body.Messages = append(body.Messages, mb)
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("model: marshal request: %w", err)
	}

	// 为 http 请求独立派生超时上下文；Recv 结束或调用方 cancel 时关闭底层流。
	reqCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, p.cfg.BaseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("model: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.hc.Do(httpReq)
	if err != nil {
		cancel()
		return nil, mapHTTPError(err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		cancel()
		return nil, mapStatusError(resp)
	}

	return &openaiStream{
		reader: bufio.NewReaderSize(resp.Body, 8*1024),
		closer: resp.Body,
		cancel: cancel,
	}, nil
}

// openaiStream 解析 Server-Sent Events，逐块转成 Chunk。
type openaiStream struct {
	reader *bufio.Reader
	closer io.Closer
	cancel context.CancelFunc
}

// streamChunk 是 OpenAI SSE 单行 data 的结构。
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string        `json:"content"`
			ToolCalls []apiToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// Recv 读取下一块增量。
// 解析策略：按行读 SSE，遇到 `data: ` 前缀的行做 JSON 解码；
// 遇到 `data: [DONE]` 返回 io.EOF；空行、注释行跳过。
func (s *openaiStream) Recv() (*Chunk, error) {
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("model: read stream: %w", err)
		}
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if !bytes.HasPrefix(trimmed, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(trimmed[len("data:"):])
		if bytes.Equal(payload, []byte("[DONE]")) {
			return nil, io.EOF
		}

		var sc streamChunk
		if err := json.Unmarshal(payload, &sc); err != nil {
			// 解析失败不中断流；记一个错误块留给上层记日志但继续读下一行。
			return nil, fmt.Errorf("model: unmarshal chunk: %w", err)
		}

		chunk := &Chunk{}
		if len(sc.Choices) > 0 {
			d := sc.Choices[0].Delta
			chunk.TextDelta = d.Content
			if len(d.ToolCalls) > 0 {
				chunk.ToolCallDelta = make([]ToolCallDelta, 0, len(d.ToolCalls))
				for i, tc := range d.ToolCalls {
					chunk.ToolCallDelta = append(chunk.ToolCallDelta, ToolCallDelta{
						Index:          i,
						ID:             tc.ID,
						Name:           tc.Function.Name,
						ArgumentsDelta: tc.Function.Arguments,
					})
				}
			}
			chunk.FinishReason = mapFinish(sc.Choices[0].FinishReason)
		}
		if sc.Usage != nil {
			chunk.Usage = &Usage{
				InputTokens:  sc.Usage.PromptTokens,
				OutputTokens: sc.Usage.CompletionTokens,
			}
		}
		// 跳过纯空块（既无文本又无工具调用又无 usage），避免前端抖动。
		if chunk.TextDelta == "" && len(chunk.ToolCallDelta) == 0 && chunk.Usage == nil && chunk.FinishReason == "" {
			continue
		}
		return chunk, nil
	}
}

func (s *openaiStream) Close() error {
	s.cancel()
	return s.closer.Close()
}

// mapFinish 把供应商返回的 finish_reason 映射到标准枚举。
func mapFinish(raw string) FinishReason {
	switch raw {
	case "stop":
		return FinishStop
	case "tool_calls", "function_call":
		return FinishToolCalls
	case "length":
		return FinishLength
	case "":
		return ""
	default:
		return FinishReason(raw)
	}
}

// mapHTTPError 把底层 http 错误映射为 errcode.Error。
func mapHTTPError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errcode.Wrap(err, errcode.CodeModelTimeout, "model request timed out")
	}
	return errcode.Wrap(err, errcode.CodeUnavailable, "model provider transport error")
}

// mapStatusError 根据供应商返回的 http 状态码生成标准化错误。
// 只读取有限的响应体，避免日志里混入大段错误正文。
func mapStatusError(resp *http.Response) error {
	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	snippet := strings.TrimSpace(string(buf[:n]))
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errcode.Newf(errcode.CodeModelAuthFailed, "model auth failed: %s", snippet)
	case http.StatusTooManyRequests:
		return errcode.Newf(errcode.CodeModelRateLimited, "model rate limited: %s", snippet)
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return errcode.Newf(errcode.CodeModelTimeout, "model timeout: %s", snippet)
	case http.StatusBadRequest:
		// 参数错误多半是上下文超长或 tool schema 不合规
		if strings.Contains(strings.ToLower(snippet), "context") {
			return errcode.Newf(errcode.CodeModelContextExceed, "context exceeded: %s", snippet)
		}
		return errcode.Newf(errcode.CodeToolParamInvalid, "model rejected request: %s", snippet)
	default:
		return errcode.Newf(errcode.CodeUnavailable, "model status %d: %s", resp.StatusCode, snippet)
	}
}
