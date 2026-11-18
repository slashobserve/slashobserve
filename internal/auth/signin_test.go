package auth

import (
	"log/slog"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func newSignInService(t *testing.T) (*Service, *dbtest.Database) {
	t.Helper()
	database := dbtest.New(t)
	s, err := New(Config{PublicURL: mustURL(t, "http://localhost:8080"), IssuerURL: "http://issuer.example/dex", SessionTTL: time.Hour}, database.Queries(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return s, database
}

func TestADeactivatedPersonCannotSignIn(t *testing.T) {
	s, database := newSignInService(t)
	who := identity{Issuer: "i", Subject: "bob", Email: "bob@example.com", Name: "Bob"}
	if _, failure := s.signIn(t.Context(), who); failure != nil {
		t.Fatalf("first sign-in failed: %+v", failure)
	}
	database.Exec(t, "UPDATE users SET deactivated_at = now(), last_login_at = now() - interval '1 day' WHERE email = 'bob@example.com'")

	token, failure := s.signIn(t.Context(), who)

	if failure == nil || failure.code != "account_deactivated" || token != "" {
		t.Fatalf("sign-in of a deactivated person = %q %+v, want account_deactivated", token, failure)
	}
	var sessions int
	var stale bool
	if err := database.SQLDB(t).QueryRowContext(t.Context(), "SELECT (SELECT count(*) FROM sessions), last_login_at < now() - interval '1 hour' FROM users WHERE email = 'bob@example.com'").Scan(&sessions, &stale); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 || !stale {
		t.Errorf("sessions = %d, last login kept = %v; want only the first session and the old last login", sessions, stale)
	}
}
