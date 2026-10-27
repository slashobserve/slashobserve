package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
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
	got, err := buildContext(nil, tools.ModeRevise, revisions.SourcePrompt, "p2", current)
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
