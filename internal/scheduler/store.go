package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

var (
	errNothingToClaim   = errors.New("no registered job is due and free")
	errEventNotRecorded = errors.New("the job finished but its audit event was not recorded")
)

type claim struct {
	name      string
	startedAt pgtype.Timestamptz
	deadline  time.Time
}

type store struct {
	db         *db.DB
	instanceID string
}

func (s *store) upsert(ctx context.Context, j job) error {
	return s.db.Queries().UpsertJob(ctx, dbsqlc.UpsertJobParams{Name: j.name, EverySeconds: httpapi.Int32(int(j.every / time.Second))})
}

func (s *store) claim(ctx context.Context, names []string, force bool) (claim, error) {
	requested := time.Now()
	row, err := s.db.Queries().ClaimJob(ctx, dbsqlc.ClaimJobParams{
		InstanceID:      s.instanceID,
		MaxLeaseSeconds: httpapi.Int32(int(maxLease / time.Second)),
		Names:           names,
		Force:           force,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return claim{}, errNothingToClaim
	}
	if err != nil {
		return claim{}, err
	}
	// The deadline counts from before the claim was sent, so it ends no later than lease_until whatever the clock skew between this server and Postgres.
	deadline := requested.Add(time.Duration(row.LeaseSeconds) * time.Second)
	return claim{name: row.Name, startedAt: row.StartedAt, deadline: deadline}, nil
}

func (s *store) finish(ctx context.Context, c claim, outcome, summary string, event *audit.Event) (bool, error) {
	held, err := s.finishWith(ctx, c, outcome, summary, event)
	if !errors.Is(err, errEventNotRecorded) {
		return held, err
	}
	audit.Dropped(audit.DroppedRecordFailed)
	held, retryErr := s.finishWith(ctx, c, outcome, summary, nil)
	if retryErr != nil {
		return false, retryErr
	}
	return held, err
}

func (s *store) finishWith(ctx context.Context, c claim, outcome, summary string, event *audit.Event) (bool, error) {
	var held bool
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		n, err := q.FinishJob(ctx, dbsqlc.FinishJobParams{
			Outcome:    outcome,
			LastError:  summary,
			Name:       c.name,
			InstanceID: s.instanceID,
			StartedAt:  c.startedAt,
		})
		if err != nil {
			return err
		}
		held = n == 1
		if !held || event == nil {
			return nil
		}
		if err := audit.Record(ctx, q, *event); err != nil {
			return fmt.Errorf("%w: %w", errEventNotRecorded, err)
		}
		return nil
	})
	return held && err == nil, err
}

func (s *store) release(ctx context.Context, c claim) (bool, error) {
	n, err := s.db.Queries().ReleaseJob(ctx, dbsqlc.ReleaseJobParams{Name: c.name, InstanceID: s.instanceID, StartedAt: c.startedAt})
	return n == 1, err
}
