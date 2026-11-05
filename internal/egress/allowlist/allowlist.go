package allowlist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
)

var (
	ErrLocked  = errors.New("outbound domains are set by configuration")
	ErrTooMany = errors.New("too many outbound domains")
	ErrExists  = errors.New("outbound domain exists")
)

type Lock struct {
	Locked  bool
	Domains domains.List
}

type Store struct {
	db     *db.DB
	lock   Lock
	logger *slog.Logger
}

func New(database *db.DB, lock Lock, logger *slog.Logger) *Store {
	if lock.Locked {
		logger.Info("outbound domains managed by configuration", slog.Int("count", len(lock.Domains)))
	}
	return &Store{db: database, lock: lock, logger: logger}
}

func (s *Store) Allowed(ctx context.Context) (domains.List, error) {
	if s.lock.Locked {
		return s.lock.Domains, nil
	}
	rows, err := s.db.Queries().ListOutboundDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("list outbound domains: %w", err)
	}
	list := make(domains.List, 0, len(rows))
	for _, r := range rows {
		p, err := domains.ParsePattern(r.Domain)
		if err != nil {
			return nil, fmt.Errorf("stored outbound domain is invalid: %w", err)
		}
		list = append(list, p)
	}
	return list, nil
}

func (s *Store) Add(ctx context.Context, by auth.Principal, pattern domains.Pattern) (dbsqlc.InsertOutboundDomainRow, error) {
	if s.lock.Locked {
		return dbsqlc.InsertOutboundDomainRow{}, ErrLocked
	}
	params := dbsqlc.InsertOutboundDomainParams{Domain: pattern.String(), CreatedByName: by.Name}
	if params.CreatedByName == "" {
		params.CreatedByName = by.Email
	}
	if id, err := db.ParseUUID(by.ID); err == nil && by.Kind == auth.KindUser {
		params.CreatedByID = id
	}
	var row dbsqlc.InsertOutboundDomainRow
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockOutboundDomains(ctx); err != nil {
			return err
		}
		n, err := q.CountOutboundDomains(ctx)
		if err != nil {
			return err
		}
		if n >= config.MaxOutboundDomains {
			return ErrTooMany
		}
		row, err = q.InsertOutboundDomain(ctx, params)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrExists
		}
		return err
	})
	return row, err
}
