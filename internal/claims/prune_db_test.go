package claims

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestPruneDeletesExpiredTokensAndWorkersOfRevokedCredentials(t *testing.T) {
	f := newFixture(t)
	queued, live := f.claimed(t)
	expired := dbsqlc.InsertRunTokenParams{
		TokenHash: make([]byte, 32), AgentRequestID: queued.ID, Attempt: 1,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
	}
	expired.TokenHash[0] = 1
	if err := f.d.Queries().InsertRunToken(context.Background(), expired); err != nil {
		t.Fatal(err)
	}

	f.d.Exec(t, "INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1, 'issuer', 'subject', 'ada@example.com', 'Ada')", mustUUID(t, userID))
	f.d.Exec(t, "INSERT INTO oauth_clients (id, redirect_uris) VALUES ('client', ARRAY['http://localhost/cb'])")
	f.d.Exec(t, "INSERT INTO oauth_grants (id, client_id, user_id, resource, expires_at) VALUES ($1, 'client', $2, 'r', now() + interval '1 day')", mustUUID(t, grantID), mustUUID(t, userID))
	f.d.Exec(t, "INSERT INTO oauth_grants (id, client_id, user_id, resource, expires_at, revoked_at, revoked_reason) VALUES ($1, 'client', $2, 'r', now() + interval '1 day', now(), 'person')", mustUUID(t, otherGrantID), mustUUID(t, userID))
	account, err := f.d.Queries().CreateServiceAccount(context.Background(), dbsqlc.CreateServiceAccountParams{Name: "ci-agent", Role: "editor"})
	if err != nil {
		t.Fatal(err)
	}
	for id, principal := range map[string]struct {
		kind string
		id   pgtype.UUID
	}{
		"grant:" + grantID:                       {"grant", mustUUID(t, grantID)},
		"grant:" + otherGrantID:                  {"grant", mustUUID(t, otherGrantID)},
		"service_account:" + account.ID.String(): {"service_account", account.ID},
		"service_account:" + userID:              {"service_account", mustUUID(t, userID)},
	} {
		f.d.Exec(t, "INSERT INTO agent_workers (id, principal_kind, principal_id) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING", id, principal.kind, principal.id)
	}
	f.d.Exec(t, "INSERT INTO agent_workers (id, principal_kind) VALUES ('builtin', 'builtin')")

	if err := PruneJob(f.d.DB)(context.Background()); err != nil {
		t.Fatal(err)
	}

	if n := f.tokenCount(t, queued.ID); n != 1 {
		t.Fatalf("%d run tokens left, want only the live one", n)
	}
	if _, ok := f.authenticate(t, "Bearer "+live.Session.Token); !ok {
		t.Fatal("the live token was pruned")
	}
	remaining := map[string]bool{}
	rows, err := f.d.SQLDB(t).Query("SELECT id FROM agent_workers")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		remaining[id] = true
	}
	want := map[string]bool{"grant:" + grantID: true, "service_account:" + account.ID.String(): true, "builtin": true}
	if len(remaining) != len(want) {
		t.Fatalf("workers left = %v, want %v", remaining, want)
	}
	for id := range want {
		if !remaining[id] {
			t.Errorf("worker %s was pruned, want it kept (left: %v)", id, remaining)
		}
	}
}
