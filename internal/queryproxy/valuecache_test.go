package queryproxy

import (
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func valueQuery(vars ...datasource.Variable) datasource.Query {
	return datasource.Query{Text: `label_values(up{job="$job"}, pod)`, Variables: vars}
}

func rangeAt(from, to time.Time) datasource.TimeRange {
	return datasource.TimeRange{From: from, To: to}
}

func TestValueKeyIncludesOnlyReferencedVariables(t *testing.T) {
	rng := testRange()
	job := datasource.Variable{Name: "job", Values: []string{"api"}}
	base := valueKey("prom", valueQuery(job, datasource.Variable{Name: "env", Values: []string{"prod"}}), rng)

	if got := valueKey("prom", valueQuery(job, datasource.Variable{Name: "env", Values: []string{"dev"}}), rng); got != base {
		t.Error("an unreferenced variable changed the key")
	}
	if got := valueKey("prom", valueQuery(job), rng); got != base {
		t.Error("dropping an unreferenced variable changed the key")
	}
	if got := valueKey("prom", valueQuery(datasource.Variable{Name: "job", Values: []string{"web"}}), rng); got == base {
		t.Error("a referenced variable's value did not change the key")
	}
	if got := valueKey("prom", valueQuery(datasource.Variable{Name: "job", All: true}), rng); got == base {
		t.Error("a referenced variable's All flag did not change the key")
	}
}

func TestValueKeySeparatesValueBoundaries(t *testing.T) {
	rng := testRange()
	one := valueKey("prom", valueQuery(datasource.Variable{Name: "job", Values: []string{"ab"}}), rng)
	two := valueKey("prom", valueQuery(datasource.Variable{Name: "job", Values: []string{"a", "b"}}), rng)
	reversed := valueKey("prom", valueQuery(datasource.Variable{Name: "job", Values: []string{"b", "a"}}), rng)
	ordered := valueKey("prom", valueQuery(datasource.Variable{Name: "job", Values: []string{"a", "b"}}), rng)
	if one == two {
		t.Error(`["ab"] and ["a","b"] share a key`)
	}
	if reversed == ordered {
		t.Error("value order does not change the key")
	}
}

func TestValueKeyDependsOnDatasourceAndQueryText(t *testing.T) {
	rng := testRange()
	q := valueQuery(datasource.Variable{Name: "job", Values: []string{"api"}})
	base := valueKey("prom", q, rng)
	if valueKey("other", q, rng) == base {
		t.Error("the datasource uid does not change the key")
	}
	changed := q
	changed.Text += " "
	if valueKey("prom", changed, rng) == base {
		t.Error("the query text does not change the key")
	}
}

func TestValueKeyFloorsTheRangeToTenSeconds(t *testing.T) {
	q := valueQuery(datasource.Variable{Name: "job", Values: []string{"api"}})
	start := time.Date(2026, 10, 18, 10, 0, 10, 0, time.UTC)
	base := valueKey("prom", q, rangeAt(start, start.Add(time.Hour)))

	if valueKey("prom", q, rangeAt(start.Add(9999*time.Millisecond), start.Add(time.Hour+9*time.Second))) != base {
		t.Error("times within the same 10s bucket produced different keys")
	}
	if valueKey("prom", q, rangeAt(start.Add(10*time.Second), start.Add(time.Hour))) == base {
		t.Error("a different 10s bucket for from shares the key")
	}
	if valueKey("prom", q, rangeAt(start, start.Add(time.Hour+10*time.Second))) == base {
		t.Error("a different 10s bucket for to shares the key")
	}
	if valueKey("prom", q, rangeAt(start.Add(-time.Nanosecond), start.Add(time.Hour))) == base {
		t.Error("a time just below the bucket boundary shares the key")
	}
}

func TestValueKeyHashesTheUserOnlyForQueriesThatReferenceIt(t *testing.T) {
	rng := testRange()
	ada := datasource.QueryUser{ID: "u1", Login: "ada@example.com", Email: "ada@example.com", Name: "Ada"}
	bob := datasource.QueryUser{ID: "u2", Login: "bob@example.com", Email: "bob@example.com", Name: "Bob"}
	withUser := func(text string, u datasource.QueryUser) datasource.Query {
		return datasource.Query{Text: text, User: u}
	}

	const userQuery = `label_values(up{who="${__user.login}"}, pod)`
	if valueKey("prom", withUser(userQuery, ada), rng) == valueKey("prom", withUser(userQuery, bob), rng) {
		t.Error("two users share a key for a query that references the user")
	}
	first := valueKey("prom", withUser(userQuery, ada), rng)
	if again := valueKey("prom", withUser(userQuery, ada), rng); again != first {
		t.Error("the same user gets different keys")
	}
	for _, field := range []datasource.QueryUser{{ID: "u1"}, {Login: "ada@example.com"}, {Email: "ada@example.com"}, {Name: "Ada"}} {
		if valueKey("prom", withUser(userQuery, field), rng) == valueKey("prom", withUser(userQuery, datasource.QueryUser{}), rng) {
			t.Errorf("user field %+v does not change the key", field)
		}
	}

	const plainQuery = `label_values(up, pod)`
	if valueKey("prom", withUser(plainQuery, ada), rng) != valueKey("prom", withUser(plainQuery, bob), rng) {
		t.Error("a query without user references is keyed by the user")
	}
}

func TestValueKeyHashesTheDashboardTitleOnlyForQueriesThatReferenceIt(t *testing.T) {
	rng := testRange()
	withTitle := func(text, title string) datasource.Query {
		return datasource.Query{Text: text, Dashboard: title}
	}
	const titled = `label_values(up{dash="$__dashboard"}, pod)`
	if valueKey("prom", withTitle(titled, "one"), rng) == valueKey("prom", withTitle(titled, "two"), rng) {
		t.Error("the dashboard title does not change the key of a query that references it")
	}
	const plain = `label_values(up, pod)`
	if valueKey("prom", withTitle(plain, "one"), rng) != valueKey("prom", withTitle(plain, "two"), rng) {
		t.Error("a query without a dashboard reference is keyed by the title")
	}
}

func TestDashboardUsesUser(t *testing.T) {
	user := `up{who="${__user.login}"}`
	tests := []struct {
		name string
		d    *spec.Dashboard
		want bool
	}{
		{"nil", nil, false},
		{"no references", dashboardOf([]spec.Panel{panelOf("p", spec.Timeseries, "prom", `up{dash="$__dashboard"}`)}), false},
		{"panel query", dashboardOf([]spec.Panel{panelOf("p", spec.Timeseries, "prom", user)}), true},
		{"row child query", dashboardOf([]spec.Panel{{ID: "r", Type: spec.Row, Panels: []spec.Panel{panelOf("c", spec.Stat, "prom", user)}}}), true},
		{"second query of a panel", dashboardOf([]spec.Panel{{ID: "p", Type: spec.Timeseries, Datasource: "prom", Queries: []spec.Query{{Query: "up"}, {Query: user}}}}), true},
		{"query variable", dashboardOf(nil, queryVariable("v", "prom", `label_values(`+user+`, job)`)), true},
		{"custom variable option is not a query", dashboardOf(nil, customVariable("v", user)), false},
		{"bare user is not a user reference", dashboardOf([]spec.Panel{panelOf("p", spec.Timeseries, "prom", `up{a="$__user"}`)}), false},
		{"user inside conditional all", dashboardOf([]spec.Panel{panelOf("p", spec.Timeseries, "ch", `$__conditionalAll(who = ${__user.login}, $svc)`)}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DashboardUsesUser(tt.d); got != tt.want {
				t.Fatalf("DashboardUsesUser = %v, want %v", got, tt.want)
			}
		})
	}
}
