package providers

import (
	"cmp"
	"fmt"
	"log/slog"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/anthropic"
	"github.com/slashobserve/slashobserve/internal/llm/openaicompat"
	"github.com/slashobserve/slashobserve/internal/llm/replay"
)

const anthropicDefaultEffort = "medium"

func New(cfg config.LLM, logger *slog.Logger) (llm.Provider, error) {
	switch cfg.Provider {
	case "":
		return nil, nil
	case config.LLMProviderAnthropic:
		return anthropic.New(anthropic.Config{
			APIKey:         cfg.APIKey,
			Model:          cfg.Model,
			Effort:         cmp.Or(cfg.Effort, anthropicDefaultEffort),
			BaseURL:        cfg.BaseURL,
			MaxTokens:      cfg.MaxTokens,
			RequestTimeout: cfg.RequestTimeout,
			IdleTimeout:    cfg.IdleTimeout,

			LogProviderErrors: cfg.LogProviderErrors,
		}), nil
	case config.LLMProviderOpenAI:
		return openaicompat.New(openaicompat.Config{
			APIKey:         cfg.APIKey,
			Model:          cfg.Model,
			Effort:         cfg.Effort,
			BaseURL:        cfg.BaseURL,
			MaxTokens:      cfg.MaxTokens,
			RequestTimeout: cfg.RequestTimeout,
			IdleTimeout:    cfg.IdleTimeout,

			LogProviderErrors: cfg.LogProviderErrors,
		}, logger), nil
	case config.LLMProviderReplay:
		p, err := replay.New(cfg.ReplayDir, cfg.MaxTokens, logger)
		if err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("llm provider %q is not available yet", cfg.Provider)
	}
}
