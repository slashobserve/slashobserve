package revisions

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestCanCancelNeedsAnActiveRequestAndTheRequester(t *testing.T) {
	requesterID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	otherID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	requester := auth.Principal{Kind: auth.KindUser, ID: requesterID.String()}
	tests := []struct {
		name   string
		status Status
		by     auth.Principal
		want   bool
	}{
		{"requester while queued", StatusQueued, requester, true},
		{"requester while running", StatusRunning, requester, true},
		{"requester after success", StatusSucceeded, requester, false},
		{"requester after failure", StatusFailed, requester, false},
		{"another user while queued", StatusQueued, auth.Principal{Kind: auth.KindUser, ID: otherID.String()}, false},
		{"another user while running", StatusRunning, auth.Principal{Kind: auth.KindUser, ID: otherID.String()}, false},
		{"service account with the requester's id", StatusRunning, auth.Principal{Kind: auth.KindServiceAccount, ID: requesterID.String()}, false},
		{"administrator who is not the requester", StatusRunning, auth.Principal{Kind: auth.KindUser, ID: otherID.String(), Role: auth.RoleAdmin}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row := dbsqlc.AgentRequest{Status: string(tc.status), RequestedByKind: string(auth.KindUser), RequestedByID: requesterID}
			if got := canCancel(row, tc.by); got != tc.want {
				t.Fatalf("canCancel = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestContinueBlockerAllowsOnlyAnUnsavedFailedRequest(t *testing.T) {
	saved := pgtype.Timestamptz{Time: time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC), Valid: true}
	tests := []struct {
		name    string
		status  Status
		savedAt pgtype.Timestamptz
		wantErr error
	}{
		{"failed and unsaved", StatusFailed, pgtype.Timestamptz{}, nil},
		{"failed and saved", StatusFailed, saved, ErrNotContinuable},
		{"queued", StatusQueued, pgtype.Timestamptz{}, ErrNotContinuable},
		{"running", StatusRunning, pgtype.Timestamptz{}, ErrNotContinuable},
		{"succeeded", StatusSucceeded, pgtype.Timestamptz{}, ErrNotContinuable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := continueBlocker(dbsqlc.AgentRequest{Status: string(tc.status), BuildSavedAt: tc.savedAt})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("continueBlocker = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestCancelOfAFinishedRequestSucceedsOnlyWhenItWasCanceled(t *testing.T) {
	canceled := dbsqlc.AgentRequest{Status: string(StatusFailed), FailureCode: pgtype.Text{String: string(FailureCanceled), Valid: true}}
	row, err := canceledOrFinished(canceled)
	if err != nil || row.Status != string(StatusFailed) {
		t.Fatalf("canceled request = %+v, %v; want the request and no error", row, err)
	}
	others := []dbsqlc.AgentRequest{
		{Status: string(StatusSucceeded)},
		{Status: string(StatusFailed), FailureCode: pgtype.Text{String: string(FailureNoProgress), Valid: true}},
		{Status: string(StatusFailed)},
	}
	for _, other := range others {
		if _, err := canceledOrFinished(other); !errors.Is(err, ErrRequestFinished) {
			t.Fatalf("canceledOrFinished(%+v) = %v, want ErrRequestFinished", other, err)
		}
	}
}
