package loki_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
)

type chunk struct{ start, end time.Time }

type chunkedLoki struct {
	mu          sync.Mutex
	chunks      []chunk
	queries     []string
	inFlight    int
	maxInFlight int
	slow        func(n int) bool
	status      int
}

func (f *chunkedLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	start, _ := time.Parse(time.RFC3339Nano, q.Get("start"))
	end, _ := time.Parse(time.RFC3339Nano, q.Get("end"))
	step, _ := strconv.ParseFloat(q.Get("step"), 64)
	f.mu.Lock()
	f.chunks = append(f.chunks, chunk{start, end})
	f.queries = append(f.queries, q.Get("query")+" step="+q.Get("step"))
	n := len(f.chunks)
	f.inFlight++
	f.maxInFlight = max(f.maxInFlight, f.inFlight)
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"parse error"}`))
		return
	}
	if f.slow != nil && f.slow(n) {
		<-r.Context().Done()
		return
	}
	var points [][2]any
	for at := start; !at.After(end); at = at.Add(time.Duration(step * float64(time.Second))) {
		points = append(points, [2]any{at.Unix(), "1"})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
		"resultType": "matrix",
		"result":     []any{map[string]any{"metric": map[string]string{"level": "error"}, "values": points}},
	}})
}

func (f *chunkedLoki) sent() []chunk {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.chunks)
}

func serveChunks(t *testing.T, fake *chunkedLoki) *loki.Datasource {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ds, err := loki.New(loki.Options{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

var volumeFrom = time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)

func volumeOver(days int, step time.Duration, timeout time.Duration) datasource.VolumeRequest {
	r := datasource.TimeRange{From: volumeFrom, To: volumeFrom.Add(time.Duration(days) * 24 * time.Hour)}
	return datasource.VolumeRequest{
		Query: datasource.Query{Text: `{service_name="checkout"}`, Limits: datasource.Limits{Timeout: timeout}},
		Range: r,
		Step:  step,
	}
}

func TestAVolumeOverDaysIsCountedADayAtATimeNewestFirst(t *testing.T) {
	fake := &chunkedLoki{}
	ds := serveChunks(t, fake)
	hist, err := ds.LogVolume(context.Background(), volumeOver(3, time.Hour, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	h := time.Hour
	want := []chunk{
		{volumeFrom.Add(49 * h), volumeFrom.Add(72 * h)},
		{volumeFrom.Add(25 * h), volumeFrom.Add(48 * h)},
		{volumeFrom.Add(h), volumeFrom.Add(24 * h)},
	}
	if got := fake.sent(); !slices.Equal(got, want) {
		t.Fatalf("chunks = %v, want %v", got, want)
	}
	query := "sum by (level, lvl, severity_text, severity, detected_level) (count_over_time(\n{service_name=\"checkout\"}\n[3600000ms])) step=3600"
	fake.mu.Lock()
	queries, most := slices.Clone(fake.queries), fake.maxInFlight
	fake.mu.Unlock()
	for i, q := range queries {
		if q != query {
			t.Fatalf("chunk %d query = %q, want %q", i, q, query)
		}
	}
	if most != 1 {
		t.Fatalf("%d chunks ran at once, want one at a time", most)
	}
	v := hist.Volume()
	if v.Partial() || v.Total != 72 || len(v.Buckets) != 72 || v.Buckets[0].Error != 1 || v.Buckets[71].Error != 1 {
		t.Fatalf("volume = %+v, want 72 complete buckets of one error line", v)
	}
}

func TestAVolumeWithinADayIsOneRequest(t *testing.T) {
	fake := &chunkedLoki{}
	ds := serveChunks(t, fake)
	step := 24 * time.Minute
	if _, err := ds.LogVolume(context.Background(), volumeOver(1, step, time.Minute)); err != nil {
		t.Fatal(err)
	}
	want := []chunk{{volumeFrom.Add(step), volumeFrom.Add(24 * time.Hour)}}
	if got := fake.sent(); !slices.Equal(got, want) {
		t.Fatalf("chunks = %v, want %v", got, want)
	}
}

func TestAVolumeThatRunsOutOfTimeKeepsTheNewestDaysAsPartial(t *testing.T) {
	fake := &chunkedLoki{slow: func(n int) bool { return n == 2 }}
	ds := serveChunks(t, fake)
	started := time.Now()
	hist, err := ds.LogVolume(context.Background(), volumeOver(3, time.Hour, 2*time.Second))
	if err != nil {
		t.Fatalf("err = %v, want the newest day counted", err)
	}
	if took := time.Since(started); took > 1900*time.Millisecond {
		t.Fatalf("took %v, want an answer before the 2 s limit", took)
	}
	v := hist.Volume()
	if !v.Partial() || !v.CountedFrom.Equal(volumeFrom.Add(48*time.Hour)) || v.Total != 24 {
		t.Fatalf("volume = %+v, want 24 lines counted from the start of the third day", v)
	}
}

func TestAVolumeWithNothingCountedInTimeIsAnEmptyPartialResult(t *testing.T) {
	fake := &chunkedLoki{slow: func(int) bool { return true }}
	ds := serveChunks(t, fake)
	req := volumeOver(1, time.Hour, 1500*time.Millisecond)
	hist, err := ds.LogVolume(context.Background(), req)
	if err != nil {
		t.Fatalf("err = %v, want an empty partial result", err)
	}
	if v := hist.Volume(); !v.Partial() || !v.CountedFrom.Equal(req.Range.To) || v.Total != 0 {
		t.Fatalf("volume = %+v, want nothing counted and partial from the end", v)
	}
}

func TestAVolumeRejectedByLokiIsAnError(t *testing.T) {
	ds := serveChunks(t, &chunkedLoki{status: http.StatusBadRequest})
	if _, err := ds.LogVolume(context.Background(), volumeOver(1, time.Hour, time.Minute)); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want an invalid query", err)
	}
}

func TestAVolumeRefusesAMetricQuery(t *testing.T) {
	ds := serveChunks(t, &chunkedLoki{})
	req := volumeOver(1, time.Hour, time.Minute)
	req.Query.Text = `rate({a="b"}[1m])`
	if _, err := ds.LogVolume(context.Background(), req); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want an invalid query", err)
	}
}
