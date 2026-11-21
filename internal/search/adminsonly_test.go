package search

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestAnAdminsOnlyFolderIsHiddenFromItsNonAdminCreator(t *testing.T) {
	r := newRig(t)
	folder := foldertest.AdminsOnlyMadeBy(t, r.db, "Payroll", editor)
	r.dashboard(t, "quokka dashboard", folder, "")
	r.finding(t, findingIn{title: "quokka finding", markdown: "quokka", folder: folder})

	if got := r.search(t, editor, "quokka"); len(got) != 0 {
		t.Fatalf("the folder's creator finds %v, want nothing", titles(got))
	}
	if got := r.search(t, payroll, "quokka"); len(got) != 0 {
		t.Fatalf("a group member finds %v, want nothing", titles(got))
	}
	if got := len(r.search(t, admin, "quokka")); got != 2 {
		t.Fatalf("an Admin finds %d results, want 2", got)
	}
}
