package model

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

// MockProvider 是一个可脚本化的 Provider，给开发模式和单元测试使用。
// 当 config 里未配置任何真实供应商时，服务用它保证 Agent 链路可跑通。
//
// 调用方通过 Script 定义"每次 Chat 返回什么"——第 N 次 Chat 会使用 Script[N]；
// 超出脚本长度则回退到简单的文本回声。
//
// 典型用法：
//   p := model.NewMockProvider(
//     model.MockScript{Text: "正在查询当前时间...", ToolCalls: []model.ToolCall{{Name:"current_time"}}},
//     model.MockScript{Text: "当前时间是 2026-05-04"},
//   )
type MockProvider struct {
	scripts []MockScript
	calls   int
}

// MockScript 描述某一次 Chat 要吐出的流式内容。
type MockScript struct {
	Text      string      // 一次性全部作为 TextDelta 返回
	ToolCalls []ToolCall  // 若非空则 finish=tool_calls，Text 会在工具调用之前返回
	Usage     *Usage      // 可选
	Finish    FinishReason
	// DelayPerChunk 让测试模拟真实延迟；MVP 默认 0。
	DelayPerChunk time.Duration
}

// NewMockProvider 构造。
func NewMockProvider(scripts ...MockScript) *MockProvider {
	return &MockProvider{scripts: scripts}
}

func (p *MockProvider) Name() string { return "mock" }

// Chat 返回一次流；不真正发出网络请求。
func (p *MockProvider) Chat(_ context.Context, req ChatRequest) (Stream, error) {
	var sc MockScript
	if p.calls < len(p.scripts) {
		sc = p.scripts[p.calls]
	} else {
		// 无脚本时的默认回声：原样返回最后一条用户消息，便于在无 LLM
		// 环境下本地验证 Agent 管线。
		sc = MockScript{Text: echoLastUser(req.Messages), Finish: FinishStop}
	}
	p.calls++
	return newMockStream(sc), nil
}

func echoLastUser(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleUser {
			return "[mock] " + msgs[i].Content
		}
	}
	return "[mock] hello"
}

type mockStream struct {
	chunks []*Chunk
	idx    int
	delay  time.Duration
}

func newMockStream(sc MockScript) *mockStream {
	s := &mockStream{delay: sc.DelayPerChunk}
	// 文本按 "词" 切片，制造多块增量以贴近真实 SSE 行为。
	if sc.Text != "" {
		for _, word := range strings.SplitAfter(sc.Text, " ") {
			if word == "" {
				continue
			}
			s.chunks = append(s.chunks, &Chunk{TextDelta: word})
		}
	}
	if len(sc.ToolCalls) > 0 {
		deltas := make([]ToolCallDelta, 0, len(sc.ToolCalls))
		for i, tc := range sc.ToolCalls {
			args := tc.Arguments
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			deltas = append(deltas, ToolCallDelta{
				Index:          i,
				ID:             tc.ID,
				Name:           tc.Name,
				ArgumentsDelta: string(args),
			})
		}
		s.chunks = append(s.chunks, &Chunk{ToolCallDelta: deltas})
	}
	if sc.Usage != nil {
		s.chunks = append(s.chunks, &Chunk{Usage: sc.Usage})
	}
	finish := sc.Finish
	if finish == "" {
		if len(sc.ToolCalls) > 0 {
			finish = FinishToolCalls
		} else {
			finish = FinishStop
		}
	}
	s.chunks = append(s.chunks, &Chunk{FinishReason: finish})
	return s
}

func (s *mockStream) Recv() (*Chunk, error) {
	if s.idx >= len(s.chunks) {
		return nil, io.EOF
	}
	c := s.chunks[s.idx]
	s.idx++
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return c, nil
}

func (s *mockStream) Close() error { return nil }

// ErrMockExhausted 留作文档占位，当前实现不会返回；保留接口一致性。
var ErrMockExhausted = errors.New("model: mock script exhausted")
