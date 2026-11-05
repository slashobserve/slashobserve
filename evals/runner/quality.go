package main

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	gridColumns          = 24
	headlineMinPanels    = 5
	maxListedQualityIDs  = 5
	qualityRuleTitles    = "titles"
	qualityRuleUnits     = "units"
	qualityRuleLegends   = "legends"
	qualityRuleDescribed = "descriptions"
	qualityRuleFullRows  = "full_rows"
	qualityRuleHeadline  = "headline"
	qualityRuleThreshold = "thresholds"
)

type qualityRule struct {
	name   string
	weight float64
	judge  func(d *spec.Dashboard) qualityJudgement
}

type qualityJudgement struct {
	share  float64
	failed []string
	what   string
}

var qualityRules = []qualityRule{
	{qualityRuleTitles, 0.15, judgeTitles},
	{qualityRuleUnits, 0.20, judgeUnits},
	{qualityRuleLegends, 0.15, judgeLegends},
	{qualityRuleDescribed, 0.10, judgeDescriptions},
	{qualityRuleFullRows, 0.15, judgeFullRows},
	{qualityRuleHeadline, 0.10, judgeHeadline},
	{qualityRuleThreshold, 0.15, judgeThresholds},
}

var (
	numericPanelTypes   = []spec.PanelType{spec.Stat, spec.Gauge, spec.BarGauge, spec.Timeseries, spec.BarChart}
	describedPanelTypes = []spec.PanelType{spec.Stat, spec.Gauge}
	thresholdPanelTypes = []spec.PanelType{spec.Stat, spec.Gauge, spec.BarGauge, spec.StateTimeline}

	groupingClause = regexp.MustCompile(`\b(by|without)\s*\(`)
)

func scoreQuality(d *spec.Dashboard) (float64, []string) {
	score := 0.0
	var findings []string
	for _, rule := range qualityRules {
		j := rule.judge(d)
		score += rule.weight * j.share
		if len(j.failed) > 0 {
			findings = append(findings, fmt.Sprintf("%s: %s %s", rule.name, listIDs(j.failed), j.what))
		}
	}
	return score, findings
}

func listIDs(ids []string) string {
	if len(ids) <= maxListedQualityIDs {
		return strings.Join(ids, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(ids[:maxListedQualityIDs], ", "), len(ids)-maxListedQualityIDs)
}

func judgePanels(panels []*spec.Panel, applies, ok func(*spec.Panel) bool, what string) qualityJudgement {
	total := 0
	var failed []string
	for _, p := range panels {
		if !applies(p) {
			continue
		}
		total++
		if !ok(p) {
			failed = append(failed, p.ID)
		}
	}
	return judged(total, failed, what)
}

func judged(total int, failed []string, what string) qualityJudgement {
	if total == 0 {
		return qualityJudgement{share: 1}
	}
	return qualityJudgement{share: float64(total-len(failed)) / float64(total), failed: failed, what: what}
}

func every(*spec.Panel) bool { return true }

func ofType(types []spec.PanelType) func(*spec.Panel) bool {
	return func(p *spec.Panel) bool { return slices.Contains(types, p.Type) }
}

func judgeTitles(d *spec.Dashboard) qualityJudgement {
	return judgePanels(dataPanels(d), every, func(p *spec.Panel) bool { return !blank(p.Title) }, "have no title")
}

func judgeUnits(d *spec.Dashboard) qualityJudgement {
	return judgePanels(dataPanels(d), ofType(numericPanelTypes), func(p *spec.Panel) bool { return p.Unit != "" }, "show numbers without a unit")
}

func judgeDescriptions(d *spec.Dashboard) qualityJudgement {
	return judgePanels(dataPanels(d), ofType(describedPanelTypes), func(p *spec.Panel) bool { return !blank(p.Description) }, "are stats or gauges without a description")
}

func judgeThresholds(d *spec.Dashboard) qualityJudgement {
	return judgePanels(dataPanels(d), hasThresholdSteps, ofType(thresholdPanelTypes), "have thresholds on a panel type that does not show them")
}

func hasThresholdSteps(p *spec.Panel) bool {
	return slices.ContainsFunc(p.Thresholds, func(t spec.Threshold) bool { return t.Value != nil })
}

func judgeLegends(d *spec.Dashboard) qualityJudgement {
	total := 0
	var failed []string
	for _, p := range dataPanels(d) {
		if p.Type != spec.Timeseries {
			continue
		}
		for i, q := range p.Queries {
			if q.Hide || q.IsExpression() || q.IsDashboardSource() || (len(p.Queries) == 1 && !groupingClause.MatchString(q.Query)) {
				continue
			}
			total++
			if blank(q.Legend) {
				failed = append(failed, p.ID+"/"+p.QueryRef(i))
			}
		}
	}
	return judged(total, failed, "are multi-series time series queries without a legend")
}

type gridLine struct {
	section string
	y       int
}

func gridSections(d *spec.Dashboard) map[string][]*spec.Panel {
	sections := map[string][]*spec.Panel{}
	for _, e := range d.AllPanels() {
		if e.Panel.Type == spec.Row {
			continue
		}
		section := ""
		if e.Row != nil {
			section = e.Row.ID
		}
		sections[section] = append(sections[section], e.Panel)
	}
	return sections
}

func lineStarts(panels []*spec.Panel) []int {
	starts := map[int]bool{}
	for _, p := range panels {
		starts[p.GridPos.Y] = true
	}
	return slices.Sorted(maps.Keys(starts))
}

func judgeFullRows(d *spec.Dashboard) qualityJudgement {
	sections := gridSections(d)
	total := 0
	var failed []string
	for _, section := range slices.Sorted(maps.Keys(sections)) {
		panels := sections[section]
		for _, y := range lineStarts(panels) {
			total++
			width := 0
			for _, p := range panels {
				if p.GridPos.Y <= y && y < p.GridPos.Y+max(1, p.GridPos.H) {
					width += p.GridPos.W
				}
			}
			if width != gridColumns {
				failed = append(failed, lineLabel(gridLine{section, y}))
			}
		}
	}
	return judged(total, failed, fmt.Sprintf("do not fill the %d columns", gridColumns))
}

func lineLabel(l gridLine) string {
	if l.section == "" {
		return fmt.Sprintf("line y=%d", l.y)
	}
	return fmt.Sprintf("line y=%d of row %s", l.y, l.section)
}

func judgeHeadline(d *spec.Dashboard) qualityJudgement {
	if len(dataPanels(d)) < headlineMinPanels {
		return qualityJudgement{share: 1}
	}
	for _, panels := range gridSections(d) {
		top := lineStarts(panels)[0]
		first := slices.DeleteFunc(slices.Clone(panels), func(p *spec.Panel) bool { return p.GridPos.Y != top })
		if !slices.ContainsFunc(first, func(p *spec.Panel) bool { return p.Type != spec.Stat }) {
			return qualityJudgement{share: 1}
		}
	}
	return qualityJudgement{failed: []string{"the dashboard"}, what: fmt.Sprintf("has more than %d panels and no section opens with a line of stats", headlineMinPanels-1)}
}
