package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const activityWait = 15 * time.Second

type activityRow struct {
	Type         string
	FolderID     string
	DashboardID  string
	ActorKind    string
	ActorID      string
	ActorName    string
	Origin       string
	OriginClient string
	RunID        string
	Data         map[string]any
}

func (r *rig) activityRows(t *testing.T, types ...string) []activityRow {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `
		SELECT type, coalesce(folder_id::text, ''), coalesce(dashboard_id::text, ''), actor_kind, actor_id::text, actor_name,
			coalesce(origin, ''), coalesce(origin_client, ''), coalesce(run_id::text, ''), data::text
		FROM activity ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []activityRow
	for rows.Next() {
		var row activityRow
		var data string
		if err := rows.Scan(&row.Type, &row.FolderID, &row.DashboardID, &row.ActorKind, &row.ActorID, &row.ActorName, &row.Origin, &row.OriginClient, &row.RunID, &data); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(data), &row.Data); err != nil {
			t.Fatal(err)
		}
		if len(types) == 0 || slices.Contains(types, row.Type) {
			got = append(got, row)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func (r *rig) activityFor(t *testing.T, dash pgtype.UUID) []activityRow {
	t.Helper()
	var got []activityRow
	for _, row := range r.activityRows(t) {
		if row.DashboardID == dash.String() {
			got = append(got, row)
		}
	}
	return got
}

func onlyActivity(t *testing.T, rows []activityRow) activityRow {
	t.Helper()
	if len(rows) != 1 {
		t.Fatalf("activity rows = %+v, want exactly one", rows)
	}
	return rows[0]
}

func TestEveryVersionOfADashboardAppendsOneActivityRow(t *testing.T) {
	r := newRig(t, true)
	mcp := origin.With(t.Context(), origin.Origin{Kind: origin.MCP, Client: "claude-code"})

	dash, _, err := r.service.Create(mcp, testUser, pgtype.UUID{}, panelsSpec("Checkout", "rps"), "first draft")
	if err != nil {
		t.Fatal(err)
	}
	id := mustUUID(t, dash.ID)
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, id, 1, panelsSpec("Checkout v2", "rps", "p99"), "add p99"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.service.Restore(t.Context(), testUser, id, 1, 2, "back to the start"); err != nil {
		t.Fatal(err)
	}

	rows := r.activityFor(t, id)
	wantTypes := []string{activity.TypeDashboardCreated, activity.TypeDashboardVersionCreated, activity.TypeDashboardRestored}
	if got := []string{rows[0].Type, rows[1].Type, rows[2].Type}; len(rows) != 3 || !slices.Equal(got, wantTypes) {
		t.Fatalf("types = %v, want %v", got, wantTypes)
	}
	created, saved := rows[0], rows[1]
	for _, row := range rows {
		if row.ActorKind != "user" || row.ActorID != testUserID || row.ActorName != "Ada" || row.RunID != "" {
			t.Fatalf("actor of %s = %+v, want Ada the user with no run", row.Type, row)
		}
		if row.FolderID == "" || row.FolderID != dash.Folder.ID.String() {
			t.Fatalf("folder of %s = %q, want the dashboard's folder %s", row.Type, row.FolderID, dash.Folder.ID)
		}
	}
	if created.Origin != "mcp" || created.OriginClient != "claude-code" || saved.Origin != "" {
		t.Fatalf("origins = %q/%q and %q, want mcp/claude-code on the first and none on the second", created.Origin, created.OriginClient, saved.Origin)
	}
	wantCreated := map[string]any{"version": 1.0, "title": "Checkout", "summary": "first draft"}
	wantSaved := map[string]any{"version": 2.0, "parent_version": 1.0, "title": "Checkout v2", "summary": "add p99"}
	wantRestored := map[string]any{"version": 3.0, "parent_version": 2.0, "restored_from": 1.0, "title": "Checkout", "summary": "back to the start"}
	for i, want := range []map[string]any{wantCreated, wantSaved, wantRestored} {
		if got := rows[i].Data; !mapsEqual(got, want) {
			t.Fatalf("data of %s = %v, want %v", rows[i].Type, got, want)
		}
	}
}

func mapsEqual(a, b map[string]any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestAnAgentRequestGivesTheVersionRowARunID(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	request := r.queueRevision(t, dash)
	if _, _, _, err := r.saveRequest(t, dash, 1, panelsSpec("t", "rps", "p99"), "agent save", request); err != nil {
		t.Fatal(err)
	}
	rows := r.activityFor(t, dash)
	last := rows[len(rows)-1]
	if last.Type != activity.TypeDashboardVersionCreated || last.RunID != request.String() {
		t.Fatalf("last row = %+v, want a version_created row with run id %s", last, request)
	}
}

func TestDuplicateAndImportAppendCreated(t *testing.T) {
	r := newRig(t, true)
	source := r.createDashboard(t)

	copied, _, err := r.service.Duplicate(t.Context(), testUser, source, pgtype.UUID{}, "Copy of t")
	if err != nil {
		t.Fatal(err)
	}
	imported, _, _, _, err := r.service.CreateImported(t.Context(), testUser, pgtype.UUID{}, panelsSpec("Imported", "rps"), "import", pgtype.UUID{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{copied.ID, imported.ID} {
		row := onlyActivity(t, r.activityFor(t, mustUUID(t, id)))
		if row.Type != activity.TypeDashboardCreated || row.ActorID != testUserID {
			t.Fatalf("row of %s = %+v, want dashboard.created by the caller", id, row)
		}
	}
}

func TestDeleteAppendsTheTitleAndTheFolderItLeft(t *testing.T) {
	r := newRig(t, true)
	dash, _, err := r.service.Create(t.Context(), testUser, pgtype.UUID{}, panelsSpec("Doomed", "rps"), "create")
	if err != nil {
		t.Fatal(err)
	}
	id := mustUUID(t, dash.ID)
	if err := r.service.Delete(origin.With(t.Context(), origin.Origin{Kind: origin.CLI}), testUser, id); err != nil {
		t.Fatal(err)
	}
	rows := r.activityFor(t, id)
	deleted := rows[len(rows)-1]
	if deleted.Type != activity.TypeDashboardDeleted || deleted.FolderID != dash.Folder.ID.String() || deleted.Origin != "cli" || deleted.ActorID != testUserID {
		t.Fatalf("last row = %+v, want dashboard.deleted in folder %s by the caller through the cli", deleted, dash.Folder.ID)
	}
	if !mapsEqual(deleted.Data, map[string]any{"title": "Doomed"}) {
		t.Fatalf("data = %v, want only the title", deleted.Data)
	}
}

func TestDeleteOfADashboardBeingCreatedAppendsNothing(t *testing.T) {
	r := newRig(t, true)
	request, err := r.revisions.SubmitObserve(t.Context(), testUser, "checkout health")
	if err != nil {
		t.Fatal(err)
	}
	dash, _, _, err := r.service.CreateRequestDashboard(t.Context(), testUser, pgtype.UUID{}, panelsSpec("Being built", "rps"), "agent", request.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := len(r.activityRows(t))
	if err := r.service.Delete(t.Context(), testUser, mustUUID(t, dash.ID)); !errors.Is(err, ErrBeingCreated) {
		t.Fatalf("delete error = %v, want ErrBeingCreated", err)
	}
	if after := len(r.activityRows(t)); after != before {
		t.Fatalf("activity rows went from %d to %d, want no change", before, after)
	}
}

func TestDeleteOfAMissingDashboardIsNotFound(t *testing.T) {
	r := newRig(t, false)
	if err := r.store().delete(t.Context(), mustUUID(t, "0b8f6f0e-3c9a-4a43-9d0a-6f2b0c1d2e3f"), actor.Actor{Kind: auth.KindUser, ID: testUserID, Name: "Ada"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete error = %v, want ErrNotFound", err)
	}
	if rows := r.activityRows(t); len(rows) != 0 {
		t.Fatalf("activity rows = %+v, want none", rows)
	}
}

func (r *rig) store() store { return r.service.store }

func appendAnnotation(ctx context.Context, q *dbsqlc.Queries, dash pgtype.UUID) error {
	target, err := db.ParseUUID("5f3c1a52-8a0e-4b8c-9a55-0a6f3e4b7d10")
	if err != nil {
		return err
	}
	entry := activity.Entry{
		Type:     activity.TypeAnnotationCreated,
		TargetID: target,
		Owner:    activity.OnDashboard(dash),
		Actor:    actor.Actor{Kind: auth.KindUser, ID: testUserID, Name: "Ada"},
	}
	_, err = activity.Append(ctx, q, entry, nil)
	return err
}

func (r *rig) newFolder(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	f, err := r.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return f.ID
}

func TestAMoveRewritesTheFolderOfEarlierActivityAndAppendsOneRow(t *testing.T) {
	r := newRig(t, true)
	general := r.generalFolder(t)
	dash, _, err := r.service.Create(t.Context(), testUser, general, panelsSpec("Checkout", "rps"), "create")
	if err != nil {
		t.Fatal(err)
	}
	id := mustUUID(t, dash.ID)
	if err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		return appendAnnotation(t.Context(), q, id)
	}); err != nil {
		t.Fatal(err)
	}
	target := r.newFolder(t, "Payments")

	if _, _, err := r.service.move(origin.With(t.Context(), origin.Origin{Kind: origin.Web}), testUser, id, target); err != nil {
		t.Fatal(err)
	}

	rows := r.activityFor(t, id)
	if len(rows) != 3 {
		t.Fatalf("rows = %+v, want created, annotation.created and moved", rows)
	}
	for _, row := range rows {
		if row.FolderID != target.String() {
			t.Fatalf("%s is in folder %s, want the new folder %s", row.Type, row.FolderID, target)
		}
	}
	moved := rows[2]
	want := map[string]any{"title": "Checkout", "from_folder_id": general.String(), "to_folder_id": target.String()}
	if moved.Type != activity.TypeDashboardMoved || moved.ActorID != testUserID || moved.Origin != "web" || !mapsEqual(moved.Data, want) {
		t.Fatalf("last row = %+v, want dashboard.moved by the caller with data %v", moved, want)
	}
}

func TestAMoveOfAMissingDashboardAppendsNothing(t *testing.T) {
	r := newRig(t, false)
	err := r.store().move(t.Context(), mustUUID(t, "0b8f6f0e-3c9a-4a43-9d0a-6f2b0c1d2e3f"), r.generalFolder(t), r.newFolder(t, "Elsewhere"), actor.Actor{Kind: auth.KindUser, ID: testUserID, Name: "Ada"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("move error = %v, want ErrNotFound", err)
	}
	if rows := r.activityRows(t); len(rows) != 0 {
		t.Fatalf("activity rows = %+v, want none", rows)
	}
}

func (r *rig) waitForLockWaiter(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(activityWait)
	for time.Now().Before(deadline) {
		var waiting int
		if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no transaction ever waited on a lock")
}

func TestAConcurrentAppendFollowsAMove(t *testing.T) {
	r := newRig(t, true)
	general := r.generalFolder(t)
	dash, _, err := r.service.Create(t.Context(), testUser, general, panelsSpec("Checkout", "rps"), "create")
	if err != nil {
		t.Fatal(err)
	}
	id := mustUUID(t, dash.ID)
	target := r.newFolder(t, "Payments")

	appended, release, holderDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		holderDone <- r.db.InTx(context.WithoutCancel(t.Context()), func(q *dbsqlc.Queries) error {
			if err := appendAnnotation(t.Context(), q, id); err != nil {
				return err
			}
			close(appended)
			<-release
			return nil
		})
	}()
	select {
	case <-appended:
	case <-time.After(activityWait):
		t.Fatal("the holding transaction never appended")
	}

	moveDone := make(chan error, 1)
	go func() {
		_, _, err := r.service.move(context.WithoutCancel(t.Context()), testUser, id, target)
		moveDone <- err
	}()
	r.waitForLockWaiter(t)
	close(release)

	for name, done := range map[string]chan error{"holder": holderDone, "move": moveDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		case <-time.After(activityWait):
			t.Fatalf("%s never finished", name)
		}
	}
	rows := r.activityRows(t, activity.TypeAnnotationCreated)
	if row := onlyActivity(t, rows); row.FolderID != target.String() {
		t.Fatalf("the concurrent annotation row is in folder %s, want the new folder %s", row.FolderID, target)
	}
}

func TestOneNotificationPerSave(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	listening := make(chan struct{})
	payloads := make(chan string, 16)
	listenDone := make(chan error, 1)
	go func() {
		listenDone <- r.db.Listen(ctx, events.Channel, func() { close(listening) }, func(p string) { payloads <- p })
	}()
	select {
	case <-listening:
	case <-time.After(activityWait):
		t.Fatal("the listener never started")
	}

	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "p99"), "save"); err != nil {
		t.Fatal(err)
	}
	const sentinel = "end-of-save"
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `SELECT pg_notify($1, $2)`, events.Channel, sentinel); err != nil {
		t.Fatal(err)
	}

	var got []string
	for {
		select {
		case p := <-payloads:
			if p == sentinel {
				cancel()
				<-listenDone
				if len(got) != 1 {
					t.Fatalf("payloads = %v, want exactly one for the save", got)
				}
				var payload map[string]any
				if err := json.Unmarshal([]byte(got[0]), &payload); err != nil {
					t.Fatal(err)
				}
				if seq, _ := payload["seq"].(float64); seq <= 0 || payload["type"] != events.TypeVersionCreated || payload["dashboard_id"] != dash.String() {
					t.Fatalf("payload = %s, want seq, type %s and the dashboard id", got[0], events.TypeVersionCreated)
				}
				return
			}
			got = append(got, p)
		case <-time.After(activityWait):
			t.Fatalf("no sentinel after the save; payloads so far = %v", got)
		}
	}
}
