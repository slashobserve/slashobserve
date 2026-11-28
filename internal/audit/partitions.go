package audit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	DaysAhead            = 14
	partitionPrefix      = "audit_events_"
	partitionDayLayout   = "20060102"
	partitionMonthLayout = "200601"
	partitionYearLayout  = "2006"
	attachLockTimeout    = "2s"
)

func (r *Recorder) PartitionJob() func(context.Context) error {
	return func(ctx context.Context) error {
		return r.keepPartitions(ctx, time.Now())
	}
}

func (r *Recorder) keepPartitions(ctx context.Context, now time.Time) error {
	today := utcDay(now)
	for day := range DaysAhead + 1 {
		if err := r.addPartition(ctx, today.AddDate(0, 0, day)); err != nil {
			return err
		}
	}
	if err := r.reportAhead(ctx, today); err != nil {
		return err
	}
	if r.retention <= 0 {
		return nil
	}
	cutoff := utcDay(now.Add(-r.retention))
	removed, err := r.removePartitionsBefore(ctx, cutoff)
	if err != nil || removed == 0 {
		return err
	}
	return r.RecordNow(ctx, Event{Action: ActionPrune, Data: map[string]any{"partitions": removed, "before": cutoff.Format(time.DateOnly)}})
}

func (r *Recorder) addPartition(ctx context.Context, day time.Time) error {
	err := r.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.SetAuditLockTimeout(ctx, attachLockTimeout); err != nil {
			return err
		}
		_, err := q.AddAuditPartition(ctx, pgtype.Date{Time: day, Valid: true})
		return err
	})
	if err != nil {
		return fmt.Errorf("add audit partition %s: %w", day.Format(time.DateOnly), err)
	}
	return nil
}

func (r *Recorder) removePartitionsBefore(ctx context.Context, cutoff time.Time) (int, error) {
	q := r.db.Queries()
	attached, err := q.ListAuditPartitions(ctx)
	if err != nil {
		return 0, fmt.Errorf("list audit partitions: %w", err)
	}
	removed := 0
	for _, p := range attached {
		if !endsBy(p.Name, cutoff) {
			continue
		}
		detach := "ALTER TABLE audit_events DETACH PARTITION " + pgx.Identifier{p.Name}.Sanitize()
		if p.DetachPending {
			detach += " FINALIZE"
		} else {
			detach += " CONCURRENTLY"
		}
		if err := r.db.ExecOutsideTransaction(ctx, detach); err != nil {
			return removed, fmt.Errorf("detach audit partition %s: %w", p.Name, err)
		}
		if err := r.dropPartition(ctx, p.Name); err != nil {
			return removed, err
		}
		removed++
	}
	detached, err := q.ListDetachedAuditPartitions(ctx)
	if err != nil {
		return removed, fmt.Errorf("list detached audit partitions: %w", err)
	}
	for _, name := range detached {
		if endsBy(name, cutoff) {
			if err := r.dropPartition(ctx, name); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

func (r *Recorder) dropPartition(ctx context.Context, name string) error {
	if err := r.db.ExecOutsideTransaction(ctx, "DROP TABLE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return fmt.Errorf("drop audit partition %s: %w", name, err)
	}
	return nil
}

func (r *Recorder) reportAhead(ctx context.Context, today time.Time) error {
	attached, err := r.db.Queries().ListAuditPartitions(ctx)
	if err != nil {
		return fmt.Errorf("list audit partitions: %w", err)
	}
	ahead := 0
	for _, p := range attached {
		if starts, _, ok := partitionSpan(p.Name); ok && starts.After(today) && !p.DetachPending {
			ahead++
		}
	}
	r.metrics.PartitionsAhead(ahead)
	return nil
}

func endsBy(name string, cutoff time.Time) bool {
	_, ends, ok := partitionSpan(name)
	return ok && !ends.After(cutoff)
}

func partitionSpan(name string) (starts, ends time.Time, ok bool) {
	suffix, ok := strings.CutPrefix(name, partitionPrefix)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	if day, err := time.Parse(partitionDayLayout, suffix); err == nil && len(suffix) == len(partitionDayLayout) {
		return day, day.AddDate(0, 0, 1), true
	}
	if month, err := time.Parse(partitionMonthLayout, suffix); err == nil && len(suffix) == len(partitionMonthLayout) {
		return month, month.AddDate(0, 1, 0), true
	}
	if year, err := time.Parse(partitionYearLayout, suffix); err == nil && len(suffix) == len(partitionYearLayout) {
		return year, year.AddDate(1, 0, 0), true
	}
	return time.Time{}, time.Time{}, false
}

func utcDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
