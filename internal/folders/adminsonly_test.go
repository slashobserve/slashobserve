package folders

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

var rootAdmin = auth.Principal{Kind: auth.KindServiceAccount, ID: "00000000-0000-4000-8000-0000000000aa", Name: "root", Role: auth.RoleAdmin}

func engineAs(t *testing.T, d *dbtest.Database, p auth.Principal, guard OpeningGuard) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
	})
	NewHandler(d.DB, NewAccess(d.DB), slog.New(slog.DiscardHandler)).GuardOpeningWith(guard).Register(auth.NewRoutes(api))
	return engine
}

func call(t *testing.T, engine *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	return rec
}

func folderMadeBy(t *testing.T, d *dbtest.Database, title string, creator pgtype.UUID) string {
	t.Helper()
	var id string
	if err := d.SQLDB(t).QueryRowContext(t.Context(),
		`INSERT INTO folders (title, created_by_kind, created_by_id, created_by_name) VALUES ($1, 'user', $2, 'creator') RETURNING id::text`,
		title, creator.String()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func sharingEvents(t *testing.T, d *dbtest.Database, folderID string) []map[string]any {
	t.Helper()
	rows, err := d.SQLDB(t).QueryContext(t.Context(),
		`SELECT data FROM audit_events WHERE action = 'folder.permissions_change' AND target_id = $1 ORDER BY occurred_at, id`, folderID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		out = append(out, data)
	}
	return out
}

func TestAnAdminsOnlyFolderHidesFromItsCreatorAndKeepsItsFlag(t *testing.T) {
	d := dbtest.New(t)
	alice := addUser(t, d, "alice")
	folder := folderMadeBy(t, d, "Payroll", alice)
	admin := engineAs(t, d, rootAdmin, nil)

	set := call(t, admin, http.MethodPut, "/api/v1/folders/"+folder+"/permissions", `{"entries":[],"admins_only":true}`)
	if set.Code != http.StatusOK {
		t.Fatalf("set = %d %s", set.Code, set.Body)
	}
	var body struct {
		Restricted bool `json:"restricted"`
		AdminsOnly bool `json:"admins_only"`
	}
	read := call(t, admin, http.MethodGet, "/api/v1/folders/"+folder+"/permissions", "")
	if err := json.Unmarshal(read.Body.Bytes(), &body); err != nil || !body.Restricted || !body.AdminsOnly {
		t.Fatalf("permissions = %d %s", read.Code, read.Body)
	}
	if got := call(t, admin, http.MethodGet, "/api/v1/folders/"+folder, ""); !strings.Contains(got.Body.String(), `"admins_only":true`) {
		t.Fatalf("folder read = %d %s, want admins_only", got.Code, got.Body)
	}

	creator := engineAs(t, d, userAs(alice, auth.RoleEditor), nil)
	if got := call(t, creator, http.MethodGet, "/api/v1/folders/"+folder, ""); got.Code != http.StatusNotFound {
		t.Fatalf("creator read = %d %s, want 404", got.Code, got.Body)
	}
	if got := call(t, creator, http.MethodGet, "/api/v1/folders", ""); strings.Contains(got.Body.String(), folder) {
		t.Fatalf("creator list = %s, want no admins-only folder", got.Body)
	}

	open := call(t, admin, http.MethodPut, "/api/v1/folders/"+folder+"/permissions", `{"entries":[]}`)
	if open.Code != http.StatusOK || strings.Contains(open.Body.String(), `"admins_only":true`) {
		t.Fatalf("open = %d %s", open.Code, open.Body)
	}
	if got := call(t, creator, http.MethodGet, "/api/v1/folders/"+folder, ""); got.Code != http.StatusOK {
		t.Fatalf("creator read after opening = %d %s", got.Code, got.Body)
	}

	events := sharingEvents(t, d, folder)
	if len(events) != 2 || events[0]["admins_only"] != true || events[0]["admins_only_before"] != false ||
		events[1]["admins_only"] != false || events[1]["admins_only_before"] != true {
		t.Fatalf("events = %v", events)
	}
}

func TestAdminsOnlyIsRefusedWithEntriesForNonAdminsAndOnPersonalFolders(t *testing.T) {
	d := dbtest.New(t)
	alice := addUser(t, d, "alice")
	folder := folderMadeBy(t, d, "Team", alice)
	admin := engineAs(t, d, rootAdmin, nil)

	withEntries := call(t, admin, http.MethodPut, "/api/v1/folders/"+folder+"/permissions",
		`{"entries":[{"group":"sre","level":"view"}],"admins_only":true}`)
	if withEntries.Code != http.StatusUnprocessableEntity {
		t.Fatalf("with entries = %d %s, want 422", withEntries.Code, withEntries.Body)
	}

	manager := engineAs(t, d, userAs(alice, auth.RoleEditor), nil)
	if got := call(t, manager, http.MethodPut, "/api/v1/folders/"+folder+"/permissions", `{"entries":[],"admins_only":true}`); got.Code != http.StatusForbidden {
		t.Fatalf("non-Admin manager = %d %s, want 403", got.Code, got.Body)
	}

	ada := addUser(t, d, "ada")
	owner := engineAs(t, d, userAs(ada, auth.RoleAdmin), nil)
	personal := personalFolderIDs(t, d, ada)[0].String()
	if got := call(t, owner, http.MethodPut, "/api/v1/folders/"+personal+"/permissions", `{"entries":[],"admins_only":true}`); got.Code != http.StatusForbidden {
		t.Fatalf("personal = %d %s, want 403", got.Code, got.Body)
	}
	if events := sharingEvents(t, d, folder); len(events) != 0 {
		t.Fatalf("refused changes recorded %v", events)
	}
}

func TestSharingAFolderThatHoldsRestrictedContentIsRefused(t *testing.T) {
	d := dbtest.New(t)
	folder := newFolder(t, d, "Payroll")
	admin := engineAs(t, d, rootAdmin, nil)
	if got := call(t, admin, http.MethodPut, "/api/v1/folders/"+folder+"/permissions", `{"entries":[],"admins_only":true}`); got.Code != http.StatusOK {
		t.Fatalf("set = %d %s", got.Code, got.Body)
	}
	guarded := engineAs(t, d, rootAdmin, func(_ context.Context, _ *dbsqlc.Queries, f dbsqlc.Folder) error {
		return RestrictedOpening([]ContentItem{{Kind: ContentDashboard, ID: "d1", Title: "Salaries", FolderID: f.ID.String(), FolderTitle: f.Title}}, 0, "payroll-db")
	})

	for name, body := range map[string]string{
		"an entry": `{"entries":[{"group":"sre","level":"view"}]}`,
		"open":     `{"entries":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			got := call(t, guarded, http.MethodPut, "/api/v1/folders/"+folder+"/permissions", body)
			var refusal struct {
				Code       string `json:"code"`
				Datasource string `json:"datasource"`
				Items      []struct {
					Kind  string `json:"kind"`
					Title string `json:"title"`
				} `json:"items"`
			}
			if err := json.Unmarshal(got.Body.Bytes(), &refusal); err != nil || got.Code != http.StatusConflict ||
				refusal.Code != CodeRestrictedContent || refusal.Datasource != "payroll-db" || len(refusal.Items) != 1 || refusal.Items[0].Title != "Salaries" {
				t.Fatalf("got %d %s", got.Code, got.Body)
			}
			if perms := call(t, admin, http.MethodGet, "/api/v1/folders/"+folder+"/permissions", ""); !strings.Contains(perms.Body.String(), `"admins_only":true`) ||
				!strings.Contains(perms.Body.String(), `"entries":[]`) {
				t.Fatalf("folder changed after a refusal: %s", perms.Body)
			}
		})
	}
}

func TestSwitchingOffOnlyAdminsForAFolderThatHoldsARestrictedAnswerIsRefused(t *testing.T) {
	d := dbtest.New(t)
	folder := newFolder(t, d, "Audit")
	admin := engineAs(t, d, rootAdmin, nil)
	if got := call(t, admin, http.MethodPut, "/api/v1/folders/"+folder+"/permissions", `{"entries":[],"admins_only":true}`); got.Code != http.StatusOK {
		t.Fatalf("set = %d %s", got.Code, got.Body)
	}
	guarded := engineAs(t, d, rootAdmin, func(_ context.Context, _ *dbsqlc.Queries, f dbsqlc.Folder) error {
		return RestrictedAnswerOpening([]ContentItem{{Kind: ContentObservation, ID: "o1", Title: "Who changed checkout", FolderID: f.ID.String()}})
	})
	for name, body := range map[string]string{
		"an entry": `{"entries":[{"group":"sre","level":"view"}]}`,
		"open":     `{"entries":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			got := call(t, guarded, http.MethodPut, "/api/v1/folders/"+folder+"/permissions", body)
			if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), `"code":"restricted_answer"`) || !strings.Contains(got.Body.String(), "Who changed checkout") {
				t.Fatalf("got %d %s", got.Code, got.Body)
			}
			if perms := call(t, admin, http.MethodGet, "/api/v1/folders/"+folder+"/permissions", ""); !strings.Contains(perms.Body.String(), `"admins_only":true`) {
				t.Fatalf("folder changed after a refusal: %s", perms.Body)
			}
		})
	}
}

func TestOnlyTheFolderRecordedAsTheAuditFolderCarriesTheAuditFlag(t *testing.T) {
	d := dbtest.New(t)
	trail := folderMadeBy(t, d, "Trail", addUser(t, d, "alice"))
	other := folderMadeBy(t, d, "Payroll", addUser(t, d, "bob"))
	admin := engineAs(t, d, rootAdmin, nil)

	flags := func() map[string]bool {
		rec := call(t, admin, http.MethodGet, "/api/v1/folders", "")
		var body struct {
			Folders []struct {
				ID    string `json:"id"`
				Audit bool   `json:"audit"`
			} `json:"folders"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("list = %d %s", rec.Code, rec.Body)
		}
		out := map[string]bool{}
		for _, f := range body.Folders {
			out[f.ID] = f.Audit
		}
		return out
	}

	if got := flags(); got[trail] || got[other] {
		t.Fatalf("before the Audit folder is recorded, flags = %v, want none", got)
	}
	if _, err := d.SQLDB(t).ExecContext(t.Context(), `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, AuditStarterKey, trail); err != nil {
		t.Fatal(err)
	}
	if got := flags(); !got[trail] || got[other] {
		t.Fatalf("flags = %v, want only %s", got, trail)
	}
	renamed := call(t, admin, http.MethodPatch, "/api/v1/folders/"+trail, `{"title":"Records"}`)
	if renamed.Code != http.StatusOK || !strings.Contains(renamed.Body.String(), `"audit":true`) {
		t.Fatalf("rename = %d %s, want audit kept", renamed.Code, renamed.Body)
	}
}

func TestANonAdminsFolderListCarriesNoAuditFolder(t *testing.T) {
	d := dbtest.New(t)
	trail := folderMadeBy(t, d, "Trail", addUser(t, d, "alice"))
	d.Exec(t, `UPDATE folders SET admins_only = true WHERE id = $1`, trail)
	d.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, AuditStarterKey, trail)
	editor := auth.Principal{Kind: auth.KindServiceAccount, ID: "00000000-0000-4000-8000-0000000000bb", Name: "ci", Role: auth.RoleEditor}

	rec := call(t, engineAs(t, d, editor, nil), http.MethodGet, "/api/v1/folders", "")
	var body struct {
		Folders []struct {
			ID    string `json:"id"`
			Audit bool   `json:"audit"`
		} `json:"folders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	for _, f := range body.Folders {
		if f.Audit || f.ID == trail {
			t.Errorf("a non-Admin's list holds %+v, want no Audit folder", f)
		}
	}
}
