package health

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/health/repairprompt"
	"github.com/slashobserve/slashobserve/internal/repairs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type Repairs struct {
	queue *repairs.Queue
}

func NewRepairs(svc *Service, revisions *revisions.Service) *Repairs {
	return &Repairs{queue: repairs.NewQueue(svc.db, svc.access, revisions)}
}

func (r *Repairs) Repair(ctx context.Context, by auth.Principal, ids []pgtype.UUID) (repairs.Outcome, error) {
	batch, err := r.queue.Begin(ctx, by, healthPlan)
	if err != nil {
		return repairs.Outcome{}, err
	}
	if err := batch.Each(ctx, ids); err != nil {
		return repairs.Outcome{}, err
	}
	return batch.Outcome(), nil
}

func healthPlan(ctx context.Context, q *dbsqlc.Queries, requester pgtype.UUID, levels folders.Levels, id pgtype.UUID) (int, string, string, error) {
	target, err := q.HealthRepairTarget(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", repairs.ReasonNotFound, nil
	}
	if err != nil {
		return 0, "", "", fmt.Errorf("read health repair target: %w", err)
	}
	if reason, err := repairs.Admit(ctx, q, levels, id, target.FolderID, target.Provisioned); err != nil || reason != "" {
		return 0, "", reason, err
	}
	if target.CheckedVersion != target.LatestVersion {
		return 0, "", repairs.ReasonNothingToRepair, nil
	}
	problems, err := decodeProblems(target.Issues)
	if err != nil {
		return 0, "", "", err
	}
	prompt, ok := repairPrompt(problems)
	if !ok {
		return 0, "", repairs.ReasonNothingToRepair, nil
	}
	mine, err := q.CountActiveHealthRepairsBy(ctx, dbsqlc.CountActiveHealthRepairsByParams{RequesterID: requester, PromptPrefix: repairprompt.Prefix})
	if err != nil {
		return 0, "", "", fmt.Errorf("count active health repairs: %w", err)
	}
	if mine >= repairs.MaxActivePerRequester {
		return 0, "", repairs.ReasonQueueFull, nil
	}
	return int(target.LatestVersion), prompt, "", nil
}
