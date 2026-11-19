package proxyhttp

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

type recordingLoki struct {
	mu          sync.Mutex
	sent        []url.Values
	inFlight    int
	maxInFlight int
}

func (f *recordingLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f.mu.Lock()
	f.sent = append(f.sent, q)
	f.inFlight++
	f.maxInFlight = max(f.maxInFlight, f.inFlight)
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()
	time.Sleep(5 * time.Millisecond)
	start, _ := time.Parse(time.RFC3339Nano, q.Get("start"))
	end, _ := time.Parse(time.RFC3339Nano, q.Get("end"))
	seconds, _ := strconv.ParseFloat(q.Get("step"), 64)
	var points [][2]any
	for at := start; !at.After(end); at = at.Add(time.Duration(seconds) * time.Second) {
		points = append(points, [2]any{at.Unix(), "1"})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
		"resultType": "matrix",
		"result":     []any{map[string]any{"metric": map[string]string{"level": "error"}, "values": points}},
	}})
}

func (f *recordingLoki) requests() ([]url.Values, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sent), f.maxInFlight
}

func newLokiVolumeHarness(t *testing.T) (*harness, *recordingLoki) {
	t.Helper()
	hn := newHarness(t)
	fake := &recordingLoki{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ds, err := loki.New(loki.Options{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	registry, err := datasource.NewRegistry(hn.h.logger,
		datasource.Instance{UID: "prom", Datasource: hn.ds},
		datasource.Instance{UID: "loki", Datasource: ds},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, fake
}

var lokiVolumeNow = time.Date(2026, 11, 19, 21, 36, 31, 456_000_000, time.UTC)

func lokiVolumeOver(span time.Duration) map[string]any {
	return map[string]any{
		"query": `{service_name="checkout"}`,
		"from":  strconv.FormatInt(lokiVolumeNow.Add(-span).UnixMilli(), 10),
		"to":    strconv.FormatInt(lokiVolumeNow.UnixMilli(), 10),
	}
}

func lokiCount(rangeMS string) string {
	return "sum by (level, lvl, severity_text, severity, detected_level) (count_over_time(\n{service_name=\"checkout\"}\n[" + rangeMS + "ms]))"
}

func lokiWindow(query, start, end, step string) url.Values {
	return url.Values{"query": {query}, "start": {start}, "end": {end}, "step": {step}}
}

func assertLokiRequests(t *testing.T, got, want []url.Values) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d requests to Loki, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !maps.EqualFunc(got[i], want[i], slices.Equal) {
			t.Fatalf("request %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestALokiVolumeOverAnHourIsOneRequestOverTheAlignedRange(t *testing.T) {
	hn, fake := newLokiVolumeHarness(t)
	body := decodeVolume(t, hn.volume("loki", lokiVolumeOver(time.Hour)))
	sent, _ := fake.requests()
	assertLokiRequests(t, sent, []url.Values{
		lokiWindow(lokiCount("60000"), "2026-11-19T20:36:00Z", "2026-11-19T21:37:00Z", "60"),
	})
	if body.Partial || body.StepMS != 60000 || len(body.Buckets) != 61 || body.Total != 61 {
		t.Fatalf("partial %v, step_ms %d, %d buckets, total %d", body.Partial, body.StepMS, len(body.Buckets), body.Total)
	}
}

func TestALokiVolumeOverADayIsTheSingleRequestItWasBeforeChunking(t *testing.T) {
	hn, fake := newLokiVolumeHarness(t)
	body := decodeVolume(t, hn.volume("loki", lokiVolumeOver(24*time.Hour)))
	sent, _ := fake.requests()
	assertLokiRequests(t, sent, []url.Values{
		lokiWindow(lokiCount("1440000"), "2026-11-18T21:36:00Z", "2026-11-19T22:00:00Z", "1440"),
	})
	if body.Partial || body.StepMS != 1440000 || len(body.Buckets) != 61 || body.Total != 61 {
		t.Fatalf("partial %v, step_ms %d, %d buckets, total %d", body.Partial, body.StepMS, len(body.Buckets), body.Total)
	}
	if body.Buckets[0].Start != "2026-11-18T21:36:00Z" || body.Buckets[60].Start != "2026-11-19T21:36:00Z" {
		t.Fatalf("buckets run from %s to %s", body.Buckets[0].Start, body.Buckets[60].Start)
	}
}

func TestALokiVolumeOverAWeekIsCountedInDaySizedRequestsOneAtATimeNewestFirst(t *testing.T) {
	hn, fake := newLokiVolumeHarness(t)
	body := decodeVolume(t, hn.volume("loki", lokiVolumeOver(7*24*time.Hour)))
	sent, maxInFlight := fake.requests()
	count := lokiCount("10080000")
	assertLokiRequests(t, sent, []url.Values{
		lokiWindow(count, "2026-11-19T00:00:00Z", "2026-11-19T22:24:00Z", "10080"),
		lokiWindow(count, "2026-11-17T22:48:00Z", "2026-11-18T21:12:00Z", "10080"),
		lokiWindow(count, "2026-11-16T21:36:00Z", "2026-11-17T20:00:00Z", "10080"),
		lokiWindow(count, "2026-11-15T20:24:00Z", "2026-11-16T18:48:00Z", "10080"),
		lokiWindow(count, "2026-11-14T19:12:00Z", "2026-11-15T17:36:00Z", "10080"),
		lokiWindow(count, "2026-11-13T18:00:00Z", "2026-11-14T16:24:00Z", "10080"),
		lokiWindow(count, "2026-11-12T19:36:00Z", "2026-11-13T15:12:00Z", "10080"),
	})
	if maxInFlight != 1 {
		t.Fatalf("%d requests in flight at once, want 1", maxInFlight)
	}
	if body.Partial || body.StepMS != 10080000 || len(body.Buckets) != 61 || body.Total != 61 {
		t.Fatalf("partial %v, step_ms %d, %d buckets, total %d", body.Partial, body.StepMS, len(body.Buckets), body.Total)
	}
	for i, b := range body.Buckets {
		if b.Error != 1 {
			t.Fatalf("bucket %d = %+v, want one error line in every bucket", i, b)
		}
	}
}
