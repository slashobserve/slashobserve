package repairprompt

import (
	"slices"
	"testing"
)

func TestPanelIDsReadBackWhatPanelLineWrote(t *testing.T) {
	prompt := Prefix +
		"\n" + PanelLine("rps", "Requests (p99) \"quoted\"", "returns no data") +
		"\n" + PanelLine("lat", "", "fails") +
		"\n- variable pod resolves to nothing" +
		"\n" + PanelLine("rps", "Requests", "fails") +
		"\n- and 3 more"
	if got, want := PanelIDs(prompt), []string{"rps", "lat"}; !slices.Equal(got, want) {
		t.Errorf("PanelIDs = %v, want %v", got, want)
	}
}

func TestPanelIDsOfAPromptWithoutPanelLinesIsEmpty(t *testing.T) {
	for _, prompt := range []string{"", Prefix, "make a heatmap", "- panel \n- panel  x"} {
		if got := PanelIDs(prompt); len(got) != 0 {
			t.Errorf("PanelIDs(%q) = %v, want none", prompt, got)
		}
	}
}

func TestIsMatchesOnlyAPromptThatStartsWithThePrefix(t *testing.T) {
	cases := map[string]bool{
		Prefix:                       true,
		Prefix + "\n- panel a fails": true,
		"please " + Prefix:           false,
		"Repair the panels":          false,
		"":                           false,
	}
	for prompt, want := range cases {
		if got := Is(prompt); got != want {
			t.Errorf("Is(%q) = %v, want %v", prompt, got, want)
		}
	}
}
