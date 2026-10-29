package allowlist

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
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
