package shopapp

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/slashobserve/slashobserve/demo/internal/fault"
	"github.com/slashobserve/slashobserve/demo/internal/metrics"
	"github.com/slashobserve/slashobserve/demo/internal/queue"
)

const (
	maxQueuedOrders    = 10000
	minOrderProcessing = 800 * time.Millisecond
	maxOrderProcessing = 1600 * time.Millisecond
	orderFailRate      = 0.01
)

type queuedOrder struct {
	id     int64
	parent trace.SpanContext
}

func (a *app) startOrdersQueue(ctx context.Context) {
	a.orders = queue.New(a.cfg.QueueWorkers, maxQueuedOrders,
		func() int { return a.faults.Pct(fault.Queue) },
		func(job queue.Job[queuedOrder]) { a.processOrder(ctx, job) },
	)
	a.recorder.WatchOrdersQueue(a.orders.Depth, a.orders.OldestAge)
	go a.orders.Run(ctx)
}

func (a *app) enqueueOrder(ctx context.Context, id int64) {
	if a.orders.Push(queuedOrder{id: id, parent: trace.SpanContextFromContext(ctx)}) {
		return
	}
	a.recorder.OrderDropped()
	a.log.ErrorContext(ctx, "order queue full", "order_id", id)
}

func (a *app) processOrder(ctx context.Context, job queue.Job[queuedOrder]) {
	wait := time.Since(job.Enqueued)
	ctx = trace.ContextWithSpanContext(ctx, job.Value.parent)
	started := time.Now()
	ctx, span, ok := a.work(ctx, "process_order", minOrderProcessing, maxOrderProcessing)
	defer span.End()
	span.SetAttributes(attribute.Int64("order.id", job.Value.id), attribute.Int64("queue.wait_ms", wait.Milliseconds()))
	if !ok {
		return
	}
	result, level := metrics.OrderOK, slog.LevelInfo
	if a.rng.Chance(orderFailRate) {
		result, level = metrics.OrderFailed, slog.LevelError
		failSpan(span, "order processing failed")
	}
	a.recorder.OrderProcessed(result, time.Since(started))
	a.log.Log(ctx, level, "order processed", "order_id", job.Value.id, "queue_wait_ms", wait.Milliseconds(), "result", result)
}
