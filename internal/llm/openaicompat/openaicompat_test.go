package openaicompat_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/openaicompat"
)

func chunk(choices, usage string) string {
	return fmt.Sprintf(`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":%s%s}`, choices, usage)
}

func stream(chunks ...string) string {
	var b strings.Builder
	for _, c := range chunks {
		b.WriteString("data: " + c + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func nextTurn(t *testing.T, body string) llm.Turn {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	provider := openaicompat.New(openaicompat.Config{Model: "m", BaseURL: base, MaxTokens: 8192}, slog.New(slog.DiscardHandler))
	session, err := provider.Start(context.Background(), llm.Request{Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := session.Next(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

func TestStopReasonSurvivesTheUsageChunk(t *testing.T) {
	usage := `,"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}`
	toolCall := `{"index":0,"id":"call_1","type":"function","function":{"name":"save","arguments":"{\"title\":\"che"}}`
	cases := map[string]struct {
		chunks []string
		want   llm.StopReason
	}{
		"length then the empty-delta usage chunk of Grid": {
			chunks: []string{
				chunk(`[{"index":0,"delta":{"role":"assistant","content":"partial"}}]`, ""),
				chunk(`[{"index":0,"delta":{},"finish_reason":"length"}]`, ""),
				chunk(`[{"index":0,"delta":{}}]`, usage),
			},
			want: llm.StopMaxTokens,
		},
		"length then the empty-choices usage chunk of OpenAI": {
			chunks: []string{
				chunk(`[{"index":0,"delta":{"role":"assistant","content":"partial"}}]`, ""),
				chunk(`[{"index":0,"delta":{},"finish_reason":"length"}]`, ""),
				chunk(`[]`, usage),
			},
			want: llm.StopMaxTokens,
		},
		"stop then the empty-delta usage chunk": {
			chunks: []string{
				chunk(`[{"index":0,"delta":{"role":"assistant","content":"done"}}]`, ""),
				chunk(`[{"index":0,"delta":{},"finish_reason":"stop"}]`, ""),
				chunk(`[{"index":0,"delta":{}}]`, usage),
			},
			want: llm.StopEndTurn,
		},
		"length with a cut-off tool call is a max tokens cut-off, not tool use": {
			chunks: []string{
				chunk(`[{"index":0,"delta":{"role":"assistant","tool_calls":[`+toolCall+`]}}]`, ""),
				chunk(`[{"index":0,"delta":{},"finish_reason":"length"}]`, ""),
				chunk(`[{"index":0,"delta":{}}]`, usage),
			},
			want: llm.StopMaxTokens,
		},
		"tool_calls then the empty-delta usage chunk": {
			chunks: []string{
				chunk(`[{"index":0,"delta":{"role":"assistant","tool_calls":[`+toolCall+`]}}]`, ""),
				chunk(`[{"index":0,"delta":{},"finish_reason":"tool_calls"}]`, ""),
				chunk(`[{"index":0,"delta":{}}]`, usage),
			},
			want: llm.StopToolUse,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			turn := nextTurn(t, stream(tc.chunks...))
			if turn.Stop != tc.want {
				t.Fatalf("Stop = %q, want %q", turn.Stop, tc.want)
			}
			if turn.Usage.OutputTokens != 5 {
				t.Fatalf("OutputTokens = %d, want 5", turn.Usage.OutputTokens)
			}
		})
	}
}
