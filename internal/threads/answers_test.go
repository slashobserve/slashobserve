package threads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
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
	page, err := r.service.List(reading(t.Context()), by, ListQuery{Target: tg, Status: StatusAll, Limit: 50, WithAnswers: true})
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
	target                                                             Target
	open, kept, shared, unshared, redacted, savedHidden, build, failed pgtype.UUID
}

func (r *rig) replyNaming(t *testing.T, tg Target, run pgtype.UUID) {
	t.Helper()
	opened, _ := r.open(t, testUser, OpenInput{Target: tg, Version: new(1), PanelID: new("rps"), Body: "a reply row that names a run"})
	r.db.Exec(t, `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name, run_id, run_assigned_kind) VALUES ($1, 'Answered.', 'user', $2, 'Ada', $3, 'builtin')`, mustParse(t, opened.ID), mustParse(t, testUserID), run)
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
	r.replyNaming(t, tg, p.unshared)

	p.redacted = r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "errors")
	r.db.Exec(t, `UPDATE agent_requests SET kind = 'build', intent = 'apply', context_from = NULL, context_to = NULL WHERE id = $1`, p.redacted)
	r.replyNaming(t, tg, p.redacted)

	_, build := r.mentionRun(t, tg)
	if err := r.runEnded(t, succeededWithVersion(build, 1)); err != nil {
		t.Fatal(err)
	}
	p.build = build.ID

	_, failed := r.mentionRun(t, tg)
	if err := r.runEnded(t, ended(failed, revisions.StateFailed)); err != nil {
		t.Fatal(err)
	}
	p.failed = failed.ID

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
	runs := map[string]pgtype.UUID{"open": p.open, "kept": p.kept, "shared": p.shared, "unshared": p.unshared, "a chat seen redacted": p.redacted, "saved in a hidden folder": p.savedHidden, "a build": p.build, "a failed run": p.failed}

	for _, reader := range []auth.Principal{admin, testUser, otherEditor, viewer, robotKey} {
		t.Run(reader.Name, func(t *testing.T) {
			listed := r.answersByRun(t, reader, p.target)
			if a := listed[p.open.String()]; a == nil {
				t.Fatalf("%s reads no answer for the open run", reader.Name)
			}
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

type countingGate struct {
	RunGate
	asked map[pgtype.UUID]int
}

func (g *countingGate) ShowsRunAnswer(ctx context.Context, run pgtype.UUID, by auth.Principal) (bool, error) {
	g.asked[run]++
	return g.RunGate.ShowsRunAnswer(ctx, run, by)
}

func TestAThreadListDecidesAccessOnlyForRunsThatSavedAnAnswer(t *testing.T) {
	r := newShareRig(t)
	p := r.mixedAnswerPage(t)
	gate := &countingGate{RunGate: r.revisions, asked: map[pgtype.UUID]int{}}
	r.service.ReadAnswersWith(gate, findingAnswers{r.findings})

	r.answersByRun(t, viewer, p.target)
	for name, run := range map[string]pgtype.UUID{"a build": p.build, "a failed run": p.failed} {
		if n := gate.asked[run]; n != 0 {
			t.Errorf("%s with no answer was read %d times", name, n)
		}
	}
	for name, run := range map[string]pgtype.UUID{"open": p.open, "kept": p.kept, "shared": p.shared} {
		if n := gate.asked[run]; n != 1 {
			t.Errorf("%s was read %d times, want once", name, n)
		}
	}
}

func TestAThreadListRecordsNoReadOfTheRunsItAnswers(t *testing.T) {
	r := newAnswersRig(t)
	p := r.mixedAnswerPage(t)
	var before int64
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT coalesce(max(seq), 0) FROM audit_events`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	recorder := audit.New(audit.Config{DB: r.db.DB, Server: "test", Version: "test", Logger: slog.New(slog.DiscardHandler)})
	audit.Install(recorder)
	t.Cleanup(func() { audit.Install(nil) })
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(ctx)
	}()

	scoped := func(who auth.Principal) context.Context {
		return audit.Open(auth.WithPrincipal(reading(t.Context()), who), audit.Scope{Actor: auth.AuditActor(who), Initiator: audit.InitiatorPerson, Origin: audit.Origin{Kind: "mcp"}})
	}
	for _, who := range []auth.Principal{admin, testUser, otherEditor, viewer, robotKey} {
		if _, err := r.service.List(scoped(who), who, ListQuery{Target: p.target, Status: StatusAll, Limit: 50, WithAnswers: true}); err != nil {
			t.Fatal(err)
		}
	}
	desk := revisions.NewRunDesk(r.revisions, revisions.ObserveOptions{})
	if _, err := desk.Run(scoped(viewer), p.open, viewer, 0); err != nil {
		t.Fatal(err)
	}
	stop()
	<-done

	var events []string
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT action || ' ' || coalesce(actor_id::text, '-') || ' ' || coalesce(target_id, '-') FROM audit_events WHERE seq > $1 ORDER BY seq`, before)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"run.view " + viewerID + " " + p.open.String()}
	if !slices.Equal(events, want) {
		t.Fatalf("audit events = %q, want only the run read %q", events, want)
	}
}
