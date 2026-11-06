package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	runBudget        = 8 * time.Minute
	checkParallelism = 2
	batchSize        = 20
	recheckAfter     = 23 * time.Hour
	recordTimeout    = 5 * time.Second

	SkippedDependsOnViewer = "depends_on_viewer"
	codeCheckFailed        = "check_failed"
	checkFailedMessage     = "the health check could not run; the server log has the cause"
)

var checkRange = spec.TimeRange{From: "now-7d", To: "now"}

type runCounts struct {
	mu                                 sync.Mutex
	checked, unhealthy, skipped, fails int
}

func (c *runCounts) add(r checkResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checked++
	switch {
	case r.skipped != "":
		c.skipped++
	case r.failed:
		c.fails++
	case len(r.problems) > 0:
		c.unhealthy++
	}
}

type checkResult struct {
	skipped  string
	problems []Problem
	failed   bool
}

func (s *Service) Run(ctx context.Context) error {
	budget, cancel := context.WithTimeout(ctx, runBudget)
	defer cancel()
	started := time.Now()
	var counts runCounts
	for budget.Err() == nil {
		rows, err := s.db.Queries().DashboardsDueForHealth(budget, dbsqlc.DashboardsDueForHealthParams{
			CheckedBefore: pgtype.Timestamptz{Time: started.Add(-recheckAfter), Valid: true},
			RowLimit:      batchSize,
		})
		if budget.Err() != nil {
			break
		}
		if err != nil {
			return fmt.Errorf("list dashboards due for a health check: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		if err := s.checkBatch(budget, rows, &counts); err != nil {
			return err
		}
	}
	s.logger.InfoContext(ctx, "dashboard health run finished",
		slog.Int("checked", counts.checked),
		slog.Int("unhealthy", counts.unhealthy),
		slog.Int("skipped", counts.skipped),
		slog.Int("failed", counts.fails),
		slog.Bool("budget_spent", errors.Is(budget.Err(), context.DeadlineExceeded) && ctx.Err() == nil),
		slog.Int64("duration_ms", time.Since(started).Milliseconds()))
	return ctx.Err()
}

func (s *Service) checkBatch(ctx context.Context, rows []dbsqlc.DashboardsDueForHealthRow, counts *runCounts) error {
	var group errgroup.Group
	group.SetLimit(checkParallelism)
	for _, row := range rows {
		group.Go(func() error {
			if ctx.Err() != nil {
				return nil
			}
			result := s.check(ctx, row)
			if ctx.Err() != nil {
				return nil
			}
			if err := s.record(ctx, row, result); err != nil {
				return err
			}
			counts.add(result)
			return nil
		})
	}
	return group.Wait()
}

func (s *Service) check(ctx context.Context, row dbsqlc.DashboardsDueForHealthRow) checkResult {
	var d spec.Dashboard
	if err := json.Unmarshal(row.Spec, &d); err != nil {
		return s.checkFailed(ctx, row, fmt.Errorf("decode stored spec: %w", err))
	}
	if queryproxy.DashboardUsesUser(&d) {
		return checkResult{skipped: SkippedDependsOnViewer}
	}
	d.Time = checkRange
	issues, err := s.runner.CheckSpec(ctx, &d, time.Now(), datasource.QueryUser{})
	if err != nil {
		return s.checkFailed(ctx, row, err)
	}
	return checkResult{problems: classify(&d, issues)}
}

func (s *Service) checkFailed(ctx context.Context, row dbsqlc.DashboardsDueForHealthRow, err error) checkResult {
	if ctx.Err() == nil {
		s.logger.WarnContext(ctx, "dashboard health check failed", logattrs.Failure(codeCheckFailed, err, slog.String("dashboard_id", row.ID.String()))...)
	}
	return checkResult{failed: true, problems: []Problem{{Kind: KindQueryFailed, Code: codeCheckFailed, Message: checkFailedMessage}}}
}

func (s *Service) record(ctx context.Context, row dbsqlc.DashboardsDueForHealthRow, r checkResult) error {
	problems := r.problems
	if problems == nil {
		problems = []Problem{}
	}
	raw, err := json.Marshal(problems)
	if err != nil {
		return fmt.Errorf("encode health problems: %w", err)
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	err = s.db.Queries().UpsertDashboardHealth(recordCtx, dbsqlc.UpsertDashboardHealthParams{
		DashboardID: row.ID,
		Version:     row.LatestVersion,
		CheckedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Skipped:     r.skipped,
		Issues:      raw,
	})
	if db.IsForeignKeyViolation(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("record dashboard health: %w", err)
	}
	return nil
}
