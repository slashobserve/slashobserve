package export

import (
	"context"
	"errors"
	"log/slog"
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
)

type Source interface {
	Since(ctx context.Context, after filter.Position, limit int) ([]trail.Positioned, error)
}

type Position interface {
	Load(ctx context.Context) (filter.Position, error)
	Advance(ctx context.Context, at filter.Position) error
}

type Post func(ctx context.Context, body []byte) (Receipt, error)

type Metrics interface {
	ExportLag(behind time.Duration)
	ExportRejected(n int)
}

type Exporter struct {
	Source  Source
	Cursor  Position
	Post    Post
	Metrics Metrics
	Logger  *slog.Logger

	Now        func() time.Time
	Wait       func(ctx context.Context, d time.Duration) error
	BatchSize  int
	PollEvery  time.Duration
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

func New(source Source, cursor Position, post Post, metrics Metrics, logger *slog.Logger) *Exporter {
	return &Exporter{
		Source: source, Cursor: cursor, Post: post, Metrics: metrics, Logger: logger,
		Now: time.Now, Wait: wait,
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

func (x *Exporter) Run(ctx context.Context) error {
	defer x.Metrics.ExportLag(0)
	var failures int
	var backoff time.Duration
	for {
		moved, err := x.step(ctx)
		if ctx.Err() != nil {
			return nil
		}
		var delay time.Duration
		switch {
		case err != nil:
			failures++
			backoff = x.nextBackoff(backoff, err)
			x.logFailure(ctx, err, failures, backoff)
			delay = backoff
		case moved:
			x.recovered(ctx, failures)
			failures, backoff = 0, 0
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

func (x *Exporter) step(ctx context.Context) (bool, error) {
	at, err := x.Cursor.Load(ctx)
	if err != nil {
		return false, stage{"audit_export_cursor_read_failed", err}
	}
	events, err := x.Source.Since(ctx, at, x.BatchSize)
	if err != nil {
		return false, stage{"audit_export_read_failed", err}
	}
	if len(events) == 0 {
		x.Metrics.ExportLag(0)
		return false, nil
	}
	x.Metrics.ExportLag(x.behind(events[0].Event))
	batch := make([]trail.Event, len(events))
	for i, e := range events {
		batch[i] = e.Event
	}
	body, err := Encode(batch)
	if err != nil {
		return false, stage{"audit_export_encode_failed", err}
	}
	receipt, err := x.Post(ctx, body)
	if err != nil {
		return false, stage{"audit_export_send_failed", err}
	}
	if err := x.Cursor.Advance(ctx, events[len(events)-1].At); err != nil {
		return false, stage{"audit_export_cursor_write_failed", err}
	}
	x.rejected(ctx, receipt)
	return true, nil
}

func (x *Exporter) rejected(ctx context.Context, receipt Receipt) {
	if receipt.Rejected == 0 {
		return
	}
	x.Metrics.ExportRejected(int(receipt.Rejected))
	x.Logger.WarnContext(ctx, "audit export records rejected by the receiver",
		slog.Int64("rejected", receipt.Rejected), slog.String("receiver_message", receipt.Message))
}

func (x *Exporter) behind(oldest trail.Event) time.Duration {
	at, err := time.Parse(time.RFC3339Nano, oldest.OccurredAt)
	if err != nil {
		return 0
	}
	return max(x.Now().Sub(at), 0)
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
