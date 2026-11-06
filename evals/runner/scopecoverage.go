package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

const (
	defaultPrometheus   = "http://localhost:29090"
	defaultCoverage     = 0.9
	maxListedUncovered  = 20
	coverageCheckName   = "coverage"
	histogramBucketPart = "_bucket"
	errNoScopeFamilies  = "no metric family matches the scope"
)

var (
	metricToken       = regexp.MustCompile(`[a-zA-Z_:][a-zA-Z0-9_:]*`)
	histogramSuffixes = []string{histogramBucketPart, "_sum", "_count"}
	excludedFamilies  = []familyExclusion{{name: "up"}, {name: "scrape_", prefix: true}, {name: "process_", prefix: true}, {name: "go_", prefix: true}, {name: "promhttp_", prefix: true}}
)

type familyExclusion struct {
	name   string
	prefix bool
}

func (x familyExclusion) matches(name string) bool {
	if x.prefix {
		return strings.HasPrefix(name, x.name)
	}
	return name == x.name
}

type coverageMeasure struct {
	scope     string
	covered   int
	families  int
	uncovered []string
	err       string
}

func (m coverageMeasure) score() *float64 {
	if m.err != "" || m.families == 0 {
		return nil
	}
	s := float64(m.covered) / float64(m.families)
	return &s
}

func seriesNames(ctx context.Context, hc *http.Client, base, scope string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/series?"+url.Values{"match[]": {scope}}.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	var body struct {
		Status string              `json:"status"`
		Error  string              `json:"error"`
		Data   []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("prometheus answered %d with no series list", resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 || body.Status != "success" {
		return nil, fmt.Errorf("prometheus answered %d: %s", resp.StatusCode, body.Error)
	}
	var names []string
	for _, labels := range body.Data {
		if name := labels["__name__"]; name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names, nil
}

func metricFamilies(scope string, names []string) map[string][]string {
	scopeTokens := metricToken.FindAllString(scope, -1)
	families := map[string][]string{}
	for _, name := range names {
		if excludedByDefault(name, scopeTokens) {
			continue
		}
		base := histogramBase(name, names)
		families[base] = append(families[base], name)
	}
	return families
}

func excludedByDefault(name string, scopeTokens []string) bool {
	for _, x := range excludedFamilies {
		if x.matches(name) && !slices.ContainsFunc(scopeTokens, x.matches) {
			return true
		}
	}
	return false
}

func histogramBase(name string, names []string) string {
	for _, suffix := range histogramSuffixes {
		base, ok := strings.CutSuffix(name, suffix)
		if ok && slices.Contains(names, base+histogramBucketPart) {
			return base
		}
	}
	return name
}

func measureCoverage(scope string, names []string, obs observation) coverageMeasure {
	m := coverageMeasure{scope: scope}
	families := metricFamilies(scope, names)
	if len(families) == 0 {
		m.err = errNoScopeFamilies
		return m
	}
	named := map[string]bool{}
	for _, p := range dataPanels(obs.dashboard) {
		for _, token := range metricToken.FindAllString(obs.searchableQuery(p), -1) {
			named[token] = true
		}
	}
	m.families = len(families)
	for _, family := range slices.Sorted(maps.Keys(families)) {
		if named[family] || slices.ContainsFunc(families[family], func(member string) bool { return named[member] }) {
			m.covered++
			continue
		}
		m.uncovered = append(m.uncovered, family)
	}
	return m
}

func (e *execution) measureCoverage(ctx context.Context) {
	scope := e.sc.Expect.Scope
	names, err := seriesNames(ctx, e.c.http, e.o.prometheus, scope)
	if err != nil {
		e.obs.coverage = &coverageMeasure{scope: scope, err: "read the series in scope: " + err.Error()}
		return
	}
	m := measureCoverage(scope, names, e.obs)
	e.obs.coverage = &m
}

func checkCoverage(ex Expect, obs observation) CheckResult {
	if c, skip := noSpec(coverageCheckName, obs); skip {
		return c
	}
	m := obs.coverage
	if m == nil {
		return result(coverageCheckName, false, "the series in scope were not read")
	}
	if m.err != "" {
		return result(coverageCheckName, false, m.err+" "+m.scope)
	}
	score := *m.score()
	detail := fmt.Sprintf("%d of %d metric families in %s are queried (%.2f, need %.2f)", m.covered, m.families, m.scope, score, ex.Coverage)
	if len(m.uncovered) > 0 {
		detail += "; not queried: " + listFirst(m.uncovered, maxListedUncovered)
	}
	return result(coverageCheckName, score >= ex.Coverage-shareEpsilon, detail)
}

func validateCoverage(sc Scenario) error {
	ex := sc.Expect
	switch {
	case ex.Scope == "" && ex.Coverage != 0:
		return errors.New("expect.coverage needs expect.scope")
	case ex.Scope == "":
		return nil
	case sc.Kind != kindCreate:
		return fmt.Errorf("expect.scope applies to %s scenarios only", kindCreate)
	case math.IsNaN(ex.Coverage) || ex.Coverage <= 0 || ex.Coverage > 1:
		return errors.New("expect.coverage must be above 0 and at most 1")
	}
	if _, err := promParser.ParseMetricSelector(ex.Scope); err != nil {
		return fmt.Errorf("expect.scope %q is not a Prometheus series selector: %w", ex.Scope, err)
	}
	return nil
}
