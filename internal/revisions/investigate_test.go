package revisions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var investigateWindowTo = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

func investigate(t *testing.T, svc *revisions.Service, p revisions.InvestigateParams) dbsqlc.AgentRequest {
	t.Helper()
	if p.Prompt == "" {
		p.Prompt = "why are checkout errors up"
	}
	if p.By.ID == "" {
		p.By = user
	}
	row, err := svc.SubmitInvestigate(context.Background(), p)
	if err != nil {
		t.Fatalf("investigate: %v", err)
	}
	return row
}

func insertFindingFor(t *testing.T, d *dbtest.Database, requestID pgtype.UUID) {
	t.Helper()
	d.Exec(t, `WITH o AS (INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name)
  VALUES (general_folder_id(), 'why', 'user', $2::uuid, 'Ada') RETURNING id),
r AS (UPDATE agent_requests SET observation_id = (SELECT id FROM o) WHERE id = $1)
INSERT INTO findings (id, observation_id, agent_request_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
SELECT o.id, o.id, $1, 'why', 'checkout-0 errors', now() - interval '1 hour', now(), 'user', $2::uuid, 'Ada' FROM o`, requestID, userID)
}

func TestInvestigateStoresItsWindow(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	from := investigateWindowTo.Add(-time.Hour)
	row := investigate(t, svc, revisions.InvestigateParams{From: &from, To: &investigateWindowTo})
	if row.Source != string(revisions.SourceInvestigate) || !row.ContextFrom.Time.Equal(from) || !row.ContextTo.Time.Equal(investigateWindowTo) || row.DashboardID.Valid {
		t.Fatalf("enqueued = %+v", row)
	}
}

func TestInvestigateCapsEachRequester(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	for range revisions.MaxActiveInvestigatePerPrincipal {
		investigate(t, svc, revisions.InvestigateParams{})
	}
	_, err := svc.SubmitInvestigate(context.Background(), revisions.InvestigateParams{Prompt: "one more", By: user})
	if !errors.Is(err, revisions.ErrTooManyRequests) || !errors.Is(err, revisions.ErrTooManyInvestigations) {
		t.Fatalf("the fourth investigation: %v", err)
	}
	if _, err := svc.SubmitObserve(context.Background(), user, "a dashboard still queues"); err != nil {
		t.Fatalf("observe beside three investigations: %v", err)
	}
}

func TestInvestigateNeedsAnEditorAndAKnownPanel(t *testing.T) {
	panels := &fakePanels{known: map[string]bool{"errors": true}}
	svc, d := newPanelService(t, panels)
	dash := seedDashboard(t, d, 1)
	viewer := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Vic", Role: auth.RoleViewer}
	if _, err := svc.SubmitInvestigate(context.Background(), revisions.InvestigateParams{Prompt: "why", By: viewer}); !errors.Is(err, revisions.ErrForbidden) {
		t.Errorf("a viewer: %v", err)
	}
	if _, err := svc.SubmitInvestigate(context.Background(), revisions.InvestigateParams{Prompt: "why", By: user, DashboardID: dash, PanelID: "nope"}); !errors.Is(err, revisions.ErrUnknownPanel) {
		t.Errorf("an unknown panel: %v", err)
	}
	row := investigate(t, svc, revisions.InvestigateParams{DashboardID: dash, PanelID: "errors"})
	if row.PanelID.String != "errors" || row.DashboardID != dash {
		t.Errorf("enqueued = %+v", row)
	}
}

func TestInvestigationsNeverHoldTheDashboard(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	investigation := investigate(t, svc, revisions.InvestigateParams{DashboardID: dash})
	if claimed, found := claimOne(t, svc); !found || claimed.ID != investigation.ID {
		t.Fatalf("claimed %+v, want the investigation", claimed)
	}
	active, err := d.Queries().CountActiveAgentRequests(context.Background(), dash)
	if err != nil || active != 0 {
		t.Fatalf("active revisions = %d, %v", active, err)
	}
	rev := submit(t, svc, dash, "add a panel")
	claimed, found := claimOne(t, svc)
	if !found || claimed.ID.String() != rev.ID {
		t.Fatalf("claimed %+v, want the revision beside the running investigation", claimed)
	}
	if _, err := svc.Get(context.Background(), dash, investigation.ID); !errors.Is(err, revisions.ErrNotFound) {
		t.Errorf("the investigation in the revision history: %v", err)
	}
}

func TestInvestigationsCannotBeContinued(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	row := investigate(t, svc, revisions.InvestigateParams{})
	d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed', failure_code = 'no_finding' WHERE id = $1", row.ID)
	if _, err := svc.Continue(context.Background(), row.ID, user, ""); !errors.Is(err, revisions.ErrNotContinuable) {
		t.Fatalf("continue: %v", err)
	}
}

func TestCancelOfAQueuedInvestigationFailsIt(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	row := investigate(t, svc, revisions.InvestigateParams{})
	got, err := svc.Cancel(context.Background(), row.ID, user)
	if err != nil || got.Status != string(revisions.StatusFailed) || got.FailureCode.String != string(revisions.FailureCanceled) {
		t.Fatalf("cancel: %+v, %v", got, err)
	}
}

func TestAnExhaustedInvestigationWithAFindingSucceeds(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	row := investigate(t, svc, revisions.InvestigateParams{})
	insertFindingFor(t, d, row.ID)
	d.Exec(t, "UPDATE agent_requests SET state = 'running', status = 'running', attempts = $2, lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", row.ID, revisions.MaxAttempts)
	claimOne(t, svc)
	got, err := svc.GetByID(context.Background(), row.ID)
	if err != nil || got.Status != string(revisions.StatusSucceeded) || got.FailureCode.Valid {
		t.Fatalf("exhausted investigation = %+v, %v", got, err)
	}
}

func TestTheWorkerFinishesAnInvestigationByItsFinding(t *testing.T) {
	cases := []struct {
		name        string
		saveFinding bool
		wantStatus  revisions.Status
		wantCode    string
	}{
		{"saved", true, revisions.StatusSucceeded, ""},
		{"none saved", false, revisions.StatusFailed, string(revisions.FailureNoFinding)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var d *dbtest.Database
			var got revisions.Request
			var svc *revisions.Service
			svc, d = newService(t, reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
				got = req
				if !c.saveFinding {
					return revisions.Result{}, nil
				}
				insertFindingFor(t, d, mustUUID(t, req.ID))
				return revisions.Result{FindingID: "saved"}, nil
			}))
			from := investigateWindowTo.Add(-time.Hour)
			row := investigate(t, svc, revisions.InvestigateParams{From: &from, To: &investigateWindowTo})
			startWorker(t, svc, d, revisions.WorkerOptions{})
			var final dbsqlc.AgentRequest
			waitFor(t, "the investigation to finish", func() bool {
				var err error
				final, err = svc.GetByID(context.Background(), row.ID)
				return err == nil && final.FinishedAt.Valid
			})
			if final.Status != string(c.wantStatus) || final.FailureCode.String != c.wantCode {
				t.Fatalf("finished %s with %q, want %s with %q", final.Status, final.FailureCode.String, c.wantStatus, c.wantCode)
			}
			if got.Source != revisions.SourceInvestigate || got.From == nil || !got.From.Equal(from) || got.To == nil || !got.To.Equal(investigateWindowTo) {
				t.Errorf("request = %+v", got)
			}
		})
	}
}

func TestTheWorkerFinishesAnAnsweredRequestByItsFinding(t *testing.T) {
	cases := []struct {
		name   string
		source revisions.Source
		submit func(*revisions.Service, pgtype.UUID) (dbsqlc.AgentRequest, error)
	}{
		{"a new-dashboard request", revisions.SourceObserve, func(svc *revisions.Service, _ pgtype.UUID) (dbsqlc.AgentRequest, error) {
			return svc.SubmitObserve(context.Background(), user, "what all logs are available?")
		}},
		{"a dashboard chat prompt", revisions.SourcePrompt, func(svc *revisions.Service, dash pgtype.UUID) (dbsqlc.AgentRequest, error) {
			return svc.SubmitFollowUp(context.Background(), dash, nil, "", "what does this dashboard show?", user)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var d *dbtest.Database
			var svc *revisions.Service
			svc, d = newService(t, reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
				insertFindingFor(t, d, mustUUID(t, req.ID))
				return revisions.Result{FindingID: "answered"}, nil
			}))
			dash := seedDashboard(t, d, 1)
			row, err := c.submit(svc, dash)
			if err != nil {
				t.Fatal(err)
			}
			startWorker(t, svc, d, revisions.WorkerOptions{})
			var final dbsqlc.AgentRequest
			waitFor(t, "the request to finish", func() bool {
				var err error
				final, err = svc.GetByID(context.Background(), row.ID)
				return err == nil && final.FinishedAt.Valid
			})
			if final.Status != string(revisions.StatusSucceeded) || final.FailureCode.Valid || final.ResultVersion.Valid || final.DashboardID != row.DashboardID {
				t.Fatalf("finished %s with %q, version %v, dashboard %v", final.Status, final.FailureCode.String, final.ResultVersion, final.DashboardID)
			}
			if final.Source != string(c.source) {
				t.Fatalf("source %q", final.Source)
			}
		})
	}
}
