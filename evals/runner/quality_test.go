package main

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/taste"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var referenceDashboards = []string{"Demo/shop-overview.json", "Demo/node-health.json", "SlashObserve/internals.json"}

func referenceDashboard(t *testing.T, path string) *spec.Dashboard {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "compose", "dashboards", path)) //nolint:gosec // a fixed path inside the repository
	if err != nil {
		t.Fatal(err)
	}
	d, err := spec.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func panelByID(t *testing.T, d *spec.Dashboard, id string) *spec.Panel {
	t.Helper()
	e, ok := d.FindPanel(id)
	if !ok {
		t.Fatalf("no panel %s", id)
	}
	return e.Panel
}

func checkedBy(d *spec.Dashboard, rule string) int {
	for _, j := range taste.Judge(d) {
		if j.Rule == rule {
			return j.Checked
		}
	}
	return 0
}

func TestQualityWeightsSumToOneAndCoverEveryRule(t *testing.T) {
	total := 0.0
	for _, rule := range taste.Rules {
		weight, ok := qualityWeights[rule]
		if !ok {
			t.Errorf("rule %s has no weight", rule)
		}
		total += weight
	}
	if len(qualityWeights) != len(taste.Rules) {
		t.Errorf("weights name %d rules, taste has %d", len(qualityWeights), len(taste.Rules))
	}
	if math.Abs(total-1) > 1e-9 {
		t.Fatalf("rule weights sum to %v", total)
	}
}

func TestQualityOfTheReferenceDashboardsIsOne(t *testing.T) {
	for _, path := range referenceDashboards {
		score, findings := scoreQuality(referenceDashboard(t, path))
		if score != 1 || len(findings) != 0 {
			t.Errorf("%s: quality %v, findings %q", path, score, findings)
		}
	}
}

func TestQualityLosesABrokenRulesWeightTimesItsFailedShare(t *testing.T) {
	tests := []struct {
		rule    string
		failed  int
		spoil   func(t *testing.T, d *spec.Dashboard)
		finding string
	}{
		{taste.Units, 2, func(t *testing.T, d *spec.Dashboard) {
			panelByID(t, d, "p_requests").Unit = ""
			panelByID(t, d, "p_queue_age_by_pod").Unit = ""
		}, "units: p_requests: a stat needs a unit, p_queue_age_by_pod: a timeseries needs a unit"},
		{taste.Legends, 1, func(t *testing.T, d *spec.Dashboard) {
			panelByID(t, d, "p_requests_by_route").Queries[0].Legend = ""
		}, "legends: p_requests_by_route: query A needs a legend"},
		{taste.Headline, 1, func(t *testing.T, d *spec.Dashboard) {
			panelByID(t, d, "p_requests").Type = spec.Gauge
		}, "headline: p_requests: the headline holds only stat panels, not gauge"},
		{taste.RowQuestions, 1, func(t *testing.T, d *spec.Dashboard) {
			panelByID(t, d, "r_orders").Title = "Orders"
		}, `row_questions: r_orders: row title "Orders" does not end with ?`},
		{taste.FullLines, 1, func(t *testing.T, d *spec.Dashboard) {
			panelByID(t, d, "p_cache_hit_ratio").GridPos.W = 6
		}, "full_lines: p_pool_by_pod: line at y 31 in r_database spans 22 of 24 columns"},
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			d := referenceDashboard(t, "Demo/shop-overview.json")
			checked := checkedBy(d, tt.rule)
			tt.spoil(t, d)
			score, findings := scoreQuality(d)
			want := 1 - qualityWeights[tt.rule]*float64(tt.failed)/float64(checked)
			if math.Abs(score-want) > 1e-9 {
				t.Errorf("quality = %v, want %v; findings %q", score, want, findings)
			}
			if !slices.Equal(findings, []string{tt.finding}) {
				t.Errorf("findings = %q, want %q", findings, tt.finding)
			}
		})
	}
}

func TestQualityListsTheFirstFiveFailuresOfARule(t *testing.T) {
	d := referenceDashboard(t, "Demo/node-health.json")
	for _, e := range d.AllPanels() {
		e.Panel.Unit = ""
	}
	_, findings := scoreQuality(d)
	if len(findings) != 1 || !strings.HasPrefix(findings[0], "units: p_cpu_busy: a stat needs a unit, ") || !strings.HasSuffix(findings[0], " and 14 more") {
		t.Fatalf("findings = %q", findings)
	}
}

func TestQualityIgnoresTheRulesTheServerOwns(t *testing.T) {
	d := referenceDashboard(t, "Demo/shop-overview.json")
	panelByID(t, d, "p_requests").ID = "requests"
	panelByID(t, d, "r_logs").GridPos.Y = 40
	if score, findings := scoreQuality(d); score != 1 || len(findings) != 0 {
		t.Fatalf("quality %v, findings %q", score, findings)
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
	obs := observation{request: okRequest(), dashboard: referenceDashboard(t, "Demo/shop-overview.json"), panels: map[string]panelOutcome{}}
	res := finish(Scenario{ID: "q", Kind: kindCreate}, Pricing{}, Result{}, obs)
	if res.Quality == nil || *res.Quality != 1 || res.QualityFindings != nil {
		t.Fatalf("quality = %v, findings %q", res.Quality, res.QualityFindings)
	}
	if res := finish(Scenario{ID: "q", Kind: kindCreate}, Pricing{}, Result{}, observation{request: okRequest(), specErr: "no version"}); res.Quality != nil {
		t.Fatalf("a run without a dashboard has no quality, got %v", *res.Quality)
	}
}
