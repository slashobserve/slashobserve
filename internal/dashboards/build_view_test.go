package dashboards

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) personalRevisionBuild(t *testing.T, owner person) pgtype.UUID {
	t.Helper()
	dash, _, err := r.service.Create(t.Context(), owner.principal, pgtype.UUID{}, panelsSpec("t", "rps"), "create")
	if err != nil {
		t.Fatal(err)
	}
	dashID, err := db.ParseUUID(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	row, err := r.revisions.Submit(t.Context(), revisions.EnqueueParams{
		DashboardID: dashID, Source: revisions.SourcePrompt, Prompt: "revise", BaseVersion: 1, By: owner.principal,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := r.db.Queries().ClaimAgentRequest(t.Context(), dbsqlc.ClaimAgentRequestParams{LeaseSeconds: 60, MaxAttempts: 2, AssignedKind: "builtin", ClaimedBy: "builtin", FromStates: revisions.RunMachine.From(revisions.ActionClaim)})
	if err != nil || claimed.AgentRequest.ID != row.ID {
		t.Fatalf("claimed %v, want %v, err %v", claimed.AgentRequest.ID, row.ID, err)
	}
	_, err = r.service.SaveBuild(t.Context(), owner.principal, BuildSave{
		RequestID: row.ID, Attempt: buildAttempt, DashboardID: dashID, BaseVersion: 1, Spec: panelsSpec("t", "rps", "p99"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return row.ID
}

func TestABuildIsReadByItsRequesterAndAnyAdminButNoOtherPerson(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.runningRequest(t, pgtype.UUID{})
	r.saveBuild(t, id, pgtype.UUID{}, 0, "a")
	admin := r.addPerson(t, "boss").principal
	admin.Role = auth.RoleAdmin
	adminAccount := auth.Principal{Kind: auth.KindServiceAccount, ID: "44444444-4444-4444-8444-444444444444", Name: "ops", Role: auth.RoleAdmin}
	editor := r.addPerson(t, "eve").principal
	cases := []struct {
		name string
		who  auth.Principal
		want error
	}{
		{"the requester", testUser, nil},
		{"an Admin", admin, nil},
		{"an Admin service account", adminAccount, nil},
		{"another Editor", editor, ErrBuildNotFound},
	}
	for _, c := range cases {
		if _, err := r.service.ViewBuild(t.Context(), c.who, id); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
}

func TestAnAdminDoesNotReadTheBuildOfADashboardInSomeonesPersonalFolder(t *testing.T) {
	r, _ := newProbeRig(t)
	owner := r.addPerson(t, "ada")
	id := r.personalRevisionBuild(t, owner)
	admin := r.addPerson(t, "boss").principal
	admin.Role = auth.RoleAdmin

	if _, err := r.service.ViewBuild(t.Context(), owner.principal, id); err != nil {
		t.Errorf("the requester: %v", err)
	}
	if _, err := r.service.ViewBuild(t.Context(), admin, id); !errors.Is(err, ErrBuildNotFound) {
		t.Errorf("an Admin: %v, want %v", err, ErrBuildNotFound)
	}
}
