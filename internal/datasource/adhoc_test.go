package datasource_test

import (
	"reflect"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func filter(key string) datasource.AdhocFilter {
	return datasource.AdhocFilter{Key: key, Op: "=", Value: "x"}
}

func orFilter(key string) datasource.AdhocFilter {
	f := filter(key)
	f.Condition = "or"
	return f
}

func scoped(f datasource.AdhocFilter, scope int) datasource.AdhocFilter {
	f.Scope = scope
	return f
}

func TestIsAdhocNumber(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"5", true}, {"-5", true}, {"5.25", true}, {"1e3", true}, {"-2.5E-3", true},
		{"", false}, {".5", false}, {"5.", false}, {"NaN", false}, {"inf", false}, {"0x10", false}, {"1_000", false}, {"5 ", false}, {"1e", false}, {"+5", false},
	}
	for _, tt := range tests {
		if got := datasource.IsAdhocNumber(tt.text); got != tt.want {
			t.Errorf("IsAdhocNumber(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestAdhocScopesSplitAtOrWithinEachScope(t *testing.T) {
	a, b, c, d := filter("a"), filter("b"), orFilter("c"), scoped(filter("d"), 1)
	tests := []struct {
		name    string
		filters []datasource.AdhocFilter
		want    []datasource.AdhocGroups
	}{
		{"no filters", nil, []datasource.AdhocGroups{}},
		{"and extends the group", []datasource.AdhocFilter{a, b}, []datasource.AdhocGroups{{{a, b}}}},
		{"or starts a group", []datasource.AdhocFilter{a, b, c}, []datasource.AdhocGroups{{{a, b}, {c}}}},
		{"a leading or is still the first group", []datasource.AdhocFilter{c, a}, []datasource.AdhocGroups{{{c, a}}}},
		{"scopes stay apart in order of first appearance", []datasource.AdhocFilter{a, d, b}, []datasource.AdhocGroups{{{a, b}}, {{d}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datasource.AdhocScopes(tt.filters); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("AdhocScopes = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAndAdhocKeepsPlainListsFlatAndScopesListsWithOr(t *testing.T) {
	a, b, c := filter("a"), filter("b"), orFilter("c")
	tests := []struct {
		name  string
		lists [][]datasource.AdhocFilter
		want  []datasource.AdhocFilter
	}{
		{"plain lists join into one scope", [][]datasource.AdhocFilter{{a}, {b}}, []datasource.AdhocFilter{a, b}},
		{"an empty list is skipped", [][]datasource.AdhocFilter{nil, {a}, nil}, []datasource.AdhocFilter{a}},
		{"nothing", nil, nil},
		{"a list with or gets its own scope next to a plain one", [][]datasource.AdhocFilter{{a}, {b, c}}, []datasource.AdhocFilter{a, scoped(b, 1), scoped(c, 1)}},
		{"each list gets a scope in order", [][]datasource.AdhocFilter{{b, c}, {a}, {b, c}}, []datasource.AdhocFilter{b, c, scoped(a, 1), scoped(b, 2), scoped(c, 2)}},
		{"an earlier result keeps its scopes apart", [][]datasource.AdhocFilter{{a, scoped(b, 1), scoped(c, 1)}, {a}}, []datasource.AdhocFilter{a, scoped(b, 1), scoped(c, 1), scoped(a, 2)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datasource.AndAdhoc(tt.lists...); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("AndAdhoc = %v, want %v", got, tt.want)
			}
		})
	}
}
