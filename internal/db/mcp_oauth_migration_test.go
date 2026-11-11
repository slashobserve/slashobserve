package db_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const oauthVersion = 31

var oauthTables = []string{"oauth_clients", "oauth_requests", "oauth_grants", "oauth_codes", "oauth_tokens"}

func TestMCPOAuthDown(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, oauthVersion); err != nil {
		t.Fatal(err)
	}
	if missing := missingOAuthTables(t, d); len(missing) > 0 {
		t.Fatalf("tables missing after the up migration: %v", missing)
	}

	if err := d.MigrateDownTo(ctx, oauthVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if missing := missingOAuthTables(t, d); len(missing) != len(oauthTables) {
		t.Fatalf("tables still exist after the down migration, missing only %v", missing)
	}

	if err := d.MigrateUpTo(ctx, oauthVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if missing := missingOAuthTables(t, d); len(missing) > 0 {
		t.Fatalf("tables missing after migrating up again: %v", missing)
	}
}

func TestMCPOAuthConstraints(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, oauthVersion); err != nil {
		t.Fatal(err)
	}
	sqlDB := d.SQLDB(t)
	var userID, grantID string
	if err := sqlDB.QueryRow(`INSERT INTO users (oidc_issuer, oidc_subject, email) VALUES ('i', 's', 'a@example.com') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO oauth_clients (id, redirect_uris) VALUES ('c', ARRAY['http://localhost/cb'])`); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRow(`INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) VALUES ('c', $1, 'r', now() + interval '1 day') RETURNING id`, userID).Scan(&grantID); err != nil {
		t.Fatal(err)
	}

	if _, err := sqlDB.Exec(`INSERT INTO oauth_tokens (token_hash, grant_id, kind, expires_at, rotated_at) VALUES (sha256('a'::bytea), $1, 'access', now(), now())`, grantID); err == nil {
		t.Fatal("an access token with rotated_at was accepted")
	}
	if _, err := sqlDB.Exec(`INSERT INTO oauth_tokens (token_hash, grant_id, kind, expires_at, rotated_at) VALUES (sha256('b'::bytea), $1, 'refresh', now(), now())`, grantID); err != nil {
		t.Fatalf("a rotated refresh token was refused: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO oauth_grants (client_id, user_id, resource, expires_at, revoked_at) VALUES ('c', $1, 'r', now(), now())`, userID); err == nil {
		t.Fatal("a revoked grant without a reason was accepted")
	}
	if _, err := sqlDB.Exec(`INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) VALUES ('c', $1, 'r', now())`, userID); err == nil {
		t.Fatal("a second live grant for one client and person was accepted")
	}
}

func missingOAuthTables(t *testing.T, d *dbtest.Database) []string {
	t.Helper()
	var missing []string
	for _, table := range oauthTables {
		var exists bool
		if err := d.SQLDB(t).QueryRow(`SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			missing = append(missing, table)
		}
	}
	return missing
}
