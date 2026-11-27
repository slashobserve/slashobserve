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

type TurnState string

const (
	TurnStarted         TurnState = "started"
	TurnAwaitingResults TurnState = "awaiting_results"
	TurnEnded           TurnState = "ended"
	TurnCompacted       TurnState = "compacted"
)

type ProtocolFault string

const (
	FaultSessionEnded   ProtocolFault = "session_ended"
	FaultResultCount    ProtocolFault = "tool_result_count"
	FaultResultMismatch ProtocolFault = "tool_result_mismatch"
)

var ErrProtocol = errors.New("llm turn protocol violated")

type ProtocolError struct {
	Provider string
	Fault    ProtocolFault
	State    TurnState
	text     string
}

func (e *ProtocolError) Error() string { return e.text }

func (e *ProtocolError) Unwrap() error { return ErrProtocol }

type TurnGuard struct {
	provider string
	state    TurnState
	pending  []string
}

func NewTurnGuard(provider string) *TurnGuard {
	return &TurnGuard{provider: provider, state: TurnStarted}
}

func (g *TurnGuard) State() TurnState { return g.state }

func (g *TurnGuard) fault(fault ProtocolFault, format string, args ...any) error {
	return &ProtocolError{Provider: g.provider, Fault: fault, State: g.state, text: g.provider + " session: " + fmt.Sprintf(format, args...)}
}

func (g *TurnGuard) Accept(results []ToolResult) error {
	if g.state == TurnEnded {
		return g.fault(FaultSessionEnded, "the previous turn made no tool calls, so the session has ended")
	}
	if len(results) != len(g.pending) {
		return g.fault(FaultResultCount, "got %d tool results for %d tool calls", len(results), len(g.pending))
	}
	for i, r := range results {
		if r.CallID != g.pending[i] {
			return g.fault(FaultResultMismatch, "tool result %d answers %q, want %q", i, r.CallID, g.pending[i])
		}
	}
	return nil
}

func (g *TurnGuard) Answered(calls []ToolCall) {
	g.pending = g.pending[:0]
	for _, c := range calls {
		g.pending = append(g.pending, c.ID)
	}
	g.state = TurnAwaitingResults
	if len(calls) == 0 {
		g.state = TurnEnded
	}
}

func (g *TurnGuard) Reopen() {
	if g.state == TurnEnded {
		g.state = TurnCompacted
	}
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
