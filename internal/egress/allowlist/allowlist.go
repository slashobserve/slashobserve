package allowlist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

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
	return s.insert(ctx, by, pattern, false)
}

func (s *Store) AddForConnection(ctx context.Context, by auth.Principal, pattern domains.Pattern) (dbsqlc.InsertOutboundDomainRow, error) {
	return s.insert(ctx, by, pattern, true)
}

func (s *Store) insert(ctx context.Context, by auth.Principal, pattern domains.Pattern, forConnection bool) (dbsqlc.InsertOutboundDomainRow, error) {
	if s.lock.Locked {
		return dbsqlc.InsertOutboundDomainRow{}, ErrLocked
	}
	params := dbsqlc.InsertOutboundDomainParams{Domain: pattern.String(), CreatedByName: by.Name, ForConnection: forConnection}
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

func (s *Store) Claim(ctx context.Context, pattern domains.Pattern) (dbsqlc.ClaimConnectionOutboundDomainRow, bool, error) {
	row, err := s.db.Queries().ClaimConnectionOutboundDomain(ctx, pattern.String())
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.ClaimConnectionOutboundDomainRow{}, false, nil
	}
	return row, err == nil, err
}

type User struct {
	Kind string
	Name string
}

func (s *Store) Users(ctx context.Context, pattern domains.Pattern) ([]User, error) {
	return usersIn(ctx, s.db.Queries(), pattern)
}

func usersIn(ctx context.Context, q *dbsqlc.Queries, pattern domains.Pattern) ([]User, error) {
	rows, err := q.ListOutboundUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list outbound users: %w", err)
	}
	return usersOf(rows, pattern), nil
}

func usersOf(rows []dbsqlc.ListOutboundUsersRow, pattern domains.Pattern) []User {
	users := []User{}
	for _, r := range rows {
		u, err := url.Parse(r.Url)
		if err != nil || !pattern.Matches(strings.ToLower(u.Hostname())) {
			continue
		}
		users = append(users, User{Kind: r.Kind, Name: r.Name})
	}
	return users
}

func (s *Store) Release(ctx context.Context, host string) error {
	if s.lock.Locked {
		return nil
	}
	pattern, err := domains.ParsePattern(host)
	if err != nil {
		return nil
	}
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockOutboundDomains(ctx); err != nil {
			return err
		}
		users, err := usersIn(ctx, q, pattern)
		if err != nil || len(users) > 0 {
			return err
		}
		_, err = q.DeleteConnectionOutboundDomain(ctx, pattern.String())
		return err
	})
	if err != nil {
		return fmt.Errorf("release outbound domain: %w", err)
	}
	return nil
}
