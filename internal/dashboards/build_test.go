package dashboards

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/grantmode"
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

	commit, err := r.service.CommitBuild(t.Context(), testUser, id, buildAttempt)
	ver := commit.Version
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

const proposalID = "0b0b0b0b-0b0b-4b0b-8b0b-0b0b0b0b0b0b"

func (r *rig) runningRevision(t *testing.T) (dash, id pgtype.UUID) {
	t.Helper()
	dash = r.createDashboard(t, "p1", "p2")
	id = r.runningRequest(t, dash)
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: requeried(panelsSpec("t", "p1", "p2"), "p1"),
	}); err != nil {
		t.Fatal(err)
	}
	return dash, id
}

func (r *rig) proposeThroughTheDatabase(seen *[]ProposeInput) {
	r.service.SetProposer(func(ctx context.Context, _ auth.Principal, in ProposeInput) (ProposalRef, error) {
		*seen = append(*seen, in)
		err := r.db.InTx(ctx, func(q *dbsqlc.Queries) error { return in.InTx(q) })
		return ProposalRef{ID: proposalID, Revision: 1, Opened: true}, err
	})
}

func TestACommentersBuildCommitsAsAProposalAndDeletesTheBuild(t *testing.T) {
	r, _ := newProbeRig(t)
	dash, id := r.runningRevision(t)
	var seen []ProposeInput
	r.proposeThroughTheDatabase(&seen)

	commit, err := r.service.CommitBuild(t.Context(), commenterNoGrant, id, buildAttempt)
	if err != nil || commit.Proposal == nil || commit.Proposal.ID != proposalID || commit.Version.Number != 0 || commit.Replayed {
		t.Fatalf("commit = proposal %+v, version %d, replayed %v, err %v", commit.Proposal, commit.Version.Number, commit.Replayed, err)
	}
	if commit.Dashboard.ID != dash.String() || commit.Dashboard.Abilities.Edit || !commit.Dashboard.Abilities.Comment {
		t.Fatalf("dashboard %s with abilities %+v", commit.Dashboard.ID, commit.Dashboard.Abilities)
	}
	if len(seen) != 1 || seen[0].DashboardID != dash || seen[0].BaseVersion != 1 || seen[0].RequestID != id || !seen[0].Partial || seen[0].Spec == nil {
		t.Fatalf("proposer saw %+v", seen)
	}
	if _, found, err := r.service.Build(t.Context(), id); err != nil || found {
		t.Fatalf("the build survived its proposal: found = %v, err %v", found, err)
	}
	if got := r.latestNumber(t, dash); got != 1 {
		t.Fatalf("latest = %d, want no new version", got)
	}
}

func TestAnEditorsBuildCommitsAsAVersionUnlessTheRunActsInProposeMode(t *testing.T) {
	for _, tc := range []struct {
		name          string
		ctx           func(context.Context) context.Context
		wantProposal  bool
		wantVersion   int
		wantProposals int
	}{
		{"direct", func(ctx context.Context) context.Context { return ctx }, false, 2, 0},
		{"propose mode", func(ctx context.Context) context.Context { return grantmode.With(ctx, grantmode.Propose) }, true, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := newProbeRig(t)
			_, id := r.runningRevision(t)
			var seen []ProposeInput
			r.proposeThroughTheDatabase(&seen)

			commit, err := r.service.CommitBuild(tc.ctx(t.Context()), testUser, id, buildAttempt)
			if err != nil {
				t.Fatal(err)
			}
			if (commit.Proposal != nil) != tc.wantProposal || commit.Version.Number != tc.wantVersion || len(seen) != tc.wantProposals {
				t.Fatalf("proposal %+v, version %d, proposer calls %d, want proposal %v, version %d, calls %d", commit.Proposal, commit.Version.Number, len(seen), tc.wantProposal, tc.wantVersion, tc.wantProposals)
			}
		})
	}
}

func TestACommitByACallerWhoCannotEvenCommentOrWithoutAProposerIsForbiddenAndKeepsTheBuild(t *testing.T) {
	r, _ := newProbeRig(t)
	_, id := r.runningRevision(t)
	var seen []ProposeInput
	r.proposeThroughTheDatabase(&seen)

	if _, err := r.service.CommitBuild(t.Context(), viewerNoGrant, id, buildAttempt); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("a viewer: err = %v, want forbidden", err)
	}
	r.service.SetProposer(nil)
	if _, err := r.service.CommitBuild(t.Context(), commenterNoGrant, id, buildAttempt); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("a commenter with no proposer: err = %v, want forbidden", err)
	}
	if len(seen) != 0 {
		t.Fatalf("the proposer ran %d times", len(seen))
	}
	if _, found, err := r.service.Build(t.Context(), id); err != nil || !found {
		t.Fatalf("the build was lost: found = %v, err %v", found, err)
	}
}

func TestACommenterMayAddToARevisionBuildButAViewerMayNot(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "p1", "p2")
	id := r.runningRequest(t, dash)
	save := func(by auth.Principal) error {
		_, err := r.service.SaveBuild(t.Context(), by, BuildSave{
			RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: requeried(panelsSpec("t", "p1", "p2"), "p1"),
		})
		return err
	}
	if err := save(viewerNoGrant); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("a viewer: err = %v, want forbidden", err)
	}
	if err := save(commenterNoGrant); err != nil {
		t.Fatalf("a commenter: %v", err)
	}
}
