package threads

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func (r *rig) adminsOnlyFolder(t *testing.T) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title, admins_only) VALUES ('Audit', true) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) moveInto(t *testing.T, table string, id, folder pgtype.UUID) {
	t.Helper()
	r.db.Exec(t, `UPDATE `+table+` SET folder_id = $2 WHERE id = $1`, id, folder)
}

func (r *rig) restrictedAnswer(t *testing.T, by auth.Principal, req revisions.StartRequest) (pgtype.UUID, findings.Finding) {
	t.Helper()
	prompt := "who changed " + sharedWord
	kind := "ask"
	req.Prompt, req.Kind = &prompt, &kind
	started, err := r.revisions.Start(reading(t.Context()), by, req)
	if err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_kind = 'observation', finished_at = now(), read_restricted = true WHERE id = $1`, started.ID)
	evidence := spec.Panel{ID: "e1", Type: "stat", Title: "errors", Datasource: "prom", Queries: []spec.Query{{Query: "up"}}}
	f, _, err := r.findings.Save(reading(t.Context()), by, findings.SaveInput{
		AgentRequestID: started.ID, AskDashboardID: started.DashboardID,
		Title: "Root changed it", Markdown: "the audit trail shows " + sharedWord, Evidence: []spec.Panel{evidence},
		SuggestedChange: "Add a panel for the changes",
		From:            time.Now().Add(-time.Hour), To: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return started.ID, f
}

func (r *rig) assertNotShared(t *testing.T, run pgtype.UUID) {
	t.Helper()
	if n := r.messageCount(t); n != 0 {
		t.Fatalf("messages after a refused share = %d, want 0", n)
	}
	stored, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThreadMessageID.Valid || stored.ObservationID.Valid {
		t.Fatalf("a refused share left the run shared: %+v", stored)
	}
}

func TestAnAnswerThatReadAdminOnlyDataCannotBeSharedWhereNonAdminsRead(t *testing.T) {
	for _, asker := range []auth.Principal{admin, testUser} {
		t.Run(string(asker.Role)+" asker", func(t *testing.T) {
			r := newShareRig(t)
			dash := r.createDashboard(t, "rps", "errors")
			id := dash.String()
			run, f := r.restrictedAnswer(t, asker, revisions.StartRequest{DashboardID: &id})
			if !f.Restricted {
				t.Fatalf("the answer of a run that read Admin-only data is not marked restricted: %+v", f)
			}
			if _, err := r.service.ShareRun(reading(t.Context()), asker, ShareInput{RunID: run}); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("show to everyone: err = %v, want the restricted answer refusal", err)
			}
			r.assertNotShared(t, run)
			if _, _, err := r.findings.SaveAsObservation(reading(t.Context()), asker, run); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("save as observation in a shared folder: err = %v", err)
			}
			r.assertNotShared(t, run)
			revise := revisions.KindRevise
			findingID := f.ID
			if _, err := r.revisions.Start(reading(t.Context()), asker, revisions.StartRequest{DashboardID: &id, Kind: &revise, AppliesFindingID: &findingID}); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("apply onto a shared dashboard: err = %v", err)
			}
		})
	}
}

func TestAnAnswerThatReadAdminOnlyDataIsSharedIntoAThreadOnlyAdminsRead(t *testing.T) {
	r := newShareRig(t)
	dash := r.createDashboard(t, "rps")
	id := dash.String()
	run, _ := r.restrictedAnswer(t, admin, revisions.StartRequest{DashboardID: &id})
	r.moveInto(t, "dashboards", dash, r.adminsOnlyFolder(t))

	here, _, err := r.service.Open(reading(t.Context()), admin, OpenInput{Target: target(KindDashboard, dash), Body: "who changed it"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := r.service.ShareRun(reading(t.Context()), admin, ShareInput{RunID: run, ThreadID: mustParse(t, here.ID)})
	if err != nil {
		t.Fatalf("share into a thread only Admins read: %v", err)
	}
	f, err := r.findingOfRun(t, admin, run)
	if err != nil || !f.Restricted || !strings.Contains(f.Markdown, sharedWord) {
		t.Fatalf("an Admin reading the thread reads the restricted answer: %+v, %v", f, err)
	}
	if _, err := r.findingOfRun(t, otherEditor, run); err == nil {
		t.Fatal("an Editor read an answer shared into a folder only Admins read")
	}
	if _, err := r.service.ReadMessage(reading(t.Context()), otherEditor, mustParse(t, shared.Message.ID)); err == nil {
		t.Fatal("an Editor read the thread in a folder only Admins read")
	}
}

func TestAnObservationAnswerThatReadAdminOnlyDataIsNotWrittenOntoASharedPage(t *testing.T) {
	r := newShareRig(t)
	observation := r.filedObservationIn(t, admin, pgtype.UUID{})
	id := observation.String()
	run, _ := r.restrictedAnswer(t, admin, revisions.StartRequest{ObservationID: &id})
	blocks := r.blockCount(t, observation)

	if _, err := r.service.ShareRun(reading(t.Context()), admin, ShareInput{RunID: run}); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("err = %v, want the restricted answer refusal", err)
	}
	if n := r.blockCount(t, observation); n != blocks {
		t.Fatalf("blocks after a refused share = %d, want %d", n, blocks)
	}

	r.moveInto(t, "observations", observation, r.adminsOnlyFolder(t))
	if _, err := r.service.ShareRun(reading(t.Context()), admin, ShareInput{RunID: run}); err != nil {
		t.Fatalf("share onto a page only Admins read: %v", err)
	}
	if n := r.blockCount(t, observation); n <= blocks {
		t.Fatalf("blocks after the share = %d, want more than %d", n, blocks)
	}
}

func TestAnAnswerThatReadNoAdminOnlyDataSharesAsBefore(t *testing.T) {
	r := newShareRig(t)
	dash := r.createDashboard(t, "rps")
	id := dash.String()
	run, _ := r.restrictedAnswer(t, admin, revisions.StartRequest{DashboardID: &id})
	r.db.Exec(t, `UPDATE agent_requests SET read_restricted = false WHERE id = $1`, run)
	if _, err := r.service.ShareRun(reading(t.Context()), admin, ShareInput{RunID: run}); err != nil {
		t.Fatalf("share of an unrestricted answer: %v", err)
	}
	f, err := r.findingOfRun(t, otherEditor, run)
	if err != nil || f.Restricted {
		t.Fatalf("a reader of the thread reads the unrestricted answer: %+v, %v", f, err)
	}
}
