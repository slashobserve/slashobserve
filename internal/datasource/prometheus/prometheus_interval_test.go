package prometheus_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
)

func TestInstantQueryIntervalFollowsTheMinimumInterval(t *testing.T) {
	for _, tc := range []struct {
		name        string
		minInterval time.Duration
		want        string
	}{
		{"no minimum keeps the scrape interval floor", 0, "rate(up[15s])"},
		{"a larger minimum wins", 5 * time.Minute, "rate(up[300s])"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				got = r.Form.Get("query")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
			}))
			t.Cleanup(server.Close)
			ds, err := prometheus.New(prometheus.Options{URL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ds.Close() })

			now := time.Now()
			rng := datasource.TimeRange{From: now.Add(-time.Hour), To: now.Add(time.Minute)}
			q := datasource.Query{Text: "rate(up[$__interval])", MinInterval: tc.minInterval}
			if _, err := ds.QueryInstant(context.Background(), q, rng); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("query sent = %q, want %q", got, tc.want)
			}
		})
	}
}
