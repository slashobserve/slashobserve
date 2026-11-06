package health

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/repairs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type Repairs struct {
	svc       *Service
	revisions *revisions.Service
}

func NewRepairs(svc *Service, revisions *revisions.Service) *Repairs {
	return &Repairs{svc: svc, revisions: revisions}
}

func (r *Repairs) Repair(ctx context.Context, by auth.Principal, ids []pgtype.UUID) (repairs.Outcome, error) {
	if !r.revisions.Available() {
		return repairs.Outcome{}, revisions.ErrUnavailable
	}
	requester, err := db.ParseUUID(by.ID)
	if err != nil {
		return repairs.Outcome{}, fmt.Errorf("requester id: %w", err)
	}
	levels, err := r.svc.access.Levels(ctx, by)
	if err != nil {
		return repairs.Outcome{}, err
	}
	out := repairs.Outcome{Queued: []repairs.Queued{}, Skipped: []repairs.Skipped{}}
	seen := make(map[pgtype.UUID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := r.repairOne(ctx, by, requester, levels, id, &out); err != nil {
			return repairs.Outcome{}, err
		}
	}
	return out, nil
}

func (r *Repairs) repairOne(ctx context.Context, by auth.Principal, requester pgtype.UUID, levels folders.Levels, id pgtype.UUID, out *repairs.Outcome) error {
	var reason string
	var queued dbsqlc.AgentRequest
	err := r.svc.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		latest, prompt, why, err := repairTarget(ctx, q, requester, levels, id)
		if err != nil || why != "" {
			reason = why
			return err
		}
		queued, err = r.revisions.Enqueue(ctx, q, revisions.EnqueueParams{
			DashboardID: id,
			Source:      revisions.SourcePrompt,
			Prompt:      prompt,
			BaseVersion: latest,
			By:          by,
		})
		return err
	})
	switch {
	case db.IsForeignKeyViolation(err):
		reason = repairs.ReasonNotFound
	case err != nil:
		return err
	}
	if reason != "" {
		out.Skipped = append(out.Skipped, repairs.Skipped{DashboardID: id.String(), Reason: reason})
		return nil
	}
	out.Queued = append(out.Queued, repairs.Queued{DashboardID: id.String(), RequestID: queued.ID.String()})
	return nil
}

func repairTarget(ctx context.Context, q *dbsqlc.Queries, requester pgtype.UUID, levels folders.Levels, id pgtype.UUID) (int, string, string, error) {
	if err := q.LockAgentRequestsForDashboard(ctx, id); err != nil {
		return 0, "", "", fmt.Errorf("lock agent requests: %w", err)
	}
	target, err := q.HealthRepairTarget(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", repairs.ReasonNotFound, nil
	}
	if err != nil {
		return 0, "", "", fmt.Errorf("read health repair target: %w", err)
	}
	abilities := levels.Of(target.FolderID)
	if !abilities.View {
		return 0, "", repairs.ReasonNotFound, nil
	}
	if !abilities.Edit {
		return 0, "", repairs.ReasonForbidden, nil
	}
	active, err := q.CountActiveAgentRequests(ctx, id)
	if err != nil {
		return 0, "", "", fmt.Errorf("count active agent requests: %w", err)
	}
	if active > 0 {
		return 0, "", repairs.ReasonActive, nil
	}
	problems, err := decodeProblems(target.Issues)
	if err != nil {
		return 0, "", "", err
	}
	prompt, ok := repairPrompt(problems)
	if !ok {
		return 0, "", repairs.ReasonNothingToRepair, nil
	}
	mine, err := q.CountActiveHealthRepairsBy(ctx, dbsqlc.CountActiveHealthRepairsByParams{RequesterID: requester, PromptPrefix: repairPromptPrefix})
	if err != nil {
		return 0, "", "", fmt.Errorf("count active health repairs: %w", err)
	}
	if mine >= repairs.MaxActivePerRequester {
		return 0, "", repairs.ReasonQueueFull, nil
	}
	return int(target.LatestVersion), prompt, "", nil
}
