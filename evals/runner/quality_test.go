package main

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func qualityStat(id string, x int) spec.Panel {
	limit := 0.05
	p := spec.Panel{
		ID: id, Type: spec.Stat, Title: "Stat " + id, Description: "What " + id + " means",
		GridPos: spec.GridPos{X: x, Y: 0, W: 6, H: 4},
		Queries: []spec.Query{{Query: "sum(rate(http_requests_total[5m]))"}},
	}
	p.Unit = "reqps"
	p.Thresholds = []spec.Threshold{{Color: "green"}, {Value: &limit, Color: "red"}}
	return p
}

func qualitySeries(id string, x int, query, legend, unit string) spec.Panel {
	p := spec.Panel{
		ID: id, Type: spec.Timeseries, Title: "Series " + id,
		GridPos: spec.GridPos{X: x, Y: 4, W: 12, H: 8},
		Queries: []spec.Query{{Query: query, Legend: legend}},
	}
	p.Unit = unit
	return p
}

func perfectQualityDashboard() *spec.Dashboard {
	table := spec.Panel{
		ID: "slowest", Type: spec.Table, Title: "Slowest routes",
		GridPos: spec.GridPos{X: 0, Y: 13, W: 24, H: 8},
		Queries: []spec.Query{{Query: "topk(5, sum by (route) (rate(http_requests_total[5m])))"}},
	}
	return &spec.Dashboard{Title: "Checkout", Panels: []spec.Panel{
		qualityStat("rps", 0), qualityStat("errors", 6), qualityStat("p95", 12), qualityStat("saturation", 18),
		qualitySeries("by-route", 0, "sum by (route) (rate(http_requests_total[5m]))", "{{route}}", "reqps"),
		qualitySeries("total", 12, "sum(rate(http_requests_total[5m]))", "", "reqps"),
		{ID: "detail", Type: spec.Row, Title: "Detail", GridPos: spec.GridPos{X: 0, Y: 12, W: 24, H: 1}, Panels: []spec.Panel{table}},
	}}
}

func panelByID(t *testing.T, d *spec.Dashboard, id string) *spec.Panel {
	t.Helper()
	e, ok := d.FindPanel(id)
	if !ok {
		t.Fatalf("no panel %s", id)
	}
	return e.Panel
}

func assertScore(t *testing.T, d *spec.Dashboard, want float64) []string {
	t.Helper()
	score, findings := scoreQuality(d)
	if math.Abs(score-want) > 1e-9 {
		t.Fatalf("quality = %v, want %v; findings %q", score, want, findings)
	}
	return findings
}

func TestQualityWeightsSumToOne(t *testing.T) {
	total := 0.0
	for _, rule := range qualityRules {
		total += rule.weight
	}
	if math.Abs(total-1) > 1e-9 {
		t.Fatalf("rule weights sum to %v", total)
	}
}

func TestQualityOfAPerfectDashboardIsOne(t *testing.T) {
	if findings := assertScore(t, perfectQualityDashboard(), 1); len(findings) != 0 {
		t.Fatalf("findings = %q", findings)
	}
}

func TestQualityWithoutUnitsAndLegendsLosesBothWeights(t *testing.T) {
	d := perfectQualityDashboard()
	for _, e := range d.DataPanels() {
		e.Panel.Unit = ""
	}
	panelByID(t, d, "by-route").Queries[0].Legend = ""
	findings := assertScore(t, d, 0.65)
	want := []string{
		"units: rps, errors, p95, saturation, by-route and 1 more show numbers without a unit",
		"legends: by-route/A are multi-series time series queries without a legend",
	}
	if !slices.Equal(findings, want) {
		t.Fatalf("findings = %q, want %q", findings, want)
	}
}

func TestQualityScoresAPerPanelRuleByItsShareOfPanels(t *testing.T) {
	d := perfectQualityDashboard()
	panelByID(t, d, "rps").Unit = ""
	panelByID(t, d, "total").Unit = ""
	panelByID(t, d, "errors").Description = " "
	assertScore(t, d, 1-0.20*2/6-0.10*1/4)
}

func TestQualityCountsEveryQueryOfAMultiQueryTimeseries(t *testing.T) {
	d := perfectQualityDashboard()
	total := panelByID(t, d, "total")
	total.Queries = append(total.Queries, spec.Query{Query: "sum(rate(http_requests_total{status=~\"5..\"}[5m]))", Legend: "errors"}, spec.Query{Query: "vector(1)", Hide: true})
	findings := assertScore(t, d, 1-0.15*1/3)
	if !slices.Equal(findings, []string{"legends: total/A are multi-series time series queries without a legend"}) {
		t.Fatalf("findings = %q", findings)
	}
}

func TestQualityFlagsLinesThatDoNotFillTheGrid(t *testing.T) {
	d := perfectQualityDashboard()
	panelByID(t, d, "total").GridPos.W = 6
	panelByID(t, d, "slowest").GridPos.W = 12
	findings := assertScore(t, d, 1-0.15*2/3)
	if !slices.Contains(findings, "full_rows: line y=4, line y=13 of row detail do not fill the 24 columns") {
		t.Fatalf("findings = %q", findings)
	}
}

func TestQualityCountsATallPanelOnEveryLineItSpans(t *testing.T) {
	tall := spec.Panel{ID: "tall", Type: spec.Text, GridPos: spec.GridPos{X: 0, Y: 0, W: 12, H: 8}}
	top := qualitySeries("top", 12, "sum(up)", "", "short")
	top.GridPos = spec.GridPos{X: 12, Y: 0, W: 12, H: 4}
	bottom := qualitySeries("bottom", 12, "sum(up)", "", "short")
	bottom.GridPos = spec.GridPos{X: 12, Y: 4, W: 12, H: 4}
	assertScore(t, &spec.Dashboard{Panels: []spec.Panel{tall, top, bottom}}, 1)
}

func TestQualityWantsAHeadlineOfStatsOnlyAboveFourPanels(t *testing.T) {
	d := perfectQualityDashboard()
	panelByID(t, d, "rps").Type = spec.Gauge
	findings := assertScore(t, d, 0.9)
	if !slices.Contains(findings, "headline: the dashboard has more than 4 panels and no section opens with a line of stats") {
		t.Fatalf("findings = %q", findings)
	}

	small := &spec.Dashboard{Panels: d.Panels[:4]}
	assertScore(t, small, 1)
}

func TestQualityAcceptsAHeadlineThatOpensARow(t *testing.T) {
	d := perfectQualityDashboard()
	for i := range 4 {
		d.Panels[i].Type = spec.Gauge
	}
	row := &d.Panels[len(d.Panels)-1]
	row.Panels = append(row.Panels, qualityStat("in-row", 0))
	row.Panels[1].GridPos.Y = 13
	row.Panels[0].GridPos.Y = 17
	row.Panels[1].GridPos.W = 24
	assertScore(t, d, 1)
}

func TestQualityFlagsThresholdsOnPanelsThatDoNotShowThem(t *testing.T) {
	d := perfectQualityDashboard()
	limit := 1.0
	panelByID(t, d, "by-route").Thresholds = []spec.Threshold{{Color: "green"}, {Value: &limit, Color: "red"}}
	panelByID(t, d, "total").Thresholds = []spec.Threshold{{Color: "green"}}
	findings := assertScore(t, d, 1-0.15*1/5)
	if !slices.Contains(findings, "thresholds: by-route have thresholds on a panel type that does not show them") {
		t.Fatalf("findings = %q", findings)
	}
}

func TestQualityListsTitlesMissingFromDataPanels(t *testing.T) {
	d := perfectQualityDashboard()
	panelByID(t, d, "slowest").Title = ""
	panelByID(t, d, "detail").Title = ""
	findings := assertScore(t, d, 1-0.15*1/7)
	if !slices.Contains(findings, "titles: slowest have no title") {
		t.Fatalf("findings = %q", findings)
	}
}

func TestQualityMedianPerKindAndItsGate(t *testing.T) {
	score := func(v float64) *float64 { return &v }
	results := []Result{
		{ID: "a", Kind: kindCreate, Quality: score(0.5)},
		{ID: "b", Kind: kindCreate, Quality: score(0.9)},
		{ID: "c", Kind: kindCreate, Quality: score(0.8)},
		{ID: "d", Kind: kindRevise, Quality: score(0.6)},
		{ID: "e", Kind: kindRevise},
		{ID: "f", Kind: kindSuggest},
		{ID: "g", Kind: kindCreate, Skipped: true, Quality: score(0)},
	}
	agg := aggregate(results)
	want := map[string]float64{kindCreate: 0.8, kindRevise: 0.6}
	if len(agg.QualityMedian) != len(want) || agg.QualityMedian[kindCreate] != 0.8 || agg.QualityMedian[kindRevise] != 0.6 {
		t.Fatalf("medians = %v, want %v", agg.QualityMedian, want)
	}
	if g := evaluateGates(agg, gateLimits{}); !g.ok() {
		t.Fatalf("--min-quality 0 must be off: %v", g.failures)
	}
	if g := evaluateGates(agg, gateLimits{minQuality: 0.6}); !g.ok() {
		t.Fatalf("medians at the limit pass: %v", g.failures)
	}
	g := evaluateGates(agg, gateLimits{minQuality: 0.7})
	if !slices.Equal(g.failures, []string{"median quality 0.60 of the revise runs is below --min-quality 0.70"}) {
		t.Fatalf("failures = %q", g.failures)
	}
	if g := evaluateGates(aggregate(results[5:6]), gateLimits{minQuality: 0.1}); g.ok() {
		t.Fatal("a run with no scored dashboard cannot pass a quality gate")
	}

	markdown := renderMarkdown(Report{Aggregates: agg, Results: results}, g)
	for _, line := range []string{"| Kind | Passed | Rate | Median quality |", "| revise | 0 of 2 | 0% | 0.60 |", "| suggest | 0 of 1 | 0% | n/a |", "| Latency | Cost | Quality |", "| n/a | n/a | 0.90 |"} {
		if !strings.Contains(markdown, line) {
			t.Errorf("markdown lacks %q:\n%s", line, markdown)
		}
	}
}

func TestQualityFlagBounds(t *testing.T) {
	for _, value := range []string{"-0.1", "1.5"} {
		if _, err := parseFlags([]string{"--min-quality", value}, &strings.Builder{}); err == nil || !strings.Contains(err.Error(), "--min-quality must be between 0 and 1") {
			t.Errorf("--min-quality %s: %v", value, err)
		}
	}
	o, err := parseFlags([]string{"--min-quality", "0.7"}, &strings.Builder{})
	if err != nil || o.minQuality != 0.7 {
		t.Fatalf("options = %+v, %v", o, err)
	}
}

func TestFinishScoresTheSavedDashboard(t *testing.T) {
	obs := observation{request: okRequest(), dashboard: perfectQualityDashboard(), panels: map[string]panelOutcome{}}
	res := finish(Scenario{ID: "q", Kind: kindCreate}, Pricing{}, Result{}, obs)
	if res.Quality == nil || *res.Quality != 1 || res.QualityFindings != nil {
		t.Fatalf("quality = %v, findings %q", res.Quality, res.QualityFindings)
	}
	if res := finish(Scenario{ID: "q", Kind: kindCreate}, Pricing{}, Result{}, observation{request: okRequest(), specErr: "no version"}); res.Quality != nil {
		t.Fatalf("a run without a dashboard has no quality, got %v", *res.Quality)
	}
}
