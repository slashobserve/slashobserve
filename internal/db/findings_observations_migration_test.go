package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const findingsObservationsVersion = 41

func foID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", 0x780000+n)
}

var (
	foDashboardFinding = foID(1)
	foFiledFinding     = foID(2)
	foServiceFinding   = foID(3)
	foPersonalFinding  = foID(4)
	foSummaryFinding   = foID(5)
	foHashFinding      = foID(6)
	foLinkedFinding    = foID(7)
	foTwoThreads       = foID(8)
	foShareA           = foID(9)
	foShareB           = foID(10)
	foExisting         = foID(11)

	foRequestDashboard = foID(21)
	foRequestLinked    = foID(22)
	foRequestShareA    = foID(23)
	foRequestShareB    = foID(24)
	foRequestExisting  = foID(25)
	foRequestComment   = foID(26)

	foStepFiveLinked   = foID(31)
	foStepFiveShared   = foID(32)
	foStepFiveExisting = foID(33)

	foThreadOld      = foID(41)
	foThreadNew      = foID(42)
	foThreadShareA   = foID(43)
	foThreadShareB   = foID(44)
	foThreadOwn      = foID(45)
	foThreadOfFinder = foID(46)
)

const (
	foPanelOne = `{"id":"p1","type":"timeseries","title":"Errors","gridPos":{"x":0,"y":0,"w":12,"h":8},"queries":[]}`
	foPanelTwo = `{"id":"p2","type":"stat","title":"Rate","gridPos":{"x":12,"y":0,"w":6,"h":4},"queries":[]}`
)

type foFixture struct {
	user, shared, personal, general string
}

func foScalar(t *testing.T, sqlDB *sql.DB, query string, args ...any) string {
	t.Helper()
	var got sql.NullString
	if err := sqlDB.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return got.String
}

func foSeedFinding(t *testing.T, d *dbtest.Database, id, title, markdown, evidence, dashboard, folder, kind, creator, request string) {
	t.Helper()
	d.Exec(t, `INSERT INTO findings (id, agent_request_id, dashboard_id, folder_id, title, markdown, evidence, time_from, time_to,
  created_by_kind, created_by_id, created_by_name, origin, created_at)
VALUES ($1::uuid, nullif($2, '')::uuid, nullif($3, '')::uuid, nullif($4, '')::uuid, $5, $6, $7::jsonb,
  '2026-01-01T10:00:00Z', '2026-01-01T11:00:00Z', $8, $9::uuid, 'Ada', 'api', '2026-01-02T09:00:00Z')`,
		id, request, dashboard, folder, title, markdown, evidence, kind, creator)
}

func foSeedRequest(t *testing.T, d *dbtest.Database, id string) {
	t.Helper()
	d.Exec(t, `INSERT INTO agent_requests (id, prompt, requested_by_kind, requested_by_id, requested_by_name, source, status, state)
VALUES ($1::uuid, 'p', 'user', gen_random_uuid(), 'Ada', 'investigate', 'succeeded', 'succeeded')`, id)
}

func foSeedStepFiveObservation(t *testing.T, d *dbtest.Database, f foFixture, id string, runs ...string) {
	t.Helper()
	d.Exec(t, `INSERT INTO observations (id, folder_id, title, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, $2::uuid, 'Written by a run', 'user', $3::uuid, 'Ada')`, id, f.general, f.user)
	for i, run := range runs {
		d.Exec(t, `INSERT INTO observation_blocks (observation_id, kind, position, body, run_id, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 'text', $2, '{"markdown":"concluded"}', $3::uuid, 'user', $4::uuid, 'Ada')`, id, i, run, f.user)
		d.Exec(t, `UPDATE agent_requests SET observation_id = $1::uuid WHERE id = $2::uuid`, id, run)
	}
}

func foSeedThread(t *testing.T, d *dbtest.Database, id, target, targetKind, status, createdAt string, messages ...[2]string) {
	t.Helper()
	d.Exec(t, fmt.Sprintf(`INSERT INTO threads (id, %s, status, resolved_at, resolved_by_kind, resolved_by_id, resolved_by_name, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, $3, CASE WHEN $3 = 'resolved' THEN $4::timestamptz END,
  CASE WHEN $3 = 'resolved' THEN 'user' END, CASE WHEN $3 = 'resolved' THEN gen_random_uuid() END,
  CASE WHEN $3 = 'resolved' THEN 'Bo' END, $4::timestamptz, $4::timestamptz)`, targetKind), id, target, status, createdAt)
	for _, m := range messages {
		d.Exec(t, `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name, created_at, updated_at)
VALUES ($1::uuid, $2, 'user', gen_random_uuid(), 'Ada', $3::timestamptz, $3::timestamptz)`, id, m[0], m[1])
	}
}

func foSeed(t *testing.T, d *dbtest.Database) foFixture {
	t.Helper()
	sqlDB := d.SQLDB(t)
	var f foFixture
	f.user = foScalar(t, sqlDB, `INSERT INTO users (oidc_issuer, oidc_subject, email, name) VALUES ('https://idp', gen_random_uuid()::text, 'ada@example.com', 'Ada') RETURNING id::text`)
	f.personal = personalFolderOf(t, sqlDB, f.user)
	f.shared = foScalar(t, sqlDB, `INSERT INTO folders (title) VALUES ('Payments') RETURNING id::text`)
	f.general = foScalar(t, sqlDB, `SELECT general_folder_id()::text`)
	serviceAccount := foScalar(t, sqlDB, `SELECT gen_random_uuid()::text`)

	seedDashboard(t, d, dashboardA, 1)
	seedVersion(t, d, dashboardA, 1, "{}")
	d.Exec(t, `UPDATE dashboards SET folder_id = $1::uuid WHERE id = $2::uuid`, f.shared, dashboardA)

	for _, run := range []string{foRequestDashboard, foRequestLinked, foRequestShareA, foRequestShareB, foRequestExisting} {
		foSeedRequest(t, d, run)
	}
	foSeedStepFiveObservation(t, d, f, foStepFiveLinked, foRequestLinked)
	foSeedStepFiveObservation(t, d, f, foStepFiveShared, foRequestShareA, foRequestShareB)
	foSeedStepFiveObservation(t, d, f, foStepFiveExisting, foRequestExisting)

	foSeedFinding(t, d, foDashboardFinding, "Latency rose", "## Latency up\ndetail", "["+foPanelOne+","+foPanelTwo+"]", dashboardA, "", "user", f.user, foRequestDashboard)
	foSeedFinding(t, d, foFiledFinding, "Filed", "filed body", "[]", "", f.shared, "user", f.user, "")
	foSeedFinding(t, d, foServiceFinding, "Unfiled by a service account", "service body", "["+foPanelOne+"]", "", "", "service_account", serviceAccount, "")
	foSeedFinding(t, d, foPersonalFinding, "Personal", "personal body", "[]", "", "", "user", f.user, "")
	foSeedFinding(t, d, foSummaryFinding, "The title", "\n\n# Checkout errors rose\nmore", "[]", "", f.shared, "user", f.user, "")
	foSeedFinding(t, d, foHashFinding, "Only a hash", "#", "[]", "", f.shared, "user", f.user, "")
	foSeedFinding(t, d, foLinkedFinding, "Linked", "linked body", "[]", "", "", "service_account", serviceAccount, foRequestLinked)
	foSeedFinding(t, d, foTwoThreads, "Two threads", "two", "[]", "", f.shared, "user", f.user, "")
	foSeedFinding(t, d, foShareA, "Share A", "a", "[]", "", "", "service_account", serviceAccount, foRequestShareA)
	foSeedFinding(t, d, foShareB, "Share B", "b", "[]", "", "", "service_account", serviceAccount, foRequestShareB)
	foSeedFinding(t, d, foExisting, "Existing discussion", "e", "[]", "", "", "service_account", serviceAccount, foRequestExisting)

	foSeedThread(t, d, foThreadOld, foTwoThreads, "finding_id", "resolved", "2026-01-02T10:00:00Z",
		[2]string{"old first", "2026-01-02T10:01:00Z"}, [2]string{"old third", "2026-01-02T10:03:00Z"})
	foSeedThread(t, d, foThreadNew, foTwoThreads, "finding_id", "open", "2026-01-02T10:02:00Z",
		[2]string{"new second", "2026-01-02T10:02:00Z"})
	foSeedThread(t, d, foThreadShareA, foShareA, "finding_id", "resolved", "2026-01-02T11:00:00Z", [2]string{"share a", "2026-01-02T11:01:00Z"})
	foSeedThread(t, d, foThreadShareB, foShareB, "finding_id", "resolved", "2026-01-02T11:30:00Z", [2]string{"share b", "2026-01-02T11:31:00Z"})
	foSeedThread(t, d, foThreadOwn, foStepFiveExisting, "observation_id", "open", "2026-01-02T12:00:00Z", [2]string{"own", "2026-01-02T12:01:00Z"})
	foSeedThread(t, d, foThreadOfFinder, foExisting, "finding_id", "open", "2026-01-02T12:30:00Z", [2]string{"from the finding", "2026-01-02T12:31:00Z"})

	d.Exec(t, `INSERT INTO agent_requests (id, dashboard_id, source, comment_id, prompt, base_version, requested_by_kind, requested_by_id, requested_by_name)
VALUES ($1::uuid, $2::uuid, 'comment', $3::uuid, 'p', 1, 'user', gen_random_uuid(), 'Ada')`, foRequestComment, dashboardA, foThreadNew)

	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, finding_id, folder_id, actor_kind, actor_id, actor_name)
VALUES ('finding.created', 'finding', $1::uuid, $1::uuid, NULL, 'service_account', $2::uuid, 'bot')`, foServiceFinding, serviceAccount)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, finding_id, folder_id, actor_kind, actor_id, actor_name)
VALUES ('thread.replied', 'thread', $1::uuid, $2::uuid, $3::uuid, 'user', $4::uuid, 'Ada')`, foThreadNew, foTwoThreads, f.shared, f.user)
	return f
}

func foMigrated(t *testing.T) (*dbtest.Database, foFixture, *sql.DB) {
	t.Helper()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(context.Background(), findingsObservationsVersion-1); err != nil {
		t.Fatal(err)
	}
	f := foSeed(t, d)
	if err := d.MigrateUpTo(context.Background(), findingsObservationsVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	return d, f, d.SQLDB(t)
}

func TestFindingsObservationsUpPlacesEveryFinding(t *testing.T) {
	_, f, sqlDB := foMigrated(t)

	if n := count(t, sqlDB, `SELECT count(*) FROM findings WHERE observation_id IS NULL`); n != 0 {
		t.Fatalf("%d findings without an observation", n)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM findings WHERE folder_id IS NOT NULL`); n != 0 {
		t.Fatalf("%d findings keep a folder", n)
	}

	placements := []struct {
		name, finding, folder string
	}{
		{"a finding on a dashboard goes to the dashboard's folder", foDashboardFinding, f.shared},
		{"a filed finding stays where it was filed", foFiledFinding, f.shared},
		{"an unfiled service account finding goes to General", foServiceFinding, f.general},
		{"an unfiled user finding goes to the personal folder", foPersonalFinding, f.personal},
	}
	for _, c := range placements {
		got := foScalar(t, sqlDB, `SELECT o.folder_id::text FROM observations o JOIN findings f ON f.observation_id = o.id WHERE f.id = $1::uuid AND o.id = f.id`, c.finding)
		if got != c.folder {
			t.Errorf("%s: folder = %s, want %s", c.name, got, c.folder)
		}
	}

	var title, state, summary, createdBy, origin, run, resolvedAtMatches, updatedAtMatches string
	err := sqlDB.QueryRow(`SELECT title, state, summary, created_by_name, origin, run_id::text,
  (resolved_at = '2026-01-02T09:00:00Z')::text, (created_at = '2026-01-02T09:00:00Z' AND updated_at = '2026-01-02T09:00:00Z')::text
FROM observations WHERE id = $1::uuid`, foDashboardFinding).
		Scan(&title, &state, &summary, &createdBy, &origin, &run, &resolvedAtMatches, &updatedAtMatches)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Latency rose" || state != "resolved" || summary != "Latency up" || createdBy != "Ada" || origin != "api" || run != foRequestDashboard ||
		resolvedAtMatches != "true" || updatedAtMatches != "true" {
		t.Errorf("observation = %q %q %q %q %q %q %s %s", title, state, summary, createdBy, origin, run, resolvedAtMatches, updatedAtMatches)
	}

	rows, err := sqlDB.Query(`SELECT position, kind, state, width, run_id::text, body::text FROM observation_blocks WHERE observation_id = $1::uuid ORDER BY position`, foDashboardFinding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rows.Close() })
	type block struct {
		position                int
		kind, state, width, run string
		body                    string
	}
	var blocks []block
	for rows.Next() {
		var b block
		if err := rows.Scan(&b.position, &b.kind, &b.state, &b.width, &b.run, &b.body); err != nil {
			t.Fatal(err)
		}
		blocks = append(blocks, b)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 3 {
		t.Fatalf("blocks = %+v, want a text block and two panel blocks", blocks)
	}
	wantBlocks := []struct{ kind, width, body string }{
		{"text", "full", `{"markdown": "## Latency up\ndetail"}`},
		{"panel", "half", `{"to": "2026-01-01T11:00:00.000000Z", "from": "2026-01-01T10:00:00.000000Z", "panel": {"type": "timeseries", "title": "Errors", "queries": []}}`},
		{"panel", "half", `{"to": "2026-01-01T11:00:00.000000Z", "from": "2026-01-01T10:00:00.000000Z", "panel": {"type": "stat", "title": "Rate", "queries": []}}`},
	}
	for i, want := range wantBlocks {
		b := blocks[i]
		sameBody := foScalar(t, sqlDB, `SELECT ($1::jsonb = $2::jsonb)::text`, b.body, want.body)
		if b.position != i || b.kind != want.kind || b.width != want.width || b.state != "live" || b.run != foRequestDashboard || sameBody != "true" {
			t.Errorf("block %d = %+v, want %s %s %s", i, b, want.kind, want.width, want.body)
		}
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM observation_blocks WHERE observation_id = $1::uuid`, foFiledFinding); n != 1 {
		t.Errorf("a finding without evidence has %d blocks, want only the text block", n)
	}
}

func TestFindingsObservationsUpSummarisesTheMarkdown(t *testing.T) {
	_, _, sqlDB := foMigrated(t)
	cases := []struct{ finding, want string }{
		{foSummaryFinding, "Checkout errors rose"},
		{foHashFinding, "Only a hash"},
	}
	for _, c := range cases {
		if got := foScalar(t, sqlDB, `SELECT summary FROM observations WHERE id = $1::uuid`, c.finding); got != c.want {
			t.Errorf("summary = %q, want %q", got, c.want)
		}
	}
}

func TestFindingsObservationsUpLinksAFindingToItsRunsObservation(t *testing.T) {
	_, _, sqlDB := foMigrated(t)
	if got := foScalar(t, sqlDB, `SELECT observation_id::text FROM findings WHERE id = $1::uuid`, foLinkedFinding); got != foStepFiveLinked {
		t.Errorf("linked finding observation = %s, want %s", got, foStepFiveLinked)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM observations WHERE id = $1::uuid`, foLinkedFinding); n != 0 {
		t.Errorf("%d new observations for a linked finding", n)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM observation_blocks WHERE observation_id = $1::uuid`, foStepFiveLinked); n != 1 {
		t.Errorf("the run's observation has %d blocks, want its own one", n)
	}
	if got := foScalar(t, sqlDB, `SELECT observation_id::text FROM agent_requests WHERE id = $1::uuid`, foRequestDashboard); got != foDashboardFinding {
		t.Errorf("run observation = %s, want %s", got, foDashboardFinding)
	}
}

func TestFindingsObservationsUpMergesThreadsIntoOneDiscussion(t *testing.T) {
	_, _, sqlDB := foMigrated(t)

	if n := count(t, sqlDB, `SELECT count(*) FROM threads WHERE finding_id IS NOT NULL`); n != 0 {
		t.Errorf("%d threads still belong to a finding", n)
	}

	var id, status string
	var resolvedAtNull, resolverNull bool
	err := sqlDB.QueryRow(`SELECT id::text, status, resolved_at IS NULL, resolved_by_id IS NULL AND resolved_by_name IS NULL FROM threads WHERE observation_id = $1::uuid`, foTwoThreads).
		Scan(&id, &status, &resolvedAtNull, &resolverNull)
	if err != nil {
		t.Fatalf("the finding's discussion: %v", err)
	}
	if id != foThreadOld || status != "open" || !resolvedAtNull || !resolverNull {
		t.Errorf("discussion = %s %s resolved_at null %v resolver null %v, want the oldest thread reopened", id, status, resolvedAtNull, resolverNull)
	}
	bodies := foScalar(t, sqlDB, `SELECT string_agg(body, ',' ORDER BY created_at, id) FROM thread_messages WHERE thread_id = $1::uuid`, foThreadOld)
	if bodies != "old first,new second,old third" {
		t.Errorf("messages = %q", bodies)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM threads WHERE id = $1::uuid`, foThreadNew); n != 0 {
		t.Error("the merged thread still exists")
	}
	if got := foScalar(t, sqlDB, `SELECT target_id::text FROM activity WHERE type = 'thread.replied'`); got != foThreadOld {
		t.Errorf("thread activity target = %s, want %s", got, foThreadOld)
	}
	if got := foScalar(t, sqlDB, `SELECT comment_id::text FROM agent_requests WHERE id = $1::uuid`, foRequestComment); got != foThreadOld {
		t.Errorf("run's thread = %s, want %s", got, foThreadOld)
	}

	if n := count(t, sqlDB, `SELECT count(*) FROM threads WHERE observation_id = $1::uuid`, foStepFiveShared); n != 1 {
		t.Errorf("two findings of one observation left %d discussions", n)
	}
	if got := foScalar(t, sqlDB, `SELECT string_agg(m.body, ',' ORDER BY m.created_at) FROM thread_messages m JOIN threads t ON t.id = m.thread_id WHERE t.observation_id = $1::uuid`, foStepFiveShared); got != "share a,share b" {
		t.Errorf("shared discussion messages = %q", got)
	}
	if got := foScalar(t, sqlDB, `SELECT status FROM threads WHERE observation_id = $1::uuid`, foStepFiveShared); got != "resolved" {
		t.Errorf("shared discussion status = %s, want its own resolution kept", got)
	}

	if got := foScalar(t, sqlDB, `SELECT id::text FROM threads WHERE observation_id = $1::uuid`, foStepFiveExisting); got != foThreadOwn {
		t.Errorf("existing discussion = %s, want %s kept", got, foThreadOwn)
	}
	if got := foScalar(t, sqlDB, `SELECT string_agg(body, ',' ORDER BY created_at) FROM thread_messages WHERE thread_id = $1::uuid`, foThreadOwn); got != "own,from the finding" {
		t.Errorf("existing discussion messages = %q", got)
	}
}

func TestFindingsObservationsUpMovesFindingActivityToTheObservation(t *testing.T) {
	_, f, sqlDB := foMigrated(t)
	var observation, folder string
	if err := sqlDB.QueryRow(`SELECT observation_id::text, folder_id::text FROM activity WHERE type = 'finding.created'`).Scan(&observation, &folder); err != nil {
		t.Fatal(err)
	}
	if observation != foServiceFinding || folder != f.general {
		t.Errorf("activity observation %s folder %s, want %s in General", observation, folder, foServiceFinding)
	}
	if got := foScalar(t, sqlDB, `SELECT observation_id::text FROM activity WHERE type = 'thread.replied'`); got != foTwoThreads {
		t.Errorf("thread activity observation = %s, want %s", got, foTwoThreads)
	}
	if got := foScalar(t, sqlDB, `SELECT target_kind FROM activity WHERE type = 'finding.created'`); got != "finding" {
		t.Errorf("history target kind = %s, want finding", got)
	}
}

func TestFindingsObservationsDeletesFollowTheObservation(t *testing.T) {
	_, f, sqlDB := foMigrated(t)

	if _, err := sqlDB.Exec(`DELETE FROM observations WHERE id = $1::uuid`, foFiledFinding); err != nil {
		t.Fatal(err)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM findings WHERE id = $1::uuid`, foFiledFinding); n != 0 {
		t.Error("deleting the observation kept its finding")
	}

	if _, err := sqlDB.Exec(`DELETE FROM users WHERE id = $1::uuid`, f.user); err != nil {
		t.Fatal(err)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM observations WHERE id = $1::uuid`, foPersonalFinding); n != 0 {
		t.Error("deleting the user kept the observation in their personal folder")
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM findings WHERE id = $1::uuid`, foPersonalFinding); n != 0 {
		t.Error("deleting the user kept the finding in their personal folder")
	}
}

func TestFindingsObservationsFolderEmptinessFollowsTheObservation(t *testing.T) {
	d, f, sqlDB := foMigrated(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, d.URL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	queries := dbsqlc.New(conn)

	folder := foScalar(t, sqlDB, `INSERT INTO folders (title) VALUES ('Only findings') RETURNING id::text`)
	if _, err := sqlDB.Exec(`UPDATE observations SET folder_id = $1::uuid WHERE id = $2::uuid`, folder, foFiledFinding); err != nil {
		t.Fatal(err)
	}
	var folderUUID pgtype.UUID
	if err := folderUUID.Scan(folder); err != nil {
		t.Fatal(err)
	}
	has, err := queries.FolderHasContents(ctx, folderUUID)
	if err != nil || !has {
		t.Fatalf("a folder holding an observation: has contents = %v, %v", has, err)
	}
	if _, err := sqlDB.Exec(`UPDATE observations SET folder_id = $1::uuid WHERE id = $2::uuid`, f.shared, foFiledFinding); err != nil {
		t.Fatal(err)
	}
	has, err = queries.FolderHasContents(ctx, folderUUID)
	if err != nil || has {
		t.Fatalf("after moving the observation: has contents = %v, %v", has, err)
	}
}

func TestFindingsObservationsDownRestoresTheFindingsAndAppliesAgain(t *testing.T) {
	d, f, sqlDB := foMigrated(t)
	ctx := context.Background()

	if err := d.MigrateDownTo(ctx, findingsObservationsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}

	if n := count(t, sqlDB, `SELECT count(*) FROM information_schema.columns WHERE table_name = 'findings' AND column_name = 'observation_id'`); n != 0 {
		t.Error("findings keeps observation_id")
	}
	folders := map[string]string{foFiledFinding: f.shared, foServiceFinding: f.general, foPersonalFinding: f.personal, foLinkedFinding: f.general}
	for finding, want := range folders {
		if got := foScalar(t, sqlDB, `SELECT folder_id::text FROM findings WHERE id = $1::uuid`, finding); got != want {
			t.Errorf("finding %s folder = %s, want %s", finding, got, want)
		}
	}
	if got := foScalar(t, sqlDB, `SELECT folder_id::text FROM findings WHERE id = $1::uuid`, foDashboardFinding); got != "" {
		t.Errorf("a dashboard finding got folder %s", got)
	}
	if got := foScalar(t, sqlDB, `SELECT folder_id::text FROM activity WHERE type = 'finding.created'`); got != f.general {
		t.Errorf("finding activity folder = %s, want %s", got, f.general)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM activity WHERE observation_id IS NOT NULL`); n != 0 {
		t.Errorf("%d activity rows keep an observation", n)
	}

	if got := foScalar(t, sqlDB, `SELECT finding_id::text FROM threads WHERE id = $1::uuid`, foThreadOld); got != foTwoThreads {
		t.Errorf("the merged thread belongs to finding %q, want %s", got, foTwoThreads)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM threads WHERE id = $1::uuid AND observation_id IS NULL`, foThreadOld); n != 1 {
		t.Error("the merged thread still belongs to an observation")
	}

	for _, converted := range []string{foDashboardFinding, foFiledFinding, foServiceFinding, foPersonalFinding, foSummaryFinding, foHashFinding} {
		if n := count(t, sqlDB, `SELECT count(*) FROM observations WHERE id = $1::uuid`, converted); n != 0 {
			t.Errorf("converted observation %s survived the down migration", converted)
		}
	}
	for _, kept := range []string{foStepFiveLinked, foStepFiveShared, foStepFiveExisting} {
		if n := count(t, sqlDB, `SELECT count(*) FROM observations WHERE id = $1::uuid`, kept); n != 1 {
			t.Errorf("run observation %s was deleted by the down migration", kept)
		}
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM findings`); n != 11 {
		t.Errorf("%d findings after the down migration, want 11", n)
	}

	if err := d.MigrateUpTo(ctx, findingsObservationsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM findings WHERE observation_id IS NULL`); n != 0 {
		t.Errorf("%d findings without an observation after applying again", n)
	}
}
