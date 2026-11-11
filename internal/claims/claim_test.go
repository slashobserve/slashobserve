package claims

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestClaimWaitIsClampedToTheLongPollLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		wait time.Duration
		want time.Duration
	}{
		{"negative", -time.Second, 0},
		{"zero", 0, 0},
		{"inside the limit", 10 * time.Second, 10 * time.Second},
		{"at the limit", 30 * time.Second, 30 * time.Second},
		{"past the limit", 90 * time.Second, 30 * time.Second},
	} {
		if got := clampWait(tc.wait); got != tc.want {
			t.Errorf("%s: clampWait(%v) = %v, want %v", tc.name, tc.wait, got, tc.want)
		}
	}
}

func TestHubWakeReleasesEveryWaiter(t *testing.T) {
	h := newHub()
	first, second := h.wait(), h.wait()
	select {
	case <-first:
		t.Fatal("a waiter was released before any wake")
	default:
	}

	h.wake()

	for i, ch := range []<-chan struct{}{first, second} {
		select {
		case <-ch:
		default:
			t.Errorf("waiter %d was not released by the wake", i)
		}
	}
	select {
	case <-h.wait():
		t.Fatal("a waiter taken after the wake was released at once")
	default:
	}
}

func TestHubRefusesWaitersPastTheLimits(t *testing.T) {
	h := newHub()
	if !h.acquire("grant:a") || !h.acquire("grant:a") {
		t.Fatal("the first two waiters of one claimer were refused")
	}
	if h.acquire("grant:a") {
		t.Fatal("a third waiter of one claimer was accepted")
	}
	h.release("grant:a")
	if !h.acquire("grant:a") {
		t.Fatal("a freed slot was not reusable")
	}

	full := newHub()
	for i := range maxWaiters {
		if !full.acquire(string(rune('A'+i/2)) + "-" + string(rune('a'+i%2))) {
			t.Fatalf("waiter %d was refused below the process limit", i+1)
		}
	}
	if full.acquire("another") {
		t.Fatalf("waiter %d was accepted past the process limit", maxWaiters+1)
	}
	full.release("A-a")
	if !full.acquire("another") {
		t.Fatal("a freed process slot was not reusable")
	}
}

func TestOfferedRunMapsARequest(t *testing.T) {
	from := time.Date(2026, 11, 11, 8, 0, 0, 0, time.FixedZone("x", 3600))
	to := from.Add(time.Hour)
	requester := auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada"}

	investigate := offeredRun(revisions.Request{
		ID: "r1", Source: revisions.SourceInvestigate, Prompt: "why slow", Attempt: 2, RequestedBy: requester, From: &from, To: &to,
	})
	if investigate.Kind != "investigate" || investigate.Source != "investigate" || investigate.Attempt != 2 {
		t.Fatalf("investigate run = %+v", investigate)
	}
	if investigate.From == nil || *investigate.From != "2026-11-11T07:00:00Z" || *investigate.To != "2026-11-11T08:00:00Z" {
		t.Fatalf("window = %v to %v, want UTC RFC 3339", investigate.From, investigate.To)
	}
	if investigate.DashboardID != nil || investigate.BaseVersion != nil || investigate.PanelID != nil || investigate.ContinueNote != nil {
		t.Fatalf("empty optionals were not nil: %+v", investigate)
	}
	if investigate.RequestedBy != (Requester{Kind: "user", ID: requester.ID, Name: "Ada"}) {
		t.Fatalf("requested_by = %+v", investigate.RequestedBy)
	}

	build := offeredRun(revisions.Request{
		ID: "r2", Source: revisions.SourcePrompt, Prompt: "add a panel", Attempt: 1, DashboardID: "d1", BaseVersion: 3, PanelID: "p1", ContinueNote: "keep going", RequestedBy: requester,
	})
	if build.Kind != "build" || build.Source != "prompt" {
		t.Fatalf("build kind %q source %q", build.Kind, build.Source)
	}
	if build.DashboardID == nil || *build.DashboardID != "d1" || build.BaseVersion == nil || *build.BaseVersion != 3 || build.PanelID == nil || *build.PanelID != "p1" || build.ContinueNote == nil || *build.ContinueNote != "keep going" {
		t.Fatalf("build run = %+v", build)
	}
	if build.From != nil || build.To != nil {
		t.Fatalf("build window = %v to %v, want none", build.From, build.To)
	}
}

func TestOfferWithoutARunEncodesOnlyTheRetryHint(t *testing.T) {
	encoded, err := json.Marshal(Offer{RetryAfterSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"run":null,"retry_after_seconds":5}` {
		t.Fatalf("encoded offer = %s", encoded)
	}
	encoded, err = json.Marshal(Offer{})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"run":null}` {
		t.Fatalf("encoded empty offer = %s", encoded)
	}
}
