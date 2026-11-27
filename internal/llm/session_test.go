package llm_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/llm"
)

func sanitize(t *testing.T, message string, log llm.MessageLog) *llm.CallError {
	t.Helper()
	err := llm.SanitizeError(context.Background(), context.Background(), "anthropic", errors.New("boom"), &llm.APIError{Status: 400, Message: message}, log)
	var callErr *llm.CallError
	if !errors.As(err, &callErr) {
		t.Fatalf("err = %T, want a CallError", err)
	}
	return callErr
}

func TestProviderMessageIsDroppedWhenLoggingIsOff(t *testing.T) {
	callErr := sanitize(t, "prompt fragment UNIQUE-1", llm.MessageLog{APIKey: "sk-secret"})
	if callErr.ProviderMessage != "" {
		t.Fatalf("ProviderMessage = %q, want empty", callErr.ProviderMessage)
	}
}

func TestProviderMessageNeverAppearsInTheErrorText(t *testing.T) {
	callErr := sanitize(t, "prompt fragment UNIQUE-2", llm.MessageLog{Enabled: true})
	if callErr.ProviderMessage != "prompt fragment UNIQUE-2" {
		t.Fatalf("ProviderMessage = %q", callErr.ProviderMessage)
	}
	for _, text := range []string{callErr.Error(), fmt.Sprintf("%v", callErr), fmt.Sprintf("%+v", callErr)} {
		if strings.Contains(text, "UNIQUE-2") {
			t.Fatalf("the provider message leaked into %q", text)
		}
	}
}

func TestProviderMessageScrubbing(t *testing.T) {
	const key = "sk-secret-key"
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{"key removed everywhere", "bad " + key + " and " + key + " again", "bad [redacted] and [redacted] again"},
		{"newlines and tabs become spaces", "line one\nline two\r\n\tline three", "line one line two   line three"},
		{"control characters become spaces", "a\x00b\x1bc\u0085d e", "a b c d e"},
		{"surrounding space is trimmed", "  padded  ", "padded"},
		{"invalid utf-8 is dropped", "ok\xff\xfeend", "okend"},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitize(t, tt.message, llm.MessageLog{Enabled: true, APIKey: key}).ProviderMessage
			if got != tt.want {
				t.Fatalf("ProviderMessage = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderMessageIsCappedOnARuneBoundary(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{"ascii at the cap", strings.Repeat("a", 1024), strings.Repeat("a", 1024)},
		{"ascii over the cap", strings.Repeat("a", 3000), strings.Repeat("a", 1024)},
		{"three byte runes are not split", strings.Repeat("€", 1000), strings.Repeat("€", 341)},
		{"four byte runes are not split", strings.Repeat("\U0001F600", 500), strings.Repeat("\U0001F600", 256)},
		{"two byte rune straddling the cap", strings.Repeat("a", 1023) + "é", strings.Repeat("a", 1023)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitize(t, tt.message, llm.MessageLog{Enabled: true}).ProviderMessage
			if got != tt.want {
				t.Fatalf("got %d bytes, want %d bytes", len(got), len(tt.want))
			}
			if !utf8.ValidString(got) {
				t.Fatal("the cap split a rune")
			}
		})
	}
}

func TestProviderMessageIsCappedAfterTheKeyIsRemoved(t *testing.T) {
	key := strings.Repeat("k", 2000)
	got := sanitize(t, "x"+key, llm.MessageLog{Enabled: true, APIKey: key}).ProviderMessage
	if got != "x[redacted]" {
		t.Fatalf("ProviderMessage = %q", got)
	}
}

func TestCallContextHasADeadlineOnlyForANonZeroTimeout(t *testing.T) {
	tests := []struct {
		timeout      time.Duration
		wantDeadline bool
	}{
		{timeout: 0, wantDeadline: false},
		{timeout: time.Minute, wantDeadline: true},
	}
	for _, tt := range tests {
		ctx, cancel := llm.CallContext(context.Background(), tt.timeout)
		_, has := ctx.Deadline()
		cancel()
		if has != tt.wantDeadline {
			t.Errorf("timeout %s: deadline set = %v, want %v", tt.timeout, has, tt.wantDeadline)
		}
	}
}

func protocolFault(t *testing.T, err error) *llm.ProtocolError {
	t.Helper()
	var fault *llm.ProtocolError
	if !errors.As(err, &fault) || !errors.Is(err, llm.ErrProtocol) {
		t.Fatalf("err = %v (%T), want a ProtocolError", err, err)
	}
	return fault
}

func TestATurnGuardWalksTheProtocol(t *testing.T) {
	g := llm.NewTurnGuard("anthropic")
	if g.State() != llm.TurnStarted {
		t.Fatalf("state = %q, want started", g.State())
	}
	if err := g.Accept(nil); err != nil {
		t.Fatalf("the first turn takes no results: %v", err)
	}
	g.Answered([]llm.ToolCall{{ID: "a"}, {ID: "b"}})
	if g.State() != llm.TurnAwaitingResults {
		t.Fatalf("state = %q, want awaiting_results", g.State())
	}
	if err := g.Accept([]llm.ToolResult{{CallID: "a"}, {CallID: "b"}}); err != nil {
		t.Fatalf("results in call order are accepted: %v", err)
	}
	g.Answered(nil)
	if g.State() != llm.TurnEnded {
		t.Fatalf("state = %q, want ended", g.State())
	}
	g.Reopen()
	if g.State() != llm.TurnCompacted {
		t.Fatalf("state = %q, want compacted", g.State())
	}
	if err := g.Accept(nil); err != nil {
		t.Fatalf("a compacted session takes the next turn: %v", err)
	}
}

func TestATurnGuardNamesEachFault(t *testing.T) {
	tests := []struct {
		name    string
		calls   []llm.ToolCall
		results []llm.ToolResult
		fault   llm.ProtocolFault
		state   llm.TurnState
		text    string
	}{
		{"a turn after the answer", nil, nil, llm.FaultSessionEnded, llm.TurnEnded, "anthropic session: the previous turn made no tool calls, so the session has ended"},
		{"a missing result", []llm.ToolCall{{ID: "a"}}, nil, llm.FaultResultCount, llm.TurnAwaitingResults, "anthropic session: got 0 tool results for 1 tool calls"},
		{"a result for another call", []llm.ToolCall{{ID: "a"}}, []llm.ToolResult{{CallID: "other"}}, llm.FaultResultMismatch, llm.TurnAwaitingResults, `anthropic session: tool result 0 answers "other", want "a"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := llm.NewTurnGuard("anthropic")
			g.Answered(tt.calls)
			fault := protocolFault(t, g.Accept(tt.results))
			if fault.Fault != tt.fault || fault.State != tt.state || fault.Provider != "anthropic" || fault.Error() != tt.text {
				t.Fatalf("fault = %+v (%q)", fault, fault.Error())
			}
			if g.State() != tt.state {
				t.Fatalf("a refused turn moved the guard to %q", g.State())
			}
		})
	}
}

func TestReopeningOnlyMovesAnEndedSession(t *testing.T) {
	g := llm.NewTurnGuard("openai")
	g.Answered([]llm.ToolCall{{ID: "a"}})
	g.Reopen()
	if g.State() != llm.TurnAwaitingResults {
		t.Fatalf("state = %q, want awaiting_results", g.State())
	}
	if fault := protocolFault(t, g.Accept(nil)); fault.Fault != llm.FaultResultCount {
		t.Fatalf("fault = %q, want tool_result_count", fault.Fault)
	}
}
