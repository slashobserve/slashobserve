package trail

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/auth"
)

func (r *httpRig) follow(t *testing.T, text, lastEventID string) *Follow {
	t.Helper()
	f, err := r.reader.Follow(auth.WithPrincipal(t.Context(), admin), text, lastEventID)
	if err != nil {
		t.Fatalf("follow %q from %q: %v", text, lastEventID, err)
	}
	return f
}

func next(t *testing.T, f *Follow) []Followed {
	t.Helper()
	page, err := f.Next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func followedIDs(page []Followed) []string {
	ids := make([]string, 0, len(page))
	for _, f := range page {
		ids = append(ids, f.Event.ID)
	}
	return ids
}

func TestTheStreamSendsOnlyNewMatchingEventsInCommitOrder(t *testing.T) {
	r := newHTTPRig(t)
	now := time.Now().UTC()
	trailtest.Insert(t, r.db, trailtest.Event{At: now.Add(-time.Minute), Outcome: "denied", Status: 403})
	f := r.follow(t, "outcome:denied", "")
	if page := next(t, f); len(page) != 0 {
		t.Fatalf("an event recorded before the stream opened was sent: %v", followedIDs(page))
	}
	ids := trailtest.Insert(t, r.db,
		trailtest.Event{At: now, Outcome: "denied", Status: 403},
		trailtest.Event{At: now, Outcome: "success"},
		trailtest.Event{At: now.Add(-2 * time.Minute), Outcome: "denied", Status: 403},
		trailtest.Event{At: now.Add(-2 * FollowSlack), Outcome: "denied", Status: 403},
	)
	page := next(t, f)
	if got := followedIDs(page); len(got) != 2 || got[0] != ids[0] || got[1] != ids[2] {
		t.Fatalf("sent %v, want %v", got, []string{ids[0], ids[2]})
	}
	if again := next(t, f); len(again) != 0 {
		t.Fatalf("sent %v again", followedIDs(again))
	}

	resumed := r.follow(t, "outcome:denied", page[0].ID)
	if got := followedIDs(next(t, resumed)); len(got) != 1 || got[0] != ids[2] {
		t.Fatalf("resumed after %s and sent %v, want %v", page[0].ID, got, ids[2:3])
	}
}

func TestTheStreamRecordsOneSearchWhenItOpensAndEachRefusal(t *testing.T) {
	r := newHTTPRig(t)
	f := r.follow(t, "action:dashboard.*", "")
	trailtest.Insert(t, r.db, trailtest.Event{At: time.Now().UTC()})
	for range 3 {
		next(t, f)
	}
	ctx := auth.WithPrincipal(context.Background(), admin)
	for _, tc := range []struct {
		text, lastEventID string
		status            int
		code              string
	}{
		{"outcome:denied and status:x", "", http.StatusUnprocessableEntity, CodeQueryInvalid},
		{"outcome:denied", "12", http.StatusBadRequest, "invalid_request"},
		{"outcome:denied", "x-1", http.StatusBadRequest, "invalid_request"},
	} {
		_, err := r.reader.Follow(ctx, tc.text, tc.lastEventID)
		problem, ok := ProblemOf(err)
		if !ok || problem.Status != tc.status || problem.Code != tc.code {
			t.Errorf("follow %q from %q = %v, want %d %s", tc.text, tc.lastEventID, err, tc.status, tc.code)
		}
	}
	reads := r.reads(t)
	if len(reads) != 4 {
		t.Fatalf("recorded %+v", reads)
	}
	if got := reads[0]; got.action != string(ActionSearch) || got.outcome != "success" || got.data["view"] != "stream" || got.data["query"] != "action:dashboard.*" || got.data["form"] != "kql" || got.data["since"] == nil {
		t.Errorf("open recorded as %+v", got)
	}
	for _, got := range reads[1:] {
		if got.action != string(ActionSearch) || got.outcome != "failed" || got.data["view"] != "stream" || got.data["query"] == nil {
			t.Errorf("refusal recorded as %+v", got)
		}
	}
}

func TestAStreamEventIDRoundTrips(t *testing.T) {
	for _, raw := range []string{"0-0", "18446744073709551615-9223372036854775807", "812-4"} {
		at, ok := parseEventID(raw)
		if !ok || eventID(at) != raw {
			t.Errorf("%q parsed to %+v %v", raw, at, ok)
		}
	}
	for _, raw := range []string{"", "12", "-1", "1-", "1--2", "x-1", "1-x", "18446744073709551616-1"} {
		if _, ok := parseEventID(raw); ok {
			t.Errorf("%q parsed", raw)
		}
	}
}
