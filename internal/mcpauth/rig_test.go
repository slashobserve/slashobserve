package mcpauth

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const (
	verifier    = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge   = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	callback    = "http://127.0.0.1:33418/callback"
	secretState = "state-4f1c9a"
)

var (
	ada  = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Email: "ada@example.com", Role: auth.RoleViewer}
	bob  = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Bob", Email: "bob@example.com", Role: auth.RoleViewer}
	root = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Root", Email: "root@example.com", Role: auth.RoleAdmin}
	bot  = auth.Principal{Kind: auth.KindServiceAccount, ID: "33333333-3333-4333-8333-333333333333", Name: "ci", Role: auth.RoleAdmin}
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type rig struct {
	db     *dbtest.Database
	server *Server
	people *auth.Service
	engine *gin.Engine
	logs   *syncBuffer
}

func newRig(t *testing.T) *rig {
	t.Helper()
	d := dbtest.New(t)
	d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1, 'https://idp', 'ada', 'ada@example.com', 'Ada'), ($2, 'https://idp', 'bob', 'bob@example.com', 'Bob'), ($3, 'https://idp', 'root', 'root@example.com', 'Root')`, ada.ID, bob.ID, root.ID)
	base, err := url.Parse(testPublicURL)
	if err != nil {
		t.Fatal(err)
	}
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	people, err := auth.New(auth.Config{IssuerURL: "https://idp", PublicURL: base}, d.Queries(), logger)
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{PublicURL: base, Logger: logger}, d.DB, people)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	principals := map[string]auth.Principal{ada.ID: ada, bob.ID: bob, root.ID: root, bot.ID: bot}
	api := engine.Group("/api/v1", func(c *gin.Context) {
		if p, ok := principals[c.GetHeader("X-Test-Principal")]; ok {
			c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
		}
	})
	server.Register(auth.NewRoutes(api))
	return &rig{db: d, server: server, people: people, engine: engine, logs: logs}
}

func (r *rig) oauth(method, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	r.server.ServeHTTP(rec, req)
	return rec
}

func (r *rig) api(by auth.Principal, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1"+path, nil)
	req.Header.Set("X-Test-Principal", by.ID)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

func (r *rig) apiJSON(by auth.Principal, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	req.Header.Set("X-Test-Principal", by.ID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

func (r *rig) registerClient(t *testing.T, name string, redirectURIs ...string) string {
	t.Helper()
	if len(redirectURIs) == 0 {
		redirectURIs = []string{callback}
	}
	body, err := json.Marshal(map[string]any{"client_name": name, "redirect_uris": redirectURIs})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ClientID == "" {
		t.Fatalf("register body %s: %v", rec.Body.String(), err)
	}
	return out.ClientID
}

func authorizeQuery(clientID, redirect string) url.Values {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {secretState},
		"resource":              {testPublicURL + "/mcp"},
	}
}

func (r *rig) authorize(query url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	r.server.ServeHTTP(rec, req)
	return rec
}

func (r *rig) pendingRequest(t *testing.T, clientID, redirect string) string {
	t.Helper()
	rec := r.authorize(authorizeQuery(clientID, redirect))
	location := rec.Header().Get("Location")
	id, ok := strings.CutPrefix(location, "/connect/")
	if rec.Code != http.StatusFound || !ok {
		t.Fatalf("authorize = %d to %q: %s", rec.Code, location, rec.Body.String())
	}
	return id
}

func redirectQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func (r *rig) approve(t *testing.T, by auth.Principal, requestID string) url.Values {
	t.Helper()
	rec := r.api(by, http.MethodPost, "/oauth/requests/"+requestID+"/approve")
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		RedirectTo string `json:"redirect_to"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return redirectQuery(t, out.RedirectTo)
}

func (r *rig) code(t *testing.T, by auth.Principal, clientID, redirect string) string {
	t.Helper()
	q := r.approve(t, by, r.pendingRequest(t, clientID, redirect))
	if q.Get("state") != secretState || q.Get("iss") != testPublicURL || q.Get("code") == "" {
		t.Fatalf("approval redirect carries %v", q)
	}
	return q.Get("code")
}

func exchangeForm(clientID, code string) url.Values {
	return url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"redirect_uri":  {callback},
		"code_verifier": {verifier},
		"resource":      {testPublicURL + "/mcp"},
	}
}

func refreshForm(clientID, refresh string) url.Values {
	return url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}}
}

type tokens struct {
	Access  string `json:"access_token"`
	Type    string `json:"token_type"`
	Expires int    `json:"expires_in"`
	Refresh string `json:"refresh_token"`
}

func (r *rig) tokens(t *testing.T, form url.Values) tokens {
	t.Helper()
	rec := r.oauth(http.MethodPost, "/oauth/token", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("token = %d: %s", rec.Code, rec.Body.String())
	}
	var out tokens
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rig) connect(t *testing.T, by auth.Principal, clientID string) tokens {
	t.Helper()
	return r.tokens(t, exchangeForm(clientID, r.code(t, by, clientID, callback)))
}

func (r *rig) tokenError(t *testing.T, form url.Values, status int, code string) {
	t.Helper()
	rec := r.oauth(http.MethodPost, "/oauth/token", form)
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("token error body %q: %v", rec.Body.String(), err)
	}
	if rec.Code != status || body["error"] != code || body["error_description"] == "" {
		t.Fatalf("token = %d %v, want %d %s", rec.Code, body, status, code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("token error headers %v lack no-store", rec.Header())
	}
}

func (r *rig) authenticate(t *testing.T, access string) (auth.Principal, string, bool) {
	t.Helper()
	got, ok, err := r.server.Authenticate(t.Context(), "Bearer "+access)
	if err != nil {
		t.Fatal(err)
	}
	return got.Principal, got.Client, ok
}

func (r *rig) revokedReason(t *testing.T, userID, clientID string) string {
	t.Helper()
	var reason *string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT revoked_reason FROM oauth_grants WHERE user_id = $1 AND client_id = $2 ORDER BY created_at DESC LIMIT 1`, userID, clientID).Scan(&reason)
	if err != nil {
		t.Fatal(err)
	}
	if reason == nil {
		return ""
	}
	return *reason
}

func hashOf(token string) []byte { return opaquetoken.Hash(token) }
