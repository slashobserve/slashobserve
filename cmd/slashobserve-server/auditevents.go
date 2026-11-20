package main

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

func recordSystemEvent(ctx context.Context, logger *slog.Logger, e audit.Event) {
	if err := audit.RecordNow(context.WithoutCancel(ctx), e); err != nil {
		logger.Warn("audit event not recorded", logattrs.Failure("audit_record_failed", err, slog.String("action", string(e.Action)))...)
	}
}

func recordMigrations(ctx context.Context, logger *slog.Logger, applied []db.AppliedMigration) {
	for _, m := range applied {
		recordSystemEvent(ctx, logger, audit.Event{
			Action:   "migration.apply",
			Target:   audit.Target{Kind: "migration", ID: strconv.FormatInt(m.Version, 10)},
			Duration: m.Duration,
		})
	}
}

func serverTarget(serverID string) audit.Target {
	return audit.Target{Kind: "server", ID: serverID}
}

func recordServerStop(ctx context.Context, logger *slog.Logger, serverID string, served error) {
	e := audit.Event{Action: "server.stop", Target: serverTarget(serverID)}
	if served != nil {
		e.Outcome, e.Reason = audit.OutcomeFailed, "http_server_failed"
	}
	recordSystemEvent(ctx, logger, e)
}
