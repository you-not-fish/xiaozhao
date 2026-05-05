package websearch

import (
	"fmt"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/pkg/config"
)

func NewProvider(cfg config.WebSearchConfig) (Provider, error) {
	switch cfg.Provider {
	case "", "mock":
		return NewMockProvider(cfg.MaxResults, cfg.DefaultFreshness), nil
	case "bocha":
		return NewBochaProvider(BochaConfig{
			BaseURL:          cfg.Bocha.BaseURL,
			APIKey:           cfg.Bocha.APIKey,
			Timeout:          time.Duration(cfg.Bocha.TimeoutSeconds) * time.Second,
			MaxResults:       cfg.MaxResults,
			DefaultFreshness: cfg.DefaultFreshness,
		}), nil
	default:
		return nil, fmt.Errorf("web_search: unsupported provider %q", cfg.Provider)
	}
}
