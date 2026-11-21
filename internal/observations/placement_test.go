package observations_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

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
