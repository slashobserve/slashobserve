package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

func TestCallKeyIgnoresWhitespaceAndKeepsTheToolName(t *testing.T) {
	compact := callKey("update_panels", json.RawMessage(`{"a":1,"b":[2,3]}`))
	if got := callKey("update_panels", json.RawMessage("{ \"a\": 1,\n \"b\": [2, 3] }")); got != compact {
		t.Fatalf("whitespace changed the key: %q vs %q", got, compact)
	}
	if got := callKey("create_dashboard", json.RawMessage(`{"a":1,"b":[2,3]}`)); got == compact {
		t.Fatal("two tools share a key")
	}
	if got := callKey("update_panels", json.RawMessage(`{"a":2,"b":[2,3]}`)); got == compact {
		t.Fatal("different input shares a key")
	}
	if !strings.HasPrefix(compact, "update_panels:") {
		t.Fatalf("key = %q, want the tool name and a colon first", compact)
	}
}

func TestCallKeyHashesInputThatIsNotJSONAsIs(t *testing.T) {
	broken := callKey("lint_query", json.RawMessage(`{"half`))
	if broken == callKey("lint_query", json.RawMessage(`{"other`)) {
		t.Fatal("different broken inputs share a key")
	}
	if broken != callKey("lint_query", json.RawMessage(`{"half`)) {
		t.Fatal("the same broken input changed key")
	}
}

func TestGuardsRefuseARepeatedSave(t *testing.T) {
	var g guards
	if g.noteSave("a") || g.noteSave("b") {
		t.Fatal("a first save was reported as a repeat")
	}
	if !g.noteSave("a") {
		t.Fatal("the second save of a key was not reported as a repeat")
	}
	if !g.noteSave("a") {
		t.Fatal("the third save of a key was not reported as a repeat")
	}
	g.resetAll()
	if g.noteSave("a") {
		t.Fatal("a save after a reset was reported as a repeat")
	}
}

func TestGuardsReplayASuccessAndRefuseTheThird(t *testing.T) {
	var g guards
	if decision, content := g.noteRead("k"); decision != readDispatch || content != nil {
		t.Fatalf("first read = %v, %q", decision, content)
	}
	g.rememberRead("k", "get_panels", []byte(`{"ok":true}`), false)
	if decision, content := g.noteRead("k"); decision != readReplay || string(content) != `{"ok":true}` {
		t.Fatalf("second read = %v, %q", decision, content)
	}
	if decision, _ := g.noteRead("k"); decision != readRefuse {
		t.Fatalf("third read = %v, want a refusal", decision)
	}
	if decision, _ := g.noteRead("k"); decision != readRefuse {
		t.Fatalf("fourth read = %v, want a refusal", decision)
	}
}

func TestGuardsNeverReplayAnError(t *testing.T) {
	var g guards
	g.noteRead("k")
	g.rememberRead("k", "list_metrics", []byte(`{"code":"datasource_unavailable"}`), true)
	if decision, _ := g.noteRead("k"); decision != readDispatch {
		t.Fatalf("second read of an error = %v, want a live call", decision)
	}
	g.rememberRead("k", "list_metrics", []byte(`{"metrics":[]}`), false)
	if decision, _ := g.noteRead("k"); decision != readRefuse {
		t.Fatalf("third read = %v, want a refusal", decision)
	}
}

func TestGuardsNeverReplayAQueryResult(t *testing.T) {
	for _, tool := range []string{"query_range", "query_instant"} {
		var g guards
		g.noteRead("k")
		g.rememberRead("k", tool, []byte(`{"series":[]}`), false)
		if decision, _ := g.noteRead("k"); decision != readDispatch {
			t.Fatalf("%s: second read = %v, want a live call", tool, decision)
		}
	}
}

func TestGuardsForgetReadsOnReset(t *testing.T) {
	var g guards
	g.noteRead("k")
	g.rememberRead("k", "get_panels", []byte("x"), false)
	g.resetReads()
	if decision, content := g.noteRead("k"); decision != readDispatch || content != nil {
		t.Fatalf("read after reset = %v, %q", decision, content)
	}
}

func TestBudgetLimits(t *testing.T) {
	if maxRejectionsInRow != 3 || maxCutOffsInRow != 2 || maxIdenticalReads != 3 || maxCallsBeforeSave != 6 || maxCallsBeforeProposal != 12 || maxStalledTurns != 2 || defaultSessionRenewBytes != 256<<10 {
		t.Fatalf("budgets = %d %d %d %d %d %d %d", maxRejectionsInRow, maxCutOffsInRow, maxIdenticalReads, maxCallsBeforeSave, maxCallsBeforeProposal, maxStalledTurns, defaultSessionRenewBytes)
	}
}

func TestReadRefusal(t *testing.T) {
	for _, c := range []struct {
		name    string
		mode    tools.Mode
		window  saveWindow
		hasSave bool
		want    guard
	}{
		{"first call", tools.ModeCreate, saveWindow{calls: 1}, false, guard{}},
		{"fifth call without a save", tools.ModeCreate, saveWindow{calls: 5}, false, guard{}},
		{"sixth call without a save", tools.ModeCreate, saveWindow{calls: 6}, false, guardSaveNow},
		{"sixth call of a revision", tools.ModeRevise, saveWindow{calls: 6}, false, guardSaveNow},
		{"a save beside the reads lets them run", tools.ModeCreate, saveWindow{calls: 9}, true, guard{}},
		{"the diagnose turn lets reads run", tools.ModeRevise, saveWindow{calls: 7, diagnose: true}, false, guard{}},
		{"suggest runs reads up to the twelfth call", tools.ModeSuggest, saveWindow{calls: 11}, false, guard{}},
		{"suggest refuses from the twelfth call", tools.ModeSuggest, saveWindow{calls: 12}, false, guardProposeNow},
		{"suggest lets a proposal beside reads run", tools.ModeSuggest, saveWindow{calls: 13}, true, guard{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := readRefusal(c.mode, c.window, c.hasSave); got != c.want {
				t.Fatalf("readRefusal(%v, %+v, %v) = %+v, want %+v", c.mode, c.window, c.hasSave, got, c.want)
			}
		})
	}
}

func TestNudgeFor(t *testing.T) {
	for _, c := range []struct {
		name   string
		mode   tools.Mode
		window saveWindow
		want   string
	}{
		{"fourth call", tools.ModeCreate, saveWindow{calls: 4}, ""},
		{"fifth call", tools.ModeCreate, saveWindow{calls: 5}, messageSaveNext},
		{"fifth call of a revision", tools.ModeRevise, saveWindow{calls: 5}, messageSaveNext},
		{"sixth call", tools.ModeCreate, saveWindow{calls: 6}, ""},
		{"suggest fifth call", tools.ModeSuggest, saveWindow{calls: 5}, ""},
		{"suggest eleventh call", tools.ModeSuggest, saveWindow{calls: 11}, messageProposeNext},
		{"suggest twelfth call", tools.ModeSuggest, saveWindow{calls: 12}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := nudgeFor(c.mode, c.window); got != c.want {
				t.Fatalf("nudgeFor(%v, %+v) = %q, want %q", c.mode, c.window, got, c.want)
			}
		})
	}
}

func TestTheSaveNowMessageNamesTheSixthCall(t *testing.T) {
	if !strings.HasPrefix(guardSaveNow.message, "6 model calls without a save; only a save can run now.") {
		t.Fatalf("message = %q", guardSaveNow.message)
	}
}

func TestChurnCountsChangesPerPanel(t *testing.T) {
	var c churn
	for i, id := range []string{"a", "b", "c"} {
		if note, stop := c.note(tools.Edits{Panels: []string{id}}); note != "" || stop {
			t.Fatalf("add %d of a new panel churned: %q, %v", i, note, stop)
		}
	}
	for edit := 2; edit <= maxChangesPerPanel; edit++ {
		if note, stop := c.note(tools.Edits{Panels: []string{"a"}}); note != "" || stop {
			t.Fatalf("change %d of one panel churned: %q, %v", edit, note, stop)
		}
	}
	note, stop := c.note(tools.Edits{Panels: []string{"a"}})
	if want := "This save changed no panel that still needs work. Leave a as it is, save the next panel, or set finish: true when every planned panel is built."; note != want || stop {
		t.Fatalf("fifth change of one panel = %q, %v", note, stop)
	}
	if note, stop := c.note(tools.Edits{Panels: []string{"d"}}); note != "" || stop || c.inRow != 0 {
		t.Fatalf("a new panel after churn = %q, %v, inRow %d", note, stop, c.inRow)
	}
	for range maxChurnedSavesInRow {
		if _, stop := c.note(tools.Edits{}); stop {
			t.Fatal("stopped before the limit")
		}
	}
	if note, stop := c.note(tools.Edits{}); !stop || !strings.Contains(note, "Leave the build as it is") {
		t.Fatalf("fourth save in a row that changed nothing = %q, %v", note, stop)
	}
}

func TestChurnAcceptsOneEditPerPanelAcrossManyPanels(t *testing.T) {
	var c churn
	for i := range 6 {
		if note, stop := c.note(tools.Edits{Panels: []string{fmt.Sprint("p", i)}}); note != "" || stop {
			t.Fatalf("save %d changing a different panel churned: %q, %v", i+1, note, stop)
		}
	}
}

func TestChurnCountsSettingsAndVariablesAsProgressUntilTheLimit(t *testing.T) {
	var c churn
	for i := 1; i <= maxChangesPerPanel; i++ {
		if note, stop := c.note(tools.Edits{Settings: true}); note != "" || stop {
			t.Fatalf("settings save %d churned: %q, %v", i, note, stop)
		}
	}
	note, stop := c.note(tools.Edits{Settings: true})
	if !strings.Contains(note, "Leave the dashboard settings as it is") || stop {
		t.Fatalf("settings save past the limit = %q, %v", note, stop)
	}
}

func TestChurnDoesNotCountAPanelAddedByASaveThatDeletesOneAsProgress(t *testing.T) {
	swap := func(id, gone string) tools.Edits {
		return tools.Edits{Panels: []string{id}, Added: []string{id}, Deleted: []string{gone}}
	}

	var once churn
	note, stop := once.note(swap("b", "a"))
	if !strings.Contains(note, "deleted a panel and added another") || stop || once.inRow != 1 {
		t.Fatalf("one replacement = %q, %v, inRow %d", note, stop, once.inRow)
	}
	if note, stop := once.note(tools.Edits{Panels: []string{"c"}, Added: []string{"c"}}); note != "" || stop || once.inRow != 0 {
		t.Fatalf("a plain new panel after a replacement = %q, %v, inRow %d", note, stop, once.inRow)
	}

	var swaps churn
	for i := range maxChurnedSavesInRow {
		if _, stop := swaps.note(swap(fmt.Sprint("n", i), fmt.Sprint("o", i))); stop {
			t.Fatalf("swap %d stopped before the limit", i+1)
		}
	}
	note, stop = swaps.note(swap("n9", "o9"))
	if !stop || !strings.Contains(note, "deleted a panel and added another") {
		t.Fatalf("swap past the limit = %q, %v", note, stop)
	}
	if swaps.changes["n0"] != 1 {
		t.Fatalf("a replacement still counts toward its panel's changes: %v", swaps.changes)
	}
}

func TestChurnStillCountsAnEditOfAnExistingPanelInASaveThatDeletes(t *testing.T) {
	var c churn
	edits := tools.Edits{Panels: []string{"a"}, Deleted: []string{"z"}}
	if note, stop := c.note(edits); note != "" || stop {
		t.Fatalf("edit beside a delete = %q, %v", note, stop)
	}
}
