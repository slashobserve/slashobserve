package llm_test

import (
	"testing"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/llm"
)

func TestCutResultKeepsARuneBoundaryAndSaysHowMuchWasCut(t *testing.T) {
	cases := []struct {
		name    string
		content string
		n       int
		want    string
	}{
		{"short content is untouched", "abc", 10, "abc"},
		{"exactly the limit is untouched", "abcd", 4, "abcd"},
		{"ascii is cut at the byte limit", "abcdefghij", 4, "abcd\n[cut from 10 bytes; call the tool again for the full result]"},
		{"a multi-byte rune is not split", "abécd", 3, "ab\n[cut from 6 bytes; call the tool again for the full result]"},
		{"a zero limit cuts nothing", "abcdef", 0, "abcdef"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := llm.CutResult(c.content, c.n)
			if got != c.want {
				t.Fatalf("CutResult(%q, %d) = %q, want %q", c.content, c.n, got, c.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("result is not valid UTF-8: %q", got)
			}
		})
	}
}

func TestTailStartKeepsTheLastAssistantAndWholeTurns(t *testing.T) {
	a := func(n int) llm.Weighed { return llm.Weighed{Assistant: true, Bytes: n} }
	u := func(n int) llm.Weighed { return llm.Weighed{Bytes: n} }
	history := []llm.Weighed{a(10), u(10), a(10), u(10), a(10), u(10), a(10)}

	cases := []struct {
		name       string
		messages   []llm.Weighed
		turns      int
		bytes      int
		wantKeepAt int
	}{
		{"no messages", nil, 4, 1000, 0},
		{"no assistant message", []llm.Weighed{u(5)}, 4, 1000, 1},
		{"the last assistant is always kept", history, 1, 1000, 6},
		{"two turns", history, 2, 1000, 4},
		{"more turns than exist keeps all", history, 9, 1000, 0},
		{"the byte limit stops before a turn that does not fit", history, 4, 35, 4},
		{"the last assistant is kept even over the byte limit", history, 4, 1, 6},
		{"zero turns still keeps the last assistant", history, 0, 1000, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := llm.TailStart(c.messages, c.turns, c.bytes); got != c.wantKeepAt {
				t.Fatalf("TailStart = %d, want %d", got, c.wantKeepAt)
			}
		})
	}
}

func TestKeptBytesCapsAtTheResultLimit(t *testing.T) {
	if got := llm.KeptBytes(5000, 100); got != 100 {
		t.Fatalf("capped = %d", got)
	}
	if got := llm.KeptBytes(50, 100); got != 50 {
		t.Fatalf("short = %d", got)
	}
	if got := llm.KeptBytes(5000, 0); got != 5000 {
		t.Fatalf("no limit = %d", got)
	}
}

func TestPrunerAdvancesItsBoundaryOnlyWhenEnoughOldBytesAccumulate(t *testing.T) {
	assistant := llm.PruneMessage{Assistant: true}
	results := func(id string, n int) llm.PruneMessage {
		return llm.PruneMessage{Results: []llm.PruneResult{{ID: id, Bytes: n}}}
	}
	history := []llm.PruneMessage{{}, assistant, results("a", 600), assistant, results("b", 600), assistant, results("c", 600), assistant}

	cases := []struct {
		name     string
		cfg      llm.Pruning
		prunable []string
		want     int
	}{
		{"below the step nothing moves", llm.Pruning{KeepTurns: 2, StepBytes: 2000}, []string{"a", "b", "c"}, 0},
		{"the step passes the boundary before the kept turns", llm.Pruning{KeepTurns: 2, StepBytes: 1000}, []string{"a", "b", "c"}, 5},
		{"results that are not prunable do not count", llm.Pruning{KeepTurns: 2, StepBytes: 1000}, []string{"a"}, 0},
		{"fewer assistant turns than kept turns moves nothing", llm.Pruning{KeepTurns: 9, StepBytes: 1}, []string{"a", "b", "c"}, 0},
		{"zero pruning is off", llm.Pruning{}, []string{"a", "b", "c"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := llm.NewPruner(c.cfg)
			for _, id := range c.prunable {
				p.Note([]llm.ToolResult{{CallID: id, Prunable: true}})
			}
			if got := p.Boundary(history); got != c.want {
				t.Fatalf("boundary = %d, want %d", got, c.want)
			}
			if c.want > 0 {
				p.Reset()
				if got := p.Boundary(history[:2]); got != 0 {
					t.Fatalf("boundary after Reset on a short history = %d", got)
				}
			}
		})
	}
}
