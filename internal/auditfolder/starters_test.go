package auditfolder

import (
	"context"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type emptyTrail struct{}

func (emptyTrail) Rows(_ context.Context, q filter.Query, w filter.Window, page filter.Page) ([]trail.Event, error) {
	if _, err := q.Rows(w, page); err != nil {
		return nil, &trail.QueryRefused{Msg: err.Error()}
	}
	return nil, nil
}

func (emptyTrail) Count(_ context.Context, q filter.Query, w filter.Window, upTo int) (int64, error) {
	if _, err := q.Count(w, upTo); err != nil {
		return 0, &trail.QueryRefused{Msg: err.Error()}
	}
	return 0, nil
}

func (emptyTrail) ReadAggregation(_ context.Context, p trail.Prepared) (trail.Aggregation, error) {
	agg, err := p.Query.Aggregate(p.Window)
	if err != nil {
		return trail.Aggregation{}, &trail.QueryRefused{Msg: err.Error()}
	}
	return trail.Aggregation{Levels: agg.Levels, Metrics: agg.Metrics}, nil
}

func TestEveryStarterDashboardIsAValidSpecWhoseQueriesTheAuditDatasourceAnswers(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	inst, err := auditsource.Instance(emptyTrail{}, logger)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := datasource.NewRegistry(logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddBuiltIn(inst); err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(registry, logger)
	ctx := datasource.WithAccess(t.Context(), datasource.Access{System: true})

	starters, err := Starters()
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, st := range starters {
		titles = append(titles, st.Title)
		t.Run(st.Key, func(t *testing.T) {
			d, err := spec.Parse(st.Spec)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for _, p := range d.Panels {
				if p.Datasource != auditsource.UID {
					t.Errorf("panel %q reads %q, want the audit datasource", p.ID, p.Datasource)
				}
			}
			issues, err := runner.CheckSpec(ctx, d, time.Now(), datasource.QueryUser{})
			if err != nil {
				t.Fatal(err)
			}
			for _, issue := range issues {
				if issue.Code != queryproxy.CodeQueryEmpty {
					t.Errorf("%s: %s", issue.Path, issue.Message)
				}
			}
		})
	}
	want := []string{"Agent and MCP activity", "Audit overview", "Data access", "Sign-ins and access"}
	if slices.Sort(titles); !slices.Equal(titles, want) {
		t.Fatalf("starter titles = %q, want %q", titles, want)
	}
}
