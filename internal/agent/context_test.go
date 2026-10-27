package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/dashboards"
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
	if lines := strings.Count(got, "\n- panel"); lines != maxUnverifiedLines {
		t.Fatalf("%d subject lines, want %d", lines, maxUnverifiedLines)
	}
	if !strings.Contains(got, "\n- ... and 10 more\n") {
		t.Fatalf("missing the remainder line in %q", got[len(got)-120:])
	}
}
