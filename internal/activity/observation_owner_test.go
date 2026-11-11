package activity

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
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
