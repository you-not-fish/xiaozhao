package embedding

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestOpenAICompatibleBatchesLargeInput(t *testing.T) {
	calls := 0
	p := NewOpenAICompatible(OpenAICompatibleConfig{
		BaseURL:        "http://embedding.test",
		APIKey:         "test",
		Model:          "embed",
		Dim:            2,
		TimeoutSeconds: 3,
	})
	p.hc.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var req embeddingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		resp := embeddingsResponse{}
		for i := range req.Input {
			resp.Data = append(resp.Data, struct {
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			}{Index: i, Embedding: []float32{float32(i), 1}})
		}
		resp.Usage.TotalTokens = len(req.Input)
		body, err := json.Marshal(resp)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    r,
		}, nil
	})
	texts := make([]string, 129)
	for i := range texts {
		texts[i] = "hello"
	}
	vecs, usage, err := p.Embed(t.Context(), texts)
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if len(vecs) != 129 || usage.InputTokens != 129 {
		t.Fatalf("shape/usage = %d/%d", len(vecs), usage.InputTokens)
	}
	if p.hc.Timeout != 3*time.Second {
		t.Fatalf("client timeout = %s", p.hc.Timeout)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
