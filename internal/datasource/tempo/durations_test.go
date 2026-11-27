package tempo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func spanJSON(name string, start time.Time, duration time.Duration, attrs ...string) string {
	kv := make([]string, 0, len(attrs)/2)
	for i := 0; i+1 < len(attrs); i += 2 {
		kv = append(kv, fmt.Sprintf(`{"key":%q,"value":{"stringValue":%q}}`, attrs[i], attrs[i+1]))
	}
	return fmt.Sprintf(`{"spanID":"01","name":%q,"startTimeUnixNano":"%d","durationNanos":"%d","attributes":[%s]}`, name, start.UnixNano(), duration.Nanoseconds(), strings.Join(kv, ","))
}

func spanSetTrace(id string, matched int, spans ...string) string {
	return fmt.Sprintf(`{"traceID":%q,"spanSets":[{"spans":[%s],"matched":%d}]}`, id, strings.Join(spans, ","), matched)
}

func spanSetBody(traces ...string) string {
	return `{"traces":[` + strings.Join(traces, ",") + `],"metrics":{"completedJobs":1,"totalJobs":1}}`
}

func TestTraceDurationsAreExactFromEverySpanASearchReturns(t *testing.T) {
	at := statsFrom.Add(10 * time.Minute)
	ds, fake := serve(t, map[string]reply{"/api/search": {0, spanSetBody(
		spanSetTrace("aa", 2,
			spanJSON("GET /cart", at, 10*time.Millisecond, "service.name", "cart", "status", "unset"),
			spanJSON("GET /cart", at, 30*time.Millisecond, "service.name", "cart", "status", "error")),
		spanSetTrace("bb", 2,
			spanJSON("charge", at, 20*time.Millisecond, "service.name", "payment", "status", "unset"),
			spanJSON("before", statsFrom.Add(-time.Minute), time.Second, "service.name", "payment", "status", "unset")),
		spanSetTrace("cc", 1, spanJSON("noattr", at, 5*time.Millisecond, "service.name", "nil", "status", "unset")),
	)}})
	got, err := ds.TraceDurations(context.Background(), datasource.TraceDurationRequest{Query: "{}", GroupBy: "resource.service.name", Range: statsRange})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exact {
		t.Fatal("every matched span was returned, yet the figures are not exact")
	}
	if got.Window.Spans != 4 || got.Window.Errors != 1 || got.Window.P50 != 20 || got.Window.P99 != 30 {
		t.Fatalf("window = %+v, want the four spans inside the range", got.Window)
	}
	if len(got.Groups) != 2 || got.Groups[0].Value != "cart" || got.Groups[0].Spans != 2 || got.Groups[0].P50 != 30 || got.Groups[1].Value != "payment" {
		t.Fatalf("groups = %+v", got.Groups)
	}
	req := fake.last(t)
	if q := req.params.Get("q"); q != "{} | select(status, resource.service.name) with (most_recent=true)" {
		t.Fatalf("q = %q", q)
	}
	if req.params.Get("spss") != "100" {
		t.Fatalf("spss = %q", req.params.Get("spss"))
	}
}

func TestTraceDurationsAreASampleWhenASpanSetIsCut(t *testing.T) {
	at := statsFrom.Add(10 * time.Minute)
	ds, _ := serve(t, map[string]reply{"/api/search": {0, spanSetBody(
		spanSetTrace("aa", 150, spanJSON("GET /cart", at, 10*time.Millisecond, "status", "unset")),
	)}})
	got, err := ds.TraceDurations(context.Background(), datasource.TraceDurationRequest{Query: "{}", GroupBy: "name", Range: statsRange})
	if err != nil {
		t.Fatal(err)
	}
	if got.Exact || got.Window.Spans != 1 || len(got.Groups) != 1 || got.Groups[0].Value != "GET /cart" {
		t.Fatalf("durations = %+v, want a one-span sample grouped by name", got)
	}
}

func TestTraceDurationsRefuseWhatTraceStatsRefuses(t *testing.T) {
	ds, _ := serve(t, nil)
	for _, req := range []datasource.TraceDurationRequest{
		{Query: "status = error", Range: statsRange},
		{Query: "{}", GroupBy: "service name", Range: statsRange},
	} {
		if _, err := ds.TraceDurations(context.Background(), req); err == nil {
			t.Errorf("%+v was accepted", req)
		}
	}
}
