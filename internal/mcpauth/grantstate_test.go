package mcpauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func at(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func TestGrantStateTellsWhyAConnectionEnded(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   grantFacts
		want string
	}{
		{"live", grantFacts{created: created, expires: created.Add(refreshTTL), now: created.Add(time.Hour)}, GrantStateActive},
		{"unused for 30 days", grantFacts{created: created, expires: created.Add(refreshTTL), now: created.Add(refreshTTL)}, GrantStateExpired},
		{"refreshed, then unused", grantFacts{created: created, expires: created.Add(50 * 24 * time.Hour), now: created.Add(60 * 24 * time.Hour)}, GrantStateExpired},
		{"at the 90 day limit", grantFacts{created: created, expires: created.Add(grantMaxAge), now: created.Add(grantMaxAge)}, GrantStateAgedOut},
		{"revoked while live", grantFacts{revoked: true, created: created, expires: created.Add(refreshTTL), now: created.Add(time.Hour)}, GrantStateRevoked},
		{"revoked, then past its expiry", grantFacts{revoked: true, created: created, expires: created.Add(refreshTTL), now: created.Add(grantMaxAge)}, GrantStateRevoked},
	}
	for _, c := range cases {
		if got := grantStateOf(c.in); got != c.want {
			t.Errorf("%s: state = %q, want %q", c.name, got, c.want)
		}
		if !GrantStates.Has(grantStateOf(c.in)) {
			t.Errorf("%s: %q is not a declared state", c.name, grantStateOf(c.in))
		}
	}
}

func TestAConnectedAgentCarriesItsStateAndRevokedReason(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	live := dbsqlc.ListOAuthGrantsRow{CreatedAt: at(created), ExpiresAt: at(created.Add(refreshTTL)), CheckedAt: at(created.Add(time.Hour))}
	if got := newGrantJSON(live); got.State != GrantStateActive || got.RevokedReason != nil {
		t.Errorf("a live connection = %q %v, want active and no reason", got.State, got.RevokedReason)
	}
	revoked := live
	revoked.RevokedAt = at(created.Add(time.Minute))
	revoked.RevokedReason = pgtype.Text{String: reasonAdmin, Valid: true}
	if got := newGrantJSON(revoked); got.State != GrantStateRevoked || got.RevokedReason == nil || *got.RevokedReason != reasonAdmin {
		t.Errorf("a revoked connection = %q %v, want revoked by an admin", got.State, got.RevokedReason)
	}
	expired := live
	expired.CheckedAt = at(created.Add(refreshTTL + time.Hour))
	if got := newGrantJSON(expired); got.State != GrantStateExpired || got.RevokedReason != nil {
		t.Errorf("an unused connection = %q %v, want expired and no reason", got.State, got.RevokedReason)
	}
	raw, err := json.Marshal(newGrantJSON(live))
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wire); err != nil || string(wire["revoked_reason"]) != "null" {
		t.Errorf("a live connection's revoked_reason on the wire = %s, want null", wire["revoked_reason"])
	}
}

func TestEachCauseOfAnEndedRefreshHasItsOwnDescription(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	live := dbsqlc.GetOAuthRefreshRow{CreatedAt: at(created), ExpiresAt: at(created.Add(refreshTTL)), DbNow: at(created.Add(time.Hour))}
	if ended := endedRefresh(live); ended != nil {
		t.Fatalf("a live refresh token is refused as %+v", ended)
	}
	seen := map[string]string{errInvalidGrant.description: "the unnamed cause", errRefreshAlreadyUsed.description: "a rotated token"}
	note := func(cause string, e *endedGrant) {
		t.Helper()
		if e == nil {
			t.Fatalf("%s is not refused", cause)
		}
		refusal := e.refusal()
		if refusal.code != "invalid_grant" || refusal.status != http.StatusBadRequest {
			t.Errorf("%s answers %d %s, want 400 invalid_grant", cause, refusal.status, refusal.code)
		}
		if other, dup := seen[refusal.description]; dup {
			t.Errorf("%s and %s share the description %q", cause, other, refusal.description)
		}
		seen[refusal.description] = cause
	}
	for _, reason := range []string{"person", "admin", "client", "replaced", "refresh_reuse", "code_reuse"} {
		row := live
		row.RevokedAt = at(created.Add(time.Minute))
		row.RevokedReason = pgtype.Text{String: reason, Valid: true}
		ended := endedRefresh(row)
		note("revoked by "+reason, ended)
		if ended.state != GrantStateRevoked || ended.revokedReason != reason {
			t.Errorf("revoked by %s = %+v", reason, ended)
		}
	}
	unused := live
	unused.DbNow = at(created.Add(refreshTTL))
	note("expired", endedRefresh(unused))
	old := live
	old.ExpiresAt = at(created.Add(grantMaxAge))
	old.DbNow = at(created.Add(grantMaxAge))
	aged := endedRefresh(old)
	note("aged out", aged)
	if aged.state != GrantStateAgedOut {
		t.Errorf("a connection at its limit = %q, want aged_out", aged.state)
	}
}

func TestAnEndedGrantAnswerKeepsTheOAuthCodeAndAddsTheCause(t *testing.T) {
	rec := httptest.NewRecorder()
	writeEndedGrant(rec, &endedGrant{state: GrantStateRevoked, revokedReason: reasonPerson})
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || body["error"] != "invalid_grant" || body["error_description"] == "" || body["grant_state"] != GrantStateRevoked || body["revoked_reason"] != reasonPerson {
		t.Errorf("answer = %d %v", rec.Code, body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("headers %v lack no-store", rec.Header())
	}
	rec = httptest.NewRecorder()
	writeEndedGrant(rec, &endedGrant{state: GrantStateExpired})
	body = map[string]string{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, has := body["revoked_reason"]; has || body["grant_state"] != GrantStateExpired {
		t.Errorf("an expired connection answers %v, want no revoked_reason", body)
	}
}

func TestEndedConnectionsAreListedOnlyWhenAskedFor(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	got := r.connect(t, ada, clientID)
	id := r.grantOf(t, ada, clientID)
	assertStatus(t, r.api(ada, http.MethodDelete, "/me/grants/"+id), http.StatusNoContent)

	if n := len(r.grants(t, ada, "/me/grants").Grants); n != 0 {
		t.Fatalf("the default list holds %d ended connections", n)
	}
	rec := r.api(ada, http.MethodGet, "/me/grants?include_ended=true")
	var page struct {
		Grants []struct {
			ID            string  `json:"id"`
			State         string  `json:"state"`
			RevokedReason *string `json:"revoked_reason"`
		} `json:"grants"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list with ended = %d %s", rec.Code, rec.Body)
	}
	if len(page.Grants) != 1 || page.Grants[0].ID != id || page.Grants[0].State != GrantStateRevoked || page.Grants[0].RevokedReason == nil || *page.Grants[0].RevokedReason != reasonPerson {
		t.Fatalf("list with ended = %s, want the connection revoked by its person", rec.Body)
	}
	assertStatus(t, r.api(ada, http.MethodGet, "/me/grants?include_ended=maybe"), http.StatusBadRequest)

	answer := r.oauth(http.MethodPost, "/oauth/token", refreshForm(clientID, got.Refresh))
	var refused map[string]string
	if err := json.Unmarshal(answer.Body.Bytes(), &refused); err != nil {
		t.Fatal(err)
	}
	if answer.Code != http.StatusBadRequest || refused["error"] != "invalid_grant" || refused["grant_state"] != GrantStateRevoked || refused["revoked_reason"] != reasonPerson {
		t.Fatalf("refresh after a disconnect = %d %v, want invalid_grant revoked by the person", answer.Code, refused)
	}
}
