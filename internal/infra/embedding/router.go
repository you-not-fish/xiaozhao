package embedding

import (
	"fmt"

	"github.com/xiaozhao/xiaozhao/internal/pkg/config"
)

func NewProvider(cfg config.EmbeddingConfig) (Provider, error) {
	switch cfg.Provider {
	case "", "mock":
		return NewMockProvider(cfg.Model, cfg.Dim), nil
	case "openai":
		if cfg.OpenAI.BaseURL == "" || cfg.OpenAI.APIKey == "" {
			return nil, fmt.Errorf("embedding.openai.base_url and api_key are required")
		}
		model := cfg.Model
		if model == "" {
			model = cfg.OpenAI.Model
		}
		return NewOpenAICompatible(OpenAICompatibleConfig{
			Name:           cfg.OpenAI.Name,
			BaseURL:        cfg.OpenAI.BaseURL,
			APIKey:         cfg.OpenAI.APIKey,
			Model:          model,
			Dim:            cfg.Dim,
			TimeoutSeconds: cfg.OpenAI.TimeoutSeconds,
		}), nil
	default:
		return nil, fmt.Errorf("embedding.provider must be one of: mock, openai (got %q)", cfg.Provider)
	}
}
