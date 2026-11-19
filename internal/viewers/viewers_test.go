package viewers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
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
	ada    = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Role: auth.RoleViewer}
	bob    = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Bob", Role: auth.RoleViewer}
	cy     = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "", Role: auth.RoleViewer}
	bot    = auth.Principal{Kind: auth.KindServiceAccount, ID: "33333333-3333-4333-8333-333333333333", Name: "ci", Role: auth.RoleAdmin}
	nobody = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Nobody", Role: auth.RoleViewer}
)

type rig struct {
	db      *dbtest.Database
	service *Service
	access  *folders.Access
}

func newRig(t *testing.T) rig {
	t.Helper()
	d := dbtest.New(t)
	d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES
		($1, 'https://idp', 'ada', 'ada@example.com', 'Ada'),
		($2, 'https://idp', 'bob', 'bob@example.com', 'bob'),
		($3, 'https://idp', 'cy', 'cy@example.com', ''),
		($4, 'https://idp', 'nobody', 'nobody@example.com', 'Nobody')`, ada.ID, bob.ID, cy.ID, nobody.ID)
	return rig{db: d, service: New(d.DB), access: folders.NewAccess(d.DB)}
}

func (r rig) dashboard(t *testing.T) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ('Checkout', 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func names(viewers []Viewer) []string {
	out := make([]string, len(viewers))
	for i, v := range viewers {
		out[i] = v.Name
	}
	return out
}

func (r rig) others(t *testing.T, by auth.Principal, dash pgtype.UUID) []string {
	t.Helper()
	got, err := r.service.Others(t.Context(), by, dash)
	if err != nil {
		t.Fatal(err)
	}
	return names(got)
}

func TestPeopleAreListedByNameWhileTheirPageKeepsReporting(t *testing.T) {
	r := newRig(t)
	dash := r.dashboard(t)
	for _, p := range []auth.Principal{cy, bob, ada} {
		if err := r.service.Record(t.Context(), p, dash); err != nil {
			t.Fatal(err)
		}
	}

	if got := r.others(t, ada, dash); !equal(got, []string{"bob", "cy@example.com"}) {
		t.Fatalf("what Ada sees = %v, want Bob and Cy by name or email, without Ada", got)
	}
	if got := r.others(t, bot, dash); len(got) != 3 {
		t.Fatalf("what a key sees = %v, want all three", got)
	}

	r.db.Exec(t, `UPDATE dashboard_viewers SET seen_at = now() - interval '2 minutes' WHERE user_id = $1::uuid`, bob.ID)
	if got := r.others(t, ada, dash); !equal(got, []string{"cy@example.com"}) {
		t.Fatalf("after Bob stopped reporting = %v, want only Cy", got)
	}
	if err := r.service.Record(t.Context(), bob, dash); err != nil {
		t.Fatal(err)
	}
	if got := r.others(t, ada, dash); len(got) != 2 {
		t.Fatalf("after Bob reported again = %v, want Bob back", got)
	}
}

func TestOnlyPeopleAreRecordedAndOldRowsAreDropped(t *testing.T) {
	r := newRig(t)
	dash := r.dashboard(t)
	if err := r.service.Record(t.Context(), bot, dash); err != nil {
		t.Fatalf("a key: %v", err)
	}
	ghost := auth.Principal{Kind: auth.KindUser, ID: "66666666-6666-4666-8666-666666666666", Name: "Ghost"}
	if err := r.service.Record(t.Context(), ghost, dash); err != nil {
		t.Fatalf("a person with no account: %v", err)
	}
	if err := r.service.Record(t.Context(), bob, dash); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE dashboard_viewers SET seen_at = now() - interval '2 hours' WHERE user_id = $1::uuid`, bob.ID)
	if err := r.service.Record(t.Context(), ada, dash); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM dashboard_viewers`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("%d rows remain, want only Ada's", rows)
	}
}

func serve(t *testing.T, r rig, by auth.Principal, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), by))
	})
	NewHandler(r.service, r.access, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1"+path, nil))
	return rec
}

func viewersOf(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Viewers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"viewers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Viewers == nil {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	out := make([]string, len(body.Viewers))
	for i, v := range body.Viewers {
		out[i] = v.Name
	}
	return out
}

func TestTheRoutesReportAndListAndHonourFolderAccess(t *testing.T) {
	r := newRig(t)
	dash := r.dashboard(t)
	path := "/dashboards/" + dash.String() + "/viewers"

	if got := viewersOf(t, serve(t, r, bob, http.MethodPost, path)); len(got) != 0 {
		t.Fatalf("Bob alone sees %v", got)
	}
	if got := viewersOf(t, serve(t, r, ada, http.MethodPost, path)); !equal(got, []string{"bob"}) {
		t.Fatalf("Ada's report answers %v, want Bob", got)
	}
	if got := viewersOf(t, serve(t, r, bot, http.MethodGet, path)); !equal(got, []string{"Ada", "bob"}) {
		t.Fatalf("a key reads %v, want both", got)
	}
	if got := viewersOf(t, serve(t, r, bot, http.MethodPost, path)); len(got) != 2 {
		t.Fatalf("a key reporting changed the list to %v", got)
	}

	restricted := foldertest.Restricted(t, r.db, "Payroll", "payroll-team")
	foldertest.Move(t, r.db, dash, restricted)
	if rec := serve(t, r, nobody, http.MethodGet, path); rec.Code != http.StatusForbidden {
		t.Fatalf("a person who cannot see the folder: status %d, want 403", rec.Code)
	}
	if rec := serve(t, r, nobody, http.MethodPost, path); rec.Code != http.StatusForbidden {
		t.Fatalf("reporting without access: status %d, want 403", rec.Code)
	}
	if rec := serve(t, r, ada, http.MethodGet, "/dashboards/00000000-0000-4000-8000-000000000000/viewers"); rec.Code != http.StatusNotFound {
		t.Fatalf("a missing dashboard: status %d, want 404", rec.Code)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
