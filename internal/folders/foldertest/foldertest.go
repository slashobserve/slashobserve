package foldertest

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func Restricted(t testing.TB, d *dbtest.Database, title, group string) pgtype.UUID {
	t.Helper()
	folder, err := d.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: title})
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	err = d.Queries().InsertFolderPermission(t.Context(), dbsqlc.InsertFolderPermissionParams{
		FolderID:  folder.ID,
		GroupName: pgtype.Text{String: group, Valid: true},
		Level:     "view",
	})
	if err != nil {
		t.Fatalf("restrict folder: %v", err)
	}
	return folder.ID
}

func Move(t testing.TB, d *dbtest.Database, dashboard, folder pgtype.UUID) {
	t.Helper()
	d.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, dashboard, folder)
}

func Open(t testing.TB, d *dbtest.Database, title string) pgtype.UUID {
	t.Helper()
	folder, err := d.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: title})
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	return folder.ID
}

func AdminsOnly(t testing.TB, d *dbtest.Database, title string) pgtype.UUID {
	t.Helper()
	id := Open(t, d, title)
	d.Exec(t, `UPDATE folders SET admins_only = true WHERE id = $1`, id)
	return id
}
