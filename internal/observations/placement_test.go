package observations_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/observations"
)

func auditPanel() json.RawMessage {
	return json.RawMessage(`{"panel":{"type":"timeseries","title":"Salaries","datasource":"audit","queries":[{"ref":"A","query":"sum(payroll)"}]}}`)
}

func (r rig) adminsOnlyDatasource(t *testing.T) {
	t.Helper()
	r.db.Exec(t, `INSERT INTO datasources (uid, type, name, config, audience) VALUES ('audit', 'prometheus', 'Audit', '{}', 'admins')`)
}

func refusedAsRestricted(t *testing.T, err error) {
	t.Helper()
	var refusal *folders.RestrictedContentError
	if !errors.As(err, &refusal) || refusal.Datasource != "audit" {
		t.Fatalf("err = %v, want a restricted content refusal naming audit", err)
	}
}

func TestAPanelOnAnAdminsOnlyDatasourceIsAddedOnlyInAnAdminsOnlyFolder(t *testing.T) {
	r := newRig(t)
	r.adminsOnlyDatasource(t)
	ctx := reading(t.Context())

	open := r.create(t, admin, r.shared)
	_, err := r.service.AddBlock(ctx, admin, mustUUID(t, open.ID), observations.BlockInput{Kind: observations.KindPanel, Body: auditPanel()})
	refusedAsRestricted(t, err)

	plain := r.addPanel(t, admin, open)
	_, err = r.service.UpdateBlock(ctx, admin, mustUUID(t, plain.ID), observations.BlockChange{Version: plain.Version, Body: auditPanel()})
	refusedAsRestricted(t, err)

	locked := r.create(t, admin, foldertest.AdminsOnly(t, r.db, "Payroll"))
	if _, err := r.service.AddBlock(ctx, admin, mustUUID(t, locked.ID), observations.BlockInput{Kind: observations.KindPanel, Body: auditPanel()}); err != nil {
		t.Fatalf("admins only folder = %v", err)
	}
}

func TestAnObservationOnAnAdminsOnlyDatasourceKeepsItsSnapshotsInAnAdminsOnlyFolder(t *testing.T) {
	r := newRig(t)
	r.adminsOnlyDatasource(t)
	ctx := reading(t.Context())
	locked := r.create(t, admin, foldertest.AdminsOnly(t, r.db, "Payroll"))
	panel, err := r.service.AddBlock(ctx, admin, mustUUID(t, locked.ID), observations.BlockInput{Kind: observations.KindPanel, Body: auditPanel()})
	if err != nil {
		t.Fatal(err)
	}
	snap := r.addSnapshot(t, admin, locked, panel)
	if err := r.service.RemoveBlock(ctx, admin, mustUUID(t, panel.ID), panel.Version, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}

	_, err = r.service.Move(ctx, admin, mustUUID(t, locked.ID), r.shared)
	refusedAsRestricted(t, err)

	if _, err := r.service.Get(ctx, editor, mustUUID(t, locked.ID)); !errors.Is(err, observations.ErrNotFound) {
		t.Fatalf("a non-Admin reads the observation: %v", err)
	}
	if _, err := r.service.Snapshot(ctx, editor, mustUUID(t, snap.SnapshotID)); !errors.Is(err, observations.ErrNotFound) {
		t.Fatalf("a non-Admin reads the snapshot: %v", err)
	}
}

func TestCreatingWithABlockOnAnAdminsOnlyDatasourceOutsideAnAdminsOnlyFolderLeavesNothing(t *testing.T) {
	r := newRig(t)
	r.adminsOnlyDatasource(t)
	ctx := reading(t.Context())

	_, err := r.service.Create(ctx, admin, observations.CreateInput{
		FolderID: r.shared, Title: "Payroll",
		Blocks: []observations.BlockInput{
			{Kind: observations.KindText, Body: textBody("fine")},
			{Kind: observations.KindPanel, Body: auditPanel()},
		},
	})
	refusedAsRestricted(t, err)
	var blockErr *observations.BlockError
	if !errors.As(err, &blockErr) || blockErr.Index != 1 {
		t.Fatalf("err = %v, want the second block named", err)
	}
	list, _, err := r.service.List(ctx, admin, observations.ListFilter{FolderID: r.shared}, 10, "")
	if err != nil || len(list) != 0 {
		t.Errorf("the refused create left %d observations, %v", len(list), err)
	}
}

func TestARunThatReadAdminOnlyDataNeitherCreatesNorMovesAnObservationWhereOthersRead(t *testing.T) {
	r := newRig(t)
	const run = "88888888-8888-4888-8888-888888888888"
	r.db.Exec(t, `INSERT INTO agent_requests (id, intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, read_restricted) VALUES ($1::uuid, 'ask', 'investigate', 'who changed it', 'user', $2::uuid, 'Root', true)`, run, admin.ID)
	locked := foldertest.AdminsOnly(t, r.db, "Payroll")
	held := r.create(t, admin, locked)
	for name, ctx := range map[string]context.Context{
		"the built-in agent": actor.WithRun(reading(t.Context()), run),
		"a connected agent":  actor.AsConnectedAgent(actor.WithRun(reading(t.Context()), run)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := r.service.Create(ctx, admin, observations.CreateInput{FolderID: r.shared, Title: "Payroll audit"}); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("create where others read = %v, want ErrRestrictedAnswer", err)
			}
			if _, err := r.service.Move(ctx, admin, mustUUID(t, held.ID), r.shared); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("move where others read = %v, want ErrRestrictedAnswer", err)
			}
			if _, err := r.service.Create(ctx, admin, observations.CreateInput{FolderID: locked, Title: "Payroll audit"}); err != nil {
				t.Fatalf("create where only Admins read: %v", err)
			}
		})
	}
}
