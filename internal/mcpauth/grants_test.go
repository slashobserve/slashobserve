package mcpauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
)

type listedGrant struct {
	ID     string `json:"id"`
	Client struct {
		ID           string  `json:"id"`
		Name         *string `json:"name"`
		RedirectHost string  `json:"redirect_host"`
		Local        bool    `json:"local"`
	} `json:"client"`
	Person struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"person"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
	ExpiresAt  string  `json:"expires_at"`
	State      string  `json:"state"`
}

type grantPageJSON struct {
	Grants     []listedGrant `json:"grants"`
	NextCursor *string       `json:"next_cursor"`
}

func (r *rig) grants(t *testing.T, by auth.Principal, path string) grantPageJSON {
	t.Helper()
	rec := r.api(by, http.MethodGet, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body.String())
	}
	var page grantPageJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func (r *rig) grantOf(t *testing.T, by auth.Principal, clientID string) string {
	t.Helper()
	for _, g := range r.grants(t, by, "/me/grants").Grants {
		if g.Client.ID == clientID {
			return g.ID
		}
	}
	t.Fatalf("%s has no grant for %s", by.Name, clientID)
	return ""
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
}

func TestPersonListsOnlyTheirOwnGrants(t *testing.T) {
	r := newRig(t)
	named, anonymous := r.registerClient(t, "claude-code"), r.registerClient(t, "")
	r.connect(t, ada, named)
	r.connect(t, ada, anonymous)
	r.connect(t, bob, named)

	mine := r.grants(t, ada, "/me/grants")
	if len(mine.Grants) != 2 || mine.NextCursor != nil {
		t.Fatalf("ada's page = %+v", mine)
	}
	for _, g := range mine.Grants {
		if g.Person.ID != ada.ID || g.Person.Name != "Ada" || g.Person.Email != "ada@example.com" {
			t.Errorf("a grant of someone else is listed: %+v", g.Person)
		}
		if g.Client.RedirectHost != "127.0.0.1:33418" || !g.Client.Local || g.LastUsedAt != nil || g.CreatedAt == "" || g.ExpiresAt == "" || g.State != "active" {
			t.Errorf("grant fields %+v", g)
		}
	}
	if theirs := r.grants(t, bob, "/me/grants"); len(theirs.Grants) != 1 || theirs.Grants[0].Person.ID != bob.ID {
		t.Fatalf("bob's page = %+v", theirs)
	}
}

func TestGrantListShowsTheClientNameAsActorsDo(t *testing.T) {
	r := newRig(t)
	named := r.registerClient(t, "claude-code")
	r.connect(t, ada, named)
	got := r.grants(t, ada, "/me/grants").Grants[0]
	if got.Client.Name == nil || *got.Client.Name != actor.ClientName("claude-code") {
		t.Fatalf("client name %v", got.Client.Name)
	}
}

func TestGrantListShowsLastUseAfterTheClientCallsMCP(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	got := r.connect(t, ada, clientID)
	if _, _, ok := r.authenticate(t, got.Access); !ok {
		t.Fatal("the access token was refused")
	}
	if g := r.grants(t, ada, "/me/grants").Grants[0]; g.LastUsedAt == nil {
		t.Fatalf("last_used_at is null after use: %+v", g)
	}
}

func TestGrantListPages(t *testing.T) {
	r := newRig(t)
	want := map[string]bool{}
	for _, name := range []string{"one", "two", "three"} {
		clientID := r.registerClient(t, name)
		r.connect(t, ada, clientID)
		want[r.grantOf(t, ada, clientID)] = true
	}

	first := r.grants(t, ada, "/me/grants?limit=2")
	if len(first.Grants) != 2 || first.NextCursor == nil {
		t.Fatalf("first page = %+v", first)
	}
	second := r.grants(t, ada, "/me/grants?limit=2&cursor="+*first.NextCursor)
	if len(second.Grants) != 1 || second.NextCursor != nil {
		t.Fatalf("second page = %+v", second)
	}
	for _, g := range append(first.Grants, second.Grants...) {
		if !want[g.ID] {
			t.Errorf("grant %s listed twice or unknown", g.ID)
		}
		delete(want, g.ID)
	}
	if len(want) != 0 {
		t.Errorf("grants never listed: %v", want)
	}
}

func TestGrantListRefusesBadParameters(t *testing.T) {
	r := newRig(t)
	for _, path := range []string{"/me/grants?limit=0", "/me/grants?limit=201", "/me/grants?limit=x", "/me/grants?cursor=nope", "/grants?cursor=nope"} {
		by := ada
		if strings.HasPrefix(path, "/grants") {
			by = root
		}
		assertStatus(t, r.api(by, http.MethodGet, path), http.StatusBadRequest)
	}
	assertStatus(t, r.api(ada, http.MethodDelete, "/me/grants/not-a-uuid"), http.StatusBadRequest)
}

func TestPersonDisconnectsOwnGrantAndTokensStop(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	got := r.connect(t, ada, clientID)
	id := r.grantOf(t, ada, clientID)

	assertStatus(t, r.api(ada, http.MethodDelete, "/me/grants/"+id), http.StatusNoContent)

	if _, _, ok := r.authenticate(t, got.Access); ok {
		t.Fatal("the access token works after the person disconnected the agent")
	}
	r.tokenError(t, refreshForm(clientID, got.Refresh), http.StatusBadRequest, "invalid_grant")
	if reason := r.revokedReason(t, ada.ID, clientID); reason != "person" {
		t.Fatalf("revoked reason %q, want person", reason)
	}
	if n := len(r.grants(t, ada, "/me/grants").Grants); n != 0 {
		t.Fatalf("%d grants still listed", n)
	}
	assertStatus(t, r.api(ada, http.MethodDelete, "/me/grants/"+id), http.StatusNotFound)

	logs := r.logs.String()
	if !strings.Contains(logs, "oauth grant revoked") || !strings.Contains(logs, id) {
		t.Errorf("the revocation was not logged: %s", logs)
	}
	for _, secret := range []string{got.Access, got.Refresh, "claude-code"} {
		if strings.Contains(logs, secret) {
			t.Errorf("logs contain %q", secret)
		}
	}
}

func TestPersonCannotDisconnectAnotherPersonsGrant(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	got := r.connect(t, ada, clientID)
	id := r.grantOf(t, ada, clientID)

	assertStatus(t, r.api(bob, http.MethodDelete, "/me/grants/"+id), http.StatusNotFound)

	if _, _, ok := r.authenticate(t, got.Access); !ok {
		t.Fatal("the grant was revoked by someone else")
	}
	if n := len(r.grants(t, ada, "/me/grants").Grants); n != 1 {
		t.Fatalf("%d grants listed, want 1", n)
	}
}

func TestAdminListsEveryonesGrantsAndDisconnectsAny(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	adas, bobs := r.connect(t, ada, clientID), r.connect(t, bob, clientID)

	all := r.grants(t, root, "/grants")
	if len(all.Grants) != 2 || all.NextCursor != nil {
		t.Fatalf("admin's page = %+v", all)
	}
	people := map[string]bool{}
	for _, g := range all.Grants {
		people[g.Person.Email] = true
	}
	if !people["ada@example.com"] || !people["bob@example.com"] {
		t.Fatalf("people listed: %v", people)
	}
	if paged := r.grants(t, root, "/grants?limit=1"); len(paged.Grants) != 1 || paged.NextCursor == nil {
		t.Fatalf("admin's first page = %+v", paged)
	}

	adaGrant := r.grantOf(t, ada, clientID)
	assertStatus(t, r.api(root, http.MethodDelete, "/grants/"+adaGrant), http.StatusNoContent)

	if _, _, ok := r.authenticate(t, adas.Access); ok {
		t.Fatal("the access token works after an admin disconnected the agent")
	}
	if _, _, ok := r.authenticate(t, bobs.Access); !ok {
		t.Fatal("another person's agent was disconnected")
	}
	if reason := r.revokedReason(t, ada.ID, clientID); reason != "admin" {
		t.Fatalf("revoked reason %q, want admin", reason)
	}
	var by string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT revoked_by::text FROM oauth_grants WHERE id = $1`, adaGrant).Scan(&by)
	if err != nil || by != root.ID {
		t.Fatalf("revoked_by = %q, %v", by, err)
	}
	assertStatus(t, r.api(root, http.MethodDelete, "/grants/"+adaGrant), http.StatusNotFound)
	if n := len(r.grants(t, root, "/grants").Grants); n != 1 {
		t.Fatalf("%d grants listed after the revoke, want 1", n)
	}
}

func TestAdminGrantRoutesAreForAdminPeopleOnly(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	got := r.connect(t, ada, clientID)
	id := r.grantOf(t, ada, clientID)

	for _, by := range []auth.Principal{ada, bot} {
		assertStatus(t, r.api(by, http.MethodGet, "/grants"), http.StatusForbidden)
		assertStatus(t, r.api(by, http.MethodDelete, "/grants/"+id), http.StatusForbidden)
	}
	assertStatus(t, r.api(bot, http.MethodGet, "/me/grants"), http.StatusForbidden)
	assertStatus(t, r.api(bot, http.MethodDelete, "/me/grants/"+id), http.StatusForbidden)
	if _, _, ok := r.authenticate(t, got.Access); !ok {
		t.Fatal("a refused call disconnected the agent")
	}
}

func TestRevokedAndExpiredGrantsAreNotListed(t *testing.T) {
	r := newRig(t)
	live, revoked, expired := r.registerClient(t, "live"), r.registerClient(t, "revoked"), r.registerClient(t, "expired")
	for _, id := range []string{live, revoked, expired} {
		r.connect(t, ada, id)
	}
	r.db.Exec(t, `UPDATE oauth_grants SET revoked_at = now(), revoked_reason = 'client' WHERE client_id = $1`, revoked)
	r.db.Exec(t, `UPDATE oauth_grants SET expires_at = now() - interval '1 second' WHERE client_id = $1`, expired)

	if got := r.grants(t, ada, "/me/grants"); len(got.Grants) != 1 || got.Grants[0].Client.ID != live {
		t.Fatalf("/me/grants lists %+v", got.Grants)
	}
	if all := r.grants(t, root, "/grants"); len(all.Grants) != 1 || all.Grants[0].Client.ID != live {
		t.Fatalf("/grants lists %+v", all.Grants)
	}
}
