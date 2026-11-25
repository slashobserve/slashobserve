package folders

import (
	"context"
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestRunOfReadsTheActorsRunAndElseTheRunOfTheContext(t *testing.T) {
	const actorRun = "11111111-1111-4111-8111-111111111111"
	const contextRun = "22222222-2222-4222-8222-222222222222"
	connected := actor.AsConnectedAgent(actor.WithRun(context.Background(), contextRun))
	cases := []struct {
		name string
		ctx  context.Context
		who  actor.Actor
		want string
	}{
		{"the actor's run", context.Background(), actor.Actor{RunID: actorRun}, actorRun},
		{"a connected agent's run, which its actor leaves out", connected, actor.Actor{}, contextRun},
		{"the actor's run over the context's", connected, actor.Actor{RunID: actorRun}, actorRun},
		{"no run", context.Background(), actor.Actor{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RunOf(c.ctx, c.who)
			if err != nil || got.Valid != (c.want != "") || (got.Valid && got.String() != c.want) {
				t.Fatalf("RunOf = %v, %v; want %q", got, err, c.want)
			}
		})
	}
}

func TestRunOfRefusesARunIdThatIsNotAUUID(t *testing.T) {
	if _, err := RunOf(actor.WithRun(context.Background(), "not-a-run"), actor.Actor{}); err == nil {
		t.Fatal("a malformed run id was accepted")
	}
}

func TestAFolderWriteOutsideARunReadsNothing(t *testing.T) {
	if err := runWriteInto(context.Background(), nil, false); err != nil {
		t.Fatalf("a write outside a run: %v", err)
	}
}

func TestAFolderWriteOfAMalformedRunIsRefusedBeforeItReads(t *testing.T) {
	if err := runWriteInto(actor.WithRun(context.Background(), "not-a-run"), nil, false); err == nil {
		t.Fatal("a malformed run id was accepted")
	}
}

func TestARunThatReadAdminOnlyDataNamesNoFolderOthersRead(t *testing.T) {
	d := dbtest.New(t)
	access := NewAccess(d.DB)
	const restricted = "33333333-3333-4333-8333-333333333333"
	const clean = "44444444-4444-4444-8444-444444444444"
	for id, read := range map[string]bool{restricted: true, clean: false} {
		d.Exec(t, `INSERT INTO agent_requests (id, intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, read_restricted) VALUES ($1::uuid, 'ask', 'investigate', 'who changed it', 'user', gen_random_uuid(), 'Root', $2)`, id, read)
	}
	inRestricted := actor.AsConnectedAgent(actor.WithRun(t.Context(), restricted))
	if _, _, err := access.CreateFolder(inRestricted, rootAdmin, "Payroll findings"); !errors.Is(err, ErrRestrictedAnswer) {
		t.Fatalf("create in a run that read Admin-only data = %v, want ErrRestrictedAnswer", err)
	}
	shared, _, err := access.CreateFolder(actor.WithRun(t.Context(), clean), rootAdmin, "Latency")
	if err != nil {
		t.Fatalf("create in a run that read nothing restricted: %v", err)
	}
	if _, _, err := access.RenameFolder(inRestricted, rootAdmin, shared.ID, "Payroll latency"); !errors.Is(err, ErrRestrictedAnswer) {
		t.Fatalf("rename of a shared folder in a run that read Admin-only data = %v, want ErrRestrictedAnswer", err)
	}
	hidden, err := access.CreateAdminsOnlyFolder(inRestricted, rootAdmin, "Payroll")
	if err != nil {
		t.Fatalf("an Admins-only folder in a run that read Admin-only data: %v", err)
	}
	if _, _, err := access.RenameFolder(inRestricted, rootAdmin, hidden, "Payroll 2026"); err != nil {
		t.Fatalf("rename of an Admins-only folder in a run that read Admin-only data: %v", err)
	}
}
