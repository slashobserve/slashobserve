package health

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/health/repairprompt"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestRepairPromptNamesEachFailingPanelAndVariable(t *testing.T) {
	got, ok := repairPrompt([]Problem{
		{Kind: KindNoData, PanelID: "rps", PanelTitle: "Requests"},
		{Kind: KindQueryFailed, PanelID: "lat"},
		{Kind: KindVariableFailed, Variable: "pod"},
		{Kind: KindDuplicateTitle},
		{Kind: KindUnopened},
		{Kind: KindQueryFailed, Code: "check_failed"},
		{Kind: KindNoData, PanelID: "rps", PanelTitle: "Requests"},
	})
	want := repairprompt.Prefix +
		"\n- panel rps (\"Requests\") returns no data" +
		"\n- panel lat fails" +
		"\n- variable pod resolves to nothing"
	if !ok || got != want {
		t.Errorf("repairPrompt = %q, %v\nwant %q", got, ok, want)
	}
	if _, valid := revisions.CleanPrompt(got); !valid {
		t.Error("the prompt is not a valid agent prompt")
	}
}

func TestRepairPromptHasNothingToRepairWithoutAPanelOrVariable(t *testing.T) {
	cases := [][]Problem{
		nil,
		{{Kind: KindDuplicateTitle}, {Kind: KindUnopened}},
		{{Kind: KindQueryFailed, Code: "check_failed"}},
	}
	for _, problems := range cases {
		if got, ok := repairPrompt(problems); ok {
			t.Errorf("repairPrompt(%+v) = %q, want nothing to repair", problems, got)
		}
	}
}

func TestRepairPromptDropsControlCharactersAndCapsTitles(t *testing.T) {
	got, ok := repairPrompt([]Problem{{Kind: KindNoData, PanelID: "a\u202eb", PanelTitle: "x\x00y\n" + strings.Repeat("t", 200)}})
	want := repairprompt.Prefix + "\n- panel ab (\"xy" + strings.Repeat("t", 78) + "\") returns no data"
	if !ok || got != want {
		t.Errorf("repairPrompt = %q\nwant %q", got, want)
	}
}

func TestRepairPromptStaysWithinThePromptLimit(t *testing.T) {
	var problems []Problem
	for i := range 200 {
		problems = append(problems, Problem{Kind: KindQueryFailed, PanelID: "panel_" + strings.Repeat("x", 20) + string(rune('a'+i%26)) + string(rune('a'+i/26)), PanelTitle: strings.Repeat("T", 60)})
	}
	got, ok := repairPrompt(problems)
	if !ok {
		t.Fatal("nothing to repair")
	}
	if n := utf8.RuneCountInString(got); n > revisions.MaxPromptRunes {
		t.Errorf("prompt has %d runes, over %d", n, revisions.MaxPromptRunes)
	}
	if !strings.HasPrefix(got, repairprompt.Prefix) || !strings.Contains(got, " more") {
		t.Errorf("prompt does not end with the remaining count: %q", got[len(got)-40:])
	}
	if _, valid := revisions.CleanPrompt(got); !valid {
		t.Error("the prompt is not a valid agent prompt")
	}
}
