package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const fakeClientID = "slashobserve"

type fakeIssuer struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	nonce  string
	email  string
}

func newFakeIssuer(t *testing.T, email string) *fakeIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIssuer{key: key, email: email}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		base := f.server.URL
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                base,
			"authorization_endpoint":                base + "/auth",
			"token_endpoint":                        base + "/token",
			"jwks_uri":                              base + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "k1", "use": "sig", "alg": "RS256",
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		claims := map[string]any{
			"iss": f.server.URL, "sub": "fake-subject", "aud": fakeClientID, "nonce": f.nonce,
			"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
			"email": f.email, "email_verified": true, "name": "Ada",
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "token_type": "Bearer", "expires_in": 3600, "id_token": f.sign(t, claims)})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeIssuer) sign(t *testing.T, claims map[string]any) string {
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Error(err)
	}
	signed := b64(header) + "." + b64(payload)
	digest := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Error(err)
	}
	return signed + "." + b64(sig)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

type callbackRig struct {
	db       *dbtest.Database
	engine   *gin.Engine
	recorder *audit.Recorder
	issuer   *fakeIssuer
	service  *Service
}

func newCallbackRig(t *testing.T, email string, domains []string) callbackRig {
	t.Helper()
	database := dbtest.New(t)
	issuer := newFakeIssuer(t, email)
	s, err := New(Config{
		PublicURL: mustURL(t, "http://localhost:8080"), IssuerURL: issuer.server.URL, ClientID: fakeClientID, ClientSecret: "secret",
		SessionTTL: time.Hour, AllowedEmailDomains: domains, AllowAnyEmailDomain: len(domains) == 0,
	}, database.Queries(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	recorder := audit.New(audit.Config{DB: database.DB})
	mw, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	s.Mount(engine.Group("/api/v1", mw))
	return callbackRig{db: database, engine: engine, recorder: recorder, issuer: issuer, service: s}
}

func (r callbackRig) signIn(t *testing.T) (*httptest.ResponseRecorder, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || rec.Code != http.StatusFound || !strings.HasPrefix(location.String(), r.issuer.server.URL) {
		t.Fatalf("login answered %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	r.issuer.mu.Lock()
	r.issuer.nonce = location.Query().Get("nonce")
	r.issuer.mu.Unlock()
	state := location.Query().Get("state")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=c&state="+url.QueryEscape(state), nil)
	req.Header.Set("Cookie", r.service.loginCookie+"="+state)
	rec = httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec, rec.Header().Get(audit.RequestIDHeader)
}

type signInEvent struct {
	action, outcome, reason, actorKind, actorID, authMethod, credentialID, targetKind, targetID, data string
}

func (r callbackRig) eventsOf(t *testing.T, request string) []signInEvent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.recorder.Run(ctx)
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT action, outcome, coalesce(reason, ''), actor_kind, coalesce(actor_id::text, ''), coalesce(auth_method, ''),
  coalesce(credential_id, ''), coalesce(target_kind, ''), coalesce(target_id, ''), data::text
FROM audit_events WHERE request_id = $1`, request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events []signInEvent
	for rows.Next() {
		var e signInEvent
		if err := rows.Scan(&e.action, &e.outcome, &e.reason, &e.actorKind, &e.actorID, &e.authMethod, &e.credentialID, &e.targetKind, &e.targetID, &e.data); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func (r callbackRig) userID(t *testing.T, email string) string {
	t.Helper()
	var id string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestASignInRecordsTheSignedInPersonAndTheirSession(t *testing.T) {
	r := newCallbackRig(t, "ada@example.com", nil)
	rec, request := r.signIn(t)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("callback answered %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	var session string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM sessions`).Scan(&session); err != nil {
		t.Fatal(err)
	}
	want := signInEvent{action: "auth.login", outcome: "success", actorKind: "user", actorID: r.userID(t, "ada@example.com"), authMethod: "session", credentialID: session, targetKind: "session", targetID: session, data: "{}"}
	if got := r.eventsOf(t, request); len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestAForgedSignInStateIsRecordedAsARefusedSignIn(t *testing.T) {
	r := newCallbackRig(t, "ada@example.com", nil)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=c&state=forged", nil))
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "error=state_mismatch") {
		t.Fatalf("callback answered %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	want := signInEvent{action: "auth.login_failed", outcome: "denied", reason: "state_mismatch", actorKind: "anonymous", authMethod: "none", data: "{}"}
	if got := r.eventsOf(t, rec.Header().Get(audit.RequestIDHeader)); len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestADeactivatedPersonsSignInIsRecordedAsRefused(t *testing.T) {
	r := newCallbackRig(t, "ada@example.com", nil)
	if rec, _ := r.signIn(t); rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("first sign-in answered %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	id := r.userID(t, "ada@example.com")
	r.db.Exec(t, `UPDATE users SET deactivated_at = now() WHERE id = $1`, id)
	rec, request := r.signIn(t)
	if !strings.Contains(rec.Header().Get("Location"), "error=account_deactivated") {
		t.Fatalf("callback answered %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	want := signInEvent{action: "auth.login_failed", outcome: "denied", reason: "account_deactivated", actorKind: "anonymous", authMethod: "none", targetKind: "user", targetID: id, data: "{}"}
	if got := r.eventsOf(t, request); len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestADisallowedEmailIsRecordedWithoutTheEmail(t *testing.T) {
	r := newCallbackRig(t, "mallory@elsewhere.example", []string{"example.com"})
	rec, request := r.signIn(t)
	if !strings.Contains(rec.Header().Get("Location"), "error=email_not_allowed") {
		t.Fatalf("callback answered %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	got := r.eventsOf(t, request)
	want := signInEvent{action: "auth.login_failed", outcome: "denied", reason: "email_not_allowed", actorKind: "anonymous", authMethod: "none", data: "{}"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
	var mentions int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE row_to_json(audit_events)::text LIKE '%mallory%'`).Scan(&mentions); err != nil {
		t.Fatal(err)
	}
	if mentions != 0 {
		t.Fatal("an audit event carries the refused email")
	}
}
