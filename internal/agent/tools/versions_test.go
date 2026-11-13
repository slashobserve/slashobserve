package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/grantmode"
)

func (r *accessRig) saveVersion(t *testing.T, id pgtype.UUID, parent int, title string) {
	t.Helper()
	_, err := r.service.Save(t.Context(), adminCaller, dashboards.SaveInput{DashboardID: id, Parent: parent, Spec: []byte(boardSpec(title)), Summary: "retitle to " + title})
	if err != nil {
		t.Fatal(err)
	}
}

func failureCode(t *testing.T, res Result) string {
	t.Helper()
	switch body := res.Content.(type) {
	case conflictOut:
		return body.Code
	default:
		return errorCodeOf(t, res)
	}
}

func (r *accessRig) callProposing(t *testing.T, by auth.Principal, tool, input string) Result {
	t.Helper()
	ctx := grantmode.With(auth.WithPrincipal(t.Context(), by), grantmode.Propose)
	res, err := r.registry.Call(ctx, tool, []byte(input))
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return res
}

func TestListDashboardVersionsReturnsNewestFirstAndHidesWhatTheCallerCannotSee(t *testing.T) {
	r := newAccessRig(t)
	b := r.seedHidden(t)
	r.saveVersion(t, b.open, 1, "Open board two")
	r.saveVersion(t, b.open, 2, "Open board three")

	res := r.call(t, viewerCaller, nameListDashboardVersions, fmt.Sprintf(`{"dashboard_id":%q,"limit":2}`, b.open.String()))
	out, ok := res.Content.(versionListOut)
	if res.IsError || !ok || len(out.Versions) != 2 || out.Versions[0].Version != 3 || out.Versions[1].Version != 2 || out.NextCursor == nil {
		t.Fatalf("first page = %s", textOf(t, res))
	}
	if out.Versions[0].Summary != "retitle to Open board three" || out.Versions[0].Author != adminCaller.Name || out.Versions[0].CreatedAt == "" || out.Versions[0].ParentVersion == nil || *out.Versions[0].ParentVersion != 2 {
		t.Errorf("newest version = %#v", out.Versions[0])
	}

	next := r.call(t, viewerCaller, nameListDashboardVersions, fmt.Sprintf(`{"dashboard_id":%q,"limit":2,"cursor":%q}`, b.open.String(), *out.NextCursor))
	rest, ok := next.Content.(versionListOut)
	if next.IsError || !ok || len(rest.Versions) != 1 || rest.Versions[0].Version != 1 || rest.NextCursor != nil {
		t.Fatalf("second page = %s", textOf(t, next))
	}

	if code := errorCode(t, r.call(t, viewerCaller, nameListDashboardVersions, fmt.Sprintf(`{"dashboard_id":%q}`, b.hidden.String()))); code != codeNotFound {
		t.Errorf("hidden dashboard answered %s, want not_found", code)
	}
	for name, input := range map[string]string{
		"bad cursor": fmt.Sprintf(`{"dashboard_id":%q,"cursor":"nope"}`, b.open.String()),
		"low limit":  fmt.Sprintf(`{"dashboard_id":%q,"limit":0}`, b.open.String()),
		"bad id":     `{"dashboard_id":"x"}`,
	} {
		if code := errorCode(t, r.call(t, viewerCaller, nameListDashboardVersions, input)); code != codeInvalidRequest {
			t.Errorf("%s answered %s, want invalid_request", name, code)
		}
	}
}

func TestDiffDashboardVersionsComparesTheLatestWithTheOneBeforeByDefault(t *testing.T) {
	r := newAccessRig(t)
	b := r.seedHidden(t)
	r.saveVersion(t, b.open, 1, "Renamed board")
	id := b.open.String()

	res := r.call(t, viewerCaller, nameDiffDashboardVersions, fmt.Sprintf(`{"dashboard_id":%q}`, id))
	out, ok := res.Content.(versionDiffOut)
	if res.IsError || !ok || out.From != 1 || out.To != 2 || len(out.Changes) == 0 || out.Truncated {
		t.Fatalf("default diff = %s", textOf(t, res))
	}
	if explicit := r.call(t, viewerCaller, nameDiffDashboardVersions, fmt.Sprintf(`{"dashboard_id":%q,"from":0,"to":1}`, id)).Content.(versionDiffOut); explicit.From != 0 || explicit.To != 1 {
		t.Errorf("explicit diff = %#v", explicit)
	}

	if code := errorCode(t, r.call(t, viewerCaller, nameDiffDashboardVersions, fmt.Sprintf(`{"dashboard_id":%q,"from":2,"to":1}`, id))); code != codeInvalidRequest {
		t.Errorf("from 2 to 1 answered %s, want invalid_request", code)
	}
	if code := errorCode(t, r.call(t, viewerCaller, nameDiffDashboardVersions, fmt.Sprintf(`{"dashboard_id":%q,"from":1,"to":9}`, id))); code != codeNotFound {
		t.Errorf("a missing version answered %s, want not_found", code)
	}
	if code := errorCode(t, r.call(t, viewerCaller, nameDiffDashboardVersions, fmt.Sprintf(`{"dashboard_id":%q}`, b.hidden.String()))); code != codeNotFound {
		t.Errorf("hidden dashboard answered %s, want not_found", code)
	}
}

func TestRestoreDashboardVersionSavesTheOldVersionAsANewOne(t *testing.T) {
	r := newAccessRig(t)
	b := r.seedHidden(t)
	r.saveVersion(t, b.open, 1, "Renamed board")
	id := b.open.String()

	res := r.call(t, editorCaller, nameRestoreDashboardVersion, fmt.Sprintf(`{"dashboard_id":%q,"version":1,"parent_version":2}`, id))
	saved, ok := res.Content.(savedOut)
	if res.IsError || !ok || saved.Version != 3 || saved.DashboardID != id {
		t.Fatalf("restore = %s", textOf(t, res))
	}
	versions, _, err := r.service.ListVersions(t.Context(), b.open, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := versions[0]; got.Number != 3 || got.Summary != "Restore version 1" || got.RestoredFrom == nil || *got.RestoredFrom != 1 {
		t.Errorf("newest version = %#v", got)
	}

	if code := failureCode(t, r.call(t, editorCaller, nameRestoreDashboardVersion, fmt.Sprintf(`{"dashboard_id":%q,"version":1,"parent_version":2}`, id))); code != codeVersionConflict {
		t.Errorf("a stale parent answered %s, want version_conflict", code)
	}
	if code := failureCode(t, r.call(t, viewerCaller, nameRestoreDashboardVersion, fmt.Sprintf(`{"dashboard_id":%q,"version":1,"parent_version":3}`, id))); code != codeForbiddenRole {
		t.Errorf("a viewer answered %s, want forbidden_role", code)
	}
	if code := failureCode(t, r.call(t, editorCaller, nameRestoreDashboardVersion, fmt.Sprintf(`{"dashboard_id":%q,"version":1,"parent_version":1}`, b.hidden.String()))); code != codeNotFound {
		t.Errorf("hidden dashboard answered %s, want not_found", code)
	}
	for name, input := range map[string]string{
		"no version": fmt.Sprintf(`{"dashboard_id":%q,"parent_version":3}`, id),
		"no parent":  fmt.Sprintf(`{"dashboard_id":%q,"version":1}`, id),
	} {
		if code := failureCode(t, r.call(t, editorCaller, nameRestoreDashboardVersion, input)); code != codeInvalidRequest {
			t.Errorf("%s answered %s, want invalid_request", name, code)
		}
	}
}

func TestRestoreAndMoveRefuseAConnectionInProposeMode(t *testing.T) {
	r := newAccessRig(t)
	id := r.seed(t, "Board")
	r.saveVersion(t, id, 1, "Board two")
	ops := r.newFolder(t, "Ops")

	restore := r.callProposing(t, editorCaller, nameRestoreDashboardVersion, fmt.Sprintf(`{"dashboard_id":%q,"version":1,"parent_version":2}`, id.String()))
	move := r.callProposing(t, editorCaller, nameMoveDashboard, fmt.Sprintf(`{"dashboard_id":%q,"folder_id":%q}`, id.String(), ops.String()))
	for name, res := range map[string]Result{"restore": restore, "move": move} {
		if code := errorCode(t, res); code != codeProposalRequired {
			t.Errorf("%s in propose mode answered %s, want proposal_required", name, code)
		}
	}
	dash, _, err := r.service.Get(context.Background(), id)
	if err != nil || dash.LatestVersion != 2 || dash.Folder.Title != "General" {
		t.Errorf("dashboard after the refusals = %#v, %v", dash, err)
	}
}
