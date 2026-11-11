package claims

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (f *fixture) authenticate(t *testing.T, header string) (revisions.Request, bool) {
	t.Helper()
	req, ok, err := f.svc.Authenticate(context.Background(), header)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	return req, ok
}

func TestAuthenticateAcceptsAFreshTokenAndNamesTheRequester(t *testing.T) {
	f := newFixture(t)
	queued := f.queue(t, revisions.AssignGrant, grantID)
	offer := f.claim(t, grantClaimer)

	req, ok := f.authenticate(t, "Bearer "+offer.Session.Token)

	if !ok {
		t.Fatal("a fresh run token was refused")
	}
	if req.ID != queued.ID.String() || req.Attempt != 1 || req.RequestedBy.ID != userID || req.RequestedBy.Name != "Ada" {
		t.Fatalf("request = %+v", req)
	}
	if _, ok := f.authenticate(t, "bearer "+offer.Session.Token); !ok {
		t.Fatal("the scheme is case-insensitive and a lowercase one was refused")
	}
}

func TestAuthenticateRefusesWhatIsNotARunToken(t *testing.T) {
	f := newFixture(t)
	f.queue(t, revisions.AssignGrant, grantID)
	offer := f.claim(t, grantClaimer)
	body := strings.TrimPrefix(offer.Session.Token, "sot_")

	for name, header := range map[string]string{
		"empty":                 "",
		"no scheme":             offer.Session.Token,
		"basic scheme":          "Basic " + offer.Session.Token,
		"api key shape":         "Bearer sok_" + body,
		"unknown token":         "Bearer sot_" + strings.Repeat("A", len(body)),
		"truncated token":       "Bearer " + offer.Session.Token[:len(offer.Session.Token)-1],
		"token with extra tail": "Bearer " + offer.Session.Token + "x",
	} {
		if _, ok := f.authenticate(t, header); ok {
			t.Errorf("%s: header was accepted", name)
		}
	}
}

func TestAuthenticateRefusesATokenOfAnEarlierAttempt(t *testing.T) {
	f := newFixture(t)
	queued := f.queue(t, revisions.AssignGrant, grantID)
	first := f.claim(t, grantClaimer)
	firstRow := f.stored(t, queued.ID)
	f.d.Exec(t, "UPDATE agent_requests SET lease_until = now() - interval '1 second' WHERE id = $1", queued.ID)

	second := f.claim(t, grantClaimer)

	if second.Run.Attempt != 2 {
		t.Fatalf("second claim attempt = %d, want 2", second.Run.Attempt)
	}
	if _, err := f.runs.RenewLease(context.Background(), firstRow); !errors.Is(err, revisions.ErrLeaseLost) {
		t.Fatalf("renewing the first attempt = %v, want ErrLeaseLost", err)
	}
	if _, ok := f.authenticate(t, "Bearer "+first.Session.Token); ok {
		t.Fatal("the attempt 1 token was accepted after attempt 2 took the run")
	}
	if req, ok := f.authenticate(t, "Bearer "+second.Session.Token); !ok || req.Attempt != 2 {
		t.Fatalf("the attempt 2 token = %+v accepted %v", req, ok)
	}
}

func TestAuthenticateRefusesATokenOnceTheRunEnded(t *testing.T) {
	f := newFixture(t)
	queued := f.queue(t, revisions.AssignGrant, grantID)
	offer := f.claim(t, grantClaimer)
	if _, err := f.runs.End(context.Background(), f.stored(t, queued.ID), revisions.Ending{}); err != nil {
		t.Fatal(err)
	}

	if _, ok := f.authenticate(t, "Bearer "+offer.Session.Token); ok {
		t.Fatal("a token of an ended run was accepted")
	}
}

func TestAuthenticateRefusesATokenWhoseLeaseLapsed(t *testing.T) {
	f := newFixture(t)
	queued := f.queue(t, revisions.AssignGrant, grantID)
	offer := f.claim(t, grantClaimer)
	f.d.Exec(t, "UPDATE agent_requests SET lease_until = now() - interval '1 second' WHERE id = $1", queued.ID)

	if _, ok := f.authenticate(t, "Bearer "+offer.Session.Token); ok {
		t.Fatal("a token was accepted after its run's lease lapsed")
	}
}

func TestAuthenticateRefusesAnExpiredToken(t *testing.T) {
	f := newFixture(t)
	queued := f.queue(t, revisions.AssignGrant, grantID)
	offer := f.claim(t, grantClaimer)
	f.d.Exec(t, "UPDATE run_tokens SET expires_at = now() - interval '1 second' WHERE agent_request_id = $1", queued.ID)

	if _, ok := f.authenticate(t, "Bearer "+offer.Session.Token); ok {
		t.Fatal("a token past its deadline was accepted")
	}
}

func TestAuthenticateRefusesATokenOfACancelingRun(t *testing.T) {
	f := newFixture(t)
	queued := f.queue(t, revisions.AssignGrant, grantID)
	offer := f.claim(t, grantClaimer)
	if _, err := f.runs.Cancel(context.Background(), queued.ID, ada); err != nil {
		t.Fatal(err)
	}
	if got := f.stored(t, queued.ID).State; got != string(revisions.StateCanceling) {
		t.Fatalf("run state = %s, want canceling", got)
	}

	if _, ok := f.authenticate(t, "Bearer "+offer.Session.Token); ok {
		t.Fatal("a token of a canceling run was accepted")
	}
}

func TestAuthenticateRefusesATokenWhoseRequesterIsGone(t *testing.T) {
	f := newFixture(t)
	f.queue(t, revisions.AssignGrant, grantID)
	offer := f.claim(t, grantClaimer)
	f.principals.gone.Store(true)

	if _, ok := f.authenticate(t, "Bearer "+offer.Session.Token); ok {
		t.Fatal("a token was accepted for a requester that no longer exists")
	}
}

func TestAuthenticateRefusesTheTokenOfAReleasedClaimOnceTheRunIsClaimedAgain(t *testing.T) {
	f := newFixture(t)
	queued := f.queue(t, revisions.AssignGrant, grantID)
	first := f.claim(t, grantClaimer)
	f.runs.Release(context.Background(), f.stored(t, queued.ID))

	second := f.claim(t, grantClaimer)

	if second.Run.Attempt != first.Run.Attempt {
		t.Fatalf("attempts %d then %d, want the released attempt number reused", first.Run.Attempt, second.Run.Attempt)
	}
	if _, ok := f.authenticate(t, "Bearer "+first.Session.Token); ok {
		t.Fatal("the token of the released claim was accepted after the run was claimed again")
	}
	if _, ok := f.authenticate(t, "Bearer "+second.Session.Token); !ok {
		t.Fatal("the token of the new claim was refused")
	}
}
