package replay

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/llm"
)

func TestFixtureAllowsTurnsAfterATurnWithoutToolCalls(t *testing.T) {
	content := `{"prompt":"p","turns":[{"tool_calls":[{"name":"t"}]},{"text":"done"},{"tool_calls":[{"name":"t"}]}]}`
	if _, err := parseFixture("f.json", []byte(content)); err != nil {
		t.Fatalf("parseFixture: %v", err)
	}
}

func TestFixtureAllowsToolCallsOnTheLastTurn(t *testing.T) {
	content := `{"prompt":"p","turns":[{"tool_calls":[{"name":"t"}]},{"tool_calls":[{"name":"t"}],"expect":[{"is_error":false}]},` +
		`{"tool_calls":[{"name":"t"}],"expect":[{"is_error":false}]}]}`
	if _, err := parseFixture("f.json", []byte(content)); err != nil {
		t.Fatalf("parseFixture: %v", err)
	}
}

func TestFixtureFaultValidation(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{"fault without text or calls", `{"prompt":"p","turns":[{"fault":{"kind":"refusal"}}]}`, ""},
		{"delay with a duration", `{"prompt":"p","turns":[{"fault":{"kind":"delay","delay_ms":5},"text":"ok"}]}`, ""},
		{"unknown kind", `{"prompt":"p","turns":[{"fault":{"kind":"meltdown"},"text":"x"}]}`, `kind "meltdown" is not one of`},
		{"missing kind", `{"prompt":"p","turns":[{"fault":{},"text":"x"}]}`, "is not one of"},
		{"negative delay", `{"prompt":"p","turns":[{"fault":{"kind":"delay","delay_ms":-1}}]}`, "delay_ms must not be negative"},
		{"unknown fault field", `{"prompt":"p","turns":[{"fault":{"kind":"delay","ms":1}}]}`, "unknown field"},
		{"turn without fault still needs content", `{"prompt":"p","turns":[{}]}`, "needs text or tool_calls"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseFixture("f.json", []byte(c.content))
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("parseFixture: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, c.wantErr)
			}
		})
	}
}

func TestFixtureResumeSegmentValidation(t *testing.T) {
	const turns = `"turns":[{"tool_calls":[{"name":"t"}]}]`
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{"valid segments", `{"prompt":"p",` + turns + `,"resumes":{"1":{"turns":[{"text":"a"}]},"12":{"turns":[{"text":"b"}]}}}`, ""},
		{"zero key", `{"prompt":"p",` + turns + `,"resumes":{"0":{"turns":[{"text":"a"}]}}}`, `key "0" is not a positive integer`},
		{"negative key", `{"prompt":"p",` + turns + `,"resumes":{"-1":{"turns":[{"text":"a"}]}}}`, `key "-1" is not a positive integer`},
		{"padded key", `{"prompt":"p",` + turns + `,"resumes":{"01":{"turns":[{"text":"a"}]}}}`, `key "01" is not a positive integer`},
		{"word key", `{"prompt":"p",` + turns + `,"resumes":{"one":{"turns":[{"text":"a"}]}}}`, `key "one" is not a positive integer`},
		{"empty segment", `{"prompt":"p",` + turns + `,"resumes":{"1":{"turns":[]}}}`, `resumes."1".turns needs at least one turn`},
		{"turn after a text turn in segment", `{"prompt":"p",` + turns + `,"resumes":{"2":{"turns":[{"text":"a"},{"text":"b"}]}}}`, ""},
		{"empty turn in segment", `{"prompt":"p",` + turns + `,"resumes":{"1":{"turns":[{}]}}}`, "needs text or tool_calls"},
		{"unknown segment field", `{"prompt":"p",` + turns + `,"resumes":{"1":{"turns":[{"text":"a"}],"extra":1}}}`, "unknown field"},
		{"turns stay required", `{"prompt":"p","resumes":{"1":{"turns":[{"text":"a"}]}}}`, "turns needs at least one turn"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseFixture("f.json", []byte(c.content))
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("parseFixture: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, c.wantErr)
			}
		})
	}
}

func newSession(t *testing.T, maxTokens int, content string) llm.Session {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := New(dir, maxTokens, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	s, err := p.Start(t.Context(), llm.Request{Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestInjectedCallErrorsFailOnceThenTheTurnAnswers(t *testing.T) {
	cases := []struct {
		kind    string
		failure llm.CallFailure
		status  int
	}{
		{"rate_limited", llm.RateLimited, 429},
		{"server_error", llm.ProviderError, 500},
		{"call_timeout", llm.CallTimeout, 0},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			s := newSession(t, 0, `{"prompt":"p","turns":[{"fault":{"kind":"`+c.kind+`"},"text":"recovered"}]}`)
			_, err := s.Next(t.Context(), nil)
			var callErr *llm.CallError
			if !errors.As(err, &callErr) || callErr.Failure != c.failure || callErr.Status != c.status {
				t.Fatalf("first call error = %v, want failure %s status %d", err, c.failure, c.status)
			}
			turn, err := s.Next(t.Context(), nil)
			if err != nil || turn.Text != "recovered" || turn.Stop != llm.StopEndTurn {
				t.Fatalf("retry = %+v, %v, want the turn's text", turn, err)
			}
		})
	}
}

func TestSpentFaultDoesNotCarryToTheNextTurn(t *testing.T) {
	s := newSession(t, 0, `{"prompt":"p","turns":[`+
		`{"fault":{"kind":"server_error"},"tool_calls":[{"name":"t"}]},`+
		`{"fault":{"kind":"server_error"},"text":"done"}]}`)
	if _, err := s.Next(t.Context(), nil); err == nil {
		t.Fatal("first turn did not fail")
	}
	first, err := s.Next(t.Context(), nil)
	if err != nil || len(first.ToolCalls) != 1 {
		t.Fatalf("retry = %+v, %v", first, err)
	}
	if _, err := s.Next(t.Context(), []llm.ToolResult{{CallID: first.ToolCalls[0].ID}}); err == nil {
		t.Fatal("second turn's own fault did not fire")
	}
}

func TestInjectedStopReasons(t *testing.T) {
	cases := []struct {
		kind string
		stop llm.StopReason
	}{
		{"max_tokens", llm.StopMaxTokens},
		{"refusal", llm.StopRefusal},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			s := newSession(t, 0, `{"prompt":"p","turns":[{"fault":{"kind":"`+c.kind+`"},"tool_calls":[{"name":"t"}]}]}`)
			turn, err := s.Next(t.Context(), nil)
			if err != nil || turn.Stop != c.stop || len(turn.ToolCalls) != 0 || turn.Usage.Calls != 1 {
				t.Fatalf("turn = %+v, %v, want stop %s with no tool calls", turn, err, c.stop)
			}
		})
	}
}

func TestDelayWaitsThenAnswers(t *testing.T) {
	s := newSession(t, 0, `{"prompt":"p","turns":[{"fault":{"kind":"delay","delay_ms":50},"text":"late"}]}`)
	start := time.Now()
	turn, err := s.Next(t.Context(), nil)
	if err != nil || turn.Text != "late" {
		t.Fatalf("turn = %+v, %v", turn, err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Fatalf("answered after %v, want at least 50ms", elapsed)
	}
}

func TestDelayStopsWhenTheContextEnds(t *testing.T) {
	s := newSession(t, 0, `{"prompt":"p","turns":[{"fault":{"kind":"delay","delay_ms":60000},"text":"late"}]}`)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := s.Next(ctx, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want the context deadline", err)
	}
}

func inputOf3200Bytes() string {
	return `{"v":"` + strings.Repeat("a", 3192) + `"}`
}

func TestOutputTokensAreEstimatedFromToolCallInput(t *testing.T) {
	input := inputOf3200Bytes()
	if len(input) != 3200 {
		t.Fatalf("test input is %d bytes", len(input))
	}
	s := newSession(t, 0, `{"prompt":"p","turns":[{"tool_calls":[{"name":"t","input":`+input+`},{"name":"t","input":{"k":1}}]}]}`)
	turn, err := s.Next(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(1003); turn.Usage.OutputTokens != want {
		t.Fatalf("OutputTokens = %d, want %d (3207 bytes at 3.2 bytes per token, rounded up)", turn.Usage.OutputTokens, want)
	}
}

func TestOversizedTurnStopsAtTheTokenCapWithoutRunningCalls(t *testing.T) {
	content := `{"prompt":"p","turns":[{"tool_calls":[{"name":"t","input":` + inputOf3200Bytes() + `}]}]}`
	turn, err := newSession(t, 999, content).Next(t.Context(), nil)
	if err != nil || turn.Stop != llm.StopMaxTokens || len(turn.ToolCalls) != 0 || turn.Usage.OutputTokens != 999 {
		t.Fatalf("turn = %+v, %v, want max_tokens with 999 output tokens and no calls", turn, err)
	}
	turn, err = newSession(t, 1000, content).Next(t.Context(), nil)
	if err != nil || turn.Stop != llm.StopToolUse || len(turn.ToolCalls) != 1 {
		t.Fatalf("turn at the cap = %+v, %v, want the tool call", turn, err)
	}
}

const saveAfterFixture = `{"prompt":"p","resume_after_saves":true,"turns":[` +
	`{"tool_calls":[{"name":"create_dashboard"}]},` +
	`{"tool_calls":[{"name":"list_metrics"}]},` +
	`{"tool_calls":[{"name":"update_panels"}]},` +
	`{"tool_calls":[{"name":"update_panels"}]}]}`

func TestResumeAfterSavesStartsAfterTheKthSaveTurn(t *testing.T) {
	f, err := parseFixture("f.json", []byte(saveAfterFixture))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		saves     int
		wantFirst string
		wantTurns int
		wantFound bool
	}{
		{"a fresh session reads every turn", 0, "create_dashboard", 4, true},
		{"after the first save the read turn follows", 1, "list_metrics", 3, true},
		{"a read turn between saves is not counted", 2, "update_panels", 1, true},
		{"after the last save nothing is left", 3, "", 0, true},
		{"past the last save there is no fixture", 4, "", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			turns, ok := f.turnsFor(c.saves)
			if ok != c.wantFound || len(turns) != c.wantTurns {
				t.Fatalf("turnsFor(%d) = %d turns, found %v; want %d turns, found %v", c.saves, len(turns), ok, c.wantTurns, c.wantFound)
			}
			if c.wantFirst != "" && turns[0].ToolCalls[0].Name != c.wantFirst {
				t.Fatalf("first resumed call = %s, want %s", turns[0].ToolCalls[0].Name, c.wantFirst)
			}
		})
	}
}

func TestResumeAfterSavesValidation(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{"valid", saveAfterFixture, ""},
		{"with resumes", `{"prompt":"p","resume_after_saves":true,"turns":[{"tool_calls":[{"name":"update_panels"}]}],"resumes":{"1":{"turns":[{"text":"a"}]}}}`, "cannot be combined with resumes"},
		{"two saves in one turn", `{"prompt":"p","resume_after_saves":true,"turns":[{"tool_calls":[{"name":"update_panels"},{"name":"update_panels"}]}]}`, "turn 0 has 2 save calls"},
		{"a read and a save in one turn", `{"prompt":"p","resume_after_saves":true,"turns":[{"tool_calls":[{"name":"lint_query"},{"name":"update_panels"}]}]}`, ""},
		{"expect after a save", `{"prompt":"p","resume_after_saves":true,"turns":[{"tool_calls":[{"name":"update_panels"}]},{"tool_calls":[{"name":"lint_query"}],"expect":[{"is_error":false}]}]}`, "expect is not allowed on the turn after a save"},
		{"expect after a read", `{"prompt":"p","resume_after_saves":true,"turns":[{"tool_calls":[{"name":"lint_query"}]},{"tool_calls":[{"name":"lint_query"}],"expect":[{"is_error":false}]}]}`, ""},
		{"two saves are fine without the flag", `{"prompt":"p","turns":[{"tool_calls":[{"name":"update_panels"},{"name":"update_panels"}]}]}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseFixture("f.json", []byte(c.content))
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("parseFixture: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, c.wantErr)
			}
		})
	}
}

func TestResumeAfterSavesServesTheResumedSession(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.json"), []byte(saveAfterFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := New(dir, 0, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	s, err := p.Start(t.Context(), llm.Request{Prompt: "p", BuildSaves: 2})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.Next(t.Context(), nil)
	if err != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "update_panels" {
		t.Fatalf("resumed turn = %+v, err %v", turn, err)
	}
	if _, err := p.Start(t.Context(), llm.Request{Prompt: "p", BuildSaves: 4}); !errors.Is(err, llm.ErrNoFixture) {
		t.Fatalf("a resume past the last save: err = %v, want ErrNoFixture", err)
	}
}
