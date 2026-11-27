package dashboards

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestANewDashboardBuildCommitsIntoTheFolderItNamedAndAnUnnamedOneIntoMyDashboards(t *testing.T) {
	r, _ := newProbeRig(t)
	ada := r.addPerson(t, "ada")
	general := r.generalFolder(t)

	named := r.runningRequestFor(t, ada)
	if build, err := r.service.SaveBuild(reading(t.Context()), ada.principal, BuildSave{RequestID: named, ClaimEpoch: buildEpoch, FolderID: general, Spec: panelsSpec("t", "p1")}); err != nil || build.FolderID != general {
		t.Fatalf("save with a folder: folder %v, err %v", build.FolderID, err)
	}
	if again, err := r.service.SaveBuild(reading(t.Context()), ada.principal, BuildSave{RequestID: named, ClaimEpoch: buildEpoch, Spec: panelsSpec("t", "p1", "p2")}); err != nil || again.FolderID != general {
		t.Fatalf("a later save without a folder: folder %v, err %v", again.FolderID, err)
	}
	commit, err := r.service.CommitBuild(reading(t.Context()), ada.principal, named, buildEpoch)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(commit.Dashboard.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.folderOfDashboard(t, id); got != general {
		t.Fatalf("the dashboard landed in %v, want the named folder %v", got, general)
	}

	unnamed := r.runningRequestFor(t, ada)
	r.saveBuildAs(t, ada, unnamed, "p1")
	commit, err = r.service.CommitBuild(reading(t.Context()), ada.principal, unnamed, buildEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if id, err = db.ParseUUID(commit.Dashboard.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.folderOfDashboard(t, id); got != ada.personal {
		t.Fatalf("a build with no folder landed in %v, want My Dashboards %v", got, ada.personal)
	}
}

func TestANewDashboardBuildRefusesAFolderTheRequesterCannotCreateIn(t *testing.T) {
	r, _ := newProbeRig(t)
	ada := r.addPerson(t, "ada")
	restricted := foldertest.Restricted(t, r.db, "Payroll", "payroll-team")
	missing := pgtype.UUID{Bytes: [16]byte{9, 9, 9}, Valid: true}
	request := r.runningRequestFor(t, ada)
	for name, tc := range map[string]struct {
		by     bool
		folder pgtype.UUID
		want   error
	}{
		"a folder the requester cannot see":          {folder: restricted, want: ErrFolderNotFound},
		"a folder that does not exist":               {folder: missing, want: ErrFolderNotFound},
		"a folder the requester can only comment in": {by: true, folder: r.generalFolder(t), want: folders.ErrForbidden},
	} {
		by := ada.principal
		if tc.by {
			by = commenterNoGrant
		}
		_, err := r.service.SaveBuild(reading(t.Context()), by, BuildSave{RequestID: request, ClaimEpoch: buildEpoch, FolderID: tc.folder, Spec: panelsSpec("t", "p1")})
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", name, err, tc.want)
		}
	}
	if _, found, err := r.service.Build(reading(t.Context()), request); err != nil || found {
		t.Fatalf("a refused folder left a build: found = %v, err %v", found, err)
	}
}
