package folders

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestPickImportFolder(t *testing.T) {
	checkout, reports := uuidOf(2), uuidOf(3)
	can := Abilities{View: true, Comment: true, Edit: true, Create: true}
	cannot := Abilities{View: true, Comment: true}
	levels := []folderLevel{
		{Folder: Folder{ID: checkout, Title: "Checkout"}, Abilities: can},
		{Folder: Folder{ID: reports, Title: "Reports"}, Abilities: cannot},
		{Folder: Folder{ID: uuidOf(1), Title: "General", General: true}, Abilities: can},
	}
	none := pgtype.UUID{}
	hidden := uuidOf(9)
	cases := []struct {
		name     string
		title    string
		existing pgtype.UUID
		role     auth.Role
		want     importChoice
	}{
		{"visible folder the caller can create in", "checkout", checkout, auth.RoleEditor, importChoice{action: importUse, title: "checkout", id: checkout}},
		{"visible folder the caller cannot create in", "Reports", reports, auth.RoleEditor, importChoice{action: importFallback, title: "Reports"}},
		{"folder the caller cannot see", "Secret", hidden, auth.RoleEditor, importChoice{action: importFallback, title: "Secret"}},
		{"folder the caller cannot see, as an admin", "Secret", hidden, auth.RoleAdmin, importChoice{action: importFallback, title: "Secret"}},
		{"missing folder for an editor", "Payments", none, auth.RoleEditor, importChoice{action: importCreate, title: "Payments"}},
		{"missing folder for an admin", "Payments", none, auth.RoleAdmin, importChoice{action: importCreate, title: "Payments"}},
		{"missing folder for a commenter", "Payments", none, auth.RoleCommenter, importChoice{action: importFallback, title: "Payments"}},
		{"title is trimmed", "  Payments  ", none, auth.RoleEditor, importChoice{action: importCreate, title: "Payments"}},
		{"empty title", "", none, auth.RoleEditor, importChoice{action: importDefault}},
		{"blank title", "   ", none, auth.RoleEditor, importChoice{action: importDefault}},
		{"General maps to the default folder", "general", none, auth.RoleEditor, importChoice{action: importDefault}},
		{"title too long", strings.Repeat("x", 101), none, auth.RoleEditor, importChoice{action: importDefault}},
		{"title with a control character", "Pay\nments", none, auth.RoleEditor, importChoice{action: importDefault}},
		{"title that is not UTF-8", "Pay\xffments", none, auth.RoleEditor, importChoice{action: importDefault}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickImportFolder(tt.title, levels, tt.existing, tt.role); got != tt.want {
				t.Fatalf("pickImportFolder() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
