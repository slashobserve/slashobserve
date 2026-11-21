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
	queryCanceled         = "57014"
	lockNotAvailable      = "55P03"
	readOnlyTransaction   = "25006"
	connectionException   = "08"
	insufficientResources = "53"
	operatorIntervention  = "57"
)

const splitAfterTimeouts = 3

var permanentCodes = []string{"08P01", "53400", "57P04"}

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

type statement struct {
	rows     []dbsqlc.InsertAuditEventParams
	handed   bool
	timeouts int
}

type Writer struct {
	recorder *Recorder
	limits   writerLimits
	queue    chan dbsqlc.InsertAuditEventParams
	folds    *folds
	lost     atomic.Int64
	held     []*statement

	mu         sync.Mutex
	handed     []dbsqlc.InsertAuditEventParams
	handedRows int
	stopped    bool
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
	if w.stopped || w.handedRows >= w.limits.buffer {
		return false
	}
	w.handedRows++
	w.handed = append(w.handed, row)
	return true
}

func (w *Writer) releaseHanded(rows int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handedRows -= rows
}

func (w *Writer) holdHandedOff(closing bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, row := range w.handed {
		w.held = append(w.held, &statement{rows: []dbsqlc.InsertAuditEventParams{row}, handed: true})
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
			if len(batch) < w.limits.batch {
				batch = append(batch, w.folds.due(time.Now())...)
			}
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
			w.abandonAll(w.flushUntilNothingMovesOrIsLeft(ctx, batch), 0)
			return
		}
	}
}

func (w *Writer) flushUntilNothingMovesOrIsLeft(ctx context.Context, batch []dbsqlc.InsertAuditEventParams) []dbsqlc.InsertAuditEventParams {
	for {
		before := len(w.held)
		batch = w.flush(ctx, batch)
		if len(w.held) == 0 || len(w.held) >= before || ctx.Err() != nil {
			return batch
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
		w.held = append(w.held, &statement{rows: slices.Clone(rows)})
	} else {
		w.abandon(rows)
	}
	return batch[:0]
}

func (w *Writer) retryHeld(ctx context.Context) bool {
	for written := 0; len(w.held) > 0; written++ {
		if written >= w.limits.batch {
			return true
		}
		st := w.held[0]
		err := w.write(ctx, st.rows)
		switch {
		case err == nil:
		case alreadyStored(err):
			w.recorder.logger.InfoContext(ctx, "audit batch was already stored by an earlier attempt")
		case retryable(err):
			w.recorder.logger.WarnContext(ctx, "audit batch not written", logattrs.Failure("audit_write_failed", err)...)
			if w.tooBig(st, err) {
				w.split(st)
			}
			return false
		default:
			w.recorder.logger.WarnContext(ctx, "audit batch not written", logattrs.Failure("audit_write_failed", err)...)
			w.abandon(st.rows)
		}
		w.held = w.held[1:]
		if st.handed {
			w.releaseHanded(1)
		}
	}
	w.held = nil
	return true
}

func (w *Writer) tooBig(st *statement, err error) bool {
	if len(st.rows) < 2 {
		return false
	}
	if tooSlow(err) {
		return true
	}
	if !timedOut(err) {
		st.timeouts = 0
		return false
	}
	st.timeouts++
	return st.timeouts >= splitAfterTimeouts
}

func (w *Writer) split(st *statement) {
	half := len(st.rows) / 2
	halves := []*statement{{rows: st.rows[:half]}, {rows: st.rows[half:]}}
	w.held = append(halves, w.held[1:]...)
}

func (w *Writer) abandonAll(batch []dbsqlc.InsertAuditEventParams, queued int) {
	events, lost := tally(batch)
	handed := 0
	for _, st := range w.held {
		heldEvents, heldLost := tally(st.rows)
		events += heldEvents
		lost += heldLost
		if st.handed {
			handed++
		}
	}
	w.held = nil
	w.releaseHanded(handed)
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

func timedOut(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err)
}

func tooSlow(err error) bool {
	var failure *pgconn.PgError
	return errors.As(err, &failure) && failure.Code == queryCanceled
}

func retryable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err) || pgconn.SafeToRetry(err) {
		return true
	}
	var failure *pgconn.PgError
	if !errors.As(err, &failure) {
		return false
	}
	if slices.Contains(permanentCodes, failure.Code) {
		return false
	}
	return failure.Code == lockNotAvailable || failure.Code == readOnlyTransaction || strings.HasPrefix(failure.Code, connectionException) || strings.HasPrefix(failure.Code, operatorIntervention) || strings.HasPrefix(failure.Code, insufficientResources)
}
