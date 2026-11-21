package revisions_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type noBuilds struct{}

func (noBuilds) ViewBuild(context.Context, auth.Principal, pgtype.UUID) (revisions.BuildView, error) {
	return revisions.BuildView{}, revisions.ErrBuildNotFound
}

func (noBuilds) SaveStoppedBuild(context.Context, auth.Principal, pgtype.UUID) (revisions.SavedBuild, error) {
	return revisions.SavedBuild{}, revisions.ErrBuildNotFound
}

func recordingAudit(t *testing.T, d *dbtest.Database) (*audit.Recorder, func()) {
	t.Helper()
	recorder := audit.New(audit.Config{DB: d.DB, Logger: quietLogger()})
	audit.Install(recorder)
	t.Cleanup(func() { audit.Install(nil) })
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(ctx)
	}()
	return recorder, func() {
		stop()
		<-done
	}
}

func mcpContext(t *testing.T, who auth.Principal) context.Context {
	t.Helper()
	return audit.Open(origin.With(auth.WithPrincipal(t.Context(), who), origin.Origin{Kind: origin.MCP}), audit.Scope{Actor: auth.AuditActor(who), Initiator: audit.InitiatorPerson, Origin: audit.Origin{Kind: "mcp"}})
}

func TestAReadableRunWithNoBuildIsNotARefusedRead(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithBuilds(noBuilds{}))
	run := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs exist")}).String()
	_, flush := recordingAudit(t, d)
	registry := tools.New(tools.Deps{Runs: revisions.NewRunDesk(svc, revisions.ObserveOptions{}), Logger: quietLogger()})
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}

	for _, who := range []auth.Principal{bea, eve} {
		res, err := registry.Call(mcpContext(t, who), "get_run_build", []byte(`{"id":"`+run+`"}`))
		if err != nil || !res.IsError {
			t.Fatalf("%s: get_run_build = %+v, %v; want a tool error", who.Name, res, err)
		}
	}
	flush()

	want := []string{
		"run.view " + adminID + " success " + run + " " + run + " " + userID,
		"run.view " + strangerID + " denied " + run + " - -",
	}
	slices.Sort(want)
	if got := recordedReads(t, d); !slices.Equal(got, want) {
		t.Fatalf("recorded reads = %q, want %q", got, want)
	}
}
