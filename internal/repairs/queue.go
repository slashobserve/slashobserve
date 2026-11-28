package repairs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type Planner func(ctx context.Context, q *dbsqlc.Queries, levels folders.Levels, id pgtype.UUID) (latest int, prompt, reason string, err error)

type Queue struct {
	db        *db.DB
	access    *folders.Access
	revisions *revisions.Service
}

func NewQueue(d *db.DB, access *folders.Access, revisions *revisions.Service) *Queue {
	return &Queue{db: d, access: access, revisions: revisions}
}

type Batch struct {
	queue  *Queue
	by     auth.Principal
	Levels folders.Levels
	plan   Planner
	seen   map[pgtype.UUID]bool
	out    Outcome
}

func (q *Queue) Begin(ctx context.Context, by auth.Principal, plan Planner) (*Batch, error) {
	if !q.revisions.Available() {
		return nil, revisions.ErrUnavailable
	}
	levels, err := q.access.Levels(ctx, by)
	if err != nil {
		return nil, err
	}
	return &Batch{queue: q, by: by, Levels: levels, plan: plan, seen: map[pgtype.UUID]bool{}, out: Outcome{Queued: []Queued{}, Skipped: []Skipped{}}}, nil
}

func (b *Batch) Outcome() Outcome { return b.out }

func (b *Batch) Each(ctx context.Context, ids []pgtype.UUID) error {
	for _, id := range ids {
		if err := b.Add(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (b *Batch) Add(ctx context.Context, id pgtype.UUID) error {
	if b.seen[id] {
		return nil
	}
	b.seen[id] = true
	var reason string
	var queued dbsqlc.AgentRequest
	err := b.queue.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockAgentRequestsForDashboard(ctx, id); err != nil {
			return fmt.Errorf("lock agent requests: %w", err)
		}
		latest, prompt, why, err := b.plan(ctx, q, b.Levels, id)
		if err != nil || why != "" {
			reason = why
			return err
		}
		queued, err = b.queue.revisions.Enqueue(ctx, q, revisions.EnqueueParams{DashboardID: id, Prompt: prompt, BaseVersion: latest, By: b.by})
		return err
	})
	switch {
	case db.IsForeignKeyViolation(err):
		reason = ReasonNotFound
	case err != nil:
		return err
	}
	if reason != "" {
		b.out.Skipped = append(b.out.Skipped, Skipped{DashboardID: id.String(), Reason: reason})
		return nil
	}
	b.out.Queued = append(b.out.Queued, Queued{DashboardID: id.String(), RequestID: queued.ID.String()})
	return nil
}

func Admit(ctx context.Context, q *dbsqlc.Queries, levels folders.Levels, id, folder pgtype.UUID, provisioned bool) (string, error) {
	abilities := levels.Of(folder)
	switch {
	case !abilities.View:
		return ReasonNotFound, nil
	case !abilities.Edit:
		return ReasonForbidden, nil
	case provisioned:
		return ReasonProvisioned, nil
	}
	active, err := q.CountActiveAgentRequests(ctx, id)
	if err != nil {
		return "", fmt.Errorf("count active agent requests: %w", err)
	}
	if active > 0 {
		return ReasonActive, nil
	}
	return "", nil
}
