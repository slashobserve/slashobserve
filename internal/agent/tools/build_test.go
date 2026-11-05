package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type noopReviser struct{}

func (noopReviser) Revise(context.Context, revisions.Request) (revisions.Result, error) {
	return revisions.Result{}, nil
}

func (r *panelsRig) createRun(t *testing.T) panelsCaller {
	t.Helper()
	queue := revisions.NewService(r.database.DB, noopReviser{}, slog.New(slog.DiscardHandler), revisions.WithAccess(folders.NewAccess(r.database.DB)))
	request, err := queue.SubmitObserve(r.ctx, placeholderUser, "build a board")
	if err != nil {
		t.Fatal(err)
	}
	r.database.Exec(t, "UPDATE agent_requests SET status = 'running', attempts = 1, lease_until = clock_timestamp() + interval '1 minute' WHERE id = $1", request.ID)
	return r.as(WithRun(r.ctx, Run{Mode: ModeCreate, Build: true, RequestID: request.ID.String(), Attempt: 1}))
}

func buildCreateJSON(panel string, extra string) string {
	return `{"spec":` + boardSpecOf(panel) + `,"summary":"Start"` + extra + `}`
}

func buildRow(id, title string, y int, children ...string) string {
	return fmt.Sprintf(`{"id":%q,"type":"row","title":%q,"gridPos":{"x":0,"y":%d,"w":24,"h":1},"panels":[%s]}`, id, title, y, strings.Join(children, ","))
}

func parseBoard(t *testing.T, panels ...string) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse([]byte(boardSpecOf(panels...)))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestChangesOfListsOnlyNewAndEditedPanels(t *testing.T) {
	prev := parseBoard(t, suggestStat("a", "A", "up", 0), suggestStat("b", "B", "sum(up)", 4))
	next := parseBoard(t,
		suggestStat("a", "A renamed", "up", 0),
		suggestStat("b", "B", "count(up)", 4),
		buildRow("r1", "Row", 8, suggestStat("c", "C", "min(up)", 9)),
	)
	changes := changesOf(prev, next)
	var ids []string
	for _, p := range changes.shown {
		ids = append(ids, p.ID)
	}
	if got := strings.Join(ids, ","); got != "b,r1,c" {
		t.Fatalf("changed ids = %s, want b,r1,c", got)
	}
	if changes.count != 2 {
		t.Fatalf("changed data panels = %d, want 2 (the row is not counted)", changes.count)
	}
}

func describedStat(id, description string, y int) string {
	return fmt.Sprintf(`{"id":%q,"type":"stat","title":"P","description":%q,"gridPos":{"x":0,"y":%d,"w":6,"h":4},"datasource":%q,"queries":[{"query":"up"}]}`, id, description, y, fakeUID)
}

func TestEditsOfReportsEveryPanelWhoseContentChanged(t *testing.T) {
	rowOf := func(descriptions []string) *spec.Dashboard {
		var children []string
		for i, d := range descriptions {
			children = append(children, describedStat(fmt.Sprint("p", i), d, 1+i*4))
		}
		return parseBoard(t, buildRow("r1", "Row", 0, children...))
	}
	descriptions := []string{"old", "old", "old", "old", "old", "old"}
	prev := rowOf(descriptions)
	for i := range descriptions {
		descriptions[i] = "new"
		next := rowOf(descriptions)
		edits := editsOf(prev, next)
		if want := []string{fmt.Sprint("p", i)}; !reflect.DeepEqual(edits.Panels, want) || edits.Settings {
			t.Fatalf("description edit %d = %+v, want panels %v", i+1, edits, want)
		}
		if changes := changesOf(prev, next); changes.count != 0 || len(changes.shown) != 0 {
			t.Fatalf("a description edit changed what needs re-verification: %+v", changes)
		}
		prev = next
	}
}

func TestEditsOfSeparatesPanelsSettingsAndNoChange(t *testing.T) {
	base := boardSpecOf(buildRow("r1", "Row", 0, suggestStat("a", "A", "up", 1)), suggestStat("top", "Top", "up", 5))
	parse := func(raw string) *spec.Dashboard {
		d, err := spec.Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	prev := parse(base)
	tests := []struct {
		name string
		next string
		want Edits
	}{
		{name: "the same dashboard changes nothing", next: base},
		{name: "a title edit of one panel", next: strings.Replace(base, `"title":"A"`, `"title":"A2"`, 1), want: Edits{Panels: []string{"a"}}},
		{name: "a moved panel", next: strings.Replace(base, `"y":5`, `"y":6`, 1), want: Edits{Panels: []string{"top"}}},
		{name: "a row retitled but not its children", next: strings.Replace(base, `"title":"Row"`, `"title":"Row2"`, 1), want: Edits{Panels: []string{"r1"}}},
		{name: "a dashboard title edit", next: strings.Replace(base, `"title":"Board"`, `"title":"Board2"`, 1), want: Edits{Settings: true}},
		{name: "a variable edit", next: strings.Replace(base, `"options":["a","b"]`, `"options":["a","c"]`, 1), want: Edits{Settings: true}},
		{name: "a deleted panel", next: boardSpecOf(buildRow("r1", "Row", 0, suggestStat("a", "A", "up", 1))), want: Edits{Deleted: []string{"top"}}},
		{name: "a panel replaced by a new id", next: boardSpecOf(buildRow("r1", "Row", 0, suggestStat("a", "A", "up", 1)), suggestStat("top2", "Top", "up", 5)), want: Edits{Panels: []string{"top2"}, Added: []string{"top2"}, Deleted: []string{"top"}}},
		{name: "a row deleted with its child", next: boardSpecOf(suggestStat("top", "Top", "up", 5)), want: Edits{Deleted: []string{"r1", "a"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := editsOf(prev, parse(tt.next)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("edits = %+v, want %+v", got, tt.want)
			}
		})
	}
	first := editsOf(nil, prev)
	if want := []string{"r1", "a", "top"}; !reflect.DeepEqual(first.Panels, want) || !reflect.DeepEqual(first.Added, want) || len(first.Deleted) > 0 || !first.Settings {
		t.Fatalf("first save edits = %+v, want panels %v all added, and settings", first, want)
	}
}

func TestChangesOfStopsListingAtFifty(t *testing.T) {
	var panels []string
	for i := range 60 {
		panels = append(panels, suggestStat(fmt.Sprintf("p%d", i), "P", "up", i*4))
	}
	changes := changesOf(nil, parseBoard(t, panels...))
	if len(changes.shown) != maxChangedPanels || changes.count != 60 {
		t.Fatalf("listed %d of %d, want %d of 60", len(changes.shown), changes.count, maxChangedPanels)
	}
}

func TestChangesOfCountsNewAndEditedPanelsAndNeverRows(t *testing.T) {
	prev := parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1)), suggestStat("top", "Top", "up", 5))
	tests := []struct {
		name       string
		prev, next *spec.Dashboard
		wantAdded  *addedPanel
		wantEdited int
		wantCount  int
	}{
		{
			name:      "a new panel in a row names it and its row",
			prev:      prev,
			next:      parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1), suggestStat("b", "B", "up", 5)), suggestStat("top", "Top", "up", 10)),
			wantAdded: &addedPanel{ID: "b", Title: "B", Row: "One"},
			wantCount: 1,
		},
		{
			name:      "a new panel above every row has no row",
			prev:      prev,
			next:      parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1)), suggestStat("top", "Top", "up", 5), suggestStat("c", "C", "up", 9)),
			wantAdded: &addedPanel{ID: "c", Title: "C"},
			wantCount: 1,
		},
		{
			name:      "the first save of a build adds its panel",
			next:      parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1))),
			wantAdded: &addedPanel{ID: "a", Title: "A", Row: "One"},
			wantCount: 1,
		},
		{
			name:      "a new row with one child counts only the child",
			prev:      prev,
			next:      parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1)), suggestStat("top", "Top", "up", 5), buildRow("r2", "Two", 9, suggestStat("n", "N", "up", 10))),
			wantAdded: &addedPanel{ID: "n", Title: "N", Row: "Two"},
			wantCount: 1,
		},
		{
			name:       "an edited query counts as changed",
			prev:       prev,
			next:       parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "count(up)", 1)), suggestStat("top", "Top", "up", 5)),
			wantEdited: 1,
			wantCount:  1,
		},
		{
			name: "a title-only edit is no change",
			prev: prev,
			next: parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A renamed", "up", 1)), suggestStat("top", "Top", "up", 5)),
		},
		{
			name: "a moved panel is neither added nor changed",
			prev: prev,
			next: parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1)), suggestStat("top", "Top", "up", 6)),
		},
		{
			name: "a new row alone is not an added panel",
			prev: prev,
			next: parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1)), suggestStat("top", "Top", "up", 5), buildRow("r2", "Two", 9)),
		},
		{
			name:      "two new panels name the first and count both",
			prev:      prev,
			next:      parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1), suggestStat("x", "X", "up", 5), suggestStat("y", "Y", "up", 9)), suggestStat("top", "Top", "up", 13)),
			wantAdded: &addedPanel{ID: "x", Title: "X", Row: "One"},
			wantCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := changesOf(tt.prev, tt.next)
			if (got.added == nil) != (tt.wantAdded == nil) || (got.added != nil && *got.added != *tt.wantAdded) {
				t.Errorf("added = %+v, want %+v", got.added, tt.wantAdded)
			}
			if got.edited != tt.wantEdited || got.count != tt.wantCount {
				t.Errorf("edited = %d count = %d, want %d and %d", got.edited, got.count, tt.wantEdited, tt.wantCount)
			}
		})
	}
}

func TestBuildMessage(t *testing.T) {
	const nextPlanned = "Next: add one panel with update_panels, alone in upsert, with row naming its row, and check its queries in the same call. Set finish: true on your last save."
	tests := []struct {
		name        string
		progress    dashboards.BuildProgress
		changes     panelChanges
		gaps        int
		wantStep    string
		wantMessage string
	}{
		{
			name:        "a panel in a row with a planned count",
			progress:    dashboards.BuildProgress{Panels: 37, PlannedPanels: 114},
			changes:     panelChanges{added: &addedPanel{Title: "Swap used", Row: "Memory"}},
			wantStep:    "Saved panel 37 of 114: Swap used (row Memory)",
			wantMessage: "Panel 37 of 114 saved to the build. " + nextPlanned,
		},
		{
			name:        "a panel without a row",
			progress:    dashboards.BuildProgress{Panels: 2, PlannedPanels: 5},
			changes:     panelChanges{added: &addedPanel{Title: "Up"}},
			wantStep:    "Saved panel 2 of 5: Up",
			wantMessage: "Panel 2 of 5 saved to the build. " + nextPlanned,
		},
		{
			name:        "a panel without a planned count",
			progress:    dashboards.BuildProgress{Panels: 37},
			changes:     panelChanges{added: &addedPanel{Title: "Swap used", Row: "Memory"}},
			wantStep:    "Saved panel 37: Swap used (row Memory)",
			wantMessage: "The build holds 37 panels. Add the next one with update_panels, one panel per save, and set finish: true on your last save, or answer with one short sentence and no tool call.",
		},
		{
			name:        "a panel beyond the planned count drops the count",
			progress:    dashboards.BuildProgress{Panels: 6, PlannedPanels: 5},
			changes:     panelChanges{added: &addedPanel{Title: "Extra"}},
			wantStep:    "Saved panel 6: Extra",
			wantMessage: allPanelsBuiltMessage,
		},
		{
			name:        "the last planned panel",
			progress:    dashboards.BuildProgress{Panels: 5, PlannedPanels: 5},
			changes:     panelChanges{added: &addedPanel{Title: "Last"}},
			wantStep:    "Saved panel 5 of 5: Last",
			wantMessage: allPanelsBuiltMessage,
		},
		{
			name:        "the last planned panel with one gap",
			progress:    dashboards.BuildProgress{Panels: 5, PlannedPanels: 5},
			changes:     panelChanges{added: &addedPanel{Title: "Last"}},
			gaps:        1,
			wantStep:    "Saved panel 5 of 5: Last",
			wantMessage: allPanelsBuiltMessage + " The layout has 1 gap, listed in layout.gaps; you can close one by resending a panel beside it with a larger w or h.",
		},
		{
			name:        "the last planned panel with two gaps",
			progress:    dashboards.BuildProgress{Panels: 5, PlannedPanels: 5},
			changes:     panelChanges{added: &addedPanel{Title: "Last"}},
			gaps:        2,
			wantStep:    "Saved panel 5 of 5: Last",
			wantMessage: allPanelsBuiltMessage + " The layout has 2 gaps, listed in layout.gaps; you can close one by resending a panel beside it with a larger w or h.",
		},
		{
			name:        "gaps are not mentioned while panels are missing",
			progress:    dashboards.BuildProgress{Panels: 3, PlannedPanels: 5},
			changes:     panelChanges{added: &addedPanel{Title: "Up"}},
			gaps:        2,
			wantStep:    "Saved panel 3 of 5: Up",
			wantMessage: "Panel 3 of 5 saved to the build. " + nextPlanned,
		},
		{
			name:        "a changed panel",
			progress:    dashboards.BuildProgress{Panels: 114, PlannedPanels: 114},
			changes:     panelChanges{edited: 1},
			wantStep:    "Changed 1 panel in the build (114 in all)",
			wantMessage: allPanelsBuiltMessage,
		},
		{
			name:        "a save that only moved panels",
			progress:    dashboards.BuildProgress{Panels: 114, PlannedPanels: 114},
			changes:     panelChanges{moved: []string{"a", "b"}},
			wantStep:    "Moved 2 panels in the build (114 in all)",
			wantMessage: allPanelsBuiltMessage,
		},
		{
			name:        "several changed panels",
			progress:    dashboards.BuildProgress{Panels: 8},
			changes:     panelChanges{edited: 3},
			wantStep:    "Changed 3 panels in the build (8 in all)",
			wantMessage: "The build holds 8 panels. Add the next one with update_panels, one panel per save, and set finish: true on your last save, or answer with one short sentence and no tool call.",
		},
		{
			name:        "a save that only moved or deleted",
			progress:    dashboards.BuildProgress{Panels: 1},
			wantStep:    "Saved the build (1 panel)",
			wantMessage: "The build holds 1 panel. Add the next one with update_panels, one panel per save, and set finish: true on your last save, or answer with one short sentence and no tool call.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step, message := buildMessage(tt.progress, tt.changes, 0, tt.gaps)
			if step != tt.wantStep {
				t.Errorf("step = %q, want %q", step, tt.wantStep)
			}
			if message != tt.wantMessage {
				t.Errorf("message = %q, want %q", message, tt.wantMessage)
			}
		})
	}
}

func TestASaveThatLowersThePlanSaysSoAndDoesNotSayDone(t *testing.T) {
	progress := dashboards.BuildProgress{Panels: 69, PlannedPanels: 69}
	changes := panelChanges{added: &addedPanel{Title: "Extra", Row: "Memory"}}
	step, message := buildMessage(progress, changes, 80, 0)
	if want := "Saved panel 69 of 69: Extra (row Memory) (planned 80 -> 69)"; step != want {
		t.Errorf("step = %q, want %q", step, want)
	}
	want := "You lowered planned_panels from 80 to 69. Keep it at the number of panels the request needs; if the request needs more than you have built, raise it and add the next panel, else set finish: true."
	if message != want || message == allPanelsBuiltMessage {
		t.Errorf("message = %q, want %q", message, want)
	}
}

func TestASaveThatRaisesThePlanNamesTheChange(t *testing.T) {
	progress := dashboards.BuildProgress{Panels: 61, PlannedPanels: 80}
	step, message := buildMessage(progress, panelChanges{added: &addedPanel{Title: "Fans"}}, 60, 0)
	if want := "Saved panel 61 of 80: Fans (planned 60 -> 80)"; step != want {
		t.Errorf("step = %q, want %q", step, want)
	}
	if !strings.HasPrefix(message, "Panel 61 of 80 saved to the build.") {
		t.Errorf("message = %q, want the usual next-panel message", message)
	}
}

func TestAnUnchangedPlanKeepsTodaysMessages(t *testing.T) {
	tests := []struct {
		name          string
		progress      dashboards.BuildProgress
		plannedBefore int
		wantStep      string
		wantMessage   string
	}{
		{"the same plan and not done", dashboards.BuildProgress{Panels: 3, PlannedPanels: 5}, 5, "Saved panel 3 of 5: Up", "Panel 3 of 5 saved to the build."},
		{"the same plan and done", dashboards.BuildProgress{Panels: 5, PlannedPanels: 5}, 5, "Saved panel 5 of 5: Up", allPanelsBuiltMessage},
		{"the first plan of a build", dashboards.BuildProgress{Panels: 1, PlannedPanels: 5}, 0, "Saved panel 1 of 5: Up", "Panel 1 of 5 saved to the build."},
		{"no plan", dashboards.BuildProgress{Panels: 2}, 0, "Saved panel 2: Up", "The build holds 2 panels."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step, message := buildMessage(tt.progress, panelChanges{added: &addedPanel{Title: "Up"}}, tt.plannedBefore, 0)
			if step != tt.wantStep || !strings.HasPrefix(message, tt.wantMessage) {
				t.Errorf("step = %q message = %q, want %q and a message starting %q", step, message, tt.wantStep, tt.wantMessage)
			}
		})
	}
}

func TestVersionShape(t *testing.T) {
	tests := []struct {
		name   string
		panels []string
		want   string
	}{
		{"rows and panels", []string{buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1), suggestStat("b", "B", "up", 5)), buildRow("r2", "Two", 9, suggestStat("c", "C", "up", 10))}, "2 rows, 3 panels"},
		{"one row", []string{buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1))}, "1 row, 1 panel"},
		{"no rows", []string{suggestStat("a", "A", "up", 0), suggestStat("b", "B", "up", 4)}, "2 panels"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionShape(parseBoard(t, tt.panels...)); got != tt.want {
				t.Errorf("shape = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgentOnlyFieldsAreRefusedOutsideABuildRun(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	for name, tc := range map[string]struct{ tool, input string }{
		"create with finish":         {nameCreateDashboard, `{"spec":{"title":"t"},"finish":true}`},
		"create with a row":          {nameCreateDashboard, `{"spec":{"title":"t"},"row":{"id":"r","title":"R"}}`},
		"update with planned panels": {nameUpdateDashboard, `{"dashboard_id":"` + dashA + `","parent_version":1,"spec":{"title":"t"},"planned_panels":3}`},
		"update panels with finish":  {nameUpdatePanels, `{"dashboard_id":"` + dashA + `","parent_version":1,"summary":"s","upsert":[],"finish":true}`},
		"update panels with a row":   {nameUpdatePanels, `{"dashboard_id":"` + dashA + `","parent_version":1,"summary":"s","upsert":[],"row":{"id":"r","title":"R"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := call(t, r, tc.tool, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if errorCode(t, res) != codeInvalidRequest || !strings.Contains(res.Content.(errorBody).Message, agentOnlyBuildFields) {
				t.Fatalf("res = %#v", res.Content)
			}
		})
	}
}

func TestAgentOnlyFieldsAreAcceptedInABuildRun(t *testing.T) {
	three := 3
	tests := []struct {
		name    string
		finish  bool
		planned *int
		row     *rowRef
		inBuild bool
		want    bool
	}{
		{"none", false, nil, nil, false, true},
		{"finish outside a build", true, nil, nil, false, false},
		{"planned panels outside a build", false, &three, nil, false, false},
		{"row outside a build", false, nil, &rowRef{ID: "r", Title: "R"}, false, false},
		{"all in a build", true, &three, &rowRef{ID: "r", Title: "R"}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := agentOnlyFields(tt.finish, tt.planned, tt.row, tt.inBuild); ok != tt.want {
				t.Errorf("ok = %v, want %v", ok, tt.want)
			}
		})
	}
}

func TestUpdateTargetInABuildRunOfACreate(t *testing.T) {
	run := Run{Mode: ModeCreate, Build: true}
	target, _, ok, err := updateTarget(saveInput{}, run, true)
	if err != nil || !ok || target.id.Valid {
		t.Fatalf("target = %+v ok = %v err = %v", target, ok, err)
	}
	_, res, ok, err := updateTarget(saveInput{DashboardID: dashA}, run, true)
	if err != nil || ok || !strings.Contains(res.Content.(errorBody).Message, "this run creates a new dashboard, which has no id until you finish; leave out dashboard_id") {
		t.Fatalf("ok = %v err = %v res = %#v", ok, err, res.Content)
	}
}

func refusal(t *testing.T, res Result) string {
	t.Helper()
	if errorCode(t, res) != codeInvalidRequest {
		t.Fatalf("res = %#v, want invalid_request", res.Content)
	}
	return res.Content.(errorBody).Message
}

func upsertOf(items ...string) spec.PanelPatch {
	var p spec.PanelPatch
	for _, item := range items {
		p.Upsert = append(p.Upsert, json.RawMessage(item))
	}
	return p
}

func placedBy(t *testing.T, base *spec.Dashboard, patch spec.PanelPatch, row *rowRef) (spec.PanelPatch, *spec.Dashboard) {
	t.Helper()
	plan, res, ok := buildPatchPlan(base, patch, row)
	if !ok || plan.added == nil {
		t.Fatalf("not a placed new panel: ok = %v, res = %#v", ok, res.Content)
	}
	slots, res, ok := plan.slots(base)
	if !ok {
		t.Fatalf("slots refused: %#v", res.Content)
	}
	placed := placement(base, slots, plan.added, row).patch
	merged, err := spec.ApplyPanelPatch(base, placed)
	if err != nil {
		t.Fatalf("placed patch does not merge: %v", err)
	}
	return placed, merged
}

func TestPlaceNewPanelCreatesTheNamedRowAtTheBottomWithThePanel(t *testing.T) {
	base := parseBoard(t, placeStat("a", 0, 0, 12, 8))
	placed, merged := placedBy(t, base, upsertOf(placeStat("n", 7, 7, 6, 4)), &rowRef{ID: "r", Title: "Memory"})
	if len(placed.Upsert) != 1 || len(placed.Layout) != 0 {
		t.Fatalf("patch = %+v, want one row upsert and no layout", placed)
	}
	e, ok := merged.FindPanel("n")
	if !ok || e.Row == nil || e.Row.ID != "r" || e.Row.Title != "Memory" {
		t.Fatalf("new panel entry = %+v", e)
	}
	if got, want := e.Panel.GridPos, (spec.GridPos{X: 0, Y: 9, W: 6, H: 4}); got != want {
		t.Errorf("panel gridPos = %+v, want %+v", got, want)
	}
	if got, want := e.Row.GridPos, (spec.GridPos{X: 0, Y: 8, W: 24, H: 1}); got != want {
		t.Errorf("row gridPos = %+v, want %+v", got, want)
	}
}

func TestPlaceNewPanelPutsAnExistingRowsPanelBesideThenBelowAndShiftsTheRowsAfterIt(t *testing.T) {
	base := parseBoard(t,
		placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8)),
		placeRow("r2", 9, false, placeStat("b", 0, 10, 12, 8)),
	)
	row := &rowRef{ID: "r1", Title: "ignored"}

	first, merged := placedBy(t, base, upsertOf(placeStat("n", 0, 0, 12, 8)), row)
	if len(first.Layout) != 1 || first.Layout[0].ID != "n" || first.Layout[0].Row == nil || *first.Layout[0].Row != "r1" {
		t.Fatalf("first layout = %+v, want only n into r1", first.Layout)
	}
	if e, _ := merged.FindPanel("n"); e.Panel.GridPos != (spec.GridPos{X: 12, Y: 1, W: 12, H: 8}) {
		t.Errorf("first panel gridPos = %+v, want beside a", e.Panel.GridPos)
	}

	second, merged := placedBy(t, merged, upsertOf(placeStat("m", 0, 0, 12, 8)), row)
	if e, _ := merged.FindPanel("m"); e.Panel.GridPos != (spec.GridPos{X: 0, Y: 9, W: 12, H: 8}) {
		t.Errorf("second panel gridPos = %+v, want below the first line", e.Panel.GridPos)
	}
	shifted := map[string]int{}
	for _, l := range second.Layout {
		if l.GridPos != nil {
			var pos spec.GridPos
			if err := json.Unmarshal(l.GridPos, &pos); err != nil {
				t.Fatal(err)
			}
			shifted[l.ID] = pos.Y
		}
	}
	if len(shifted) != 2 || shifted["r2"] != 17 || shifted["b"] != 18 {
		t.Errorf("shifts = %v, want r2 at 17 and b at 18 only", shifted)
	}
}

func TestPlaceNewPanelWithoutARowGoesAboveTheFirstRow(t *testing.T) {
	base := parseBoard(t, placeStat("t", 0, 0, 12, 8), placeRow("r1", 8, false, placeStat("a", 0, 9, 12, 8)))
	_, merged := placedBy(t, base, upsertOf(placeStat("n", 0, 0, 12, 8)), nil)
	if e, _ := merged.FindPanel("n"); e.Row != nil || e.Panel.GridPos != (spec.GridPos{X: 12, Y: 0, W: 12, H: 8}) {
		t.Errorf("new panel = %+v at %+v, want top level beside t", e.Row, e.Panel.GridPos)
	}
}

func TestANewPanelKeepsEachValidSizeFieldAndTheServerChoosesTheRest(t *testing.T) {
	base := parseBoard(t, placeStat("a", 0, 0, 24, 8))
	sized := func(gridPos string) string {
		panel := `{"id":"n","type":"stat","title":"n","datasource":"` + fakeUID + `","queries":[{"query":"up"}]`
		if gridPos != "" {
			panel += `,"gridPos":` + gridPos
		}
		return panel + `}`
	}
	tests := []struct {
		name    string
		gridPos string
		w, h    int
	}{
		{"valid size", `{"x":0,"y":0,"w":6,"h":4}`, 6, 4},
		{"largest size", `{"w":24,"h":100}`, 24, 100},
		{"no position or size", `{"x":0,"y":0}`, 6, 4},
		{"no gridPos", ``, 6, 4},
		{"width only", `{"w":4}`, 4, 4},
		{"height only", `{"h":3}`, 6, 3},
		{"width too large and a valid height", `{"w":25,"h":3}`, 6, 3},
		{"height too large and a valid width", `{"w":8,"h":101}`, 8, 4},
		{"zero height", `{"w":6,"h":0}`, 6, 4},
		{"fractional width and a valid height", `{"w":6.5,"h":3}`, 6, 3},
		{"unreadable width", `{"w":"6","h":3}`, 6, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, merged := placedBy(t, base, upsertOf(sized(tt.gridPos)), nil)
			e, _ := merged.FindPanel("n")
			if e.Panel.GridPos.W != tt.w || e.Panel.GridPos.H != tt.h {
				t.Errorf("size = %dx%d, want %dx%d", e.Panel.GridPos.W, e.Panel.GridPos.H, tt.w, tt.h)
			}
		})
	}
}

func TestAPanelWithoutGridPosIsSizedAndPlaced(t *testing.T) {
	base := parseBoard(t, placeRow("row_sys", 53, false, placeStat("uptime", 0, 54, 6, 4)))
	fd := `{"id":"fd","type":"stat","title":"fd","datasource":"` + fakeUID + `","queries":[{"query":"up"}]}`
	merged, _, res, err := mergePatch(base, upsertOf(fd), &rowRef{ID: "row_sys"})
	if err != nil || merged == nil {
		t.Fatalf("merge refused: %#v, %v", res.Content, err)
	}
	e, found := merged.FindPanel("fd")
	if !found || e.Row == nil || e.Row.ID != "row_sys" || e.Panel.GridPos != (spec.GridPos{X: 6, Y: 54, W: 6, H: 4}) {
		t.Errorf("fd = %+v in %+v, want (6,54,6,4) in row_sys", e.Panel.GridPos, e.Row)
	}
}

func TestPlaceNewPanelLeavesASaveWithoutANewPanelAlone(t *testing.T) {
	base := parseBoard(t, placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8)))
	t.Run("only a delete", func(t *testing.T) {
		patch := spec.PanelPatch{Delete: []string{"a"}}
		merged, moved, res, err := mergePatch(base, patch, nil)
		if err != nil || merged == nil {
			t.Fatalf("merge refused: %#v, %v", res.Content, err)
		}
		want, err := spec.ApplyPanelPatch(base, patch)
		if err != nil || !reflect.DeepEqual(merged, want) || len(moved) != 0 {
			t.Errorf("merged = %+v moved = %v, want the plain patch result %+v (%v)", merged, moved, want, err)
		}
	})
	t.Run("an edit of a panel", func(t *testing.T) {
		merged, moved, res, err := mergePatch(base, upsertOf(placeStat("a", 3, 3, 12, 8)), nil)
		if err != nil || merged == nil {
			t.Fatalf("merge refused: %#v, %v", res.Content, err)
		}
		if e, _ := merged.FindPanel("a"); e.Panel.GridPos != (spec.GridPos{X: 0, Y: 1, W: 12, H: 8}) || len(moved) != 0 {
			t.Errorf("a = %+v moved = %v, want it kept at its stored position", e.Panel.GridPos, moved)
		}
	})
	t.Run("no upsert, only layout", func(t *testing.T) {
		patch := spec.PanelPatch{Layout: []spec.PanelPlacement{{ID: "a", GridPos: json.RawMessage(`{"x":0,"y":1,"w":6,"h":4}`)}}}
		merged, moved, res, err := mergePatch(base, patch, nil)
		if err != nil || merged == nil {
			t.Fatalf("merge refused: %#v, %v", res.Content, err)
		}
		if e, _ := merged.FindPanel("a"); e.Panel.GridPos != (spec.GridPos{X: 0, Y: 1, W: 6, H: 4}) || !reflect.DeepEqual(moved, []string{"a"}) {
			t.Errorf("a = %+v moved = %v, want it resized in place", e.Panel.GridPos, moved)
		}
	})
	t.Run("an empty patch", func(t *testing.T) {
		merged, _, res, err := mergePatch(base, spec.PanelPatch{}, nil)
		if err != nil || merged != nil {
			t.Fatalf("merged = %v, err = %v, want the empty patch rejected", merged, err)
		}
		if out, ok := res.Content.(patchRejectedOut); !ok || len(out.Issues) == 0 {
			t.Errorf("res = %#v, want a rejection", res.Content)
		}
	})
}

func TestPlaceNewPanelRefusals(t *testing.T) {
	base := parseBoard(t, placeStat("t", 0, 0, 12, 8), placeRow("r1", 8, false, placeStat("a", 0, 9, 12, 8)))
	row := &rowRef{ID: "r1", Title: "One"}
	tests := []struct {
		name  string
		patch spec.PanelPatch
		row   *rowRef
		want  string
	}{
		{"two panels", upsertOf(placeStat("n1", 0, 0, 6, 4), placeStat("n2", 6, 0, 6, 4)), row, "one panel per save: this save adds or changes 2 panels (n1, n2); save them one at a time"},
		{"a new row in upsert", upsertOf(placeRow("r9", 30, false)), nil, rowNeedsPanel},
		{"a new row in upsert with row", upsertOf(placeRow("r9", 30, false)), row, rowNeedsPanel},
		{"row naming a child panel", upsertOf(placeStat("n", 0, 0, 6, 4)), &rowRef{ID: "a", Title: "A"}, `panel "a" is not a row`},
		{"row naming a top level panel", upsertOf(placeStat("n", 0, 0, 6, 4)), &rowRef{ID: "t", Title: "T"}, `panel "t" is not a row`},
		{"row with an existing row", upsertOf(placeRow("r1", 8, false)), row, rowsStayInPlace},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, _, res, err := mergePatch(base, tt.patch, tt.row)
			if err != nil || merged != nil {
				t.Fatalf("merged = %v, err = %v, want a refusal", merged, err)
			}
			if got := refusal(t, res); got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
			if wantTyped := tt.name == "two panels"; (res.Refusal == RefusalOnePanel) != wantTyped {
				t.Errorf("refusal = %q, one-panel refusal wanted: %v", res.Refusal, wantTyped)
			}
		})
	}
}

func TestALayoutEntryForARowIsRefused(t *testing.T) {
	base := parseBoard(t, placeStat("t", 0, 0, 12, 8), placeRow("r1", 8, false, placeStat("a", 0, 9, 12, 8)))
	patch := spec.PanelPatch{Layout: []spec.PanelPlacement{{ID: "r1", GridPos: json.RawMessage(`{"x":0,"y":0,"w":24,"h":1}`)}}}
	merged, _, res, err := mergePatch(base, patch, nil)
	if err != nil || merged != nil {
		t.Fatalf("merged = %v, err = %v, want a refusal", merged, err)
	}
	if got := refusal(t, res); got != rowsStayInPlace {
		t.Errorf("message = %q, want %q", got, rowsStayInPlace)
	}
}

func TestLayoutEntriesNamingNothingUsableAreRefused(t *testing.T) {
	base := parseBoard(t, placeStat("t", 0, 0, 12, 8), placeRow("r1", 8, false, placeStat("a", 0, 9, 12, 8)))
	r9, empty := "r9", ""
	tests := []struct {
		name   string
		layout []spec.PanelPlacement
		want   string
	}{
		{"a panel that does not exist", []spec.PanelPlacement{{ID: "ghost", Row: &empty}}, `no panel with id "ghost" to place`},
		{"a row that does not exist", []spec.PanelPlacement{{ID: "a", Row: &r9}}, `no row with id "r9" to place the panel in; name a new row in row`},
		{"a panel named twice", []spec.PanelPlacement{{ID: "a", Row: &empty}, {ID: "a", Row: &empty}}, `panel "a" appears more than once in layout`},
		{"a panel as the row", []spec.PanelPlacement{{ID: "a", Row: ptr("t")}}, `panel "t" is not a row`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, _, res, err := mergePatch(base, spec.PanelPatch{Layout: tt.layout}, nil)
			if err != nil || merged != nil {
				t.Fatalf("merged = %v, err = %v, want a refusal", merged, err)
			}
			if got := refusal(t, res); got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

func systemBoard() []string {
	var rows []string
	for i := range 5 {
		y := i * 11
		id := fmt.Sprintf("r%d", i+1)
		rows = append(rows, placeRow(id, y, false, placeStat(id+"a", 0, y+1, 12, 8), placeStat(id+"b", 12, y+1, 12, 8)))
	}
	return append(rows, placeRow("row_sys", 53, false, placeStat("uptime", 0, 54, 6, 4), placeStat("cs", 6, 54, 10, 4), placeStat("fd", 16, 54, 8, 4)))
}

func assertOthersKept(t *testing.T, before, after *spec.Dashboard, except string) {
	t.Helper()
	for _, e := range before.AllPanels() {
		if e.Panel.ID == except {
			continue
		}
		got, ok := after.FindPanel(e.Panel.ID)
		if !ok || got.Panel.GridPos != e.Panel.GridPos {
			t.Errorf("panel %q is at %+v, want %+v", e.Panel.ID, got.Panel.GridPos, e.Panel.GridPos)
		}
	}
}

func TestAResentPanelKeepsItsPositionWhateverGridPosItCarries(t *testing.T) {
	base := parseBoard(t, systemBoard()...)
	tests := []struct {
		name  string
		panel string
		row   *rowRef
		want  spec.GridPos
	}{
		{"with its row", placeStat("cs", 0, 0, 10, 4), &rowRef{ID: "row_sys"}, spec.GridPos{X: 6, Y: 54, W: 10, H: 4}},
		{"without a row", placeStat("fd", 0, 0, 8, 4), nil, spec.GridPos{X: 16, Y: 54, W: 8, H: 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, moved, res, err := mergePatch(base, upsertOf(tt.panel), tt.row)
			if err != nil || merged == nil {
				t.Fatalf("refused: %#v, %v", res.Content, err)
			}
			var head upsertHead
			_ = json.Unmarshal([]byte(tt.panel), &head)
			if e, _ := merged.FindPanel(head.ID); e.Panel.GridPos != tt.want {
				t.Errorf("%s at %+v, want %+v", head.ID, e.Panel.GridPos, tt.want)
			}
			if len(moved) != 0 {
				t.Errorf("moved = %v, want none", moved)
			}
			assertOthersKept(t, base, merged, head.ID)
		})
	}
}

func TestAResentPanelInCaseCKeepsItsPosition(t *testing.T) {
	base := parseBoard(t, caseCBoard()...)
	merged, _, res, err := mergePatch(base, upsertOf(placeStat("p_mem_busy", 0, 0, 6, 5)), nil)
	if err != nil || merged == nil {
		t.Fatalf("refused: %#v, %v", res.Content, err)
	}
	if e, _ := merged.FindPanel("p_mem_busy"); e.Panel.GridPos != (spec.GridPos{X: 0, Y: 6, W: 6, H: 5}) {
		t.Errorf("p_mem_busy at %+v, want it unchanged", e.Panel.GridPos)
	}
	assertOthersKept(t, base, merged, "p_mem_busy")
}

func TestAResizedPanelThatNoLongerFitsIsPlacedAgain(t *testing.T) {
	base := parseBoard(t, systemBoard()...)
	merged, moved, res, err := mergePatch(base, upsertOf(placeStat("cs", 0, 0, 12, 4)), &rowRef{ID: "row_sys"})
	if err != nil || merged == nil {
		t.Fatalf("refused: %#v, %v", res.Content, err)
	}
	if e, _ := merged.FindPanel("cs"); e.Panel.GridPos != (spec.GridPos{X: 0, Y: 58, W: 12, H: 4}) {
		t.Errorf("cs at %+v, want a new line under the row's panels", e.Panel.GridPos)
	}
	if !reflect.DeepEqual(moved, []string{"cs"}) {
		t.Errorf("moved = %v, want cs", moved)
	}
	assertOthersKept(t, base, merged, "cs")
}

func TestRowOnAnExistingPanelMovesIt(t *testing.T) {
	base := parseBoard(t, caseCBoard()...)
	t.Run("to an existing row", func(t *testing.T) {
		merged, moved, res, err := mergePatch(base, upsertOf(placeStat("p_cpu_busy", 0, 0, 4, 5)), &rowRef{ID: "r_quick", Title: "ignored"})
		if err != nil || merged == nil {
			t.Fatalf("refused: %#v, %v", res.Content, err)
		}
		e, _ := merged.FindPanel("p_cpu_busy")
		if e.Row == nil || e.Row.ID != "r_quick" || e.Panel.GridPos != (spec.GridPos{X: 6, Y: 6, W: 4, H: 5}) {
			t.Errorf("p_cpu_busy = %+v in %+v, want (6,6,4,5) in r_quick", e.Panel.GridPos, e.Row)
		}
		if !reflect.DeepEqual(moved, []string{"p_cpu_busy"}) {
			t.Errorf("moved = %v, want p_cpu_busy", moved)
		}
	})
	t.Run("to a new row", func(t *testing.T) {
		merged, _, res, err := mergePatch(base, upsertOf(placeStat("p_cpu_busy", 0, 0, 4, 5)), &rowRef{ID: "r_new", Title: "New"})
		if err != nil || merged == nil {
			t.Fatalf("refused: %#v, %v", res.Content, err)
		}
		e, _ := merged.FindPanel("p_cpu_busy")
		if e.Row == nil || e.Row.ID != "r_new" || e.Row.Title != "New" {
			t.Fatalf("p_cpu_busy in %+v, want the new row r_new", e.Row)
		}
		if e.Row.GridPos != (spec.GridPos{X: 0, Y: 11, W: 24, H: 1}) || e.Panel.GridPos != (spec.GridPos{X: 0, Y: 12, W: 4, H: 5}) {
			t.Errorf("row at %+v and panel at %+v, want the row at the bottom with the panel under it", e.Row.GridPos, e.Panel.GridPos)
		}
	})
}

func TestALayoutEntryForTheNewPanelGivesItsSpot(t *testing.T) {
	tests := []struct {
		name   string
		board  []string
		panel  string
		layout spec.PanelPlacement
		want   spec.GridPos
	}{
		{
			name:   "beside a wide panel",
			board:  []string{placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8))},
			panel:  placeStat("n", 0, 0, 12, 8),
			layout: spec.PanelPlacement{ID: "n", GridPos: json.RawMessage(`{"x":12,"y":1,"w":12,"h":8}`)},
			want:   spec.GridPos{X: 12, Y: 1, W: 12, H: 8},
		},
		{
			name:   "past the first free spot",
			board:  []string{placeRow("r1", 0, false, placeStat("a", 0, 1, 6, 8))},
			panel:  placeStat("n", 0, 0, 6, 8),
			layout: spec.PanelPlacement{ID: "n", GridPos: json.RawMessage(`{"x":12,"y":1,"w":6,"h":8}`)},
			want:   spec.GridPos{X: 12, Y: 1, W: 6, H: 8},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patch := upsertOf(tt.panel)
			patch.Layout = []spec.PanelPlacement{tt.layout}
			merged, _, res, err := mergePatch(parseBoard(t, tt.board...), patch, &rowRef{ID: "r1"})
			if err != nil || merged == nil {
				t.Fatalf("refused: %#v, %v", res.Content, err)
			}
			if e, _ := merged.FindPanel("n"); e.Panel.GridPos != tt.want {
				t.Errorf("n at %+v, want %+v", e.Panel.GridPos, tt.want)
			}
		})
	}
	t.Run("naming another row is refused", func(t *testing.T) {
		patch := upsertOf(placeStat("n", 0, 0, 12, 8))
		patch.Layout = []spec.PanelPlacement{{ID: "n", Row: ptr("r2")}}
		merged, _, res, err := mergePatch(parseBoard(t, placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8))), patch, &rowRef{ID: "r1"})
		if err != nil || merged != nil {
			t.Fatalf("merged = %v, err = %v, want a refusal", merged, err)
		}
		if got := refusal(t, res); got != newPanelRowOnce {
			t.Errorf("message = %q, want %q", got, newPanelRowOnce)
		}
	})
}

func TestOnePanelPerSave(t *testing.T) {
	four := func(extra ...string) *spec.Dashboard {
		children := []string{suggestStat("a", "A", "up", 1), suggestStat("b", "B", "up", 5), suggestStat("c", "C", "up", 9), suggestStat("d", "D", "up", 13)}
		return parseBoard(t, buildRow("r1", "One", 0, append(children, extra...)...))
	}
	var seven []string
	for i := range 7 {
		seven = append(seven, suggestStat(fmt.Sprintf("p%d", i+1), "P", "up", i*4))
	}
	tests := []struct {
		name       string
		prev, next *spec.Dashboard
		wantErr    string
	}{
		{
			name:    "two new panels",
			prev:    four(),
			next:    four(suggestStat("x", "X", "up", 17), suggestStat("y", "Y", "up", 21)),
			wantErr: "one panel per save: this save adds or changes 2 panels (x, y); save them one at a time",
		},
		{
			name:    "a new panel and an edited one",
			prev:    four(),
			next:    parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "count(up)", 1), suggestStat("b", "B", "up", 5), suggestStat("c", "C", "up", 9), suggestStat("d", "D", "up", 13), suggestStat("x", "X", "up", 17))),
			wantErr: "(a, x)",
		},
		{
			name:    "a first save of seven panels lists five ids",
			next:    parseBoard(t, seven...),
			wantErr: "adds or changes 7 panels (p1, p2, p3, p4, p5); save",
		},
		{
			name: "a first save of one panel in a row",
			next: parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "up", 1))),
		},
		{
			name: "one edit with three deletes",
			prev: four(),
			next: parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "count(up)", 1))),
		},
		{
			name: "only moved panels",
			prev: four(),
			next: parseBoard(t, buildRow("r1", "One", 0, suggestStat("a", "A", "up", 2), suggestStat("b", "B", "up", 6), suggestStat("c", "C", "up", 10), suggestStat("d", "D", "up", 14))),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, ok := onePanelPerSave(changesOf(tt.prev, tt.next))
			if tt.wantErr == "" {
				if !ok {
					t.Fatalf("refused: %#v", res.Content)
				}
				return
			}
			if ok {
				t.Fatal("the save was accepted")
			}
			if got := refusal(t, res); !strings.Contains(got, tt.wantErr) {
				t.Errorf("message = %q, want it to contain %q", got, tt.wantErr)
			}
			if res.Refusal != RefusalOnePanel || !res.IsError {
				t.Errorf("refusal = %q, is_error %v, want a one-panel refusal", res.Refusal, res.IsError)
			}
		})
	}
}

func newPanelPatch() spec.PanelPatch {
	return upsertOf(placeStat("n", 0, 0, 24, 8))
}

func TestANewPanelBesideAMoveOfAPanelBelowItIsAccepted(t *testing.T) {
	base := parseBoard(t, placeStat("a", 0, 0, 12, 8), placeRow("r1", 8, false, placeStat("b", 0, 9, 12, 8)))
	patch := newPanelPatch()
	patch.Layout = []spec.PanelPlacement{{ID: "b", GridPos: json.RawMessage(`{"x":12,"y":9,"w":12,"h":8}`)}}

	merged, _, res, err := mergePatch(base, patch, nil)
	if err != nil || merged == nil {
		t.Fatalf("refused: %#v, %v", res.Content, err)
	}
	n, _ := merged.FindPanel("n")
	b, _ := merged.FindPanel("b")
	if n.Panel.GridPos.Y != 8 {
		t.Errorf("n gridPos = %+v, want it right below a", n.Panel.GridPos)
	}
	if b.Panel.GridPos.X != 12 || b.Panel.GridPos.Y < n.Panel.GridPos.Y+n.Panel.GridPos.H {
		t.Errorf("b gridPos = %+v, want the model's move kept and b below n", b.Panel.GridPos)
	}
}

func TestANewPanelBesideADeleteOfAPanelBelowItIsAccepted(t *testing.T) {
	base := parseBoard(t, placeStat("a", 0, 0, 12, 8), placeRow("r1", 8, false, placeStat("b", 0, 9, 12, 8)))
	patch := newPanelPatch()
	patch.Delete = []string{"b"}

	merged, _, res, err := mergePatch(base, patch, nil)
	if err != nil || merged == nil {
		t.Fatalf("refused: %#v, %v", res.Content, err)
	}
	if _, found := merged.FindPanel("b"); found {
		t.Error("b survived the delete")
	}
	if r1, _ := merged.FindPanel("r1"); r1.Panel.GridPos.Y != 16 {
		t.Errorf("r1 gridPos = %+v, want it below n", r1.Panel.GridPos)
	}
}

func TestARejectedRestOfThePatchNamesTheModelsIndex(t *testing.T) {
	base := parseBoard(t, placeStat("a", 0, 0, 12, 8))
	patch := newPanelPatch()
	patch.Delete = []string{"ghost"}

	merged, _, res, err := mergePatch(base, patch, nil)
	if err != nil || merged != nil {
		t.Fatalf("merged = %v, err = %v, want a rejection", merged, err)
	}
	out, ok := res.Content.(patchRejectedOut)
	if !ok || len(out.Issues) == 0 || out.Issues[0].Path != "/delete/0" {
		t.Fatalf("res = %#v, want an issue at /delete/0", res.Content)
	}
}

func TestANewPanelWithAnEmptyDashboardObjectIsPlacedAsBefore(t *testing.T) {
	base := parseBoard(t, placeStat("a", 0, 0, 12, 8))
	patch := newPanelPatch()
	patch.Dashboard = &spec.DashboardFields{}

	merged, _, res, err := mergePatch(base, patch, nil)
	if err != nil || merged == nil {
		t.Fatalf("refused: %#v, %v", res.Content, err)
	}
}

func TestABuildPatchRejectionCarriesTheRepairMessage(t *testing.T) {
	base := parseBoard(t, placeStat("a", 0, 0, 12, 8))
	merged, _, res, err := mergePatch(base, spec.PanelPatch{Delete: []string{"ghost"}}, nil)
	if err != nil || merged != nil {
		t.Fatalf("merged = %v, err = %v, want a rejection", merged, err)
	}
	out, ok := res.Content.(patchRejectedOut)
	if want := "Fix only the panels in failing_panels and save again. If an issue names a field you do not know, call get_spec_guide for the JSON Schema."; !ok || out.Message != want {
		t.Fatalf("res = %#v, want the repair message", res.Content)
	}
}

func TestABuildCreateRejectionListsFailingPanelsAndTheRepair(t *testing.T) {
	rig := newPanelsRig(t)
	run := rig.createRun(t)

	res, _ := run.run(t, nameCreateDashboard, buildCreateJSON(suggestStat("p1", "One", "broken", 0), ""))
	out := rejected(t, res)
	if want := "Fix only the panels in failing_panels and save again. The next call may run query_range or query_instant on the failing query to see why."; !reflect.DeepEqual(out.FailingPanels, []string{"p1"}) || out.Message != want {
		t.Fatalf("out = %+v, want failing panel p1 and the query repair message", out)
	}
	if !strings.HasPrefix(res.Step, "Save rejected: 1 issue") {
		t.Errorf("step = %q", res.Step)
	}
}

func TestACreateRunReadsAnotherDashboardsPanels(t *testing.T) {
	rig := newPanelsRig(t)
	other := rig.seedStats(t)
	run := rig.createRun(t)

	res, _ := run.run(t, nameGetPanels, `{"dashboard_id":"`+other+`","ids":["p1"]}`)
	out, ok := res.Content.(panelsOut)
	if res.IsError || !ok || out.DashboardID != other || out.Version != 1 || out.Build || len(out.Panels) != 1 || out.Panels[0].ID != "p1" {
		t.Fatalf("res = %#v, want panel p1 of the other dashboard, not the build", res.Content)
	}
}

func TestACreateRunWithoutADashboardIDReadsItsBuild(t *testing.T) {
	rig := newPanelsRig(t)
	run := rig.createRun(t)

	if res, _ := run.run(t, nameGetPanels, `{"ids":["p1"]}`); errorCodeOf(t, res) != codeInvalidRequest {
		t.Fatalf("res = %#v, want the no-build-yet refusal", res.Content)
	}
	if res, _ := run.run(t, nameCreateDashboard, buildCreateJSON(suggestStat("p1", "One", "count(up)", 0), "")); res.IsError {
		t.Fatalf("first save: %#v", res.Content)
	}
	res, _ := run.run(t, nameGetPanels, `{"ids":["p1"]}`)
	out, ok := res.Content.(panelsOut)
	if res.IsError || !ok || !out.Build || len(out.Panels) != 1 {
		t.Fatalf("res = %#v, want the panel from the build", res.Content)
	}
}

func TestAFinishSaveThatAddsAPanelNamesItInItsStep(t *testing.T) {
	rig := newPanelsRig(t)
	run := rig.createRun(t)

	res, _ := run.run(t, nameCreateDashboard, buildCreateJSON(suggestStat("p1", "One", "count(up)", 0), `,"finish":true`))
	savedVersion(t, res)
	if want := "Saved panel 1: One, then saved version 1 (1 panel)"; res.Step != want {
		t.Errorf("step = %q, want %q", res.Step, want)
	}
}

func TestAFinishSaveWithoutAPanelKeepsTheVersionStep(t *testing.T) {
	rig := newPanelsRig(t)
	run := rig.createRun(t)

	if res, _ := run.run(t, nameCreateDashboard, buildCreateJSON(suggestStat("p1", "One", "count(up)", 0), "")); res.IsError {
		t.Fatalf("first save: %#v", res.Content)
	}
	res, _ := run.run(t, nameUpdatePanels, `{"summary":"Rename","dashboard":{"title":"Renamed"},"finish":true}`)
	savedVersion(t, res)
	if want := "Saved version 1 (1 panel)"; res.Step != want {
		t.Errorf("step = %q, want %q", res.Step, want)
	}
}

func firstPanelSpec(panels ...string) json.RawMessage {
	return json.RawMessage(boardSpecOf(panels...))
}

func TestPlaceFirstPanelPutsTheOnlyPanelInANewRowAtTheTop(t *testing.T) {
	placed, res, err := placeFirstPanel(firstPanelSpec(placeStat("cpu", 7, 7, 6, 5)), &rowRef{ID: "r1", Title: "CPU"})
	if err != nil || placed == nil {
		t.Fatalf("refused: %#v, %v", res.Content, err)
	}
	d, err := spec.Parse(placed)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Panels) != 1 || d.Panels[0].Type != spec.Row || d.Panels[0].ID != "r1" || d.Panels[0].Title != "CPU" {
		t.Fatalf("top level = %+v, want only the row r1", d.Panels)
	}
	if got, want := d.Panels[0].GridPos, (spec.GridPos{X: 0, Y: 0, W: 24, H: 1}); got != want {
		t.Errorf("row gridPos = %+v, want %+v", got, want)
	}
	e, ok := d.FindPanel("cpu")
	if !ok || e.Row == nil || e.Row.ID != "r1" {
		t.Fatalf("cpu entry = %+v, want it inside r1", e)
	}
	if got, want := e.Panel.GridPos, (spec.GridPos{X: 0, Y: 1, W: 6, H: 5}); got != want {
		t.Errorf("cpu gridPos = %+v, want %+v", got, want)
	}
	if d.Title != "Board" || len(d.Variables) != 1 {
		t.Errorf("title = %q, variables = %d, want the rest of the spec kept", d.Title, len(d.Variables))
	}
}

func TestPlaceFirstPanelRefusals(t *testing.T) {
	row := &rowRef{ID: "r1", Title: "CPU"}
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"no panel", firstPanelSpec(), rowNeedsFirstPanel},
		{"two panels", firstPanelSpec(placeStat("a", 0, 0, 6, 4), placeStat("b", 6, 0, 6, 4)), rowNeedsFirstPanel},
		{"no panels key", json.RawMessage(`{"specVersion":"0","title":"Board"}`), rowNeedsFirstPanel},
		{"a row instead of a panel", firstPanelSpec(placeRow("r9", 0, false, placeStat("a", 0, 1, 6, 4))), rowNeedsPanel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			placed, res, err := placeFirstPanel(tt.raw, row)
			if err != nil || placed != nil {
				t.Fatalf("placed = %s, err = %v, want a refusal", placed, err)
			}
			if got := refusal(t, res); got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestACreateSaveWithARowSavesItsPanelInTheRowAndNamesIt(t *testing.T) {
	rig := newPanelsRig(t)
	run := rig.createRun(t)

	res, _ := run.run(t, nameCreateDashboard, buildCreateJSON(placeStat("cpu", 0, 0, 6, 5), `,"planned_panels":4,"row":{"id":"r1","title":"CPU"}`))
	if res.IsError || res.Step != "Saved panel 1 of 4: cpu (row CPU)" {
		t.Fatalf("step = %q, res = %#v", res.Step, res.Content)
	}
	if out, ok := res.Content.(buildOut); !ok || out.Layout == nil || !reflect.DeepEqual(out.Layout.Rows, []string{`r1 "CPU" 1`}) ||
		out.Layout.Row == nil || out.Layout.Row.ID != "r1" || !reflect.DeepEqual(out.Layout.Row.Panels, []string{"cpu x0 y1 w6 h5"}) || len(out.Layout.Gaps) != 0 {
		t.Fatalf("layout = %+v, want the one row and its panel", out.Layout)
	}
	res, _ = run.run(t, nameUpdatePanels, `{"summary":"s","upsert":[`+placeStat("mem", 0, 0, 6, 5)+`],"row":{"id":"r2","title":"Memory"}}`)
	if res.IsError || res.Step != "Saved panel 2 of 4: mem (row Memory)" {
		t.Fatalf("step = %q, res = %#v", res.Step, res.Content)
	}
	res, _ = run.run(t, nameGetPanels, `{"ids":["r1","r2","cpu","mem"]}`)
	out, ok := res.Content.(panelsOut)
	if res.IsError || !ok || len(out.Panels) != 4 {
		t.Fatalf("res = %#v, want the four panels of the build", res.Content)
	}
}

func TestACreateSaveWithARowAndTwoPanelsIsRefused(t *testing.T) {
	rig := newPanelsRig(t)
	run := rig.createRun(t)

	spec := boardSpecOf(placeStat("a", 0, 0, 6, 4), placeStat("b", 6, 0, 6, 4))
	res, _ := run.run(t, nameCreateDashboard, `{"spec":`+spec+`,"summary":"Start","row":{"id":"r1","title":"CPU"}}`)
	if got := refusal(t, res); got != rowNeedsFirstPanel {
		t.Errorf("message = %q, want %q", got, rowNeedsFirstPanel)
	}
}

func TestANewPanelAfterALoosePanelAboveTheFirstRowNeverOverlaps(t *testing.T) {
	loose := placeStat("cpu", 0, 0, 6, 5)
	first, _ := placedBy(t, parseBoard(t, loose), upsertOf(placeStat("mem", 0, 0, 6, 5)), &rowRef{ID: "r1", Title: "Memory"})
	base, err := spec.ApplyPanelPatch(parseBoard(t, loose), first)
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := base.FindPanel("mem"); e.Panel.GridPos != (spec.GridPos{X: 0, Y: 6, W: 6, H: 5}) {
		t.Fatalf("setup: mem at %+v, want y 6 under a row at y 5", e.Panel.GridPos)
	}
	tests := []struct {
		name string
		row  *rowRef
		w, h int
	}{
		{"into the existing row", &rowRef{ID: "r1", Title: "Memory"}, 6, 5},
		{"into the existing row, taller", &rowRef{ID: "r1", Title: "Memory"}, 24, 9},
		{"into a new row", &rowRef{ID: "r2", Title: "Disk"}, 6, 5},
		{"with no row, as tall as the loose panel", nil, 6, 5},
		{"with no row, taller than the loose panel", nil, 18, 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, merged := placedBy(t, base, upsertOf(placeStat("n", 0, 0, tt.w, tt.h)), tt.row); merged == nil {
				t.Fatal("no merged dashboard")
			}
		})
	}
}

func TestACreateWithoutARowPlacesItsOnlyPanel(t *testing.T) {
	t.Run("a top level panel goes to the origin", func(t *testing.T) {
		placed, res, err := placeFirstPanel(firstPanelSpec(placeStat("p", 5, 7, 6, 4)), nil)
		if err != nil || placed == nil {
			t.Fatalf("refused: %#v, %v", res.Content, err)
		}
		d, err := spec.Parse(placed)
		if err != nil {
			t.Fatal(err)
		}
		if e, _ := d.FindPanel("p"); e.Row != nil || e.Panel.GridPos != (spec.GridPos{X: 0, Y: 0, W: 6, H: 4}) {
			t.Errorf("p = %+v in %+v, want (0,0,6,4) at the top level", e.Panel.GridPos, e.Row)
		}
	})
	t.Run("a row holding the panel is placed like the row form", func(t *testing.T) {
		placed, res, err := placeFirstPanel(firstPanelSpec(buildRow("r", "CPU", 3, placeStat("p", 0, 0, 12, 8))), nil)
		if err != nil || placed == nil {
			t.Fatalf("refused: %#v, %v", res.Content, err)
		}
		d, err := spec.Parse(placed)
		if err != nil {
			t.Fatal(err)
		}
		e, _ := d.FindPanel("p")
		if e.Row == nil || e.Row.ID != "r" || e.Row.Title != "CPU" || e.Row.GridPos != (spec.GridPos{X: 0, Y: 0, W: 24, H: 1}) || e.Panel.GridPos != (spec.GridPos{X: 0, Y: 1, W: 12, H: 8}) {
			t.Errorf("p = %+v in %+v, want (0,1,12,8) in a row at (0,0,24,1)", e.Panel.GridPos, e.Row)
		}
	})
	for name, doc := range map[string]json.RawMessage{
		"two rows and one panel": firstPanelSpec(buildRow("r1", "A", 0, placeStat("a", 0, 1, 6, 4)), buildRow("r2", "B", 5, placeStat("b", 0, 6, 6, 4)), placeStat("c", 0, 11, 6, 4)),
		"an empty row":           firstPanelSpec(buildRow("r", "A", 0)),
		"a row with two panels":  firstPanelSpec(buildRow("r", "A", 0, placeStat("a", 0, 1, 6, 4), placeStat("b", 6, 1, 6, 4))),
		"two panels":             firstPanelSpec(placeStat("a", 0, 0, 6, 4), placeStat("b", 6, 0, 6, 4)),
	} {
		t.Run(name+" is saved as sent", func(t *testing.T) {
			placed, _, err := placeFirstPanel(doc, nil)
			if err != nil || string(placed) != string(doc) {
				t.Errorf("placed = %s, err = %v, want the document untouched", placed, err)
			}
		})
	}
}

func TestUpdateDashboardKeepsBuildPositions(t *testing.T) {
	base := parseBoard(t, placeStat("a", 0, 0, 12, 8), placeStat("b", 12, 0, 12, 8))
	t.Run("positions the model sends are ignored", func(t *testing.T) {
		doc := boardSpecOf(placeStat("a", 0, 40, 12, 8), placeStat("b", 0, 0, 12, 8))
		_, merged, res, ok, err := stageUpdate(base, json.RawMessage(doc))
		if err != nil || !ok {
			t.Fatalf("refused: %#v, %v", res.Content, err)
		}
		for id, want := range map[string]spec.GridPos{"a": {X: 0, Y: 0, W: 12, H: 8}, "b": {X: 12, Y: 0, W: 12, H: 8}} {
			if e, _ := merged.FindPanel(id); e.Panel.GridPos != want {
				t.Errorf("%s at %+v, want %+v", id, e.Panel.GridPos, want)
			}
		}
	})
	t.Run("a new panel is placed by the server", func(t *testing.T) {
		doc := boardSpecOf(placeStat("a", 0, 40, 12, 8), placeStat("b", 0, 0, 12, 8), placeStat("c", 0, 0, 12, 8))
		raw, merged, res, ok, err := stageUpdate(base, json.RawMessage(doc))
		if err != nil || !ok {
			t.Fatalf("refused: %#v, %v", res.Content, err)
		}
		if e, _ := merged.FindPanel("c"); e.Panel.GridPos != (spec.GridPos{X: 0, Y: 8, W: 12, H: 8}) {
			t.Errorf("c at %+v, want (0,8,12,8)", e.Panel.GridPos)
		}
		if saved, err := spec.Parse(raw); err != nil || !reflect.DeepEqual(saved, merged) {
			t.Errorf("the saved document differs from the merged one (%v)", err)
		}
		assertOthersKept(t, base, merged, "c")
	})
	t.Run("a new panel in an existing row", func(t *testing.T) {
		rowBase := parseBoard(t, placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8)))
		doc := boardSpecOf(placeRow("r1", 0, false, placeStat("a", 5, 5, 12, 8), placeStat("n", 0, 0, 12, 8)))
		_, merged, res, ok, err := stageUpdate(rowBase, json.RawMessage(doc))
		if err != nil || !ok {
			t.Fatalf("refused: %#v, %v", res.Content, err)
		}
		e, _ := merged.FindPanel("n")
		if e.Row == nil || e.Row.ID != "r1" || e.Panel.GridPos != (spec.GridPos{X: 12, Y: 1, W: 12, H: 8}) {
			t.Errorf("n = %+v in %+v, want (12,1,12,8) in r1", e.Panel.GridPos, e.Row)
		}
		assertOthersKept(t, rowBase, merged, "n")
	})
	t.Run("a new row holding only the new panel", func(t *testing.T) {
		doc := boardSpecOf(placeStat("a", 0, 0, 12, 8), placeStat("b", 0, 0, 12, 8), buildRow("r9", "More", 99, placeStat("n", 0, 0, 6, 4)))
		_, merged, res, ok, err := stageUpdate(base, json.RawMessage(doc))
		if err != nil || !ok {
			t.Fatalf("refused: %#v, %v", res.Content, err)
		}
		e, _ := merged.FindPanel("n")
		if e.Row == nil || e.Row.ID != "r9" || e.Row.Title != "More" || e.Row.GridPos != (spec.GridPos{X: 0, Y: 8, W: 24, H: 1}) || e.Panel.GridPos != (spec.GridPos{X: 0, Y: 9, W: 6, H: 4}) {
			t.Errorf("n = %+v in %+v, want (0,9,6,4) in a row at (0,8,24,1)", e.Panel.GridPos, e.Row)
		}
	})
}

func TestUpdateDashboardRefusesAnEmptyNewRow(t *testing.T) {
	base := parseBoard(t, placeStat("a", 0, 0, 12, 8))
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{"an empty new row", boardSpecOf(placeStat("a", 0, 0, 12, 8), buildRow("r9", "More", 8)), newRowNeedsItsPanel},
		{"a new row with an existing panel", boardSpecOf(buildRow("r9", "More", 0, placeStat("a", 0, 1, 12, 8))), newRowNeedsItsPanel},
		{"a new row with two new panels", boardSpecOf(placeStat("a", 0, 0, 12, 8), buildRow("r9", "More", 8, placeStat("n", 0, 9, 6, 4), placeStat("m", 6, 9, 6, 4))), newRowNeedsItsPanel},
		{"two new panels", boardSpecOf(placeStat("a", 0, 0, 12, 8), placeStat("n", 12, 0, 6, 4), placeStat("m", 18, 0, 6, 4)), "one panel per save: this save adds or changes 2 panels (n, m); save them one at a time"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, res, ok, err := stageUpdate(base, json.RawMessage(tt.doc))
			if err != nil || ok {
				t.Fatalf("ok = %v, err = %v, want a refusal", ok, err)
			}
			if got := refusal(t, res); got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

func manyRows(t *testing.T, rows, perRow int) *spec.Dashboard {
	var panels []string
	y := 0
	for r := range rows {
		var children []string
		for c := range perRow {
			children = append(children, placeStat(fmt.Sprintf("r%dp%d", r, c), (c%2)*12, y+1+(c/2)*8, 12, 8))
		}
		height := 1 + ((perRow+1)/2)*8
		panels = append(panels, fmt.Sprintf(`{"id":"r%d","type":"row","title":%q,"gridPos":{"x":0,"y":%d,"w":24,"h":1},"panels":[%s]}`, r, fmt.Sprintf("Row number %d with a rather long descriptive title about what the panels below it show and why", r), y, strings.Join(children, ",")))
		y += height
	}
	return parseBoard(t, panels...)
}

func TestLayoutOutFitsItsBudget(t *testing.T) {
	big := manyRows(t, 16, 8)
	out := layoutOf(big, "r0p0", gaps(big))
	if raw, _ := json.Marshal(out); len(raw) > maxLayoutBytes || !out.Truncated || len(out.Rows) == 0 || len(out.Rows) >= 16 {
		t.Errorf("layout of 16 rows = %d bytes, truncated %v, %d rows listed; want at most %d bytes, truncated, some rows dropped", len(raw), out.Truncated, len(out.Rows), maxLayoutBytes)
	}

	odd := manyRows(t, 16, 7)
	out = layoutOf(odd, "r0p0", gaps(odd))
	if raw, _ := json.Marshal(out); len(raw) > maxLayoutBytes || !out.Truncated || out.Open != "r0 x12 y25 w12 h8" {
		t.Errorf("layout of 16 rows with an open end = %d bytes, truncated %v, open %q; want at most %d bytes, truncated, and open %q", len(raw), out.Truncated, out.Open, maxLayoutBytes, "r0 x12 y25 w12 h8")
	}

	small := parseBoard(t, buildRow("r1", "One", 0, placeStat("a", 0, 1, 12, 8)), buildRow("r2", "Two", 9, placeStat("b", 0, 10, 12, 8)), placeStat("t", 0, 20, 12, 8))
	out = layoutOf(small, "b", gaps(small))
	if out.Truncated || len(out.Rows) != 3 || out.Row == nil || out.Row.ID != "r2" {
		t.Errorf("layout of 3 panels = %+v, want every section listed, not truncated, and the row of b", out)
	}
	if want := []string{"(top level) 1", `r1 "One" 1`, `r2 "Two" 1`}; !reflect.DeepEqual(out.Rows, want) {
		t.Errorf("rows = %q, want %q", out.Rows, want)
	}
}

func TestLayoutOutFocusesTheSectionOfTheLastPlacedPanel(t *testing.T) {
	d := parseBoard(t, buildRow("r1", "One", 0, placeStat("a", 0, 1, 12, 8)), buildRow("r2", "Two", 9, placeStat("b", 0, 10, 12, 8)))
	tests := []struct {
		name    string
		changes panelChanges
		want    string
	}{
		{"an added panel", panelChanges{added: &addedPanel{ID: "a"}, moved: []string{"b"}}, "r1"},
		{"the last moved panel", panelChanges{moved: []string{"a", "b"}}, "r2"},
		{"the first changed panel", panelChanges{shown: []panelOut{{ID: "r1", Type: spec.Row}, {ID: "b", Type: spec.Stat}}}, "r2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := layoutOf(d, tt.changes.focus(), nil)
			if out.Row == nil || out.Row.ID != tt.want {
				t.Errorf("row = %+v, want %s", out.Row, tt.want)
			}
		})
	}
	if out := layoutOf(d, panelChanges{}.focus(), nil); out.Row != nil {
		t.Errorf("row = %+v, want none when the save changed no panel", out.Row)
	}
}

func TestLayoutOutShowsTheOpenEndOfTheRow(t *testing.T) {
	tests := []struct {
		name  string
		board []string
		focus string
		want  string
	}{
		{"half a line in a row", []string{buildRow("row_cpu", "CPU", 0, placeSeries("usage", 0, 1, 12, 8))}, "usage", "row_cpu x12 y1 w12 h8"},
		{"the lowest line decides", []string{buildRow("row_cpu", "CPU", 0, placeSeries("a", 0, 1, 12, 8), placeSeries("b", 12, 1, 12, 8), placeStat("c", 0, 9, 6, 4))}, "c", "row_cpu x6 y9 w18 h4"},
		{"the top level", []string{placeSeries("t", 0, 0, 8, 8)}, "t", "(top level) x8 y0 w16 h8"},
		{"a full line", []string{buildRow("row_cpu", "CPU", 0, placeSeries("a", 0, 1, 12, 8), placeSeries("b", 12, 1, 12, 8))}, "a", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := parseBoard(t, tt.board...)
			out := layoutOf(d, tt.focus, gaps(d))
			if out.Open != tt.want {
				t.Errorf("open = %q, want %q", out.Open, tt.want)
			}
			raw, _ := json.Marshal(out)
			if has := strings.Contains(string(raw), `"open"`); has != (tt.want != "") {
				t.Errorf("marshalled layout %s, want the open key present only when there is an open end", raw)
			}
		})
	}
}
