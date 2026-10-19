package grafana

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

var testDatasources = []Datasource{
	{UID: "prom", Type: "prometheus"},
	{UID: "ch", Type: "clickhouse"},
}

func dashboardJSON(extra string, panels ...string) string {
	if extra != "" {
		extra += ","
	}
	return fmt.Sprintf(`{"title":"Test",%s"panels":[%s]}`, extra, strings.Join(panels, ","))
}

func panelJSON(id, x, y, w, h int, extra string) string {
	if extra != "" {
		extra = "," + extra
	}
	return fmt.Sprintf(`{"id":%d,"type":"text","title":"P%d","gridPos":{"x":%d,"y":%d,"w":%d,"h":%d}%s}`, id, id, x, y, w, h, extra)
}

func rowJSON(id, y int, collapsed bool, extra string) string {
	if extra != "" {
		extra = "," + extra
	}
	return fmt.Sprintf(`{"id":%d,"type":"row","title":"Row %d","collapsed":%v,"gridPos":{"x":0,"y":%d,"w":24,"h":1}%s}`, id, id, collapsed, y, extra)
}

func convertDoc(t *testing.T, doc string) *Result {
	t.Helper()
	return convertWith(t, doc, Options{Datasources: testDatasources})
}

func convertWith(t *testing.T, doc string, opts Options) *Result {
	t.Helper()
	res, err := Convert([]byte(doc), opts)
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, doc)
	}
	if res.Ready() {
		if _, err := json.Marshal(res.Spec); err != nil || res.Spec == nil {
			t.Fatalf("a ready result has no usable Spec: %v", err)
		}
	}
	return res
}

func panelLine(t *testing.T, res *Result, id string) PanelLine {
	t.Helper()
	for _, line := range res.Report.Panels {
		if line.PanelID == id {
			return line
		}
	}
	t.Fatalf("no panel line %q in %+v", id, res.Report.Panels)
	return PanelLine{}
}

func isMoved(line PanelLine) bool {
	return slices.Contains(line.Reasons, movedReason)
}

func notesFor(res *Result, subject string) []Note {
	var out []Note
	for _, n := range res.Report.Notes {
		if n.Subject == subject {
			out = append(out, n)
		}
	}
	return out
}

func hasPath(notes []Note, path string) bool {
	return slices.ContainsFunc(notes, func(n Note) bool { return slices.Contains(n.Paths, path) })
}

func hasLine(res *Result, id string) bool {
	return slices.ContainsFunc(res.Report.Panels, func(l PanelLine) bool { return l.PanelID == id })
}
