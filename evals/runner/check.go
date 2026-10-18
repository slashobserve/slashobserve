package main

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type CheckResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type panelOutcome struct {
	data PanelData
	err  error
}

type observation struct {
	request   AgentRequest
	specErr   string
	dashboard *spec.Dashboard
	panels    map[string]panelOutcome

	datasourceTypes map[string]string
}

func dataPanels(d *spec.Dashboard) []*spec.Panel {
	entries := d.DataPanels()
	out := make([]*spec.Panel, len(entries))
	for i, e := range entries {
		out[i] = e.Panel
	}
	return out
}

func (o observation) panelDatasourceTypes(p *spec.Panel) []string {
	var types []string
	for j := range p.Queries {
		uid := p.QueryDatasource(j)
		t, ok := o.datasourceTypes[uid]
		if !ok {
			t = uid
		}
		if !slices.Contains(types, t) {
			types = append(types, t)
		}
	}
	return types
}

func runChecks(sc Scenario, obs observation) []CheckResult {
	ex := sc.Expect
	checks := []CheckResult{checkValidity(obs), checkData(obs), checkSource(ex, obs)}
	if len(ex.Metrics) > 0 {
		checks = append(checks, checkQueries("metrics", ex.Metrics, spec.Prometheus, obs))
	}
	if len(ex.Tables) > 0 {
		checks = append(checks, checkQueries("tables", ex.Tables, spec.ClickHouse, obs))
	}
	if len(ex.Units) > 0 {
		checks = append(checks, checkUnits(ex, obs))
	}
	if len(ex.QueryContainsAny) > 0 {
		checks = append(checks, checkQueryGroups(ex, obs))
	}
	if len(ex.TimeRangeFrom) > 0 {
		checks = append(checks, checkTimeRange(ex, obs))
	}
	checks = append(checks, checkPanels(ex, obs))
	if ex.Thresholds || len(ex.ThresholdAt) > 0 {
		checks = append(checks, checkThresholds(ex, obs))
	}
	return append(checks, checkLatency(ex, obs))
}

func allPassed(checks []CheckResult) bool {
	return !slices.ContainsFunc(checks, func(c CheckResult) bool { return !c.Passed })
}

func result(name string, passed bool, detail string) CheckResult {
	if passed {
		detail = ""
	}
	return CheckResult{Name: name, Passed: passed, Detail: detail}
}

func noSpec(name string, obs observation) (CheckResult, bool) {
	if obs.dashboard != nil {
		return CheckResult{}, false
	}
	return CheckResult{Name: name, Detail: "no valid saved spec"}, true
}

func checkValidity(obs observation) CheckResult {
	switch {
	case obs.request.Status != "succeeded":
		return result("validity", false, fmt.Sprintf("request %s", obs.request.Status))
	case obs.specErr != "":
		return result("validity", false, obs.specErr)
	}
	return result("validity", true, "")
}

func checkData(obs observation) CheckResult {
	if c, skip := noSpec("data", obs); skip {
		return c
	}
	var failures []string
	for _, panel := range dataPanels(obs.dashboard) {
		outcome, ok := obs.panels[panel.ID]
		switch {
		case !ok:
			failures = append(failures, panel.ID+": not fetched")
		case outcome.err != nil:
			failures = append(failures, panel.ID+": "+outcome.err.Error())
		case !hasValue(outcome.data):
			failures = append(failures, panel.ID+": no data")
		}
	}
	return result("data", len(failures) == 0, strings.Join(failures, "; "))
}

func hasValue(data PanelData) bool {
	for _, frame := range data.Frames {
		for _, field := range frame.Fields {
			if field.Type == "time" {
				continue
			}
			if slices.ContainsFunc(field.Values, func(v any) bool { return v != nil }) {
				return true
			}
		}
	}
	return false
}

func checkSource(ex Expect, obs observation) CheckResult {
	if c, skip := noSpec("source", obs); skip {
		return c
	}
	got := map[string]bool{}
	for _, panel := range dataPanels(obs.dashboard) {
		for _, t := range obs.panelDatasourceTypes(panel) {
			got[t] = true
		}
	}
	names := make([]string, 0, len(got))
	for name := range got {
		names = append(names, name)
	}
	slices.Sort(names)
	want := slices.Clone(ex.Datasources)
	slices.Sort(want)
	return result("source", slices.Equal(names, want), fmt.Sprintf("panels use %v, expected %v", names, want))
}

func checkQueries(name string, needles []string, kind spec.DatasourceType, obs observation) CheckResult {
	if c, skip := noSpec(name, obs); skip {
		return c
	}
	var missing []string
	for _, needle := range needles {
		found := slices.ContainsFunc(dataPanels(obs.dashboard), func(p *spec.Panel) bool {
			return slices.Contains(obs.panelDatasourceTypes(p), string(kind)) && strings.Contains(obs.searchableQuery(p), needle)
		})
		if !found {
			missing = append(missing, needle)
		}
	}
	return result(name, len(missing) == 0, "no "+string(kind)+" panel query contains "+strings.Join(missing, ", "))
}

func checkUnits(ex Expect, obs observation) CheckResult {
	if c, skip := noSpec("units", obs); skip {
		return c
	}
	found := slices.ContainsFunc(dataPanels(obs.dashboard), func(p *spec.Panel) bool {
		return slices.Contains(ex.Units, p.Unit)
	})
	return result("units", found, fmt.Sprintf("no panel unit in %v", ex.Units))
}

func checkPanels(ex Expect, obs observation) CheckResult {
	if c, skip := noSpec("panels", obs); skip {
		return c
	}
	n := len(dataPanels(obs.dashboard))
	return result("panels", n >= ex.MinPanels && n <= ex.MaxPanels, fmt.Sprintf("%d panels, expected %d to %d", n, ex.MinPanels, ex.MaxPanels))
}

func checkThresholds(ex Expect, obs observation) CheckResult {
	if c, skip := noSpec("thresholds", obs); skip {
		return c
	}
	var failures []string
	if ex.Thresholds && !slices.ContainsFunc(dataPanels(obs.dashboard), func(p *spec.Panel) bool { return len(p.Thresholds) > 0 }) {
		failures = append(failures, "no panel has thresholds")
	}
	if len(ex.ThresholdAt) > 0 && !slices.ContainsFunc(dataPanels(obs.dashboard), func(p *spec.Panel) bool { return hasThresholdAt(p, ex.ThresholdAt) }) {
		failures = append(failures, fmt.Sprintf("no panel has a threshold at %v for its unit", ex.ThresholdAt))
	}
	return result("thresholds", len(failures) == 0, strings.Join(failures, "; "))
}

func hasThresholdAt(p *spec.Panel, at map[string]float64) bool {
	want, ok := at[p.Unit]
	if !ok {
		return false
	}
	return slices.ContainsFunc(p.Thresholds, func(t spec.Threshold) bool { return t.Value != nil && math.Abs(*t.Value-want) < 1e-9 })
}

func checkQueryGroups(ex Expect, obs observation) CheckResult {
	if c, skip := noSpec("query", obs); skip {
		return c
	}
	panels := dataPanels(obs.dashboard)
	queries := make([]string, len(panels))
	for i, p := range panels {
		queries[i] = obs.searchableQuery(p)
	}
	var missing []string
	for _, group := range ex.QueryContainsAny {
		patterns := make([]*regexp.Regexp, len(group))
		for i, alt := range group {
			patterns[i] = alternativePattern(alt)
		}
		found := slices.ContainsFunc(queries, func(q string) bool {
			return slices.ContainsFunc(patterns, func(re *regexp.Regexp) bool { return re.MatchString(q) })
		})
		if !found {
			missing = append(missing, fmt.Sprintf("%v", group))
		}
	}
	return result("query", len(missing) == 0, "no panel query contains any of "+strings.Join(missing, ", "))
}

func alternativePattern(alt string) *regexp.Regexp {
	words := strings.Fields(alt)
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	pattern := strings.Join(words, `\s+`)
	if r, _ := utf8.DecodeRuneInString(alt); unicode.IsLetter(r) || unicode.IsDigit(r) {
		pattern = `\b` + pattern
	}
	if r, _ := utf8.DecodeLastRuneInString(alt); unicode.IsLetter(r) || unicode.IsDigit(r) {
		pattern += `\b`
	}
	return regexp.MustCompile(`(?i)` + pattern)
}

func (o observation) searchableQuery(p *spec.Panel) string {
	texts := make([]string, len(p.Queries))
	for j, q := range p.Queries {
		texts[j] = q.Query
		if o.datasourceTypes[p.QueryDatasource(j)] == string(spec.Prometheus) {
			texts[j] = stripPromQLComments(q.Query)
		}
	}
	return strings.Join(texts, "\n")
}

func stripPromQLComments(q string) string {
	var b strings.Builder
	var quote rune
	escaped, comment := false, false
	for _, r := range q {
		switch {
		case comment:
			if r != '\n' {
				continue
			}
			comment = false
		case quote != 0:
			switch {
			case escaped:
				escaped = false
			case r == '\\' && quote != '`':
				escaped = true
			case r == quote:
				quote = 0
			}
		case r == '"' || r == '\'' || r == '`':
			quote = r
		case r == '#':
			comment = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func checkTimeRange(ex Expect, obs observation) CheckResult {
	if c, skip := noSpec("time_range", obs); skip {
		return c
	}
	from := obs.dashboard.Time.From
	return result("time_range", slices.Contains(ex.TimeRangeFrom, from), fmt.Sprintf("time.from is %q, expected one of %v", from, ex.TimeRangeFrom))
}

func checkLatency(ex Expect, obs observation) CheckResult {
	elapsed, ok := latency(obs.request)
	if !ok {
		return result("latency", false, "the request did not finish")
	}
	limit := time.Duration(ex.MaxSeconds) * time.Second
	return result("latency", elapsed <= limit, fmt.Sprintf("took %.1fs, limit %ds", elapsed.Seconds(), ex.MaxSeconds))
}

func latency(req AgentRequest) (time.Duration, bool) {
	if req.FinishedAt == nil || req.CreatedAt.IsZero() {
		return 0, false
	}
	return req.FinishedAt.Sub(req.CreatedAt), true
}
