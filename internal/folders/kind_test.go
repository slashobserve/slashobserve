package folders

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func decoded(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return body
}

func stringsOf(t *testing.T, v any) []string {
	t.Helper()
	items, ok := v.([]any)
	if !ok {
		t.Fatalf("%#v is not a list", v)
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i], _ = item.(string)
	}
	return out
}

func TestAFolderReadSaysItsKindAndWhatTheCallerMayDo(t *testing.T) {
	auditID := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	cases := []struct {
		name    string
		folder  Folder
		level   Level
		kind    string
		actions []string
	}{
		{"General for an Editor", Folder{General: true}, LevelEdit, "general", []string{"create_in", "move_into"}},
		{"General for an Admin", Folder{General: true}, LevelAdmin, "general", []string{"create_in", "move_into"}},
		{"General for a Viewer", Folder{General: true}, LevelView, "general", []string{}},
		{"the owner's personal folder", Folder{Personal: true}, LevelEdit, "personal", []string{"create_in"}},
		{"an open folder for its manager", Folder{}, LevelAdmin, "open", []string{"create_in", "move_into", "rename", "set_permissions", "delete"}},
		{"an open folder for an Editor", Folder{}, LevelEdit, "open", []string{"create_in", "move_into"}},
		{"an open folder for a Commenter", Folder{}, LevelComment, "open", []string{}},
		{"a listed folder", Folder{Restricted: true}, LevelEdit, "restricted", []string{"create_in", "move_into"}},
		{"an Admins-only folder", Folder{Restricted: true, AdminsOnly: true}, LevelAdmin, "admins_only", []string{"create_in", "move_into", "rename", "set_permissions", "delete"}},
		{"the Audit folder", Folder{ID: auditID, Restricted: true, AdminsOnly: true}, LevelAdmin, "audit", []string{"create_in", "move_into", "rename", "set_permissions", "delete"}},
		{"the Audit folder after an Admin opened it", Folder{ID: auditID}, LevelEdit, "audit", []string{"create_in", "move_into"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(folderBody(tc.folder, AbilitiesFor(tc.level), dbsqlc.ListFolderDetailsRow{}, auditID))
			if err != nil {
				t.Fatal(err)
			}
			body := decoded(t, raw)
			if body["kind"] != tc.kind {
				t.Errorf("kind = %v, want %s", body["kind"], tc.kind)
			}
			if got := stringsOf(t, body["allowed_actions"]); !slices.Equal(got, tc.actions) {
				t.Errorf("allowed_actions = %v, want %v", got, tc.actions)
			}
		})
	}
}

func refusalOf(t *testing.T, role auth.Role, folder folderAccess, handle gin.HandlerFunc) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/folders/x", nil).
		WithContext(auth.WithPrincipal(t.Context(), auth.Principal{Kind: auth.KindUser, ID: "00000000-0000-4000-8000-000000000001", Role: role}))
	c.Set(folderKey{}, folder)
	handle(c)
	return rec.Code, decoded(t, rec.Body.Bytes())
}

func TestRefusingAChangeToGeneralSaysWhatGeneralAllows(t *testing.T) {
	general := folderAccess{Folder: Folder{Title: "General", General: true}, Abilities: AbilitiesFor(LevelAdmin)}
	h := NewHandler(nil, nil, slog.New(slog.DiscardHandler))
	cases := []struct {
		name   string
		handle gin.HandlerFunc
	}{
		{"delete", h.remove},
		{"rename", func(c *gin.Context) { h.abortTitleWrite(c, ErrGeneralFolder, "rename folder") }},
		{"set permissions", func(c *gin.Context) { abortPermissionsRefusal(c, errGeneralFolder) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := refusalOf(t, auth.RoleAdmin, general, tc.handle)
			if status != http.StatusConflict || body["code"] != "general_folder" {
				t.Fatalf("refusal = %d %v", status, body)
			}
			if got := stringsOf(t, body["allowed_actions"]); !slices.Equal(got, []string{"create_in", "move_into"}) {
				t.Errorf("allowed_actions = %v, want create_in and move_into", got)
			}
		})
	}
}

func TestRefusingAMoveIntoMyDashboardsSaysWhatItsOwnerMayDoThere(t *testing.T) {
	for _, role := range allRoles {
		status, body := refusalOf(t, role, folderAccess{}, func(c *gin.Context) { AbortPersonalFolder(c, ErrMoveIntoPersonal.Error()) })
		if status != http.StatusConflict || body["code"] != "personal_folder" || body["error"] != ErrMoveIntoPersonal.Error() {
			t.Fatalf("%s: refusal = %d %v", role, status, body)
		}
		if got := stringsOf(t, body["allowed_actions"]); !slices.Equal(got, []string{"create_in"}) {
			t.Errorf("%s: allowed_actions = %v, want create_in only", role, got)
		}
	}
}

func TestRefusingLinkedContentInAnAdminsOnlyFolderSaysWhatAnAdminMayDoThere(t *testing.T) {
	for _, refusal := range []error{ErrLinkedMove, ErrLinkedObservationMove, ErrLinkedFolder} {
		status, body := refusalOf(t, auth.RoleAdmin, folderAccess{}, func(c *gin.Context) {
			if !AbortRestricted(c, refusal) {
				t.Errorf("%v was not refused", refusal)
			}
		})
		if status != http.StatusConflict || body["code"] != CodeAdminsOnlyFolder || body["error"] != refusal.Error() {
			t.Fatalf("refusal = %d %v", status, body)
		}
		if got := stringsOf(t, body["allowed_actions"]); !slices.Equal(got, []string{"create_in", "move_into", "rename", "set_permissions", "delete"}) {
			t.Errorf("allowed_actions = %v, want every folder action", got)
		}
	}
}

func TestOnlyAdminsActInAnAdminsOnlyFolder(t *testing.T) {
	for _, role := range []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor} {
		if got := AdminsOnlyActions(role); len(got) != 0 {
			t.Errorf("%s: actions = %v, want none", role, got)
		}
	}
}

func TestAFolderIsRestrictedWhenItListsSomeoneOrOnlyAdminsReadIt(t *testing.T) {
	entry := dbsqlc.ListFolderPermissionsRow{Level: "view", GroupName: pgtype.Text{String: "sre", Valid: true}}
	cases := []struct {
		name   string
		folder dbsqlc.Folder
		rows   []dbsqlc.ListFolderPermissionsRow
		want   bool
	}{
		{"open", dbsqlc.Folder{}, nil, false},
		{"listed", dbsqlc.Folder{}, []dbsqlc.ListFolderPermissionsRow{entry}, true},
		{"Admins only", dbsqlc.Folder{AdminsOnly: true}, nil, true},
	}
	for _, tc := range cases {
		body := permissionsBody(tc.folder, tc.rows)
		if body.Restricted != tc.want || body.Restricted != Restricted(tc.folder.AdminsOnly, len(tc.rows) > 0) {
			t.Errorf("%s: restricted = %v, want %v", tc.name, body.Restricted, tc.want)
		}
	}
}

func TestEveryKindAndActionIsNamedOnce(t *testing.T) {
	kinds := []Kind{KindGeneral, KindPersonal, KindAudit, KindAdminsOnly, KindRestricted, KindOpen}
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	if !slices.Equal(Kinds.Names(), names) {
		t.Errorf("Kinds = %v, want %v in the order KindOf decides them", Kinds.Names(), names)
	}
	everything := AllowedActions(Folder{}, AbilitiesFor(LevelAdmin))
	if !slices.Equal(everything, Actions) {
		t.Errorf("a managed open folder allows %v, want every action %v", everything, Actions)
	}
}
