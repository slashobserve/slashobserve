package claims

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestCleanUsage(t *testing.T) {
	cost := func(v float64) *float64 { return &v }
	long := strings.Repeat("m", 300)

	if cleanUsage(nil) != nil {
		t.Fatal("nil usage did not stay nil")
	}
	for _, tc := range []struct {
		name     string
		in       FinishUsage
		model    string
		input    int64
		output   int64
		wantCost *float64
	}{
		{"long model name is cut", FinishUsage{Model: long}, long[:100], 0, 0, nil},
		{"text is trimmed", FinishUsage{Provider: "  anthropic ", Model: " claude "}, "claude", 0, 0, nil},
		{"negative tokens become zero", FinishUsage{InputTokens: -5, OutputTokens: 7}, "", 0, 7, nil},
		{"NaN cost is dropped", FinishUsage{CostUSD: cost(math.NaN())}, "", 0, 0, nil},
		{"infinite cost is dropped", FinishUsage{CostUSD: cost(math.Inf(1))}, "", 0, 0, nil},
		{"negative cost is dropped", FinishUsage{CostUSD: cost(-0.01)}, "", 0, 0, nil},
		{"cost above the ceiling is dropped", FinishUsage{CostUSD: cost(20000)}, "", 0, 0, nil},
		{"a reported cost is kept", FinishUsage{CostUSD: cost(0.42)}, "", 0, 0, cost(0.42)},
		{"zero cost is kept", FinishUsage{CostUSD: cost(0)}, "", 0, 0, cost(0)},
	} {
		got := cleanUsage(&tc.in)
		if got.Model != tc.model || got.InputTokens != tc.input || got.OutputTokens != tc.output {
			t.Errorf("%s: usage = model %q input %d output %d", tc.name, got.Model, got.InputTokens, got.OutputTokens)
		}
		switch {
		case tc.wantCost == nil && got.CostUSD != nil:
			t.Errorf("%s: cost = %v, want none", tc.name, *got.CostUSD)
		case tc.wantCost != nil && (got.CostUSD == nil || *got.CostUSD != *tc.wantCost):
			t.Errorf("%s: cost = %v, want %v", tc.name, got.CostUSD, *tc.wantCost)
		}
	}
	if got := cleanUsage(&FinishUsage{Provider: "  anthropic "}); got.Provider != "anthropic" {
		t.Errorf("provider = %q, want it trimmed", got.Provider)
	}
}

func TestFinishRejectsAnUnknownOutcomeBeforeLoadingTheRun(t *testing.T) {
	s := &Service{}
	for _, outcome := range []string{"", "success", "Succeeded", "canceled"} {
		if _, err := s.Finish(t.Context(), grantClaimer, FinishInput{Outcome: outcome}); !errors.Is(err, ErrInvalidOutcome) {
			t.Errorf("outcome %q: error = %v, want ErrInvalidOutcome", outcome, err)
		}
	}
}

func TestEndingOfAFinishInput(t *testing.T) {
	if got := endingOf(FinishInput{Outcome: "succeeded", Message: "ignored"}); got.Failure != "" || got.Message != "" {
		t.Errorf("succeeded ending = %+v, want no failure", got)
	}
	if got := endingOf(FinishInput{Outcome: "failed", Message: "  no data \x00 "}); got.Failure != "agent_failed" || got.Message != "no data" {
		t.Errorf("failed ending = %+v, want the cleaned message", got)
	}
	if got := endingOf(FinishInput{Outcome: "failed", Message: "   "}); got.Message != "the agent did not say why" {
		t.Errorf("failed ending without a message = %q", got.Message)
	}
	if got := endingOf(FinishInput{Outcome: "failed", Message: strings.Repeat("é", 800)}); len([]rune(got.Message)) != 500 {
		t.Errorf("failed message has %d runes, want 500", len([]rune(got.Message)))
	}
}

func TestCleanTextCutsAtRunesAndDropsInvalidBytes(t *testing.T) {
	for name, tc := range map[string]struct {
		in   string
		max  int
		want string
	}{
		"trims":            {"  hi  ", 10, "hi"},
		"cuts by rune":     {"héllo wörld", 5, "héllo"},
		"drops NUL":        {"a\x00b", 10, "ab"},
		"drops bad UTF-8":  {"a\xffb", 10, "ab"},
		"only whitespace":  {" \n\t ", 10, ""},
		"exactly at limit": {"abcde", 5, "abcde"},
	} {
		if got := cleanText(tc.in, tc.max); got != tc.want {
			t.Errorf("%s: cleanText(%q, %d) = %q, want %q", name, tc.in, tc.max, got, tc.want)
		}
	}
}
