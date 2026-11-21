package trail

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
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
		{"outcome:denied", "x-1-2", http.StatusBadRequest, "invalid_request"},
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

func TestAResumedStreamReadsBackFromItsLastEventNotFromNow(t *testing.T) {
	r := newHTTPRig(t)
	now := time.Now().UTC()
	r.reader.now = func() time.Time { return now.Add(-30 * time.Minute) }
	f := r.follow(t, "", "")
	first := trailtest.Insert(t, r.db, trailtest.Event{At: now.Add(-30 * time.Minute)})
	page := next(t, f)
	if got := followedIDs(page); len(got) != 1 || got[0] != first[0] {
		t.Fatalf("sent %v, want %v", got, first)
	}

	r.reader.now = time.Now
	later := trailtest.Insert(t, r.db,
		trailtest.Event{At: now.Add(-25 * time.Minute)},
		trailtest.Event{At: now.Add(-30*time.Minute - 2*FollowSlack)},
		trailtest.Event{At: now},
	)
	resumed := r.follow(t, "", page[0].ID)
	if got := followedIDs(next(t, resumed)); len(got) != 2 || got[0] != later[0] || got[1] != later[2] {
		t.Fatalf("resumed and sent %v, want %v", got, []string{later[0], later[2]})
	}
}

func TestAStreamResumesOnlyWithinADayOfItsLastEvent(t *testing.T) {
	r := newHTTPRig(t)
	now := time.Now().UTC()
	ctx := auth.WithPrincipal(context.Background(), admin)
	for _, at := range []time.Time{now.Add(-MaxResumeAge - time.Minute), now.Add(time.Hour)} {
		_, err := r.reader.Follow(ctx, "", eventID(filter.Position{Xid: 5, Seq: 9}, at))
		problem, ok := ProblemOf(err)
		if !ok || problem.Status != http.StatusBadRequest || problem.Code != CodeInvalidRequest {
			t.Errorf("resume from %s = %v", at, err)
		}
	}
	_, err := r.reader.Follow(ctx, "", eventID(filter.Position{Xid: 5, Seq: 9}, now.Add(-MaxResumeAge-time.Minute)))
	if problem, _ := ProblemOf(err); !strings.Contains(problem.Message, "search") {
		t.Errorf("a resume past a day says %q, want it to point at a search", problem.Message)
	}
	if _, err := r.reader.Follow(ctx, "", eventID(filter.Position{Xid: 5, Seq: 9}, now.Add(-MaxResumeAge+time.Minute))); err != nil {
		t.Errorf("a resume within a day = %v", err)
	}
}

func TestAStreamEventIDRoundTrips(t *testing.T) {
	at := time.Date(2026, 11, 21, 10, 4, 5, 123456000, time.UTC)
	for _, raw := range []string{"0-0-0", "18446744073709551615-9223372036854775807-1795255445123456", "812-4-1795255445123456"} {
		pos, when, ok := parseEventID(raw)
		if !ok || eventID(pos, when) != raw {
			t.Errorf("%q parsed to %+v %s %v", raw, pos, when, ok)
		}
	}
	if pos, when, ok := parseEventID(eventID(filter.Position{Xid: 3, Seq: 4}, at)); !ok || !when.Equal(at) || pos.Xid != 3 || pos.Seq != 4 {
		t.Errorf("an id for %s parsed to %+v %s %v", at, pos, when, ok)
	}
	for _, raw := range []string{"", "12", "1-2", "-1-2", "1--2-3", "x-1-2", "1-x-2", "1-2-x", "1-2--3", "18446744073709551616-1-2", "1-2-3-4"} {
		if _, _, ok := parseEventID(raw); ok {
			t.Errorf("%q parsed", raw)
		}
	}
}

func TestAStreamResumedFromItsStartSendsTheEventsRecordedBeforeItsFirstFrame(t *testing.T) {
	r := newHTTPRig(t)
	now := time.Now().UTC()
	trailtest.Insert(t, r.db, trailtest.Event{At: now.Add(-time.Minute)})
	f := r.follow(t, "", "")
	if _, _, ok := parseEventID(f.Start()); !ok {
		t.Fatalf("the start id %q is not a stream id", f.Start())
	}
	missed := trailtest.Insert(t, r.db, trailtest.Event{At: now})

	resumed := r.follow(t, "", f.Start())
	if got := followedIDs(next(t, resumed)); len(got) != 1 || got[0] != missed[0] {
		t.Fatalf("resumed from the start and sent %v, want %v", got, missed)
	}
	if resumed.Start() != f.Start() {
		t.Errorf("a stream resumed from %q starts at %q", f.Start(), resumed.Start())
	}
}
