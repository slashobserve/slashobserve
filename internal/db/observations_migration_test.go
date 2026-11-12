package db_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	observationsVersion = 38

	obsDashboard = "00000000-0000-4000-8000-0000000007a1"
	obsRequest   = "00000000-0000-4000-8000-0000000007a2"
	obsThread    = "00000000-0000-4000-8000-0000000007b1"
	obsOtherPair = "00000000-0000-4000-8000-0000000007b2"
)

type observationFixture struct {
	observation, snapshot, block, rca, user string
}

func seedObservation(t *testing.T, d *dbtest.Database, folder string) observationFixture {
	t.Helper()
	var f observationFixture
	sqlDB := d.SQLDB(t)
	scan := func(dst *string, query string, args ...any) {
		t.Helper()
		if err := sqlDB.QueryRow(query, args...).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	scan(&f.user, `INSERT INTO users (oidc_issuer, oidc_subject, email, name) VALUES ('https://idp', gen_random_uuid()::text, gen_random_uuid()::text || '@example.com', 'Ada') RETURNING id::text`)
	scan(&f.observation, `INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name)
VALUES (CASE WHEN $1 = '' THEN general_folder_id() ELSE $1::uuid END, 'Checkout slow', 'user', $2::uuid, 'Ada') RETURNING id::text`, folder, f.user)
	scan(&f.snapshot, `INSERT INTO snapshots (observation_id, panel, query, frames, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, '{"type":"timeseries"}', '[]', '{"frames":[]}', now() - interval '1 hour', now(), 'user', $2::uuid, 'Ada') RETURNING id::text`, f.observation, f.user)
	scan(&f.block, `INSERT INTO observation_blocks (observation_id, kind, position, body, snapshot_id, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 'snapshot', 0, '{}', $2::uuid, 'user', $3::uuid, 'Ada') RETURNING id::text`, f.observation, f.snapshot, f.user)
	scan(&f.rca, `INSERT INTO observation_rcas (observation_id, revision, title, observation_state, content, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 1, 'Checkout slow', 'open', '{}', 'user', $2::uuid, 'Ada') RETURNING id::text`, f.observation, f.user)
	return f
}

func count(t *testing.T, sqlDB *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := sqlDB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestObservationsRejectInconsistentRows(t *testing.T) {
	d := dbtest.New(t)
	f := seedObservation(t, d, "")
	sqlDB := d.SQLDB(t)
	cases := []struct {
		name, statement, want string
	}{
		{"resolved without a summary", `UPDATE observations SET state = 'resolved', resolved_at = now() WHERE id = $1::uuid`, "observations_resolved_check"},
		{"resolved with a summary and a time", `UPDATE observations SET state = 'resolved', summary = 'rolled back', resolved_at = now() WHERE id = $1::uuid`, ""},
		{"resolved without a time", `UPDATE observations SET resolved_at = NULL WHERE id = $1::uuid`, "observations_resolved_at_check"},
		{"open with a resolution time", `UPDATE observations SET state = 'open', resolved_at = now() WHERE id = $1::uuid`, "observations_resolved_at_check"},
		{"a range with one end", `UPDATE observations SET time_from = now() WHERE id = $1::uuid`, "observations_time_check"},
		{"a range that ends first", `UPDATE observations SET time_from = now(), time_to = now() - interval '1 hour' WHERE id = $1::uuid`, "observations_time_check"},
		{"an empty title", `UPDATE observations SET title = '' WHERE id = $1::uuid`, "observations_title_check"},
		{"a summary that is too long", `UPDATE observations SET summary = repeat('x', 501) WHERE id = $1::uuid`, "observations_summary_check"},
	}
	for _, c := range cases {
		_, err := sqlDB.Exec(c.statement, f.observation)
		if got := constraintOf(err); got != c.want || (c.want == "" && err != nil) {
			t.Errorf("%s: error = %v, want violation %q", c.name, err, c.want)
		}
	}
}

func TestBlocksNeedASnapshotExactlyWhenTheyAreSnapshots(t *testing.T) {
	d := dbtest.New(t)
	f := seedObservation(t, d, "")
	sqlDB := d.SQLDB(t)
	insert := func(kind, snapshot string, position int) error {
		_, err := sqlDB.Exec(`INSERT INTO observation_blocks (observation_id, kind, position, body, snapshot_id, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, $2, $3, '{}', nullif($4, '')::uuid, 'user', $5::uuid, 'Ada')`, f.observation, kind, position, snapshot, f.user)
		return err
	}
	cases := []struct {
		name, kind, snapshot string
		position             int
		want                 string
	}{
		{"a snapshot block without a snapshot", "snapshot", "", 1, "observation_blocks_snapshot_check"},
		{"a text block with a snapshot", "text", f.snapshot, 1, "observation_blocks_snapshot_check"},
		{"a text block", "text", "", 1, ""},
		{"a run block without a run", "run", "", 2, ""},
		{"a position already used", "text", "", 1, "observation_blocks_position_key"},
	}
	for _, c := range cases {
		err := insert(c.kind, c.snapshot, c.position)
		if got := constraintOf(err); got != c.want || (c.want == "" && err != nil) {
			t.Errorf("%s: error = %v, want violation %q", c.name, err, c.want)
		}
	}
}

func TestBlockPositionsMayShiftInOneStatement(t *testing.T) {
	d := dbtest.New(t)
	f := seedObservation(t, d, "")
	sqlDB := d.SQLDB(t)
	for position := 1; position <= 2; position++ {
		d.Exec(t, `INSERT INTO observation_blocks (observation_id, kind, position, body, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 'text', $2, '{}', 'user', $3::uuid, 'Ada')`, f.observation, position, f.user)
	}
	if _, err := sqlDB.Exec(`UPDATE observation_blocks SET position = position + 1 WHERE observation_id = $1::uuid`, f.observation); err != nil {
		t.Fatalf("shift: %v", err)
	}
}

func TestOnlyOneThreadMayHangOnAnObservation(t *testing.T) {
	d := dbtest.New(t)
	f := seedObservation(t, d, "")
	seedDashboard(t, d, obsDashboard, 1)
	seedVersion(t, d, obsDashboard, 1, "{}")
	sqlDB := d.SQLDB(t)
	if _, err := sqlDB.Exec(`INSERT INTO threads (id, observation_id) VALUES ($1::uuid, $2::uuid)`, obsThread, f.observation); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, statement, want string
		args                  []any
	}{
		{"a second thread", `INSERT INTO threads (observation_id) VALUES ($1::uuid)`, "threads_observation_idx", []any{f.observation}},
		{"a dashboard and an observation", `INSERT INTO threads (dashboard_id, dashboard_version, observation_id) VALUES ($1::uuid, 1, $2::uuid)`, "threads_target_check", []any{obsDashboard, f.observation}},
		{"no target", `INSERT INTO threads DEFAULT VALUES`, "threads_target_check", nil},
	}
	for _, c := range cases {
		_, err := sqlDB.Exec(c.statement, c.args...)
		if got := constraintOf(err); got != c.want {
			t.Errorf("%s: error = %v, want violation %q", c.name, err, c.want)
		}
	}
}

const insertPublicLinkSQL = `INSERT INTO public_links (dashboard_id, rca_id, token_hash, display_prefix, variables, created_by_kind, created_by_id)
VALUES (nullif($1, '')::uuid, nullif($2, '')::uuid, decode(md5($3) || md5($3), 'hex'), 'abc', 'frozen', 'user', gen_random_uuid())`

func TestAPublicLinkHasExactlyOneSubject(t *testing.T) {
	d := dbtest.New(t)
	f := seedObservation(t, d, "")
	seedDashboard(t, d, obsDashboard, 1)
	sqlDB := d.SQLDB(t)
	cases := []struct {
		name, dashboard, rca, want string
	}{
		{"both subjects", obsDashboard, f.rca, "public_links_subject_check"},
		{"no subject", "", "", "public_links_subject_check"},
		{"a dashboard", obsDashboard, "", ""},
		{"an RCA", "", f.rca, ""},
	}
	for i, c := range cases {
		_, err := sqlDB.Exec(insertPublicLinkSQL, c.dashboard, c.rca, string(rune('a'+i)))
		if got := constraintOf(err); got != c.want || (c.want == "" && err != nil) {
			t.Errorf("%s: error = %v, want violation %q", c.name, err, c.want)
		}
	}
}

func TestSnapshotsAndRCAsCannotBeChanged(t *testing.T) {
	d := dbtest.New(t)
	f := seedObservation(t, d, "")
	sqlDB := d.SQLDB(t)
	for _, statement := range []string{
		`UPDATE observation_rcas SET title = 'x' WHERE id = $1::uuid`,
		`UPDATE snapshots SET frames = '{}' WHERE id = $1::uuid`,
	} {
		id := f.rca
		if strings.Contains(statement, "snapshots") {
			id = f.snapshot
		}
		_, err := sqlDB.Exec(statement, id)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s: error = %v, want a check violation", statement, err)
		}
	}
	if _, err := sqlDB.Exec(`DELETE FROM observations WHERE id = $1::uuid`, f.observation); err != nil {
		t.Fatalf("delete the observation: %v", err)
	}
	for _, table := range []string{"observation_rcas", "snapshots", "observation_blocks"} {
		if n := count(t, sqlDB, `SELECT count(*) FROM `+table); n != 0 {
			t.Errorf("%d rows of %s outlive their observation", n, table)
		}
	}
}

func TestSnapshotFramesAreCappedAtTwoMebibytes(t *testing.T) {
	d := dbtest.New(t)
	f := seedObservation(t, d, "")
	_, err := d.SQLDB(t).Exec(`INSERT INTO snapshots (observation_id, panel, query, frames, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, '{}', '[]', jsonb_build_object('frames', (SELECT string_agg(md5(i::text), '') FROM generate_series(1, 150000) i)), now() - interval '1 hour', now(), 'user', $2::uuid, 'Ada')`, f.observation, f.user)
	if got := constraintOf(err); got != "snapshots_size_check" {
		t.Fatalf("error = %v, want snapshots_size_check", err)
	}
}

func TestDeletingAUserDeletesTheObservationsOfTheirPersonalFolder(t *testing.T) {
	d := dbtest.New(t)
	sqlDB := d.SQLDB(t)
	var user, personal string
	if err := sqlDB.QueryRow(`INSERT INTO users (oidc_issuer, oidc_subject, email, name) VALUES ('https://idp', 'grace', 'grace@example.com', 'Grace') RETURNING id::text`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRow(`SELECT id::text FROM folders WHERE personal AND owner_id = $1::uuid`, user).Scan(&personal); err != nil {
		t.Fatal(err)
	}
	seedObservation(t, d, personal)
	seedObservation(t, d, "")
	if _, err := sqlDB.Exec(`DELETE FROM users WHERE id = $1::uuid`, user); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM observations`); n != 1 {
		t.Fatalf("%d observations left, want only the one in General", n)
	}
}

func TestDeletingARunKeepsWhatItWrote(t *testing.T) {
	d := dbtest.New(t)
	f := seedObservation(t, d, "")
	sqlDB := d.SQLDB(t)
	d.Exec(t, `INSERT INTO agent_requests (id, prompt, requested_by_kind, requested_by_id, requested_by_name, source) VALUES ($1::uuid, 'p', 'user', $2::uuid, 'Ada', 'investigate')`, obsRequest, f.user)
	d.Exec(t, `UPDATE observations SET run_id = $1::uuid WHERE id = $2::uuid`, obsRequest, f.observation)
	d.Exec(t, `UPDATE observation_blocks SET run_id = $1::uuid WHERE id = $2::uuid`, obsRequest, f.block)
	d.Exec(t, `INSERT INTO snapshots (observation_id, panel, query, frames, time_from, time_to, created_by_kind, created_by_id, created_by_name, run_id)
VALUES ($1::uuid, '{}', '[]', '{}', now() - interval '1 hour', now(), 'user', $2::uuid, 'Ada', $3::uuid)`, f.observation, f.user, obsRequest)
	d.Exec(t, `INSERT INTO observation_rcas (observation_id, revision, title, observation_state, content, created_by_kind, created_by_id, created_by_name, run_id)
VALUES ($1::uuid, 2, 'Checkout slow', 'open', '{}', 'user', $2::uuid, 'Ada', $3::uuid)`, f.observation, f.user, obsRequest)
	if _, err := sqlDB.Exec(`DELETE FROM agent_requests WHERE id = $1::uuid`, obsRequest); err != nil {
		t.Fatalf("delete the run: %v", err)
	}
	for _, table := range []string{"observations", "observation_blocks", "snapshots", "observation_rcas"} {
		if n := count(t, sqlDB, `SELECT count(*) FROM `+table+` WHERE run_id IS NOT NULL`); n != 0 {
			t.Errorf("%s keeps %d run ids after the run was deleted", table, n)
		}
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM snapshots`); n != 2 {
		t.Errorf("%d snapshots, want 2", n)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM observation_rcas`); n != 2 {
		t.Errorf("%d RCA revisions, want 2", n)
	}
}

func TestObservationSearchIndexesServeTheirVectorExpressions(t *testing.T) {
	d := dbtest.New(t)
	sqlDB := d.SQLDB(t)
	cases := []struct{ name, index, explain string }{
		{"observations", "observations_search_idx", `EXPLAIN SELECT 1 FROM observations WHERE observation_search_vector(title, summary) @@ search_query('zebra', NULL)`},
		{"observation_blocks", "observation_blocks_search_idx", `EXPLAIN SELECT 1 FROM observation_blocks WHERE kind = 'text' AND state = 'live' AND observation_block_search_vector(body) @@ search_query('zebra', NULL)`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tx, err := sqlDB.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.Exec(`SET LOCAL enable_seqscan = off`); err != nil {
				t.Fatal(err)
			}
			rows, err := tx.Query(c.explain)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rows.Close() }()
			var plan strings.Builder
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					t.Fatal(err)
				}
				plan.WriteString(line + "\n")
			}
			if !strings.Contains(plan.String(), c.index) {
				t.Fatalf("plan does not name %s:\n%s", c.index, plan.String())
			}
		})
	}
}

func TestObservationSearchVectorsMatchTheirWords(t *testing.T) {
	d := dbtest.New(t)
	sqlDB := d.SQLDB(t)
	cases := []struct {
		name, query string
		want        bool
	}{
		{"the title", `SELECT observation_search_vector('Checkout latency', NULL) @@ search_query('latency', NULL)`, true},
		{"the summary", `SELECT observation_search_vector('Checkout', 'pool exhausted') @@ search_query('exhausted', NULL)`, true},
		{"a missing summary", `SELECT observation_search_vector('Checkout', NULL) @@ search_query('exhausted', NULL)`, false},
		{"the markdown of a block", `SELECT observation_block_search_vector('{"markdown":"the deploy broke it"}') @@ search_query('deploy', NULL)`, true},
		{"a body without markdown", `SELECT observation_block_search_vector('{"snapshot_id":"deploy"}') @@ search_query('deploy', NULL)`, false},
	}
	for _, c := range cases {
		var got bool
		if err := sqlDB.QueryRow(c.query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: matched %v, want %v", c.name, got, c.want)
		}
	}
}

func TestObservationsDownAndUpAgain(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, observationsVersion); err != nil {
		t.Fatal(err)
	}
	f := seedObservation(t, d, "")
	seedDashboard(t, d, obsDashboard, 1)
	seedVersion(t, d, obsDashboard, 1, "{}")
	d.Exec(t, `INSERT INTO threads (id, observation_id) VALUES ($1::uuid, $2::uuid)`, obsThread, f.observation)
	d.Exec(t, `INSERT INTO threads (id, dashboard_id, dashboard_version) VALUES ($1::uuid, $2::uuid, 1)`, obsOtherPair, obsDashboard)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, observation_id, folder_id, actor_kind, actor_id, actor_name)
VALUES ('observation.created', 'observation', $1::uuid, $1::uuid, general_folder_id(), 'user', $2::uuid, 'Ada')`, f.observation, f.user)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, dashboard_id, folder_id, actor_kind, actor_id, actor_name)
VALUES ('dashboard.created', 'dashboard', $1::uuid, $1::uuid, general_folder_id(), 'user', $2::uuid, 'Ada')`, obsDashboard, f.user)
	d.Exec(t, `INSERT INTO agent_requests (id, prompt, requested_by_kind, requested_by_id, requested_by_name, source, observation_id) VALUES ($1::uuid, 'p', 'user', $2::uuid, 'Ada', 'investigate', $3::uuid)`, obsRequest, f.user, f.observation)
	d.Exec(t, insertPublicLinkSQL, obsDashboard, "", "a")
	d.Exec(t, insertPublicLinkSQL, "", f.rca, "b")
	sqlDB := d.SQLDB(t)

	if err := d.MigrateDownTo(ctx, observationsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	for _, table := range []string{"observations", "observation_blocks", "snapshots", "observation_rcas"} {
		if count(t, sqlDB, `SELECT count(*) FROM pg_class WHERE relname = $1`, table) != 0 {
			t.Errorf("%s still exists after the down migration", table)
		}
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM threads`); n != 1 {
		t.Errorf("%d threads after the down migration, want the dashboard's only", n)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM activity`); n != 1 {
		t.Errorf("%d activity rows after the down migration, want the dashboard's only", n)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM public_links`); n != 1 {
		t.Errorf("%d public links after the down migration, want the dashboard's only", n)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM agent_requests`); n != 1 {
		t.Errorf("%d runs after the down migration, want the run kept", n)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM information_schema.columns WHERE column_name = 'observation_id' AND table_name IN ('threads', 'activity', 'agent_requests')`); n != 0 {
		t.Errorf("%d observation_id columns remain", n)
	}
	_, err := sqlDB.Exec(`INSERT INTO public_links (token_hash, display_prefix, variables, created_by_kind, created_by_id)
VALUES (decode(md5('c') || md5('c'), 'hex'), 'abc', 'frozen', 'user', gen_random_uuid())`)
	if err == nil || !strings.Contains(err.Error(), "dashboard_id") {
		t.Errorf("a public link without a dashboard: error = %v, want a not null violation on dashboard_id", err)
	}
	_, err = sqlDB.Exec(`INSERT INTO threads (dashboard_id, dashboard_version, annotation_id) VALUES ($1::uuid, 1, gen_random_uuid())`, obsDashboard)
	if got := constraintOf(err); got != "threads_target_check" {
		t.Errorf("a thread with two targets: error = %v, want threads_target_check", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO threads (dashboard_id, dashboard_version) VALUES ($1::uuid, 1)`, obsDashboard); err != nil {
		t.Errorf("a dashboard thread after the down migration: %v", err)
	}

	if err := d.MigrateUpTo(ctx, observationsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	seedObservation(t, d, "")
	if n := count(t, sqlDB, `SELECT count(*) FROM observations`); n != 1 {
		t.Errorf("%d observations after migrating up again, want the one just seeded", n)
	}
}
