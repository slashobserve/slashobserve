package tools

import (
	"testing"
	"time"
)

func span(id, parent, service, name, status string, offset, duration time.Duration) traceSpan {
	return traceSpan{id: id, parent: parent, service: service, name: name, status: status, start: fixtureStart.Add(offset), duration: float64(duration)}
}

func TestOriginOfNamesTheDeepestErrorWithNoErrorBelow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		spans      []traceSpan
		wantOrigin string
		wantRelays []string
	}{
		{
			name: "no error",
			spans: []traceSpan{
				span("r", "", "front", "GET /", "Unset", 0, time.Second),
			},
		},
		{
			name: "a relayed chain names the deepest",
			spans: []traceSpan{
				span("r", "", "front", "GET /", "Error", 0, time.Second),
				span("c", "r", "checkout", "PlaceOrder", "Error", time.Millisecond, 500*time.Millisecond),
				span("p", "c", "payment", "Charge", "Error", 2*time.Millisecond, 100*time.Millisecond),
			},
			wantOrigin: "p", wantRelays: []string{"c", "r"},
		},
		{
			name: "an ok span between errors still relays",
			spans: []traceSpan{
				span("r", "", "front", "GET /", "Error", 0, time.Second),
				span("c", "r", "checkout", "PlaceOrder", "Ok", time.Millisecond, 500*time.Millisecond),
				span("p", "c", "payment", "Charge", "Error", 2*time.Millisecond, 100*time.Millisecond),
			},
			wantOrigin: "p", wantRelays: []string{"r"},
		},
		{
			name: "two origins pick the deeper",
			spans: []traceSpan{
				span("r", "", "front", "GET /", "Unset", 0, time.Second),
				span("a", "r", "cart", "GetCart", "Error", time.Millisecond, 10*time.Millisecond),
				span("b", "r", "checkout", "PlaceOrder", "Unset", 2*time.Millisecond, 10*time.Millisecond),
				span("d", "b", "currency", "Convert", "STATUS_CODE_ERROR", 3*time.Millisecond, 5*time.Millisecond),
			},
			wantOrigin: "d",
		},
		{
			name: "equal depths pick the earlier",
			spans: []traceSpan{
				span("r", "", "front", "GET /", "Unset", 0, time.Second),
				span("b", "r", "checkout", "PlaceOrder", "Error", 5*time.Millisecond, 10*time.Millisecond),
				span("a", "r", "cart", "GetCart", "Error", time.Millisecond, 10*time.Millisecond),
			},
			wantOrigin: "a",
		},
		{
			name: "a missing parent counts one level and a loop ends",
			spans: []traceSpan{
				span("x", "gone", "worker", "Consume", "Error", 0, time.Second),
				span("l1", "l2", "loop", "One", "Unset", 0, time.Second),
				span("l2", "l1", "loop", "Two", "Unset", 0, time.Second),
			},
			wantOrigin: "x",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := originOf(tc.spans)
			if tc.wantOrigin == "" {
				if ok {
					t.Fatalf("origin = %s, want none", got.span.id)
				}
				return
			}
			if !ok || got.span.id != tc.wantOrigin {
				t.Fatalf("origin = %+v, want %s", got, tc.wantOrigin)
			}
			relays := []string{}
			for _, r := range got.relays {
				relays = append(relays, r.id)
			}
			if len(relays) != len(tc.wantRelays) {
				t.Fatalf("relays = %v, want %v", relays, tc.wantRelays)
			}
			for i := range relays {
				if relays[i] != tc.wantRelays[i] {
					t.Fatalf("relays = %v, want %v", relays, tc.wantRelays)
				}
			}
		})
	}
}

func TestSelfTimesSubtractTheChildrenOnce(t *testing.T) {
	spans := []traceSpan{
		span("r", "", "front", "GET /", "Unset", 0, 100*time.Millisecond),
		span("a", "r", "cart", "GetCart", "Unset", 10*time.Millisecond, 30*time.Millisecond),
		span("b", "r", "cart", "GetCart", "Unset", 20*time.Millisecond, 30*time.Millisecond),
		span("c", "r", "db", "Query", "Unset", 90*time.Millisecond, 50*time.Millisecond),
	}
	got := selfTimes(spans)
	for id, want := range map[string]time.Duration{"r": 50 * time.Millisecond, "a": 30 * time.Millisecond, "c": 50 * time.Millisecond} {
		if got[id] != float64(want) {
			t.Errorf("self time of %s = %v, want %v", id, time.Duration(got[id]), want)
		}
	}
	if d := time.Duration(extentOf(spans)); d != 140*time.Millisecond {
		t.Errorf("extent = %v, want 140ms", d)
	}
}
