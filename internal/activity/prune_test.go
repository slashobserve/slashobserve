package activity

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func (r rig) backdate(t *testing.T, age time.Duration, seqs ...int64) {
	t.Helper()
	r.db.Exec(t, `UPDATE activity SET created_at = now() - make_interval(secs => $1) WHERE seq = ANY($2)`, age.Seconds(), seqs)
}

func (r rig) remaining(t *testing.T) []int64 {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT seq FROM activity ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var seqs []int64
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, seq)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return seqs
}

func TestPruneDeletesExpiredRowsInBatchesAndKeepsNewerOnes(t *testing.T) {
	r := newRig(t)
	var old []int64
	for range 5 {
		old = append(old, r.append(t, Entry{Type: TypeAnnotationCreated}))
	}
	recent := r.append(t, Entry{Type: TypeAnnotationCreated})
	boundary := r.append(t, Entry{Type: TypeAnnotationCreated})
	r.backdate(t, 48*time.Hour, old...)
	r.backdate(t, 23*time.Hour, boundary)

	deleted, err := prune(t.Context(), r.db.Queries(), time.Now().Add(-24*time.Hour), 2)

	if err != nil {
		t.Fatal(err)
	}
	if deleted != 5 {
		t.Errorf("deleted %d rows, want 5 across three batches of at most 2", deleted)
	}
	if got := r.remaining(t); !slices.Equal(got, []int64{recent, boundary}) {
		t.Errorf("remaining %v, want [%d %d]", got, recent, boundary)
	}
}

func TestPruneStopsWhenTheContextEnds(t *testing.T) {
	r := newRig(t)
	seq := r.append(t, Entry{Type: TypeAnnotationCreated})
	r.backdate(t, 48*time.Hour, seq)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	deleted, err := prune(ctx, r.db.Queries(), time.Now().Add(-24*time.Hour), 2)

	if !errors.Is(err, context.Canceled) || deleted != 0 {
		t.Errorf("prune with an ended context = %d, %v; want 0 and context.Canceled", deleted, err)
	}
	if got := r.remaining(t); !slices.Equal(got, []int64{seq}) {
		t.Errorf("remaining %v, want the row kept", got)
	}
}

func TestPruneJobUsesTheRetention(t *testing.T) {
	r := newRig(t)
	expired := r.append(t, Entry{Type: TypeAnnotationCreated})
	kept := r.append(t, Entry{Type: TypeAnnotationCreated})
	r.backdate(t, 30*time.Hour, expired)
	r.backdate(t, 10*time.Hour, kept)

	if err := PruneJob(r.db.DB, 24*time.Hour)(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := r.remaining(t); !slices.Equal(got, []int64{kept}) {
		t.Errorf("remaining %v, want [%d]", got, kept)
	}
}
