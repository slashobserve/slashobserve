package homedash

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func (r rig) get(t *testing.T, by auth.Principal) Home {
	t.Helper()
	home, err := r.service.Get(t.Context(), by)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return home
}

func TestGetIsNoneUntilSetAndSetReplacesTheChoice(t *testing.T) {
	r := newRig(t)
	checkout, payments := r.dashboard(t, "Checkout"), r.dashboard(t, "Payments")
	if home := r.get(t, ada); home.Status != StatusNone || home.DashboardID != "" {
		t.Fatalf("before setting = %+v, want none", home)
	}

	if err := r.service.Set(t.Context(), ada, checkout); err != nil {
		t.Fatal(err)
	}
	home := r.get(t, ada)
	if home.Status != StatusOK || home.DashboardID != checkout.String() || home.Title != "Checkout" || home.FolderTitle != "General" || home.FolderID == "" || home.SetAt.IsZero() {
		t.Fatalf("after setting = %+v, want Checkout in General", home)
	}
	if other := r.get(t, bob); other.Status != StatusNone {
		t.Fatalf("another person sees %+v, want none", other)
	}

	for range 2 {
		if err := r.service.Set(t.Context(), ada, payments); err != nil {
			t.Fatal(err)
		}
	}
	if home := r.get(t, ada); home.Status != StatusOK || home.Title != "Payments" {
		t.Fatalf("after replacing = %+v, want Payments", home)
	}
}

func TestClearIsIdempotent(t *testing.T) {
	r := newRig(t)
	if err := r.service.Set(t.Context(), ada, r.dashboard(t, "Checkout")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := r.service.Clear(t.Context(), ada); err != nil {
			t.Fatal(err)
		}
	}
	if home := r.get(t, ada); home.Status != StatusNone {
		t.Fatalf("after clearing = %+v, want none", home)
	}
}

func TestSetRefusesWhatTheCallerCannotSee(t *testing.T) {
	r := newRig(t)
	payroll := r.dashboard(t, "Payroll")
	foldertest.Move(t, r.db, payroll, foldertest.Restricted(t, r.db, "Payroll", "payroll-team"))
	if err := r.service.Set(t.Context(), ada, payroll); err != ErrNotFound {
		t.Fatalf("restricted = %v, want ErrNotFound", err)
	}

	gone := r.dashboard(t, "Gone")
	r.db.Exec(t, `DELETE FROM dashboards WHERE id = $1`, gone.String())
	if err := r.service.Set(t.Context(), ada, gone); err != ErrNotFound {
		t.Fatalf("deleted = %v, want ErrNotFound", err)
	}
	if home := r.get(t, ada); home.Status != StatusNone {
		t.Fatalf("after the refusals = %+v, want none", home)
	}
}

func TestGetSaysGoneAfterADeleteAndUnreadableAfterARestriction(t *testing.T) {
	r := newRig(t)
	checkout, payroll := r.dashboard(t, "Checkout"), r.dashboard(t, "Payroll")

	if err := r.service.Set(t.Context(), ada, payroll); err != nil {
		t.Fatal(err)
	}
	foldertest.Move(t, r.db, payroll, foldertest.Restricted(t, r.db, "Payroll", "payroll-team"))
	home := r.get(t, ada)
	if home.Status != StatusUnreadable || home.DashboardID != payroll.String() || home.Title != "" {
		t.Fatalf("restricted = %+v, want unreadable without a title", home)
	}

	if err := r.service.Set(t.Context(), ada, checkout); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `DELETE FROM dashboards WHERE id = $1`, checkout.String())
	home = r.get(t, ada)
	if home.Status != StatusGone || home.DashboardID != checkout.String() {
		t.Fatalf("deleted = %+v, want gone", home)
	}
}

func serve(t *testing.T, r rig, by auth.Principal, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), by))
	})
	NewHandler(r.service, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/me/home-dashboard", strings.NewReader(body)))
	return rec
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

func TestHTTPRefusesServiceAccountKeys(t *testing.T) {
	r := newRig(t)
	body := `{"dashboard_id":"` + r.dashboard(t, "Checkout").String() + `"}`
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := serve(t, r, key, method, body)
		if rec.Code != http.StatusForbidden || codeOf(t, rec) != "user_session_required" {
			t.Fatalf("%s by a key = %d %s, want 403 user_session_required", method, rec.Code, rec.Body)
		}
	}
}

func TestHTTPSetGetAndClear(t *testing.T) {
	r := newRig(t)
	checkout := r.dashboard(t, "Checkout")

	rec := serve(t, r, ada, http.MethodGet, "")
	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"none","dashboard":null,"set_at":null}` {
		t.Fatalf("get before = %d %s", rec.Code, rec.Body)
	}

	rec = serve(t, r, ada, http.MethodPut, `{"dashboard_id":"`+checkout.String()+`"}`)
	var set struct {
		Status    string `json:"status"`
		SetAt     string `json:"set_at"`
		Dashboard struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			FolderTitle string `json:"folder_title"`
		} `json:"dashboard"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &set); err != nil || rec.Code != http.StatusOK || set.Status != "ok" || set.Dashboard.ID != checkout.String() || set.Dashboard.Title != "Checkout" || set.Dashboard.FolderTitle != "General" || set.SetAt == "" {
		t.Fatalf("put = %d %s (%v)", rec.Code, rec.Body, err)
	}

	if rec := serve(t, r, ada, http.MethodDelete, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := serve(t, r, ada, http.MethodGet, ""); !strings.Contains(rec.Body.String(), `"none"`) {
		t.Fatalf("get after delete = %s", rec.Body)
	}
}

func TestHTTPRejectsABadBodyAndAnUnknownDashboard(t *testing.T) {
	r := newRig(t)
	for body, want := range map[string]int{
		`{}`:                      http.StatusBadRequest,
		`{"dashboard_id":"nope"}`: http.StatusBadRequest,
		`{"dashboard_id":"00000000-0000-4000-8000-000000000000"}`:       http.StatusNotFound,
		`{"dashboard_id":"00000000-0000-4000-8000-000000000000","x":1}`: http.StatusBadRequest,
	} {
		if rec := serve(t, r, ada, http.MethodPut, body); rec.Code != want {
			t.Errorf("PUT %s = %d %s, want %d", body, rec.Code, rec.Body, want)
		}
	}
}
