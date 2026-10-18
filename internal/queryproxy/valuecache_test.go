package queryproxy

import (
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
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
