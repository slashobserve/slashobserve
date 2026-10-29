package prometheus_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
)

func matrixServer(t *testing.T, series, samples int, tail string) (*prometheus.Datasource, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		var b strings.Builder
		b.WriteString(`{"status":"success","data":{"resultType":"matrix","result":[`)
		for s := 0; s < series; s++ {
			if s > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"metric":{"__name__":"m","i":"%d"},"values":[`, s)
			for i := 0; i < samples; i++ {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `[%d,"1"]`, 1_700_000_000+i)
			}
			b.WriteString("]}")
		}
		b.WriteString(tail + "]}}")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	ds, err := prometheus.New(prometheus.Options{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds, &hits
}

func TestQueryRangeEnforcesSeriesAndPointLimits(t *testing.T) {
	now := time.Now()
	r := datasource.TimeRange{From: now.Add(-2 * time.Minute), To: now}
	cases := []struct {
		name            string
		series, samples int
		limits          datasource.Limits
		wantErr         bool
	}{
		{"series at the limit", 3, 2, datasource.Limits{MaxSeries: 3}, false},
		{"one series over the limit", 4, 2, datasource.Limits{MaxSeries: 3}, true},
		{"points at the limit", 3, 2, datasource.Limits{MaxPoints: 6}, false},
		{"one point over the limit", 3, 2, datasource.Limits{MaxPoints: 5}, true},
	}
	t.Run("limit is reached before the rest of the response is decoded", func(t *testing.T) {
		ds, _ := matrixServer(t, 4, 1, `,{"metric":{},"values":"not samples"}`)
		_, err := ds.QueryRange(context.Background(), datasource.Query{Text: "m", Limits: datasource.Limits{MaxSeries: 3}}, r, time.Minute)
		if !errors.Is(err, datasource.ErrLimitExceeded) {
			t.Fatalf("want ErrLimitExceeded, got %v", err)
		}
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, _ := matrixServer(t, tc.series, tc.samples, "")
			frames, err := ds.QueryRange(context.Background(), datasource.Query{Text: "m", Limits: tc.limits}, r, time.Minute)
			if tc.wantErr {
				if !errors.Is(err, datasource.ErrLimitExceeded) {
					t.Fatalf("want ErrLimitExceeded, got %v", err)
				}
				return
			}
			if err != nil || len(frames) != tc.series {
				t.Fatalf("got %d frames, err %v, want %d frames", len(frames), err, tc.series)
			}
		})
	}
}

func TestQueryRangeAcceptsExactlyTheMaximumNumberOfSteps(t *testing.T) {
	to := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		steps   int
		wantErr bool
	}{
		{"exactly 11000 steps", 11000, false},
		{"11001 steps", 11001, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, hits := matrixServer(t, 1, 1, "")
			r := datasource.TimeRange{From: to.Add(-time.Duration(tc.steps) * time.Minute), To: to}
			_, err := ds.QueryRange(context.Background(), datasource.Query{Text: "m"}, r, time.Minute)
			if tc.wantErr {
				if !errors.Is(err, datasource.ErrLimitExceeded) || hits.Load() != 0 {
					t.Fatalf("want ErrLimitExceeded before any request, got %v after %d requests", err, hits.Load())
				}
				return
			}
			if err != nil {
				t.Fatalf("QueryRange: %v", err)
			}
		})
	}
}
