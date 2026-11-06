package replay

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/llm"
)

const resumeFixture = `{"prompt":"p",
"turns":[{"tool_calls":[{"name":"save"}]},{"expect":[{"is_error":false}],"text":"first segment done"}],
"resumes":{
 "1":{"turns":[{"text":"after one save"}]},
 "3":{"turns":[{"fault":{"kind":"server_error"},"tool_calls":[{"name":"save"}]},{"tool_calls":[{"name":"save"}],"expect":[{"is_error":false}]}]}
}}`

func provider(t *testing.T, content string) *Provider {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := New(dir, 0, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStartPicksTheSegmentByBuildSaves(t *testing.T) {
	p := provider(t, resumeFixture)
	cases := []struct {
		buildSaves int
		wantText   string
		wantCalls  int
	}{
		{0, "", 1},
		{1, "after one save", 0},
	}
	for _, c := range cases {
		s, err := p.Start(t.Context(), llm.Request{Prompt: "p", BuildSaves: c.buildSaves})
		if err != nil {
			t.Fatalf("Start with %d build saves: %v", c.buildSaves, err)
		}
		turn, err := s.Next(t.Context(), nil)
		if err != nil || turn.Text != c.wantText || len(turn.ToolCalls) != c.wantCalls {
			t.Fatalf("first turn with %d build saves = %+v, %v", c.buildSaves, turn, err)
		}
	}
}

func TestStartWithoutTheSegmentIsNoFixture(t *testing.T) {
	p := provider(t, resumeFixture)
	for _, buildSaves := range []int{2, 4, -1} {
		if _, err := p.Start(t.Context(), llm.Request{Prompt: "p", BuildSaves: buildSaves}); !errors.Is(err, llm.ErrNoFixture) {
			t.Fatalf("Start with %d build saves: error = %v, want ErrNoFixture", buildSaves, err)
		}
	}
	noResumes := provider(t, `{"prompt":"p","turns":[{"text":"x"}]}`)
	if _, err := noResumes.Start(t.Context(), llm.Request{Prompt: "p", BuildSaves: 1}); !errors.Is(err, llm.ErrNoFixture) {
		t.Fatalf("a fixture without resumes: error = %v, want ErrNoFixture", err)
	}
}

func TestFaultInAResumeSegmentFires(t *testing.T) {
	s, err := provider(t, resumeFixture).Start(t.Context(), llm.Request{Prompt: "p", BuildSaves: 3})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Next(t.Context(), nil)
	var callErr *llm.CallError
	if !errors.As(err, &callErr) || callErr.Failure != llm.ProviderError {
		t.Fatalf("first call error = %v, want the injected server error", err)
	}
	turn, err := s.Next(t.Context(), nil)
	if err != nil || len(turn.ToolCalls) != 1 {
		t.Fatalf("retry = %+v, %v, want the segment's tool call", turn, err)
	}
}

func TestPastTheLastTurnTheSessionEndsTheTurn(t *testing.T) {
	s := newSession(t, 0, `{"prompt":"p","turns":[{"tool_calls":[{"name":"t"},{"name":"t"}]}]}`)
	first, err := s.Next(t.Context(), nil)
	if err != nil || len(first.ToolCalls) != 2 {
		t.Fatalf("first = %+v, %v", first, err)
	}
	results := []llm.ToolResult{{CallID: first.ToolCalls[0].ID}, {CallID: first.ToolCalls[1].ID}}
	end, err := s.Next(t.Context(), results)
	if err != nil || end.Stop != llm.StopEndTurn || len(end.ToolCalls) != 0 || end.Text != "" || end.Usage.Calls != 1 {
		t.Fatalf("past the end = %+v, %v, want an end turn counting one call", end, err)
	}
}

func TestPastTheLastTurnStillChecksResults(t *testing.T) {
	cases := []struct {
		name    string
		results func(first llm.Turn) []llm.ToolResult
		wantErr string
	}{
		{"missing result", func(llm.Turn) []llm.ToolResult { return nil }, "got 0 tool results for 1 tool calls"},
		{"wrong id", func(llm.Turn) []llm.ToolResult { return []llm.ToolResult{{CallID: "other"}} }, `tool result 0 answers "other"`},
		{"unexpected error", func(first llm.Turn) []llm.ToolResult {
			return []llm.ToolResult{{CallID: first.ToolCalls[0].ID, IsError: true}}
		}, "is_error was true, expected false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSession(t, 0, `{"prompt":"p","turns":[{"tool_calls":[{"name":"t"}]}]}`)
			first, err := s.Next(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Next(t.Context(), c.results(first)); err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, c.wantErr)
			}
		})
	}
}

func TestPastTheLastTurnOfAResumeSegmentEndsTheTurn(t *testing.T) {
	s, err := provider(t, resumeFixture).Start(t.Context(), llm.Request{Prompt: "p", BuildSaves: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Next(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Compact(llm.Compaction{Message: "shortened", TailTurns: 4, TailBytes: 1 << 20, ResultBytes: 100}); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	end, err := s.Next(t.Context(), nil)
	if err != nil || end.Stop != llm.StopEndTurn || len(end.ToolCalls) != 0 {
		t.Fatalf("past the segment's end = %+v, %v, want an end turn", end, err)
	}
}

const nudgeFixture = `{"prompt":"p","turns":[{"tool_calls":[{"name":"save"}]},{"text":"that is all"},{"tool_calls":[{"name":"save"}]}]}`

func TestNextAfterATurnWithoutToolCallsNeedsACompactionFirst(t *testing.T) {
	s := newSession(t, 0, nudgeFixture)
	first, err := s.Next(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := s.Next(t.Context(), []llm.ToolResult{{CallID: first.ToolCalls[0].ID}})
	if err != nil || answer.Text != "that is all" || len(answer.ToolCalls) != 0 {
		t.Fatalf("answer = %+v, %v", answer, err)
	}
	if _, err := s.Next(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "the session has ended") {
		t.Fatalf("Next after the answer: error = %v, want the session has ended", err)
	}
	if err := s.Compact(llm.Compaction{Message: "shortened", TailTurns: 4, TailBytes: 1 << 20, ResultBytes: 100}); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	third, err := s.Next(t.Context(), nil)
	if err != nil || len(third.ToolCalls) != 1 || third.Stop != llm.StopToolUse {
		t.Fatalf("turn after the compaction = %+v, %v, want the fixture's next turn", third, err)
	}
}

func TestCompactKeepsTheReplayPosition(t *testing.T) {
	p := provider(t, resumeFixture)
	s, err := p.Start(t.Context(), llm.Request{Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Next(t.Context(), nil)
	if err != nil || len(first.ToolCalls) != 1 {
		t.Fatalf("first turn = %+v, %v", first, err)
	}
	if err := s.Compact(llm.Compaction{Message: "shortened", TailTurns: 4, TailBytes: 1 << 20, ResultBytes: 100}); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	second, err := s.Next(t.Context(), []llm.ToolResult{{CallID: first.ToolCalls[0].ID, Content: "{}"}})
	if err != nil || second.Text != "first segment done" {
		t.Fatalf("turn after Compact = %+v, %v", second, err)
	}
}
