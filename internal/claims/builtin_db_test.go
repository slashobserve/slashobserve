package claims

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (f *fixture) queueBuiltin(t *testing.T) string {
	t.Helper()
	row, err := f.runs.Submit(context.Background(), revisions.EnqueueParams{
		DashboardID: f.dashboard, Source: revisions.SourcePrompt, Prompt: "add a latency panel", BaseVersion: 1, By: ada,
	})
	if err != nil {
		t.Fatal(err)
	}
	return row.ID.String()
}

func TestAnOutsideClaimersOfferHasNoModelSettings(t *testing.T) {
	f := newFixture(t)
	f.queue(t, revisions.AssignGrant, grantID)

	offer := f.claim(t, grantClaimer)

	encoded, err := json.Marshal(offer)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if _, found := fields["model"]; found || offer.Model != nil {
		t.Fatalf("an outside claimer's offer carries model settings: %s", encoded)
	}
}

func TestTheBuiltInClaimersOfferHasModelSettingsAndAnInternalSessionURL(t *testing.T) {
	f := newFixture(t)
	runID := f.queueBuiltin(t)

	offer := f.claim(t, builtinClaimer)

	if offer.Run.ID != runID || offer.Session.URL != "http://127.0.0.1:8080/mcp" {
		t.Fatalf("run %s, session url %q", offer.Run.ID, offer.Session.URL)
	}
	if offer.Model == nil || *offer.Model != modelSettings {
		t.Fatalf("model = %+v, want %+v", offer.Model, modelSettings)
	}
	settingsType := reflect.TypeFor[ModelSettings]()
	var sent []string
	for i := range settingsType.NumField() {
		field := settingsType.Field(i)
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if strings.Contains(field.Tag.Get("json"), "omitempty") {
			t.Errorf("model settings field %q is omitted when empty", name)
		}
		sent = append(sent, name)
	}
	want := []string{"provider", "model", "api_key", "base_url", "effort", "max_tokens", "request_timeout_seconds", "idle_timeout_seconds", "replay_dir", "log_provider_errors", "session_renew_bytes"}
	if !slices.Equal(sent, want) {
		t.Errorf("model settings fields = %v, want %v", sent, want)
	}
}

func TestTheBuiltInClaimerIsNoConnectedAgent(t *testing.T) {
	f := newFixture(t)
	f.queueBuiltin(t)

	f.claim(t, builtinClaimer)

	var n int
	if err := f.d.SQLDB(t).QueryRow("SELECT count(*) FROM agent_workers").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d agent_workers rows, want none", n)
	}
}

func TestTheBuiltInClaimerOfAnotherServerCannotClaim(t *testing.T) {
	f := newFixture(t)
	f.queueBuiltin(t)

	_, err := f.svc.Claim(context.Background(), Claimer{Kind: revisions.AssignBuiltin, ID: "server-2"}, 0)

	if !errors.Is(err, ErrNotAWorker) {
		t.Fatalf("err = %v, want ErrNotAWorker", err)
	}
}

func TestReleaseHeldPutsTheRunsAClaimerHoldsBackInTheQueue(t *testing.T) {
	f := newFixture(t)
	runID := f.queueBuiltin(t)
	offer := f.claim(t, builtinClaimer)
	other := f.queue(t, revisions.AssignGrant, grantID)
	f.d.Exec(t, "UPDATE agent_requests SET dashboard_id = $3, claimed_by = $2, state = 'running', status = 'running', attempts = 1 WHERE id = $1", other.ID, grantClaimer.claimant().Key(), seedDashboard(t, f.d))

	if err := f.svc.ReleaseHeld(context.Background(), builtinClaimer); err != nil {
		t.Fatal(err)
	}

	released := f.stored(t, mustUUID(t, runID))
	if released.State != string(revisions.StateQueued) || released.Attempts != 0 {
		t.Fatalf("released run = state %q attempts %d, want queued with attempt 0", released.State, released.Attempts)
	}
	if held := f.stored(t, other.ID); held.State != string(revisions.StateRunning) {
		t.Fatalf("another claimer's run = %q, want it left running", held.State)
	}
	if offer.Run.Attempt != 1 {
		t.Fatalf("claimed attempt = %d", offer.Run.Attempt)
	}
}

func TestClaimedRunNamesTheRunAndItsRequesterOnlyForItsClaimer(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)

	req, err := f.svc.ClaimedRun(context.Background(), grantClaimer, queued.ID.String(), 1)
	if err != nil || req.ID != queued.ID.String() || req.Attempt != 1 || req.RequestedBy.ID != userID {
		t.Fatalf("request = %+v, err %v", req, err)
	}
	for name, tc := range map[string]struct {
		claimer Claimer
		attempt int
	}{
		"another claimer": {Claimer{Kind: revisions.AssignGrant, ID: otherGrantID, Person: userID}, 1},
		"another attempt": {grantClaimer, 2},
	} {
		if _, err := f.svc.ClaimedRun(context.Background(), tc.claimer, queued.ID.String(), tc.attempt); !errors.Is(err, ErrNotYourRun) {
			t.Errorf("%s: err = %v, want ErrNotYourRun", name, err)
		}
	}
}
