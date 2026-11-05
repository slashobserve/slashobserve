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

func TestDeletingAPersonDeletesTheirPersonalFolderAndItsDashboards(t *testing.T) {
	d := dbtest.New(t)
	access := NewAccess(d.DB)
	a, b, admin := addUser(t, d, "alice"), addUser(t, d, "bob"), addUser(t, d, "root")
	mine, theirs := personalFolderIDs(t, d, a)[0], personalFolderIDs(t, d, b)[0]
	insert := func(folder pgtype.UUID) string {
		var id string
		if err := d.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, folder_id, created_by_kind, created_by_id, created_by_name, created_via)
			VALUES ('T', 1, $1::uuid, 'user', $2::uuid, 'alice', 'direct') RETURNING id::text`, folder.String(), a.String()).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	var generalID pgtype.UUID
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT id FROM folders WHERE general`).Scan(&generalID); err != nil {
		t.Fatal(err)
	}
	private, shared, bobs := insert(mine), insert(generalID), insert(theirs)

	d.Exec(t, `DELETE FROM users WHERE id = $1`, a)

	exists := func(table, id string) bool {
		var n int
		if err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM `+table+` WHERE id = $1::uuid`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if exists("folders", mine.String()) || exists("dashboards", private) {
		t.Fatal("the deleted person's personal folder or dashboard is still there")
	}
	if !exists("dashboards", shared) || !exists("folders", theirs.String()) || !exists("dashboards", bobs) {
		t.Fatal("deleting a person removed a shared dashboard or another person's folder or dashboard")
	}
	levels, err := access.Levels(t.Context(), userAs(admin, auth.RoleAdmin))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range levels.Visible() {
		if id == mine || id == theirs {
			t.Fatalf("an Admin sees the personal folder %v", id)
		}
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
