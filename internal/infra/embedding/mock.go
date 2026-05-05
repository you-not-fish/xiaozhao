package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
)

type MockProvider struct {
	model string
	dim   int
}

func NewMockProvider(model string, dim int) *MockProvider {
	if model == "" {
		model = "mock-embedding"
	}
	if dim <= 0 {
		dim = 1536
	}
	return &MockProvider{model: model, dim: dim}
}

func (p *MockProvider) Name() string  { return "mock" }
func (p *MockProvider) Model() string { return p.model }
func (p *MockProvider) Dim() int      { return p.dim }

func (p *MockProvider) Embed(_ context.Context, texts []string) ([][]float32, Usage, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec := make([]float32, p.dim)
		for i := 0; i < p.dim; i += 8 {
			sum := sha256.Sum256([]byte(text + string(rune(i))))
			for j := 0; j < 8 && i+j < p.dim; j++ {
				v := binary.BigEndian.Uint32(sum[j*4 : j*4+4])
				vec[i+j] = float32(float64(v%20000)/10000.0 - 1.0)
			}
		}
		normalize(vec)
		out = append(out, vec)
	}
	return out, Usage{InputTokens: len([]rune(joinTexts(texts))) / 2}, nil
}

func normalize(v []float32) {
	var sum float64
	for _, f := range v {
		sum += float64(f * f)
	}
	if sum == 0 {
		return
	}
	n := float32(math.Sqrt(sum))
	for i := range v {
		v[i] /= n
	}
}

func joinTexts(texts []string) string {
	n := 0
	for _, t := range texts {
		n += len(t)
	}
	b := make([]byte, 0, n)
	for _, t := range texts {
		b = append(b, t...)
	}
	return string(b)
}
