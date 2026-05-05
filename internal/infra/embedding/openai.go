package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type OpenAICompatibleConfig struct {
	Name           string
	BaseURL        string
	APIKey         string
	Model          string
	Dim            int
	TimeoutSeconds int
}

type OpenAICompatible struct {
	cfg OpenAICompatibleConfig
	hc  *http.Client
}

func NewOpenAICompatible(cfg OpenAICompatibleConfig) *OpenAICompatible {
	if cfg.Name == "" {
		cfg.Name = "openai"
	}
	if cfg.Model == "" {
		cfg.Model = "text-embedding-3-small"
	}
	if cfg.Dim <= 0 {
		cfg.Dim = 1536
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 60
	}
	return &OpenAICompatible{cfg: cfg, hc: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}}
}

func (p *OpenAICompatible) Name() string  { return p.cfg.Name }
func (p *OpenAICompatible) Model() string { return p.cfg.Model }
func (p *OpenAICompatible) Dim() int      { return p.cfg.Dim }

type embeddingsRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
	Dimensions     int      `json:"dimensions,omitempty"`
}

type embeddingsResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func (p *OpenAICompatible) Embed(ctx context.Context, texts []string) ([][]float32, Usage, error) {
	if len(texts) == 0 {
		return nil, Usage{}, nil
	}
	const maxBatchSize = 128
	if len(texts) > maxBatchSize {
		out := make([][]float32, 0, len(texts))
		total := Usage{}
		for start := 0; start < len(texts); start += maxBatchSize {
			end := start + maxBatchSize
			if end > len(texts) {
				end = len(texts)
			}
			vecs, usage, err := p.embedBatch(ctx, texts[start:end])
			if err != nil {
				return nil, Usage{}, err
			}
			out = append(out, vecs...)
			total.InputTokens += usage.InputTokens
		}
		return out, total, nil
	}
	return p.embedBatch(ctx, texts)
}

func (p *OpenAICompatible) embedBatch(ctx context.Context, texts []string) ([][]float32, Usage, error) {
	body, err := json.Marshal(embeddingsRequest{
		Model:          p.cfg.Model,
		Input:          texts,
		EncodingFormat: "float",
		Dimensions:     p.cfg.Dim,
	})
	if err != nil {
		return nil, Usage{}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(p.cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, strings.TrimRight(p.cfg.BaseURL, "/")+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, Usage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.hc.Do(req)
	if err != nil {
		return nil, Usage{}, errcode.Wrap(err, errcode.CodeUnavailable, "embedding provider transport error")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, Usage{}, errcode.Newf(errcode.CodeUnavailable, "embedding provider status %d", resp.StatusCode)
	}
	var decoded embeddingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, Usage{}, fmt.Errorf("embedding: decode response: %w", err)
	}
	out := make([][]float32, len(texts))
	for _, item := range decoded.Data {
		if item.Index >= 0 && item.Index < len(out) {
			out[item.Index] = item.Embedding
		}
	}
	for i, vec := range out {
		if len(vec) != p.cfg.Dim {
			return nil, Usage{}, errcode.Newf(errcode.CodeInvalidArgument, "embedding dimension mismatch at index %d", i)
		}
	}
	tokens := decoded.Usage.TotalTokens
	if tokens == 0 {
		tokens = decoded.Usage.PromptTokens
	}
	return out, Usage{InputTokens: tokens}, nil
}
