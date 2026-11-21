package threads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var robotKey = auth.Principal{Kind: auth.KindServiceAccount, ID: "77777777-7777-4777-8777-777777777777", Name: "ci", Role: auth.RoleViewer}

type findingAnswers struct {
	findings *findings.Service
}

func (a findingAnswers) RunAnswer(ctx context.Context, by auth.Principal, run pgtype.UUID) (Answer, bool, error) {
	f, found, err := a.findings.ForRequest(ctx, by, run)
	if err != nil || !found {
		return Answer{}, false, err
	}
	raw, err := json.Marshal(findings.JSONOf(f, ""))
	if err != nil {
		return Answer{}, false, err
	}
	return Answer{RunID: run.String(), FindingID: f.ID, Title: f.Title, ObservationID: f.ObservationID, CanSave: f.SavableBy(by), Finding: raw}, true, nil
}

func (r *rig) singleRead(t *testing.T, by auth.Principal, run pgtype.UUID) json.RawMessage {
	t.Helper()
	desk := revisions.NewRunDesk(r.revisions, revisions.ObserveOptions{Finding: func(ctx context.Context, by auth.Principal, run pgtype.UUID) (json.RawMessage, bool, error) {
		a, found, err := findingAnswers{r.findings}.RunAnswer(ctx, by, run)
		return a.Finding, found, err
	}})
	raw, err := desk.Run(reading(t.Context()), run, by, 0)
	if errors.Is(err, revisions.ErrRequestNotFound) {
		return nil
	}
	if err != nil {
		t.Fatalf("%s reads run %s: %v", by.Name, run, err)
	}
	var shown struct {
		Finding json.RawMessage `json:"finding"`
	}
	if err := json.Unmarshal(raw, &shown); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(shown.Finding, []byte("null")) {
		return nil
	}
	return shown.Finding
}

func newAnswersRig(t *testing.T) *rig {
	t.Helper()
	r := newShareRig(t)
	r.service.ReadAnswersWith(r.revisions, findingAnswers{r.findings})
	return r
}

func (r *rig) answeredMentionRun(t *testing.T, tg Target) (dbsqlc.AgentRequest, findings.Finding) {
	t.Helper()
	_, run := r.mentionRun(t, tg)
	return r.answerMention(t, run)
}

func (r *rig) answerMention(t *testing.T, run dbsqlc.AgentRequest) (dbsqlc.AgentRequest, findings.Finding) {
	t.Helper()
	f := r.answer(t, testUser, findings.SaveInput{AgentRequestID: run.ID})
	finished := ended(run, revisions.StateSucceeded)
	finished.ResultKind = string(revisions.ResultObservation)
	if err := r.runEnded(t, finished); err != nil {
		t.Fatal(err)
	}
	return run, f
}

func (r *rig) answersByRun(t *testing.T, by auth.Principal, tg Target) map[string]*Answer {
	t.Helper()
	page, err := r.service.List(reading(t.Context()), by, ListQuery{Target: tg, Status: StatusAll, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	runOf := map[string]string{}
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT id::text, run_id::text FROM thread_messages WHERE run_id IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, run string
		if err := rows.Scan(&id, &run); err != nil {
			t.Fatal(err)
		}
		runOf[id] = run
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	out := map[string]*Answer{}
	for _, m := range page.Messages {
		if run, ok := runOf[m.ID]; ok {
			out[run] = m.Answer
		}
	}
	return out
}

type answerPage struct {
	target                                    Target
	open, kept, shared, unshared, savedHidden pgtype.UUID
}

func (r *rig) mixedAnswerPage(t *testing.T) answerPage {
	t.Helper()
	dash := r.createDashboard(t, "rps", "errors")
	tg := target(KindDashboard, dash)
	p := answerPage{target: tg}

	open, _ := r.answeredMentionRun(t, tg)
	p.open = open.ID

	id := dash.String()
	p.shared = r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "errors")
	if _, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: p.shared}); err != nil {
		t.Fatal(err)
	}

	p.unshared = r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "errors")
	openThread, _ := r.open(t, testUser, OpenInput{Target: tg, Version: new(1), PanelID: new("rps"), Body: "a reply row that names a private ask"})
	r.db.Exec(t, `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name, run_id, run_assigned_kind) VALUES ($1, 'Answered.', 'user', $2, 'Ada', $3, 'builtin')`, mustParse(t, openThread.ID), mustParse(t, testUserID), p.unshared)

	saved, _ := r.answeredMentionRun(t, tg)
	if _, _, err := r.findings.SaveAsObservation(reading(t.Context()), testUser, saved.ID); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = (SELECT observation_id FROM findings WHERE agent_request_id = $1)`, saved.ID, foldertest.Restricted(t, r.db, "Payments", "payments"))
	p.savedHidden = saved.ID

	_, keptRun := r.mentionRun(t, tg)
	r.answer(t, testUser, findings.SaveInput{AgentRequestID: keptRun.ID})
	r.db.Exec(t, `UPDATE agent_requests SET read_restricted = true WHERE id = $1`, keptRun.ID)
	keptRun.ReadRestricted = true
	keptEnd := ended(keptRun, revisions.StateSucceeded)
	keptEnd.ResultKind = string(revisions.ResultObservation)
	if err := r.runEnded(t, keptEnd); err != nil {
		t.Fatal(err)
	}
	p.kept = keptRun.ID
	return p
}

func TestAThreadListGivesEachReaderTheAnswersTheSingleReadGivesThem(t *testing.T) {
	r := newAnswersRig(t)
	p := r.mixedAnswerPage(t)
	runs := map[string]pgtype.UUID{"open": p.open, "kept": p.kept, "shared": p.shared, "unshared": p.unshared, "saved in a hidden folder": p.savedHidden}

	for _, reader := range []auth.Principal{admin, testUser, otherEditor, viewer, robotKey} {
		t.Run(reader.Name, func(t *testing.T) {
			listed := r.answersByRun(t, reader, p.target)
			for name, run := range runs {
				single := r.singleRead(t, reader, run)
				var batch json.RawMessage
				if a := listed[run.String()]; a != nil {
					batch = a.Finding
				}
				if !jsonEqual(t, batch, single) {
					t.Errorf("%s: the thread list gives %s, the single read %s", name, batch, single)
				}
			}
		})
	}
}

func TestAThreadListKeepsAKeptAnswerWithItsAsker(t *testing.T) {
	r := newAnswersRig(t)
	p := r.mixedAnswerPage(t)

	for _, tt := range []struct {
		reader auth.Principal
		reads  bool
	}{{admin, true}, {testUser, true}, {otherEditor, false}, {viewer, false}, {robotKey, false}} {
		a := r.answersByRun(t, tt.reader, p.target)[p.kept.String()]
		if (a != nil) != tt.reads {
			t.Errorf("%s reads the kept answer: %v, want %v", tt.reader.Name, a != nil, tt.reads)
			continue
		}
		if a == nil {
			continue
		}
		var f findings.FindingJSON
		if err := json.Unmarshal(a.Finding, &f); err != nil || !f.Restricted {
			t.Errorf("%s reads the kept answer without restricted: %+v, %v", tt.reader.Name, f, err)
		}
	}
}

func TestAThreadListNamesNoObservationItsReaderCannotSee(t *testing.T) {
	r := newAnswersRig(t)
	p := r.mixedAnswerPage(t)

	for _, reader := range []auth.Principal{testUser, otherEditor, viewer, robotKey} {
		a := r.answersByRun(t, reader, p.target)[p.savedHidden.String()]
		if a == nil {
			t.Errorf("%s lost the answer asked in a thread they read", reader.Name)
			continue
		}
		var f findings.FindingJSON
		if err := json.Unmarshal(a.Finding, &f); err != nil || f.ObservationID != nil || a.ObservationID != "" || a.CanSave {
			t.Errorf("%s reads the saved answer as %+v (observation %q, can_save %v), %v", reader.Name, f, a.ObservationID, a.CanSave, err)
		}
	}
}

func TestAThreadListGivesAPrivateAskOnlyToWhoReadsItInFull(t *testing.T) {
	r := newAnswersRig(t)
	p := r.mixedAnswerPage(t)

	for _, tt := range []struct {
		reader           auth.Principal
		shared, unshared bool
	}{{admin, true, true}, {testUser, true, true}, {otherEditor, true, false}, {viewer, true, false}, {robotKey, true, false}} {
		listed := r.answersByRun(t, tt.reader, p.target)
		if got := listed[p.shared.String()] != nil; got != tt.shared {
			t.Errorf("%s reads the shared private ask: %v, want %v", tt.reader.Name, got, tt.shared)
		}
		if got := listed[p.unshared.String()] != nil; got != tt.unshared {
			t.Errorf("%s reads a private ask that was never shared: %v, want %v", tt.reader.Name, got, tt.unshared)
		}
	}
}

func TestAThreadListOffersSavingOnlyToTheAsker(t *testing.T) {
	r := newAnswersRig(t)
	tg := target(KindDashboard, r.createDashboard(t, "rps", "errors"))
	run, f := r.answeredMentionRun(t, tg)

	for _, tt := range []struct {
		reader  auth.Principal
		canSave bool
	}{{testUser, true}, {admin, false}, {viewer, false}} {
		a := r.answersByRun(t, tt.reader, tg)[run.ID.String()]
		if a == nil || a.FindingID != f.ID || a.Title != "Checkout is slow" || a.CanSave != tt.canSave {
			t.Errorf("%s reads %+v, want finding %s with can_save %v", tt.reader.Name, a, f.ID, tt.canSave)
		}
	}
}

func TestAThreadListWithoutAnAnswerReaderCarriesNoAnswers(t *testing.T) {
	r := newShareRig(t)
	tg := target(KindDashboard, r.createDashboard(t, "rps", "errors"))
	run, _ := r.answeredMentionRun(t, tg)

	if a := r.answersByRun(t, testUser, tg)[run.ID.String()]; a != nil {
		t.Fatalf("answer = %+v with no answer reader set", a)
	}
}

func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	var ca, cb bytes.Buffer
	if err := json.Compact(&ca, a); err != nil {
		t.Fatal(err)
	}
	if err := json.Compact(&cb, b); err != nil {
		t.Fatal(err)
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}
