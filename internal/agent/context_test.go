package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func unverifiedRevision(t *testing.T, panels, variables int) *revision {
	t.Helper()
	raw := `{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"panels":[{"id":"p2","type":"stat","title":"Missing","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"query":"up"}]}]}`
	d, err := spec.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	issue := spec.Issue{Path: "/panels/0/queries", Code: "query_empty", Message: "query returned\nno data"}
	u := dashboards.Unverified{}
	for i := range panels {
		id := "p2"
		if i > 0 {
			id = fmt.Sprintf("gone%d", i)
		}
		u.Panels = append(u.Panels, dashboards.UnverifiedPanel{PanelID: id, Issues: []spec.Issue{issue}})
	}
	for i := range variables {
		u.Variables = append(u.Variables, dashboards.UnverifiedVariable{Name: fmt.Sprintf("v%d", i), Issues: []spec.Issue{{Code: "variable_failed", Message: "boom"}}})
	}
	return &revision{dashboardID: "d", number: 3, spec: d, unverified: u}
}

func TestWriteUnverifiedListsPanelsThenVariables(t *testing.T) {
	var b strings.Builder
	writeUnverified(&b, unverifiedRevision(t, 1, 1))
	want := "\nUnverified in version 3: their queries failed when it was saved. A save keeps them unverified while their queries and the variables those use stay unchanged; a changed or new query must return data." +
		"\n- panel p2 \"Missing\": query_empty: query returned no data" +
		"\n- variable v0: variable_failed: boom" +
		"\nKeep unverified panels unless the request asks to delete them."
	if got := b.String(); got != want {
		t.Fatalf("block:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteUnverifiedWritesNothingWhenEmpty(t *testing.T) {
	var b strings.Builder
	writeUnverified(&b, &revision{number: 1})
	if b.Len() != 0 {
		t.Fatalf("block %q, want none", b.String())
	}
}

func TestWriteUnverifiedStopsAtTheLineLimit(t *testing.T) {
	var b strings.Builder
	writeUnverified(&b, unverifiedRevision(t, 70, 0))
	got := b.String()
	if lines := strings.Count(got, "\n- panel"); lines != maxListLines {
		t.Fatalf("%d subject lines, want %d", lines, maxListLines)
	}
	if !strings.Contains(got, "\n- ... and 10 more\n") {
		t.Fatalf("missing the remainder line in %q", got[len(got)-120:])
	}
}

func TestWriteConversionListsPartialAndPlaceholderLines(t *testing.T) {
	var b strings.Builder
	writeConversion(&b, &revision{number: 1, conversion: []tools.Attention{
		{PanelID: "p4", Title: "Requests and errors by route", Status: "partial", Reason: "Some overrides use matchers\nor properties that are not supported yet."},
		{PanelID: "p10", Title: "Requests by pod", Status: "placeholder", Reason: "Grafana panel type barchart is not supported yet."},
	}})
	want := "\nVersion 1 is a Grafana import. The importer converted these panels only partly or kept them as placeholders:" +
		"\n- partial p4 \"Requests and errors by route\": Some overrides use matchers or properties that are not supported yet." +
		"\n- placeholder p10 \"Requests by pod\": Grafana panel type barchart is not supported yet."
	if got := b.String(); got != want {
		t.Fatalf("block:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteConversionWritesNothingWhenEmpty(t *testing.T) {
	var b strings.Builder
	writeConversion(&b, &revision{number: 1})
	if b.Len() != 0 {
		t.Fatalf("block %q, want none", b.String())
	}
}

func TestWriteConversionStopsAtTheLineLimit(t *testing.T) {
	notes := make([]tools.Attention, 70)
	for i := range notes {
		notes[i] = tools.Attention{PanelID: fmt.Sprintf("p%d", i), Title: "T", Status: "partial", Reason: "r"}
	}
	var b strings.Builder
	writeConversion(&b, &revision{number: 1, conversion: notes})
	got := b.String()
	if lines := strings.Count(got, "\n- partial"); lines != maxListLines {
		t.Fatalf("%d subject lines, want %d", lines, maxListLines)
	}
	if !strings.HasSuffix(got, "\n- ... and 10 more") {
		t.Fatalf("missing the remainder line in %q", got[len(got)-120:])
	}
}

func TestBuildContextPutsConversionAfterUnverified(t *testing.T) {
	current := unverifiedRevision(t, 1, 1)
	current.conversion = []tools.Attention{{PanelID: "p2", Title: "Missing", Status: "partial", Reason: "dropped overrides"}}
	got, err := buildContext(nil, tools.ModeRevise, revisions.SourcePrompt, "p2", "", current, seed{})
	if err != nil {
		t.Fatal(err)
	}
	unverified := strings.Index(got, "Unverified in version")
	conversion := strings.Index(got, "is a Grafana import")
	panel := strings.Index(got, "The request is about panel")
	if unverified < 0 || conversion <= unverified || panel <= conversion {
		t.Fatalf("order unverified=%d conversion=%d panel=%d in %q", unverified, conversion, panel, got)
	}
}

func nodeExporterFullRevision(t *testing.T) *revision {
	t.Helper()
	data, err := os.ReadFile("../../pkg/grafana/testdata/golden/rfmoz/node-exporter-full.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Spec json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	d, err := spec.Parse(golden.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return &revision{dashboardID: "d", number: 7, spec: d}
}

func TestNeedsOutlineAroundTheThreshold(t *testing.T) {
	for _, c := range []struct {
		bytes int
		want  bool
	}{
		{0, false},
		{16<<10 - 1, false},
		{16 << 10, false},
		{16<<10 + 1, true},
		{228749, true},
	} {
		if got := needsOutline(c.bytes); got != c.want {
			t.Errorf("needsOutline(%d) = %v, want %v", c.bytes, got, c.want)
		}
	}
}

func TestBuildContextOfASmallDashboardKeepsTheFullJSON(t *testing.T) {
	current := unverifiedRevision(t, 0, 0)
	statuses := []datasource.Status{{UID: "prom", Type: "prometheus", Healthy: true}}
	body, err := json.Marshal(current.spec)
	if err != nil {
		t.Fatal(err)
	}
	head := "Configured datasources:\n- prom (prometheus): healthy\n"

	got, err := buildContext(statuses, tools.ModeRevise, revisions.SourcePrompt, "p2", "change p2", current, seed{})
	if err != nil {
		t.Fatal(err)
	}
	want := head + "\nYou are revising dashboard d, currently version 3.\n```json\n" + string(body) + "\n```\nThe request is about panel p2 titled \"Missing\"."
	if got != want {
		t.Fatalf("revise context:\n%s\nwant:\n%s", got, want)
	}

	got, err = buildContext(statuses, tools.ModeSuggest, revisions.SourcePrompt, "p2", "change p2", current, seed{})
	if err != nil {
		t.Fatal(err)
	}
	want = head + "\nYou are proposing a change to dashboard d, version 3.\n```json\n" + string(body) + "\n```" +
		"\nPropose a change to panel p2 titled \"Missing\" for the request. Call update_panels with only that panel, with id p2, in upsert; everything else stays as in version 3, and nothing is saved."
	if got != want {
		t.Fatalf("suggest context:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildContextOfNodeExporterFullIsAnOutlineWithTheTargetInFull(t *testing.T) {
	current := nodeExporterFullRevision(t)
	const target = "p323"
	entry, ok := current.spec.FindPanel(target)
	if !ok {
		t.Fatalf("golden has no panel %s", target)
	}

	got, err := buildContext(nil, tools.ModeRevise, revisions.SourcePrompt, target, "make it red", current, seed{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) >= 24<<10 {
		t.Fatalf("context is %d bytes, want under 24 KiB", len(got))
	}
	for _, e := range current.spec.AllPanels() {
		if n := strings.Count(got, "\n- "+e.Panel.ID+" "); n != 1 {
			t.Errorf("panel %s listed %d times in the outline", e.Panel.ID, n)
		}
	}
	full, err := json.Marshal(entry.Panel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, string(full)) {
		t.Fatalf("context lacks the JSON of %s", target)
	}
	if strings.Contains(got, "```json\n{") {
		t.Fatal("context holds the whole Spec")
	}
}

func TestOutlineHoldsTheDashboardFieldsAModelCanChange(t *testing.T) {
	current := nodeExporterFullRevision(t)
	var b strings.Builder
	writeOutline(&b, current.spec, nil)
	got := b.String()
	for _, want := range []string{
		`Title: "` + current.spec.Title + `"`,
		`Time: {"from":"` + current.spec.Time.From + `","to":"` + current.spec.Time.To + `"}`,
		`Tags: ["linux"]`,
		"Links: [{",
		"Variables: [{",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("outline lacks %q in:\n%.500s", want, got)
		}
	}
	if strings.Contains(got, "Panels in full") {
		t.Fatal("an outline with no targets holds panel JSON")
	}
}

func TestOutlineLinePutsAPanelInItsRow(t *testing.T) {
	d, err := spec.Parse([]byte(`{"specVersion":"0","title":"t","time":{"from":"now-1h","to":"now"},"panels":[` +
		`{"id":"r1","type":"row","title":"Row","gridPos":{"x":0,"y":0,"w":24,"h":1},"panels":[` +
		`{"id":"p1","type":"stat","title":"Up","gridPos":{"x":0,"y":1,"w":3,"h":4},"datasource":"prom","queries":[{"query":"up"}]}]},` +
		`{"id":"p2","type":"stat","title":"Top","gridPos":{"x":3,"y":5,"w":3,"h":4},"datasource":"prom","queries":[{"query":"up"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	writeOutline(&b, d, []string{"r1"})
	got := b.String()
	for _, want := range []string{
		"\n- r1 row \"Row\" x0 y0 w24 h1",
		"\n- p1 stat \"Up\" row=r1 x0 y1 w3 h4",
		"\n- p2 stat \"Top\" x3 y5 w3 h4",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("outline lacks %q in:\n%s", want, got)
		}
	}
	if _, full, _ := strings.Cut(got, "Panels in full"); strings.Contains(full, `"p1"`) {
		t.Fatalf("a row target carries its children:\n%s", got)
	}
}

func TestOutlineTargetsAreThePanelAndWholeTokensOfThePrompt(t *testing.T) {
	var panels []string
	for i, id := range []string{"p1", "p12", "cpu-load", "p4"} {
		panels = append(panels, `{"id":"`+id+`","type":"stat","title":"T","gridPos":{"x":0,"y":`+fmt.Sprint(i*4)+`,"w":3,"h":4},"datasource":"prom","queries":[{"query":"up"}]}`)
	}
	d, err := spec.Parse([]byte(`{"specVersion":"0","title":"t","time":{"from":"now-1h","to":"now"},"panels":[` + strings.Join(panels, ",") + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, panel, prompt string
		want                []string
	}{
		{"nothing mentioned", "", "make it red", nil},
		{"the panel of the request", "p4", "make it red", []string{"p4"}},
		{"a token is not a substring", "", "fix p12 but not p1x or xp1", []string{"p12"}},
		{"punctuation ends a token", "", "(p1), cpu-load.", []string{"p1", "cpu-load"}},
		{"the panel is listed once", "p12", "p12 and p12", []string{"p12"}},
		{"the panel comes first", "p4", "p1 p12", []string{"p4", "p1", "p12"}},
		{"a panel the dashboard lacks", "gone", "gone", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := outlineTargets(d, c.panel, c.prompt); !slices.Equal(got, c.want) {
				t.Fatalf("targets = %v, want %v", got, c.want)
			}
		})
	}
}

func TestOutlineTargetsAreCapped(t *testing.T) {
	var panels, ids []string
	for i := range maxOutlineTargets + 5 {
		id := fmt.Sprintf("p%d", i)
		ids = append(ids, id)
		panels = append(panels, `{"id":"`+id+`","type":"stat","title":"T","gridPos":{"x":0,"y":`+fmt.Sprint(i*4)+`,"w":3,"h":4},"datasource":"prom","queries":[{"query":"up"}]}`)
	}
	d, err := spec.Parse([]byte(`{"specVersion":"0","title":"t","time":{"from":"now-1h","to":"now"},"panels":[` + strings.Join(panels, ",") + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := outlineTargets(d, "", strings.Join(ids, " ")); len(got) != maxOutlineTargets {
		t.Fatalf("%d targets, want %d", len(got), maxOutlineTargets)
	}
}

func TestBuildSystemHoldsTheRulesTheGuideAndTheExampleButNotTheSchema(t *testing.T) {
	system := buildSystem()
	for _, want := range []string{
		"You are SlashObserve's dashboard agent.",
		"\n\n# Authoring guide\n",
		"\n\n# Example Spec (format only)\n```json\n",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("system text lacks %q", want)
		}
	}
	if strings.Contains(system, "# ObserveSpec v0 JSON Schema") || strings.Contains(system, `"$defs"`) {
		t.Fatal("system text carries the JSON Schema")
	}
	if len(system) >= 20<<10 {
		t.Fatalf("system text is %d bytes, want under 20 KiB", len(system))
	}
	if !strings.Contains(system, "- Never create a placeholder panel. Keep existing placeholders unchanged apart from their position, and delete one only when the request asks.") {
		t.Fatal("system prompt lacks the placeholder rule")
	}
}

func TestBuildSystemStatesTheRulesTheLoopEnforces(t *testing.T) {
	system := buildSystem()
	for _, want := range []string{
		"from the sixth model call without an accepted save only a save runs, and reads in the same call as a save still run",
		"the next call may run reads, such as `query_range` on a failing query, to see why",
		"Every rejection of a save lists the panels in `failing_panels`",
		"call `get_spec_guide` for the JSON Schema",
		"Set `planned_panels` once from the request",
		"names a dashboard to rebuild",
		"A third identical call is refused, and a `<note>` at the end of a tool result is an instruction from the server",
		"when `truncated` is true, pass `next_offset` to page on",
		"Pick widths in a row that add up to 24",
		"with `row` naming its row",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
	for _, gone := range []string{
		"two to eight panels unless the request asks for more",
		"after an `update_panels` rejection resend only",
		"The schema here leaves out",
	} {
		if strings.Contains(system, gone) {
			t.Errorf("system text still holds %q", gone)
		}
	}
}
