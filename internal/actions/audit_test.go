package actions

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/egress"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

func recordedRig(t *testing.T) (*rig, *dbtest.Database, map[string]string) {
	t.Helper()
	d := dbtest.New(t)
	audit.Install(audit.New(audit.Config{DB: d.DB}))
	t.Cleanup(func() { audit.Install(nil) })
	person := uuid.NewString()
	return newRig(t), d, map[string]string{"X-Test-Kind": "user", "X-Test-User": person, "X-Test-Edit": "yes"}
}

func TestRunRecordsOneEventForTheCallerWithNoDetailsOfTheCall(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		status      int
		wantOutcome string
		wantReason  string
	}{
		{"ok", nil, http.StatusNoContent, "success", ""},
		{"remote error", nil, http.StatusInternalServerError, "failed", "remote_error"},
		{"refused", &egress.HostError{Host: "deploy.example.com", Err: egress.ErrNotAllowed}, 0, "denied", "refused"},
		{"unavailable", &egress.HostError{Host: "deploy.example.com", Err: egress.ErrUnavailable}, 0, "failed", "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, d, headers := recordedRig(t)
			r.sender.result, r.sender.err = egress.SendResult{Status: tc.status}, tc.err
			r.post(validBody(), headers)

			want := dbtest.AuditEvent{Action: "action.run", Outcome: tc.wantOutcome, Reason: tc.wantReason, ActorKind: "user", ActorID: headers["X-Test-User"], Initiator: "person", Origin: "job", TargetKind: "dashboard", TargetID: dashboardID}
			if got := d.AuditEvents(t, "action.run"); fmt.Sprint(got) != fmt.Sprint([]dbtest.AuditEvent{want}) {
				t.Errorf("events %+v, want %+v", got, want)
			}
		})
	}
}

func TestRunRecordsNoEventWhenNothingWasSent(t *testing.T) {
	r, d, headers := recordedRig(t)
	r.post(with(validBody(), "version", 6), headers)
	r.post(with(validBody(), "action", 5), headers)
	if len(r.sender.calls()) != 0 {
		t.Fatal("a request that should be refused was sent")
	}
	if n := len(d.AuditEvents(t, "action.run")); n != 0 {
		t.Errorf("events = %d, want 0", n)
	}
}

func TestRunRecordsTheEventWhenTheCallerDisconnectsAfterTheActionWasSent(t *testing.T) {
	r, d, headers := recordedRig(t)
	ctx, disconnect := context.WithCancel(t.Context())
	r.sender.after = disconnect
	r.postWithContext(ctx, validBody(), headers)

	if n := len(d.AuditEvents(t, "action.run")); n != 1 {
		t.Errorf("events = %d, want the sent action recorded although the caller left", n)
	}
}
