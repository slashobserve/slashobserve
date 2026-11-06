package taste

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	Titles       = "titles"
	Units        = "units"
	Legends      = "legends"
	Descriptions = "descriptions"
	Thresholds   = "thresholds"
	FullLines    = "full_lines"
	Headline     = "headline"
	RowQuestions = "row_questions"
	LonePanels   = "lone_panels"
	Stacking     = "stacking"
	IDs          = "ids"
)

var Rules = []string{Titles, Units, Legends, Descriptions, Thresholds, FullLines, Headline, RowQuestions, LonePanels, Stacking, IDs}

type Judgement struct {
	Rule    string
	Checked int
	Failed  []string
}

func (j *Judgement) check(ok bool, failure func() string) {
	j.Checked++
	if !ok {
		j.Failed = append(j.Failed, failure())
	}
}

var (
	lineSizes      = []int{2, 3, 4, 6}
	headlineSizes  = []int{3, 4, 6}
	fullWidthTypes = []spec.PanelType{spec.Logs, spec.Table, spec.Traces, spec.StateTimeline, spec.StatusHistory, spec.NodeGraph, spec.Text}
	unitTypes      = []spec.PanelType{spec.Timeseries, spec.Stat, spec.Gauge, spec.BarGauge}
	describedTypes = []spec.PanelType{spec.Stat, spec.Gauge}
	legendTypes    = []spec.PanelType{spec.Timeseries, spec.BarGauge, spec.BarChart, spec.PieChart}
	thresholdTypes = []spec.PanelType{spec.Stat, spec.Gauge, spec.BarGauge, spec.StateTimeline}
	groupingClause = regexp.MustCompile(`\b(by|without)\s*\(([^)]*)\)`)
)

const (
	topLevelSection  = "the top level"
	quantileGrouping = "le"
)

func Judge(d *spec.Dashboard) []Judgement {
	judged := make(map[string]*Judgement, len(Rules))
	for _, rule := range Rules {
		judged[rule] = &Judgement{Rule: rule}
	}
	judgeFields(d, judged)
	judgeLines(d, judged[FullLines], judged[LonePanels])
	judgeHeadline(d, judged[Headline], judged[RowQuestions])
	judgeStacking(d, judged[Stacking])
	out := make([]Judgement, len(Rules))
	for i, rule := range Rules {
		out[i] = *judged[rule]
	}
	return out
}

func judgeFields(d *spec.Dashboard, judged map[string]*Judgement) {
	for _, entry := range d.AllPanels() {
		p := entry.Panel
		judgeID(p, judged[IDs])
		if p.Type == spec.Row {
			continue
		}
		judged[Titles].check(!blank(p.Title), func() string { return fmt.Sprintf("%s: a %s needs a title", p.ID, p.Type) })
		if slices.Contains(unitTypes, p.Type) {
			judged[Units].check(p.Unit != "", func() string { return fmt.Sprintf("%s: a %s needs a unit", p.ID, p.Type) })
		}
		if slices.Contains(describedTypes, p.Type) {
			judged[Descriptions].check(!blank(p.Description), func() string { return fmt.Sprintf("%s: a %s needs a description", p.ID, p.Type) })
		}
		if hasThresholdSteps(p) {
			judged[Thresholds].check(slices.Contains(thresholdTypes, p.Type), func() string {
				return fmt.Sprintf("%s: a %s does not show thresholds", p.ID, p.Type)
			})
		}
		for i, q := range p.Queries {
			if needsLegend(p, i) {
				judged[Legends].check(!blank(q.Legend), func() string { return fmt.Sprintf("%s: query %s needs a legend", p.ID, p.QueryRef(i)) })
			}
		}
	}
}

func judgeID(p *spec.Panel, j *Judgement) {
	prefix := "p_"
	if p.Type == spec.Row {
		prefix = "r_"
	}
	j.check(strings.HasPrefix(p.ID, prefix), func() string { return fmt.Sprintf("%s: a %s id starts with %s", p.ID, p.Type, prefix) })
}

func hasThresholdSteps(p *spec.Panel) bool {
	return slices.ContainsFunc(p.Thresholds, func(t spec.Threshold) bool { return t.Value != nil })
}

func needsLegend(p *spec.Panel, i int) bool {
	q := p.Queries[i]
	if !slices.Contains(legendTypes, p.Type) || q.Hide || q.IsExpression() || q.IsDashboardSource() {
		return false
	}
	visible := 0
	for _, other := range p.Queries {
		if !other.Hide {
			visible++
		}
	}
	return visible >= 2 || groups(q.Query)
}

func groups(query string) bool {
	for _, match := range groupingClause.FindAllStringSubmatch(query, -1) {
		if match[1] == "without" {
			return true
		}
		for label := range strings.SplitSeq(match[2], ",") {
			if strings.TrimSpace(label) != quantileGrouping {
				return true
			}
		}
	}
	return false
}

func judgeLines(d *spec.Dashboard, full, lone *Judgement) {
	var top []spec.Panel
	for _, p := range d.Panels {
		if p.Type != spec.Row {
			top = append(top, p)
		}
	}
	judgeSection(topLevelSection, top, full, lone)
	for _, p := range d.Panels {
		if p.Type == spec.Row {
			judgeSection(p.ID, p.Panels, full, lone)
		}
	}
}

func judgeSection(section string, panels []spec.Panel, full, lone *Judgement) {
	for _, line := range linesOf(panels) {
		first := line[0]
		problems := lineProblems(line)
		full.check(len(problems) == 0, func() string {
			return fmt.Sprintf("%s: line at y %d in %s %s", first.ID, first.GridPos.Y, section, strings.Join(problems, " and "))
		})
		if len(line) == 1 && first.GridPos.W == spec.GridColumns {
			lone.check(standsAlone(&first), func() string {
				return fmt.Sprintf("%s: a %s alone on its line must be a logs, table, traces, state-timeline, status-history, nodeGraph or text panel, or a timeseries with a table or right legend", first.ID, first.Type)
			})
		}
	}
}

func lineProblems(line []spec.Panel) []string {
	var problems []string
	first := line[0]
	width, nextX := 0, 0
	for _, p := range line {
		width += p.GridPos.W
		if p.GridPos.X != nextX {
			problems = append(problems, fmt.Sprintf("has %s at x %d where the panel before it ends at x %d", p.ID, p.GridPos.X, nextX))
		}
		nextX = p.GridPos.X + p.GridPos.W
		if p.GridPos.H != first.GridPos.H {
			problems = append(problems, fmt.Sprintf("has %s %d high where %s is %d high", p.ID, p.GridPos.H, first.ID, first.GridPos.H))
		}
	}
	if width != spec.GridColumns {
		problems = append(problems, fmt.Sprintf("spans %d of %d columns", width, spec.GridColumns))
	}
	if len(line) > 1 && !slices.Contains(lineSizes, len(line)) {
		problems = append(problems, fmt.Sprintf("holds %d panels, not 2, 3, 4 or 6", len(line)))
	}
	return problems
}

func standsAlone(p *spec.Panel) bool {
	if slices.Contains(fullWidthTypes, p.Type) {
		return true
	}
	return p.Type == spec.Timeseries && p.Display != nil && (p.Display.LegendMode == "table" || p.Display.Legend == "right")
}

func judgeHeadline(d *spec.Dashboard, headline, rows *Judgement) {
	var top []spec.Panel
	for _, p := range d.Panels {
		if p.GridPos.Y == 0 {
			top = append(top, p)
			continue
		}
		rows.check(p.Type == spec.Row && strings.HasSuffix(p.Title, "?"), func() string {
			if p.Type != spec.Row {
				return fmt.Sprintf("%s: a %s below the headline must sit in a row", p.ID, p.Type)
			}
			return fmt.Sprintf("%s: row title %q does not end with ?", p.ID, p.Title)
		})
	}
	headline.check(slices.Contains(headlineSizes, len(top)), func() string {
		return fmt.Sprintf("the top line holds %d panels at y 0, not 3, 4 or 6 stats", len(top))
	})
	for _, p := range top {
		var problems []string
		if p.Type != spec.Stat {
			problems = append(problems, fmt.Sprintf("the headline holds only stat panels, not %s", p.Type))
		}
		if p.GridPos.W != top[0].GridPos.W {
			problems = append(problems, fmt.Sprintf("headline width %d differs from %s's width %d", p.GridPos.W, top[0].ID, top[0].GridPos.W))
		}
		headline.check(len(problems) == 0, func() string { return p.ID + ": " + strings.Join(problems, " and ") })
	}
}

func judgeStacking(d *spec.Dashboard, j *Judgement) {
	bottom := 0
	place := func(line []spec.Panel) {
		y := line[0].GridPos.Y
		j.check(y == bottom, func() string {
			return fmt.Sprintf("%s: starts at y %d, but the line above ends at y %d", line[0].ID, y, bottom)
		})
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

func blank(s string) bool { return strings.TrimSpace(s) == "" }
