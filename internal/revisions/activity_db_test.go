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
	rows, err := d.SQLDB(t).QueryContext(t.Context(), `SELECT type, coalesce(dashboard_id::text, ''), coalesce(folder_id::text, ''), actor_id::text, actor_name, coalesce(run_id::text, ''), data
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
	f, err := d.Queries().CreateFolder(context.Background(), dbsqlc.CreateFolderParams{Title: "Payments"})
	if err != nil {
		t.Fatal(err)
	}
	folder = f.ID

	row, err := svc.SubmitObserve(context.Background(), user, "build checkout health")
	if err != nil {
		t.Fatal(err)
	}
	assertTypes(t, runRecords(t, d, row.ID), "run.queued")

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitFor(t, "the observe request to succeed", func() bool {
		got, err := svc.GetByID(context.Background(), row.ID)
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
	for _, r := range records {
		if r.ActorID != userID || r.ActorName != user.Name || r.Data["source"] != "observe" {
			t.Errorf("%s actor %q %q data %v, want the requester and source observe", r.Type, r.ActorID, r.ActorName, r.Data)
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

	if _, err := svc.Cancel(context.Background(), mustUUID(t, rev.ID), user); err != nil {
		t.Fatal(err)
	}

	records := runRecords(t, d, mustUUID(t, rev.ID))
	assertTypes(t, records, "run.queued", "run.canceled")
	if got := records[1]; got.RunID != rev.ID || got.ActorID != userID || got.DashboardID != dash.String() {
		t.Errorf("run.canceled = %+v, want the requester running as the request on its dashboard", got)
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
	folder, err := d.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Payments"})
	if err != nil {
		t.Fatal(err)
	}
	foldertest.Move(t, d, dash, folder.ID)
	rev := submit(t, svc, dash, "add a panel")

	tx, err := d.SQLDB(t).BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(t.Context(), `SELECT title FROM dashboards WHERE id = $1 FOR UPDATE`, dash.String()); err != nil {
		t.Fatal(err)
	}

	type claimed struct {
		found bool
		err   error
	}
	done := make(chan claimed, 1)
	go func() {
		_, found, err := svc.ClaimRow(t.Context())
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

	if _, err := tx.ExecContext(t.Context(), `DELETE FROM dashboards WHERE id = $1`, dash.String()); err != nil {
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
	ctx, cancel := context.WithCancel(context.Background())
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

	row, err := svc.SubmitObserve(context.Background(), user, "build something")
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

	if _, err := svc.Cancel(context.Background(), mustUUID(t, rev.ID), user); err != nil {
		t.Fatal(err)
	}

	sent := got.drain(t, d)
	if len(sent) != 1 || sent[0]["status"] != "failed" || sent[0]["seq"] == nil {
		t.Fatalf("cancel sent %v, want one revision.updated with a seq", sent)
	}
}
