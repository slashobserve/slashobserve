package prometheus_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestGroupByJoinsAggregations(t *testing.T) {
	tests := []struct {
		name string
		text string
		keys []string
		want string
	}{
		{"an aggregation without grouping gains by", `sum(rate(x[5m]))`, []string{"job"}, `sum by (job) (rate(x[5m]))`},
		{"keys are added after the existing grouping", `sum by (instance) (x)`, []string{"job"}, `sum by (instance, job) (x)`},
		{"a key already in the grouping is not doubled", `sum by (job) (x)`, []string{"job"}, `sum by (job) (x)`},
		{"several keys keep their order", `sum(x)`, []string{"job", "instance"}, `sum by (job, instance) (x)`},
		{"some keys already present", `avg by (job, pod) (x)`, []string{"job", "instance"}, `avg by (job, pod, instance) (x)`},
		{"a without aggregation gains the keys in its list", `sum without (pod) (x)`, []string{"job"}, `sum without (pod, job) (x)`},
		{"a key already in a without list is not doubled", `sum without (pod, job) (x)`, []string{"job"}, `sum without (pod, job) (x)`},
		{"a query without aggregation is left alone", `rate(x[5m])`, []string{"job"}, `rate(x[5m])`},
		{"every nested aggregation gains the keys", `sum(max by (pod) (x))`, []string{"job"}, `sum by (job) (max by (pod, job) (x))`},
		{"a without aggregation inside another gains the keys", `sum(max without (pod) (x))`, []string{"job"}, `sum by (job) (max without (pod, job) (x))`},
		{"topk and its inner aggregation", `topk(5, sum by (pod) (x))`, []string{"job"}, `topk by (job) (5, sum by (pod, job) (x))`},
		{"count_values", `count_values("v", x)`, []string{"job"}, `count_values by (job) ("v", x)`},
		{"both sides of a binary operation", `sum(a) / sum(b)`, []string{"job"}, `sum by (job) (a) / sum by (job) (b)`},
		{"a binary operation with on keeps its matching", `sum by (pod) (a) / on(pod) sum by (pod) (b)`, []string{"job"}, `sum by (pod, job) (a) / on (pod) sum by (pod, job) (b)`},
		{"an aggregation in a subquery", `max_over_time(sum(x)[1h:5m])`, []string{"job"}, `max_over_time(sum by (job) (x)[1h:5m])`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds, rec := recordingDS(t)
			now := time.Now()
			r := datasource.TimeRange{From: now.Add(-time.Hour), To: now}
			if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: tc.text, GroupBy: tc.keys}, r); err != nil {
				t.Fatalf("QueryInstant: %v", err)
			}
			if got := rec.to("/api/v1/query")[0].form.Get("query"); got != tc.want {
				t.Errorf("query = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGroupByWithoutKeysSendsTheTextUnchanged(t *testing.T) {
	ds, rec := recordingDS(t)
	now := time.Now()
	r := datasource.TimeRange{From: now.Add(-time.Hour), To: now}
	if _, err := ds.QueryRange(context.Background(), datasource.Query{Text: `sum(x)   `}, r, time.Minute); err != nil {
		t.Fatal(err)
	}
	if got := rec.to("/api/v1/query_range")[0].form.Get("query"); got != `sum(x)   ` {
		t.Errorf("query = %q, want it unchanged", got)
	}
}

func TestGroupByAndFiltersApplyTogether(t *testing.T) {
	ds, rec := recordingDS(t)
	now := time.Now()
	r := datasource.TimeRange{From: now.Add(-time.Hour), To: now}
	q := datasource.Query{
		Text:    `sum(rate(x[5m]))`,
		Filters: []datasource.AdhocFilter{{Key: "job", Op: "=", Value: "api"}},
		GroupBy: []string{"pod"},
	}
	if _, err := ds.QueryRange(context.Background(), q, r, time.Minute); err != nil {
		t.Fatal(err)
	}
	want := `sum by (pod) (rate(x{job="api"}[5m]))`
	if got := rec.to("/api/v1/query_range")[0].form.Get("query"); got != want {
		t.Errorf("query = %q, want %q", got, want)
	}
}

func TestGroupByOnAnUnparsableQueryIsInvalid(t *testing.T) {
	ds, rec := recordingDS(t)
	now := time.Now()
	r := datasource.TimeRange{From: now.Add(-time.Hour), To: now}
	_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `sum(`, GroupBy: []string{"job"}}, r)
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery", err)
	}
	if n := len(rec.all()); n != 0 {
		t.Errorf("%d upstream requests, want none", n)
	}
}
