package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/goccy/go-yaml"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	categoryPrometheus = "prometheus"
	categoryClickHouse = "clickhouse"
	categoryMixed      = "mixed"

	kindCreate        = "create"
	kindRevise        = "revise"
	kindFixUnverified = "fix_unverified"
	kindSuggest       = "suggest"
	kindChatImport    = "chat_import"
	kindSizeGuard     = "size_guard"

	suiteReplay = "replay"
	suiteLive   = "live"

	tagReplaySmoke = "replay-smoke"

	faultErrors  = "errors"
	faultLatency = "latency"

	placeholderPanelID    = "{panel_id}"
	placeholderPanelTitle = "{panel_title}"
	placeholderRowTitle   = "{row_title}"

	maxPromptRunes    = 4000
	defaultMinPanels  = 1
	defaultMaxPanels  = 12
	defaultMaxSeconds = 60
)

var (
	kinds  = []string{kindCreate, kindRevise, kindFixUnverified, kindSuggest, kindChatImport, kindSizeGuard}
	suites = []string{suiteReplay, suiteLive}

	idPattern = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)
)

type Scenario struct {
	ID       string   `yaml:"id" json:"id"`
	Prompt   string   `yaml:"prompt" json:"prompt"`
	Category string   `yaml:"category" json:"category"`
	DoneWhen bool     `yaml:"done_when" json:"done_when"`
	Tags     []string `yaml:"tags" json:"tags"`
	Expect   Expect   `yaml:"expect" json:"expect"`
	Kind     string   `yaml:"kind" json:"kind"`
	Suites   []string `yaml:"suites" json:"suites"`
	Seed     *Seed    `yaml:"seed" json:"seed,omitempty"`
	Fault    *Fault   `yaml:"fault" json:"fault,omitempty"`
}

type Seed struct {
	Corpus     string            `yaml:"corpus" json:"corpus"`
	BreakPanel bool              `yaml:"break_panel" json:"break_panel"`
	Mapping    map[string]string `yaml:"mapping" json:"mapping,omitempty"`
	Target     string            `yaml:"target" json:"target,omitempty"`
	Row        string            `yaml:"row" json:"row,omitempty"`
}

type Fault struct {
	Pod         string `yaml:"pod" json:"pod"`
	Type        string `yaml:"type" json:"type"`
	Pct         int    `yaml:"pct" json:"pct"`
	SoakSeconds int    `yaml:"soak_seconds" json:"soak_seconds"`
}

type PanelValue struct {
	QueryContains string            `yaml:"query_contains" json:"query_contains"`
	Series        map[string]string `yaml:"series" json:"series,omitempty"`
	Min           float64           `yaml:"min" json:"min"`
	Max           float64           `yaml:"max" json:"max"`
}

type Expect struct {
	Datasources []string `yaml:"datasources" json:"datasources"`
	Metrics     []string `yaml:"metrics" json:"metrics,omitempty"`
	Tables      []string `yaml:"tables" json:"tables,omitempty"`
	Units       []string `yaml:"units" json:"units,omitempty"`
	MinPanels   int      `yaml:"min_panels" json:"min_panels"`
	MaxPanels   int      `yaml:"max_panels" json:"max_panels"`
	Thresholds  bool     `yaml:"thresholds" json:"thresholds,omitempty"`
	MaxSeconds  int      `yaml:"max_seconds" json:"max_seconds"`

	QueryContainsAny [][]string         `yaml:"query_contains_any" json:"query_contains_any,omitempty"`
	TimeRangeFrom    []string           `yaml:"time_range_from" json:"time_range_from,omitempty"`
	ThresholdAt      map[string]float64 `yaml:"threshold_at" json:"threshold_at,omitempty"`
	ChangedPaths     []string           `yaml:"changed_paths" json:"changed_paths,omitempty"`

	FailureCode         string         `yaml:"failure_code" json:"failure_code,omitempty"`
	OnlyTargetChanged   bool           `yaml:"only_target_changed" json:"only_target_changed,omitempty"`
	TargetReturnsData   bool           `yaml:"target_returns_data" json:"target_returns_data,omitempty"`
	UnverifiedCleared   bool           `yaml:"unverified_cleared" json:"unverified_cleared,omitempty"`
	KeepsPanels         bool           `yaml:"keeps_panels" json:"keeps_panels,omitempty"`
	MaxModelCalls       int            `yaml:"max_model_calls" json:"max_model_calls,omitempty"`
	MaxOutputTokens     int64          `yaml:"max_output_tokens" json:"max_output_tokens,omitempty"`
	MaxSaveOutputTokens int64          `yaml:"max_save_output_tokens" json:"max_save_output_tokens,omitempty"`
	PanelValues         []PanelValue   `yaml:"panel_values" json:"panel_values,omitempty"`
	RebuildMetricShare  float64        `yaml:"rebuild_metric_share" json:"rebuild_metric_share,omitempty"`
	RebuildMetricCap    int            `yaml:"rebuild_metric_cap" json:"rebuild_metric_cap,omitempty"`
	TargetType          string         `yaml:"target_type" json:"target_type,omitempty"`
	ChangedPanels       int            `yaml:"changed_panels" json:"changed_panels,omitempty"`
	Usage               map[string]int `yaml:"usage" json:"usage,omitempty"`
}

type Price struct {
	Input      float64 `yaml:"input"`
	Output     float64 `yaml:"output"`
	CacheRead  float64 `yaml:"cache_read"`
	CacheWrite float64 `yaml:"cache_write"`
}

type Pricing struct {
	Models map[string]Price `yaml:"models"`
}

func decodeStrict(content []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(content), yaml.Strict(), yaml.DisallowUnknownField())
	if err := decoder.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty file")
		}
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("multiple YAML documents are not supported")
	}
	return nil
}

func loadScenarios(dir string) ([]Scenario, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no scenarios in %s", dir)
	}
	slices.Sort(paths)

	var errs []error
	seen := map[string]string{}
	out := make([]Scenario, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path) //nolint:gosec // the path comes from the operator's flags
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var sc Scenario
		if err := decodeStrict(content, &sc); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		applyDefaults(&sc)
		if err := validateScenario(sc); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		if previous, ok := seen[sc.ID]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate id %q (also in %s)", path, sc.ID, previous))
			continue
		}
		seen[sc.ID] = path
		out = append(out, sc)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

func applyDefaults(sc *Scenario) {
	if sc.Tags == nil {
		sc.Tags = []string{}
	}
	if sc.Kind == "" {
		sc.Kind = kindCreate
	}
	if len(sc.Suites) == 0 {
		sc.Suites = []string{suiteLive}
	}
	if slices.Contains(sc.Tags, tagReplaySmoke) && !slices.Contains(sc.Suites, suiteReplay) {
		sc.Suites = append(sc.Suites, suiteReplay)
	}
	if sc.Expect.MinPanels == 0 {
		sc.Expect.MinPanels = defaultMinPanels
	}
	if sc.Kind == kindCreate && sc.Expect.MaxPanels == 0 {
		sc.Expect.MaxPanels = defaultMaxPanels
	}
	if sc.Expect.MaxSeconds == 0 {
		sc.Expect.MaxSeconds = defaultMaxSeconds
	}
}

func validateScenario(sc Scenario) error {
	if !idPattern.MatchString(sc.ID) {
		return fmt.Errorf("id %q must match %s", sc.ID, idPattern)
	}
	if strings.TrimSpace(sc.Prompt) == "" {
		return errors.New("prompt is required")
	}
	if sc.Kind != kindChatImport && sc.Kind != kindSizeGuard && utf8.RuneCountInString(sc.Prompt) > maxPromptRunes {
		return fmt.Errorf("prompt is longer than %d characters", maxPromptRunes)
	}
	if err := validateKind(sc); err != nil {
		return err
	}
	if sc.Kind == kindCreate || sc.Category != "" || len(sc.Expect.Datasources) > 0 {
		if err := validateDatasources(sc); err != nil {
			return err
		}
	}
	if sc.Expect.MinPanels < 1 || (sc.Expect.MaxPanels > 0 && sc.Expect.MaxPanels < sc.Expect.MinPanels) {
		return fmt.Errorf("expect.min_panels %d and expect.max_panels %d must satisfy 1 <= min <= max", sc.Expect.MinPanels, sc.Expect.MaxPanels)
	}
	if sc.Expect.MaxSeconds < 1 {
		return errors.New("expect.max_seconds must be positive")
	}
	return validateExpectations(sc)
}

func validateDatasources(sc Scenario) error {
	want, ok := datasourcesFor(sc.Category)
	if !ok {
		return fmt.Errorf("category %q must be prometheus, clickhouse or mixed", sc.Category)
	}
	got := slices.Clone(sc.Expect.Datasources)
	slices.Sort(got)
	if len(got) == 0 || !slices.Equal(got, want) {
		return fmt.Errorf("expect.datasources %v must be %v for category %s", sc.Expect.Datasources, want, sc.Category)
	}
	if len(sc.Expect.Metrics) > 0 && !slices.Contains(got, string(spec.Prometheus)) {
		return errors.New("expect.metrics needs a prometheus datasource")
	}
	if len(sc.Expect.Tables) > 0 && !slices.Contains(got, string(spec.ClickHouse)) {
		return errors.New("expect.tables needs a clickhouse datasource")
	}
	return nil
}

func validateKind(sc Scenario) error {
	if !slices.Contains(kinds, sc.Kind) {
		return fmt.Errorf("kind %q must be one of %s", sc.Kind, strings.Join(kinds, ", "))
	}
	if sc.Kind == kindSizeGuard && sc.Expect.FailureCode != failurePromptTooLong {
		return fmt.Errorf("a %s scenario must expect failure_code %s", kindSizeGuard, failurePromptTooLong)
	}
	if err := validateSuites(sc.Suites); err != nil {
		return err
	}
	if err := validateSeedUse(sc); err != nil {
		return err
	}
	if sc.Fault != nil {
		return validateFault(*sc.Fault)
	}
	return nil
}

func validateSeedUse(sc Scenario) error {
	needsSeed := sc.Kind == kindRevise || sc.Kind == kindFixUnverified || sc.Kind == kindSuggest
	usesSeed := sc.Expect.readsSeed() || promptReadsSeed(sc.Prompt)
	switch {
	case sc.Seed == nil && needsSeed:
		return fmt.Errorf("seed is required for kind %s", sc.Kind)
	case sc.Seed == nil && usesSeed:
		return errors.New("seed is required for the target and keeps_panels expectations and the panel placeholders")
	case sc.Seed != nil:
		return validateSeed(*sc.Seed)
	}
	return nil
}

func (e Expect) readsSeed() bool {
	return e.OnlyTargetChanged || e.TargetReturnsData || e.UnverifiedCleared || e.KeepsPanels || e.TargetType != "" || e.ChangedPanels != 0
}

func promptReadsSeed(prompt string) bool {
	return strings.Contains(prompt, placeholderPanelID) || strings.Contains(prompt, placeholderPanelTitle) || strings.Contains(prompt, placeholderRowTitle)
}

func validateSuites(list []string) error {
	for _, suite := range list {
		if !slices.Contains(suites, suite) {
			return fmt.Errorf("suites entry %q must be replay or live", suite)
		}
	}
	if dup, ok := firstDuplicate(list); ok {
		return fmt.Errorf("suites contain %q twice", dup)
	}
	return nil
}

func validateSeed(seed Seed) error {
	if !strings.HasSuffix(seed.Corpus, ".json") || !filepath.IsLocal(seed.Corpus) {
		return fmt.Errorf("seed.corpus %q must be a relative .json path inside the corpus", seed.Corpus)
	}
	if seed.BreakPanel && seed.Target != "" {
		return errors.New("seed.target cannot be combined with break_panel, which picks its own target")
	}
	for from, to := range seed.Mapping {
		if blank(from) || blank(to) {
			return errors.New("seed.mapping must not contain an empty entry")
		}
	}
	return nil
}

func validateFault(f Fault) error {
	switch {
	case blank(f.Pod):
		return errors.New("fault.pod is required")
	case f.Type != faultErrors && f.Type != faultLatency:
		return fmt.Errorf("fault.type %q must be %s or %s", f.Type, faultErrors, faultLatency)
	case f.Pct < 1 || f.Pct > 100:
		return fmt.Errorf("fault.pct %d must be between 1 and 100", f.Pct)
	case f.SoakSeconds < 0:
		return errors.New("fault.soak_seconds must not be negative")
	}
	return nil
}

func validateExpectations(sc Scenario) error {
	ex := sc.Expect
	lists := []struct {
		field   string
		entries []string
	}{
		{"tags", sc.Tags},
		{"expect.metrics", ex.Metrics},
		{"expect.tables", ex.Tables},
		{"expect.units", ex.Units},
		{"expect.time_range_from", ex.TimeRangeFrom},
		{"expect.changed_paths", ex.ChangedPaths},
	}
	for _, l := range lists {
		if slices.ContainsFunc(l.entries, blank) {
			return fmt.Errorf("%s must not contain an empty entry", l.field)
		}
	}
	if dup, ok := firstDuplicate(sc.Tags); ok {
		return fmt.Errorf("tags contain %q twice", dup)
	}
	if unknown := unknownUnits(append(slices.Clone(ex.Units), slices.Collect(maps.Keys(ex.ThresholdAt))...)); len(unknown) > 0 {
		return fmt.Errorf("unknown units %s, expected ids from spec/schema/observespec.v0.json", strings.Join(unknown, ", "))
	}
	for unit, value := range ex.ThresholdAt {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("expect.threshold_at[%s] must be a finite number", unit)
		}
	}
	for i, group := range ex.QueryContainsAny {
		if len(group) == 0 || slices.ContainsFunc(group, blank) {
			return fmt.Errorf("expect.query_contains_any[%d] needs at least one alternative and no empty entry", i)
		}
	}
	for _, from := range ex.TimeRangeFrom {
		if _, err := spec.ResolveTime(from, time.Now(), false, spec.TimeOptions{}); err != nil {
			return fmt.Errorf("expect.time_range_from: %w", err)
		}
	}
	return validateBudgetsAndValues(ex)
}

func validateBudgetsAndValues(ex Expect) error {
	if ex.MaxModelCalls < 0 || ex.MaxOutputTokens < 0 || ex.MaxSaveOutputTokens < 0 {
		return errors.New("expect.max_model_calls, max_output_tokens and max_save_output_tokens must not be negative")
	}
	if ex.RebuildMetricShare < 0 || ex.RebuildMetricShare > 1 {
		return errors.New("expect.rebuild_metric_share must be between 0 and 1")
	}
	if ex.RebuildMetricCap < 0 {
		return errors.New("expect.rebuild_metric_cap must not be negative")
	}
	if blank(ex.FailureCode) && ex.FailureCode != "" {
		return errors.New("expect.failure_code must not be blank")
	}
	if ex.TargetType != "" && !spec.PanelType(ex.TargetType).IsData() {
		return fmt.Errorf("expect.target_type %q must be a data panel type", ex.TargetType)
	}
	if ex.ChangedPanels < 0 {
		return errors.New("expect.changed_panels must be at least 1 when set")
	}
	if err := validateUsagePins(ex.Usage); err != nil {
		return err
	}
	return validatePanelValues(ex.PanelValues)
}

func validateUsagePins(pins map[string]int) error {
	for _, key := range slices.Sorted(maps.Keys(pins)) {
		if _, ok := usageCounters[key]; !ok {
			return fmt.Errorf("expect.usage key %q must be one of %s", key, strings.Join(slices.Sorted(maps.Keys(usageCounters)), ", "))
		}
		if pins[key] < 0 {
			return fmt.Errorf("expect.usage.%s must not be negative", key)
		}
	}
	return nil
}

func validatePanelValues(values []PanelValue) error {
	for i, pv := range values {
		switch {
		case blank(pv.QueryContains):
			return fmt.Errorf("expect.panel_values[%d].query_contains is required", i)
		case math.IsNaN(pv.Min) || math.IsInf(pv.Min, 0) || math.IsNaN(pv.Max) || math.IsInf(pv.Max, 0) || pv.Min > pv.Max:
			return fmt.Errorf("expect.panel_values[%d] needs finite min <= max", i)
		}
		for key, value := range pv.Series {
			if blank(key) || blank(value) {
				return fmt.Errorf("expect.panel_values[%d].series must not contain an empty entry", i)
			}
		}
	}
	return nil
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

func firstDuplicate(values []string) (string, bool) {
	seen := map[string]bool{}
	for _, v := range values {
		if seen[v] {
			return v, true
		}
		seen[v] = true
	}
	return "", false
}

func unknownUnits(units []string) []string {
	valid := spec.Units()
	var unknown []string
	seen := map[string]bool{}
	for _, unit := range units {
		if slices.Contains(valid, unit) || seen[unit] {
			continue
		}
		seen[unit] = true
		unknown = append(unknown, strconv.Quote(unit))
	}
	slices.Sort(unknown)
	return unknown
}

func datasourcesFor(category string) ([]string, bool) {
	switch category {
	case categoryPrometheus:
		return []string{string(spec.Prometheus)}, true
	case categoryClickHouse:
		return []string{string(spec.ClickHouse)}, true
	case categoryMixed:
		return []string{string(spec.ClickHouse), string(spec.Prometheus)}, true
	}
	return nil, false
}

func filterScenarios(all []Scenario, ids, tags []string) ([]Scenario, error) {
	for _, id := range ids {
		if !slices.ContainsFunc(all, func(sc Scenario) bool { return sc.ID == id }) {
			return nil, fmt.Errorf("unknown scenario id %q", id)
		}
	}
	var out []Scenario
	for _, sc := range all {
		if len(ids) > 0 && !slices.Contains(ids, sc.ID) {
			continue
		}
		if len(tags) > 0 && !slices.ContainsFunc(tags, func(tag string) bool { return slices.Contains(sc.Tags, tag) }) {
			continue
		}
		out = append(out, sc)
	}
	if len(out) == 0 {
		return nil, errors.New("no scenario matches the --id and --tag filters")
	}
	return out, nil
}

func loadPricing(path string) (Pricing, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path comes from the operator's flags
	if err != nil {
		return Pricing{}, err
	}
	var pricing Pricing
	if err := decodeStrict(content, &pricing); err != nil {
		return Pricing{}, fmt.Errorf("%s: %w", path, err)
	}
	return pricing, nil
}

func (p Pricing) cost(u *Usage) *float64 {
	if u == nil {
		return nil
	}
	price, ok := p.Models[u.Model]
	if !ok {
		return nil
	}
	usd := (float64(u.InputTokens)*price.Input +
		float64(u.OutputTokens)*price.Output +
		float64(u.CacheReadTokens)*price.CacheRead +
		float64(u.CacheWriteTokens)*price.CacheWrite) / 1e6
	return &usd
}
