package auditsource

import (
	"bytes"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/filter"
)

func TestARepeatIsTheSameQueryOverTheSamePeriod(t *testing.T) {
	scope := audit.Scope{Method: "POST", Route: "/api/v1/datasources/:uid/query"}
	at := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	read := func(from, to time.Time, bounds int64) searchRequest {
		histogram := map[string]any{"field": TimeField, "fixed_interval": "1m", "extended_bounds": map[string]any{"min": bounds, "max": bounds + 1}}
		return searchRequest{
			window: filter.Window{From: from, To: to},
			clause: map[string]any{"term": map[string]any{"outcome": "denied"}},
			aggs:   map[string]any{"count": map[string]any{"date_histogram": histogram}},
		}
	}
	key := func(r searchRequest) []byte {
		raw, err := readKey(scope, r)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	last15 := func(now time.Time) searchRequest { return read(now.Add(-15*time.Minute), now, now.UnixMilli()) }
	first := key(last15(at.Add(time.Minute)))

	for name, tc := range map[string]struct {
		read searchRequest
		same bool
	}{
		"a refresh in the same 15 minutes":  {last15(at.Add(5 * time.Minute)), true},
		"a refresh in the next 15 minutes":  {last15(at.Add(16 * time.Minute)), false},
		"the same query since 1970":         {read(time.Unix(0, 0).UTC(), at.Add(time.Minute), 0), false},
		"the same query over the last hour": {read(at.Add(-time.Hour), at.Add(time.Minute), 0), false},
		"the same length on another day":    {last15(at.Add(-24*time.Hour + time.Minute)), false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := bytes.Equal(key(tc.read), first); got != tc.same {
				t.Fatalf("same key = %v, want %v", got, tc.same)
			}
		})
	}
}
