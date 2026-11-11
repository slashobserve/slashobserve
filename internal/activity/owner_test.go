package activity

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func (r rig) findingIn(t *testing.T, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	var raw string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO findings (folder_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES (nullif($1, '')::uuid, 't', 'm', now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'Seed') RETURNING id::text`, folder.String()).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type ownerColumns struct{ dashboard, finding, savedSearch, folder string }

func (r rig) ownerColumnsOf(t *testing.T, seq int64) ownerColumns {
	t.Helper()
	var got ownerColumns
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT COALESCE(dashboard_id::text, ''), COALESCE(finding_id::text, ''), COALESCE(saved_search_id::text, ''), COALESCE(folder_id::text, '') FROM activity WHERE seq = $1`, seq).
		Scan(&got.dashboard, &got.finding, &got.savedSearch, &got.folder)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAnAppendOwnedByAFilingFindingRecordsTheFindingAndItsFolder(t *testing.T) {
	r := newRig(t)
	folder := r.openFolder(t, "Incidents")
	finding := r.findingIn(t, folder)

	seq := r.append(t, Entry{Type: TypeFindingCreated, TargetID: finding, Owner: OnFinding(finding)})
	if got, want := r.ownerColumnsOf(t, seq), (ownerColumns{finding: finding.String(), folder: folder.String()}); got != want {
		t.Fatalf("activity = %+v, want %+v", got, want)
	}
}

func TestAnAppendOwnedByAnUnfiledFindingHasNoFolder(t *testing.T) {
	r := newRig(t)
	finding := r.findingIn(t, pgtype.UUID{})

	seq := r.append(t, Entry{Type: TypeFindingCreated, TargetID: finding, Owner: OnFinding(finding)})
	if got, want := r.ownerColumnsOf(t, seq), (ownerColumns{finding: finding.String()}); got != want {
		t.Fatalf("activity = %+v, want %+v", got, want)
	}
}

func TestAnAppendOwnedByADashboardRecordsTheDashboardAndItsFolder(t *testing.T) {
	r := newRig(t)
	folder := r.openFolder(t, "Ops")
	dashboard := r.dashboardIn(t, folder)

	seq := r.append(t, Entry{Type: TypeDashboardCreated, TargetID: dashboard, Owner: OnDashboard(dashboard)})
	if got, want := r.ownerColumnsOf(t, seq), (ownerColumns{dashboard: dashboard.String(), folder: folder.String()}); got != want {
		t.Fatalf("activity = %+v, want %+v", got, want)
	}
}

func TestAnAppendWithAnInvalidOwnerIDHasNoOwner(t *testing.T) {
	r := newRig(t)
	for name, owner := range map[string]Owner{"dashboard": OnDashboard(pgtype.UUID{}), "finding": OnFinding(pgtype.UUID{}), "saved search": OnSavedSearch(pgtype.UUID{})} {
		seq := r.append(t, Entry{Type: TypeFindingCreated, Owner: owner})
		if got := r.ownerColumnsOf(t, seq); got != (ownerColumns{}) {
			t.Errorf("%s owner with no id: activity = %+v, want no owner and no folder", name, got)
		}
	}
}

func TestMovingAFindingRewritesOnlyItsOwnRows(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	from, to := r.openFolder(t, "From"), r.openFolder(t, "To")
	moving, staying := r.findingIn(t, from), r.findingIn(t, from)
	dashboard := r.dashboardIn(t, from)

	movingSeq := r.append(t, Entry{Type: TypeFindingCreated, TargetID: moving, Owner: OnFinding(moving)})
	stayingSeq := r.append(t, Entry{Type: TypeFindingCreated, TargetID: staying, Owner: OnFinding(staying)})
	dashboardSeq := r.append(t, Entry{Type: TypeDashboardCreated, TargetID: dashboard, Owner: OnDashboard(dashboard)})

	r.db.Exec(t, `UPDATE findings SET folder_id = $2 WHERE id = $1`, moving, to)
	err := r.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		return Move(ctx, q, OnFinding(moving), to)
	})
	if err != nil {
		t.Fatal(err)
	}
	for seq, want := range map[int64]pgtype.UUID{movingSeq: to, stayingSeq: from, dashboardSeq: from} {
		if got := r.folderOf(t, seq); got != want.String() {
			t.Errorf("activity %d folder = %s, want %s", seq, got, want)
		}
	}
}

func TestMoveRefusesOwnersThatHaveNoFolderToFollow(t *testing.T) {
	r := newRig(t)
	for name, owner := range map[string]Owner{"none": {}, "general": General(), "invalid id": OnFinding(pgtype.UUID{})} {
		err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
			return Move(t.Context(), q, owner, r.openFolder(t, "To "+name))
		})
		if err == nil {
			t.Errorf("Move of the %s owner succeeded, want an error", name)
		}
	}
}
