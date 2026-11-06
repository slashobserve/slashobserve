package main

import (
	"context"
	"io"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func rate(v float64) *float64 { return &v }

func pricingFixture(t *testing.T, body string) (Pricing, error) {
	t.Helper()
	return loadPricing(writeFile(t, "pricing.yaml", body, 0o600))
}

const anthropicPricing = `models:
  claude-haiku-5-5: { input: 0.10, output: 0.50, cache_read: 0.01, cache_write: 0.125, match: [claude-haiku-5] }
  claude-sonnet-5-5: { input: 2.00, output: 10.00, cache_read: 0.20, cache_write: 2.50, match: [claude-sonnet-5, claude-sonnet-5-5] }
  partial: { input: 1.00, output: null, match: [partial-] }
`

func costOf(t *testing.T, p Pricing, model string, input, output, cacheRead, cacheWrite int64) *float64 {
	t.Helper()
	return p.cost(&Usage{Model: model, InputTokens: input, OutputTokens: output, CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite})
}

func TestPricingLooksUpExactThenLongestPrefix(t *testing.T) {
	p, err := pricingFixture(t, anthropicPricing)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]float64{
		"claude-haiku-5-5":          0.10 + 0.50 + 0.01 + 0.125,
		"claude-haiku-5-5-20261001": 0.10 + 0.50 + 0.01 + 0.125,
		"claude-sonnet-5-5-latest":  2 + 10 + 0.20 + 2.50,
	}
	for model, want := range cases {
		got := costOf(t, p, model, 1e6, 1e6, 1e6, 1e6)
		if got == nil || math.Abs(*got-want) > 1e-9 {
			t.Errorf("%s: cost %v, want %v", model, got, want)
		}
	}
	if _, ok := p.lookup("claude-opus-5-5"); ok {
		t.Error("an unlisted model must stay unknown")
	}
	if got := costOf(t, p, "claude-opus-5-5", 10, 10, 0, 0); got != nil {
		t.Errorf("an unknown model costs nothing known, got %v", *got)
	}
}

func TestPricingLeavesTheCostBlankWhenAUsedRateIsNull(t *testing.T) {
	p, err := pricingFixture(t, anthropicPricing)
	if err != nil {
		t.Fatal(err)
	}
	if got := costOf(t, p, "partial-1", 1e6, 0, 0, 0); got == nil || *got != 1 {
		t.Fatalf("only priced token classes were used: %v", got)
	}
	if got := costOf(t, p, "partial-1", 1e6, 1, 0, 0); got != nil {
		t.Fatalf("output tokens at a null rate must leave the cost blank, got %v", *got)
	}
}

func TestPricingOverrideReplacesTheLookup(t *testing.T) {
	p, err := pricingFixture(t, anthropicPricing)
	if err != nil {
		t.Fatal(err)
	}
	override := p.with(&Price{Input: rate(1), Output: rate(2), CacheRead: rate(0), CacheWrite: rate(0)})
	if got := costOf(t, override, "claude-opus-5-5", 1e6, 1e6, 0, 0); got == nil || *got != 3 {
		t.Fatalf("override cost = %v, want 3", got)
	}
	if got := costOf(t, p, "claude-haiku-5-5", 1e6, 0, 0, 0); got == nil || *got != 0.10 {
		t.Fatalf("with must not change the shared pricing, got %v", got)
	}
}

func TestPricingRejectsAmbiguousOrNegativeEntries(t *testing.T) {
	cases := map[string]string{
		"is also listed by":           "models:\n  a: { input: 1, match: [claude-] }\n  b: { input: 1, match: [claude-] }\n",
		"must not contain an empty":   "models:\n  a: { input: 1, match: [''] }\n",
		"prices must not be negative": "models:\n  a: { input: -1 }\n",
		"unknown field":               "models:\n  a: { input: 1, matches: [a] }\n",
	}
	for want, body := range cases {
		if _, err := pricingFixture(t, body); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error %v, want %q", err, want)
		}
	}
}

func TestPricingShippedFilePricesTheAnthropicModelsByPrefix(t *testing.T) {
	p, err := loadPricing(filepath.Join("..", "pricing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"claude-haiku-5-5", "claude-sonnet-5-5", "claude-opus-5-5", "claude-haiku-5-5-20261001", "claude-opus-5-5-latest", "replay"} {
		if got := costOf(t, p, model, 1000, 1000, 1000, 1000); got == nil {
			t.Errorf("%s has no price", model)
		}
	}
}

func TestPricingReportsAnUnknownModelOnce(t *testing.T) {
	cost := 0.5
	results := []Result{
		{ID: "a", Usage: &Usage{Model: "mystery-1"}},
		{ID: "b", Usage: &Usage{Model: "mystery-1"}},
		{ID: "c", Usage: &Usage{Model: "claude-haiku-5-5"}, CostUSD: &cost},
		{ID: "d", Usage: &Usage{Model: "mystery-2"}, Skipped: true},
		{ID: "e"},
	}
	agg := aggregate(results)
	if len(agg.Cost.UnpricedModels) != 1 || agg.Cost.UnpricedModels[0] != "mystery-1" {
		t.Fatalf("unpriced = %v", agg.Cost.UnpricedModels)
	}
	markdown := renderMarkdown(Report{Aggregates: agg, Results: results}, gateOutcome{})
	if strings.Count(markdown, "mystery-1") != 1 || !strings.Contains(markdown, "Cost unknown for model `mystery-1`") {
		t.Fatalf("markdown must name the unknown model once:\n%s", markdown)
	}
	if !strings.Contains(markdown, "| a | n/a | n/a | 0 | 0 | FAIL |  | n/a | n/a |") {
		t.Fatalf("an unknown cost stays blank:\n%s", markdown)
	}
}

func TestPricingMatrixEntryOverridesTheLookup(t *testing.T) {
	entries, err := loadMatrix(filepath.Join("..", "matrix.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Pricing != nil || entries[1].Pricing == nil || *entries[1].Pricing.Output != 5 {
		t.Fatalf("entries %+v", entries)
	}
	if _, err := loadMatrix(writeFile(t, "matrix.yaml", "models:\n  - name: a\n    env_file: a.env\n    pricing: { input: -2 }\n", 0o600)); err == nil || !strings.Contains(err.Error(), "models[0].pricing: prices must not be negative") {
		t.Fatalf("error %v", err)
	}

	var priced []*Price
	step := matrixStep{
		switchTo: func(context.Context, string) error { return nil },
		saveLogs: func(context.Context, string) error { return nil },
		ready:    func(context.Context) error { return nil },
		run: func(_ context.Context, entry matrixEntry) (Report, gateOutcome, error) {
			priced = append(priced, entry.Pricing)
			return cannedReport("p", "m", 1, 1, kindCreate), gateOutcome{}, nil
		},
	}
	if ok, err := runMatrix(t.Context(), entries, step, t.TempDir(), io.Discard); err != nil || !ok {
		t.Fatalf("ok %t, err %v", ok, err)
	}
	if len(priced) != 2 || priced[0] != nil || priced[1] != entries[1].Pricing {
		t.Fatalf("each run must get its own entry's pricing: %v", priced)
	}
}
