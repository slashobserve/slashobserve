package activity

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

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

func TestAFindingEventOwnedByItsObservationRecordsTheFindingTheObservationAndTheFolder(t *testing.T) {
	r := newRig(t)
	folder := r.openFolder(t, "Incidents")
	observation := r.observationIn(t, folder)

	seq := r.append(t, Entry{Type: TypeFindingCreated, TargetID: observation, Owner: OnObservation(observation)})
	if got, want := r.ownerColumnsOf(t, seq), (ownerColumns{finding: observation.String(), folder: folder.String()}); got != want {
		t.Fatalf("activity = %+v, want %+v", got, want)
	}
	if got := r.observationOf(t, seq); got != observation.String() {
		t.Fatalf("observation_id = %q, want %s", got, observation)
	}
}

func TestAFindingEventOwnedByADashboardCarriesNoFindingColumn(t *testing.T) {
	r := newRig(t)
	folder := r.openFolder(t, "Ops")
	dashboard := r.dashboardIn(t, folder)
	finding := r.observationIn(t, folder)

	seq := r.append(t, Entry{Type: TypeFindingPinned, TargetID: finding, Owner: OnDashboard(dashboard)})
	if got, want := r.ownerColumnsOf(t, seq), (ownerColumns{dashboard: dashboard.String(), folder: folder.String()}); got != want {
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
	for name, owner := range map[string]Owner{"dashboard": OnDashboard(pgtype.UUID{}), "observation": OnObservation(pgtype.UUID{}), "saved search": OnSavedSearch(pgtype.UUID{})} {
		seq := r.append(t, Entry{Type: TypeFindingCreated, Owner: owner})
		if got := r.ownerColumnsOf(t, seq); got != (ownerColumns{}) {
			t.Errorf("%s owner with no id: activity = %+v, want no owner and no folder", name, got)
		}
	}
}

func TestMoveRefusesOwnersThatHaveNoFolderToFollow(t *testing.T) {
	r := newRig(t)
	for name, owner := range map[string]Owner{"none": {}, "general": General(), "invalid id": OnObservation(pgtype.UUID{})} {
		err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
			return Move(t.Context(), q, owner, r.openFolder(t, "To "+name))
		})
		if err == nil {
			t.Errorf("Move of the %s owner succeeded, want an error", name)
		}
	}
}
