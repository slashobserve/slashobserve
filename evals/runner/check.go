package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	shareEpsilon = 1e-9

	checkNameLatency = "latency"
	checkNameBudgets = "budgets"
	checkNameUsage   = "usage"
)

var usageCounters = map[string]func(*Usage) int{
	"build_saves":        func(u *Usage) int { return u.BuildSaves },
	"renewals":           func(u *Usage) int { return u.Renewals },
	"compactions":        func(u *Usage) int { return u.Compactions },
	"nudges":             func(u *Usage) int { return u.Nudges },
	"retries":            func(u *Usage) int { return u.Retries },
	"save_now_refusals":  func(u *Usage) int { return u.SaveNowRefusals },
	"stalled_turns":      func(u *Usage) int { return u.StalledTurns },
	"one_panel_refusals": func(u *Usage) int { return u.OnePanelRefusals },
}

func latencyOnly(checks []CheckResult) bool {
	failed := 0
	for _, c := range checks {
		if c.Passed {
			continue
		}
		if c.Name != checkNameLatency && c.Name != checkNameBudgets {
			return false
		}
		failed++
	}
	return failed > 0
}

type CheckResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type panelOutcome struct {
	data   PanelData
	labels []map[string]string
	err    error
}

type observation struct {
	request   AgentRequest
	specErr   string
	dashboard *spec.Dashboard
	panels    map[string]panelOutcome

	datasourceTypes map[string]string

	parent     *spec.Dashboard
	target     string
	unverified Unverified

	suggestion   *Suggestion
	versionAdded bool

	importRecord *ImportRecord
	importErr    string

	coverage *coverageMeasure
}

func (o observation) failureCode() string {
	if code := o.request.FailureCode; code != nil {
		return *code
	}
	return ""
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
	for j, q := range p.Queries {
		if q.IsExpression() || q.IsDashboardSource() {
			continue
		}
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

func kindOf(sc Scenario) string {
	if sc.Kind == "" {
		return kindCreate
	}
	return sc.Kind
}

func hasBudgets(ex Expect) bool {
	return ex.MaxModelCalls > 0 || ex.MaxOutputTokens > 0 || ex.MaxSaveOutputTokens > 0
}

func runChecks(sc Scenario, obs observation) []CheckResult {
	checks := scenarioChecks(sc, obs)
	if len(sc.Expect.Usage) > 0 {
		checks = append(checks, checkUsage(sc.Expect, obs))
	}
	return checks
}

func checkUsage(ex Expect, obs observation) CheckResult {
	u := obs.request.Usage
	if u == nil {
		return result(checkNameUsage, false, "the request reports no usage")
	}
	var wrong []string
	for _, key := range slices.Sorted(maps.Keys(ex.Usage)) {
		if got := usageCounters[key](u); got != ex.Usage[key] {
			wrong = append(wrong, fmt.Sprintf("%s %d, expected %d", key, got, ex.Usage[key]))
		}
	}
	return result(checkNameUsage, len(wrong) == 0, strings.Join(wrong, "; "))
}

func scenarioChecks(sc Scenario, obs observation) []CheckResult {
	ex := sc.Expect
	if ex.FailureCode != "" {
		return []CheckResult{checkFailureCode(ex, obs)}
	}
	var checks []CheckResult
	switch kindOf(sc) {
	case kindFixUnverified:
		checks = []CheckResult{checkFailureCode(ex, obs), checkOnlyTargetChanged(obs), checkTargetData(obs), checkUnverifiedCleared(obs), checkLatency(ex, obs)}
	case kindRevise:
		checks = reviseChecks(ex, obs)
	case kindSuggest:
		checks = []CheckResult{checkFailureCode(ex, obs), checkProposal(obs), checkNoVersionAdded(obs), checkLatency(ex, obs)}
	case kindChatImport:
		checks = []CheckResult{checkFailureCode(ex, obs), checkImported(obs), checkLatency(ex, obs)}
	case kindInvestigate:
		checks = []CheckResult{checkFailureCode(ex, obs), checkFindingMentions(ex, obs), checkEvidence(ex, obs), checkLatency(ex, obs)}
	case kindQuestion:
		checks = questionChecks(ex, obs)
	default:
		checks = createChecks(ex, obs)
	}
	if hasBudgets(ex) {
		checks = append(checks, checkBudgets(ex, obs))
	}
	return checks
}

func questionChecks(ex Expect, obs observation) []CheckResult {
	checks := []CheckResult{checkFailureCode(ex, obs), checkAnswered(obs)}
	if len(ex.FindingMentions) > 0 {
		checks = append(checks, checkFindingMentions(ex, obs))
	}
	if ex.EvidenceMin > 0 {
		checks = append(checks, checkEvidence(ex, obs))
	}
	return append(checks, checkLatency(ex, obs))
}

func checkAnswered(obs observation) CheckResult {
	const name = "answered"
	req := obs.request
	switch {
	case req.Status != "succeeded":
		return result(name, false, "request "+req.Status)
	case req.ResultVersion != nil || req.DashboardID != nil:
		return result(name, false, "the run built a dashboard instead of answering")
	case req.Finding == nil:
		return result(name, false, "the request has no finding")
	}
	return result(name, true, "")
}

func reviseChecks(ex Expect, obs observation) []CheckResult {
	checks := []CheckResult{checkFailureCode(ex, obs), checkKeepsPanels(obs)}
	if len(ex.ChangedPaths) == 0 && !ex.OnlyTargetChanged && ex.ChangedPanels == 0 {
		checks = append(checks, checkNewPanelData(obs))
	}
	checks = append(checks, checkPanelValues(ex, obs))
	if len(ex.ChangedPaths) > 0 {
		checks = append(checks, checkChangedPaths(ex, obs))
	}
	if len(ex.TimeRangeFrom) > 0 {
		checks = append(checks, checkTimeRange(ex, obs))
	}
	if ex.OnlyTargetChanged {
		checks = append(checks, checkOnlyTargetChanged(obs))
	}
	if ex.TargetReturnsData {
		checks = append(checks, checkTargetData(obs))
	}
	if ex.TargetType != "" {
		checks = append(checks, checkTargetType(ex, obs))
	}
	if ex.ChangedPanels > 0 {
		checks = append(checks, checkChangedPanels(ex, obs))
	}
	return append(checks, checkLatency(ex, obs))
}

func createChecks(ex Expect, obs observation) []CheckResult {
	checks := []CheckResult{checkValidity(obs), checkData(obs), checkSource(ex, obs)}
	if len(ex.Metrics) > 0 {
		checks = append(checks, checkQueries("metrics", ex.Metrics, spec.Prometheus, ex.RebuildMetricShare, ex.RebuildMetricCap, obs))
	}
	if len(ex.Tables) > 0 {
		checks = append(checks, checkQueries("tables", ex.Tables, spec.ClickHouse, ex.RebuildMetricShare, ex.RebuildMetricCap, obs))
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
	if ex.Scope != "" {
		checks = append(checks, checkCoverage(ex, obs))
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

func checkQueries(name string, needles []string, kind spec.DatasourceType, share float64, limit int, obs observation) CheckResult {
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
	found, need := len(needles)-len(missing), requiredCount(len(needles), share, limit)
	detail := "no " + string(kind) + " panel query contains " + strings.Join(missing, ", ")
	if share > 0 {
		detail = fmt.Sprintf("%d of %d found, need %d; %s", found, len(needles), need, detail)
	}
	return result(name, found >= need, detail)
}

func requiredCount(total int, share float64, limit int) int {
	if share <= 0 {
		return total
	}
	need := min(total, int(math.Ceil(share*float64(total)-shareEpsilon)))
	if limit > 0 {
		need = min(need, limit)
	}
	return need
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
	const name = checkNameLatency
	elapsed, ok := latency(obs.request)
	if !ok {
		return result(name, false, "the request did not finish")
	}
	limit := time.Duration(ex.MaxSeconds) * time.Second
	return result(name, elapsed <= limit, fmt.Sprintf("took %.1fs, limit %ds", elapsed.Seconds(), ex.MaxSeconds))
}

func latency(req AgentRequest) (time.Duration, bool) {
	if req.FinishedAt == nil || req.CreatedAt.IsZero() {
		return 0, false
	}
	return req.FinishedAt.Sub(req.CreatedAt), true
}

func checkFindingMentions(ex Expect, obs observation) CheckResult {
	const name = "finding_mentions"
	f := obs.request.Finding
	if f == nil {
		return result(name, false, "the request has no finding")
	}
	text := strings.ToLower(f.Markdown)
	var missing []string
	for _, want := range ex.FindingMentions {
		if !strings.Contains(text, strings.ToLower(want)) {
			missing = append(missing, strconv.Quote(want))
		}
	}
	return result(name, len(missing) == 0, "the finding does not mention "+strings.Join(missing, ", "))
}

func checkEvidence(ex Expect, obs observation) CheckResult {
	const name = "evidence"
	got := 0
	if f := obs.request.Finding; f != nil {
		got = len(f.Evidence)
	}
	return result(name, obs.request.Finding != nil && got >= ex.EvidenceMin, fmt.Sprintf("the finding has %d evidence panels, expected at least %d", got, ex.EvidenceMin))
}

func checkFailureCode(ex Expect, obs observation) CheckResult {
	got := obs.failureCode()
	if ex.FailureCode != "" {
		return result("failure_code", got == ex.FailureCode, fmt.Sprintf("request %s with failure code %q, expected %q", obs.request.Status, got, ex.FailureCode))
	}
	detail := "request " + obs.request.Status
	if got != "" {
		detail += " with failure code " + got
	}
	return result("failure_code", obs.request.Status == "succeeded", detail)
}

func panelIDs(d *spec.Dashboard) map[string]bool {
	ids := map[string]bool{}
	for _, e := range d.AllPanels() {
		ids[e.Panel.ID] = true
	}
	return ids
}

const maxListedChanges = 5

func checkOnlyTargetChanged(obs observation) CheckResult {
	const name = "only_target_changed"
	if c, skip := noSpec(name, obs); skip {
		return c
	}
	if obs.parent == nil || obs.target == "" {
		return result(name, false, "the seeded parent or target is unknown")
	}
	var outside []string
	for _, change := range spec.Diff(obs.parent, obs.dashboard) {
		if id, ok := panelOfPath(change.Path); !ok || id != obs.target {
			outside = append(outside, string(change.Op)+" "+change.Path)
		}
	}
	if len(outside) > maxListedChanges {
		outside = append(outside[:maxListedChanges], fmt.Sprintf("and %d more", len(outside)-maxListedChanges))
	}
	return result(name, len(outside) == 0, "changes outside panel "+obs.target+": "+strings.Join(outside, ", "))
}

func underAny(path string, roots []string) bool {
	return slices.ContainsFunc(roots, func(root string) bool {
		return path == root || strings.HasPrefix(path, root+"/")
	})
}

func checkChangedPaths(ex Expect, obs observation) CheckResult {
	const name = "changed_paths"
	if c, skip := noSpec(name, obs); skip {
		return c
	}
	if obs.parent == nil {
		return result(name, false, "the seeded parent is unknown")
	}
	var outside []string
	for _, change := range spec.Diff(obs.parent, obs.dashboard) {
		if !underAny(change.Path, ex.ChangedPaths) {
			outside = append(outside, string(change.Op)+" "+change.Path)
		}
	}
	if len(outside) > maxListedChanges {
		outside = append(outside[:maxListedChanges], fmt.Sprintf("and %d more", len(outside)-maxListedChanges))
	}
	return result(name, len(outside) == 0, fmt.Sprintf("changes outside %v: %s", ex.ChangedPaths, strings.Join(outside, ", ")))
}

func checkTargetType(ex Expect, obs observation) CheckResult {
	const name = "target_type"
	if c, skip := noSpec(name, obs); skip {
		return c
	}
	entry, ok := obs.dashboard.FindPanel(obs.target)
	if !ok {
		return result(name, false, "panel "+obs.target+" is gone")
	}
	return result(name, string(entry.Panel.Type) == ex.TargetType, fmt.Sprintf("panel %s is a %s, expected %s", obs.target, entry.Panel.Type, ex.TargetType))
}

func checkChangedPanels(ex Expect, obs observation) CheckResult {
	const name = "changed_panels"
	if c, skip := noSpec(name, obs); skip {
		return c
	}
	if obs.parent == nil {
		return result(name, false, "the seeded parent is unknown")
	}
	changed := map[string]bool{}
	for _, change := range spec.Diff(obs.parent, obs.dashboard) {
		if id, ok := panelOfPath(change.Path); ok {
			changed[id] = true
		}
	}
	ids := slices.Sorted(maps.Keys(changed))
	if len(ids) > maxListedChanges {
		ids = append(ids[:maxListedChanges], fmt.Sprintf("and %d more", len(ids)-maxListedChanges))
	}
	return result(name, len(changed) == ex.ChangedPanels, fmt.Sprintf("%d panels changed (%s), expected %d", len(changed), strings.Join(ids, ", "), ex.ChangedPanels))
}

func panelDataProblem(id string, obs observation) string {
	outcome, ok := obs.panels[id]
	switch {
	case !ok:
		return "not fetched"
	case outcome.err != nil:
		return outcome.err.Error()
	case !hasValue(outcome.data):
		return "no data"
	}
	return ""
}

func checkTargetData(obs observation) CheckResult {
	const name = "target_data"
	if c, skip := noSpec(name, obs); skip {
		return c
	}
	if _, ok := obs.dashboard.FindDataPanel(obs.target); !ok {
		return result(name, false, "panel "+obs.target+" is no longer a data panel")
	}
	problem := panelDataProblem(obs.target, obs)
	return result(name, problem == "", obs.target+": "+problem)
}

func checkUnverifiedCleared(obs observation) CheckResult {
	const name = "unverified_cleared"
	if c, skip := noSpec(name, obs); skip {
		return c
	}
	return result(name, !obs.unverified.hasPanel(obs.target), "panel "+obs.target+" is still unverified")
}

func checkKeepsPanels(obs observation) CheckResult {
	const name = "keeps_panels"
	if c, skip := noSpec(name, obs); skip {
		return c
	}
	if obs.parent == nil {
		return result(name, false, "the seeded parent is unknown")
	}
	kept := panelIDs(obs.dashboard)
	var removed []string
	for id := range panelIDs(obs.parent) {
		if !kept[id] {
			removed = append(removed, id)
		}
	}
	slices.Sort(removed)
	return result(name, len(removed) == 0, "removed panels "+strings.Join(removed, ", "))
}

func newDataPanels(obs observation) []*spec.Panel {
	known := map[string]bool{}
	if obs.parent != nil {
		known = panelIDs(obs.parent)
	}
	return slices.DeleteFunc(dataPanels(obs.dashboard), func(p *spec.Panel) bool { return known[p.ID] })
}

func checkNewPanelData(obs observation) CheckResult {
	const name = "new_panel_data"
	if c, skip := noSpec(name, obs); skip {
		return c
	}
	added := newDataPanels(obs)
	if len(added) == 0 {
		return result(name, false, "no data panel was added")
	}
	var failures []string
	for _, p := range added {
		if problem := panelDataProblem(p.ID, obs); problem != "" {
			failures = append(failures, p.ID+": "+problem)
		}
	}
	return result(name, len(failures) == 0, strings.Join(failures, "; "))
}

func checkPanelValues(ex Expect, obs observation) CheckResult {
	const name = "panel_values"
	if len(ex.PanelValues) == 0 {
		return result(name, true, "")
	}
	if c, skip := noSpec(name, obs); skip {
		return c
	}
	var failures []string
	for _, pv := range ex.PanelValues {
		if problem := panelValueProblem(pv, obs); problem != "" {
			failures = append(failures, problem)
		}
	}
	return result(name, len(failures) == 0, strings.Join(failures, "; "))
}

func panelValueProblem(pv PanelValue, obs observation) string {
	var tried []string
	for _, panel := range dataPanels(obs.dashboard) {
		if !strings.Contains(obs.searchableQuery(panel), pv.QueryContains) {
			continue
		}
		problem := seriesProblem(pv, obs.panels[panel.ID])
		if problem == "" {
			return ""
		}
		tried = append(tried, fmt.Sprintf("panel %s: %s", panel.ID, problem))
	}
	if len(tried) == 0 {
		return fmt.Sprintf("no panel query contains %q", pv.QueryContains)
	}
	return strings.Join(tried, ", ")
}

func seriesProblem(pv PanelValue, outcome panelOutcome) string {
	if outcome.err != nil {
		return outcome.err.Error()
	}
	var problems []string
	matched := 0
	for i, frame := range outcome.data.Frames {
		if i >= len(outcome.labels) || !hasLabels(outcome.labels[i], pv.Series) {
			continue
		}
		matched++
		v, ok := lastValue(frame)
		switch {
		case !ok:
			problems = append(problems, "a matching series has no value")
		case v < pv.Min || v > pv.Max:
			problems = append(problems, fmt.Sprintf("value %g is outside %g to %g", v, pv.Min, pv.Max))
		}
	}
	if matched == 0 {
		return fmt.Sprintf("no series matches %v", pv.Series)
	}
	return strings.Join(problems, ", ")
}

func hasLabels(have, want map[string]string) bool {
	for key, value := range want {
		if got, ok := have[key]; !ok || got != value {
			return false
		}
	}
	return true
}

func lastValue(frame Frame) (float64, bool) {
	for _, field := range frame.Fields {
		if field.Type == "time" {
			continue
		}
		for i := len(field.Values) - 1; i >= 0; i-- {
			if v, ok := field.Values[i].(float64); ok {
				return v, true
			}
		}
		return 0, false
	}
	return 0, false
}

func checkProposal(obs observation) CheckResult {
	const name = "proposal"
	if obs.suggestion == nil {
		return result(name, false, "the suggestion did not succeed")
	}
	var panel struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(obs.suggestion.Panel, &panel); err != nil {
		return result(name, false, "the proposal is not a panel object")
	}
	return result(name, panel.ID == obs.target, fmt.Sprintf("the proposal has panel id %q, expected %q", panel.ID, obs.target))
}

func checkNoVersionAdded(obs observation) CheckResult {
	if obs.specErr != "" {
		return result("no_version_added", false, obs.specErr)
	}
	return result("no_version_added", !obs.versionAdded, "the suggestion saved a new version")
}

func checkImported(obs observation) CheckResult {
	const name = "imported"
	switch {
	case obs.request.Status != "succeeded":
		return result(name, false, "request "+obs.request.Status)
	case obs.request.DashboardID == nil || obs.request.ResultVersion == nil:
		return result(name, false, "the request saved no dashboard version")
	case obs.importErr != "":
		return result(name, false, obs.importErr)
	}
	return result(name, obs.importRecord != nil, "the dashboard has no import record")
}

func checkBudgets(ex Expect, obs observation) CheckResult {
	const name = checkNameBudgets
	u := obs.request.Usage
	budgets := []struct {
		label string
		limit int64
		used  func(*Usage) int64
	}{
		{"model calls", int64(ex.MaxModelCalls), func(u *Usage) int64 { return int64(u.ModelCalls) }},
		{"output tokens", ex.MaxOutputTokens, func(u *Usage) int64 { return u.OutputTokens }},
		{"largest save output tokens", ex.MaxSaveOutputTokens, func(u *Usage) int64 { return u.MaxSaveOutputTokens }},
	}
	var over, unknown []string
	for _, b := range budgets {
		if b.limit == 0 {
			continue
		}
		if u == nil || b.used(u) == 0 {
			unknown = append(unknown, b.label)
			continue
		}
		if got := b.used(u); got > b.limit {
			over = append(over, fmt.Sprintf("%s %d, limit %d", b.label, got, b.limit))
		}
	}
	var details []string
	if len(over) > 0 {
		details = append(details, "over budget: "+strings.Join(over, "; "))
	}
	if len(unknown) > 0 {
		details = append(details, "unknown, the provider reported none: "+strings.Join(unknown, ", "))
	}
	return result(name, len(details) == 0, strings.Join(details, "; "))
}

func panelsToFetch(sc Scenario, obs observation) func(*spec.Panel) bool {
	switch kindOf(sc) {
	case kindFixUnverified:
		return func(p *spec.Panel) bool { return p.ID == obs.target }
	case kindRevise:
		added := map[string]bool{}
		for _, p := range newDataPanels(obs) {
			added[p.ID] = true
		}
		return func(p *spec.Panel) bool {
			return added[p.ID] || wantsSeries(sc.Expect, obs, p) || (sc.Expect.TargetReturnsData && p.ID == obs.target)
		}
	}
	return func(*spec.Panel) bool { return true }
}

func wantsSeries(ex Expect, obs observation, p *spec.Panel) bool {
	query := obs.searchableQuery(p)
	return slices.ContainsFunc(ex.PanelValues, func(pv PanelValue) bool { return strings.Contains(query, pv.QueryContains) })
}

func (o observation) panelDataRejected() bool {
	for _, outcome := range o.panels {
		var api *apiError
		if errors.As(outcome.err, &api) && api.Status == http.StatusUnprocessableEntity {
			return true
		}
	}
	return false
}
