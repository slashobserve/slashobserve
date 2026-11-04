package folders

import (
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

func addUser(t *testing.T, d *dbtest.Database, subject string) pgtype.UUID {
	t.Helper()
	u, err := d.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{
		OidcIssuer: "i", OidcSubject: subject, Email: subject + "@example.com", Name: subject,
	})
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func userAs(id pgtype.UUID, role auth.Role) auth.Principal {
	return auth.Principal{Kind: auth.KindUser, ID: id.String(), Role: role}
}

func personalFolderIDs(t *testing.T, d *dbtest.Database, owner pgtype.UUID) []pgtype.UUID {
	t.Helper()
	rows, err := d.SQLDB(t).QueryContext(t.Context(), `SELECT id FROM folders WHERE personal AND owner_id = $1`, owner.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []pgtype.UUID
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		var id pgtype.UUID
		if err := id.Scan(s); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestEveryNewUserGetsOnePersonalFolder(t *testing.T) {
	d := dbtest.New(t)
	a, b := addUser(t, d, "alice"), addUser(t, d, "bob")
	for _, owner := range []pgtype.UUID{a, b} {
		if got := len(personalFolderIDs(t, d, owner)); got != 1 {
			t.Fatalf("user %s has %d personal folders, want 1", owner, got)
		}
		var title, kind, createdBy string
		err := d.SQLDB(t).QueryRowContext(t.Context(),
			`SELECT title, created_by_kind, created_by_id::text FROM folders WHERE owner_id = $1`, owner.String()).Scan(&title, &kind, &createdBy)
		if err != nil {
			t.Fatal(err)
		}
		if title != personalTitle || kind != "user" || createdBy != owner.String() {
			t.Fatalf("personal folder = %q created by %s %s", title, kind, createdBy)
		}
	}
	addUser(t, d, "alice")
	if got := len(personalFolderIDs(t, d, a)); got != 1 {
		t.Fatalf("after signing in again user has %d personal folders, want 1", got)
	}
}

func TestPersonalFolderIsTheOwnersAlone(t *testing.T) {
	d := dbtest.New(t)
	access := NewAccess(d.DB)
	a, b, admin := addUser(t, d, "alice"), addUser(t, d, "bob"), addUser(t, d, "root")
	mine := personalFolderIDs(t, d, a)[0]

	levels, err := access.Levels(t.Context(), userAs(a, auth.RoleViewer))
	if err != nil {
		t.Fatal(err)
	}
	if got := levels.Of(mine); !got.Edit || !got.Create || got.Manage {
		t.Fatalf("a Viewer's abilities in their own folder = %+v, want edit and create without manage", got)
	}
	if folder, ok := levels.Personal(); !ok || folder.ID != mine || !folder.Personal {
		t.Fatalf("Personal() = %+v, %v", folder, ok)
	}
	created, err := access.CreationFolder(t.Context(), userAs(a, auth.RoleViewer), pgtype.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != mine {
		t.Fatalf("a Viewer's creation folder = %v, want their personal folder %v", created.ID, mine)
	}

	for name, other := range map[string]auth.Principal{"another user": userAs(b, auth.RoleEditor), "an admin": userAs(admin, auth.RoleAdmin)} {
		levels, err := access.Levels(t.Context(), other)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range levels.Visible() {
			if id == mine {
				t.Fatalf("%s sees someone else's personal folder", name)
			}
		}
	}
}

func TestEditorsStillCreateInGeneral(t *testing.T) {
	d := dbtest.New(t)
	editor := addUser(t, d, "eve")
	got, err := NewAccess(d.DB).CreationFolder(t.Context(), userAs(editor, auth.RoleEditor), pgtype.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.General {
		t.Fatalf("an Editor's creation folder = %+v, want General", got)
	}
}

func TestServiceAccountsHaveNoPersonalFolder(t *testing.T) {
	d := dbtest.New(t)
	addUser(t, d, "alice")
	var keyID string
	err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT gen_random_uuid()::text`).Scan(&keyID)
	if err != nil {
		t.Fatal(err)
	}
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: keyID, Role: auth.RoleEditor}
	access := NewAccess(d.DB)
	levels, err := access.Levels(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := levels.Personal(); ok {
		t.Fatal("a service account has a personal folder")
	}
	for _, f := range levels.Folders() {
		if f.Personal {
			t.Fatalf("a service account sees the personal folder %v", f.ID)
		}
	}
	folder, err := access.CreationFolder(t.Context(), key, pgtype.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	if !folder.General {
		t.Fatalf("a key's creation folder = %+v, want General", folder)
	}
}

func TestOrphanedPersonalFolderIsAdminOnly(t *testing.T) {
	d := dbtest.New(t)
	access := NewAccess(d.DB)
	a, admin, editor := addUser(t, d, "alice"), addUser(t, d, "root"), addUser(t, d, "eve")
	mine := personalFolderIDs(t, d, a)[0]
	d.Exec(t, `DELETE FROM users WHERE id = $1`, a)

	var orphaned bool
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT owner_id IS NULL FROM folders WHERE id = $1`, mine.String()).Scan(&orphaned); err != nil {
		t.Fatal(err)
	}
	if !orphaned {
		t.Fatal("deleting the owner kept owner_id")
	}
	levels, err := access.Levels(t.Context(), userAs(admin, auth.RoleAdmin))
	if err != nil {
		t.Fatal(err)
	}
	if got := levels.Of(mine); !got.Manage {
		t.Fatalf("an Admin's abilities in an orphaned folder = %+v, want manage", got)
	}
	levels, err = access.Levels(t.Context(), userAs(editor, auth.RoleEditor))
	if err != nil {
		t.Fatal(err)
	}
	if got := levels.Of(mine); got.View {
		t.Fatalf("an Editor's abilities in an orphaned folder = %+v, want none", got)
	}
}

func TestDashboardInAPersonalFolderIsHiddenFromOthers(t *testing.T) {
	d := dbtest.New(t)
	access := NewAccess(d.DB)
	a, b := addUser(t, d, "alice"), addUser(t, d, "bob")
	mine := personalFolderIDs(t, d, a)[0]
	var raw string
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via)
		VALUES ('T', 1, 'user', $1::uuid, 'alice', 'direct') RETURNING id::text`, a.String()).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var dashboardID pgtype.UUID
	if err := dashboardID.Scan(raw); err != nil {
		t.Fatal(err)
	}
	foldertest.Move(t, d, dashboardID, mine)

	if _, err := access.Dashboard(t.Context(), userAs(b, auth.RoleEditor), dashboardID); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("another user's access error = %v, want ErrNoAccess", err)
	}
	got, err := access.Dashboard(t.Context(), userAs(a, auth.RoleViewer), dashboardID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Abilities.Edit {
		t.Fatalf("the owner's abilities = %+v, want edit", got.Abilities)
	}
}
