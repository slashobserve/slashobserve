package folders

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestAnAdminsOnlyFolderIsCreatedRestrictedInOneStepAndRecorded(t *testing.T) {
	d := dbtest.New(t)
	access := NewAccess(d.DB)
	id, err := access.CreateAdminsOnlyFolder(t.Context(), rootAdmin, "Audit")
	if err != nil {
		t.Fatal(err)
	}
	row, err := d.Queries().GetFolder(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !AdminsOnly(row) || row.Title != "Audit" {
		t.Fatalf("folder = %q admins only %v, want Audit only for Admins", row.Title, row.AdminsOnly)
	}
	events := storedEvents(t, d)
	if len(events) != 1 || events[0].Action != "folder.create" || events[0].TargetID != id.String() {
		t.Fatalf("events = %+v, want one folder.create of %s", events, id)
	}
	if _, err := access.CreateAdminsOnlyFolder(t.Context(), rootAdmin, "audit"); !errors.Is(err, ErrFolderExists) {
		t.Fatalf("second create = %v, want ErrFolderExists", err)
	}
	editor := auth.Principal{Kind: auth.KindUser, ID: "00000000-0000-4000-8000-0000000000ed", Name: "ed", Role: auth.RoleEditor}
	if _, err := access.CreateAdminsOnlyFolder(t.Context(), editor, "Payroll"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("an Editor's create = %v, want ErrForbidden", err)
	}
}
