package tracecompare

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func sets(n int, pairs func(i int) []datasource.TraceAttribute) []datasource.TraceAttributeSet {
	out := make([]datasource.TraceAttributeSet, n)
	for i := range out {
		out[i] = datasource.TraceAttributeSet{ID: fmt.Sprintf("%032x", i+1), Attributes: pairs(i)}
	}
	return out
}

func pair(key, value string) datasource.TraceAttribute {
	return datasource.TraceAttribute{Key: key, Value: value}
}

func TestCompareRanksByGapAndDropsUniformPairs(t *testing.T) {
	selected := sets(20, func(i int) []datasource.TraceAttribute {
		pairs := []datasource.TraceAttribute{pair("kind", "Server"), pair("span.db.system", "postgres")}
		if i < 18 {
			pairs = append(pairs, pair("resource.k8s.pod.name", "checkout-0"))
		}
		if i < 3 {
			pairs = append(pairs, pair("status", "Error"))
		}
		return pairs
	})
	baseline := sets(20, func(i int) []datasource.TraceAttribute {
		pairs := []datasource.TraceAttribute{pair("kind", "Server"), pair("span.db.system", "postgres")}
		if i < 2 {
			pairs = append(pairs, pair("resource.k8s.pod.name", "checkout-0"))
		}
		if i < 2 {
			pairs = append(pairs, pair("status", "Error"))
		}
		return pairs
	})
	got := Compare(selected, baseline, 20)
	want := Result{Attributes: 4, Differences: []Difference{
		{Attribute: "resource.k8s.pod.name", Value: "checkout-0", SelectedShare: 0.9, BaselineShare: 0.1, SelectedCount: 18, BaselineCount: 2},
	}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("result = %+v\nwant %+v", got, want)
	}
}

func TestCompareOrdersTiesByAttributeThenValue(t *testing.T) {
	selected := sets(10, func(int) []datasource.TraceAttribute {
		return []datasource.TraceAttribute{pair("span.b", "x"), pair("span.a", "y"), pair("span.a", "x"), pair("name", "GET /")}
	})
	baseline := sets(10, func(i int) []datasource.TraceAttribute {
		if i < 2 {
			return []datasource.TraceAttribute{pair("name", "GET /")}
		}
		return nil
	})
	got := Compare(selected, baseline, 20)
	order := make([]string, len(got.Differences))
	for i, d := range got.Differences {
		order[i] = d.Attribute + "=" + d.Value
	}
	if want := "span.a=x span.a=y span.b=x name=GET /"; strings.Join(order, " ") != want {
		t.Fatalf("order = %q, want %q", strings.Join(order, " "), want)
	}
}

func TestCompareTruncatesAtLimit(t *testing.T) {
	selected := sets(4, func(int) []datasource.TraceAttribute {
		return []datasource.TraceAttribute{pair("span.a", "1"), pair("span.b", "2"), pair("span.c", "3")}
	})
	got := Compare(selected, nil, 2)
	if len(got.Differences) != 2 || !got.Truncated || got.Attributes != 3 {
		t.Fatalf("result = %+v, want two differences, truncated, three attributes", got)
	}
	if got = Compare(selected, nil, 3); len(got.Differences) != 3 || got.Truncated {
		t.Fatalf("result = %+v, want every difference and no truncation", got)
	}
}

func TestCompareWithEmptyBaseline(t *testing.T) {
	selected := sets(3, func(int) []datasource.TraceAttribute {
		return []datasource.TraceAttribute{pair("status", "Error"), pair("status", "Error")}
	})
	got := Compare(selected, nil, 20)
	want := Result{Attributes: 1, Differences: []Difference{{Attribute: "status", Value: "Error", SelectedShare: 1, SelectedCount: 3}}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("result = %+v\nwant %+v", got, want)
	}
	empty := Compare(nil, nil, 20)
	if empty.Attributes != 0 || empty.Differences == nil || len(empty.Differences) != 0 || empty.Truncated {
		t.Fatalf("empty = %+v", empty)
	}
}

func TestCompareRoundsSharesToFourDecimals(t *testing.T) {
	selected := sets(3, func(i int) []datasource.TraceAttribute {
		if i < 2 {
			return []datasource.TraceAttribute{pair("span.a", "1")}
		}
		return nil
	})
	baseline := sets(7, func(i int) []datasource.TraceAttribute {
		if i < 3 {
			return []datasource.TraceAttribute{pair("span.a", "1")}
		}
		return nil
	})
	got := Compare(selected, baseline, 20)
	if len(got.Differences) != 1 || got.Differences[0].SelectedShare != 0.6667 || got.Differences[0].BaselineShare != 0.4286 {
		t.Fatalf("result = %+v", got)
	}
}

func TestCompareCutsLongValues(t *testing.T) {
	long := strings.Repeat("x", MaxValueBytes-1) + "é" + strings.Repeat("y", 10)
	selected := sets(2, func(int) []datasource.TraceAttribute {
		return []datasource.TraceAttribute{pair("span.sql", long), pair("span.sql", long+"z")}
	})
	got := Compare(selected, nil, 20)
	if len(got.Differences) != 1 {
		t.Fatalf("differences = %+v, want the two long values folded into one", got.Differences)
	}
	d := got.Differences[0]
	if d.Value != strings.Repeat("x", MaxValueBytes-1) || d.SelectedCount != 2 || d.SelectedShare != 1 {
		t.Fatalf("difference = %+v, want the value cut before the split rune and counted once per trace", d)
	}
}
