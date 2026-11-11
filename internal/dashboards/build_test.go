package dashboards

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func (r *rig) saveConcurrent(t *testing.T, dash pgtype.UUID, parent int, raw []byte) {
	t.Helper()
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, parent, raw, "concurrent edit"); err != nil {
		t.Fatal(err)
	}
}

func renamed(raw []byte, id, title string) []byte {
	return bytes.Replace(raw, fmt.Appendf(nil, `"title":%q`, id), fmt.Appendf(nil, `"title":%q`, title), 1)
}

func requeried(raw []byte, id string) []byte {
	return bytes.Replace(raw, fmt.Appendf(nil, `"title":%q,"gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]`, id),
		fmt.Appendf(nil, `"title":%q,"gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"rate(errors[5m])"}]`, id), 1)
}

func TestARebaseMovesTheBuildOntoADisjointNewerVersion(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "p1", "p2")
	id := r.runningRequest(t, dash)
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: requeried(panelsSpec("t", "p1", "p2"), "p1"),
	}); err != nil {
		t.Fatal(err)
	}
	r.saveConcurrent(t, dash, 1, renamed(panelsSpec("t", "p1", "p2"), "p2", "p2 renamed"))

	onto, rebased, err := r.service.RebaseBuild(t.Context(), testUser, id, buildAttempt)
	if err != nil || !rebased || onto != 2 {
		t.Fatalf("rebase = onto %d, rebased %v, err %v", onto, rebased, err)
	}
	build, _, err := r.service.Build(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	p1, _ := build.Spec.FindPanel("p1")
	p2, _ := build.Spec.FindPanel("p2")
	if build.BaseVersion != 2 || build.Saves != 1 || p1.Panel.Queries[0].Query != "rate(errors[5m])" || p2.Panel.Title != "p2 renamed" {
		t.Fatalf("build = base %d, saves %d, p1 query %q, p2 title %q", build.BaseVersion, build.Saves, p1.Panel.Queries[0].Query, p2.Panel.Title)
	}

	_, ver, _, err := r.service.CommitBuild(t.Context(), testUser, id, buildAttempt)
	if err != nil || ver.Number != 3 || ver.ParentVersion == nil || *ver.ParentVersion != 2 {
		t.Fatalf("commit after the rebase = version %d, err %v", ver.Number, err)
	}
}

func TestARebaseKeepsADashboardFieldTheBuildChangedWhenTheNewerVersionLeftItAlone(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "p1", "p2")
	id := r.runningRequest(t, dash)
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: panelsSpec("retitled", "p1", "p2"),
	}); err != nil {
		t.Fatal(err)
	}
	r.saveConcurrent(t, dash, 1, renamed(panelsSpec("t", "p1", "p2"), "p2", "p2 renamed"))

	onto, rebased, err := r.service.RebaseBuild(t.Context(), testUser, id, buildAttempt)
	if err != nil || !rebased || onto != 2 {
		t.Fatalf("rebase = onto %d, rebased %v, err %v", onto, rebased, err)
	}
	build, _, err := r.service.Build(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := build.Spec.FindPanel("p2")
	if build.Spec.Title != "retitled" || p2.Panel.Title != "p2 renamed" {
		t.Fatalf("build title %q, p2 title %q", build.Spec.Title, p2.Panel.Title)
	}
}

func TestARebaseRefusesABuildThatChangedWhatTheNewerVersionChanged(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "p1", "p2")
	id := r.runningRequest(t, dash)
	before := requeried(panelsSpec("t", "p1", "p2"), "p1")
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: before}); err != nil {
		t.Fatal(err)
	}
	r.saveConcurrent(t, dash, 1, renamed(panelsSpec("t", "p1", "p2"), "p1", "p1 renamed"))

	if _, rebased, err := r.service.RebaseBuild(t.Context(), testUser, id, buildAttempt); err != nil || rebased {
		t.Fatalf("rebase = %v, err %v, want no rebase", rebased, err)
	}
	build, _, err := r.service.Build(t.Context(), id)
	if err != nil || build.BaseVersion != 1 {
		t.Fatalf("build base = %d, err %v, want the build untouched", build.BaseVersion, err)
	}
	p1, _ := build.Spec.FindPanel("p1")
	if p1.Panel.Queries[0].Query != "rate(errors[5m])" || p1.Panel.Title != "p1" {
		t.Fatalf("build p1 = %+v, want the build's own edit", p1.Panel)
	}
}

func TestARebaseRefusesABuildPanelThatFailsOnTheNewerVersion(t *testing.T) {
	r, p := newProbeRig(t)
	dash := r.createDashboard(t, "p1", "p2")
	id := r.runningRequest(t, dash)
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: requeried(panelsSpec("t", "p1", "p2"), "p1"),
	}); err != nil {
		t.Fatal(err)
	}
	r.saveConcurrent(t, dash, 1, renamed(panelsSpec("t", "p1", "p2"), "p2", "p2 renamed"))
	p.setFail("rate(errors", true)

	if _, rebased, err := r.service.RebaseBuild(t.Context(), testUser, id, buildAttempt); err != nil || rebased {
		t.Fatalf("rebase = %v, err %v, want no rebase", rebased, err)
	}
}

func TestARebaseByALostAttemptWritesNothing(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "p1", "p2")
	id := r.runningRequest(t, dash)
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: requeried(panelsSpec("t", "p1", "p2"), "p1"),
	}); err != nil {
		t.Fatal(err)
	}
	r.saveConcurrent(t, dash, 1, renamed(panelsSpec("t", "p1", "p2"), "p2", "p2 renamed"))

	if _, _, err := r.service.RebaseBuild(t.Context(), testUser, id, buildAttempt+1); !errors.Is(err, ErrBuildLost) {
		t.Fatalf("err = %v, want ErrBuildLost", err)
	}
	if build, _, err := r.service.Build(t.Context(), id); err != nil || build.BaseVersion != 1 {
		t.Fatalf("build base = %d, err %v, want it untouched", build.BaseVersion, err)
	}
}
