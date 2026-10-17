package spec_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestResolveTime(t *testing.T) {
	now := mustTime(t, "2026-10-21T15:04:05.678Z")
	cases := []struct {
		expr     string
		from, to string
	}{
		{"now", "2026-10-21T15:04:05.678Z", "2026-10-21T15:04:05.678Z"},
		{"now-5s", "2026-10-21T15:04:00.678Z", "2026-10-21T15:04:00.678Z"},
		{"now-5m", "2026-10-21T14:59:05.678Z", "2026-10-21T14:59:05.678Z"},
		{"now-2h", "2026-10-21T13:04:05.678Z", "2026-10-21T13:04:05.678Z"},
		{"now-1d", "2026-10-20T15:04:05.678Z", "2026-10-20T15:04:05.678Z"},
		{"now-1w", "2026-10-14T15:04:05.678Z", "2026-10-14T15:04:05.678Z"},
		{"now-1M", "2026-09-21T15:04:05.678Z", "2026-09-21T15:04:05.678Z"},
		{"now-1y", "2025-10-21T15:04:05.678Z", "2025-10-21T15:04:05.678Z"},
		{"now+1h", "2026-10-21T16:04:05.678Z", "2026-10-21T16:04:05.678Z"},
		{"now/s", "2026-10-21T15:04:05Z", "2026-10-21T15:04:05.999Z"},
		{"now/m", "2026-10-21T15:04:00Z", "2026-10-21T15:04:59.999Z"},
		{"now/h", "2026-10-21T15:00:00Z", "2026-10-21T15:59:59.999Z"},
		{"now/d", "2026-10-21T00:00:00Z", "2026-10-21T23:59:59.999Z"},
		{"now/w", "2026-10-19T00:00:00Z", "2026-10-25T23:59:59.999Z"},
		{"now/M", "2026-10-01T00:00:00Z", "2026-10-31T23:59:59.999Z"},
		{"now/y", "2026-01-01T00:00:00Z", "2026-12-31T23:59:59.999Z"},
		{"now-1d/d", "2026-10-20T00:00:00Z", "2026-10-20T23:59:59.999Z"},
		{"2026-01-02T03:04:05+02:00", "2026-01-02T01:04:05Z", "2026-01-02T01:04:05Z"},
		{"1700000000000", "2023-11-14T22:13:20Z", "2023-11-14T22:13:20Z"},
	}
	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			for _, roundUp := range []bool{false, true} {
				want := c.from
				if roundUp {
					want = c.to
				}
				got, err := spec.ResolveTime(c.expr, now, roundUp)
				if err != nil {
					t.Fatal(err)
				}
				if !got.Equal(mustTime(t, want)) {
					t.Errorf("roundUp=%v: got %s, want %s", roundUp, got.Format(time.RFC3339Nano), want)
				}
			}
		})
	}
}

func TestResolveTimeMonthAndYearClamp(t *testing.T) {
	cases := []struct{ now, expr, want string }{
		{"2026-03-31T12:00:00Z", "now-1M", "2026-02-28T12:00:00Z"},
		{"2028-03-31T12:00:00Z", "now-1M", "2028-02-29T12:00:00Z"},
		{"2026-03-31T12:00:00Z", "now+1M", "2026-04-30T12:00:00Z"},
		{"2026-01-31T12:00:00Z", "now+1M", "2026-02-28T12:00:00Z"},
		{"2028-02-29T12:00:00Z", "now+1y", "2029-02-28T12:00:00Z"},
		{"2028-02-29T12:00:00Z", "now-1y", "2027-02-28T12:00:00Z"},
		{"2028-02-29T12:00:00Z", "now+4y", "2032-02-29T12:00:00Z"},
		{"2026-03-31T12:00:00.5Z", "now-1M", "2026-02-28T12:00:00.5Z"},
	}
	for _, c := range cases {
		t.Run(c.now+" "+c.expr, func(t *testing.T) {
			got, err := spec.ResolveTime(c.expr, mustTime(t, c.now), false)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(mustTime(t, c.want)) {
				t.Fatalf("got %s, want %s", got.Format(time.RFC3339Nano), c.want)
			}
		})
	}
}

func TestResolveTimeErrors(t *testing.T) {
	now := mustTime(t, "2026-10-21T15:04:05Z")
	for _, expr := range []string{
		"", "yesterday", "now-", "now+1", "now-1x", "now/x", "now-1d/", "NOW",
		"2026-13-45T00:00:00Z", "0000-01-01T00:00:00Z", "0001-01-01T00:00:00Z",
		"now-99999999999s", "now-2147483648s", "now-10000y", "now-2147483647y", "now+2147483647d",
		"12345678901234567", "now-9999999999999d",
	} {
		t.Run(expr, func(t *testing.T) {
			if got, err := spec.ResolveTime(expr, now, false); err == nil {
				t.Fatalf("got %s, want an error", got)
			}
		})
	}
}

func TestResolveTimeRangeOrder(t *testing.T) {
	now := mustTime(t, "2026-10-21T15:04:05Z")
	cases := []struct {
		from, to string
		ok       bool
	}{
		{"now-1h", "now", true},
		{"now", "now", false},
		{"now", "now-1h", false},
		{"now/d", "now/d", true},
		{"now/d", "now-1d/d", false},
		{"now/w", "now-1w/w", false},
		{"bogus", "now", false},
		{"now-1h", "bogus", false},
	}
	for _, c := range cases {
		t.Run(c.from+".."+c.to, func(t *testing.T) {
			_, _, err := spec.ResolveTimeRange(spec.TimeRange{From: c.from, To: c.to}, now)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v, want ok=%v", err, c.ok)
			}
		})
	}
}

func TestParsedTimeRangesResolve(t *testing.T) {
	minimal := string(readExample(t, "minimal.json"))
	parse := func(from, to string) error {
		data := strings.Replace(minimal, `"timeRange": { "from": "now-15m", "to": "now" }`, fmt.Sprintf(`"timeRange": { "from": %q, "to": %q }`, from, to), 1)
		_, err := spec.Parse([]byte(data))
		return err
	}
	nows := []time.Time{
		mustTime(t, "2026-03-31T12:00:00Z"),
		mustTime(t, "2026-01-01T00:00:00Z"),
		mustTime(t, "2028-02-29T23:59:59Z"),
		mustTime(t, "2026-10-19T12:00:00Z"),
	}
	accepted := [][2]string{
		{"now-15m", "now"}, {"now-6h", "now"}, {"now-7d", "now"}, {"now-1M", "now"}, {"now-1y", "now"},
		{"now-1h", "now+1h"}, {"now-30d", "now-1d"}, {"now-2M", "now-1M"}, {"now-1y", "now-1M"},
		{"now-1d/d", "now"}, {"now/d", "now/d"}, {"now/w", "now/w"}, {"now/M", "now/M"}, {"now/y", "now/y"},
		{"now-1d/d", "now-1d/d"}, {"now-1w/w", "now-1w/w"}, {"now-1M/M", "now-1M/M"}, {"now-7d/d", "now-1d/d"},
		{"2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"},
	}
	for _, r := range accepted {
		t.Run(r[0]+".."+r[1], func(t *testing.T) {
			if err := parse(r[0], r[1]); err != nil {
				t.Fatalf("Parse: %v", err)
			}
			for _, now := range nows {
				if _, _, err := spec.ResolveTimeRange(spec.TimeRange{From: r[0], To: r[1]}, now); err != nil {
					t.Errorf("now=%s: %v", now.Format(time.RFC3339), err)
				}
			}
		})
	}
	for _, r := range [][2]string{{"now/d", "now-1d/d"}, {"now/w", "now-1w/w"}, {"now", "now"}, {"now-1d", "now-1M"}} {
		t.Run("rejected "+r[0]+".."+r[1], func(t *testing.T) {
			if err := parse(r[0], r[1]); err == nil {
				t.Fatal("Parse accepted the range")
			}
			for _, now := range nows {
				if _, _, err := spec.ResolveTimeRange(spec.TimeRange{From: r[0], To: r[1]}, now); err == nil {
					t.Errorf("now=%s: resolved a range Parse rejects", now.Format(time.RFC3339))
				}
			}
		})
	}
}
