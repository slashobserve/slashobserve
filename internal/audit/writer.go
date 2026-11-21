package audit

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	uniqueViolation       = "23505"
	lockNotAvailable      = "55P03"
	connectionException   = "08"
	insufficientResources = "53"
	operatorIntervention  = "57"
)

type writerLimits struct {
	batch   int
	every   time.Duration
	buffer  int
	wait    time.Duration
	timeout time.Duration
	settle  time.Duration
	fold    time.Duration
	folded  int
}

func (l writerLimits) orDefaults() writerLimits {
	if l.batch == 0 {
		l.batch = 500
	}
	if l.every == 0 {
		l.every = time.Second
	}
	if l.buffer == 0 {
		l.buffer = 10 * l.batch
	}
	if l.wait == 0 {
		l.wait = 100 * time.Millisecond
	}
	if l.timeout == 0 {
		l.timeout = 5 * time.Second
	}
	if l.settle == 0 {
		l.settle = 5 * time.Second
	}
	if l.fold == 0 {
		l.fold = time.Minute
	}
	if l.folded == 0 {
		l.folded = 1000
	}
	return l
}

type Writer struct {
	recorder *Recorder
	limits   writerLimits
	queue    chan dbsqlc.InsertAuditEventParams
	folds    *folds
	lost     atomic.Int64
	held     [][]dbsqlc.InsertAuditEventParams

	mu      sync.Mutex
	handed  []dbsqlc.InsertAuditEventParams
	stopped bool
}

func newWriter(r *Recorder, limits writerLimits) *Writer {
	limits = limits.orDefaults()
	return &Writer{recorder: r, limits: limits, queue: make(chan dbsqlc.InsertAuditEventParams, limits.buffer), folds: newFolds(limits.fold, limits.folded)}
}

func (w *Writer) enqueue(row dbsqlc.InsertAuditEventParams) {
	row, enqueue := w.folds.absorb(row, time.Now())
	if !enqueue {
		return
	}
	select {
	case w.queue <- row:
		return
	default:
	}
	wait := time.NewTimer(w.limits.wait)
	defer wait.Stop()
	select {
	case w.queue <- row:
	case <-wait.C:
		w.lost.Add(1)
		w.recorder.metrics.EventsDropped(DroppedBufferFull, 1)
	}
}

func (w *Writer) handOff(row dbsqlc.InsertAuditEventParams) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped || len(w.handed) >= w.limits.buffer {
		return false
	}
	w.handed = append(w.handed, row)
	return true
}

func (w *Writer) holdHandedOff(closing bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, row := range w.handed {
		w.held = append(w.held, []dbsqlc.InsertAuditEventParams{row})
	}
	w.handed = nil
	w.stopped = w.stopped || closing
}

func (r *Recorder) Run(ctx context.Context) {
	r.writer.run(ctx)
}

func (w *Writer) run(ctx context.Context) {
	ticker := time.NewTicker(w.limits.every)
	defer ticker.Stop()
	batch := make([]dbsqlc.InsertAuditEventParams, 0, w.limits.batch)
	for {
		queue := w.queue
		if len(batch) >= w.limits.batch {
			queue = nil
		}
		select {
		case row := <-queue:
			batch = append(batch, row)
			if len(batch) >= w.limits.batch {
				batch = w.flush(context.WithoutCancel(ctx), batch)
			}
		case <-ticker.C:
			batch = append(batch, w.folds.due(time.Now())...)
			batch = w.flush(context.WithoutCancel(ctx), batch)
		case <-ctx.Done():
			w.stop(context.WithoutCancel(ctx), batch)
			return
		}
	}
}

func (w *Writer) stop(ctx context.Context, batch []dbsqlc.InsertAuditEventParams) {
	settled := time.Now().Add(w.limits.settle)
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for w.recorder.inflight.Load() > 0 && time.Now().Before(settled) {
		select {
		case row := <-w.queue:
			batch = append(batch, row)
			if len(batch) >= w.limits.batch {
				batch = w.flush(ctx, batch)
			}
		case <-poll.C:
		}
	}
	if n := w.recorder.inflight.Load(); n > 0 {
		w.recorder.logger.WarnContext(ctx, "audit writer stopped with requests still running", slog.Int64("requests", n))
	}
	batch = append(batch, w.folds.drain()...)
	dctx, cancel := context.WithTimeout(ctx, w.limits.timeout)
	defer cancel()
	w.drain(dctx, batch)
}

func (w *Writer) drain(ctx context.Context, batch []dbsqlc.InsertAuditEventParams) {
	for {
		if ctx.Err() != nil {
			w.holdHandedOff(true)
			w.abandonAll(batch, len(w.queue))
			return
		}
		select {
		case row := <-w.queue:
			batch = append(batch, row)
			if len(batch) >= w.limits.batch {
				batch = w.flush(ctx, batch)
			}
		default:
			w.holdHandedOff(true)
			w.abandonAll(w.flush(ctx, batch), 0)
			return
		}
	}
}

func (w *Writer) flush(ctx context.Context, batch []dbsqlc.InsertAuditEventParams) []dbsqlc.InsertAuditEventParams {
	w.recorder.metrics.BufferedEvents(len(w.queue))
	w.holdHandedOff(false)
	if !w.retryHeld(ctx) {
		return batch
	}
	lost := w.lost.Swap(0)
	rows := batch
	if lost > 0 {
		if gap, err := w.recorder.row(context.Background(), Event{Action: ActionGap, Data: map[string]any{"lost": lost}}); err == nil {
			rows = append(slices.Clip(batch), gap)
		} else {
			w.lost.Add(lost)
		}
	}
	if len(rows) == 0 {
		return batch
	}
	err := w.write(ctx, rows)
	if err == nil {
		return batch[:0]
	}
	w.recorder.logger.WarnContext(ctx, "audit batch not written", logattrs.Failure("audit_write_failed", err)...)
	if retryable(err) {
		w.held = append(w.held, slices.Clone(rows))
	} else {
		w.abandon(rows)
	}
	return batch[:0]
}

func (w *Writer) retryHeld(ctx context.Context) bool {
	for len(w.held) > 0 {
		rows := w.held[0]
		err := w.write(ctx, rows)
		switch {
		case err == nil:
		case alreadyStored(err):
			w.recorder.logger.InfoContext(ctx, "audit batch was already stored by an earlier attempt")
		case retryable(err):
			w.recorder.logger.WarnContext(ctx, "audit batch not written", logattrs.Failure("audit_write_failed", err)...)
			return false
		default:
			w.recorder.logger.WarnContext(ctx, "audit batch not written", logattrs.Failure("audit_write_failed", err)...)
			w.abandon(rows)
		}
		w.held = w.held[1:]
	}
	w.held = nil
	return true
}

func (w *Writer) abandonAll(batch []dbsqlc.InsertAuditEventParams, queued int) {
	events, lost := tally(batch)
	for _, rows := range w.held {
		heldEvents, heldLost := tally(rows)
		events += heldEvents
		lost += heldLost
	}
	w.held = nil
	events += queued
	lost += int64(queued)
	if events > 0 {
		w.recorder.metrics.EventsDropped(DroppedWriteFailed, events)
	}
	w.lost.Add(lost)
}

func (w *Writer) abandon(rows []dbsqlc.InsertAuditEventParams) {
	events, lost := tally(rows)
	if events > 0 {
		w.recorder.metrics.EventsDropped(DroppedWriteFailed, events)
	}
	w.lost.Add(lost)
}

func tally(rows []dbsqlc.InsertAuditEventParams) (events int, lost int64) {
	for _, row := range rows {
		if row.Action == string(ActionGap) {
			lost += gapSize(row)
			continue
		}
		events += weightOf(row)
	}
	return events, lost + int64(events)
}

func gapSize(row dbsqlc.InsertAuditEventParams) int64 {
	var data struct {
		Lost int64 `json:"lost"`
	}
	_ = json.Unmarshal(row.Data, &data)
	return data.Lost
}

func (w *Writer) write(ctx context.Context, batch []dbsqlc.InsertAuditEventParams) error {
	rows := make([]dbsqlc.CopyAuditEventsParams, len(batch))
	for i, row := range batch {
		rows[i] = dbsqlc.CopyAuditEventsParams(row)
	}
	start := time.Now()
	wctx, cancel := context.WithTimeout(ctx, w.limits.timeout)
	defer cancel()
	_, err := w.recorder.db.Queries().CopyAuditEvents(wctx, rows)
	w.recorder.metrics.BatchWritten(time.Since(start))
	return err
}

func alreadyStored(err error) bool {
	var failure *pgconn.PgError
	return errors.As(err, &failure) && failure.Code == uniqueViolation
}

func retryable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err) || pgconn.SafeToRetry(err) {
		return true
	}
	var failure *pgconn.PgError
	if !errors.As(err, &failure) {
		return false
	}
	return failure.Code == lockNotAvailable || strings.HasPrefix(failure.Code, connectionException) || strings.HasPrefix(failure.Code, operatorIntervention) || strings.HasPrefix(failure.Code, insufficientResources)
}
