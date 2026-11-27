package dashboards

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func (r *rig) markReadRestricted(t *testing.T, request pgtype.UUID) {
	t.Helper()
	r.db.Exec(t, `UPDATE agent_requests SET read_restricted = true WHERE id = $1`, request)
}

func TestABuildThatReadAdminOnlyDataIsSavedOnlyIntoAFolderOnlyAdminsRead(t *testing.T) {
	r, _ := newProbeRig(t)
	ada := r.addPerson(t, "ada")
	ada.principal.Role = auth.RoleAdmin
	locked := r.folder(t, "Audit", true)

	for name, folder := range map[string]pgtype.UUID{"the General folder": r.generalFolder(t), "My Dashboards": {}} {
		request := r.runningRequestFor(t, ada)
		r.markReadRestricted(t, request)
		_, err := r.service.SaveBuild(reading(t.Context()), ada.principal, BuildSave{RequestID: request, ClaimEpoch: buildEpoch, FolderID: folder, Spec: panelsSpec("t", "p1")})
		if !errors.Is(err, folders.ErrRestrictedAnswer) {
			t.Fatalf("%s: err = %v, want the restricted answer refusal", name, err)
		}
		if _, found, err := r.service.Build(reading(t.Context()), request); err != nil || found {
			t.Fatalf("%s: a refused save left a build: found = %v, err %v", name, found, err)
		}
	}

	request := r.runningRequestFor(t, ada)
	r.markReadRestricted(t, request)
	if _, err := r.service.SaveBuild(reading(t.Context()), ada.principal, BuildSave{RequestID: request, ClaimEpoch: buildEpoch, FolderID: locked, Spec: panelsSpec("t", "p1")}); err != nil {
		t.Fatalf("save into a folder only Admins read: %v", err)
	}
	if _, err := r.service.CommitBuild(reading(t.Context()), ada.principal, request, buildEpoch); err != nil {
		t.Fatalf("commit into a folder only Admins read: %v", err)
	}
}

func TestABuildThatReadAdminOnlyDataAfterItsLastSaveIsNotCommittedWhereNonAdminsRead(t *testing.T) {
	r, _ := newProbeRig(t)
	ada := r.addPerson(t, "ada")
	ada.principal.Role = auth.RoleAdmin
	request := r.runningRequestFor(t, ada)
	if _, err := r.service.SaveBuild(reading(t.Context()), ada.principal, BuildSave{RequestID: request, ClaimEpoch: buildEpoch, FolderID: r.generalFolder(t), Spec: panelsSpec("t", "p1")}); err != nil {
		t.Fatal(err)
	}
	r.markReadRestricted(t, request)
	if _, err := r.service.CommitBuild(reading(t.Context()), ada.principal, request, buildEpoch); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("err = %v, want the restricted answer refusal", err)
	}
}
