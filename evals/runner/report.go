package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
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
	Kind           string        `json:"kind"`
	Suite          string        `json:"suite,omitempty"`
	Skipped        bool          `json:"skipped,omitempty"`
	Reason         string        `json:"reason,omitempty"`
	Passed         bool          `json:"passed"`
	Checks         []CheckResult `json:"checks"`
	RequestID      string        `json:"request_id,omitempty"`
	DashboardID    string        `json:"dashboard_id,omitempty"`
	Status         string        `json:"status"`
	Error          string        `json:"error,omitempty"`
	FailureCode    string        `json:"failure_code,omitempty"`
	Uncoded        bool          `json:"uncoded,omitempty"`
	RunnerCode     string        `json:"runner_code,omitempty"`
	LatencyOnly    bool          `json:"latency_only,omitempty"`
	StartedAt      time.Time     `json:"started_at"`
	FinishedAt     time.Time     `json:"finished_at"`
	ModelCalls     *int          `json:"model_calls"`
	LatencySeconds *float64      `json:"latency_seconds"`
	Panels         int           `json:"panels"`
	Usage          *Usage        `json:"usage"`
	CostUSD        *float64      `json:"cost_usd"`

	FirstPanelSeconds   *float64 `json:"first_panel_seconds"`
	PanelsPerMinute     *float64 `json:"panels_per_minute"`
	CallsPerPanel       *float64 `json:"calls_per_panel"`
	InputTokensPerCall  *float64 `json:"input_tokens_per_call"`
	OutputTokensPerCall *float64 `json:"output_tokens_per_call"`
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

type FirstPanelStats struct {
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
	Max float64 `json:"max"`
	N   int     `json:"n"`
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
	Scenarios    Tally          `json:"scenarios"`
	DoneWhen     Tally          `json:"done_when"`
	Categories   tallies        `json:"categories"`
	Kinds        tallies        `json:"kinds"`
	SizeGuard    Tally          `json:"size_guard"`
	Skipped      int            `json:"skipped"`
	FailureCodes map[string]int `json:"failure_codes"`
	Uncoded      int            `json:"uncoded_failures"`
	RunnerCodes  map[string]int `json:"runner_codes"`
	LatencyOnly  int            `json:"latency_only_failures"`
	Latency      *LatencyStats  `json:"latency_seconds"`
	Cost         CostStats      `json:"cost"`
	Tokens       TokenStats     `json:"tokens"`

	FirstPanel         *FirstPanelStats `json:"first_panel"`
	PanelsPerMinuteP50 *float64         `json:"panels_per_minute_p50"`
	BuildSaves         int              `json:"build_saves"`
	Renewals           int              `json:"renewals"`
	SaveNowRefusals    int              `json:"save_now_refusals"`
	StalledTurns       int              `json:"stalled_turns"`
	OnePanelRefusals   int              `json:"one_panel_refusals"`
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

type tallies map[string]Tally

func (g tallies) add(key string, passed bool) {
	if key == "" {
		return
	}
	t := g[key]
	t.Total++
	if passed {
		t.Passed++
	}
	g[key] = t
}

func (g tallies) rate() {
	for key, t := range g {
		g[key] = tally(t.Passed, t.Total)
	}
}

func splitSizeGuard(results []Result) ([]Result, Tally) {
	var rest []Result
	var passed, total int
	for _, r := range results {
		switch {
		case r.Kind != kindSizeGuard || r.Skipped:
			rest = append(rest, r)
		case r.Passed:
			passed++
			total++
		default:
			total++
		}
	}
	return rest, tally(passed, total)
}

func aggregate(results []Result) Aggregates {
	agg := Aggregates{Categories: tallies{}, Kinds: tallies{}, FailureCodes: map[string]int{}, RunnerCodes: map[string]int{}}
	results, agg.SizeGuard = splitSizeGuard(results)
	var passed, doneWhenPassed, doneWhenTotal int
	var latencies, firstPanels, panelRates []float64
	var costTotal float64
	for _, r := range results {
		if r.Skipped {
			agg.Skipped++
			continue
		}
		agg.Kinds.add(cmp.Or(r.Kind, kindCreate), r.Passed)
		agg.Categories.add(r.Category, r.Passed)
		if r.Passed {
			passed++
		}
		if r.DoneWhen {
			doneWhenTotal++
			if r.Passed {
				doneWhenPassed++
			}
		}
		if r.FailureCode != "" {
			agg.FailureCodes[r.FailureCode]++
		}
		if r.Uncoded {
			agg.Uncoded++
		}
		if r.RunnerCode != "" {
			agg.RunnerCodes[r.RunnerCode]++
		}
		if r.LatencyOnly {
			agg.LatencyOnly++
		}
		latencies = appendValue(latencies, r.LatencySeconds)
		firstPanels = appendValue(firstPanels, r.FirstPanelSeconds)
		panelRates = appendValue(panelRates, r.PanelsPerMinute)
		if r.CostUSD != nil {
			costTotal += *r.CostUSD
			agg.Cost.Priced++
		}
		agg.addUsage(r.Usage)
	}
	agg.Scenarios = tally(passed, len(results)-agg.Skipped)
	agg.DoneWhen = tally(doneWhenPassed, doneWhenTotal)
	agg.Categories.rate()
	agg.Kinds.rate()
	agg.Latency = latencyStats(latencies)
	agg.FirstPanel = firstPanelStats(firstPanels)
	if len(panelRates) > 0 {
		slices.Sort(panelRates)
		p50 := nearestRank(panelRates, 0.5)
		agg.PanelsPerMinuteP50 = &p50
	}
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

func appendValue(values []float64, v *float64) []float64 {
	if v == nil {
		return values
	}
	return append(values, *v)
}

func (agg *Aggregates) addUsage(u *Usage) {
	if u == nil {
		return
	}
	agg.BuildSaves += u.BuildSaves
	agg.Renewals += u.Renewals
	agg.SaveNowRefusals += u.SaveNowRefusals
	agg.StalledTurns += u.StalledTurns
	agg.OnePanelRefusals += u.OnePanelRefusals
	agg.Tokens.Input += u.InputTokens
	agg.Tokens.Output += u.OutputTokens
	agg.Tokens.CacheRead += u.CacheReadTokens
	agg.Tokens.CacheWrite += u.CacheWriteTokens
}

func firstPanelStats(values []float64) *FirstPanelStats {
	stats := latencyStats(values)
	if stats == nil {
		return nil
	}
	return &FirstPanelStats{P50: stats.P50, P90: stats.P90, Max: stats.Max, N: len(values)}
}

var panelSaveTools = []string{"create_dashboard", "update_dashboard", "update_panels"}

func isPanelSave(s Step) bool {
	return s.Status == "ok" && (s.Phase == "save" || s.Phase == "repair") && slices.Contains(panelSaveTools, s.Tool)
}

func earliestSave(steps []Step, seen *time.Time) *time.Time {
	first := seen
	for _, s := range steps {
		if isPanelSave(s) && (first == nil || s.At.Before(*first)) {
			at := s.At
			first = &at
		}
	}
	return first
}

func firstPanelSeconds(created time.Time, firstSave *time.Time) *float64 {
	if firstSave == nil || created.IsZero() {
		return nil
	}
	seconds := firstSave.Sub(created).Seconds()
	return &seconds
}

func panelsPerMinute(req AgentRequest, panels int) *float64 {
	if req.FirstSaveAt == nil || req.FinishedAt == nil || panels < 2 {
		return nil
	}
	minutes := req.FinishedAt.Sub(*req.FirstSaveAt).Minutes()
	if minutes <= 0 {
		return nil
	}
	rate := float64(panels-1) / minutes
	return &rate
}

func ratio(total int64, count int) *float64 {
	if count <= 0 {
		return nil
	}
	v := float64(total) / float64(count)
	return &v
}

func withMetrics(res Result, kind string, req AgentRequest) Result {
	switch kind {
	case kindSuggest, kindChatImport, kindSizeGuard:
	default:
		res.FirstPanelSeconds = firstPanelSeconds(req.CreatedAt, req.FirstSaveAt)
	}
	if kind == kindCreate {
		res.PanelsPerMinute = panelsPerMinute(req, res.Panels)
		if u := req.Usage; u != nil && u.ModelCalls > 0 {
			res.CallsPerPanel = ratio(int64(u.ModelCalls), res.Panels)
		}
	}
	if u := req.Usage; u != nil {
		res.InputTokensPerCall = ratio(u.InputTokens+u.CacheReadTokens+u.CacheWriteTokens, u.ModelCalls)
		res.OutputTokensPerCall = ratio(u.OutputTokens, u.ModelCalls)
	}
	return res
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
	if agg.Uncoded > 0 {
		out.failures = append(out.failures, fmt.Sprintf("%d failed runs have no failure code", agg.Uncoded))
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

	b.WriteString(firstPanelLine(agg.FirstPanel) + "\n\n")
	b.WriteString("| Summary | Value |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| Scenarios passed | %s |\n", percent(agg.Scenarios))
	if agg.Skipped > 0 {
		fmt.Fprintf(&b, "| Skipped, left out of the rates | %d |\n", agg.Skipped)
	}
	if agg.LatencyOnly > 0 {
		fmt.Fprintf(&b, "| Failed on latency or budget only, included in the failures | %d |\n", agg.LatencyOnly)
	}
	fmt.Fprintf(&b, "| Done-when passed | %s |\n", percent(agg.DoneWhen))
	if agg.Latency != nil {
		fmt.Fprintf(&b, "| Latency p50 / p90 / max | %.1fs / %.1fs / %.1fs |\n", agg.Latency.P50, agg.Latency.P90, agg.Latency.Max)
	}
	fmt.Fprintf(&b, "| Cost total / mean | %s / %s%s |\n", usd(agg.Cost.TotalUSD), usd(agg.Cost.MeanUSD), pricedNote(agg))
	fmt.Fprintf(&b, "| Tokens in / out / cache read / cache write | %d / %d / %d / %d |\n", agg.Tokens.Input, agg.Tokens.Output, agg.Tokens.CacheRead, agg.Tokens.CacheWrite)
	fmt.Fprintf(&b, "| Cache-read share of prompt tokens | %.0f%% |\n\n", agg.Tokens.CacheReadShare*100)
	if agg.SizeGuard.Total > 0 {
		fmt.Fprintf(&b, "Size guard: %d of %d rejected as expected\n\n", agg.SizeGuard.Passed, agg.SizeGuard.Total)
	}

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

	b.WriteString("\n| Kind | Passed | Rate |\n| --- | --- | --- |\n")
	for _, kind := range slices.Sorted(maps.Keys(agg.Kinds)) {
		t := agg.Kinds[kind]
		fmt.Fprintf(&b, "| %s | %d of %d | %.0f%% |\n", kind, t.Passed, t.Total, t.Rate*100)
	}

	b.WriteString("\n| Failure code | Runs |\n| --- | --- |\n")
	codes := make([]string, 0, len(agg.FailureCodes))
	for code := range agg.FailureCodes {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	for _, code := range codes {
		fmt.Fprintf(&b, "| %s | %d |\n", code, agg.FailureCodes[code])
	}
	if agg.Uncoded > 0 {
		fmt.Fprintf(&b, "| uncoded | %d |\n", agg.Uncoded)
	}

	writeRunnerOutcomes(&b, agg)

	b.WriteString("\n| Scenario | First panel | Panels/min | Saves | Refusals | Result | Category | Latency | Cost | Failure code | Failed checks | Agent error |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	rows := slices.Clone(report.Results)
	slices.SortStableFunc(rows, func(a, c Result) int { return cmp.Compare(a.ID, c.ID) })
	for _, r := range rows {
		seconds := "n/a"
		if r.LatencySeconds != nil {
			seconds = fmt.Sprintf("%.1fs", *r.LatencySeconds)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", r.ID, optionalSeconds(r.FirstPanelSeconds), optionalFloat(r.PanelsPerMinute), savesCell(r), refusalsCell(r), verdictOf(r), r.Category, seconds, usd(r.CostUSD), failureLabel(r), escapeCell(failedChecks(r)), escapeCell(cmp.Or(r.Error, r.Reason)))
	}

	if !gates.ok() {
		b.WriteString("\nGates failed:\n\n")
		for _, f := range gates.failures {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	return b.String()
}

func firstPanelLine(stats *FirstPanelStats) string {
	if stats == nil {
		return "Time to first panel: no runs"
	}
	return fmt.Sprintf("Time to first panel: p50 %.1fs, p90 %.1fs, max %.1fs (%d runs)", stats.P50, stats.P90, stats.Max, stats.N)
}

func optionalSeconds(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1fs", *v)
}

func optionalFloat(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1f", *v)
}

func savesCell(r Result) string {
	if r.Usage == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d", r.Usage.BuildSaves)
}

func refusalsCell(r Result) string {
	if r.Usage == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d", r.Usage.SaveNowRefusals+r.Usage.OnePanelRefusals)
}

func writeRunnerOutcomes(b *strings.Builder, agg Aggregates) {
	if len(agg.RunnerCodes) == 0 {
		return
	}
	b.WriteString("\n| Runner outcome, not a server failure code | Runs |\n| --- | --- |\n")
	for _, code := range slices.Sorted(maps.Keys(agg.RunnerCodes)) {
		fmt.Fprintf(b, "| %s | %d |\n", code, agg.RunnerCodes[code])
	}
}

func verdictOf(r Result) string {
	switch {
	case r.Skipped:
		return "SKIP"
	case r.LatencyOnly:
		return "FAIL (latency only)"
	case !r.Passed:
		return "FAIL"
	}
	return "PASS"
}

func failureLabel(r Result) string {
	switch {
	case r.FailureCode != "":
		return r.FailureCode
	case r.Uncoded:
		return "uncoded"
	case r.RunnerCode != "":
		return r.RunnerCode
	}
	return ""
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
