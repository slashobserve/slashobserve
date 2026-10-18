package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"time"
)

type Result struct {
	ID             string        `json:"id"`
	Prompt         string        `json:"prompt"`
	Category       string        `json:"category"`
	DoneWhen       bool          `json:"done_when"`
	Tags           []string      `json:"tags"`
	Passed         bool          `json:"passed"`
	Checks         []CheckResult `json:"checks"`
	RequestID      string        `json:"request_id,omitempty"`
	DashboardID    string        `json:"dashboard_id,omitempty"`
	Status         string        `json:"status"`
	Error          string        `json:"error,omitempty"`
	LatencySeconds *float64      `json:"latency_seconds"`
	Panels         int           `json:"panels"`
	Usage          *Usage        `json:"usage"`
	CostUSD        *float64      `json:"cost_usd"`
}

type Tally struct {
	Passed int     `json:"passed"`
	Total  int     `json:"total"`
	Rate   float64 `json:"rate"`
}

type LatencyStats struct {
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
	Max float64 `json:"max"`
}

type CostStats struct {
	TotalUSD *float64 `json:"total_usd"`
	MeanUSD  *float64 `json:"mean_usd"`
	Priced   int      `json:"priced_scenarios"`
}

type TokenStats struct {
	Input          int64   `json:"input"`
	Output         int64   `json:"output"`
	CacheRead      int64   `json:"cache_read"`
	CacheWrite     int64   `json:"cache_write"`
	CacheReadShare float64 `json:"cache_read_share"`
}

type Aggregates struct {
	Scenarios  Tally            `json:"scenarios"`
	DoneWhen   Tally            `json:"done_when"`
	Categories map[string]Tally `json:"categories"`
	Latency    *LatencyStats    `json:"latency_seconds"`
	Cost       CostStats        `json:"cost"`
	Tokens     TokenStats       `json:"tokens"`
}

type Report struct {
	StartedAt  time.Time  `json:"started_at"`
	Provider   string     `json:"provider"`
	Model      string     `json:"model"`
	Aggregates Aggregates `json:"aggregates"`
	Results    []Result   `json:"results"`
}

func tally(passed, total int) Tally {
	t := Tally{Passed: passed, Total: total}
	if total > 0 {
		t.Rate = float64(passed) / float64(total)
	}
	return t
}

func aggregate(results []Result) Aggregates {
	agg := Aggregates{Categories: map[string]Tally{}}
	var passed, doneWhenPassed, doneWhenTotal int
	categoryPassed := map[string]int{}
	categoryTotal := map[string]int{}
	var latencies []float64
	var costTotal float64
	for _, r := range results {
		categoryTotal[r.Category]++
		if r.Passed {
			passed++
			categoryPassed[r.Category]++
		}
		if r.DoneWhen {
			doneWhenTotal++
			if r.Passed {
				doneWhenPassed++
			}
		}
		if r.LatencySeconds != nil {
			latencies = append(latencies, *r.LatencySeconds)
		}
		if r.CostUSD != nil {
			costTotal += *r.CostUSD
			agg.Cost.Priced++
		}
		if r.Usage != nil {
			agg.Tokens.Input += r.Usage.InputTokens
			agg.Tokens.Output += r.Usage.OutputTokens
			agg.Tokens.CacheRead += r.Usage.CacheReadTokens
			agg.Tokens.CacheWrite += r.Usage.CacheWriteTokens
		}
	}
	agg.Scenarios = tally(passed, len(results))
	agg.DoneWhen = tally(doneWhenPassed, doneWhenTotal)
	for category, total := range categoryTotal {
		agg.Categories[category] = tally(categoryPassed[category], total)
	}
	agg.Latency = latencyStats(latencies)
	if agg.Cost.Priced > 0 {
		mean := costTotal / float64(agg.Cost.Priced)
		agg.Cost.TotalUSD, agg.Cost.MeanUSD = &costTotal, &mean
	}
	if prompt := agg.Tokens.Input + agg.Tokens.CacheRead + agg.Tokens.CacheWrite; prompt > 0 {
		agg.Tokens.CacheReadShare = float64(agg.Tokens.CacheRead) / float64(prompt)
	}
	return agg
}

func latencyStats(values []float64) *LatencyStats {
	if len(values) == 0 {
		return nil
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return &LatencyStats{P50: nearestRank(sorted, 0.5), P90: nearestRank(sorted, 0.9), Max: sorted[len(sorted)-1]}
}

func nearestRank(sorted []float64, q float64) float64 {
	rank := int(math.Ceil(q*float64(len(sorted)))) - 1
	return sorted[max(0, min(rank, len(sorted)-1))]
}

type gateOutcome struct {
	failures []string
}

func (g gateOutcome) ok() bool { return len(g.failures) == 0 }

func evaluateGates(agg Aggregates, minPass float64, minDoneWhen, doneWhenScenarios int) gateOutcome {
	var out gateOutcome
	if agg.Scenarios.Rate < minPass {
		out.failures = append(out.failures, fmt.Sprintf("pass rate %.0f%% is below --min-pass %.0f%%", agg.Scenarios.Rate*100, minPass*100))
	}
	if doneWhenScenarios > 0 && agg.DoneWhen.Total == doneWhenScenarios && agg.DoneWhen.Passed < minDoneWhen {
		out.failures = append(out.failures, fmt.Sprintf("%d of %d done-when scenarios passed, below --min-done-when %d", agg.DoneWhen.Passed, agg.DoneWhen.Total, minDoneWhen))
	}
	return out
}

func writeJSON(path string, report Report) error {
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o600)
}

func percent(t Tally) string {
	return fmt.Sprintf("%d of %d (%.0f%%)", t.Passed, t.Total, t.Rate*100)
}

func usd(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("$%.4f", *v)
}

func pricedNote(agg Aggregates) string {
	if agg.Cost.Priced == 0 || agg.Cost.Priced == agg.Scenarios.Total {
		return ""
	}
	return fmt.Sprintf(" (priced %d of %d scenarios)", agg.Cost.Priced, agg.Scenarios.Total)
}

func escapeCell(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " ", "|", "\\|").Replace(s)
	return strings.TrimSpace(s)
}

func renderMarkdown(report Report, gates gateOutcome) string {
	var b strings.Builder
	agg := report.Aggregates
	fmt.Fprintf(&b, "# Eval report\n\n")
	fmt.Fprintf(&b, "Provider `%s`, model `%s`, started %s.\n\n", report.Provider, report.Model, report.StartedAt.UTC().Format(time.RFC3339))

	b.WriteString("| Summary | Value |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| Scenarios passed | %s |\n", percent(agg.Scenarios))
	fmt.Fprintf(&b, "| Done-when passed | %s |\n", percent(agg.DoneWhen))
	if agg.Latency != nil {
		fmt.Fprintf(&b, "| Latency p50 / p90 / max | %.1fs / %.1fs / %.1fs |\n", agg.Latency.P50, agg.Latency.P90, agg.Latency.Max)
	}
	fmt.Fprintf(&b, "| Cost total / mean | %s / %s%s |\n", usd(agg.Cost.TotalUSD), usd(agg.Cost.MeanUSD), pricedNote(agg))
	fmt.Fprintf(&b, "| Tokens in / out / cache read / cache write | %d / %d / %d / %d |\n", agg.Tokens.Input, agg.Tokens.Output, agg.Tokens.CacheRead, agg.Tokens.CacheWrite)
	fmt.Fprintf(&b, "| Cache-read share of prompt tokens | %.0f%% |\n\n", agg.Tokens.CacheReadShare*100)

	b.WriteString("| Category | Passed | Rate |\n| --- | --- | --- |\n")
	categories := make([]string, 0, len(agg.Categories))
	for category := range agg.Categories {
		categories = append(categories, category)
	}
	slices.Sort(categories)
	for _, category := range categories {
		t := agg.Categories[category]
		fmt.Fprintf(&b, "| %s | %d of %d | %.0f%% |\n", category, t.Passed, t.Total, t.Rate*100)
	}

	b.WriteString("\n| Scenario | Result | Category | Latency | Cost | Failed checks | Agent error |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	rows := slices.Clone(report.Results)
	slices.SortStableFunc(rows, func(a, c Result) int { return cmp.Compare(a.ID, c.ID) })
	for _, r := range rows {
		verdict, seconds := "PASS", "n/a"
		if !r.Passed {
			verdict = "FAIL"
		}
		if r.LatencySeconds != nil {
			seconds = fmt.Sprintf("%.1fs", *r.LatencySeconds)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n", r.ID, verdict, r.Category, seconds, usd(r.CostUSD), escapeCell(failedChecks(r)), escapeCell(r.Error))
	}

	if !gates.ok() {
		b.WriteString("\nGates failed:\n\n")
		for _, f := range gates.failures {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	return b.String()
}

func failedChecks(r Result) string {
	var parts []string
	for _, c := range r.Checks {
		if c.Passed {
			continue
		}
		if c.Detail == "" {
			parts = append(parts, c.Name)
			continue
		}
		parts = append(parts, c.Name+": "+c.Detail)
	}
	return strings.Join(parts, "; ")
}
