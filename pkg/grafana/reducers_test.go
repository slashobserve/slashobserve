package grafana

import (
	"fmt"
	"maps"
	"slices"
	"testing"
)

func TestReducerIsReducerAcceptsNumericIDsOnly(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"p1", true}, {"p50", true}, {"p99", true}, {"variance", true}, {"countAll", true},
		{"step", true}, {"diffperc", true}, {"logmin", true}, {"lastNotNull", true},
		{"p0", false}, {"p100", false}, {"p05", false}, {"p", false}, {"px", false},
		{"p+5", false}, {"p-5", false}, {"P50", false}, {"", false}, {"allIsNull", false}, {"uniqueValues", false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if got := isReducer(tc.id); got != tc.want {
				t.Fatalf("isReducer(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestReducerIsAnyReducerAddsTheNonNumberIDs(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"allIsNull", true}, {"allIsZero", true}, {"allValues", true}, {"uniqueValues", true},
		{"p95", true}, {"sum", true},
		{"p100", false}, {"all", false}, {"", false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if got := isAnyReducer(tc.id); got != tc.want {
				t.Fatalf("isAnyReducer(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestReducerDisplayNames(t *testing.T) {
	cases := []struct{ id, want string }{
		{"lastNotNull", "Last *"}, {"sum", "Total"}, {"distinctCount", "Distinct Count"},
		{"variance", "Variance"}, {"step", "Step"}, {"diffperc", "Difference percent"},
		{"logmin", "Min (above zero)"}, {"countAll", "Count all"},
		{"allIsNull", "All nulls"}, {"allIsZero", "All zeros"}, {"allValues", "All values"}, {"uniqueValues", "All unique values"},
		{"p1", "1st %"}, {"p2", "2nd %"}, {"p3", "3rd %"}, {"p4", "4th %"},
		{"p11", "11th %"}, {"p12", "12th %"}, {"p13", "13th %"},
		{"p21", "21st %"}, {"p22", "22nd %"}, {"p23", "23rd %"}, {"p50", "50th %"}, {"p95", "95th %"}, {"p99", "99th %"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			got, ok := reducerDisplayName(tc.id)
			if !ok || got != tc.want {
				t.Fatalf("reducerDisplayName(%q) = %q, %v, want %q", tc.id, got, ok, tc.want)
			}
		})
	}
	for _, id := range []string{"p0", "p100", "bogus", ""} {
		if name, ok := reducerDisplayName(id); ok {
			t.Fatalf("reducerDisplayName(%q) = %q, want none", id, name)
		}
	}
}

func TestReducerFromDisplayNameRoundTripsEveryID(t *testing.T) {
	ids := slices.Collect(maps.Keys(numericReducerNames))
	ids = slices.AppendSeq(ids, maps.Keys(nonNumberReducerNames))
	for n := 1; n <= 99; n++ {
		ids = append(ids, fmt.Sprintf("p%d", n))
	}
	if len(ids) != 124 {
		t.Fatalf("%d reducers, want 124", len(ids))
	}
	names := map[string]string{}
	for _, id := range ids {
		name, ok := reducerDisplayName(id)
		if !ok {
			t.Fatalf("reducerDisplayName(%q) is missing", id)
		}
		if other, taken := names[name]; taken {
			t.Fatalf("%q and %q share the display name %q", id, other, name)
		}
		names[name] = id
		back, ok := reducerFromDisplayName(name)
		if !ok || back != id {
			t.Fatalf("reducerFromDisplayName(%q) = %q, %v, want %q", name, back, ok, id)
		}
	}
	for _, name := range []string{"95th %", "Last *", "Total"} {
		if _, ok := reducerFromDisplayName(name); !ok {
			t.Fatalf("reducerFromDisplayName(%q) is missing", name)
		}
	}
	for _, name := range []string{"", "100th %", "0th %", "Name", "last"} {
		if id, ok := reducerFromDisplayName(name); ok {
			t.Fatalf("reducerFromDisplayName(%q) = %q, want none", name, id)
		}
	}
}

func TestReducerLegendValuesAndSortUseTheSharedList(t *testing.T) {
	d, line := timeseriesLegend(t, `{"displayMode":"table","calcs":["p95","variance","allIsNull","uniqueValues","countAll"],"sortBy":"95th %"}`)
	requireImported(t, line)
	if want := []string{"p95", "variance", "allIsNull", "uniqueValues", "countAll"}; !slices.Equal(d.LegendValues, want) {
		t.Fatalf("legendValues = %v, want %v", d.LegendValues, want)
	}
	if d.LegendSortBy != "p95" {
		t.Fatalf("legendSortBy = %q, want p95", d.LegendSortBy)
	}

	d, line = timeseriesLegend(t, `{"displayMode":"table","calcs":["allZeros","p100"],"sortBy":"All nulls"}`)
	if line.Status != statusPartial || !slices.Contains(line.Paths, "options.legend.calcs") || !slices.Contains(line.Paths, "options.legend.sortBy") {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
	if len(d.LegendValues) != 0 || d.LegendSortBy != "" {
		t.Fatalf("legend = %+v, want no values and no sort", d)
	}
}
