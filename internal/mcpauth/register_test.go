package mcpauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func (r *rig) postRegistration(body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.server.ServeHTTP(rec, req)
	return rec
}

func TestRegistrationRefusesWithTheRFC7591Codes(t *testing.T) {
	r := newRig(t)
	elevenURIs := `["` + strings.Repeat(`http://localhost/cb","`, 10) + `http://localhost/cb"]`
	cases := []struct {
		name, body, code string
	}{
		{"unknown scheme", `{"redirect_uris":["cursor://x/cb"]}`, "invalid_redirect_uri"},
		{"plain http host", `{"redirect_uris":["http://example.com/cb"]}`, "invalid_redirect_uri"},
		{"fragment", `{"redirect_uris":["https://h/cb#x"]}`, "invalid_redirect_uri"},
		{"eleven uris", `{"redirect_uris":` + elevenURIs + `}`, "invalid_redirect_uri"},
		{"missing redirect uris", `{"client_name":"x"}`, "invalid_redirect_uri"},
		{"empty redirect uris", `{"redirect_uris":[]}`, "invalid_redirect_uri"},
		{"client credentials", `{"redirect_uris":["http://localhost/cb"],"grant_types":["client_credentials"]}`, "invalid_client_metadata"},
		{"implicit response type", `{"redirect_uris":["http://localhost/cb"],"response_types":["token"]}`, "invalid_client_metadata"},
		{"not json", `nope`, "invalid_client_metadata"},
		{"wrong field type", `{"redirect_uris":"http://localhost/cb"}`, "invalid_client_metadata"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := r.postRegistration(c.body)
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body %q: %v", rec.Body.String(), err)
			}
			if rec.Code != http.StatusBadRequest || body["error"] != c.code || body["error_description"] == "" {
				t.Fatalf("register = %d %v, want 400 %s", rec.Code, body, c.code)
			}
		})
	}
	var clients int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM oauth_clients`).Scan(&clients); err != nil || clients != 0 {
		t.Fatalf("%d clients stored by refused registrations, err %v", clients, err)
	}
}

func TestRegistrationRequiresJSON(t *testing.T) {
	r := newRig(t)
	rec := r.oauth(http.MethodPost, "/oauth/register", map[string][]string{"redirect_uris": {"http://localhost/cb"}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_client_metadata") {
		t.Fatalf("form registration = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRegistrationAnswersAPublicClient(t *testing.T) {
	r := newRig(t)
	rec := r.postRegistration(`{"client_name":"claude-code","redirect_uris":["http://localhost:3118/callback","https://claude.ai/api/mcp/auth_callback"],"token_endpoint_auth_method":"client_secret_basic","scope":"admin","unknown":1}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["client_id"] == "" || body["client_id_issued_at"] == nil || body["client_name"] != "claude-code" || body["token_endpoint_auth_method"] != "none" {
		t.Fatalf("body %v", body)
	}
	for _, secret := range []string{"client_secret", "registration_access_token"} {
		if _, ok := body[secret]; ok {
			t.Errorf("registration answered a %s", secret)
		}
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("registration lacks CORS for browser clients")
	}
}

func TestRegistrationStoresAnEmptyOrInvisibleNameAsNull(t *testing.T) {
	r := newRig(t)
	for _, name := range []string{"", "   ", "\u200b\u202e\u0000"} {
		body, _ := json.Marshal(map[string]any{"client_name": name, "redirect_uris": []string{"http://localhost/cb"}})
		rec := r.postRegistration(string(body))
		if rec.Code != http.StatusCreated {
			t.Fatalf("register %q = %d: %s", name, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "client_name") {
			t.Errorf("response names an unnamed client: %s", rec.Body.String())
		}
	}
	var named int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM oauth_clients WHERE name IS NOT NULL`).Scan(&named); err != nil || named != 0 {
		t.Fatalf("%d clients have a name, err %v", named, err)
	}
}

func TestRegistrationCapsClientsNoGrantUses(t *testing.T) {
	r := newRig(t)
	r.db.Exec(t, `INSERT INTO oauth_clients (redirect_uris) SELECT ARRAY['http://localhost/cb'] FROM generate_series(1, $1::int)`, maxUnusedClients)
	rec := r.postRegistration(`{"redirect_uris":["http://localhost/cb"]}`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), "temporarily_unavailable") {
		t.Fatalf("register at the cap = %d %v: %s", rec.Code, rec.Header(), rec.Body.String())
	}
}

func TestRegistrationIsRateLimited(t *testing.T) {
	r := newRig(t)
	for range registerPerMinute {
		if rec := r.postRegistration(`{"redirect_uris":["http://localhost/cb"]}`); rec.Code != http.StatusCreated {
			t.Fatalf("register = %d: %s", rec.Code, rec.Body.String())
		}
	}
	rec := r.postRegistration(`{"redirect_uris":["http://localhost/cb"]}`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("31st register = %d %v", rec.Code, rec.Header())
	}
}
