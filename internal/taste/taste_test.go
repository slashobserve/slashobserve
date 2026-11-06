package taste

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	baseStatA = `{ "id": "p_a", "gridPos": { "x": 0, "y": 0, "w": 8, "h": 4 }, "type": "stat", "title": "A", "description": "A.", "datasource": "prom", "unit": "short", "queries": [{ "query": "sum(up)" }] }`
	baseStatB = `{ "id": "p_b", "gridPos": { "x": 8, "y": 0, "w": 8, "h": 4 }, "type": "stat", "title": "B", "description": "B.", "datasource": "prom", "unit": "short", "queries": [{ "query": "sum(up)" }] }`
	baseStatC = `{ "id": "p_c", "gridPos": { "x": 16, "y": 0, "w": 8, "h": 4 }, "type": "stat", "title": "C", "description": "C.", "datasource": "prom", "unit": "short", "queries": [{ "query": "sum(up)" }] }`
	baseRow   = `{ "id": "r_a", "gridPos": { "x": 0, "y": 4, "w": 24, "h": 1 }, "type": "row", "title": "Is it up?", "panels": [`
	baseLineD = `{ "id": "p_d", "gridPos": { "x": 0, "y": 5, "w": 12, "h": 8 }, "type": "timeseries", "title": "D", "datasource": "prom", "unit": "short", "queries": [{ "query": "sum(up)" }] }`
	baseLineE = `{ "id": "p_e", "gridPos": { "x": 12, "y": 5, "w": 12, "h": 8 }, "type": "timeseries", "title": "E", "datasource": "prom", "unit": "short", "queries": [{ "query": "sum(up)" }] }`
	baseSpec  = `{ "specVersion": "0", "title": "Base", "time": { "from": "now-1h", "to": "now" }, "panels": [
  ` + baseStatA + `,
  ` + baseStatB + `,
  ` + baseStatC + `,
  ` + baseRow + `
    ` + baseLineD + `,
    ` + baseLineE + `
  ] }
] }`
)

func edited(t *testing.T, edits ...string) *spec.Dashboard {
	t.Helper()
	for i := 0; i < len(edits); i += 2 {
		if !strings.Contains(baseSpec, edits[i]) {
			t.Fatalf("edit %q matches nothing in the base", edits[i])
		}
	}
	d, err := spec.Parse([]byte(strings.NewReplacer(edits...).Replace(baseSpec)))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func failures(judgements []Judgement) []string {
	var out []string
	for _, j := range judgements {
		for _, f := range j.Failed {
			out = append(out, j.Rule+": "+f)
		}
	}
	return out
}

func TestJudgeNamesEachBrokenRule(t *testing.T) {
	quarter := func(id string, x, w int) string {
		return fmt.Sprintf(`{ "id": %q, "gridPos": { "x": %d, "y": 5, "w": %d, "h": 8 }, "type": "timeseries", "title": "Q", "datasource": "prom", "unit": "short", "queries": [{ "query": "sum(up)" }] }`, id, x, w)
	}
	alone := []string{
		`"x": 0, "y": 5, "w": 12`, `"x": 0, "y": 5, "w": 24`,
		`"x": 12, "y": 5, "w": 12`, `"x": 0, "y": 13, "w": 24`,
	}
	tests := []struct {
		name  string
		edits []string
		want  string
	}{
		{name: "valid base"},
		{
			name:  "line of 12 and 8",
			edits: []string{`"x": 12, "y": 5, "w": 12`, `"x": 12, "y": 5, "w": 8`},
			want:  "full_lines: p_d: line at y 5 in r_a spans 20 of 24 columns",
		},
		{
			name: "line of five panels",
			edits: []string{
				`"x": 0, "y": 5, "w": 12`, `"x": 0, "y": 5, "w": 4`,
				baseLineE, strings.Join([]string{quarter("p_e", 4, 4), quarter("p_f", 8, 4), quarter("p_g", 12, 4), quarter("p_h", 16, 8)}, ",\n"),
			},
			want: "full_lines: p_d: line at y 5 in r_a holds 5 panels, not 2, 3, 4 or 6",
		},
		{
			name:  "gap inside a line",
			edits: []string{`"x": 12, "y": 5, "w": 12`, `"x": 13, "y": 5, "w": 11`},
			want:  "full_lines: p_d: line at y 5 in r_a has p_e at x 13 where the panel before it ends at x 12 and spans 23 of 24 columns",
		},
		{
			name:  "uneven heights on a line",
			edits: []string{`"x": 12, "y": 5, "w": 12, "h": 8`, `"x": 12, "y": 5, "w": 12, "h": 6`},
			want:  "full_lines: p_d: line at y 5 in r_a has p_e 6 high where p_d is 8 high",
		},
		{
			name:  "single full width timeseries without a table or right legend",
			edits: alone,
			want:  "lone_panels: p_d: a timeseries alone on its line must be",
		},
		{
			name:  "single full width timeseries with a right legend",
			edits: append(slices.Clone(alone), `"title": "D", `, `"title": "D", "display": { "legend": "right" }, `, `"title": "E", `, `"title": "E", "display": { "legendMode": "table" }, `),
		},
		{
			name:  "headline holding a timeseries",
			edits: []string{baseStatC, strings.Replace(baseStatC, `"type": "stat"`, `"type": "timeseries"`, 1)},
			want:  "headline: p_c: the headline holds only stat panels, not timeseries",
		},
		{
			name:  "headline of two stats",
			edits: []string{baseStatC + ",", "", `"x": 8, "y": 0, "w": 8`, `"x": 8, "y": 0, "w": 16`},
			want:  "headline: the top line holds 2 panels at y 0, not 3, 4 or 6 stats",
		},
		{
			name:  "row title without a question mark",
			edits: []string{`"title": "Is it up?"`, `"title": "Uptime"`},
			want:  `row_questions: r_a: row title "Uptime" does not end with ?`,
		},
		{
			name:  "stat without description",
			edits: []string{baseStatA, strings.Replace(baseStatA, `"description": "A.", `, "", 1)},
			want:  "descriptions: p_a: a stat needs a description",
		},
		{
			name:  "timeseries without unit",
			edits: []string{baseLineD, strings.Replace(baseLineD, `"unit": "short", `, "", 1)},
			want:  "units: p_d: a timeseries needs a unit",
		},
		{
			name:  "stat without title",
			edits: []string{baseStatB, strings.Replace(baseStatB, `"title": "B", `, `"title": " ", `, 1)},
			want:  "titles: p_b: a stat needs a title",
		},
		{
			name:  "grouped query without legend",
			edits: []string{baseLineD, strings.Replace(baseLineD, `sum(up)`, `sum by (route) (up)`, 1)},
			want:  "legends: p_d: query A needs a legend",
		},
		{
			name:  "without grouping counts as grouping",
			edits: []string{baseLineD, strings.Replace(baseLineD, `sum(up)`, `sum without (instance) (up)`, 1)},
			want:  "legends: p_d: query A needs a legend",
		},
		{
			name:  "two queries with one unnamed",
			edits: []string{baseLineD, strings.Replace(baseLineD, `{ "query": "sum(up)" }`, `{ "query": "sum(up)", "legend": "up" }, { "query": "count(up)" }`, 1)},
			want:  "legends: p_d: query B needs a legend",
		},
		{
			name:  "a hidden second query needs no legend",
			edits: []string{baseLineD, strings.Replace(baseLineD, `{ "query": "sum(up)" }`, `{ "query": "sum(up)" }, { "query": "count(up)", "hide": true }`, 1)},
		},
		{
			name:  "quantile grouped only by le needs no legend",
			edits: []string{baseLineD, strings.Replace(baseLineD, `sum(up)`, `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))`, 1)},
		},
		{
			name:  "thresholds on a timeseries",
			edits: []string{baseLineE, strings.Replace(baseLineE, `"unit": "short", `, `"unit": "short", "thresholds": [{ "color": "ok" }, { "value": 1, "color": "critical" }], `, 1)},
			want:  "thresholds: p_e: a timeseries does not show thresholds",
		},
		{
			name:  "panel id without its prefix",
			edits: []string{`"id": "p_e"`, `"id": "e"`},
			want:  "ids: e: a timeseries id starts with p_",
		},
		{
			name: "vertical gap below the headline",
			edits: []string{
				`"x": 0, "y": 4, "w": 24, "h": 1`, `"x": 0, "y": 6, "w": 24, "h": 1`,
				`"x": 0, "y": 5, "w": 12`, `"x": 0, "y": 7, "w": 12`,
				`"x": 12, "y": 5, "w": 12`, `"x": 12, "y": 7, "w": 12`,
			},
			want: "stacking: r_a: starts at y 6, but the line above ends at y 4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := failures(Judge(edited(t, tt.edits...)))
			if tt.want == "" {
				if len(got) > 0 {
					t.Fatalf("want no failure, got %q", got)
				}
				return
			}
			if !slices.ContainsFunc(got, func(f string) bool { return strings.HasPrefix(f, tt.want) }) {
				t.Fatalf("want a failure starting %q, got %q", tt.want, got)
			}
		})
	}
}

func TestJudgeCountsWhatEachRuleChecked(t *testing.T) {
	d := edited(t, baseLineD, strings.Replace(baseLineD, `{ "query": "sum(up)" }`, `{ "query": "sum(up)", "legend": "up" }, { "query": "count(up)", "legend": "count" }`, 1))
	want := map[string]int{
		Titles: 5, Units: 5, Legends: 2, Descriptions: 3, Thresholds: 0,
		FullLines: 2, Headline: 4, RowQuestions: 1, LonePanels: 0, Stacking: 3, IDs: 6,
	}
	judgements := Judge(d)
	var rules []string
	for _, j := range judgements {
		rules = append(rules, j.Rule)
		if j.Checked != want[j.Rule] {
			t.Errorf("%s checked %d, want %d", j.Rule, j.Checked, want[j.Rule])
		}
		if len(j.Failed) > 0 {
			t.Errorf("%s failed %q", j.Rule, j.Failed)
		}
	}
	if !slices.Equal(rules, Rules) {
		t.Fatalf("rules %v, want %v in that order", rules, Rules)
	}
}

func TestJudgeFailsAtMostOncePerCheck(t *testing.T) {
	d := edited(t, baseStatC, strings.Replace(strings.Replace(baseStatC, `"type": "stat"`, `"type": "timeseries"`, 1), `"w": 8`, `"w": 7`, 1))
	for _, j := range Judge(d) {
		if len(j.Failed) > j.Checked {
			t.Errorf("%s failed %d of %d checks: %q", j.Rule, len(j.Failed), j.Checked, j.Failed)
		}
		if j.Rule == Headline && !slices.Equal(j.Failed, []string{"p_c: the headline holds only stat panels, not timeseries and headline width 7 differs from p_a's width 8"}) {
			t.Errorf("headline failed %q", j.Failed)
		}
	}
}
