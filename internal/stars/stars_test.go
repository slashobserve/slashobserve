package stars

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

var (
	ada = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Role: auth.RoleViewer}
	bob = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Bob", Role: auth.RoleViewer}
	key = auth.Principal{Kind: auth.KindServiceAccount, ID: "33333333-3333-4333-8333-333333333333", Name: "ci", Role: auth.RoleAdmin}
)

type rig struct {
	db      *dbtest.Database
	service *Service
	access  *folders.Access
}

func newRig(t *testing.T) rig {
	t.Helper()
	d := dbtest.New(t)
	d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1, 'https://idp', 'ada', 'ada@example.com', 'Ada'), ($2, 'https://idp', 'bob', 'bob@example.com', 'Bob')`, ada.ID, bob.ID)
	access := folders.NewAccess(d.DB)
	return rig{db: d, service: New(d.DB, access), access: access}
}

func (r rig) dashboard(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ($1, 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`, title).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r rig) star(t *testing.T, by auth.Principal, id pgtype.UUID, minutesAgo int) {
	t.Helper()
	if err := r.service.Star(t.Context(), by, id); err != nil {
		t.Fatalf("star: %v", err)
	}
	r.db.Exec(t, `UPDATE dashboard_stars SET starred_at = now() - make_interval(mins => $3) WHERE user_id = $1 AND dashboard_id = $2`, by.ID, id.String(), minutesAgo)
}

func (r rig) titles(t *testing.T, by auth.Principal) []string {
	t.Helper()
	stars, err := r.service.List(t.Context(), by)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	out := make([]string, 0, len(stars))
	for _, s := range stars {
		out = append(out, s.Title)
	}
	return out
}

func TestStarsListNewestFirstAndBelongToOnePerson(t *testing.T) {
	r := newRig(t)
	checkout, payments, search := r.dashboard(t, "Checkout"), r.dashboard(t, "Payments"), r.dashboard(t, "Search")
	r.star(t, ada, payments, 30)
	r.star(t, ada, checkout, 5)
	r.star(t, ada, search, 60)
	r.star(t, bob, search, 1)

	if got, want := r.titles(t, ada), []string{"Checkout", "Payments", "Search"}; !slices.Equal(got, want) {
		t.Fatalf("Ada's stars = %v, want %v", got, want)
	}
	if got, want := r.titles(t, bob), []string{"Search"}; !slices.Equal(got, want) {
		t.Fatalf("Bob's stars = %v, want %v", got, want)
	}

	stars, err := r.service.List(t.Context(), ada)
	if err != nil {
		t.Fatal(err)
	}
	if first := stars[0]; first.DashboardID != checkout.String() || first.FolderTitle != "General" || first.FolderID == "" || first.StarredAt.IsZero() {
		t.Fatalf("first star = %+v, want Checkout in General with its time", first)
	}
}

func TestStarringTwiceKeepsOneStarAndUnstarRemovesIt(t *testing.T) {
	r := newRig(t)
	checkout := r.dashboard(t, "Checkout")
	r.star(t, ada, checkout, 10)
	if err := r.service.Star(t.Context(), ada, checkout); err != nil {
		t.Fatal(err)
	}
	if got := r.titles(t, ada); !slices.Equal(got, []string{"Checkout"}) {
		t.Fatalf("stars = %v, want one Checkout", got)
	}
	for range 2 {
		if err := r.service.Unstar(t.Context(), ada, checkout); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.titles(t, ada); len(got) != 0 {
		t.Fatalf("stars after unstar = %v, want none", got)
	}
}

func TestStarsHideDashboardsTheCallerCannotSeeAndGoWithTheDashboard(t *testing.T) {
	r := newRig(t)
	checkout, payroll := r.dashboard(t, "Checkout"), r.dashboard(t, "Payroll")
	r.star(t, ada, checkout, 2)
	r.star(t, ada, payroll, 1)
	foldertest.Move(t, r.db, payroll, foldertest.Restricted(t, r.db, "Payroll", "payroll-team"))
	if got := r.titles(t, ada); !slices.Equal(got, []string{"Checkout"}) {
		t.Fatalf("stars = %v, want only Checkout once Payroll is restricted", got)
	}

	r.db.Exec(t, `DELETE FROM dashboards WHERE id = $1`, checkout.String())
	if got := r.titles(t, ada); len(got) != 0 {
		t.Fatalf("stars = %v, want none after the delete", got)
	}
}

func TestStarringADeletedDashboardIsNotFound(t *testing.T) {
	r := newRig(t)
	gone := r.dashboard(t, "Gone")
	r.db.Exec(t, `DELETE FROM dashboards WHERE id = $1`, gone.String())
	if err := r.service.Star(t.Context(), ada, gone); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func serve(t *testing.T, r rig, by auth.Principal, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), by))
	})
	NewHandler(r.service, r.access.RequireDashboard(folders.NeedView), slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1"+path, nil))
	return rec
}

func TestHTTPStarsAndListsForPeopleOnly(t *testing.T) {
	r := newRig(t)
	checkout := r.dashboard(t, "Checkout")
	path := "/dashboards/" + checkout.String() + "/star"

	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		rec := serve(t, r, key, method, path)
		if rec.Code != http.StatusForbidden || !json.Valid(rec.Body.Bytes()) || codeOf(t, rec) != "user_session_required" {
			t.Fatalf("%s by a key = %d %s, want 403 user_session_required", method, rec.Code, rec.Body)
		}
	}
	if rec := serve(t, r, key, http.MethodGet, "/stars"); rec.Code != http.StatusForbidden || codeOf(t, rec) != "user_session_required" {
		t.Fatalf("list by a key = %d %s, want 403 user_session_required", rec.Code, rec.Body)
	}

	if rec := serve(t, r, ada, http.MethodPut, path); rec.Code != http.StatusNoContent {
		t.Fatalf("star = %d %s, want 204", rec.Code, rec.Body)
	}
	rec := serve(t, r, ada, http.MethodGet, "/stars")
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK || len(body.Items) != 1 || body.Items[0]["id"] != checkout.String() || body.Items[0]["title"] != "Checkout" {
		t.Fatalf("list = %d %s, want Checkout", rec.Code, rec.Body)
	}
	if rec := serve(t, r, ada, http.MethodDelete, path); rec.Code != http.StatusNoContent {
		t.Fatalf("unstar = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := serve(t, r, ada, http.MethodGet, "/stars"); rec.Code != http.StatusOK || rec.Body.String() != `{"items":[]}` {
		t.Fatalf("list after unstar = %d %s, want an empty list", rec.Code, rec.Body)
	}

	if rec := serve(t, r, ada, http.MethodPut, "/dashboards/00000000-0000-4000-8000-000000000000/star"); rec.Code != http.StatusNotFound {
		t.Fatalf("star of an unknown dashboard = %d %s, want 404", rec.Code, rec.Body)
	}
}

func codeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return body.Code
}
