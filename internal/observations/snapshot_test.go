package observations_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func (r rig) addSnapshot(t *testing.T, by auth.Principal, o observations.Observation, from observations.Block) observations.Block {
	t.Helper()
	b, err := r.service.AddBlock(t.Context(), by, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindSnapshot, FromBlockID: mustUUID(t, from.ID)})
	if err != nil {
		t.Fatalf("add snapshot block: %v", err)
	}
	return b
}

func TestASnapshotFreezesThePanelBlockNowAndKeepsItsQueryApart(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	panel := r.addPanel(t, editor, o)
	r.freezer.userScoped = true

	block := r.addSnapshot(t, editor, o, panel)
	if block.Kind != observations.KindSnapshot || block.SnapshotID == "" || block.State != observations.BlockLive || block.Position != 1 {
		t.Fatalf("block %+v", block)
	}
	if !strings.Contains(string(block.Body), block.SnapshotID) {
		t.Errorf("body %s", block.Body)
	}
	if r.freezer.panelID != observations.BlockPanelID || r.freezer.title != "Checkout 5xx" || !r.freezer.to.After(r.freezer.from) {
		t.Errorf("freezer saw panel %q title %q range %v-%v", r.freezer.panelID, r.freezer.title, r.freezer.from, r.freezer.to)
	}

	snap, err := r.service.Snapshot(t.Context(), viewer, mustUUID(t, block.SnapshotID))
	if err != nil {
		t.Fatal(err)
	}
	if snap.ObservationID != o.ID || !snap.UserScoped || !jsonEqual(t, snap.Data, json.RawMessage(frozenData)) {
		t.Errorf("snapshot %+v", snap)
	}
	var query []map[string]any
	if err := json.Unmarshal(snap.Query, &query); err != nil || len(query) != 1 || query[0]["query"] != "sum(rate(http_requests_total[5m]))" {
		t.Errorf("query %s, %v", snap.Query, err)
	}
	var drawn map[string]any
	if err := json.Unmarshal(snap.Panel, &drawn); err != nil {
		t.Fatal(err)
	}
	if _, has := drawn["queries"]; has || drawn["type"] != "timeseries" {
		t.Errorf("the drawn panel keeps %v", drawn)
	}
	if !snap.From.Equal(r.freezer.from) || !snap.To.Equal(r.freezer.to) || snap.Actor.ID != editor.ID {
		t.Errorf("range %v-%v actor %+v", snap.From, snap.To, snap.Actor)
	}
}

func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	ax, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return string(ax) == string(by)
}

func TestTheFrozenRangeIsTheBlocksThenTheObservationsThenTheLastHour(t *testing.T) {
	r := newRig(t)
	own := `{"panel":{"type":"stat","title":"Up","datasource":"prom","queries":[{"ref":"A","query":"up"}]},"from":"2026-10-01T00:00:00Z","to":"2026-10-01T06:00:00Z"}`
	obsFrom, obsTo := windowTo.Add(-3*time.Hour), windowTo
	withRange, err := r.service.Create(t.Context(), editor, observations.CreateInput{FolderID: r.shared, Title: "ranged", From: &obsFrom, To: &obsTo})
	if err != nil {
		t.Fatal(err)
	}
	bare := r.create(t, editor, r.shared)

	ownBlock, err := r.service.AddBlock(t.Context(), editor, mustUUID(t, withRange.ID), observations.BlockInput{Kind: observations.KindPanel, Body: json.RawMessage(own)})
	if err != nil {
		t.Fatal(err)
	}
	r.addSnapshot(t, editor, withRange, ownBlock)
	if got := r.freezer.from.Format(time.RFC3339); got != "2026-10-01T00:00:00Z" {
		t.Errorf("the block's own range: from %s", got)
	}

	inherited := r.addPanel(t, editor, withRange)
	r.addSnapshot(t, editor, withRange, inherited)
	if !r.freezer.from.Equal(obsFrom) || !r.freezer.to.Equal(obsTo) {
		t.Errorf("the observation's range: %v-%v", r.freezer.from, r.freezer.to)
	}

	loose := r.addPanel(t, editor, bare)
	r.addSnapshot(t, editor, bare, loose)
	if d := r.freezer.to.Sub(r.freezer.from); d != time.Hour || time.Since(r.freezer.to) > time.Minute {
		t.Errorf("the last hour: %v-%v", r.freezer.from, r.freezer.to)
	}
}

func TestTheStoredRangeIsTheOneTheFreezerWasGivenDownToTheMicrosecond(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	own := `{"panel":{"type":"stat","title":"Up","datasource":"prom","queries":[{"ref":"A","query":"up"}]},"from":"2026-10-01T00:00:00.123456789Z","to":"2026-10-01T06:00:00.987654321Z"}`
	block, err := r.service.AddBlock(t.Context(), editor, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindPanel, Body: json.RawMessage(own)})
	if err != nil {
		t.Fatal(err)
	}

	frozen := r.addSnapshot(t, editor, o, block)
	snap, err := r.service.Snapshot(t.Context(), viewer, mustUUID(t, frozen.SnapshotID))
	if err != nil {
		t.Fatal(err)
	}
	if !snap.From.Equal(r.freezer.from) || !snap.To.Equal(r.freezer.to) {
		t.Errorf("the freezer drew %v-%v, the snapshot says %v-%v", r.freezer.from, r.freezer.to, snap.From, snap.To)
	}
	if r.freezer.from.Nanosecond()%1000 != 0 || r.freezer.to.Nanosecond()%1000 != 0 {
		t.Errorf("the freezer was given sub-microsecond bounds %v-%v", r.freezer.from, r.freezer.to)
	}
}

func TestAFreezerErrorOrAnOversizeBodyInsertsNothing(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	panel := r.addPanel(t, editor, o)
	in := observations.BlockInput{Kind: observations.KindSnapshot, FromBlockID: mustUUID(t, panel.ID)}

	r.freezer.err = queryproxy.ErrUpstreamBusy
	if _, err := r.service.AddBlock(t.Context(), editor, mustUUID(t, o.ID), in); !errors.Is(err, queryproxy.ErrUpstreamBusy) {
		t.Errorf("freezer error: %v", err)
	}
	r.freezer.err = nil

	r.freezer.data = json.RawMessage(`{"frames":["` + strings.Repeat("x", observations.MaxSnapshotBytes) + `"]}`)
	if _, err := r.service.AddBlock(t.Context(), editor, mustUUID(t, o.ID), in); !errors.Is(err, observations.ErrSnapshotTooLarge) {
		t.Errorf("oversize: %v", err)
	}
	var invalid *observations.InvalidInputError
	if _, err := r.service.AddBlock(t.Context(), editor, mustUUID(t, o.ID), in); !errors.As(err, &invalid) || invalid.Code != observations.CodeSnapshotTooLarge {
		t.Errorf("oversize code: %v", err)
	}

	var snapshots int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM snapshots`).Scan(&snapshots); err != nil || snapshots != 0 {
		t.Errorf("%d snapshots stored, %v", snapshots, err)
	}
	if got := r.positions(t, editor, o); len(got) != 1 {
		t.Errorf("blocks %v", got)
	}
}

func TestASnapshotIsReadableOnlyWhereTheObservationIs(t *testing.T) {
	r := newRig(t)
	o := r.create(t, admin, r.hiddenFolder(t))
	panel := r.addPanel(t, admin, o)
	block := r.addSnapshot(t, admin, o, panel)
	if _, err := r.service.Snapshot(t.Context(), admin, mustUUID(t, block.SnapshotID)); err != nil {
		t.Errorf("an admin: %v", err)
	}
	if _, err := r.service.Snapshot(t.Context(), editor, mustUUID(t, block.SnapshotID)); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("a non-viewer: %v", err)
	}
	if _, err := r.service.Snapshot(t.Context(), admin, pgtype.UUID{Bytes: [16]byte{15: 1}, Valid: true}); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("a missing snapshot: %v", err)
	}
}

func TestRemovingASnapshotBlockKeepsItsSnapshotRowAndDeletingTheObservationDropsIt(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	panel := r.addPanel(t, editor, o)
	block := r.addSnapshot(t, editor, o, panel)
	if err := r.service.RemoveBlock(t.Context(), editor, mustUUID(t, block.ID), 1, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		var n int
		if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM snapshots`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count() != 1 {
		t.Fatalf("%d snapshots after removing the block", count())
	}
	r.db.Exec(t, `DELETE FROM observations WHERE id = $1::uuid`, o.ID)
	if count() != 0 {
		t.Errorf("%d snapshots after deleting the observation", count())
	}
}

func TestThePanelDashboardWrapsOnePanelUnderTheBlockPanelID(t *testing.T) {
	r := newRig(t)
	from, to := windowTo.Add(-3*time.Hour), windowTo
	o, err := r.service.Create(t.Context(), editor, observations.CreateInput{FolderID: r.shared, Title: "Checkout 5xx", From: &from, To: &to})
	if err != nil {
		t.Fatal(err)
	}
	panel := r.addPanel(t, editor, o)
	text := r.addText(t, editor, o, "words")

	d, gotFrom, gotTo, err := r.service.PanelDashboard(t.Context(), viewer, mustUUID(t, panel.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Panels) != 1 || d.Panels[0].ID != observations.BlockPanelID || d.Title != "Checkout 5xx" {
		t.Errorf("dashboard %+v", d)
	}
	if !gotFrom.Equal(from) || !gotTo.Equal(to) || d.Time.From != from.Format(time.RFC3339Nano) {
		t.Errorf("range %v-%v, dashboard time %+v", gotFrom, gotTo, d.Time)
	}
	if _, _, _, err := r.service.PanelDashboard(t.Context(), viewer, mustUUID(t, text.ID)); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("a text block has no data: %v", err)
	}
	if _, _, _, err := r.service.PanelDashboard(t.Context(), viewer, pgtype.UUID{Bytes: [16]byte{15: 1}, Valid: true}); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("a missing block: %v", err)
	}
}

func TestACommenterRequestsAPanelButCannotFreezeOne(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	asked, err := r.service.AddBlock(t.Context(), commenter, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindPanel, Body: panelBody()})
	if err != nil || asked.State != observations.BlockRequested {
		t.Fatalf("requested panel %+v, %v", asked, err)
	}
	snap, err := r.service.AddBlock(t.Context(), commenter, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindSnapshot, FromBlockID: mustUUID(t, asked.ID)})
	if err != nil || snap.State != observations.BlockRequested {
		t.Errorf("a requested snapshot %+v, %v", snap, err)
	}
	if _, err := r.service.AddBlock(t.Context(), viewer, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindSnapshot, FromBlockID: mustUUID(t, asked.ID)}); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer: %v", err)
	}
}
