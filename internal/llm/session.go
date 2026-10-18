package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
)

func CheckResults(provider string, started bool, pending []string, results []ToolResult) error {
	if started && len(pending) == 0 {
		return fmt.Errorf("%s session: the previous turn made no tool calls, so the session has ended", provider)
	}
	if len(results) != len(pending) {
		return fmt.Errorf("%s session: got %d tool results for %d tool calls", provider, len(results), len(pending))
	}
	for i, r := range results {
		if r.CallID != pending[i] {
			return fmt.Errorf("%s session: tool result %d answers %q, want %q", provider, i, r.CallID, pending[i])
		}
	}
	return nil
}

func PendingIDs(calls []ToolCall) []string {
	var ids []string
	for _, c := range calls {
		ids = append(ids, c.ID)
	}
	return ids
}

type agentRequestKey struct{}

func WithAgentRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, agentRequestKey{}, id)
}

func agentRequestID(ctx context.Context) string {
	id, _ := ctx.Value(agentRequestKey{}).(string)
	return id
}

type APIError struct {
	Status    int
	Type      string
	RequestID string
}

var safeToken = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func IsPlainToken(s string) bool { return safeToken.MatchString(s) }

// Only the status, error type and request ids may reach the log or the returned
// error: the response body, URL and credentials can carry secrets.
func SanitizeError(callerCtx, callCtx context.Context, logger *slog.Logger, provider string, err error, api *APIError) error {
	if ctxErr := callerCtx.Err(); ctxErr != nil {
		return ctxErr
	}
	correlation := []any{}
	if id := agentRequestID(callerCtx); id != "" {
		correlation = append(correlation, slog.String("agent_request_id", id))
	}
	if api != nil {
		attrs := append([]any{slog.String("provider", provider), slog.Int("status", api.Status)}, correlation...)
		if IsPlainToken(api.Type) {
			attrs = append(attrs, slog.String("provider_error_type", api.Type))
		}
		if IsPlainToken(api.RequestID) {
			attrs = append(attrs, slog.String("request_id", api.RequestID))
		}
		logger.Warn("llm request failed", attrs...)
		return fmt.Errorf("%w: %s returned status %d", ErrProvider, provider, api.Status)
	}
	if errors.Is(err, context.DeadlineExceeded) || callCtx.Err() != nil {
		logger.Warn("llm request timed out", append([]any{slog.String("provider", provider)}, correlation...)...)
		return fmt.Errorf("%w: %s request timed out", ErrProvider, provider)
	}
	logger.Warn("llm request failed", append([]any{slog.String("provider", provider)}, correlation...)...)
	return fmt.Errorf("%w: %s request failed", ErrProvider, provider)
}
