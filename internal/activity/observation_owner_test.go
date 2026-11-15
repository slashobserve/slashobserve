package activity

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r rig) observationIn(t *testing.T, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	var raw string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 't', 'user', gen_random_uuid(), 'Seed') RETURNING id::text`, folder.String()).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r rig) observationOf(t *testing.T, seq int64) string {
	t.Helper()
	var got string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT COALESCE(observation_id::text, '') FROM activity WHERE seq = $1`, seq).Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAnAppendOwnedByAnObservationRecordsTheObservationAndItsFolder(t *testing.T) {
	r := newRig(t)
	folder := r.openFolder(t, "Incidents")
	observation := r.observationIn(t, folder)

	seq := r.append(t, Entry{Type: TypeObservationCreated, TargetID: observation, Owner: OnObservation(observation)})
	if got := r.observationOf(t, seq); got != observation.String() {
		t.Fatalf("observation_id = %q, want %s", got, observation)
	}
	if got, want := r.ownerColumnsOf(t, seq), (ownerColumns{folder: folder.String()}); got != want {
		t.Fatalf("other owner columns = %+v, want only the folder", got)
	}
}

func TestMovingAnObservationRewritesOnlyItsOwnRows(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	from, to := r.openFolder(t, "From"), r.openFolder(t, "To")
	moving, staying := r.observationIn(t, from), r.observationIn(t, from)

	movingSeq := r.append(t, Entry{Type: TypeObservationCreated, TargetID: moving, Owner: OnObservation(moving)})
	stayingSeq := r.append(t, Entry{Type: TypeObservationCreated, TargetID: staying, Owner: OnObservation(staying)})

	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, moving, to)
	err := r.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		return Move(ctx, q, OnObservation(moving), to)
	})
	if err != nil {
		t.Fatal(err)
	}
	for seq, want := range map[int64]pgtype.UUID{movingSeq: to, stayingSeq: from} {
		if got := r.folderOf(t, seq); got != want.String() {
			t.Errorf("activity %d folder = %s, want %s", seq, got, want)
		}
	}
}

func TestAnObservationFilterReturnsOnlyThatObservationsEventsWithinTheCallersFolders(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	open := r.openFolder(t, "Incidents")
	restricted := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)
	wanted, other, hidden := r.observationIn(t, open), r.observationIn(t, open), r.observationIn(t, restricted)

	created := r.append(t, Entry{Type: TypeObservationCreated, TargetID: wanted, Owner: OnObservation(wanted)})
	renamed := r.append(t, Entry{Type: TypeObservationUpdated, TargetID: wanted, Owner: OnObservation(wanted)})
	r.append(t, Entry{Type: TypeObservationCreated, TargetID: other, Owner: OnObservation(other)})
	hiddenCreated := r.append(t, Entry{Type: TypeObservationCreated, TargetID: hidden, Owner: OnObservation(hidden)})

	tests := []struct {
		name   string
		who    auth.Principal
		filter Filter
		want   []int64
	}{
		{"the observation's own events, newest first", plain, Filter{ObservationID: wanted}, []int64{renamed, created}},
		{"an observation in a folder the caller cannot view", plain, Filter{ObservationID: hidden}, []int64{}},
		{"the same observation for a member of the folder's group", payroll, Filter{ObservationID: hidden}, []int64{hiddenCreated}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := r.store.List(ctx, r.audience(t, tt.who), tt.filter, "", 10)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(seqsOf(got), tt.want) {
				t.Fatalf("got %v, want %v", seqsOf(got), tt.want)
			}
		})
	}
}

func TestAnObservationFilterAppliesToFollowers(t *testing.T) {
	r := newRig(t)
	open := r.openFolder(t, "Incidents")
	wanted, other := r.observationIn(t, open), r.observationIn(t, open)
	r.append(t, Entry{Type: TypeObservationCreated, TargetID: other, Owner: OnObservation(other)})
	created := r.append(t, Entry{Type: TypeObservationCreated, TargetID: wanted, Owner: OnObservation(wanted)})

	got, _, _, err := r.store.Next(t.Context(), r.audience(t, admin), Filter{ObservationID: wanted}, Position{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seqsOf(got), []int64{created}) {
		t.Fatalf("got %v, want [%d]", seqsOf(got), created)
	}
}
