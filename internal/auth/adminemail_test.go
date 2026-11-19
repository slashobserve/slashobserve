package auth

import (
	"log/slog"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func adminEmailRig(t *testing.T) (*Service, *dbtest.Database) {
	t.Helper()
	return adminEmailRigLogging(t, slog.New(slog.DiscardHandler))
}

func adminEmailRigLogging(t *testing.T, logger *slog.Logger) (*Service, *dbtest.Database) {
	t.Helper()
	database := dbtest.New(t)
	policy := RolePolicy{AdminEmails: []string{"boss@example.com"}}
	s, err := New(Config{PublicURL: mustURL(t, "http://localhost:8080"), IssuerURL: "http://issuer.example/dex", RolePolicy: policy}, database.Queries(), logger)
	if err != nil {
		t.Fatal(err)
	}
	return s, database
}

func signIn(t *testing.T, s *Service, database *dbtest.Database, subject, email string, verified bool) dbsqlc.UpsertUserRow {
	t.Helper()
	user, err := database.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: subject, Email: email, Name: subject})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.assignRole(t.Context(), user, nil, verified); err != nil {
		t.Fatal(err)
	}
	return user
}

func roleOf(t *testing.T, database *dbtest.Database, id any) (role, source string) {
	t.Helper()
	if err := database.SQLDB(t).QueryRowContext(t.Context(), "SELECT role, role_source FROM users WHERE id = $1", id).Scan(&role, &source); err != nil {
		t.Fatal(err)
	}
	return role, source
}

func TestAnUnverifiedAddressOnTheAdminListDoesNotSignInAsAdmin(t *testing.T) {
	s, database := adminEmailRig(t)
	first := signIn(t, s, database, "other", "other@example.com", true)
	claimant := signIn(t, s, database, "claimant", "boss@example.com", false)

	if role, _ := roleOf(t, database, claimant.ID); role == "admin" {
		t.Fatal("a sign-in with an unverified address on the admin list became an Admin")
	}
	if role, _ := roleOf(t, database, first.ID); role == "admin" {
		t.Fatal("an address that is not on the admin list became an Admin")
	}
}

func TestAVerifiedAddressOnTheAdminListSignsInAsAdminAndKeepsItAcrossARestart(t *testing.T) {
	s, database := adminEmailRig(t)
	boss := signIn(t, s, database, "boss", "boss@example.com", true)
	if role, source := roleOf(t, database, boss.ID); role != "admin" || source != "admin_emails" {
		t.Fatalf("role = %s from %s, want admin from admin_emails", role, source)
	}

	if err := s.ReconcileRoles(t.Context()); err != nil {
		t.Fatal(err)
	}

	if role, _ := roleOf(t, database, boss.ID); role != "admin" {
		t.Fatalf("role after reconcile = %s, want admin", role)
	}
}

func TestReconcileDoesNotGrantAdminByEmailToSomeoneWhoHasNotSignedInVerified(t *testing.T) {
	s, database := adminEmailRig(t)
	claimant := signIn(t, s, database, "claimant", "boss@example.com", false)

	if err := s.ReconcileRoles(t.Context()); err != nil {
		t.Fatal(err)
	}

	if role, _ := roleOf(t, database, claimant.ID); role == "admin" {
		t.Fatal("reconcile granted Admin by email to a person who never signed in with a verified address")
	}
}

func TestAnUnverifiedSignInRemovesAdminThatCameFromTheAddress(t *testing.T) {
	s, database := adminEmailRig(t)
	boss := signIn(t, s, database, "boss", "boss@example.com", true)
	signIn(t, s, database, "boss", "boss@example.com", false)

	if role, source := roleOf(t, database, boss.ID); role == "admin" || source == "admin_emails" {
		t.Fatalf("role = %s from %s after an unverified sign-in, want no Admin from the address", role, source)
	}
}

func TestAnUnverifiedSignInWithAListedAddressWarnsWithoutTheAddress(t *testing.T) {
	logger, logs := logtest.New()
	s, database := adminEmailRigLogging(t, logger)
	signIn(t, s, database, "other", "other@example.com", false)
	claimant := signIn(t, s, database, "claimant", "boss@example.com", false)

	record := logs.Find(t, "admin email signed in unverified")
	if record["level"] != "WARN" || record["user_id"] != claimant.ID.String() {
		t.Fatalf("warning = %v, want WARN with the user id", record)
	}
	logs.AssertAbsent(t, "boss@example.com")
}
