package audit

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type writerLimits struct {
	batch   int
	every   time.Duration
	buffer  int
	wait    time.Duration
	timeout time.Duration
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
	return l
}

type Writer struct {
	recorder *Recorder
	limits   writerLimits
	queue    chan dbsqlc.InsertAuditEventParams
	lost     atomic.Int64
}

func newWriter(r *Recorder, limits writerLimits) *Writer {
	limits = limits.orDefaults()
	return &Writer{recorder: r, limits: limits, queue: make(chan dbsqlc.InsertAuditEventParams, limits.buffer)}
}

func (w *Writer) enqueue(row dbsqlc.InsertAuditEventParams) {
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

func (r *Recorder) Run(ctx context.Context) {
	r.writer.run(ctx)
}

func (w *Writer) run(ctx context.Context) {
	ticker := time.NewTicker(w.limits.every)
	defer ticker.Stop()
	batch := make([]dbsqlc.InsertAuditEventParams, 0, w.limits.batch)
	for {
		select {
		case row := <-w.queue:
			batch = append(batch, row)
			if len(batch) >= w.limits.batch {
				batch = w.flush(context.WithoutCancel(ctx), batch)
			}
		case <-ticker.C:
			batch = w.flush(context.WithoutCancel(ctx), batch)
		case <-ctx.Done():
			w.drain(context.WithoutCancel(ctx), batch)
			return
		}
	}
}

func (w *Writer) drain(ctx context.Context, batch []dbsqlc.InsertAuditEventParams) {
	for {
		select {
		case row := <-w.queue:
			batch = append(batch, row)
			if len(batch) >= w.limits.batch {
				batch = w.flush(ctx, batch)
			}
		default:
			w.flush(ctx, batch)
			return
		}
	}
}

func (w *Writer) flush(ctx context.Context, batch []dbsqlc.InsertAuditEventParams) []dbsqlc.InsertAuditEventParams {
	w.recorder.metrics.BufferedEvents(len(w.queue))
	events := len(batch)
	lost := w.lost.Swap(0)
	if lost > 0 {
		if gap, err := w.recorder.row(context.Background(), Event{Action: ActionGap, Data: map[string]any{"lost": lost}}); err == nil {
			batch = append(batch, gap)
		}
	}
	if len(batch) == 0 {
		return batch
	}
	rows := make([]dbsqlc.CopyAuditEventsParams, len(batch))
	for i, row := range batch {
		rows[i] = dbsqlc.CopyAuditEventsParams(row)
	}
	start := time.Now()
	wctx, cancel := context.WithTimeout(ctx, w.limits.timeout)
	_, err := w.recorder.db.Queries().CopyAuditEvents(wctx, rows)
	cancel()
	w.recorder.metrics.BatchWritten(time.Since(start))
	if err != nil {
		w.recorder.logger.WarnContext(ctx, "audit batch not written", logattrs.Failure("audit_write_failed", err)...)
		w.recorder.metrics.EventsDropped(DroppedWriteFailed, events)
		w.lost.Add(lost + int64(events))
		return batch[:0]
	}
	return batch[:0]
}
