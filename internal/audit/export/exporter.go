package export

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	JobName  = "audit_export"
	JobEvery = 10 * time.Minute

	BatchSize  = 500
	PollEvery  = 5 * time.Second
	MinBackoff = time.Second
	MaxBackoff = time.Minute

	advanceTimeout = 10 * time.Second

	maxSkipsInARow = 3

	DroppedRefused   = "refused"
	DroppedRetention = "retention"
)

type Source interface {
	Since(ctx context.Context, after filter.Position, limit int) ([]trail.Positioned, error)
}

type Position interface {
	Load(ctx context.Context) (Stored, error)
	Oldest(ctx context.Context) (time.Time, error)
	Advance(ctx context.Context, at filter.Position, occurredAt time.Time) error
}

type Post func(ctx context.Context, body []byte) (Receipt, error)

type Metrics interface {
	ExportRejected(n int)
	ExportDropped(reason string, n int)
}

type Exporter struct {
	Source  Source
	Cursor  Position
	Post    Post
	Metrics Metrics
	Logger  *slog.Logger

	Wait       func(ctx context.Context, d time.Duration) error
	gapSeen    filter.Position
	skipped    int
	refusedAll bool
	BatchSize  int
	PollEvery  time.Duration
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

func New(source Source, cursor Position, post Post, metrics Metrics, logger *slog.Logger) *Exporter {
	return &Exporter{
		Source: source, Cursor: cursor, Post: post, Metrics: metrics, Logger: logger,
		Wait:      wait,
		BatchSize: BatchSize, PollEvery: PollEvery, MinBackoff: MinBackoff, MaxBackoff: MaxBackoff,
	}
}

func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type stage struct {
	code string
	err  error
}

func (s stage) Error() string { return s.code }

func (s stage) Unwrap() error { return s.err }

type progress struct {
	moved    bool
	skipped  bool
	shrinkTo int
}

func (x *Exporter) Run(ctx context.Context) error {
	size := x.BatchSize
	var failures int
	var backoff time.Duration
	checkRetention := true
	for {
		p, err := x.step(ctx, size, checkRetention)
		if ctx.Err() != nil {
			return nil
		}
		checkRetention = err != nil
		var delay time.Duration
		switch {
		case err != nil:
			failures++
			backoff = x.nextBackoff(backoff, err)
			x.logFailure(ctx, err, failures, backoff)
			delay = backoff
		case p.shrinkTo > 0:
			size = p.shrinkTo
			continue
		case p.moved:
			x.recovered(ctx, failures)
			failures, backoff = 0, 0
			if p.skipped {
				size = x.BatchSize
			}
			continue
		default:
			x.recovered(ctx, failures)
			failures, backoff = 0, 0
			delay = x.PollEvery
		}
		if x.Wait(ctx, delay) != nil {
			return nil
		}
	}
}

func (x *Exporter) step(ctx context.Context, size int, checkRetention bool) (progress, error) {
	stored, err := x.Cursor.Load(ctx)
	if err != nil {
		return progress{}, stage{"audit_export_cursor_read_failed", err}
	}
	if checkRetention {
		if err := x.checkRetention(ctx, stored); err != nil {
			return progress{}, err
		}
	}
	events, err := x.Source.Since(ctx, stored.At, size)
	if err != nil {
		return progress{}, stage{"audit_export_read_failed", err}
	}
	if len(events) == 0 {
		return progress{}, nil
	}
	batch := make([]trail.Event, len(events))
	for i, e := range events {
		batch[i] = e.Event
	}
	body, err := Encode(batch)
	if err != nil {
		return progress{}, stage{"audit_export_encode_failed", err}
	}
	receipt, err := x.Post(ctx, body)
	if err != nil {
		return x.refused(ctx, events, err)
	}
	if err := x.advance(ctx, events[len(events)-1]); err != nil {
		return progress{}, stage{"audit_export_cursor_write_failed", err}
	}
	x.skipped, x.refusedAll = 0, false
	x.rejected(ctx, receipt)
	return progress{moved: true}, nil
}

func (x *Exporter) checkRetention(ctx context.Context, stored Stored) error {
	if !stored.Exists || stored.At == x.gapSeen {
		return nil
	}
	oldest, err := x.Cursor.Oldest(ctx)
	if err != nil {
		return stage{"audit_export_retention_check_failed", err}
	}
	if oldest.IsZero() || !stored.OccurredAt.Before(oldest) {
		return nil
	}
	x.gapSeen = stored.At
	x.Metrics.ExportDropped(DroppedRetention, 1)
	x.Logger.WarnContext(ctx, "audit export lost events that retention deleted before they were exported",
		slog.Duration("gap", oldest.Sub(stored.OccurredAt)))
	return nil
}

func (x *Exporter) refused(ctx context.Context, events []trail.Positioned, sendErr error) (progress, error) {
	var status *StatusError
	if !errors.As(sendErr, &status) || !refusesThePayload(status.Status) {
		return progress{}, stage{"audit_export_send_failed", sendErr}
	}
	if len(events) > 1 {
		return progress{shrinkTo: len(events) / 2}, nil
	}
	if x.skipped >= maxSkipsInARow {
		if !x.refusedAll {
			x.refusedAll = true
			x.Logger.ErrorContext(ctx, "audit export receiver refuses every event, so none is skipped any more",
				slog.Int("status", status.Status))
		}
		return progress{}, stage{"audit_export_receiver_refuses_events", sendErr}
	}
	if err := x.advance(ctx, events[0]); err != nil {
		return progress{}, stage{"audit_export_cursor_write_failed", err}
	}
	x.skipped++
	x.Metrics.ExportDropped(DroppedRefused, 1)
	x.Logger.WarnContext(ctx, "audit export skipped an event the receiver refuses",
		slog.Int64("seq", events[0].At.Seq), slog.Int("status", status.Status))
	return progress{moved: true, skipped: true}, nil
}

func refusesThePayload(status int) bool {
	return status == http.StatusBadRequest || status == http.StatusRequestEntityTooLarge || status == http.StatusUnprocessableEntity
}

func (x *Exporter) rejected(ctx context.Context, receipt Receipt) {
	if receipt.Rejected == 0 {
		return
	}
	x.Metrics.ExportRejected(int(receipt.Rejected))
	x.Logger.WarnContext(ctx, "audit export records rejected by the receiver",
		slog.Int64("rejected", receipt.Rejected), slog.String("receiver_message", receipt.Message))
}

func (x *Exporter) advance(ctx context.Context, last trail.Positioned) error {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), advanceTimeout)
	defer cancel()
	return x.Cursor.Advance(writeCtx, last.At, occurredAt(last.Event))
}

func occurredAt(e trail.Event) time.Time {
	at, _ := time.Parse(time.RFC3339Nano, e.OccurredAt)
	return at.UTC()
}

func (x *Exporter) nextBackoff(previous time.Duration, err error) time.Duration {
	next := x.MinBackoff
	if previous > 0 {
		next = min(previous*2, x.MaxBackoff)
	}
	var status *StatusError
	if errors.As(err, &status) {
		next = max(next, min(status.RetryAfter, x.MaxBackoff))
	}
	return next
}

func (x *Exporter) logFailure(ctx context.Context, err error, failures int, retryIn time.Duration) {
	code := "audit_export_failed"
	var s stage
	if errors.As(err, &s) {
		code = s.code
	}
	attrs := []any{slog.Int("failures", failures), slog.Duration("retry_in", retryIn)}
	var status *StatusError
	if errors.As(err, &status) {
		attrs = append(attrs, slog.Int("status", status.Status))
	}
	x.Logger.WarnContext(ctx, "audit export failed", logattrs.Failure(code, s.err, attrs...)...)
}

func (x *Exporter) recovered(ctx context.Context, failures int) {
	if failures > 0 {
		x.Logger.InfoContext(ctx, "audit export resumed", slog.Int("failures", failures))
	}
}
