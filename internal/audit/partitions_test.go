package audit

import (
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func partitionName(day time.Time) string {
	return partitionPrefix + day.Format(partitionDayLayout)
}

func (s storeRig) partitions(t *testing.T) []string {
	t.Helper()
	attached, err := s.db.Queries().ListAuditPartitions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(attached))
	for i, p := range attached {
		names[i] = p.Name
	}
	slices.Sort(names)
	return names
}

func TestThePartitionJobCreatesAheadAndDropsBehind(t *testing.T) {
	s := newStoreRig(t, writerLimits{}, 72*time.Hour)
	now := time.Now()
	today := utcDay(now)
	for back := 5; back <= 10; back++ {
		if err := s.recorder.addPartition(t.Context(), today.AddDate(0, 0, -back)); err != nil {
			t.Fatal(err)
		}
	}
	old := s.recorder.rowAt(t, today.AddDate(0, 0, -7).Add(time.Hour))
	if err := s.db.Queries().InsertAuditEvent(t.Context(), old); err != nil {
		t.Fatal(err)
	}

	if err := s.recorder.keepPartitions(t.Context(), now); err != nil {
		t.Fatal(err)
	}

	names := s.partitions(t)
	for day := range DaysAhead + 1 {
		if want := partitionName(today.AddDate(0, 0, day)); !slices.Contains(names, want) {
			t.Errorf("partition %s is missing from %v", want, names)
		}
	}
	cutoff := today.AddDate(0, 0, -3)
	for _, name := range names {
		if day, _ := partitionDay(name); day.Before(cutoff) {
			t.Errorf("partition %s is older than the retention and still attached", name)
		}
	}
	if n := s.count(t, "occurred_at < $1", cutoff); n != 0 {
		t.Errorf("events older than the retention = %d, want 0", n)
	}
	if s.metrics.ahead != DaysAhead {
		t.Errorf("partitions ahead = %d, want %d", s.metrics.ahead, DaysAhead)
	}
	if n := s.count(t, "action = 'audit.prune' AND (data->>'partitions')::int = 6 AND data->>'before' = $1", cutoff.Format(time.DateOnly)); n != 1 {
		t.Errorf("prune events naming 6 partitions before %s = %d, want 1", cutoff.Format(time.DateOnly), n)
	}

	if err := s.recorder.keepPartitions(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	if n := s.count(t, "action = 'audit.prune'"); n != 1 {
		t.Errorf("a second run with nothing to remove recorded %d prune events, want 1", n)
	}
	if got := s.partitions(t); !slices.Equal(got, names) {
		t.Errorf("a second run changed the partitions from %v to %v", names, got)
	}
}

func TestAPartitionLeftDetachedIsDropped(t *testing.T) {
	s := newStoreRig(t, writerLimits{}, 72*time.Hour)
	day := utcDay(time.Now()).AddDate(0, 0, -9)
	if err := s.recorder.addPartition(t.Context(), day); err != nil {
		t.Fatal(err)
	}
	s.db.Exec(t, "ALTER TABLE audit_events DETACH PARTITION "+partitionName(day))
	if err := s.recorder.keepPartitions(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := s.db.SQLDB(t).QueryRowContext(t.Context(), "SELECT to_regclass($1) IS NOT NULL", partitionName(day)).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatalf("detached partition %s was not dropped", partitionName(day))
	}
}

func TestAddingAPartitionTwiceIsANoOp(t *testing.T) {
	s := newStoreRig(t, writerLimits{}, 0)
	day := utcDay(time.Now()).AddDate(0, 0, DaysAhead+3)
	for range 2 {
		if err := s.recorder.addPartition(t.Context(), day); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Contains(s.partitions(t), partitionName(day)) {
		t.Fatalf("partition %s is missing", partitionName(day))
	}
}

func (r *Recorder) rowAt(t *testing.T, at time.Time) dbsqlc.InsertAuditEventParams {
	t.Helper()
	row, err := r.row(t.Context(), Event{Action: "dashboard.view"})
	if err != nil {
		t.Fatal(err)
	}
	row.OccurredAt = pgtype.Timestamptz{Time: at, Valid: true}
	return row
}
