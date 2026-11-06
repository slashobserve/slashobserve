package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestPlanProvision(t *testing.T) {
	oldHash, newHash := hashToken("old"), hashToken("new")
	want := provisionTarget{Role: RoleEditor, Description: "Posts markers", KeyHash: newHash}
	cases := []struct {
		name    string
		current provisionedState
		want    provisionPlan
		refused bool
	}{
		{"no account yet", provisionedState{}, provisionPlan{Account: accountCreate, Key: keyAdd}, false},
		{"same account and key", provisionedState{Exists: true, Provisioned: true, Role: RoleEditor, Description: "Posts markers", KeyHash: newHash}, provisionPlan{Account: accountKeep, Key: keyKeep}, false},
		{"role changed in the file", provisionedState{Exists: true, Provisioned: true, Role: RoleViewer, Description: "Posts markers", KeyHash: newHash}, provisionPlan{Account: accountUpdate, Key: keyKeep}, false},
		{"description changed in the file", provisionedState{Exists: true, Provisioned: true, Role: RoleEditor, Description: "", KeyHash: newHash}, provisionPlan{Account: accountUpdate, Key: keyKeep}, false},
		{"key rotated in the file", provisionedState{Exists: true, Provisioned: true, Role: RoleEditor, Description: "Posts markers", KeyHash: oldHash}, provisionPlan{Account: accountKeep, Key: keyReplace}, false},
		{"file key revoked or expired", provisionedState{Exists: true, Provisioned: true, Role: RoleEditor, Description: "Posts markers"}, provisionPlan{Account: accountKeep, Key: keyAdd}, false},
		{"role and key both changed", provisionedState{Exists: true, Provisioned: true, Role: RoleAdmin, Description: "Posts markers", KeyHash: oldHash}, provisionPlan{Account: accountUpdate, Key: keyReplace}, false},
		{"account made in the web app", provisionedState{Exists: true, Role: RoleEditor, Description: "Posts markers", KeyHash: newHash}, provisionPlan{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := planProvision(tc.current, want)
			if tc.refused {
				if !errors.Is(err, errNotProvisioned) {
					t.Fatalf("got %+v, %v, want a refusal", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %+v, %v, want %+v", got, err, tc.want)
			}
		})
	}
}

type provisionRig struct {
	t       *testing.T
	db      *dbtest.Database
	service *Service
	engine  *gin.Engine
	cookie  string
	logs    *bytes.Buffer
}

func newProvisionRig(t *testing.T) *provisionRig {
	t.Helper()
	database := dbtest.New(t)
	logs := &bytes.Buffer{}
	s, err := New(Config{PublicURL: mustURL(t, "http://localhost:8080"), IssuerURL: "http://issuer.example/dex"}, database.Queries(), slog.New(slog.NewJSONHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	user, err := database.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "s", Email: "ada@example.com", Name: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	token, hash, err := newToken("sos_")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Queries().CreateSession(t.Context(), dbsqlc.CreateSessionParams{
		TokenHash: hash, UserID: user.ID, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	database.Exec(t, "UPDATE users SET role = 'admin' WHERE id = $1", user.ID)
	engine := gin.New()
	s.Mount(engine.Group("/api/v1"))
	return &provisionRig{t: t, db: database, service: s, engine: engine, cookie: s.sessionCookie + "=" + token, logs: logs}
}

func (r *provisionRig) request(method, path, body string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range header {
		req.Header[k] = v
	}
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Cookie", r.cookie)
		req.Header.Set("Origin", "http://localhost:8080")
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

func (r *provisionRig) bearer(key string) int {
	return r.request(http.MethodGet, "/api/v1/me", "", http.Header{"Authorization": {"Bearer " + key}}).Code
}

func (r *provisionRig) accounts() []serviceAccountJSON {
	r.t.Helper()
	rec := r.request(http.MethodGet, "/api/v1/service-accounts", "", nil)
	var body struct {
		Items []serviceAccountJSON `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		r.t.Fatalf("list service accounts = %d %s", rec.Code, rec.Body)
	}
	return body.Items
}

func (r *provisionRig) fileKeys(accountID string) (active, revoked int) {
	r.t.Helper()
	rows, err := r.db.SQLDB(r.t).QueryContext(r.t.Context(), `SELECT revoked_at IS NULL FROM service_account_keys WHERE service_account_id = $1 AND name = 'From file'`, accountID)
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var isActive bool
		if err := rows.Scan(&isActive); err != nil {
			r.t.Fatal(err)
		}
		if isActive {
			active++
		} else {
			revoked++
		}
	}
	if err := rows.Err(); err != nil {
		r.t.Fatal(err)
	}
	return active, revoked
}

func fileAccount(t *testing.T, role string) (config.ServiceAccount, string) {
	t.Helper()
	key, _, err := newToken(keyPrefix)
	if err != nil {
		t.Fatal(err)
	}
	return config.ServiceAccount{Name: "demo-loadgen", Role: role, Description: "Posts the demo's deploy markers", Key: datasource.NewSecret(key)}, key
}

func TestEnsureProvisionedCreatesUpdatesAndRotates(t *testing.T) {
	r := newProvisionRig(t)
	account, key := fileAccount(t, "editor")
	if err := r.service.EnsureProvisioned(t.Context(), []config.ServiceAccount{account}); err != nil {
		t.Fatal(err)
	}
	if code := r.bearer(key); code != http.StatusOK {
		t.Fatalf("the file key answers %d, want 200", code)
	}
	listed := r.accounts()
	if len(listed) != 1 || listed[0].Name != "demo-loadgen" || listed[0].Role != RoleEditor || !listed[0].Provisioned || listed[0].Description != account.Description {
		t.Fatalf("accounts = %+v, want the provisioned editor", listed)
	}
	id := listed[0].ID

	if err := r.service.EnsureProvisioned(t.Context(), []config.ServiceAccount{account}); err != nil {
		t.Fatal(err)
	}
	if active, revoked := r.fileKeys(id); active != 1 || revoked != 0 {
		t.Fatalf("after an unchanged boot: %d active and %d revoked file keys, want 1 and 0", active, revoked)
	}

	rotated, newKey := fileAccount(t, "viewer")
	rotated.Description = "Reads the demo"
	if err := r.service.EnsureProvisioned(t.Context(), []config.ServiceAccount{rotated}); err != nil {
		t.Fatal(err)
	}
	if r.bearer(key) != http.StatusUnauthorized || r.bearer(newKey) != http.StatusOK {
		t.Fatalf("after rotation the old key answers %d and the new one %d, want 401 and 200", r.bearer(key), r.bearer(newKey))
	}
	if active, revoked := r.fileKeys(id); active != 1 || revoked != 1 {
		t.Fatalf("after rotation: %d active and %d revoked file keys, want 1 and 1", active, revoked)
	}
	if listed := r.accounts(); listed[0].ID != id || listed[0].Role != RoleViewer || listed[0].Description != "Reads the demo" {
		t.Fatalf("accounts = %+v, want the same account as a viewer with the new description", listed)
	}

	if err := r.service.EnsureProvisioned(t.Context(), []config.ServiceAccount{account}); err == nil || !strings.Contains(err.Error(), "used before") {
		t.Fatalf("going back to a revoked key = %v, want a refusal", err)
	}

	for _, secret := range []string{key, newKey, key[4:], newKey[4:]} {
		if strings.Contains(r.logs.String(), secret) {
			t.Fatal("the logs hold a file key")
		}
	}
}

func TestEnsureProvisionedNeverAdoptsAnAccountMadeThroughTheAPI(t *testing.T) {
	r := newProvisionRig(t)
	if rec := r.request(http.MethodPost, "/api/v1/service-accounts", `{"name":"demo-loadgen","role":"viewer"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	account, key := fileAccount(t, "admin")
	err := r.service.EnsureProvisioned(t.Context(), []config.ServiceAccount{account})
	var refused *ProvisionRefusedError
	if !errors.As(err, &refused) || refused.Name != "demo-loadgen" || !strings.Contains(err.Error(), "never taken over") {
		t.Fatalf("got %v, want a refusal naming the account", err)
	}
	if strings.Contains(err.Error(), key) {
		t.Fatal("the refusal holds the key")
	}
	if listed := r.accounts(); listed[0].Role != RoleViewer || listed[0].Provisioned {
		t.Fatalf("accounts = %+v, want the API account unchanged", listed)
	}
	if code := r.bearer(key); code != http.StatusUnauthorized {
		t.Fatalf("the file key answers %d, want 401", code)
	}
}

func TestProvisionedAccountsRefuseRoleChangesAndRevocation(t *testing.T) {
	r := newProvisionRig(t)
	account, _ := fileAccount(t, "editor")
	if err := r.service.EnsureProvisioned(t.Context(), []config.ServiceAccount{account}); err != nil {
		t.Fatal(err)
	}
	id := r.accounts()[0].ID
	var keyID string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM service_account_keys WHERE service_account_id = $1`, id).Scan(&keyID); err != nil {
		t.Fatal(err)
	}

	for _, call := range []struct{ method, path, body string }{
		{http.MethodPatch, "/api/v1/service-accounts/" + id, `{"role":"admin"}`},
		{http.MethodDelete, "/api/v1/service-accounts/" + id + "/keys/" + keyID, ""},
	} {
		rec := r.request(call.method, call.path, call.body, nil)
		var body struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusConflict || body.Code != "provisioned" {
			t.Fatalf("%s %s = %d %s, want 409 provisioned", call.method, call.path, rec.Code, rec.Body)
		}
	}
	if listed := r.accounts(); listed[0].Role != RoleEditor {
		t.Fatalf("role = %s, want editor kept", listed[0].Role)
	}

	if rec := r.request(http.MethodPost, "/api/v1/service-accounts", `{"name":"ci","role":"viewer"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	other := r.accounts()[0]
	if other.Name != "ci" || other.Provisioned {
		t.Fatalf("first account = %+v, want ci, not provisioned", other)
	}
	if rec := r.request(http.MethodPatch, "/api/v1/service-accounts/"+other.ID, `{"role":"editor"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("role change of an API account = %d %s, want 200", rec.Code, rec.Body)
	}
}
