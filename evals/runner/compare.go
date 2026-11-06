package main

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	compareFile   = "compare.md"
	compareAllRow = "all models"
)

func loadReport(path string) (Report, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path comes from the operator's flags
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(content, &report); err != nil {
		return Report{}, fmt.Errorf("%s: %w", path, err)
	}
	return report, nil
}

func executeCompare(basePath, newPath string, stdout, stderr io.Writer) int {
	next, err := loadReport(newPath)
	if err != nil {
		return fail(stderr, err)
	}
	if err := compareWith(basePath, next, newPath, stdout); err != nil {
		return fail(stderr, err)
	}
	return 0
}

func compareWith(basePath string, next Report, newPath string, stdout io.Writer) error {
	base, err := loadReport(basePath)
	if err != nil {
		return err
	}
	markdown := renderCompare(base, next, basePath, newPath)
	_, _ = io.WriteString(stdout, markdown)
	path := filepath.Join(filepath.Dir(newPath), compareFile)
	if err := os.WriteFile(path, []byte(markdown), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func renderCompare(base, next Report, basePath, newPath string) string {
	var b strings.Builder
	b.WriteString("# Eval comparison\n\n")
	fmt.Fprintf(&b, "Base `%s` (model `%s`, started %s), new `%s` (model `%s`, started %s).\n\n",
		basePath, base.Model, base.StartedAt.UTC().Format(time.RFC3339),
		newPath, next.Model, next.StartedAt.UTC().Format(time.RFC3339))
	writeModelSummary(&b, base, next)
	writeScenarioDeltas(&b, base, next)
	return b.String()
}

type runSummary struct {
	passed     *Tally
	cost       *float64
	firstPanel *float64
	quality    *float64
}

func summarize(results []Result) runSummary {
	if len(results) == 0 {
		return runSummary{}
	}
	agg := aggregate(results)
	s := runSummary{passed: &agg.Scenarios, cost: agg.Cost.TotalUSD}
	if agg.FirstPanel != nil {
		s.firstPanel = &agg.FirstPanel.P50
	}
	var qualities []float64
	for _, r := range results {
		if !r.Skipped {
			qualities = appendValue(qualities, r.Quality)
		}
	}
	if stats := latencyStats(qualities); stats != nil {
		s.quality = &stats.P50
	}
	return s
}

func passedCell(t *Tally) string {
	if t == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d of %d", t.Passed, t.Total)
}

func passedDelta(base, next *Tally) string {
	if base == nil || next == nil {
		return "n/a"
	}
	return fmt.Sprintf("%+d", next.Passed-base.Passed)
}

func resultModel(r Result, report Report) string {
	if r.Usage != nil && r.Usage.Model != "" {
		return r.Usage.Model
	}
	return report.Model
}

func byModel(report Report) map[string][]Result {
	groups := map[string][]Result{}
	for _, r := range report.Results {
		model := resultModel(r, report)
		groups[model] = append(groups[model], r)
	}
	return groups
}

func writeModelSummary(b *strings.Builder, base, next Report) {
	baseGroups, nextGroups := byModel(base), byModel(next)
	models := slices.Sorted(maps.Keys(baseGroups))
	for model := range nextGroups {
		if !slices.Contains(models, model) {
			models = append(models, model)
		}
	}
	slices.Sort(models)
	b.WriteString("| Model | Passed base | Passed new | Passed delta | Cost base | Cost new | Cost delta | First panel p50 base | First panel p50 new | First panel delta | Median quality base | Median quality new | Quality delta |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, model := range models {
		writeSummaryRow(b, model, summarize(baseGroups[model]), summarize(nextGroups[model]))
	}
	if len(models) > 1 {
		writeSummaryRow(b, compareAllRow, summarize(base.Results), summarize(next.Results))
	}
	b.WriteString("\n")
}

func writeSummaryRow(b *strings.Builder, model string, base, next runSummary) {
	fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
		escapeCell(model), passedCell(base.passed), passedCell(next.passed), passedDelta(base.passed, next.passed),
		usd(base.cost), usd(next.cost), usdDelta(base.cost, next.cost),
		optionalSeconds(base.firstPanel), optionalSeconds(next.firstPanel), secondsDelta(base.firstPanel, next.firstPanel),
		optionalScore(base.quality), optionalScore(next.quality), scoreDelta(base.quality, next.quality))
}

func writeScenarioDeltas(b *strings.Builder, base, next Report) {
	baseByID, nextByID := runsByID(base.Results), runsByID(next.Results)
	b.WriteString("| Scenario | Status | Latency base / new | Latency delta | Cost base / new | Cost delta | Quality base / new | Quality delta |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	var onlyBase, onlyNew []string
	for _, id := range slices.Sorted(maps.Keys(baseByID)) {
		was := baseByID[id]
		now, ok := nextByID[id]
		if !ok {
			onlyBase = append(onlyBase, id)
			continue
		}
		fmt.Fprintf(b, "| %s | %s | %s / %s | %s | %s / %s | %s | %s / %s | %s |\n",
			escapeCell(id), statusChange(was, now),
			optionalSeconds(was.latency), optionalSeconds(now.latency), secondsDelta(was.latency, now.latency),
			usd(was.cost), usd(now.cost), usdDelta(was.cost, now.cost),
			optionalScore(was.quality), optionalScore(now.quality), scoreDelta(was.quality, now.quality))
	}
	for _, id := range slices.Sorted(maps.Keys(nextByID)) {
		if _, ok := baseByID[id]; !ok {
			onlyNew = append(onlyNew, id)
		}
	}
	if len(onlyBase) > 0 {
		fmt.Fprintf(b, "\nOnly in the base report: %s\n", strings.Join(onlyBase, ", "))
	}
	if len(onlyNew) > 0 {
		fmt.Fprintf(b, "\nOnly in the new report: %s\n", strings.Join(onlyNew, ", "))
	}
}

type scenarioRuns struct {
	results                []Result
	passed                 Tally
	latency, cost, quality *float64
}

func runsByID(results []Result) map[string]scenarioRuns {
	grouped := map[string][]Result{}
	for _, r := range results {
		grouped[r.ID] = append(grouped[r.ID], r)
	}
	byID := make(map[string]scenarioRuns, len(grouped))
	for id, runs := range grouped {
		var passed, scored int
		var latencies, costs, qualities []float64
		for _, r := range runs {
			if r.Skipped {
				continue
			}
			scored++
			if r.Passed {
				passed++
			}
			latencies = appendValue(latencies, r.LatencySeconds)
			costs = appendValue(costs, r.CostUSD)
			qualities = appendValue(qualities, r.Quality)
		}
		byID[id] = scenarioRuns{results: runs, passed: tally(passed, scored), latency: median(latencies), cost: mean(costs), quality: median(qualities)}
	}
	return byID
}

func median(values []float64) *float64 {
	stats := latencyStats(values)
	if stats == nil {
		return nil
	}
	return &stats.P50
}

func mean(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	total := 0.0
	for _, v := range values {
		total += v
	}
	m := total / float64(len(values))
	return &m
}

func statusChange(was, now scenarioRuns) string {
	if len(was.results) == 1 && len(now.results) == 1 {
		return change(verdictOf(was.results[0]), verdictOf(now.results[0]))
	}
	return change(percent(was.passed), percent(now.passed))
}

func change(before, after string) string {
	if before == after {
		return after
	}
	return before + " -> " + after
}

func delta(base, next *float64) (float64, bool) {
	if base == nil || next == nil {
		return 0, false
	}
	return *next - *base, true
}

func secondsDelta(base, next *float64) string {
	d, ok := delta(base, next)
	if !ok {
		return "n/a"
	}
	return fmt.Sprintf("%+.1fs", d)
}

func scoreDelta(base, next *float64) string {
	d, ok := delta(base, next)
	if !ok {
		return "n/a"
	}
	return fmt.Sprintf("%+.2f", d)
}

func usdDelta(base, next *float64) string {
	d, ok := delta(base, next)
	if !ok {
		return "n/a"
	}
	sign := "+"
	if d < 0 {
		sign = "-"
	}
	return fmt.Sprintf("%s$%.4f", sign, math.Abs(d))
}
