package export

import (
	"context"
	"log/slog"
	"time"

	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const LagEvery = 15 * time.Second

type LagSource interface {
	Lag(ctx context.Context) (time.Duration, error)
}

type LagGauge interface {
	ExportLag(behind time.Duration)
}

type LagWatcher struct {
	Source LagSource
	Gauge  LagGauge
	Logger *slog.Logger
	Wait   func(ctx context.Context, d time.Duration) error
}

func NewLagWatcher(source LagSource, gauge LagGauge, logger *slog.Logger) *LagWatcher {
	return &LagWatcher{Source: source, Gauge: gauge, Logger: logger, Wait: wait}
}

func (w *LagWatcher) Run(ctx context.Context) {
	for {
		lag, err := w.Source.Lag(ctx)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			w.Logger.WarnContext(ctx, "audit export lag unavailable", logattrs.Failure("audit_export_lag_failed", err)...)
		default:
			w.Gauge.ExportLag(lag)
		}
		if w.Wait(ctx, LagEvery) != nil {
			return
		}
	}
}
