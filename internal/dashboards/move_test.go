package dashboards

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

type person struct {
	principal auth.Principal
	personal  pgtype.UUID
}

func (r *rig) addPerson(t *testing.T, subject string) person {
	t.Helper()
	u, err := r.db.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{
		OidcIssuer: "i", OidcSubject: subject, Email: subject + "@example.com", Name: subject,
	})
	if err != nil {
		t.Fatal(err)
	}
	var personal string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM folders WHERE owner_id = $1`, u.ID.String()).Scan(&personal); err != nil {
		t.Fatal(err)
	}
	folder, err := db.ParseUUID(personal)
	if err != nil {
		t.Fatal(err)
	}
	return person{
		principal: auth.Principal{Kind: auth.KindUser, ID: u.ID.String(), Name: subject, Email: subject + "@example.com", Role: auth.RoleEditor},
		personal:  folder,
	}
}

func (r *rig) createAs(t *testing.T, p person) pgtype.UUID {
	t.Helper()
	dash, _, err := r.service.Create(t.Context(), p.principal, pgtype.UUID{}, panelsSpec("mine", "rps"), "create")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) folderOfDashboard(t *testing.T, id pgtype.UUID) pgtype.UUID {
	t.Helper()
	folder, err := r.db.Queries().DashboardFolder(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return folder
}

func TestOnlyTheCreatorMovesADashboardIntoAPersonalFolder(t *testing.T) {
	r := newRig(t, false)
	ada, bob := r.addPerson(t, "ada"), r.addPerson(t, "bob")
	dash := r.createAs(t, ada)
	general := r.folderOfDashboard(t, dash)

	if _, _, err := r.service.move(t.Context(), bob.principal, dash, bob.personal); !errors.Is(err, folders.ErrNotCreator) {
		t.Fatalf("another user's move into their own personal folder: error = %v, want ErrNotCreator", err)
	}
	if got := r.folderOfDashboard(t, dash); got != general {
		t.Fatalf("a refused move changed the folder to %v", got)
	}

	moved, _, err := r.service.move(t.Context(), ada.principal, dash, ada.personal)
	if err != nil {
		t.Fatal(err)
	}
	if !moved.Folder.Personal || moved.Folder.ID != ada.personal {
		t.Fatalf("moved dashboard folder = %+v, want Ada's personal folder", moved.Folder)
	}
	if got := r.folderOfDashboard(t, dash); got != ada.personal {
		t.Fatalf("folder after the creator's move = %v, want %v", got, ada.personal)
	}

	if _, _, err := r.service.move(t.Context(), ada.principal, dash, general); err != nil {
		t.Fatalf("moving out of a personal folder: %v", err)
	}
	if got := r.folderOfDashboard(t, dash); got != general {
		t.Fatalf("folder after moving out = %v, want General %v", got, general)
	}
}

func TestEachPersonMovesTheirOwnDashboardIntoTheirPersonalFolder(t *testing.T) {
	r := newRig(t, false)
	bob := r.addPerson(t, "bob")
	dash := r.createAs(t, bob)
	if _, _, err := r.service.move(t.Context(), bob.principal, dash, bob.personal); err != nil {
		t.Fatal(err)
	}
	if got := r.folderOfDashboard(t, dash); got != bob.personal {
		t.Fatalf("folder = %v, want %v", got, bob.personal)
	}
}
