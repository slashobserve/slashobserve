package provision

import (
	"cmp"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

var referenceDir = filepath.Join("..", "..", "deploy", "compose", "dashboards")

var referenceFolders = map[string]string{
	"Demo/shop-overview.json":     "Demo",
	"Demo/node-health.json":       "Demo",
	"SlashObserve/internals.json": "SlashObserve",
}

var (
	referenceDatasources = []string{"prom", "ch"}
	lineSizes            = []int{2, 3, 4, 6}
	headlineSizes        = []int{3, 4, 6}
	fullWidthTypes       = []spec.PanelType{spec.Logs, spec.Table, spec.Traces, spec.StateTimeline, spec.StatusHistory, spec.NodeGraph, spec.Text}
	unitTypes            = []spec.PanelType{spec.Timeseries, spec.Stat, spec.Gauge, spec.BarGauge}
	describedTypes       = []spec.PanelType{spec.Stat, spec.Gauge}
	legendTypes          = []spec.PanelType{spec.Timeseries, spec.BarGauge, spec.BarChart, spec.PieChart}
	groupingClause       = regexp.MustCompile(`\bby\s*\(([^)]*)\)`)
)

const gridWidth = 24

func TestReferenceDashboardsLandInTheirFolders(t *testing.T) {
	files, err := scan(referenceDir)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, f := range files {
		found[f.path] = f.folder
	}
	for path, folder := range referenceFolders {
		got, ok := found[path]
		if !ok {
			t.Errorf("%s is missing from %s", path, referenceDir)
			continue
		}
		if got != folder {
			t.Errorf("%s lands in folder %q, want %q", path, got, folder)
		}
	}
}

func TestReferenceDashboardsFollowTheLayoutRules(t *testing.T) {
	files, err := scan(referenceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no dashboard file in %s", referenceDir)
	}
	dir := os.DirFS(referenceDir)
	for _, f := range files {
		t.Run(f.path, func(t *testing.T) {
			data, err := fs.ReadFile(dir, f.path)
			if err != nil {
				t.Fatal(err)
			}
			d, err := spec.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, problem := range layoutProblems(d) {
				t.Error(problem)
			}
		})
	}
}

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

func TestLayoutProblemsNamesEachBrokenRule(t *testing.T) {
	quarter := func(id string, x, w int) string {
		return fmt.Sprintf(`{ "id": %q, "gridPos": { "x": %d, "y": 5, "w": %d, "h": 8 }, "type": "timeseries", "title": "Q", "datasource": "prom", "unit": "short", "queries": [{ "query": "sum(up)" }] }`, id, x, w)
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
			want:  "p_d: line at y 5 in r_a spans 20 of 24 columns",
		},
		{
			name: "line of five panels",
			edits: []string{
				`"x": 0, "y": 5, "w": 12`, `"x": 0, "y": 5, "w": 4`,
				baseLineE, strings.Join([]string{quarter("p_e", 4, 4), quarter("p_f", 8, 4), quarter("p_g", 12, 4), quarter("p_h", 16, 8)}, ",\n"),
			},
			want: "p_d: line at y 5 in r_a holds 5 panels, not 2, 3, 4 or 6",
		},
		{
			name: "single full width timeseries without a table legend",
			edits: []string{
				`"x": 0, "y": 5, "w": 12`, `"x": 0, "y": 5, "w": 24`,
				`"x": 12, "y": 5, "w": 12`, `"x": 0, "y": 13, "w": 24`,
			},
			want: "p_d: a timeseries alone on its line must be",
		},
		{
			name:  "headline holding a timeseries",
			edits: []string{baseStatC, strings.Replace(baseStatC, `"type": "stat"`, `"type": "timeseries"`, 1)},
			want:  "p_c: the headline holds only stat panels, not timeseries",
		},
		{
			name:  "row title without a question mark",
			edits: []string{`"title": "Is it up?"`, `"title": "Uptime"`},
			want:  `r_a: row title "Uptime" does not end with ?`,
		},
		{
			name:  "stat without description",
			edits: []string{baseStatA, strings.Replace(baseStatA, `"description": "A.", `, "", 1)},
			want:  "p_a: a stat needs a description",
		},
		{
			name:  "timeseries without unit",
			edits: []string{baseLineD, strings.Replace(baseLineD, `"unit": "short", `, "", 1)},
			want:  "p_d: a timeseries needs a unit",
		},
		{
			name:  "grouped query without legend",
			edits: []string{baseLineD, strings.Replace(baseLineD, `sum(up)`, `sum by (route) (up)`, 1)},
			want:  "p_d: query 1 needs a legend",
		},
		{
			name:  "two queries with one unnamed",
			edits: []string{baseLineD, strings.Replace(baseLineD, `{ "query": "sum(up)" }`, `{ "query": "sum(up)", "legend": "up" }, { "query": "count(up)" }`, 1)},
			want:  "p_d: query 2 needs a legend",
		},
		{
			name:  "quantile grouped only by le needs no legend",
			edits: []string{baseLineD, strings.Replace(baseLineD, `sum(up)`, `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))`, 1)},
		},
		{
			name:  "panel on loki",
			edits: []string{baseLineD, strings.Replace(baseLineD, `"datasource": "prom"`, `"datasource": "loki"`, 1)},
			want:  "p_d: datasource loki is not one of prom, ch",
		},
		{
			name: "vertical gap below the headline",
			edits: []string{
				`"x": 0, "y": 4, "w": 24, "h": 1`, `"x": 0, "y": 6, "w": 24, "h": 1`,
				`"x": 0, "y": 5, "w": 12`, `"x": 0, "y": 7, "w": 12`,
				`"x": 12, "y": 5, "w": 12`, `"x": 12, "y": 7, "w": 12`,
			},
			want: "r_a: starts at y 6, but the line above ends at y 4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < len(tt.edits); i += 2 {
				if !strings.Contains(baseSpec, tt.edits[i]) {
					t.Fatalf("edit %q matches nothing in the base", tt.edits[i])
				}
			}
			d, err := spec.Parse([]byte(strings.NewReplacer(tt.edits...).Replace(baseSpec)))
			if err != nil {
				t.Fatal(err)
			}
			problems := layoutProblems(d)
			if tt.want == "" {
				if len(problems) > 0 {
					t.Fatalf("want no problem, got %q", problems)
				}
				return
			}
			if !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, tt.want) }) {
				t.Fatalf("want a problem containing %q, got %q", tt.want, problems)
			}
		})
	}
}

func layoutProblems(d *spec.Dashboard) []string {
	problems := headlineProblems(d)
	var top []spec.Panel
	for _, p := range d.Panels {
		if p.Type != spec.Row {
			top = append(top, p)
		}
	}
	problems = append(problems, lineProblems("top level", top)...)
	for _, p := range d.Panels {
		if p.Type == spec.Row {
			problems = append(problems, lineProblems(p.ID, p.Panels)...)
		}
	}
	problems = append(problems, stackingProblems(d)...)
	for _, entry := range d.AllPanels() {
		problems = append(problems, panelProblems(entry.Panel)...)
	}
	return problems
}

func headlineProblems(d *spec.Dashboard) []string {
	var problems []string
	var headline []spec.Panel
	for _, p := range d.Panels {
		if p.GridPos.Y == 0 {
			headline = append(headline, p)
			continue
		}
		if p.Type != spec.Row {
			problems = append(problems, fmt.Sprintf("%s: a %s below the headline must sit in a row", p.ID, p.Type))
			continue
		}
		if !strings.HasSuffix(p.Title, "?") {
			problems = append(problems, fmt.Sprintf("%s: row title %q does not end with ?", p.ID, p.Title))
		}
	}
	if !slices.Contains(headlineSizes, len(headline)) {
		problems = append(problems, fmt.Sprintf("headline: holds %d panels at y 0, not 3, 4 or 6 stats", len(headline)))
	}
	for _, p := range headline {
		if p.Type != spec.Stat {
			problems = append(problems, fmt.Sprintf("%s: the headline holds only stat panels, not %s", p.ID, p.Type))
		}
		if p.GridPos.W != headline[0].GridPos.W {
			problems = append(problems, fmt.Sprintf("%s: headline width %d differs from %s's width %d", p.ID, p.GridPos.W, headline[0].ID, headline[0].GridPos.W))
		}
	}
	return problems
}

func lineProblems(parent string, panels []spec.Panel) []string {
	var problems []string
	for _, line := range linesOf(panels) {
		first := line[0]
		width, nextX := 0, 0
		for _, p := range line {
			width += p.GridPos.W
			if p.GridPos.X != nextX {
				problems = append(problems, fmt.Sprintf("%s: starts at x %d, but the panel before it ends at x %d", p.ID, p.GridPos.X, nextX))
			}
			nextX = p.GridPos.X + p.GridPos.W
			if p.GridPos.H != first.GridPos.H {
				problems = append(problems, fmt.Sprintf("%s: height %d differs from %s's height %d on its line", p.ID, p.GridPos.H, first.ID, first.GridPos.H))
			}
		}
		if width != gridWidth {
			problems = append(problems, fmt.Sprintf("%s: line at y %d in %s spans %d of %d columns", first.ID, first.GridPos.Y, parent, width, gridWidth))
		}
		switch {
		case len(line) == 1:
			if first.GridPos.W == gridWidth && !standsAlone(&first) {
				problems = append(problems, fmt.Sprintf("%s: a %s alone on its line must be a logs, table, traces, state-timeline, status-history, nodeGraph or text panel, or a timeseries with a table legend", first.ID, first.Type))
			}
		case !slices.Contains(lineSizes, len(line)):
			problems = append(problems, fmt.Sprintf("%s: line at y %d in %s holds %d panels, not 2, 3, 4 or 6", first.ID, first.GridPos.Y, parent, len(line)))
		}
	}
	return problems
}

func standsAlone(p *spec.Panel) bool {
	if slices.Contains(fullWidthTypes, p.Type) {
		return true
	}
	return p.Type == spec.Timeseries && p.Display != nil && p.Display.LegendMode == "table"
}

func stackingProblems(d *spec.Dashboard) []string {
	var problems []string
	bottom := 0
	place := func(line []spec.Panel) {
		y := line[0].GridPos.Y
		if y != bottom {
			problems = append(problems, fmt.Sprintf("%s: starts at y %d, but the line above ends at y %d", line[0].ID, y, bottom))
		}
		for _, p := range line {
			bottom = max(bottom, p.GridPos.Y+p.GridPos.H)
		}
	}
	for _, line := range linesOf(d.Panels) {
		place(line)
		for _, p := range line {
			if p.Type == spec.Row {
				for _, child := range linesOf(p.Panels) {
					place(child)
				}
			}
		}
	}
	return problems
}

func panelProblems(p *spec.Panel) []string {
	var problems []string
	prefix := "p_"
	if p.Type == spec.Row {
		prefix = "r_"
	}
	if !strings.HasPrefix(p.ID, prefix) {
		problems = append(problems, fmt.Sprintf("%s: a %s id starts with %s", p.ID, p.Type, prefix))
	}
	if p.Type != spec.Row && p.Title == "" {
		problems = append(problems, fmt.Sprintf("%s: a %s needs a title", p.ID, p.Type))
	}
	if slices.Contains(unitTypes, p.Type) && p.Unit == "" {
		problems = append(problems, fmt.Sprintf("%s: a %s needs a unit", p.ID, p.Type))
	}
	if slices.Contains(describedTypes, p.Type) && p.Description == "" {
		problems = append(problems, fmt.Sprintf("%s: a %s needs a description", p.ID, p.Type))
	}
	var datasources []string
	if p.Datasource != "" {
		datasources = append(datasources, p.Datasource)
	}
	for i := range p.Queries {
		datasources = append(datasources, p.QueryDatasource(i))
	}
	slices.Sort(datasources)
	for _, ds := range slices.Compact(datasources) {
		if !slices.Contains(referenceDatasources, ds) {
			problems = append(problems, fmt.Sprintf("%s: datasource %s is not one of %s", p.ID, ds, strings.Join(referenceDatasources, ", ")))
		}
	}
	for i, q := range p.Queries {
		if needsLegend(p, i) && q.Legend == "" {
			problems = append(problems, fmt.Sprintf("%s: query %d needs a legend", p.ID, i+1))
		}
	}
	return problems
}

func needsLegend(p *spec.Panel, i int) bool {
	q := p.Queries[i]
	if !slices.Contains(legendTypes, p.Type) || q.Hide || p.QueryDatasource(i) != "prom" {
		return false
	}
	visible := 0
	for _, other := range p.Queries {
		if !other.Hide {
			visible++
		}
	}
	if visible >= 2 {
		return true
	}
	for _, match := range groupingClause.FindAllStringSubmatch(q.Query, -1) {
		for label := range strings.SplitSeq(match[1], ",") {
			if strings.TrimSpace(label) != "le" {
				return true
			}
		}
	}
	return false
}

func linesOf(panels []spec.Panel) [][]spec.Panel {
	sorted := slices.Clone(panels)
	slices.SortStableFunc(sorted, func(a, b spec.Panel) int {
		return cmp.Or(cmp.Compare(a.GridPos.Y, b.GridPos.Y), cmp.Compare(a.GridPos.X, b.GridPos.X))
	})
	var lines [][]spec.Panel
	for _, p := range sorted {
		if n := len(lines); n > 0 && lines[n-1][0].GridPos.Y == p.GridPos.Y {
			lines[n-1] = append(lines[n-1], p)
			continue
		}
		lines = append(lines, []spec.Panel{p})
	}
	return lines
}
