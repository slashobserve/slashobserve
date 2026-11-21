package db_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	threadAgentVersion = 76
	chatsVersion       = 77

	convDashboard = "00000000-0000-4000-8000-0000000009a1"
)

func convID(n int) string {
	return foID(0x9000 + n)
}

func convUser(t *testing.T, sqlDB *sql.DB, name string) string {
	t.Helper()
	return foScalar(t, sqlDB, `INSERT INTO users (oidc_issuer, oidc_subject, email, name) VALUES ('https://idp', gen_random_uuid()::text, gen_random_uuid()::text || '@example.com', $1) RETURNING id::text`, name)
}

func convRequest(t *testing.T, d *dbtest.Database, id, source, user, dashboard, chat, assignedKind, assignedID string) {
	t.Helper()
	d.Exec(t, `INSERT INTO agent_requests (id, prompt, requested_by_kind, requested_by_id, requested_by_name, source, dashboard_id, base_version, chat_id, assigned_kind, assigned_id, created_at)
VALUES ($1::uuid, 'p', 'user', $2::uuid, 'Ada', $3, nullif($4, '')::uuid, CASE WHEN $4 = '' THEN NULL ELSE 1 END, nullif($5, '')::uuid, $6, nullif($7, '')::uuid, clock_timestamp())`,
		id, user, source, dashboard, chat, assignedKind, assignedID)
}

func convReply(t *testing.T, sqlDB *sql.DB, thread, run, kind string) {
	t.Helper()
	foScalar(t, sqlDB, `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name, run_id, run_assigned_kind, created_at)
VALUES ($1::uuid, 'done', 'user', gen_random_uuid(), 'Ada', $2::uuid, $3, clock_timestamp()) RETURNING id::text`, thread, run, kind)
}

func convThread(t *testing.T, sqlDB *sql.DB) string {
	t.Helper()
	return foScalar(t, sqlDB, `INSERT INTO threads (dashboard_id, dashboard_version) VALUES ($1::uuid, 1) RETURNING id::text`, convDashboard)
}

func TestThreadAgentIsTheLastLiveAgentThatReplied(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, threadAgentVersion-1)
	sqlDB := d.SQLDB(t)
	user := convUser(t, sqlDB, "Ada")
	seedDashboard(t, d, convDashboard, 1)
	seedVersion(t, d, convDashboard, 1, "{}")
	d.Exec(t, `INSERT INTO oauth_clients (id, redirect_uris) VALUES ('c', ARRAY['http://localhost/cb'])`)
	live := foScalar(t, sqlDB, `INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) VALUES ('c', $1::uuid, 'http://localhost/mcp', now() + interval '1 day') RETURNING id::text`, user)
	revoked := foScalar(t, sqlDB, `INSERT INTO oauth_grants (client_id, user_id, resource, expires_at, revoked_at, revoked_reason) VALUES ('c', $1::uuid, 'http://localhost/mcp', now() + interval '1 day', now(), 'person') RETURNING id::text`, user)

	builtinThread, grantThread, revokedThread, quietThread := convThread(t, sqlDB), convThread(t, sqlDB), convThread(t, sqlDB), convThread(t, sqlDB)
	convRequest(t, d, convID(1), "comment", user, convDashboard, "", "grant", revoked)
	convReply(t, sqlDB, builtinThread, convID(1), "grant")
	convRequest(t, d, convID(2), "comment", user, convDashboard, "", "builtin", "")
	convReply(t, sqlDB, builtinThread, convID(2), "builtin")
	convRequest(t, d, convID(3), "comment", user, convDashboard, "", "grant", live)
	convReply(t, sqlDB, grantThread, convID(3), "grant")
	convRequest(t, d, convID(4), "comment", user, convDashboard, "", "grant", revoked)
	convReply(t, sqlDB, revokedThread, convID(4), "grant")

	if err := d.MigrateUpTo(ctx, threadAgentVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	agentOf := func(thread string) string {
		return foScalar(t, sqlDB, `SELECT coalesce(agent_kind, 'none') || ':' || coalesce(agent_id::text, '') || ':' || agent_off::text FROM threads WHERE id = $1::uuid`, thread)
	}
	for _, c := range []struct{ name, thread, want string }{
		{"the newest reply was the built-in agent", builtinThread, "builtin::false"},
		{"a live grant replied", grantThread, "grant:" + live + ":false"},
		{"only a revoked grant replied", revokedThread, "none::false"},
		{"no agent replied", quietThread, "none::false"},
	} {
		if got := agentOf(c.thread); got != c.want {
			t.Errorf("%s: agent = %q, want %q", c.name, got, c.want)
		}
	}
	_, err := sqlDB.Exec(`UPDATE threads SET agent_kind = 'grant' WHERE id = $1::uuid`, quietThread)
	if constraintOf(err) != "threads_agent_check" {
		t.Errorf("a grant agent without an id: error = %v, want a violation of threads_agent_check", err)
	}

	if err := d.MigrateDownTo(ctx, threadAgentVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if err := d.MigrateUpTo(ctx, threadAgentVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
}

func TestAnObservationHoldsOnePageThreadAndManyAnchoredOnes(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, threadAgentVersion)
	f := seedObservation(t, d, "")
	sqlDB := d.SQLDB(t)
	page := foScalar(t, sqlDB, `INSERT INTO threads (observation_id) VALUES ($1::uuid) RETURNING id::text`, f.observation)
	for i := range 2 {
		if _, err := sqlDB.Exec(`INSERT INTO threads (observation_id, anchor) VALUES ($1::uuid, '{"type":"time"}')`, f.observation); err != nil {
			t.Fatalf("anchored thread %d: %v", i, err)
		}
	}
	if _, err := sqlDB.Exec(`INSERT INTO threads (observation_id, anchor, block_id) VALUES ($1::uuid, '{"type":"time"}', $2::uuid)`, f.observation, f.block); err != nil {
		t.Fatalf("block thread: %v", err)
	}
	cases := []struct {
		name, statement, want string
		args                  []any
	}{
		{"a second page thread", `INSERT INTO threads (observation_id) VALUES ($1::uuid)`, "threads_observation_page_idx", []any{f.observation}},
		{"a block without an anchor", `INSERT INTO threads (observation_id, block_id) VALUES ($1::uuid, $2::uuid)`, "threads_block_point_check", []any{f.observation, f.block}},
		{"an anchor that is not an object", `INSERT INTO threads (observation_id, anchor) VALUES ($1::uuid, '[1]')`, "threads_point_check", []any{f.observation}},
	}
	for _, c := range cases {
		_, err := sqlDB.Exec(c.statement, c.args...)
		if got := constraintOf(err); got != c.want {
			t.Errorf("%s: error = %v, want violation %q", c.name, err, c.want)
		}
	}

	if err := d.MigrateDownTo(ctx, threadAgentVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := foScalar(t, sqlDB, `SELECT string_agg(id::text, ',') FROM threads WHERE observation_id = $1::uuid`, f.observation); got != page {
		t.Errorf("threads on the observation after down = %q, want only the page thread %q", got, page)
	}
}

func TestMessageClientIDIsUniquePerAuthor(t *testing.T) {
	d := newMigratedTo(t, threadAgentVersion)
	sqlDB := d.SQLDB(t)
	seedDashboard(t, d, convDashboard, 1)
	seedVersion(t, d, convDashboard, 1, "{}")
	thread := convThread(t, sqlDB)
	author := convUser(t, sqlDB, "Ada")
	insert := `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name, client_id)
VALUES ($1::uuid, 'hello', 'user', $2::uuid, 'Ada', '00000000-0000-4000-8000-00000000c11e')`
	if _, err := sqlDB.Exec(insert, thread, author); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(insert, thread, author); constraintOf(err) != "thread_messages_client_idx" {
		t.Errorf("a second message with one client id: error = %v, want a violation of thread_messages_client_idx", err)
	}
	if _, err := sqlDB.Exec(insert, thread, convUser(t, sqlDB, "Grace")); err != nil {
		t.Errorf("another author with the same client id was refused: %v", err)
	}
}

func TestChatsAreBackfilledFromTheirTurns(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, chatsVersion-1)
	sqlDB := d.SQLDB(t)
	ada, grace := convUser(t, sqlDB, "Ada"), convUser(t, sqlDB, "Grace")
	seedDashboard(t, d, convDashboard, 1)
	seedVersion(t, d, convDashboard, 1, "{}")

	observeRoot, observeTurn, observeAlone := convID(10), convID(11), convID(12)
	promptFirst, promptSecond, promptOther := convID(13), convID(14), convID(15)
	comment, investigation := convID(16), convID(17)
	chainSecond, chainThird, chainFourth := convID(18), convID(19), convID(20)
	convRequest(t, d, observeRoot, "observe", ada, "", "", "builtin", "")
	d.Exec(t, `UPDATE agent_requests SET title = 'Checkout', pinned_at = now() WHERE id = $1::uuid`, observeRoot)
	convRequest(t, d, observeTurn, "observe", ada, "", observeRoot, "builtin", "")
	convRequest(t, d, chainSecond, "observe", ada, "", observeTurn, "builtin", "")
	convRequest(t, d, chainThird, "observe", ada, "", chainSecond, "builtin", "")
	convRequest(t, d, chainFourth, "observe", ada, "", chainThird, "builtin", "")
	convRequest(t, d, observeAlone, "observe", ada, "", "", "builtin", "")
	convRequest(t, d, promptFirst, "prompt", ada, convDashboard, "", "builtin", "")
	convRequest(t, d, promptSecond, "prompt", ada, convDashboard, "", "builtin", "")
	convRequest(t, d, promptOther, "prompt", grace, convDashboard, "", "builtin", "")
	convRequest(t, d, comment, "comment", ada, convDashboard, "", "builtin", "")
	convRequest(t, d, investigation, "investigate", ada, "", "", "builtin", "")

	if err := d.MigrateUpTo(ctx, chatsVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	chatOf := func(id string) string {
		return foScalar(t, sqlDB, `SELECT coalesce(chat_id::text, 'none') FROM agent_requests WHERE id = $1::uuid`, id)
	}
	for _, c := range []struct{ name, id, want string }{
		{"an observe root", observeRoot, observeRoot},
		{"an observe follow-up", observeTurn, observeRoot},
		{"a turn that followed a follow-up", chainSecond, observeRoot},
		{"a turn three follow-ups deep", chainThird, observeRoot},
		{"a turn four follow-ups deep", chainFourth, observeRoot},
		{"a lone observe run", observeAlone, observeAlone},
		{"the first dashboard prompt", promptFirst, promptFirst},
		{"a later dashboard prompt of the same person", promptSecond, promptFirst},
		{"another person's dashboard prompt", promptOther, promptOther},
		{"a comment run", comment, "none"},
		{"a person's investigation", investigation, investigation},
	} {
		if got := chatOf(c.id); got != c.want {
			t.Errorf("%s: chat_id = %s, want %s", c.name, got, c.want)
		}
	}
	if got := foScalar(t, sqlDB, `SELECT count(*)::text FROM chats`); got != "5" {
		t.Errorf("chats = %s, want 5", got)
	}
	root := foScalar(t, sqlDB, `SELECT owner_kind || ':' || owner_id::text || ':' || coalesce(title, '') || ':' || (pinned_at IS NOT NULL)::text || ':' || coalesce(dashboard_id::text, 'none') FROM chats WHERE id = $1::uuid`, observeRoot)
	if want := "user:" + ada + ":Checkout:true:none"; root != want {
		t.Errorf("observe chat = %q, want %q", root, want)
	}
	if got := foScalar(t, sqlDB, `SELECT owner_id::text || ':' || coalesce(dashboard_id::text, 'none') FROM chats WHERE id = $1::uuid`, promptFirst); got != ada+":"+convDashboard {
		t.Errorf("dashboard chat = %q, want %q", got, ada+":"+convDashboard)
	}
	if got := foScalar(t, sqlDB, `SELECT (last_turn_at = (SELECT created_at FROM agent_requests WHERE id = $2::uuid))::text FROM chats WHERE id = $1::uuid`, observeRoot, chainFourth); got != "true" {
		t.Errorf("the chain's last turn is not its deepest follow-up")
	}
	if got := chatsSchemaState(t, sqlDB); got != "chats:true,true,true" {
		t.Errorf("after up, foreign key and indexes = %q, want them pointing at chats and valid", got)
	}

	backfilled := chatsBackfillState(t, sqlDB)
	runChatsMigrationHalf(t, sqlDB, "Up")
	if got := chatsBackfillState(t, sqlDB); got != backfilled {
		t.Errorf("running the up statements again changed the rows:\n%s\nwant\n%s", got, backfilled)
	}
	if got := chatsSchemaState(t, sqlDB); got != "chats:true,true,true" {
		t.Errorf("after a second up, foreign key and indexes = %q, want them pointing at chats and valid", got)
	}

	clientInsert := `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, client_id)
VALUES ('p', 'user', $1::uuid, 'Ada', 'observe', '00000000-0000-4000-8000-00000000c11e')`
	if _, err := sqlDB.Exec(clientInsert, ada); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(clientInsert, ada); constraintOf(err) != "agent_requests_client_idx" {
		t.Errorf("a second run with one client id: error = %v, want a violation of agent_requests_client_idx", err)
	}
	d.Exec(t, `DELETE FROM chats WHERE id = $1::uuid`, observeAlone)
	if got := chatOf(observeAlone); got != "none" {
		t.Errorf("chat_id after its chat was deleted = %s, want none", got)
	}

	if err := d.MigrateDownTo(ctx, chatsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	runChatsMigrationHalf(t, sqlDB, "Down")
	for _, c := range []struct{ name, id, want string }{
		{"an observe root", observeRoot, "none"},
		{"an observe follow-up", observeTurn, observeRoot},
		{"a turn four follow-ups deep", chainFourth, observeRoot},
		{"a later dashboard prompt", promptSecond, "none"},
	} {
		if got := chatOf(c.id); got != c.want {
			t.Errorf("after down, %s: chat_id = %s, want %s", c.name, got, c.want)
		}
	}
	if got := foScalar(t, sqlDB, `SELECT confrelid::regclass::text || ':' || convalidated::text FROM pg_constraint WHERE conname = 'agent_requests_chat_id_fkey'`); got != "agent_requests:true" {
		t.Errorf("after down, chat_id foreign key = %q, want a valid one to agent_requests", got)
	}

	if err := d.MigrateUpTo(ctx, chatsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if got := chatOf(chainFourth); got != observeRoot {
		t.Errorf("after up again, the deepest turn's chat_id = %s, want %s", got, observeRoot)
	}
}

func chatsBackfillState(t *testing.T, sqlDB *sql.DB) string {
	t.Helper()
	return foScalar(t, sqlDB, `SELECT coalesce((SELECT string_agg(id::text || '=' || coalesce(chat_id::text, '-'), ',' ORDER BY id) FROM agent_requests), '') || E'\n' || coalesce((SELECT string_agg(c::text, ',' ORDER BY c.id) FROM chats c), '')`)
}

func chatsSchemaState(t *testing.T, sqlDB *sql.DB) string {
	t.Helper()
	return foScalar(t, sqlDB, `SELECT (SELECT confrelid::regclass::text || ':' || convalidated::text FROM pg_constraint WHERE conname = 'agent_requests_chat_id_fkey')
  || ',' || coalesce((SELECT indisvalid::text FROM pg_index WHERE indexrelid = 'agent_requests_client_idx'::regclass), 'missing')
  || ',' || coalesce((SELECT indisvalid::text FROM pg_index WHERE indexrelid = 'agent_requests_chat_order_idx'::regclass), 'missing')`)
}

const chatsMigrationFile = "migrations/00077_chats.sql"

func runChatsMigrationHalf(t *testing.T, sqlDB *sql.DB, half string) {
	t.Helper()
	body, err := os.ReadFile(chatsMigrationFile)
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(body), "-- +goose "+half+"\n")
	if !ok {
		t.Fatalf("%s has no %s section", chatsMigrationFile, half)
	}
	rest, _, _ = strings.Cut(rest, "-- +goose ")
	for statement := range strings.SplitSeq(rest, ";\n") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("%s again: %v\n%s", half, err, statement)
		}
	}
}
