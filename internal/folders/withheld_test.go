package folders

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestAnAdminWritingWhereOthersReadSeesNoAdminsOnlyFolder(t *testing.T) {
	d := dbtest.New(t)
	var adminsOnly string
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title, admins_only) VALUES ('Audit notes', true) RETURNING id::text`).Scan(&adminsOnly); err != nil {
		t.Fatal(err)
	}
	other := rootAdmin
	other.ID = "00000000-0000-4000-8000-0000000000bb"
	for _, c := range []struct {
		name    string
		access  *datasource.Access
		visible bool
	}{
		{"no access in the context", nil, true},
		{"its own request outside a run", &datasource.Access{By: rootAdmin}, true},
		{"its private chat", &datasource.Access{By: rootAdmin, Audience: datasource.AudienceCaller}, true},
		{"writing where only Admins read", &datasource.Access{By: rootAdmin, Audience: datasource.AudienceAdmins}, true},
		{"writing where others read", &datasource.Access{By: rootAdmin, Audience: datasource.AudienceEveryone}, false},
		{"someone else writing where others read", &datasource.Access{By: other, Audience: datasource.AudienceEveryone}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := t.Context()
			if c.access != nil {
				ctx = datasource.WithAccess(ctx, *c.access)
			}
			levels, err := NewAccess(d.DB).Levels(ctx, rootAdmin)
			if err != nil {
				t.Fatal(err)
			}
			seen := false
			for _, id := range levels.Visible() {
				seen = seen || id.String() == adminsOnly
			}
			if seen != c.visible {
				t.Fatalf("the Admins-only folder is visible %v, want %v", seen, c.visible)
			}
		})
	}
}
