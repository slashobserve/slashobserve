package threads

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auditfolder"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func (r *rig) adminsOnlyFolder(t *testing.T) pgtype.UUID {
	t.Helper()
	return r.adminsOnlyFolderNamed(t, "Audit")
}

func (r *rig) adminsOnlyFolderNamed(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title, admins_only) VALUES ($1, true) RETURNING id`, title).Scan(&id); err != nil {
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

func (r *rig) builtInAuditFolder(t *testing.T) pgtype.UUID {
	t.Helper()
	folder := r.adminsOnlyFolder(t)
	r.db.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, auditfolder.FolderKey, folder)
	return folder
}

func (r *rig) savedInto(t *testing.T, run pgtype.UUID) pgtype.UUID {
	t.Helper()
	var folder pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT o.folder_id FROM observations o JOIN findings f ON f.observation_id = o.id WHERE f.agent_request_id = $1`, run).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	return folder
}

func TestAnAdminsPrivateAskOnAnAuditDashboardSavesItsAnswerInTheAuditFolder(t *testing.T) {
	r := newShareRig(t)
	dash := r.createDashboard(t, "rps")
	audit := r.builtInAuditFolder(t)
	r.moveInto(t, "dashboards", dash, audit)
	id := dash.String()
	run, _ := r.restrictedAnswer(t, admin, revisions.StartRequest{DashboardID: &id})

	saved, _, err := r.findings.SaveAsObservation(reading(t.Context()), admin, run)
	if err != nil || !saved.Restricted {
		t.Fatalf("save the answer as an observation: restricted %v, %v", saved.Restricted, err)
	}
	if folder := r.savedInto(t, run); folder != audit {
		t.Fatalf("the observation is in folder %v, want the Audit folder %v", folder, audit)
	}
	if _, err := r.findingOfRun(t, otherEditor, run); err == nil {
		t.Fatal("an Editor read the answer saved in the Audit folder")
	}
}

func TestARestrictedAnswerIsSavedWhereItsSubjectIsElseInTheAuditFolder(t *testing.T) {
	r := newShareRig(t)
	audit := r.builtInAuditFolder(t)
	other := r.adminsOnlyFolderNamed(t, "Payroll")
	inOther := r.createDashboard(t, "rps")
	r.moveInto(t, "dashboards", inOther, other)
	shared := r.createDashboard(t, "errors")
	cases := map[string]struct {
		req  revisions.StartRequest
		want pgtype.UUID
	}{
		"an ask with no subject":                              {revisions.StartRequest{}, audit},
		"an ask on a dashboard in a shared folder":            {revisions.StartRequest{DashboardID: new(shared.String())}, audit},
		"an ask on a dashboard in another Only Admins folder": {revisions.StartRequest{DashboardID: new(inOther.String())}, other},
	}
	for name, tc := range cases {
		run, _ := r.restrictedAnswer(t, admin, tc.req)
		if _, _, err := r.findings.SaveAsObservation(reading(t.Context()), admin, run); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if folder := r.savedInto(t, run); folder != tc.want {
			t.Fatalf("%s: saved into %v, want %v", name, folder, tc.want)
		}
	}
}

func TestARestrictedAnswerWithNoOnlyAdminsFolderToGoToIsNotSaved(t *testing.T) {
	r := newShareRig(t)
	run, _ := r.restrictedAnswer(t, admin, revisions.StartRequest{})
	if _, _, err := r.findings.SaveAsObservation(reading(t.Context()), admin, run); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("err = %v, want the restricted answer refusal", err)
	}
	r.assertNotShared(t, run)
}

func TestAnUnrestrictedAnswerIsSavedInTheDefaultFolderEvenWithAnAuditFolder(t *testing.T) {
	r := newShareRig(t)
	audit := r.builtInAuditFolder(t)
	run, _ := r.restrictedAnswer(t, admin, revisions.StartRequest{})
	r.db.Exec(t, `UPDATE agent_requests SET read_restricted = false WHERE id = $1`, run)
	if _, _, err := r.findings.SaveAsObservation(reading(t.Context()), admin, run); err != nil {
		t.Fatal(err)
	}
	if folder := r.savedInto(t, run); folder == audit {
		t.Fatal("an unrestricted answer was saved into the Audit folder")
	}
}

func TestAnAnswerSavedAfterItsSubjectGainedARestrictedAnswerIsMarkedLikeIt(t *testing.T) {
	r := newShareRig(t)
	observation := r.filedObservationIn(t, admin, r.adminsOnlyFolder(t))
	id := observation.String()
	prompt, kind := "what else changed", "ask"
	started, err := r.revisions.Start(reading(t.Context()), admin, revisions.StartRequest{ObservationID: &id, Prompt: &prompt, Kind: &kind})
	if err != nil || started.ReadRestricted {
		t.Fatalf("the ask before its subject held a restricted answer: marked %v, %v", started.ReadRestricted, err)
	}
	earlier, _ := r.restrictedAnswer(t, admin, revisions.StartRequest{})
	r.db.Exec(t, `INSERT INTO observation_blocks (observation_id, kind, position, body, created_by_kind, created_by_id, created_by_name, run_id) VALUES ($1, 'text', 99, '{"markdown":"Root changed it"}', 'user', gen_random_uuid(), 'Root', $2)`, observation, earlier)

	r.db.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_kind = 'observation', finished_at = now() WHERE id = $1`, started.ID)
	f, _, err := r.findings.Save(reading(t.Context()), admin, findings.SaveInput{AgentRequestID: started.ID, Title: "More changes", Markdown: "the alerts too", From: time.Now().Add(-time.Hour), To: time.Now()})
	if err != nil || !f.Restricted {
		t.Fatalf("the answer saved after its subject held a restricted answer: restricted %v, %v", f.Restricted, err)
	}
}

func TestARunThatReadAdminOnlyDataRepliesWithoutItsAnswerWhereNonAdminsRead(t *testing.T) {
	cases := []struct {
		name       string
		adminsOnly bool
		want       string
	}{
		{"a thread others read", false, replyRestricted},
		{"a thread only Admins read", true, "Saved version 3 of this dashboard."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newAgentRig(t, true)
			dash := r.createDashboard(t, "rps", "errors")
			thread, run := r.mentionRun(t, target(KindDashboard, dash))
			r.db.Exec(t, `UPDATE agent_requests SET read_restricted = true WHERE id = $1`, run.ID)
			if tc.adminsOnly {
				r.moveInto(t, "dashboards", dash, r.adminsOnlyFolder(t))
			}

			if err := r.runEnded(t, succeededWithVersion(run, 3)); err != nil {
				t.Fatal(err)
			}

			replies := r.runReplies(t, thread)
			if len(replies) != 1 || replies[0].Body != tc.want {
				t.Fatalf("replies = %+v, want one with %q", replies, tc.want)
			}
		})
	}
}
