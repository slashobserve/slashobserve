package draftbudget

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	Window       = time.Hour
	MaxPerWindow = 60
)

type Store struct{ db *db.DB }

func New(database *db.DB) *Store { return &Store{db: database} }

func (s *Store) Spend(ctx context.Context, principalKey string) (allowed bool, resetsIn time.Duration, err error) {
	now := time.Now().UTC()
	start := now.Truncate(Window)
	_, err = s.db.Queries().SpendDraftQuery(ctx, dbsqlc.SpendDraftQueryParams{
		PrincipalKey: principalKey,
		WindowStart:  pgtype.Timestamptz{Time: start, Valid: true},
		MaxCalls:     MaxPerWindow,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, start.Add(Window).Sub(now), nil
	}
	return err == nil, 0, err
}
