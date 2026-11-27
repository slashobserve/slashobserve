package inboundmcp

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type Discovery struct {
	Source     Source
	Tally      Tally
	Superseded bool
}

func (s *Service) fetch(ctx context.Context, row dbsqlc.McpSource) ([]Found, string, error) {
	secret, failure := s.open(row)
	if failure != "" {
		return nil, failure, nil
	}
	found, err := s.connector.Tools(ctx, connectTarget(row, secret))
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, failureCode(err), nil
	}
	return found, "", nil
}

func (s *Service) Discover(ctx context.Context, by auth.Principal, uid string) (Discovery, error) {
	row, err := s.row(ctx, uid)
	if err != nil {
		return Discovery{}, err
	}
	found, failure, err := s.fetch(ctx, row)
	if err != nil {
		return Discovery{}, err
	}
	var tally Tally
	var source Source
	superseded := false
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		locked, err := q.LockMCPSource(ctx, uid)
		if err != nil {
			return err
		}
		if !locked.UpdatedAt.Time.Equal(row.UpdatedAt.Time) {
			return ErrSourceChanged
		}
		superseded = locked.Discoveries != row.Discoveries
		if !superseded {
			if err := s.recordFetch(ctx, q, locked, found, failure, &tally); err != nil {
				return err
			}
		}
		checked, err := q.GetMCPSource(ctx, uid)
		if err != nil {
			return err
		}
		tools, err := q.ListMCPSourceTools(ctx, checked.ID)
		if err != nil {
			return err
		}
		source = sourceOf(checked, countsOf(tools))
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Discovery{}, ErrNotFound
	}
	if err != nil {
		return Discovery{}, err
	}
	if superseded {
		return Discovery{Source: source, Superseded: true}, nil
	}
	if failure != "" {
		s.logger.InfoContext(ctx, "MCP source discovery failed", slog.String("actor_id", by.ID), slog.String("uid", uid), slog.String("failure", failure))
	}
	return Discovery{Source: source, Tally: tally}, nil
}

func (s *Service) recordFetch(ctx context.Context, q *dbsqlc.Queries, locked dbsqlc.McpSource, found []Found, failure string, tally *Tally) error {
	if failure == "" {
		var err error
		if *tally, err = reconcileStored(ctx, q, locked, found); err != nil {
			return err
		}
	}
	if err := q.MarkMCPSourceChecked(ctx, dbsqlc.MarkMCPSourceCheckedParams{ID: locked.ID, Failure: text(failure)}); err != nil {
		return err
	}
	return recordDiscovered(ctx, q, locked, *tally, failure)
}

func reconcileStored(ctx context.Context, q *dbsqlc.Queries, source dbsqlc.McpSource, found []Found) (Tally, error) {
	rows, err := q.ListMCPSourceTools(ctx, source.ID)
	if err != nil {
		return Tally{}, err
	}
	stored := make([]Tool, len(rows))
	for i, r := range rows {
		stored[i] = toolOf(r)
	}
	changes, tally := reconcile(stored, found)
	for _, c := range changes {
		if c.Remove {
			err = q.DeleteMCPSourceTool(ctx, dbsqlc.DeleteMCPSourceToolParams{SourceID: source.ID, Name: c.Tool.Name})
		} else {
			err = q.PutMCPSourceTool(ctx, putParams(source.ID, c.Tool))
		}
		if err != nil {
			return Tally{}, err
		}
	}
	return tally, nil
}
