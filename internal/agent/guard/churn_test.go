package guard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

func TestChurnCountsChangesPerPanel(t *testing.T) {
	var c State
	for i, id := range []string{"a", "b", "c"} {
		if note, stop := c.noteChurn(tools.Edits{Panels: []string{id}}); note != "" || stop {
			t.Fatalf("add %d of a new panel churned: %q, %v", i, note, stop)
		}
	}
	for edit := 2; edit <= maxChangesPerPanel; edit++ {
		if note, stop := c.noteChurn(tools.Edits{Panels: []string{"a"}}); note != "" || stop {
			t.Fatalf("change %d of one panel churned: %q, %v", edit, note, stop)
		}
	}
	note, stop := c.noteChurn(tools.Edits{Panels: []string{"a"}})
	if want := "This save changed no panel that still needs work. Leave a as it is, save the next panel, or set finish: true when every planned panel is built."; note != want || stop {
		t.Fatalf("fifth change of one panel = %q, %v", note, stop)
	}
	if note, stop := c.noteChurn(tools.Edits{Panels: []string{"d"}}); note != "" || stop || c.ChurnedInRow != 0 {
		t.Fatalf("a new panel after churn = %q, %v, inRow %d", note, stop, c.ChurnedInRow)
	}
	for range maxChurnedSavesInRow {
		if _, stop := c.noteChurn(tools.Edits{}); stop {
			t.Fatal("stopped before the limit")
		}
	}
	if note, stop := c.noteChurn(tools.Edits{}); !stop || !strings.Contains(note, "Leave the build as it is") {
		t.Fatalf("fourth save in a row that changed nothing = %q, %v", note, stop)
	}
}

func TestChurnAcceptsOneEditPerPanelAcrossManyPanels(t *testing.T) {
	var c State
	for i := range 6 {
		if note, stop := c.noteChurn(tools.Edits{Panels: []string{fmt.Sprint("p", i)}}); note != "" || stop {
			t.Fatalf("save %d changing a different panel churned: %q, %v", i+1, note, stop)
		}
	}
}

func TestChurnCountsSettingsAndVariablesAsProgressUntilTheLimit(t *testing.T) {
	var c State
	for i := 1; i <= maxChangesPerPanel; i++ {
		if note, stop := c.noteChurn(tools.Edits{Settings: true}); note != "" || stop {
			t.Fatalf("settings save %d churned: %q, %v", i, note, stop)
		}
	}
	note, stop := c.noteChurn(tools.Edits{Settings: true})
	if !strings.Contains(note, "Leave the dashboard settings as it is") || stop {
		t.Fatalf("settings save past the limit = %q, %v", note, stop)
	}
}

func TestChurnDoesNotCountAPanelAddedByASaveThatDeletesOneAsProgress(t *testing.T) {
	swap := func(id, gone string) tools.Edits {
		return tools.Edits{Panels: []string{id}, Added: []string{id}, Deleted: []string{gone}}
	}

	var once State
	note, stop := once.noteChurn(swap("b", "a"))
	if !strings.Contains(note, "deleted a panel and added another") || stop || once.ChurnedInRow != 1 {
		t.Fatalf("one replacement = %q, %v, inRow %d", note, stop, once.ChurnedInRow)
	}
	if note, stop := once.noteChurn(tools.Edits{Panels: []string{"c"}, Added: []string{"c"}}); note != "" || stop || once.ChurnedInRow != 0 {
		t.Fatalf("a plain new panel after a replacement = %q, %v, inRow %d", note, stop, once.ChurnedInRow)
	}

	var swaps State
	for i := range maxChurnedSavesInRow {
		if _, stop := swaps.noteChurn(swap(fmt.Sprint("n", i), fmt.Sprint("o", i))); stop {
			t.Fatalf("swap %d stopped before the limit", i+1)
		}
	}
	note, stop = swaps.noteChurn(swap("n9", "o9"))
	if !stop || !strings.Contains(note, "deleted a panel and added another") {
		t.Fatalf("swap past the limit = %q, %v", note, stop)
	}
	if swaps.Churn["n0"] != 1 {
		t.Fatalf("a replacement still counts toward its panel's changes: %v", swaps.Churn)
	}
}

func TestChurnStillCountsAnEditOfAnExistingPanelInASaveThatDeletes(t *testing.T) {
	var c State
	edits := tools.Edits{Panels: []string{"a"}, Deleted: []string{"z"}}
	if note, stop := c.noteChurn(edits); note != "" || stop {
		t.Fatalf("edit beside a delete = %q, %v", note, stop)
	}
}
