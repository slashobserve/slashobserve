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
