package embedding

import "context"

type Usage struct {
	InputTokens int
}

type Provider interface {
	Name() string
	Model() string
	Dim() int
	Embed(ctx context.Context, texts []string) ([][]float32, Usage, error)
}
