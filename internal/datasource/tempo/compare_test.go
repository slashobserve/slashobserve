package tempo_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/tempo"
)

const (
	secondTraceID = "1b2c3d4e5f60718293a4b5c6d7e8f901"
	missingID     = "00f067aa0ba902b700f067aa0ba902b7"
)

func pairText(set datasource.TraceAttributeSet) string {
	parts := make([]string, len(set.Attributes))
	for i, a := range set.Attributes {
		parts[i] = a.Key + "=" + a.Value
	}
	return strings.Join(parts, " ")
}

func TestTraceAttributesReadsEverySetInOrder(t *testing.T) {
	ds, _ := serve(t, map[string]reply{
		tracePath:                      {0, traceBody},
		"/api/traces/" + secondTraceID: {0, eventTraceBody},
	})
	sets, err := ds.TraceAttributes(context.Background(), datasource.TraceAttributesRequest{IDs: []string{traceID, missingID, secondTraceID}, Range: lastHour, MaxSpans: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 2 || sets[0].ID != traceID || sets[1].ID != secondTraceID {
		t.Fatalf("sets = %+v, want the two found traces in request order", sets)
	}
	want := "kind=Client kind=Server name=POST /checkout name=charge resource.k8s.pod.name=checkout-0 resource.service.name=checkout " +
		"span.http.method=POST span.http.status_code=500 span.ratio=0.5 span.retry=true span.weird=NaN status=Error status=Unset"
	if got := pairText(sets[0]); got != want {
		t.Fatalf("pairs =\n%s\nwant\n%s", got, want)
	}
	want = "kind=Client kind=Server kind=Unspecified name=POST /checkout name=charge name=queue resource.service.name=checkout status=Error status=Unset"
	if got := pairText(sets[1]); got != want {
		t.Fatalf("pairs =\n%s\nwant\n%s", got, want)
	}
}

func TestTraceAttributesKeepsTheEarliestSpans(t *testing.T) {
	ds, _ := serve(t, map[string]reply{tracePath: {0, manySpans(25)}})
	sets, err := ds.TraceAttributes(context.Background(), datasource.TraceAttributesRequest{IDs: []string{traceID}, Range: lastHour, MaxSpans: 3})
	if err != nil {
		t.Fatal(err)
	}
	if want := "kind=Unspecified name=s22 name=s23 name=s24 resource.service.name=checkout status=Unset"; len(sets) != 1 || pairText(sets[0]) != want {
		t.Fatalf("sets = %+v, want %s", sets, want)
	}
}

func TestTraceAttributesStopsAtTheDeadline(t *testing.T) {
	var served atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		served.Add(1)
		_, _ = w.Write([]byte(traceBody))
	}))
	t.Cleanup(srv.Close)
	ds, err := tempo.New(tempo.Options{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	ids := []string{traceID, secondTraceID, missingID, "b7ad6b7169203331", "00f067aa0ba902b7"}
	sets, err := ds.TraceAttributes(ctx, datasource.TraceAttributesRequest{IDs: ids, Range: lastHour, MaxSpans: 100})
	if err != nil {
		t.Fatalf("err = %v, want the sets read before the deadline", err)
	}
	if len(sets) < 1 || len(sets) > 4 {
		t.Fatalf("read %d sets, want at least one and at most the first batch", len(sets))
	}
	for i := range sets {
		if sets[i].ID != ids[i] {
			t.Fatalf("sets = %+v, want request order", sets)
		}
	}
	expired, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, err := ds.TraceAttributes(expired, datasource.TraceAttributesRequest{IDs: ids[:1], Range: lastHour, MaxSpans: 100}); err == nil {
		t.Fatal("no error when nothing could be read")
	}
}

func TestTraceAttributesReturnsUpstreamFailures(t *testing.T) {
	ds, _ := serve(t, map[string]reply{tracePath: {http.StatusInternalServerError, "boom"}})
	if _, err := ds.TraceAttributes(context.Background(), datasource.TraceAttributesRequest{IDs: []string{traceID}, Range: lastHour, MaxSpans: 100}); err == nil {
		t.Fatal("a 500 from Tempo was swallowed")
	}
}
