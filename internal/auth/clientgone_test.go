package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func lockTable(t *testing.T, r *sessionRig, table string) *sql.DB {
	t.Helper()
	sqlDB := r.db.SQLDB(t)
	tx, err := sqlDB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	if _, err := tx.ExecContext(t.Context(), "LOCK TABLE "+table+" IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	return sqlDB
}

func waitForBlockedQuery(t *testing.T, sqlDB *sql.DB, table string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var blocked int
		err := sqlDB.QueryRowContext(t.Context(),
			"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE '%FROM ' || $1 || '%'", table).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no query is waiting on a lock for %s", table)
}

func assertNoServerError(t *testing.T, logs *logtest.Capture, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != clientgone.Status {
		t.Errorf("status %d, want %d", rec.Code, clientgone.Status)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body %q, want none", rec.Body.String())
	}
	for _, record := range logs.Records(t) {
		if record["level"] == "ERROR" {
			t.Errorf("logged an ERROR for a client that went away: %v", record)
		}
	}
}

func TestClientGoingAwayDuringAnAuthDatabaseCallIsNotAServerError(t *testing.T) {
	bearer := "Bearer " + keyPrefix + strings.Repeat("a", keyLength-len(keyPrefix))
	cases := []struct {
		name   string
		method string
		path   string
		table  string
		header string
		cookie func(r *sessionRig) string
	}{
		{"session endpoint canceled", http.MethodGet, "/api/v1/auth/session", "sessions", "", func(r *sessionRig) string { return r.cookie }},
		{"session middleware canceled", http.MethodGet, "/api/v1/ping", "sessions", "", func(r *sessionRig) string { return r.cookie }},
		{"bearer middleware canceled", http.MethodGet, "/api/v1/ping", "service_account_keys", bearer, nil},
		{"service account list canceled", http.MethodGet, "/api/v1/service-accounts", "service_accounts", "", func(r *sessionRig) string { return r.cookie }},
		{"logout canceled", http.MethodPost, "/api/v1/auth/logout", "sessions", "", func(r *sessionRig) string { return r.cookie }},
		{"sign-in callback canceled", http.MethodGet, "/api/v1/auth/callback?state=abc", "oidc_login_states", "", func(r *sessionRig) string { return r.login + "=abc" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger, logs := logtest.New()
			r := newSessionRigWithLogger(t, logger)
			sqlDB := lockTable(t, r, tc.table)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			req := httptest.NewRequestWithContext(ctx, tc.method, tc.path, nil)
			if tc.cookie != nil {
				req.Header.Set("Cookie", tc.cookie(r))
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				r.engine.ServeHTTP(rec, req)
			}()
			waitForBlockedQuery(t, sqlDB, tc.table)
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("the handler did not return after the request was canceled")
			}
			assertNoServerError(t, logs, rec)
		})
	}
}

func TestClientGoingAwayWhileStoringTheLoginStateIsNotAServerError(t *testing.T) {
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 base,
			"authorization_endpoint": base + "/auth",
			"token_endpoint":         base + "/token",
			"jwks_uri":               base + "/keys",
		})
	}))
	t.Cleanup(issuer.Close)
	logger, logs := logtest.New()
	r := newSessionRigWithIssuer(t, logger, issuer.URL)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/auth/login", nil))
	assertNoServerError(t, logs, rec)
}
