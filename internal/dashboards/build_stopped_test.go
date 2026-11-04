package dashboards

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const buildAttempt = 1

func (r *rig) runningRequest(t *testing.T, dash pgtype.UUID) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if dash.Valid {
		id = r.queueRevision(t, dash)
	} else {
		row, err := r.revisions.SubmitObserve(t.Context(), testUser, "build it")
		if err != nil {
			t.Fatal(err)
		}
		id = row.ID
	}
	claimed, err := r.db.Queries().ClaimAgentRequest(t.Context(), dbsqlc.ClaimAgentRequestParams{LeaseSeconds: 60, MaxAttempts: 2})
	if err != nil || claimed.ID != id || claimed.Attempts != buildAttempt {
		t.Fatalf("claimed %v attempt %d, want %v attempt %d, err %v", claimed.ID, claimed.Attempts, id, buildAttempt, err)
	}
	return id
}

func (r *rig) failRequest(t *testing.T, id pgtype.UUID) {
	t.Helper()
	_, err := r.db.Queries().FailAgentRequest(t.Context(), dbsqlc.FailAgentRequestParams{
		Error: "stopped", FailureCode: pgtype.Text{String: "no_progress", Valid: true}, ID: id, Attempts: buildAttempt,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (r *rig) saveBuild(t *testing.T, id, dash pgtype.UUID, baseVersion int, panelIDs ...string) Build {
	t.Helper()
	build, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: baseVersion, Spec: panelsSpec("t", panelIDs...),
	})
	if err != nil {
		t.Fatal(err)
	}
	return build
}

func (r *rig) requestRow(t *testing.T, id pgtype.UUID) dbsqlc.AgentRequest {
	t.Helper()
	row, err := r.db.Queries().GetAgentRequestByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestTwoBuildSavesAndACommitMakeOneVersionWithBothPanels(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.runningRequest(t, pgtype.UUID{})

	if build := r.saveBuild(t, id, pgtype.UUID{}, 0, "a"); build.Saves != 1 {
		t.Fatalf("saves after the first = %d", build.Saves)
	}
	if build := r.saveBuild(t, id, pgtype.UUID{}, 0, "a", "b"); build.Saves != 2 {
		t.Fatalf("saves after the second = %d", build.Saves)
	}
	if _, found, err := r.service.Build(t.Context(), id); err != nil || !found {
		t.Fatalf("build found = %v, err %v", found, err)
	}

	dash, ver, replayed, err := r.service.CommitBuild(t.Context(), testUser, id, buildAttempt)
	if err != nil || replayed || ver.Number != 1 || len(ver.Spec.Panels) != 2 {
		t.Fatalf("commit: version %d, panels %d, replayed %v, err %v", ver.Number, len(ver.Spec.Panels), replayed, err)
	}
	if dash.LatestVersion != 1 {
		t.Fatalf("latest version = %d", dash.LatestVersion)
	}
	if _, found, err := r.service.Build(t.Context(), id); err != nil || found {
		t.Fatalf("the build survived its commit: found = %v, err %v", found, err)
	}
}

func TestACommitOfAnAlreadyCommittedRequestReplaysItsVersion(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.runningRequest(t, pgtype.UUID{})
	r.saveBuild(t, id, pgtype.UUID{}, 0, "a")
	_, first, _, err := r.service.CommitBuild(t.Context(), testUser, id, buildAttempt)
	if err != nil {
		t.Fatal(err)
	}

	_, again, replayed, err := r.service.CommitBuild(t.Context(), testUser, id, buildAttempt)
	if err != nil || !replayed || again.Number != first.Number {
		t.Fatalf("second commit: version %d, replayed %v, err %v", again.Number, replayed, err)
	}
}

func TestACommitWithoutABuildIsNotFound(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.runningRequest(t, pgtype.UUID{})

	if _, _, _, err := r.service.CommitBuild(t.Context(), testUser, id, buildAttempt); !errors.Is(err, ErrBuildNotFound) {
		t.Fatalf("err = %v, want ErrBuildNotFound", err)
	}
}

func TestAStoppedBuildIsSavedAsOneVersionAndTheRequestStaysFailed(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.runningRequest(t, pgtype.UUID{})
	r.saveBuild(t, id, pgtype.UUID{}, 0, "a", "b")
	r.failRequest(t, id)

	dash, ver, replayed, err := r.service.SaveStoppedBuild(t.Context(), testUser, id)
	if err != nil || replayed || ver.Number != 1 || len(ver.Spec.Panels) != 2 {
		t.Fatalf("save: version %d, panels %d, replayed %v, err %v", ver.Number, len(ver.Spec.Panels), replayed, err)
	}
	row := r.requestRow(t, id)
	if row.Status != "failed" || !row.BuildSavedAt.Valid || row.ResultVersion.Int32 != 1 || !row.DashboardID.Valid || dash.ID == "" {
		t.Fatalf("request = status %q, saved %v, result version %d, dashboard %v", row.Status, row.BuildSavedAt.Valid, row.ResultVersion.Int32, row.DashboardID)
	}
	if _, found, err := r.service.Build(t.Context(), id); err != nil || found {
		t.Fatalf("the build survived the save: found = %v, err %v", found, err)
	}
}

func TestSavingAStoppedBuildAgainReplaysTheVersion(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.runningRequest(t, pgtype.UUID{})
	r.saveBuild(t, id, pgtype.UUID{}, 0, "a")
	r.failRequest(t, id)
	_, first, _, err := r.service.SaveStoppedBuild(t.Context(), testUser, id)
	if err != nil {
		t.Fatal(err)
	}

	_, again, replayed, err := r.service.SaveStoppedBuild(t.Context(), testUser, id)
	if err != nil || !replayed || again.Number != first.Number {
		t.Fatalf("second save: version %d, replayed %v, err %v", again.Number, replayed, err)
	}
}

func TestAStoppedBuildOfARequestThatIsStillRunningIsNotSavable(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.runningRequest(t, pgtype.UUID{})
	r.saveBuild(t, id, pgtype.UUID{}, 0, "a")

	if _, _, _, err := r.service.SaveStoppedBuild(t.Context(), testUser, id); !errors.Is(err, ErrBuildNotSavable) {
		t.Fatalf("err = %v, want ErrBuildNotSavable", err)
	}
	if _, found, err := r.service.Build(t.Context(), id); err != nil || !found {
		t.Fatalf("the refused save removed the build: found = %v, err %v", found, err)
	}
	if row := r.requestRow(t, id); row.BuildSavedAt.Valid || row.DashboardID.Valid {
		t.Fatalf("the refused save marked the request: %+v", row)
	}
}

func TestAStoppedBuildWithoutPanelsIsEmpty(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.runningRequest(t, pgtype.UUID{})
	_, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt,
		Spec: []byte(`{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"panels":[{"id":"row0","type":"row","title":"Row","gridPos":{"x":0,"y":0,"w":24,"h":1},"panels":[]}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	r.failRequest(t, id)

	if _, _, _, err := r.service.SaveStoppedBuild(t.Context(), testUser, id); !errors.Is(err, ErrBuildEmpty) {
		t.Fatalf("err = %v, want ErrBuildEmpty", err)
	}
}

func TestAStoppedBuildOfAMovedOnDashboardIsAVersionConflictAndKeepsTheBuild(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "rps")
	id := r.runningRequest(t, dash)
	r.saveBuild(t, id, dash, 1, "rps", "p99")
	r.failRequest(t, id)
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "other"), "someone else"); err != nil {
		t.Fatal(err)
	}

	_, _, _, err := r.service.SaveStoppedBuild(t.Context(), testUser, id)
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.Latest != 2 {
		t.Fatalf("err = %v, want a ConflictError at version 2", err)
	}
	if _, found, err := r.service.Build(t.Context(), id); err != nil || !found {
		t.Fatalf("the conflict removed the build: found = %v, err %v", found, err)
	}
	if latest, err := r.service.LatestVersion(t.Context(), dash); err != nil || latest != 2 {
		t.Fatalf("latest = %d, err %v", latest, err)
	}
}

func TestAStoppedRevisionBuildOnTheBaseVersionIsTheNextVersion(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "rps")
	id := r.runningRequest(t, dash)
	r.saveBuild(t, id, dash, 1, "rps", "p99")
	r.failRequest(t, id)

	_, ver, replayed, err := r.service.SaveStoppedBuild(t.Context(), testUser, id)
	if err != nil || replayed || ver.Number != 2 || len(ver.Spec.Panels) != 2 {
		t.Fatalf("save: version %d, panels %d, replayed %v, err %v", ver.Number, len(ver.Spec.Panels), replayed, err)
	}
}
