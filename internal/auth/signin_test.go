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

func addInvite(t *testing.T, database *dbtest.Database, email, role string) {
	t.Helper()
	database.Exec(t, "INSERT INTO user_invites (email, role, created_by_kind, created_by_id, created_by_name) VALUES ($1, $2, 'user', gen_random_uuid(), 'Ada')", email, role)
}

func roleOf(t *testing.T, database *dbtest.Database, email string) (role, source, by string) {
	t.Helper()
	var setBy *string
	if err := database.SQLDB(t).QueryRowContext(t.Context(), "SELECT role, role_source, manual_role_by_name FROM users WHERE email = $1", email).Scan(&role, &source, &setBy); err != nil {
		t.Fatal(err)
	}
	if setBy != nil {
		by = *setBy
	}
	return role, source, by
}

func countInvites(t *testing.T, database *dbtest.Database) int {
	t.Helper()
	var n int
	if err := database.SQLDB(t).QueryRowContext(t.Context(), "SELECT count(*) FROM user_invites").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAVerifiedFirstSignInTakesTheInvitedRole(t *testing.T) {
	s, database := newSignInService(t)
	if _, failure := s.signIn(t.Context(), identity{Issuer: "i", Subject: "first", Email: "first@example.com"}); failure != nil {
		t.Fatal(failure)
	}
	addInvite(t, database, "New@Example.com", "viewer")

	if _, failure := s.signIn(t.Context(), identity{Issuer: "i", Subject: "new", Email: "new@example.com", EmailVerified: true}); failure != nil {
		t.Fatal(failure)
	}

	if role, source, by := roleOf(t, database, "new@example.com"); role != "viewer" || source != "manual" || by != "Ada" {
		t.Errorf("role = %s from %s by %q, want viewer chosen by Ada", role, source, by)
	}
	if n := countInvites(t, database); n != 0 {
		t.Errorf("%d invites left, want the accepted one gone", n)
	}
}

func TestAnUnverifiedSignInLeavesTheInvitePending(t *testing.T) {
	s, database := newSignInService(t)
	if _, failure := s.signIn(t.Context(), identity{Issuer: "i", Subject: "first", Email: "first@example.com"}); failure != nil {
		t.Fatal(failure)
	}
	addInvite(t, database, "new@example.com", "viewer")

	if _, failure := s.signIn(t.Context(), identity{Issuer: "i", Subject: "new", Email: "new@example.com"}); failure != nil {
		t.Fatal(failure)
	}

	if role, source, _ := roleOf(t, database, "new@example.com"); role != "editor" || source != "default" {
		t.Errorf("role = %s from %s, want the default Editor", role, source)
	}
	if n := countInvites(t, database); n != 1 {
		t.Errorf("%d invites, want the invite still pending", n)
	}
}
