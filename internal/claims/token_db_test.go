package claims

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestAuthenticateCarriesTheModeOfTheGrantTheRunIsAssignedTo(t *testing.T) {
	f := newFixture(t)
	f.d.Exec(t, "INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1, 'issuer', 'subject', 'ada@example.com', 'Ada')", mustUUID(t, userID))
	f.d.Exec(t, "INSERT INTO oauth_clients (id, redirect_uris) VALUES ('client', ARRAY['http://localhost/cb'])")
	f.d.Exec(t, "INSERT INTO oauth_grants (id, client_id, user_id, resource, expires_at) VALUES ($1, 'client', $2, 'r', now() + interval '1 day')", mustUUID(t, grantID), mustUUID(t, userID))
	f.queue(t, revisions.AssignGrant, grantID)
	offer := f.claim(t, grantClaimer)

	if access, ok := f.authenticateRun(t, "Bearer "+offer.Session.Token); !ok || access.Mode != grantmode.Direct {
		t.Fatalf("a run of a direct grant = %q accepted %v, want direct", access.Mode, ok)
	}
	f.d.Exec(t, "UPDATE oauth_grants SET mode = 'propose' WHERE id = $1", mustUUID(t, grantID))
	if access, ok := f.authenticateRun(t, "Bearer "+offer.Session.Token); !ok || access.Mode != grantmode.Propose {
		t.Fatalf("a run of a propose grant = %q accepted %v, want propose on its next request", access.Mode, ok)
	}
}

func TestAuthenticateGivesARunOfAServiceAccountTheDirectMode(t *testing.T) {
	f := newFixture(t)
	account, err := f.d.Queries().CreateServiceAccount(context.Background(), dbsqlc.CreateServiceAccountParams{Name: "ci-agent", Role: "editor"})
	if err != nil {
		t.Fatal(err)
	}
	f.d.Exec(t, "UPDATE service_accounts SET takes_runs = true WHERE id = $1", account.ID)
	claimer := Claimer{Kind: revisions.AssignServiceAccount, ID: account.ID.String(), Client: "ci"}
	f.queue(t, revisions.AssignServiceAccount, claimer.ID)
	offer := f.claim(t, claimer)

	if access, ok := f.authenticateRun(t, "Bearer "+offer.Session.Token); !ok || access.Mode != grantmode.Direct {
		t.Fatalf("a service account run = %q accepted %v, want direct", access.Mode, ok)
	}
}

func (f *fixture) authenticateRun(t *testing.T, header string) (RunAccess, bool) {
	t.Helper()
	access, ok, err := f.svc.Authenticate(context.Background(), header)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	return access, ok
}

func (f *fixture) authenticate(t *testing.T, header string) (revisions.Request, bool) {
	t.Helper()
	access, ok := f.authenticateRun(t, header)
	return access.Request, ok
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
