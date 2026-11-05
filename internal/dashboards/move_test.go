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

func (r *rig) generalFolder(t *testing.T) pgtype.UUID {
	t.Helper()
	var general pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id FROM folders WHERE general`).Scan(&general); err != nil {
		t.Fatal(err)
	}
	return general
}

func (r *rig) createAs(t *testing.T, p person, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	dash, _, err := r.service.Create(t.Context(), p.principal, folder, panelsSpec("mine", "rps"), "create")
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

func TestNothingMovesIntoAPersonalFolder(t *testing.T) {
	r := newRig(t, false)
	ada, bob := r.addPerson(t, "ada"), r.addPerson(t, "bob")
	general := r.generalFolder(t)
	dash := r.createAs(t, ada, general)

	if _, _, err := r.service.move(t.Context(), ada.principal, dash, ada.personal); !errors.Is(err, folders.ErrMoveIntoPersonal) {
		t.Fatalf("the owner's move of their own dashboard into their personal folder: error = %v, want ErrMoveIntoPersonal", err)
	}
	if _, _, err := r.service.move(t.Context(), ada.principal, dash, bob.personal); !errors.Is(err, ErrFolderNotFound) {
		t.Fatalf("a move into another person's personal folder: error = %v, want ErrFolderNotFound", err)
	}
	if got := r.folderOfDashboard(t, dash); got != general {
		t.Fatalf("a refused move changed the folder to %v", got)
	}
}

func TestPublishingMovesAPrivateDashboardToASharedFolder(t *testing.T) {
	r := newRig(t, false)
	ada, bob := r.addPerson(t, "ada"), r.addPerson(t, "bob")
	dash := r.createAs(t, ada, pgtype.UUID{})
	if got := r.folderOfDashboard(t, dash); got != ada.personal {
		t.Fatalf("a new dashboard landed in %v, want Ada's personal folder %v", got, ada.personal)
	}
	if _, err := r.service.need(t.Context(), bob.principal, dash, folders.NeedView); !errors.Is(err, folders.ErrNoAccess) {
		t.Fatalf("another Editor's access to a private dashboard: error = %v, want ErrNoAccess", err)
	}
	_, before, err := r.service.Get(t.Context(), dash)
	if err != nil {
		t.Fatal(err)
	}

	general := r.generalFolder(t)
	moved, after, err := r.service.move(t.Context(), ada.principal, dash, general)
	if err != nil {
		t.Fatal(err)
	}
	if moved.ID != dash.String() || moved.Folder.ID != general || after.Number != before.Number {
		t.Fatalf("published dashboard = %s in %v at version %d, want %s in General at version %d", moved.ID, moved.Folder.ID, after.Number, dash, before.Number)
	}
	if _, err := r.service.need(t.Context(), bob.principal, dash, folders.NeedView); err != nil {
		t.Fatalf("another Editor's access after publishing: %v", err)
	}
}
