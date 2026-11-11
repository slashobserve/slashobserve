package revisions

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestCancelNeedsAnActiveRequestAndTheRequester(t *testing.T) {
	requesterID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	otherID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	requester := auth.Principal{Kind: auth.KindUser, ID: requesterID.String()}
	tests := []struct {
		name   string
		state  State
		source Source
		by     auth.Principal
		want   bool
	}{
		{"requester while queued", StateQueued, SourcePrompt, requester, true},
		{"requester while running", StateRunning, SourcePrompt, requester, true},
		{"requester again while canceling", StateCanceling, SourcePrompt, requester, true},
		{"requester after success", StateSucceeded, SourcePrompt, requester, false},
		{"requester after failure", StateFailed, SourcePrompt, requester, false},
		{"requester after cancel", StateCanceled, SourcePrompt, requester, false},
		{"requester of a running suggestion", StateRunning, SourceSuggest, requester, false},
		{"another user while queued", StateQueued, SourcePrompt, auth.Principal{Kind: auth.KindUser, ID: otherID.String()}, false},
		{"another user while running", StateRunning, SourcePrompt, auth.Principal{Kind: auth.KindUser, ID: otherID.String()}, false},
		{"service account with the requester's id", StateRunning, SourcePrompt, auth.Principal{Kind: auth.KindServiceAccount, ID: requesterID.String()}, false},
		{"administrator who is not the requester", StateRunning, SourcePrompt, auth.Principal{Kind: auth.KindUser, ID: otherID.String(), Role: auth.RoleAdmin}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row := dbsqlc.AgentRequest{State: string(tc.state), Source: string(tc.source), RequestedByKind: string(auth.KindUser), RequestedByID: requesterID}
			if got := slices.Contains(allowedActions(factsOf(row, buildState{}, tc.by, true)), ActionCancel); got != tc.want {
				t.Fatalf("cancel allowed = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestContinueIsAllowedOnlyForAnUnsavedStoppedBuildOfTheRequester(t *testing.T) {
	requesterID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	requester := auth.Principal{Kind: auth.KindUser, ID: requesterID.String()}
	saved := pgtype.Timestamptz{Time: time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC), Valid: true}
	continuedBy := "next"
	withPanels := buildState{build: &buildProgressJSON{Panels: 2}}
	tests := []struct {
		name    string
		state   State
		source  Source
		savedAt pgtype.Timestamptz
		build   buildState
		want    bool
	}{
		{"failed and unsaved", StateFailed, SourcePrompt, pgtype.Timestamptz{}, withPanels, true},
		{"canceled and unsaved", StateCanceled, SourceObserve, pgtype.Timestamptz{}, withPanels, true},
		{"failed and saved", StateFailed, SourcePrompt, saved, withPanels, false},
		{"failed with no panels in the build", StateFailed, SourcePrompt, pgtype.Timestamptz{}, buildState{build: &buildProgressJSON{}}, false},
		{"failed with no build", StateFailed, SourcePrompt, pgtype.Timestamptz{}, buildState{}, false},
		{"failed and already continued", StateFailed, SourcePrompt, pgtype.Timestamptz{}, buildState{build: &buildProgressJSON{Panels: 2}, continuedBy: &continuedBy}, false},
		{"failed investigation", StateFailed, SourceInvestigate, pgtype.Timestamptz{}, withPanels, false},
		{"queued", StateQueued, SourcePrompt, pgtype.Timestamptz{}, withPanels, false},
		{"running", StateRunning, SourcePrompt, pgtype.Timestamptz{}, withPanels, false},
		{"canceling", StateCanceling, SourcePrompt, pgtype.Timestamptz{}, withPanels, false},
		{"succeeded", StateSucceeded, SourcePrompt, pgtype.Timestamptz{}, withPanels, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row := dbsqlc.AgentRequest{State: string(tc.state), Source: string(tc.source), BuildSavedAt: tc.savedAt, RequestedByKind: string(auth.KindUser), RequestedByID: requesterID}
			if got := slices.Contains(allowedActions(factsOf(row, tc.build, requester, false)), ActionContinue); got != tc.want {
				t.Fatalf("continue allowed = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestCancelOfAFinishedRequestSucceedsOnlyWhenItWasCanceled(t *testing.T) {
	canceled := dbsqlc.AgentRequest{Status: string(StatusFailed), State: string(StateCanceled), FailureCode: pgtype.Text{String: string(FailureCanceled), Valid: true}}
	row, err := canceledOrFinished(canceled)
	if err != nil || row.State != string(StateCanceled) {
		t.Fatalf("canceled request = %+v, %v; want the request and no error", row, err)
	}
	others := []dbsqlc.AgentRequest{
		{Status: string(StatusSucceeded), State: string(StateSucceeded)},
		{Status: string(StatusFailed), State: string(StateFailed), FailureCode: pgtype.Text{String: string(FailureNoProgress), Valid: true}},
		{Status: string(StatusFailed), State: string(StateFailed)},
	}
	for _, other := range others {
		if _, err := canceledOrFinished(other); !errors.Is(err, ErrRequestFinished) {
			t.Fatalf("canceledOrFinished(%+v) = %v, want ErrRequestFinished", other, err)
		}
	}
}
