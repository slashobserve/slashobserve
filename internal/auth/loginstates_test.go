package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginStopsStoringStatesAtTheOutstandingCap(t *testing.T) {
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
	r := newSessionRigWithIssuer(t, slog.New(slog.DiscardHandler), issuer.URL)
	r.db.Exec(t, `INSERT INTO oidc_login_states (state_hash, nonce, code_verifier, return_to, expires_at)
SELECT sha256(i::text::bytea), 'n', 'v', '/', now() + interval '10 minutes' FROM generate_series(1, $1::int) AS i`, maxLoginStates)
	stored := func() int {
		var n int
		if err := r.db.SQLDB(t).QueryRowContext(t.Context(), "SELECT count(*) FROM oidc_login_states").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	login := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
		return rec
	}

	rec := login()
	if got := rec.Header().Get("Location"); rec.Code != http.StatusFound || !strings.Contains(got, "error=unavailable") || stored() != maxLoginStates {
		t.Fatalf("at the cap: %d to %q with %d states", rec.Code, got, stored())
	}

	r.db.Exec(t, "DELETE FROM oidc_login_states WHERE state_hash = sha256('1'::bytea)")
	rec = login()
	if got := rec.Header().Get("Location"); rec.Code != http.StatusFound || !strings.HasPrefix(got, issuer.URL+"/auth") || stored() != maxLoginStates {
		t.Fatalf("below the cap: %d to %q with %d states", rec.Code, got, stored())
	}
}
