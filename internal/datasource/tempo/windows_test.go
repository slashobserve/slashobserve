package tempo_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/tempo"
)

type window struct{ start, end int64 }

type windowedTempo struct {
	mu      sync.Mutex
	windows []window
	slow    func(n int) bool
}

func (f *windowedTempo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start, _ := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
	end, _ := strconv.ParseInt(r.URL.Query().Get("end"), 10, 64)
	f.mu.Lock()
	f.windows = append(f.windows, window{start, end})
	n := len(f.windows)
	f.mu.Unlock()
	if f.slow != nil && f.slow(n) {
		<-r.Context().Done()
		return
	}
	at := time.Unix(end, 0).Add(-time.Minute).UnixNano()
	_, _ = fmt.Fprintf(w, `{"traces":[{"traceID":"%032x","rootServiceName":"checkout","rootTraceName":"GET /","startTimeUnixNano":"%d","durationMs":1}],"metrics":{"completedJobs":1,"totalJobs":1}}`, n, at)
}

func (f *windowedTempo) sent() []window {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.windows)
}

func serveWindows(t *testing.T, fake *windowedTempo) *tempo.Datasource {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ds, err := tempo.New(tempo.Options{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func TestABroadSearchAsksNewestFirstInGrowingWindowsUntilTheLimitIsMet(t *testing.T) {
	fake := &windowedTempo{}
	ds := serveWindows(t, fake)
	day := datasource.TimeRange{From: rangeEnd.Add(-24 * time.Hour), To: rangeEnd}
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{}`, Limits: datasource.Limits{MaxRows: 3, Timeout: time.Minute}}, day)
	if err != nil {
		t.Fatal(err)
	}
	end := rangeEnd.Unix()
	want := []window{
		{end - 3600, end + 1},
		{end - 3*3600, end - 3600 + 1},
		{end - 7*3600, end - 3*3600 + 1},
	}
	if got := fake.sent(); !slices.Equal(got, want) {
		t.Fatalf("windows = %v, want %v", got, want)
	}
	if len(frames) != 1 || frames[0].Rows() != 3 {
		t.Fatalf("frames = %+v, want one frame of 3 traces", frames)
	}
	if meta := frames[0].Meta; meta == nil || meta.Partial || meta.JobsDone != 3 || meta.JobsTotal != 3 {
		t.Fatalf("meta = %+v, want complete with the jobs of every window", meta)
	}
}

func TestASearchCoversTheWholeRangeWhenTheLimitIsNotMet(t *testing.T) {
	fake := &windowedTempo{}
	ds := serveWindows(t, fake)
	day := datasource.TimeRange{From: rangeEnd.Add(-24 * time.Hour), To: rangeEnd}
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{}`, Limits: datasource.Limits{MaxRows: 100, Timeout: time.Minute}}, day); err != nil {
		t.Fatal(err)
	}
	got := fake.sent()
	if len(got) != 5 || got[len(got)-1].start != day.From.Unix() {
		t.Fatalf("windows = %v, want 1, 2, 4, 8 and the last 9 hours down to the start", got)
	}
}

func TestASearchThatRunsOutOfTimeReturnsTheNewestTracesAsPartial(t *testing.T) {
	fake := &windowedTempo{slow: func(n int) bool { return n == 2 }}
	ds := serveWindows(t, fake)
	day := datasource.TimeRange{From: rangeEnd.Add(-24 * time.Hour), To: rangeEnd}
	started := time.Now()
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{}`, Limits: datasource.Limits{MaxRows: 10, Timeout: 2 * time.Second}}, day)
	if err != nil {
		t.Fatalf("err = %v, want the traces found before the time limit", err)
	}
	if took := time.Since(started); took > 1900*time.Millisecond {
		t.Fatalf("took %v, want an answer before the 2 s limit", took)
	}
	if len(frames) != 1 || frames[0].Rows() != 1 || frames[0].Meta == nil || !frames[0].Meta.Partial {
		t.Fatalf("frames = %+v, want the first window's trace marked partial", frames)
	}
}

func TestASearchWithNothingFoundBeforeTheTimeLimitIsAnEmptyPartialResult(t *testing.T) {
	fake := &windowedTempo{slow: func(int) bool { return true }}
	ds := serveWindows(t, fake)
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{}`, Limits: datasource.Limits{MaxRows: 10, Timeout: 1500 * time.Millisecond}}, lastHour)
	if err != nil {
		t.Fatalf("err = %v, want an empty partial result", err)
	}
	if len(frames) != 1 || frames[0].Rows() != 0 || frames[0].Meta == nil || !frames[0].Meta.Partial {
		t.Fatalf("frames = %+v, want one empty frame marked partial", frames)
	}
}

func TestAPageCutByTheTimeLimitOffersTheOlderTracesNext(t *testing.T) {
	fake := &windowedTempo{slow: func(n int) bool { return n == 2 }}
	ds := serveWindows(t, fake)
	day := datasource.TimeRange{From: rangeEnd.Add(-24 * time.Hour), To: rangeEnd}
	frames, info, err := ds.QueryPage(context.Background(), datasource.PageRequest{
		Query:    datasource.Query{Text: `{}`, Limits: datasource.Limits{MaxRows: 1000, Timeout: 2 * time.Second}},
		Range:    day,
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].Meta == nil || !frames[0].Meta.Partial {
		t.Fatalf("frames = %+v, want a partial page", frames)
	}
	if !info.HasNext || info.Next == nil || info.TotalRelation != datasource.TotalUnknown {
		t.Fatalf("info = %+v, want a next page from the oldest trace found and no total", info)
	}
}

func TestAnEmptyPageCutByTheTimeLimitStaysPartial(t *testing.T) {
	fake := &windowedTempo{slow: func(int) bool { return true }}
	ds := serveWindows(t, fake)
	frames, info, err := ds.QueryPage(context.Background(), datasource.PageRequest{
		Query:    datasource.Query{Text: `{}`, Limits: datasource.Limits{MaxRows: 1000, Timeout: 1500 * time.Millisecond}},
		Range:    lastHour,
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].Rows() != 0 || frames[0].Meta == nil || !frames[0].Meta.Partial || info.HasNext {
		t.Fatalf("frames = %+v, info = %+v, want one empty partial frame and no next page", frames, info)
	}
}
