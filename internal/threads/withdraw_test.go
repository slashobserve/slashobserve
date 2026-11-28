package threads

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) sharedDashboardAnswer(t *testing.T) (pgtype.UUID, pgtype.UUID, Shared) {
	t.Helper()
	dash := r.createDashboard(t, "rps", "errors")
	id := dash.String()
	run := r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "errors")
	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil {
		t.Fatal(err)
	}
	return dash, run, shared
}

func TestTheAskerWithdrawsASharedAnswerBackToOnlyThem(t *testing.T) {
	r := newShareRig(t)
	_, run, shared := r.sharedDashboardAnswer(t)
	r.reply(t, testUser, shared.Thread.ID, "and the latency too")

	if _, err := r.findingOfRun(t, otherEditor, run); err != nil {
		t.Fatalf("a second viewer reads the shared answer before the withdraw: %v", err)
	}
	if err := r.service.Withdraw(reading(t.Context()), otherEditor, mustParse(t, shared.Thread.ID)); !errors.Is(err, ErrNothingShared) {
		t.Fatalf("a second viewer withdrew the asker's answer: %v", err)
	}

	if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); err != nil {
		t.Fatal(err)
	}
	if n := r.messageCount(t); n != 0 {
		t.Fatalf("messages after the withdraw = %d, want 0", n)
	}
	stored, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThreadMessageID.Valid || stored.ThreadRepliedAt.Valid || !revisions.PrivateAsk(stored) {
		t.Fatalf("the withdrawn run is %+v", stored)
	}
	if f, err := r.findingOfRun(t, testUser, run); err != nil || f.ObservationID != "" {
		t.Fatalf("the asker reads the answer after the withdraw: %+v, %v", f, err)
	}
	if _, err := r.findingOfRun(t, otherEditor, run); !errors.Is(err, findings.ErrNotFound) {
		t.Fatalf("a second viewer reads the answer after the withdraw: %v", err)
	}
	if _, err := r.service.ReadMessage(reading(t.Context()), otherEditor, mustParse(t, shared.Message.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a second viewer reads the withdrawn thread: %v", err)
	}
	events := r.db.AuditEvents(t, "run.withdraw")
	if len(events) != 1 || events[0].TargetKind != "run" || events[0].TargetID != run.String() || events[0].ActorID != testUserID {
		t.Fatalf("run.withdraw events = %+v, want one for the run by its requester", events)
	}
	answer, err := r.findingOfRun(t, testUser, run)
	if err != nil {
		t.Fatal(err)
	}
	if links := r.auditLinksOf(t, "run.withdraw"); len(links) != 1 || links[0] != (auditLinks{thread: shared.Thread.ID, run: run.String(), finding: answer.ID}) {
		t.Fatalf("run.withdraw links = %+v, want the thread, the run and the finding %s", links, answer.ID)
	}
	if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a second withdraw: %v", err)
	}

	again, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil || again.Replayed || again.Thread.ID == shared.Thread.ID {
		t.Fatalf("share after the withdraw = %+v, %v", again, err)
	}
	if replies := r.runReplies(t, again.Thread.ID); len(replies) != 1 {
		t.Fatalf("replies after the share again = %+v, want one", replies)
	}
}

func TestAWithdrawIsRefusedOnceAnotherPersonReplied(t *testing.T) {
	r := newShareRig(t)
	_, run, shared := r.sharedDashboardAnswer(t)
	r.reply(t, otherEditor, shared.Thread.ID, "seen it here too")

	if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); !errors.Is(err, ErrOthersWroteHere) {
		t.Fatalf("withdraw after another person replied: %v", err)
	}
	stored, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThreadMessageID.String() != shared.Message.ID {
		t.Fatalf("the refused withdraw changed the run: %+v", stored)
	}
	if n := r.messageCount(t); n != 3 {
		t.Fatalf("messages after a refused withdraw = %d, want 3", n)
	}
	if events := r.db.AuditEvents(t, "run.withdraw"); len(events) != 0 {
		t.Fatalf("run.withdraw events after a refusal = %+v", events)
	}
	r.call(t, testUser, http.MethodDelete, "/threads/"+shared.Thread.ID+"/share", nil).expect(t, http.StatusConflict, CodeOthersWroteHere)
}

func TestSomeoneWhoCannotViewTheThreadCannotWithdrawIt(t *testing.T) {
	r := newShareRig(t)
	dash, _, shared := r.sharedDashboardAnswer(t)
	foldertest.Move(t, r.db, dash, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))

	if err := r.service.Withdraw(reading(t.Context()), viewer, mustParse(t, shared.Thread.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("withdraw by someone who cannot view: %v", err)
	}
	r.call(t, viewer, http.MethodDelete, "/threads/"+shared.Thread.ID+"/share", nil).expect(t, http.StatusNotFound, "not_found")
	if n := r.messageCount(t); n != 2 {
		t.Fatalf("messages after the refusal = %d, want 2", n)
	}
}

func (r *rig) listedFindingsWith(t *testing.T, by auth.Principal, observation pgtype.UUID) []string {
	t.Helper()
	var folder pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT folder_id FROM observations WHERE id = $1`, observation).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	listed, _, err := r.findings.List(reading(t.Context()), by, findings.ListFilter{FolderID: folder}, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(listed))
	for _, f := range listed {
		ids = append(ids, f.ID)
	}
	return ids
}

func TestWithdrawingAnObservationAnswerMakesItPrivateAgainAndKeepsTheBlocksItWrote(t *testing.T) {
	r := newShareRig(t)
	observation := r.filedObservationIn(t, testUser, pgtype.UUID{})
	id := observation.String()
	run := r.privateAnswer(t, revisions.StartRequest{ObservationID: &id}, "prom", "")
	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil {
		t.Fatal(err)
	}
	blocks := r.blockCount(t, observation)
	answer, err := r.findingOfRun(t, otherEditor, run)
	if err != nil || !slices.Contains(r.listedFindingsWith(t, otherEditor, observation), answer.ID) {
		t.Fatalf("an observation reader before the withdraw: %+v, %v", answer, err)
	}

	if got := r.callStatus(t, testUser, http.MethodDelete, "/threads/"+shared.Thread.ID+"/share"); got != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", got)
	}
	if after := r.blockCount(t, observation); after != blocks {
		t.Fatalf("blocks %d -> %d, want the written blocks kept", blocks, after)
	}
	if n := r.messageCount(t); n != 0 {
		t.Fatalf("messages after the withdraw = %d, want 0", n)
	}
	if _, err := r.findingOfRun(t, otherEditor, run); !errors.Is(err, findings.ErrNotFound) {
		t.Fatalf("an observation reader reads the withdrawn answer: %v", err)
	}
	if code := r.observeStatus(t, otherEditor, run); code != http.StatusNotFound {
		t.Fatalf("an observation reader: GET /observe/{id} of the withdrawn run answered %d", code)
	}
	if listed := r.listedFindingsWith(t, otherEditor, observation); slices.Contains(listed, answer.ID) {
		t.Fatalf("the withdrawn answer is still listed: %v", listed)
	}
	if f, err := r.findingOfRun(t, testUser, run); err != nil || f.ObservationID != "" {
		t.Fatalf("the asker reads the withdrawn answer as %+v, %v", f, err)
	}
	events := r.db.AuditEvents(t, "run.withdraw")
	if len(events) != 1 || events[0].TargetID != run.String() {
		t.Fatalf("run.withdraw events = %+v", events)
	}
}

func TestAnAnswerSavedAfterItsShareStaysInTheObservationWhenTheShareIsWithdrawn(t *testing.T) {
	r := newShareRig(t)
	observation := r.filedObservationIn(t, testUser, pgtype.UUID{})
	id := observation.String()
	run := r.privateAnswer(t, revisions.StartRequest{ObservationID: &id}, "prom", "")
	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil {
		t.Fatal(err)
	}
	saved, outcome, err := r.findings.SaveAsObservation(reading(t.Context()), testUser, run)
	if err != nil || outcome != findings.SaveKept || saved.ObservationID != id {
		t.Fatalf("save of the shown answer: observation %q, outcome %s, %v; want it kept in %s", saved.ObservationID, outcome, err, id)
	}
	if _, outcome, err := r.findings.SaveAsObservation(reading(t.Context()), testUser, run); err != nil || outcome != findings.SaveUnchanged {
		t.Fatalf("second save: outcome %s, %v; want unchanged", outcome, err)
	}

	if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); err != nil {
		t.Fatal(err)
	}
	if n := r.messageCount(t); n != 0 {
		t.Fatalf("messages after the withdraw = %d, want the share gone", n)
	}
	answer, err := r.findingOfRun(t, otherEditor, run)
	if err != nil || answer.ObservationID != id {
		t.Fatalf("an observation reader after the withdraw reads %+v, %v; want the saved answer in %s", answer, err, id)
	}
	if listed := r.listedFindingsWith(t, otherEditor, observation); !slices.Contains(listed, answer.ID) {
		t.Fatalf("the saved answer left the observation's list: %v", listed)
	}
}

func (r *rig) sharedThreadShown(t *testing.T, by auth.Principal, run pgtype.UUID) (onRun, onTurn *string) {
	t.Helper()
	desk := revisions.NewRunDesk(r.revisions, revisions.ObserveOptions{})
	raw, err := desk.Run(reading(t.Context()), run, by, 0)
	if err != nil {
		t.Fatalf("%s reads the run: %v", by.Name, err)
	}
	var one struct {
		SharedThread *string `json:"shared_thread_id"`
		ChatID       *string `json:"chat_id"`
	}
	if err := json.Unmarshal(raw, &one); err != nil {
		t.Fatal(err)
	}
	if one.ChatID == nil {
		t.Fatalf("run %s has no chat: %s", run, raw)
	}
	raw, err = desk.Chat(reading(t.Context()), by, mustParse(t, *one.ChatID))
	if err != nil {
		t.Fatalf("%s reads the chat: %v", by.Name, err)
	}
	var detail struct {
		Turns []struct {
			ID           string  `json:"id"`
			SharedThread *string `json:"shared_thread_id"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	for _, turn := range detail.Turns {
		if turn.ID == run.String() {
			return one.SharedThread, turn.SharedThread
		}
	}
	t.Fatalf("run %s is not a turn of its chat", run)
	return nil, nil
}

func TestOnlyTheAskerSeesWhichThreadTheirAnswerIsSharedIn(t *testing.T) {
	r := newShareRig(t)
	_, run, shared := r.sharedDashboardAnswer(t)

	onRun, onTurn := r.sharedThreadShown(t, testUser, run)
	if onRun == nil || *onRun != shared.Thread.ID || onTurn == nil || *onTurn != shared.Thread.ID {
		t.Fatalf("the asker reads shared_thread_id %v on the run and %v on the turn, want %s", onRun, onTurn, shared.Thread.ID)
	}
	if onRun, onTurn := r.sharedThreadShown(t, admin, run); onRun != nil || onTurn != nil {
		t.Fatalf("an Admin reads shared_thread_id %v on the run and %v on the turn, want none", onRun, onTurn)
	}
	if shown := r.runAsReadBy(t, otherEditor, run); !shown.ReadOnly || shown.SharedThread != nil || shown.ChatID != nil {
		t.Fatalf("another reader of the thread reads the run as %+v, want read only with no thread or chat", shown)
	}

	if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); err != nil {
		t.Fatal(err)
	}
	if onRun, onTurn := r.sharedThreadShown(t, testUser, run); onRun != nil || onTurn != nil {
		t.Fatalf("after the withdraw the asker reads shared_thread_id %v on the run and %v on the turn", onRun, onTurn)
	}
}

func TestAnAdminCannotShareOrWithdrawSomeoneElsesPrivateAnswer(t *testing.T) {
	r := newShareRig(t)
	dash := r.createDashboard(t, "rps", "errors")
	id := dash.String()
	run := r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "errors")

	if _, err := r.service.ShareRun(reading(t.Context()), admin, ShareInput{RunID: run}); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("an Admin shared someone else's private answer: %v", err)
	}
	if n := r.messageCount(t); n != 0 {
		t.Fatalf("messages after the Admin's refused share = %d, want 0", n)
	}

	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.service.Withdraw(reading(t.Context()), admin, mustParse(t, shared.Thread.ID)); !errors.Is(err, ErrNothingShared) {
		t.Fatalf("an Admin withdrew someone else's answer: %v", err)
	}
	r.call(t, admin, http.MethodDelete, "/threads/"+shared.Thread.ID+"/share", nil).expect(t, http.StatusNotFound, "not_found")
	if n := r.messageCount(t); n != 2 {
		t.Fatalf("messages after the Admin's refused withdraw = %d, want 2", n)
	}
	if events := r.db.AuditEvents(t, "run.withdraw"); len(events) != 0 {
		t.Fatalf("run.withdraw events after the Admin's refusal = %+v", events)
	}
}

type runShown struct {
	ReadOnly       bool     `json:"read_only"`
	AllowedActions []string `json:"allowed_actions"`
	SharedThread   *string  `json:"shared_thread_id"`
	ChatID         *string  `json:"chat_id"`
}

func (r *rig) runAsReadBy(t *testing.T, by auth.Principal, run pgtype.UUID) runShown {
	t.Helper()
	raw, err := revisions.NewRunDesk(r.revisions, revisions.ObserveOptions{}).Run(reading(t.Context()), run, by, 0)
	if err != nil {
		t.Fatalf("%s reads the run: %v", by.Name, err)
	}
	var shown runShown
	if err := json.Unmarshal(raw, &shown); err != nil {
		t.Fatal(err)
	}
	return shown
}

func (r *rig) observeStatus(t *testing.T, who auth.Principal, run pgtype.UUID) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	revisions.NewObserveHandler(r.revisions, revisions.ObserveOptions{}).Register(auth.NewRoutes(engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), who))
	})))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/observe/"+run.String(), nil))
	return rec.Code
}

func TestASharedAnswerIsReadByTheThreadsReadersUntilItIsWithdrawn(t *testing.T) {
	r := newShareRig(t)
	_, run, shared := r.sharedDashboardAnswer(t)
	desk := revisions.NewRunDesk(r.revisions, revisions.ObserveOptions{})
	robot := auth.Principal{Kind: auth.KindServiceAccount, ID: "66666666-6666-4666-8666-666666666666", Name: "ci", Role: auth.RoleViewer}

	if shown := r.runAsReadBy(t, testUser, run); shown.ReadOnly || shown.SharedThread == nil || len(shown.AllowedActions) == 0 {
		t.Fatalf("the asker reads the run as %+v", shown)
	}
	for _, reader := range []auth.Principal{otherEditor, commenter, viewer, robot, admin} {
		if shown := r.runAsReadBy(t, reader, run); !shown.ReadOnly || len(shown.AllowedActions) != 0 || shown.SharedThread != nil || shown.ChatID != nil && reader.ID != adminID {
			t.Errorf("%s reads the shared run as %+v, want read only", reader.Name, shown)
		}
		if _, err := r.findingOfRun(t, reader, run); err != nil {
			t.Errorf("%s reads the shared answer: %v", reader.Name, err)
		}
		if code := r.observeStatus(t, reader, run); code != http.StatusOK {
			t.Errorf("%s: GET /observe/{id} of the shared run answered %d", reader.Name, code)
		}
	}

	if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); err != nil {
		t.Fatal(err)
	}
	for _, former := range []auth.Principal{otherEditor, commenter, viewer, robot} {
		if _, err := desk.Run(reading(t.Context()), run, former, 0); !errors.Is(err, revisions.ErrRequestNotFound) {
			t.Errorf("%s reads the withdrawn run: %v", former.Name, err)
		}
		if code := r.observeStatus(t, former, run); code != http.StatusNotFound {
			t.Errorf("%s: GET /observe/{id} of the withdrawn run answered %d", former.Name, code)
		}
	}
	if shown := r.runAsReadBy(t, admin, run); !shown.ReadOnly {
		t.Errorf("an Admin reads the withdrawn run as %+v", shown)
	}
	if shown := r.runAsReadBy(t, testUser, run); shown.ReadOnly {
		t.Errorf("the asker reads the withdrawn run as %+v", shown)
	}
}

func TestOnlyThoseWhoReadTheThreadReadASharedRunAndMayApplyItsSuggestion(t *testing.T) {
	r := newShareRig(t)
	dash, run, shared := r.sharedDashboardAnswer(t)
	r.db.Exec(t, `UPDATE findings SET suggested_change = $2 WHERE agent_request_id = $1`, run, declinesPanel)
	desk := revisions.NewRunDesk(r.revisions, revisions.ObserveOptions{})
	finding, err := r.findingOfRun(t, otherEditor, run)
	if err != nil {
		t.Fatal(err)
	}

	sent := r.applied(t, otherEditor, shared.Thread.ID, mustParse(t, finding.ID))
	applyRun, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), mustParse(t, sent.Run.ID))
	if err != nil || applyRun.RequestedByID.String() != otherEditor.ID || applyRun.Intent != string(revisions.IntentApply) {
		t.Fatalf("the reader's apply run = %+v, %v", applyRun, err)
	}

	insider := auth.Principal{Kind: auth.KindUser, ID: "77777777-7777-4777-8777-777777777777", Name: "In", Role: auth.RoleEditor, Groups: []string{payrollGroup}}
	foldertest.Move(t, r.db, dash, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))
	if _, err := desk.Run(reading(t.Context()), run, otherEditor, 0); !errors.Is(err, revisions.ErrRequestNotFound) {
		t.Errorf("someone who can no longer read the thread reads the run: %v", err)
	}
	if _, err := r.findingOfRun(t, otherEditor, run); !errors.Is(err, findings.ErrNotFound) {
		t.Errorf("someone who can no longer read the thread reads the answer: %v", err)
	}
	if shown := r.runAsReadBy(t, insider, run); !shown.ReadOnly {
		t.Errorf("a reader of the restricted thread reads the run as %+v", shown)
	}
}
