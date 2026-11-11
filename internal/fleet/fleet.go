package fleet

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	beatEvery    = 10 * time.Second
	staleAfter   = 30 * time.Second
	leaveTimeout = 5 * time.Second
)

type Membership struct {
	db      *db.DB
	id      string
	version string
	logger  *slog.Logger
	beat    time.Duration
	stale   time.Duration
	live    atomic.Int64
}

func New(database *db.DB, id, version string, logger *slog.Logger) *Membership {
	return &Membership{db: database, id: id, version: version, logger: logger, beat: beatEvery, stale: staleAfter}
}

func (m *Membership) Live() int {
	return max(1, int(m.live.Load()))
}

func (m *Membership) Share(total int) int {
	if total < 1 {
		return 1
	}
	live := m.Live()
	return max(1, (total+live-1)/live)
}

func (m *Membership) Run(ctx context.Context) {
	defer m.leave(ctx)
	ticker := time.NewTicker(m.beat)
	defer ticker.Stop()
	failing := false
	for {
		err := m.heartbeat(ctx)
		switch {
		case err == nil:
			failing = false
		case ctx.Err() != nil:
			return
		case !failing:
			failing = true
			m.logger.WarnContext(ctx, "server heartbeat failed", logattrs.Failure("fleet_heartbeat_failed", err)...)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Membership) heartbeat(ctx context.Context) error {
	var live int32
	err := m.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.UpsertServerInstance(ctx, dbsqlc.UpsertServerInstanceParams{ID: m.id, Version: m.version}); err != nil {
			return err
		}
		if err := q.DeleteStaleServerInstances(ctx, m.stale.Seconds()); err != nil {
			return err
		}
		count, err := q.CountLiveServerInstances(ctx, m.stale.Seconds())
		live = count
		return err
	})
	if err != nil {
		return err
	}
	m.live.Store(int64(max(1, live)))
	return nil
}

func (m *Membership) leave(ctx context.Context) {
	leaveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), leaveTimeout)
	defer cancel()
	if err := m.db.Queries().DeleteServerInstance(leaveCtx, m.id); err != nil {
		m.logger.WarnContext(ctx, "server leave failed", logattrs.Failure("fleet_leave_failed", err)...)
	}
}
