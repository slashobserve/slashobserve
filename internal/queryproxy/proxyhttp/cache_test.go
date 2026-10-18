package proxyhttp

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

var (
	idA = pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	idB = pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
)

func baseAlignment() alignment {
	return alignment{
		rng:           datasource.TimeRange{From: at(-time.Hour), To: at(0)},
		step:          15 * time.Second,
		maxDataPoints: 240,
	}
}

func sel(all bool, values ...string) queryproxy.Selection {
	return queryproxy.Selection{All: all, Values: values}
}

func TestDataKeyDiffersOnEveryInput(t *testing.T) {
	base := map[string]queryproxy.Selection{"pod": sel(false, "a")}
	key := dataKey
	reference := key(idA, 1, "p", queryproxy.KindRange, baseAlignment(), base)
	if reference != key(idA, 1, "p", queryproxy.KindRange, baseAlignment(), map[string]queryproxy.Selection{"pod": sel(false, "a")}) {
		t.Fatal("equal inputs produced different keys")
	}

	moved := baseAlignment()
	moved.rng.To = moved.rng.To.Add(time.Second)
	started := baseAlignment()
	started.rng.From = started.rng.From.Add(-time.Second)
	slower := baseAlignment()
	slower.step = 30 * time.Second

	variants := map[string]cacheKey{
		"dashboard":      key(idB, 1, "p", queryproxy.KindRange, baseAlignment(), base),
		"version":        key(idA, 2, "p", queryproxy.KindRange, baseAlignment(), base),
		"panel":          key(idA, 1, "q", queryproxy.KindRange, baseAlignment(), base),
		"kind":           key(idA, 1, "p", queryproxy.KindInstant, baseAlignment(), base),
		"to":             key(idA, 1, "p", queryproxy.KindRange, moved, base),
		"from":           key(idA, 1, "p", queryproxy.KindRange, started, base),
		"step":           key(idA, 1, "p", queryproxy.KindRange, slower, base),
		"variable name":  key(idA, 1, "p", queryproxy.KindRange, baseAlignment(), map[string]queryproxy.Selection{"node": sel(false, "a")}),
		"variable value": key(idA, 1, "p", queryproxy.KindRange, baseAlignment(), map[string]queryproxy.Selection{"pod": sel(false, "b")}),
		"extra variable": key(idA, 1, "p", queryproxy.KindRange, baseAlignment(), map[string]queryproxy.Selection{"pod": sel(false, "a"), "x": sel(false, "1")}),
		"no variables":   key(idA, 1, "p", queryproxy.KindRange, baseAlignment(), nil),
	}
	seen := map[cacheKey]string{reference: "reference"}
	for name, k := range variants {
		if other, dup := seen[k]; dup {
			t.Errorf("%s collides with %s", name, other)
		}
		seen[k] = name
	}
}

func TestSelectionEncodingIsUnambiguous(t *testing.T) {
	cases := map[string][2]map[string]queryproxy.Selection{
		"All flag":        {{"v": sel(true)}, {"v": sel(false)}},
		"All with values": {{"v": sel(true, "a")}, {"v": sel(false, "a")}},
		"value order":     {{"v": sel(false, "a", "b")}, {"v": sel(false, "b", "a")}},
		"value split":     {{"v": sel(false, "ab")}, {"v": sel(false, "a", "b")}},
		"name boundary":   {{"a": sel(false, "bc")}, {"ab": sel(false, "c")}},
		"empty value":     {{"v": sel(false, "")}, {"v": sel(false)}},
		"value into name": {{"a": sel(false, "x"), "b": sel(false)}, {"a": sel(false), "b": sel(false, "x")}},
	}
	for name, pair := range cases {
		d1 := dataKey(idA, 1, "p", queryproxy.KindRange, baseAlignment(), pair[0])
		d2 := dataKey(idA, 1, "p", queryproxy.KindRange, baseAlignment(), pair[1])
		if d1 == d2 {
			t.Errorf("dataKey: %s collides", name)
		}
		v1 := valuesKey(idA, 1, "n", at(-time.Hour), at(0), pair[0])
		v2 := valuesKey(idA, 1, "n", at(-time.Hour), at(0), pair[1])
		if v1 == v2 {
			t.Errorf("valuesKey: %s collides", name)
		}
	}
}

func TestSelectionEncodingIgnoresMapOrder(t *testing.T) {
	a := map[string]queryproxy.Selection{"x": sel(false, "1"), "y": sel(false, "2"), "z": sel(true)}
	first := dataKey(idA, 1, "p", queryproxy.KindRange, baseAlignment(), a)
	for range 20 {
		b := map[string]queryproxy.Selection{"z": sel(true), "y": sel(false, "2"), "x": sel(false, "1")}
		if dataKey(idA, 1, "p", queryproxy.KindRange, baseAlignment(), b) != first {
			t.Fatal("key depends on map iteration order")
		}
	}
}

func TestValuesKeyDiffersOnInputs(t *testing.T) {
	base := map[string]queryproxy.Selection{"job": sel(false, "a")}
	from, to := at(-time.Hour), at(0)
	reference := valuesKey(idA, 1, "pod", from, to, base)
	variants := map[string]cacheKey{
		"dashboard": valuesKey(idB, 1, "pod", from, to, base),
		"version":   valuesKey(idA, 2, "pod", from, to, base),
		"name":      valuesKey(idA, 1, "node", from, to, base),
		"from":      valuesKey(idA, 1, "pod", from.Add(-10*time.Second), to, base),
		"to":        valuesKey(idA, 1, "pod", from, to.Add(10*time.Second), base),
		"selection": valuesKey(idA, 1, "pod", from, to, map[string]queryproxy.Selection{"job": sel(false, "b")}),
	}
	for name, k := range variants {
		if k == reference {
			t.Errorf("%s does not change the key", name)
		}
	}
	if valuesKey(idA, 1, "pod", from.Add(2*time.Second), to.Add(3*time.Second), base) != reference {
		t.Error("the range is not floored to the values resolution")
	}
}

func TestDataAndValuesKeysNeverCollide(t *testing.T) {
	if dataKey(idA, 1, "n", queryproxy.KindRange, alignment{}, nil) == valuesKey(idA, 1, "n", time.Time{}, time.Time{}, nil) {
		t.Fatal("data and values share a key space")
	}
}

func TestValuesEntryCostGrowsWithContent(t *testing.T) {
	small := valuesEntry{values: []string{"a"}}.cost()
	large := valuesEntry{values: []string{"a", "bbbbbbbbbb"}}.cost()
	if small <= 0 || large <= small {
		t.Fatalf("costs %d and %d", small, large)
	}
}
