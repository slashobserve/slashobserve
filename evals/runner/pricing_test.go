package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/llm/pricing"
)

func rate(v float64) *float64 { return &v }

const anthropicPricing = `models:
  claude-haiku-5-5: { input: 0.10, output: 0.50, cache_read: 0.01, cache_write: 0.125, match: [claude-haiku-5] }
`

func costOf(t *testing.T, p Pricing, model string, input, output, cacheRead, cacheWrite int64) *float64 {
	t.Helper()
	return p.cost(&Usage{Model: model, InputTokens: input, OutputTokens: output, CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite})
}

func TestPricingDefaultsToTheServerTable(t *testing.T) {
	p, err := loadPricing("")
	if err != nil {
		t.Fatal(err)
	}
	if got := costOf(t, p, "claude-haiku-5-5-20261001", 1e6, 0, 0, 0); got == nil || *got != 0.10 {
		t.Fatalf("the built-in table prices dated Haiku ids, got %v", got)
	}
	if got := p.cost(nil); got != nil {
		t.Fatalf("no usage has no cost, got %v", *got)
	}
}

func TestPricingFileReplacesTheServerTable(t *testing.T) {
	p, err := loadPricing(writeFile(t, "prices.yaml", anthropicPricing, 0o600))
	if err != nil {
		t.Fatal(err)
	}
	if got := costOf(t, p, "claude-haiku-5-5", 1e6, 0, 0, 0); got == nil || *got != 0.10 {
		t.Fatalf("file cost = %v", got)
	}
	if got := costOf(t, p, "claude-opus-5-5", 10, 10, 0, 0); got != nil {
		t.Fatalf("a model missing from the file must stay unpriced, got %v", *got)
	}
	if _, err := loadPricing(writeFile(t, "bad.yaml", "models:\n  a: { input: -1 }\n", 0o600)); err == nil {
		t.Fatal("an invalid file must fail")
	}
}

func TestPricingOverrideReplacesTheLookup(t *testing.T) {
	p, err := loadPricing(writeFile(t, "prices.yaml", anthropicPricing, 0o600))
	if err != nil {
		t.Fatal(err)
	}
	override := p.with(&pricing.Price{Input: rate(1), Output: rate(2), CacheRead: rate(0), CacheWrite: rate(0)})
	if got := costOf(t, override, "claude-opus-5-5", 1e6, 1e6, 0, 0); got == nil || *got != 3 {
		t.Fatalf("override cost = %v, want 3", got)
	}
	if got := costOf(t, p, "claude-haiku-5-5", 1e6, 0, 0, 0); got == nil || *got != 0.10 {
		t.Fatalf("with must not change the shared pricing, got %v", got)
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

	var priced []*pricing.Price
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
