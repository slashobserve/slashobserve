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

	maxPromptRunes    = 4000
	defaultMinPanels  = 1
	defaultMaxPanels  = 12
	defaultMaxSeconds = 60
)

var idPattern = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)

type Scenario struct {
	ID       string   `yaml:"id" json:"id"`
	Prompt   string   `yaml:"prompt" json:"prompt"`
	Category string   `yaml:"category" json:"category"`
	DoneWhen bool     `yaml:"done_when" json:"done_when"`
	Tags     []string `yaml:"tags" json:"tags"`
	Expect   Expect   `yaml:"expect" json:"expect"`
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
	if sc.Expect.MinPanels == 0 {
		sc.Expect.MinPanels = defaultMinPanels
	}
	if sc.Expect.MaxPanels == 0 {
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
	if utf8.RuneCountInString(sc.Prompt) > maxPromptRunes {
		return fmt.Errorf("prompt is longer than %d characters", maxPromptRunes)
	}
	want, ok := datasourcesFor(sc.Category)
	if !ok {
		return fmt.Errorf("category %q must be prometheus, clickhouse or mixed", sc.Category)
	}
	got := slices.Clone(sc.Expect.Datasources)
	slices.Sort(got)
	if len(got) == 0 || !slices.Equal(got, want) {
		return fmt.Errorf("expect.datasources %v must be %v for category %s", sc.Expect.Datasources, want, sc.Category)
	}
	if sc.Expect.MinPanels < 1 || sc.Expect.MaxPanels < sc.Expect.MinPanels {
		return fmt.Errorf("expect.min_panels %d and expect.max_panels %d must satisfy 1 <= min <= max", sc.Expect.MinPanels, sc.Expect.MaxPanels)
	}
	if sc.Expect.MaxSeconds < 1 {
		return errors.New("expect.max_seconds must be positive")
	}
	if len(sc.Expect.Metrics) > 0 && !slices.Contains(got, string(spec.Prometheus)) {
		return errors.New("expect.metrics needs a prometheus datasource")
	}
	if len(sc.Expect.Tables) > 0 && !slices.Contains(got, string(spec.ClickHouse)) {
		return errors.New("expect.tables needs a clickhouse datasource")
	}
	return validateExpectations(sc)
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
		if _, err := spec.ResolveTime(from, time.Now(), false); err != nil {
			return fmt.Errorf("expect.time_range_from: %w", err)
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
