package shopapp

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/slashobserve/slashobserve/demo/internal/fault"
	"github.com/slashobserve/slashobserve/demo/internal/inventory"
)

const (
	dbPoolSize         = 5
	minSlowDBDelay     = 300 * time.Millisecond
	maxSlowDBDelay     = 900 * time.Millisecond
	slowQueryThreshold = 200 * time.Millisecond
	cacheHitRatio      = 0.85

	queryGetProduct   = "get_product"
	queryReserveStock = "reserve_stock"
)

func (a *app) openInventory() {
	a.db = inventory.NewPool(dbPoolSize)
	a.recorder.WatchDBPool(a.db.InUse, a.db.Size())
}

func (a *app) lookupProduct(ctx context.Context, lo, hi time.Duration) bool {
	took := a.rng.Between(lo, hi)
	hit := a.rng.Chance(cacheHitRatio)
	a.recorder.CacheLookup(hit)
	if hit {
		return sleep(ctx, took)
	}
	return a.query(ctx, queryGetProduct, took)
}

func (a *app) query(ctx context.Context, name string, took time.Duration) bool {
	ctx, span := otel.Tracer("shop").Start(ctx, "db.query", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.namespace", "inventory"),
		attribute.String("db.operation.name", name),
	))
	defer span.End()
	if a.rng.Percent(a.faults.Pct(fault.SlowDB)) {
		took += a.rng.Between(minSlowDBDelay, maxSlowDBDelay)
	}
	started := time.Now()
	ok := a.db.Hold(ctx, took)
	elapsed := time.Since(started)
	a.recorder.ObserveQuery(name, elapsed)
	if elapsed > slowQueryThreshold {
		a.log.WarnContext(ctx, "slow query", "db.system", "postgresql", "db.operation.name", name, "duration_ms", elapsed.Milliseconds())
	}
	return ok
}
