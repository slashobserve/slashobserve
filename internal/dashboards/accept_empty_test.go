package dashboards

import (
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const quietWindow = time.Hour

func quietPatch() spec.PanelPatch {
	return upsert(2, panelDef{id: "quiet", query: "quiet"})
}

func TestSavePatchRefusesAnEmptyPanelUnlessAPersonAcceptsIt(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(2)})
	p.setEmptyBelow("quiet", quietWindow)

	_, err := r.service.SavePatch(reading(t.Context()), testUser, PatchInput{DashboardID: id, Parent: 1, Patch: quietPatch(), Summary: "Add quiet"})
	invalid := wantPatchError(t, err)
	if len(invalid.Issues) != 1 || invalid.Issues[0].Code != queryproxy.CodeQueryEmpty {
		t.Fatalf("issues %v, want the empty panel", invalid.Issues)
	}

	res, err := r.service.SavePatch(reading(t.Context()), testUser, PatchInput{DashboardID: id, Parent: 1, Patch: quietPatch(), Summary: "Add quiet", AcceptEmpty: true})
	if err != nil {
		t.Fatal(err)
	}
	if ids := panelIDs(res.Version.Unverified); len(ids) != 1 || ids[0] != "quiet" {
		t.Fatalf("unverified %v, want only quiet", ids)
	}
	if kept, _ := res.Version.Unverified.Panel("quiet"); len(kept.Issues) != 1 || kept.Issues[0].Code != queryproxy.CodeQueryEmpty {
		t.Fatalf("issues kept %v, want the empty result", kept.Issues)
	}
}

func TestSavePatchAcceptEmptyIsForSignedInPeopleOnly(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(2)})
	p.setEmptyBelow("quiet", quietWindow)
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: "33333333-3333-4333-8333-333333333333", Name: "ci", Role: auth.RoleEditor}

	_, err := r.service.SavePatch(reading(t.Context()), key, PatchInput{DashboardID: id, Parent: 1, Patch: quietPatch(), Summary: "Add quiet", AcceptEmpty: true})
	if !errors.Is(err, ErrUserSessionRequired) {
		t.Fatalf("err = %v, want ErrUserSessionRequired", err)
	}
	if latest, err := r.service.store.latestVersion(reading(t.Context()), id); err != nil || latest != 1 {
		t.Fatalf("latest version %d (%v), want 1", latest, err)
	}
}

func TestSavePatchAcceptEmptyStillRefusesAFailingQuery(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(2)})
	p.setFail("quiet", true)

	_, err := r.service.SavePatch(reading(t.Context()), testUser, PatchInput{DashboardID: id, Parent: 1, Patch: quietPatch(), Summary: "Add quiet", AcceptEmpty: true})
	invalid := wantPatchError(t, err)
	if len(invalid.Issues) == 0 || invalid.Issues[0].Code == queryproxy.CodeQueryEmpty {
		t.Fatalf("issues %v, want the failure", invalid.Issues)
	}
}

func TestAnEmptyPanelAPersonKeptDoesNotBlockLaterSaves(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(2)})
	p.setEmptyBelow("quiet", quietWindow)
	kept, err := r.service.SavePatch(reading(t.Context()), testUser, PatchInput{DashboardID: id, Parent: 1, Patch: quietPatch(), Summary: "Add quiet", AcceptEmpty: true})
	if err != nil {
		t.Fatal(err)
	}

	raw := r.mergedPatch(t, id, kept.Version.Number, upsert(0, panelDef{id: "p0", query: "edited"}))
	_, ver, _, err := r.service.AddVersion(reading(t.Context()), testUser, id, kept.Version.Number, raw, "Edit p0")
	if err != nil {
		t.Fatalf("a full save after the kept empty panel: %v", err)
	}
	if ids := panelIDs(ver.Unverified); len(ids) != 1 || ids[0] != "quiet" {
		t.Fatalf("unverified %v, want quiet carried", ids)
	}
}
