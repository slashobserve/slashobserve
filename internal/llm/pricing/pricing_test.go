package pricing

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const anthropicPricing = `models:
  claude-haiku-5-5: { input: 0.10, output: 0.50, cache_read: 0.01, cache_write: 0.125, match: [claude-haiku-5] }
  claude-sonnet-5-5: { input: 2.00, output: 10.00, cache_read: 0.20, cache_write: 2.50, match: [claude-sonnet-5, claude-sonnet-5-5] }
  partial: { input: 1.00, output: null, match: [partial-] }
`

func parseFixture(t *testing.T, body string) Table {
	t.Helper()
	table, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func TestTableLooksUpExactThenLongestPrefix(t *testing.T) {
	table := parseFixture(t, anthropicPricing)
	million := Tokens{Input: 1e6, Output: 1e6, CacheRead: 1e6, CacheWrite: 1e6}
	cases := map[string]float64{
		"claude-haiku-5-5":          0.10 + 0.50 + 0.01 + 0.125,
		"claude-haiku-5-5-20261001": 0.10 + 0.50 + 0.01 + 0.125,
		"claude-sonnet-5-5-latest":  2 + 10 + 0.20 + 2.50,
	}
	for name, want := range cases {
		got, ok := table.Cost(name, million)
		if !ok || math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: cost %v, %t, want %v", name, got, ok, want)
		}
	}
	if _, ok := table.Lookup("claude-opus-5-5"); ok {
		t.Error("an unlisted model must stay unknown")
	}
	if got, ok := table.Cost("claude-opus-5-5", Tokens{Input: 10, Output: 10}); ok || got != 0 {
		t.Errorf("an unknown model has no known cost, got %v, %t", got, ok)
	}
}

func TestZeroTablePricesNothing(t *testing.T) {
	if _, ok := (Table{}).Lookup("claude-haiku-5-5"); ok {
		t.Fatal("the zero table must not price any model")
	}
}

func TestCostIsBlankWhenAUsedRateIsNull(t *testing.T) {
	table := parseFixture(t, anthropicPricing)
	if got, ok := table.Cost("partial-1", Tokens{Input: 1e6}); !ok || got != 1 {
		t.Fatalf("only priced token classes were used: %v, %t", got, ok)
	}
	if got, ok := table.Cost("partial-1", Tokens{Input: 1e6, Output: 1}); ok || got != 0 {
		t.Fatalf("output tokens at a null rate must leave the cost blank, got %v, %t", got, ok)
	}
}

func TestPriceCompleteNeedsEveryRate(t *testing.T) {
	one := 1.0
	cases := map[string]struct {
		price Price
		want  bool
	}{
		"all four":       {Price{&one, &one, &one, &one}, true},
		"no input":       {Price{nil, &one, &one, &one}, false},
		"no output":      {Price{&one, nil, &one, &one}, false},
		"no cache read":  {Price{&one, &one, nil, &one}, false},
		"no cache write": {Price{&one, &one, &one, nil}, false},
		"none":           {Price{}, false},
	}
	for name, tc := range cases {
		if got := tc.price.Complete(); got != tc.want {
			t.Errorf("%s: Complete = %t, want %t", name, got, tc.want)
		}
	}
}

func TestParseRejectsAmbiguousOrNegativeEntries(t *testing.T) {
	cases := map[string]string{
		"is also listed by":           "models:\n  a: { input: 1, match: [claude-] }\n  b: { input: 1, match: [claude-] }\n",
		"must not contain an empty":   "models:\n  a: { input: 1, match: [''] }\n",
		"prices must not be negative": "models:\n  a: { input: -1 }\n",
		"unknown field":               "models:\n  a: { input: 1, matches: [a] }\n",
		"empty file":                  "",
		"multiple YAML documents":     "models:\n  a: { input: 1 }\n---\nmodels:\n  b: { input: 1 }\n",
	}
	for want, body := range cases {
		if _, err := Parse([]byte(body)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error %v, want %q", err, want)
		}
	}
}

func TestLoadNamesTheFileInErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prices.yaml")
	if err := os.WriteFile(path, []byte("models:\n  a: { input: -1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("error %v must name %s", err, path)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("a missing file must fail")
	}
	if err := os.WriteFile(path, []byte(anthropicPricing), 0o600); err != nil {
		t.Fatal(err)
	}
	table, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := table.Lookup("claude-haiku-5-5"); !ok {
		t.Fatal("a valid file must load")
	}
}

func TestDefaultPricesTheAnthropicModelsByPrefixAndReplayAtZero(t *testing.T) {
	table := Default()
	used := Tokens{Input: 1000, Output: 1000, CacheRead: 1000, CacheWrite: 1000}
	for _, name := range []string{"claude-haiku-5-5", "claude-sonnet-5-5", "claude-opus-5-5", "claude-haiku-5-5-20261001", "claude-opus-5-5-latest"} {
		price, ok := table.Lookup(name)
		if !ok || !price.Complete() {
			t.Errorf("%s has no complete price", name)
		}
		if got, ok := table.Cost(name, used); !ok || got <= 0 {
			t.Errorf("%s: cost %v, %t", name, got, ok)
		}
	}
	if got, ok := table.Cost("replay", used); !ok || got != 0 {
		t.Errorf("replay must be priced at zero, got %v, %t", got, ok)
	}
}
