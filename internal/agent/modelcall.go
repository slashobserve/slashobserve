package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/slashobserve/slashobserve/internal/llm"
)

type callTotals struct {
	failedCalls int
	retries     int
	modelTime   time.Duration
}

func (a *Agent) modelCall(ctx context.Context, chat llm.Session, results []llm.ToolResult, totals *callTotals) (llm.Turn, error) {
	callCtx, trace := llm.WithCallTrace(ctx)
	start := time.Now()
	turn, err := chat.Next(callCtx, results)
	took := time.Since(start)

	retries := max(0, trace.Attempts-1)
	totals.modelTime += took
	totals.retries += retries
	attrs := []any{
		agentRequestAttr(ctx),
		slog.String("provider", a.provider.Name()),
		slog.String("model", a.provider.Model()),
		slog.Duration("duration", took),
		slog.Int("attempts", trace.Attempts),
		slog.Int("retries", retries),
		slog.Int("status", trace.Status),
	}
	if trace.RetryAfter != "" {
		attrs = append(attrs, slog.String("retry_after", trace.RetryAfter))
	}
	if err != nil {
		totals.failedCalls++
		attrs = append(attrs, slog.String("failure_code", string(failureCode(err))))
		var callErr *llm.CallError
		if errors.As(err, &callErr) {
			if callErr.Idle {
				attrs = append(attrs, slog.Bool("idle_timeout", true))
			}
			attrs = appendIfSet(attrs, "provider_error_type", callErr.Type)
			attrs = appendIfSet(attrs, "provider_error_code", callErr.Code)
			attrs = appendIfSet(attrs, "provider_error_param", callErr.Param)
			attrs = appendIfSet(attrs, "request_id", callErr.RequestID)
			attrs = appendIfSet(attrs, "provider_error_message", callErr.ProviderMessage)
		}
		a.logger.WarnContext(ctx, "llm call finished", attrs...)
		return turn, err
	}
	attrs = append(attrs,
		slog.Int64("input_tokens", turn.Usage.InputTokens),
		slog.Int64("output_tokens", turn.Usage.OutputTokens),
		slog.Int64("cache_read_tokens", turn.Usage.CacheReadTokens),
		slog.Int64("cache_write_tokens", turn.Usage.CacheWriteTokens),
		slog.String("stop_reason", string(turn.Stop)),
	)
	a.logger.InfoContext(ctx, "llm call finished", attrs...)
	return turn, nil
}

func appendIfSet(attrs []any, key, value string) []any {
	if value == "" {
		return attrs
	}
	return append(attrs, slog.String(key, value))
}
