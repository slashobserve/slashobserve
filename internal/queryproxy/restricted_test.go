package queryproxy

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestDatasourceRefsReachEveryDatasourceTheDashboardCanQuery(t *testing.T) {
	d := &spec.Dashboard{
		Variables: []spec.Variable{
			{Name: "host", Datasource: "labels"},
			{Name: "ds", Type: spec.DatasourceVariable, Default: &spec.Selection{Values: []string{"by-default"}},
				Options: []spec.VariableOption{spec.PlainOption("listed")}, StaticOptions: []spec.StaticOption{{Text: "Fixed", Value: "fixed"}}, Value: "chosen"},
			{Name: "plain", Value: "not-a-datasource", Options: []spec.VariableOption{spec.PlainOption("also-not")}},
		},
		Panels: []spec.Panel{
			{Datasource: "panel", Queries: []spec.Query{{Datasource: "query"}, {Datasource: "${ds}"}, {Datasource: "panel"}}},
			{Panels: []spec.Panel{{Datasource: "in-row"}}},
		},
	}
	got := DatasourceRefs(d)
	want := []string{"labels", "by-default", "listed", "fixed", "chosen", "panel", "query", "in-row"}
	if !slices.Equal(got, want) {
		t.Fatalf("refs = %v, want %v", got, want)
	}
	if DatasourceRefs(nil) != nil {
		t.Fatal("a nil dashboard has refs")
	}
}

func TestFirstRestrictedNamesTheFirstAdminsOnlyRef(t *testing.T) {
	adminsOnly := func(uid string) bool { return uid == "payroll" || uid == "audit" }
	if ds, ok := FirstRestricted([]string{"open", "audit", "payroll"}, adminsOnly); !ok || ds != "audit" {
		t.Fatalf("FirstRestricted = %q %v", ds, ok)
	}
	if _, ok := FirstRestricted([]string{"open"}, adminsOnly); ok {
		t.Fatal("an open datasource counted as restricted")
	}
}
