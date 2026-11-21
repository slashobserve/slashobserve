package folders_test

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestContentOutsideAnAdminsOnlyFolderIsReadAsEveryoneReadsIt(t *testing.T) {
	admin := auth.Principal{Kind: auth.KindUser, ID: "a", Role: auth.RoleAdmin}
	for _, c := range []struct {
		name   string
		access datasource.Access
		folder folders.Folder
		reads  bool
	}{
		{"an Admin in a shared folder", datasource.Access{By: admin}, folders.Folder{}, false},
		{"an Admin in a personal folder", datasource.Access{By: admin}, folders.Folder{Personal: true}, false},
		{"the system in a shared folder", datasource.Access{System: true}, folders.Folder{}, false},
		{"an Admin in an Admins-only folder", datasource.Access{By: admin}, folders.Folder{AdminsOnly: true}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := folders.ReadingIn(datasource.WithAccess(t.Context(), c.access), c.folder)
			if got := datasource.ReadsRestricted(ctx); got != c.reads {
				t.Fatalf("reads restricted = %v, want %v", got, c.reads)
			}
			if a, _ := datasource.AccessFrom(ctx); a.By.ID != c.access.By.ID {
				t.Fatalf("principal = %+v, want %+v", a.By, c.access.By)
			}
		})
	}
	if _, ok := datasource.AccessFrom(folders.ReadingIn(t.Context(), folders.Folder{})); ok {
		t.Fatal("a context without access gained one")
	}
}

func TestReadingAnAdminsOnlyFolderMarksOnlyARunThatMayReadIt(t *testing.T) {
	admin := auth.Principal{Kind: auth.KindUser, ID: "a", Role: auth.RoleAdmin}
	for _, c := range []struct {
		name   string
		access datasource.Access
		folder folders.Folder
		marked bool
	}{
		{"an Admin's private run in an Admins-only folder", datasource.Access{By: admin}, folders.Folder{AdminsOnly: true}, true},
		{"an Admin's run for Admins in an Admins-only folder", datasource.Access{By: admin, Audience: datasource.AudienceAdmins}, folders.Folder{AdminsOnly: true}, true},
		{"an Admin's private run in a shared folder", datasource.Access{By: admin}, folders.Folder{}, false},
		{"the system in an Admins-only folder", datasource.Access{System: true}, folders.Folder{AdminsOnly: true}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx, marked := datasource.NotingRestrictedReads(datasource.WithAccess(t.Context(), c.access))
			folders.Read(ctx, c.folder)
			if marked() != c.marked {
				t.Fatalf("marked = %v, want %v", marked(), c.marked)
			}
		})
	}
}
