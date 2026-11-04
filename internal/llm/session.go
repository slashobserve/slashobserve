package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxProviderMessageBytes = 1024

const redactedKey = "[redacted]"

func CallContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

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

type APIError struct {
	Status    int
	Type      string
	Code      string
	Param     string
	RequestID string
	Message   string

	RetryAfter time.Duration
}

type MessageLog struct {
	Enabled bool
	APIKey  string
}

var safeToken = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func IsPlainToken(s string) bool { return safeToken.MatchString(s) }

// Only the status, error type, code, param and request id leave this function, plus the scrubbed provider message when logging it is on: the response body, URL and credentials can carry secrets.
func SanitizeError(callerCtx, callCtx context.Context, provider string, err error, api *APIError, messageLog MessageLog) error {
	if ctxErr := callerCtx.Err(); ctxErr != nil {
		return ctxErr
	}
	if api != nil {
		failure := ProviderError
		if api.Status == http.StatusTooManyRequests {
			failure = RateLimited
		}
		callErr := NewCallError(provider, failure, api.Status)
		callErr.Type = plainToken(api.Type)
		callErr.Code = plainToken(api.Code)
		callErr.Param = plainToken(api.Param)
		callErr.RequestID = plainToken(api.RequestID)
		callErr.RetryAfter = api.RetryAfter
		if messageLog.Enabled {
			callErr.ProviderMessage = scrubMessage(api.Message, messageLog.APIKey)
		}
		return callErr
	}
	if errors.Is(err, context.DeadlineExceeded) || callCtx.Err() != nil {
		return NewCallError(provider, CallTimeout, 0)
	}
	return NewCallError(provider, ProviderError, 0)
}

func plainToken(s string) string {
	if IsPlainToken(s) {
		return s
	}
	return ""
}

func scrubMessage(message, apiKey string) string {
	if apiKey != "" {
		message = strings.ReplaceAll(message, apiKey, redactedKey)
	}
	message = strings.ToValidUTF8(message, "")
	message = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return ' '
	}, message)
	message = strings.TrimSpace(message)
	if len(message) <= maxProviderMessageBytes {
		return message
	}
	end := maxProviderMessageBytes
	for end > 0 && !utf8.RuneStart(message[end]) {
		end--
	}
	return message[:end]
}
