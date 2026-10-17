package providers

import (
	"fmt"
	"log/slog"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/anthropic"
	"github.com/slashobserve/slashobserve/internal/llm/replay"
)

func New(cfg config.LLM, logger *slog.Logger) (llm.Provider, error) {
	switch cfg.Provider {
	case "":
		return nil, nil
	case config.LLMProviderAnthropic:
		return anthropic.New(anthropic.Config{
			APIKey:         cfg.APIKey,
			Model:          cfg.Model,
			Effort:         cfg.Effort,
			BaseURL:        cfg.BaseURL,
			MaxTokens:      cfg.MaxTokens,
			RequestTimeout: cfg.RequestTimeout,
		}, logger), nil
	case config.LLMProviderReplay:
		p, err := replay.New(cfg.ReplayDir, logger)
		if err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("llm provider %q is not available yet", cfg.Provider)
	}
}
