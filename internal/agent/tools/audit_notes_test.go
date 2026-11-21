package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func notedBy(t *testing.T, r *Registry, name, input string) []audit.Query {
	t.Helper()
	ctx := audit.Open(auth.WithPrincipal(reading(context.Background()), auth.Principal{Role: auth.RoleAdmin}), audit.Scope{})
	res, err := r.Call(ctx, name, []byte(input))
	if err != nil || res.IsError {
		t.Fatalf("%s: %#v, %v", name, res.Content, err)
	}
	return audit.Noted(ctx)
}

func TestEveryQueryToolNotesTheQueryItRanForTheAuditTrail(t *testing.T) {
	t.Run("query_range", func(t *testing.T) {
		ds := &fakeDS{}
		noted := notedBy(t, newTestRegistry(t, ds, nil), nameQueryRange, `{"datasource":"fake","query":"up","from":"now-6h","max_data_points":200.0}`)
		if len(noted) != 1 || noted[0].Datasource != fakeUID || noted[0].Text != "up" || noted[0].From.IsZero() || !noted[0].To.After(noted[0].From) {
			t.Fatalf("noted %+v", noted)
		}
	})
	t.Run("log_facets", func(t *testing.T) {
		ds := &facetFakeDS{fakeDS: &fakeDS{typ: "loki"}}
		ds.onFacets = func(context.Context, datasource.FacetRequest) (datasource.Facets, error) {
			return manyFacets(2, 3), nil
		}
		noted := notedBy(t, newFacetRegistry(t, ds), nameLogFacets, `{"datasource":"fake","query":"{app=\"shop\"}","from":"now-6h","to":"now"}`)
		if len(noted) != 1 || noted[0].Text != `{app="shop"}` || noted[0].Rows != 6 {
			t.Fatalf("noted %+v", noted)
		}
	})
	t.Run("logs_for_trace", func(t *testing.T) {
		logs := &traceLogFake{fakeDS: &fakeDS{typ: "loki"}}
		noted := notedBy(t, traceLogRegistry(t, logs), nameLogsForTrace, `{"datasource":"logs","trace_id":"`+traceID+`","span_id":"`+spanID+`","service":"checkout","from":"now-30m","to":"now"}`)
		want := "trace logs " + traceID + " span " + spanID + " service checkout"
		if len(noted) != 1 || noted[0].Datasource != logsUID || noted[0].Text != want || noted[0].Rows != datasource.FrameRows([]datasource.Frame{logTableFrame()}) {
			t.Fatalf("noted %+v, want text %q", noted, want)
		}
	})
	t.Run("log_context", func(t *testing.T) {
		ds := &fakeLineDS{fakeDS: &fakeDS{typ: "loki"}}
		ds.linesFn = func(req datasource.LinesRequest) ([]datasource.Frame, error) {
			if req.Direction == datasource.Backward {
				return []datasource.Frame{lokiLines(false, -2, -1)}, nil
			}
			return []datasource.Frame{lokiLines(false, 1)}, nil
		}
		noted := notedBy(t, newLineRegistry(t, ds), nameLogContext, contextInput(`,"before":3,"after":3`))
		if len(noted) != 2 || noted[0].Text != `{service_name="checkout"}` || noted[0].Rows+noted[1].Rows != 3 {
			t.Fatalf("noted %+v", noted)
		}
	})
	t.Run("get_trace", func(t *testing.T) {
		r, _ := newTraceRig(t)
		noted := notedBy(t, r, nameGetTrace, `{"datasource":"tempo","trace_id":"`+fixtureTraceID+`"}`)
		if len(noted) != 1 || noted[0].Datasource != "tempo" || noted[0].Text != "trace "+fixtureTraceID || noted[0].Rows != 30 {
			t.Fatalf("noted %+v", noted)
		}
	})
	t.Run("compare_traces", func(t *testing.T) {
		r, _ := newCompareRig(t)
		selected, baseline := compareTraceIDs('a', 2), compareTraceIDs('b', 3)
		noted := notedBy(t, r, nameCompareTraces, compareInput(selected, baseline, ""))
		if len(noted) != 2 || noted[0].Text != "trace attributes "+strings.Join(selected, ",") || noted[0].Rows != 2 || noted[1].Text != "trace attributes "+strings.Join(baseline, ",") || noted[1].Rows != 3 {
			t.Fatalf("noted %+v", noted)
		}
	})
	t.Run("sample_rows", func(t *testing.T) {
		noted := notedBy(t, newTestRegistry(t, &fakeDS{typ: "loki"}, nil), nameSampleRows, `{"datasource":"fake"}`)
		if len(noted) != 1 || noted[0].Datasource != fakeUID || !strings.HasPrefix(noted[0].Text, "sample ") {
			t.Fatalf("noted %+v", noted)
		}
	})
}
