package proxyhttp

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logvolume"
)

func requireNote(t *testing.T, got []audit.Query, want audit.Query) {
	t.Helper()
	if len(got) != 1 || got[0] != want {
		t.Fatalf("noted %+v, want exactly %+v", got, want)
	}
}

func TestEveryQueryPathNotesTheQueryItRanWithItsWindowAndRows(t *testing.T) {
	hour := datasource.TimeRange{From: volumeStart, To: volumeStart.Add(time.Hour)}
	t.Run("facets", func(t *testing.T) {
		hn, lokiDS := newFacetsHarness(t)
		lokiDS.onFacets = func(context.Context, datasource.FacetRequest) (datasource.Facets, error) {
			return datasource.Facets{Fields: []datasource.FacetField{
				{Name: "a", Values: []datasource.FacetValue{{Value: "x", Count: 1}, {Value: "y", Count: 1}}},
				{Name: "b", Values: []datasource.FacetValue{{Value: "z", Count: 1}}},
			}}, nil
		}
		decodeFacets(t, hn.facets("logs", facetsBody(`{app="shop"}`)))
		requireNote(t, hn.noted(), audit.Query{Datasource: "logs", Text: `{app="shop"}`, From: hour.From, To: hour.To, Rows: 3})
	})
	t.Run("facets that fail", func(t *testing.T) {
		hn, lokiDS := newFacetsHarness(t)
		lokiDS.onFacets = func(context.Context, datasource.FacetRequest) (datasource.Facets, error) {
			return datasource.Facets{}, errors.New("boom")
		}
		if rec := hn.facets("logs", facetsBody(`{app="shop"}`)); rec.Code == http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		requireNote(t, hn.noted(), audit.Query{Datasource: "logs", Text: `{app="shop"}`, From: hour.From, To: hour.To})
	})
	t.Run("context", func(t *testing.T) {
		hn, logs := newContextHarness(t)
		logs.onLines = func(req datasource.LinesRequest) ([]datasource.Frame, error) {
			if req.Direction == datasource.Backward {
				return contextLines(false, -1, -2), nil
			}
			return contextLines(true, 1, 2, 3), nil
		}
		decodeContext(t, hn.logContext("logs", map[string]any{"query": `{service_name="checkout"}`, "anchor": contextAnchor, "before": 2, "after": 3, "window_ms": 60000}))
		noted := hn.noted()
		if len(noted) != 2 || noted[0].Datasource != "logs" || noted[0].Text != `{service_name="checkout"}` || noted[0].Rows+noted[1].Rows != 5 {
			t.Fatalf("noted %+v, want the query once per side with 5 lines in all", noted)
		}
	})
	t.Run("explore", func(t *testing.T) {
		hn, recorder := newExploreHarness(t)
		recorder.onInstant = nil
		if rec := hn.explore(auth.KindUser, "prom", instantBody("up")); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
		}
		noted := hn.noted()
		if len(noted) != 1 || noted[0].Datasource != "prom" || noted[0].Text != "up" || noted[0].From.IsZero() || !noted[0].To.After(noted[0].From) {
			t.Fatalf("noted %+v", noted)
		}
	})
	t.Run("a paged explore", func(t *testing.T) {
		hn, _ := newPageHarness(t)
		if rec := hn.explore(auth.KindUser, "logs", pagedBody(100)); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
		}
		noted := hn.noted()
		if len(noted) != 1 || noted[0].Datasource != "logs" || noted[0].Text != pagedQuery || noted[0].Rows != 3 {
			t.Fatalf("noted %+v", noted)
		}
	})
	t.Run("trace", func(t *testing.T) {
		hn, _ := newTraceHarness(t)
		if rec := hn.trace(auth.KindServiceAccount, "tempo", traceFixtureID, "?from=now-6h&to=now"); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		noted := hn.noted()
		if len(noted) != 1 || noted[0].Datasource != "tempo" || noted[0].Text != "trace "+traceFixtureID || noted[0].Rows != datasource.TraceSpanBudget {
			t.Fatalf("noted %+v", noted)
		}
	})
	t.Run("trace compare", func(t *testing.T) {
		hn, ds := newCompareHarness(t)
		ds.onAttributes = func(_ context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
			return attributeSetsFor(req.IDs, func(int) []datasource.TraceAttribute { return nil }), nil
		}
		selected, baseline := compareIDs('a', 2), compareIDs('b', 3)
		if rec := hn.traceCompare("tempo", traceCompareBody(selected, baseline)); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
		}
		want := []audit.Query{
			{Datasource: "tempo", Text: "trace attributes " + strings.Join(selected, ","), From: hour.From, To: hour.To, Rows: 2},
			{Datasource: "tempo", Text: "trace attributes " + strings.Join(baseline, ","), From: hour.From, To: hour.To, Rows: 3},
		}
		if got := hn.noted(); !slices.Equal(got, want) {
			t.Fatalf("noted %+v, want %+v", got, want)
		}
	})
	t.Run("trace stats", func(t *testing.T) {
		hn, tempoDS := newTraceHarness(t)
		tempoDS.onStats = func(_ context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
			return datasource.TraceStats{Step: req.Step, Buckets: []datasource.TraceBucket{{Start: req.Range.From}, {Start: req.Range.From.Add(req.Step)}}}, nil
		}
		if rec := hn.traceStats("tempo", traceStatsBody(`{ status = error }`)); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
		}
		noted := hn.noted()
		if len(noted) != 1 || noted[0].Datasource != "tempo" || noted[0].Text != `{ status = error }` || noted[0].Rows != 2 || !noted[0].From.Equal(volumeStart) {
			t.Fatalf("noted %+v", noted)
		}
	})
	t.Run("trace logs", func(t *testing.T) {
		hn, lokiDS, _ := newTraceLogsHarness(t)
		lokiDS.answer = func(datasource.TraceLogsRequest) ([]datasource.Frame, error) { return logFrames(), nil }
		body := traceLogsBody()
		body["span_id"] = spanID
		if rec := hn.traceLogs("logs", body); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
		}
		noted := hn.noted()
		want := "trace logs " + traceID + " span " + spanID
		if len(noted) != 1 || noted[0].Datasource != "logs" || noted[0].Text != want || noted[0].Rows != datasource.FrameRows(logFrames()) {
			t.Fatalf("noted %+v, want text %q", noted, want)
		}
	})
	t.Run("volume", func(t *testing.T) {
		hn, search := newVolumeCapabilityHarness(t)
		search.onVolume = func(req datasource.VolumeRequest) (*logvolume.Histogram, error) {
			return logvolume.New(req.Range.From, req.Step, req.Buckets()), nil
		}
		decodeVolume(t, hn.volume("search", volumeBody(`service.name:checkout`)))
		requireNote(t, hn.noted(), audit.Query{Datasource: "search", Text: "service.name:checkout", From: hour.From, To: hour.To, Rows: 60})
	})
	t.Run("log patterns", func(t *testing.T) {
		hn, logs, _ := newPatternsHarness(t)
		logs.onPatterns = func(datasource.PatternRequest) ([]datasource.NativePattern, error) {
			return []datasource.NativePattern{{Pattern: "a", Samples: []datasource.PatternSample{{At: volumeStart, Count: 5}}}}, nil
		}
		body := patternsBody(`{app="shop"}`)
		body["sample"] = 100
		decodePatterns(t, hn.logPatterns("logs", body))
		noted := hn.noted()
		if len(noted) == 0 || noted[0].Datasource != "logs" || noted[0].Text != `{app="shop"}` || noted[0].Rows != 1 {
			t.Fatalf("noted %+v", noted)
		}
	})
	t.Run("draft preview", func(t *testing.T) {
		hn := newPreviewHarness(t)
		if rec := hn.previewAs(auth.KindUser, "prom", nil); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
		}
		noted := hn.noted()
		if len(noted) != 1 || noted[0].Datasource != "prom" || noted[0].Text != `up{job="$job"}` {
			t.Fatalf("noted %+v", noted)
		}
	})
}

func TestAnUnknownTraceIdOnAVisibleDatasourceIsAFailedReadNotADenial(t *testing.T) {
	hn, tempoDS := newTraceHarness(t)
	tempoDS.onTrace = func(context.Context, string, datasource.TimeRange, int) (datasource.Frame, error) {
		return datasource.Frame{}, datasource.Errorf(datasource.ErrNotFound, "trace not found")
	}
	rec := hn.trace(auth.KindUser, "tempo", traceFixtureID, "?from=now-6h&to=now")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	got := hn.concluded()
	if got == nil || got.Outcome != audit.OutcomeFailed || got.Target != (audit.Target{Kind: "datasource", ID: "tempo"}) {
		t.Fatalf("concluded %+v, want a failed read of datasource tempo", got)
	}
}
