package dashboards

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func linePanel(kind, id string, x, y, w, h int, extra string) string {
	return fmt.Sprintf(`{"id":%q,"type":%q,"title":%q,"gridPos":{"x":%d,"y":%d,"w":%d,"h":%d},"datasource":"prom","queries":[{"query":"up"}]%s}`, id, kind, id, x, y, w, h, extra)
}

func withQueries(panel, queries string) string {
	return strings.Replace(panel, `"queries":[{"query":"up"}]`, `"queries":`+queries, 1)
}

func lineRow(id string, y int, collapsed bool, children ...string) string {
	return fmt.Sprintf(`{"id":%q,"type":"row","title":%q,"collapsed":%t,"gridPos":{"x":0,"y":%d,"w":24,"h":1},"panels":[%s]}`, id, id, collapsed, y, strings.Join(children, ","))
}

func lineBoard(t *testing.T, panels ...string) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse([]byte(fmt.Sprintf(`{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"panels":[%s]}`, strings.Join(panels, ","))))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCloseLines(t *testing.T) {
	tests := []struct {
		name       string
		board      []string
		want       map[string]spec.GridPos
		wantLegend map[string]string
		unchanged  bool
	}{
		{
			name:  "a lone half-width panel widens",
			board: []string{linePanel("timeseries", "a", 0, 0, 12, 8, "")},
			want:  map[string]spec.GridPos{"a": {X: 0, Y: 0, W: 24, H: 8}},
		},
		{
			name:  "a pair of thirds becomes halves",
			board: []string{linePanel("stat", "a", 0, 0, 8, 8, ""), linePanel("stat", "b", 8, 0, 8, 8, "")},
			want:  map[string]spec.GridPos{"a": {X: 0, Y: 0, W: 12, H: 8}, "b": {X: 12, Y: 0, W: 12, H: 8}},
		},
		{
			name: "five stats share 24",
			board: []string{
				linePanel("stat", "s0", 0, 0, 4, 4, ""), linePanel("stat", "s1", 4, 0, 4, 4, ""), linePanel("stat", "s2", 8, 0, 4, 4, ""),
				linePanel("stat", "s3", 12, 0, 4, 4, ""), linePanel("stat", "s4", 16, 0, 4, 4, ""),
			},
			want: map[string]spec.GridPos{
				"s0": {X: 0, Y: 0, W: 5, H: 4}, "s1": {X: 5, Y: 0, W: 5, H: 4}, "s2": {X: 10, Y: 0, W: 5, H: 4},
				"s3": {X: 15, Y: 0, W: 5, H: 4}, "s4": {X: 20, Y: 0, W: 4, H: 4},
			},
		},
		{
			name: "case B wrap line",
			board: []string{lineRow("r", 5, false,
				linePanel("stat", "b1", 0, 6, 4, 4, ""), linePanel("stat", "b2", 4, 6, 8, 4, ""),
				linePanel("stat", "b3", 12, 6, 8, 4, ""), linePanel("stat", "b4", 0, 10, 8, 4, ""))},
			want: map[string]spec.GridPos{
				"b1": {X: 0, Y: 6, W: 5, H: 4}, "b2": {X: 5, Y: 6, W: 10, H: 4}, "b3": {X: 15, Y: 6, W: 9, H: 4}, "b4": {X: 0, Y: 10, W: 24, H: 4},
			},
		},
		{
			name: "mixed heights stay (case A)",
			board: []string{lineRow("row_cpu", 0, false,
				linePanel("timeseries", "usage", 0, 1, 12, 8, ""), linePanel("stat", "cores", 12, 1, 4, 4, ""), linePanel("stat", "load", 16, 1, 8, 4, ""))},
			unchanged: true,
		},
		{
			name: "a line beside a taller panel stays",
			board: []string{
				linePanel("timeseries", "a", 0, 0, 12, 16, ""), linePanel("timeseries", "b", 12, 0, 12, 8, ""), linePanel("timeseries", "c", 12, 8, 8, 8, ""),
			},
			unchanged: true,
		},
		{
			name:      "full lines stay",
			board:     []string{linePanel("stat", "a", 0, 0, 12, 8, ""), linePanel("stat", "b", 12, 0, 12, 8, "")},
			unchanged: true,
		},
		{
			name:  "collapsed row children widen among themselves",
			board: []string{lineRow("r", 0, true, linePanel("timeseries", "a", 0, 1, 12, 8, ""))},
			want:  map[string]spec.GridPos{"a": {X: 0, Y: 1, W: 24, H: 8}},
		},
		{
			name:       "a widened multi-series chart gets a right legend",
			board:      []string{withQueries(linePanel("timeseries", "a", 0, 0, 12, 8, ""), `[{"query":"up","legend":"{{route}}"}]`)},
			want:       map[string]spec.GridPos{"a": {X: 0, Y: 0, W: 24, H: 8}},
			wantLegend: map[string]string{"a": "right"},
		},
		{
			name:  "a single unnamed series keeps the default legend",
			board: []string{linePanel("timeseries", "a", 0, 0, 12, 8, "")},
			want:  map[string]spec.GridPos{"a": {X: 0, Y: 0, W: 24, H: 8}},
		},
		{
			name:      "a set legend is kept",
			board:     []string{linePanel("timeseries", "a", 0, 0, 24, 8, `,"display":{"legend":"bottom"}`)},
			unchanged: true,
		},
		{
			name:  "other types get no legend",
			board: []string{withQueries(linePanel("table", "a", 0, 0, 12, 8, ""), `[{"query":"up"},{"query":"down"}]`)},
			want:  map[string]spec.GridPos{"a": {X: 0, Y: 0, W: 24, H: 8}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := lineBoard(t, tt.board...)
			beforeRaw, _ := json.Marshal(before)
			got, err := closeLines(before)
			if err != nil {
				t.Fatalf("closeLines: %v", err)
			}
			if raw, _ := json.Marshal(before); string(raw) != string(beforeRaw) {
				t.Errorf("the input board changed")
			}
			if tt.unchanged {
				if got != before {
					t.Errorf("returned a new dashboard, want the same pointer")
				}
				return
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := spec.Parse(raw); err != nil {
				t.Fatalf("the closed board does not parse: %v", err)
			}
			for _, e := range got.AllPanels() {
				want, named := tt.want[e.Panel.ID]
				if !named {
					was, _ := before.FindPanel(e.Panel.ID)
					if e.Panel.GridPos != was.Panel.GridPos {
						t.Errorf("%s = %+v, want it kept at %+v", e.Panel.ID, e.Panel.GridPos, was.Panel.GridPos)
					}
					continue
				}
				if e.Panel.GridPos != want {
					t.Errorf("%s = %+v, want %+v", e.Panel.ID, e.Panel.GridPos, want)
				}
				wantLegend := tt.wantLegend[e.Panel.ID]
				switch {
				case wantLegend == "" && e.Panel.Display != nil:
					t.Errorf("%s display = %+v, want none", e.Panel.ID, e.Panel.Display)
				case wantLegend != "" && (e.Panel.Display == nil || e.Panel.Display.Legend != wantLegend):
					t.Errorf("%s display = %+v, want legend %q", e.Panel.ID, e.Panel.Display, wantLegend)
				}
			}
		})
	}
}
