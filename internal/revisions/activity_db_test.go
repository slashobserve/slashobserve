package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/health/repairprompt"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type runRecord struct {
	Type        string
	DashboardID string
	FolderID    string
	ActorID     string
	ActorName   string
	RunID       string
	Data        map[string]any
}

func runRecords(t *testing.T, d *dbtest.Database, requestID pgtype.UUID) []runRecord {
	t.Helper()
	rows, err := d.SQLDB(t).QueryContext(reading(t.Context()), `SELECT type, coalesce(dashboard_id::text, ''), coalesce(folder_id::text, ''), actor_id::text, actor_name, coalesce(run_id::text, ''), data
FROM activity WHERE target_kind = 'run' AND target_id = $1 ORDER BY seq`, requestID.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []runRecord
	for rows.Next() {
		var r runRecord
		var data []byte
		if err := rows.Scan(&r.Type, &r.DashboardID, &r.FolderID, &r.ActorID, &r.ActorName, &r.RunID, &data); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &r.Data); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func typesOf(records []runRecord) []string {
	out := make([]string, len(records))
	for i, r := range records {
		out[i] = r.Type
	}
	return out
}

func assertTypes(t *testing.T, records []runRecord, want ...string) {
	t.Helper()
	got := typesOf(records)
	if len(got) != len(want) {
		t.Fatalf("activity types = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("activity types = %v, want %v", got, want)
		}
	}
}

func TestANewDashboardRunIsRecordedWithoutAFolderUntilItsDashboardExists(t *testing.T) {
	var d *dbtest.Database
	var folder, created pgtype.UUID
	svc, d := newService(t, reviserFunc(func(ctx context.Context, req revisions.Request) (revisions.Result, error) {
		dash := seedDashboard(t, d, 0)
		foldertest.Move(t, d, dash, folder)
		d.Exec(t, `INSERT INTO dashboard_versions (dashboard_id, version, spec, summary, author_kind, author_id, author_name, agent_request_id)
VALUES ($1, 1, '{}', 's', 'user', $2::uuid, 'Ada', $3)`, dash, userID, mustUUID(t, req.ID))
		d.Exec(t, "UPDATE dashboards SET latest_version = 1 WHERE id = $1", dash)
		created = dash
		return revisions.Result{Version: 1}, nil
	}))
	f, err := d.Queries().CreateFolder(reading(context.Background()), dbsqlc.CreateFolderParams{Title: "Payments"})
	if err != nil {
		t.Fatal(err)
	}
	folder = f.ID

	row, err := svc.SubmitObserve(reading(context.Background()), user, "build checkout health")
	if err != nil {
		t.Fatal(err)
	}
	assertTypes(t, runRecords(t, d, row.ID), "run.queued")

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitFor(t, "the observe request to succeed", func() bool {
		got, err := svc.GetByID(reading(context.Background()), row.ID)
		return err == nil && got.Status == string(revisions.StatusSucceeded)
	})

	records := runRecords(t, d, row.ID)
	assertTypes(t, records, "run.queued", "run.started", "run.succeeded")
	queued, started, succeeded := records[0], records[1], records[2]
	for _, r := range records[:2] {
		if r.DashboardID != "" || r.FolderID != "" {
			t.Errorf("%s has dashboard %q and folder %q, want neither before the dashboard exists", r.Type, r.DashboardID, r.FolderID)
		}
	}
	if succeeded.DashboardID != created.String() || succeeded.FolderID != folder.String() {
		t.Errorf("run.succeeded has dashboard %q and folder %q, want %q and %q", succeeded.DashboardID, succeeded.FolderID, created, folder)
	}
	if queued.RunID != "" || started.RunID != row.ID.String() || succeeded.RunID != row.ID.String() {
		t.Errorf("run ids = %q, %q, %q; want none for the request and the request id once it runs", queued.RunID, started.RunID, succeeded.RunID)
	}
	wantIntent := map[string]string{"run.queued": "ask", "run.started": "ask", "run.succeeded": "apply"}
	for _, r := range records {
		if r.ActorID != userID || r.ActorName != user.Name || r.Data["intent"] != wantIntent[r.Type] {
			t.Errorf("%s actor %q %q data %v, want the requester and intent %s", r.Type, r.ActorID, r.ActorName, r.Data, wantIntent[r.Type])
		}
	}
	if started.Data["attempt"] != float64(1) || succeeded.Data["result_version"] != float64(1) {
		t.Errorf("data = %v and %v, want attempt 1 and result_version 1", started.Data, succeeded.Data)
	}
}

func TestAFailedRunRecordsItsFailureCode(t *testing.T) {
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		return revisions.Result{FailureCode: revisions.FailureNoProgress}, errors.New("stuck")
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "go")

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusFailed)

	records := runRecords(t, d, mustUUID(t, rev.ID))
	assertTypes(t, records, "run.queued", "run.started", "run.failed")
	if got := records[2]; got.Data["failure_code"] != "no_progress" || got.DashboardID != dash.String() {
		t.Errorf("run.failed = %+v, want failure_code no_progress on the dashboard", got)
	}
}

func TestACanceledQueuedRequestRecordsRunCanceled(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "go")
	assignTo(t, d, rev.ID, "grant", grantA)

	if _, err := svc.Cancel(reading(context.Background()), mustUUID(t, rev.ID), user); err != nil {
		t.Fatal(err)
	}

	records := runRecords(t, d, mustUUID(t, rev.ID))
	assertTypes(t, records, "run.queued", "run.canceled")
	if got := records[1]; got.RunID != rev.ID || got.ActorID != userID || got.DashboardID != dash.String() {
		t.Errorf("run.canceled = %+v, want the requester running as the request on its dashboard", got)
	}
	if got := auditedAgents(t, d, rev.ID); got["run.finish"] != "" || got["run.queue"] != "" {
		t.Errorf("agents of the audit events = %v, want none, because no agent took the run", got)
	}
}

func auditedAgents(t *testing.T, d *dbtest.Database, runID string) map[string]string {
	t.Helper()
	rows, err := d.SQLDB(t).QueryContext(reading(t.Context()), `SELECT action, coalesce(agent_name, '') FROM audit_events WHERE target_id = $1`, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var action, agent string
		if err := rows.Scan(&action, &agent); err != nil {
			t.Fatal(err)
		}
		out[action] = agent
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestARunExpiredAsUnclaimedNamesNoAgent(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	rev := submit(t, svc, seedDashboard(t, d, 1), "nobody came")
	assignTo(t, d, rev.ID, "grant", grantA)
	d.Exec(t, "UPDATE agent_requests SET created_at = now() - interval '11 minutes' WHERE id = $1", mustUUID(t, rev.ID))

	if err := svc.Sweep(reading(context.Background())); err != nil {
		t.Fatal(err)
	}

	got := auditedAgents(t, d, rev.ID)
	if _, finished := got["run.finish"]; !finished || got["run.finish"] != "" {
		t.Fatalf("agents of the audit events = %v, want run.finish with no agent", got)
	}
}

func TestARunTheBuiltInAgentTookNamesItOnStartAndFinish(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	rev := submit(t, svc, seedDashboard(t, d, 1), "go")
	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitFor(t, "the request to succeed", func() bool {
		got, err := svc.GetByID(reading(context.Background()), mustUUID(t, rev.ID))
		return err == nil && got.Status == string(revisions.StatusSucceeded)
	})

	got := auditedAgents(t, d, rev.ID)
	if got["run.start"] != "Observe Agent" || got["run.finish"] != "Observe Agent" || got["run.queue"] != "" {
		t.Fatalf("agents of the audit events = %v, want Observe Agent on start and finish only", got)
	}
}

func finishAudit(t *testing.T, d *dbtest.Database, runID string) (outcome, result string) {
	t.Helper()
	err := d.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT outcome, coalesce(data->>'result', '') FROM audit_events WHERE target_id = $1 AND action = 'run.finish'`, runID).Scan(&outcome, &result)
	if err != nil {
		t.Fatalf("run.finish of %s: %v", runID, err)
	}
	return outcome, result
}

func runsToTheEnd(t *testing.T, svc *revisions.Service, d *dbtest.Database, id string) {
	t.Helper()
	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitFor(t, "the run to end", func() bool { return revisions.State(storedRun(t, svc, id).State).Terminal() })
}

func canceledWhileRunning(t *testing.T, svc *revisions.Service, _ *dbtest.Database, id string) {
	t.Helper()
	claimed, found := claimOne(t, svc)
	if !found || claimed.ID != mustUUID(t, id) {
		t.Fatalf("claim = %v found %v, want run %s", claimed.ID, found, id)
	}
	if _, err := svc.Cancel(reading(context.Background()), claimed.ID, user); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReleaseRow(reading(context.Background()), claimed, false); err != nil {
		t.Fatal(err)
	}
}

func canceledWhileQueued(t *testing.T, svc *revisions.Service, _ *dbtest.Database, id string) {
	t.Helper()
	if _, err := svc.Cancel(reading(context.Background()), mustUUID(t, id), user); err != nil {
		t.Fatal(err)
	}
}

func TestTheRunFinishEventAgreesWithTheFinalStateOfTheRun(t *testing.T) {
	failingWith := func(code revisions.FailureCode) reviserFunc {
		return func(context.Context, revisions.Request) (revisions.Result, error) {
			return revisions.Result{FailureCode: code}, errors.New("stopped")
		}
	}
	tests := []struct {
		name        string
		reviser     reviserFunc
		end         func(t *testing.T, svc *revisions.Service, d *dbtest.Database, id string)
		wantOutcome string
		wantResult  string
		wantCode    revisions.FailureCode
	}{
		{"a run that saved its result", succeedWith(2), runsToTheEnd, "success", "succeeded", ""},
		{"a run that failed", failingWith(revisions.FailureNoProgress), runsToTheEnd, "failed", "failed", revisions.FailureNoProgress},
		{"a run that timed out", failingWith(revisions.FailureCallTimeout), runsToTheEnd, "failed", "failed", revisions.FailureCallTimeout},
		{"a run canceled while it was running", succeedWith(2), canceledWhileRunning, "failed", "canceled", revisions.FailureCanceled},
		{"a run canceled while it was queued", succeedWith(2), canceledWhileQueued, "failed", "canceled", revisions.FailureCanceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, d := newService(t, tt.reviser)
			rev := submit(t, svc, seedDashboard(t, d, 1), "go")
			tt.end(t, svc, d, rev.ID)
			stored := storedRun(t, svc, rev.ID)
			outcome, result := finishAudit(t, d, rev.ID)
			if outcome != tt.wantOutcome || result != tt.wantResult || stored.FailureCode.String != string(tt.wantCode) {
				t.Fatalf("run.finish outcome %q result %q for a run with status %s and failure code %q, want outcome %q result %q and code %q", outcome, result, stored.Status, stored.FailureCode.String, tt.wantOutcome, tt.wantResult, tt.wantCode)
			}
		})
	}
}

func TestASuggestionRecordsNoActivity(t *testing.T) {
	svc, d := newSuggestService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	row := suggestFor(t, svc, dash, user)
	if _, found := claimOne(t, svc); !found {
		t.Fatal("nothing claimed")
	}

	if got := runRecords(t, d, row.ID); len(got) != 0 {
		t.Fatalf("a suggestion recorded %v", typesOf(got))
	}
}

func TestAReleaseRecordsTheRunReleased(t *testing.T) {
	started := make(chan struct{})
	svc, d := newService(t, reviserFunc(func(ctx context.Context, _ revisions.Request) (revisions.Result, error) {
		close(started)
		<-ctx.Done()
		return revisions.Result{}, ctx.Err()
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "go")
	stop := startWorker(t, svc, d, revisions.WorkerOptions{})
	<-started
	stop()

	if got := get(t, svc, dash, rev.ID); got.Status != revisions.StatusQueued {
		t.Fatalf("after shutdown the request is %s, want it released to queued", got.Status)
	}
	assertTypes(t, runRecords(t, d, mustUUID(t, rev.ID)), "run.queued", "run.started", "run.released")
}

func TestAClaimDoesNotDeadlockWithADashboardDelete(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	folder, err := d.Queries().CreateFolder(reading(t.Context()), dbsqlc.CreateFolderParams{Title: "Payments"})
	if err != nil {
		t.Fatal(err)
	}
	foldertest.Move(t, d, dash, folder.ID)
	rev := submit(t, svc, dash, "add a panel")

	tx, err := d.SQLDB(t).BeginTx(reading(t.Context()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(reading(t.Context()), `SELECT title FROM dashboards WHERE id = $1 FOR UPDATE`, dash.String()); err != nil {
		t.Fatal(err)
	}

	type claimed struct {
		found bool
		err   error
	}
	done := make(chan claimed, 1)
	go func() {
		_, found, err := svc.ClaimRow(reading(t.Context()))
		done <- claimed{found, err}
	}()
	select {
	case got := <-done:
		if got.err != nil || !got.found {
			t.Fatalf("claim found %v with error %v, want a claimed request", got.found, got.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("claim waited on the dashboard row lock")
	}

	if _, err := tx.ExecContext(reading(t.Context()), `DELETE FROM dashboards WHERE id = $1`, dash.String()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	records := runRecords(t, d, mustUUID(t, rev.ID))
	assertTypes(t, records, "run.queued", "run.started")
	if got := records[1].FolderID; got != folder.ID.String() {
		t.Fatalf("run.started folder = %q, want %q", got, folder.ID)
	}
}

type notices struct {
	mu       sync.Mutex
	payloads []map[string]any
	sentinel chan struct{}
}

func listenForNotices(t *testing.T, d *dbtest.Database) *notices {
	t.Helper()
	n := &notices{sentinel: make(chan struct{}, 1)}
	listening := make(chan struct{})
	ctx, cancel := context.WithCancel(reading(context.Background()))
	t.Cleanup(cancel)
	go func() {
		_ = d.Listen(ctx, events.Channel, func() { close(listening) }, func(payload string) {
			var p map[string]any
			if err := json.Unmarshal([]byte(payload), &p); err != nil {
				return
			}
			if p["type"] == "sentinel" {
				n.sentinel <- struct{}{}
				return
			}
			n.mu.Lock()
			n.payloads = append(n.payloads, p)
			n.mu.Unlock()
		})
	}()
	<-listening
	return n
}

func (n *notices) drain(t *testing.T, d *dbtest.Database) []map[string]any {
	t.Helper()
	d.Exec(t, `SELECT pg_notify($1, '{"type":"sentinel","dashboard_id":"x"}')`, events.Channel)
	<-n.sentinel
	n.mu.Lock()
	defer n.mu.Unlock()
	out := n.payloads
	n.payloads = nil
	return out
}

func TestOneTransitionSendsOneNotificationCarryingItsSequence(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	got := listenForNotices(t, d)

	rev := submit(t, svc, dash, "go")
	queued := got.drain(t, d)
	if len(queued) != 1 || queued[0]["type"] != events.TypeRevisionUpdated || queued[0]["status"] != "queued" || queued[0]["seq"] == nil {
		t.Fatalf("enqueue sent %v, want one revision.updated with a seq", queued)
	}

	if _, found := claimOne(t, svc); !found {
		t.Fatal("nothing claimed")
	}
	claimed := got.drain(t, d)
	if len(claimed) != 1 || claimed[0]["status"] != "running" || claimed[0]["revision_id"] != rev.ID || claimed[0]["seq"] == nil {
		t.Fatalf("claim sent %v, want one revision.updated for the claimed request with a seq", claimed)
	}
	if claimed[0]["seq"] == queued[0]["seq"] {
		t.Fatalf("both transitions carry seq %v", claimed[0]["seq"])
	}
}

func TestATransitionWithoutALiveEventSendsOnlyItsSequence(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	got := listenForNotices(t, d)

	row, err := svc.SubmitObserve(reading(context.Background()), user, "build something")
	if err != nil {
		t.Fatal(err)
	}

	sent := got.drain(t, d)
	if len(sent) != 1 || len(sent[0]) != 1 || sent[0]["seq"] == nil {
		t.Fatalf("a new-dashboard request sent %v, want exactly {seq}", sent)
	}
	assertTypes(t, runRecords(t, d, row.ID), "run.queued")
}

func TestARecordedTransitionNeverSendsASecondNotification(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "go")
	got := listenForNotices(t, d)

	if _, err := svc.Cancel(reading(context.Background()), mustUUID(t, rev.ID), user); err != nil {
		t.Fatal(err)
	}

	sent := got.drain(t, d)
	if len(sent) != 1 || sent[0]["status"] != "failed" || sent[0]["seq"] == nil {
		t.Fatalf("cancel sent %v, want one revision.updated with a seq", sent)
	}
}

const repairedParentSpec = `{"title":"Node resources","panels":[
  {"id":"cpu","type":"timeseries","queries":[{"query":"bad"}]},
  {"id":"mem","type":"timeseries","queries":[{"query":"bad"}]},
  {"id":"disk","type":"timeseries","queries":[{"query":"bad"}]}]}`

const repairedSavedSpec = `{"title":"Node resources","panels":[
  {"id":"cpu","type":"timeseries","queries":[{"query":"good"}]},
  {"id":"mem","type":"timeseries","queries":[{"query":"bad"}]},
  {"id":"disk","type":"timeseries","queries":[{"query":"still bad"}]}]}`

func seedRepairedDashboard(t *testing.T, d *dbtest.Database) pgtype.UUID {
	t.Helper()
	dash := seedDashboard(t, d, 2)
	d.Exec(t, `UPDATE dashboard_versions SET spec = $2::jsonb WHERE dashboard_id = $1 AND version = 1`, dash, repairedParentSpec)
	d.Exec(t, `UPDATE dashboard_versions SET spec = $2::jsonb, unverified = '{"panels":[{"panel_id":"disk","issues":[]}],"variables":[]}' WHERE dashboard_id = $1 AND version = 2`, dash, repairedSavedSpec)
	return dash
}

func TestARepairRunThatSavedAVersionRecordsTheDashboardTitleAndRepairedPanelCount(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedRepairedDashboard(t, d)
	prompt := repairprompt.Prefix + "\n- panel cpu (\"CPU\") fails\n- panel mem returns no data\n- panel disk fails\n- variable node resolves to nothing"
	rev := submit(t, svc, dash, prompt)

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)

	records := runRecords(t, d, mustUUID(t, rev.ID))
	assertTypes(t, records, "run.queued", "run.started", "run.succeeded")
	got := records[2].Data
	if got["intent"] != "apply" || got["result_version"] != float64(2) {
		t.Errorf("run.succeeded data = %v, want its intent and result_version unchanged", got)
	}
	if got["dashboard_title"] != "Node resources" || got["panels_repaired"] != float64(1) {
		t.Errorf("run.succeeded data = %v, want the dashboard title and one repaired panel", got)
	}
	for _, r := range records[:2] {
		if _, ok := r.Data["dashboard_title"]; ok {
			t.Errorf("%s carries dashboard_title, only run.succeeded does", r.Type)
		}
	}
}

func TestARunThatIsNotAHealthRepairRecordsNoRepairFields(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedRepairedDashboard(t, d)
	rev := submit(t, svc, dash, "fix the cpu panel")

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)

	got := runRecords(t, d, mustUUID(t, rev.ID))[2].Data
	if _, ok := got["dashboard_title"]; ok {
		t.Errorf("run.succeeded data = %v, want no repair fields", got)
	}
	if _, ok := got["panels_repaired"]; ok {
		t.Errorf("run.succeeded data = %v, want no repair fields", got)
	}
}
