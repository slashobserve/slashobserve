package contextsources

import (
	"context"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func (s *Service) Provision(ctx context.Context, entries []config.ContextSource) error {
	return s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockContextSourceCount(ctx); err != nil {
			return err
		}
		rows, err := q.ListContextSources(ctx)
		if err != nil {
			return err
		}
		wanted := make(map[string]bool, len(entries))
		for _, e := range entries {
			wanted[e.UID] = true
		}
		stored := make(map[string]dbsqlc.ContextSource, len(rows))
		count := len(rows)
		for _, r := range rows {
			if r.Provisioned && !wanted[r.Uid] {
				if _, err := q.DeleteContextSource(ctx, r.Uid); err != nil {
					return err
				}
				if err := recordDeleted(ctx, q, r); err != nil {
					return err
				}
				count--
				continue
			}
			stored[r.Uid] = r
		}
		for _, e := range entries {
			created, err := s.provisionOne(ctx, q, e, stored[e.UID], count)
			if err != nil {
				return fmt.Errorf("context source %q: %w", e.UID, err)
			}
			if created {
				count++
			}
		}
		return nil
	})
}

func (s *Service) provisionOne(ctx context.Context, q *dbsqlc.Queries, e config.ContextSource, row dbsqlc.ContextSource, count int) (bool, error) {
	in := Input{UID: e.UID, Name: e.Name, Description: e.Description, URL: e.URL, Audience: e.Audience, SecretHeader: e.SecretHeader, Secret: e.Secret.Reveal(), AllowPrivate: e.AllowPrivate}.normalized()
	host, err := in.check()
	if err != nil {
		return false, err
	}
	switch {
	case row.ID.Valid && !row.Provisioned:
		return false, ErrUIDTaken
	case !row.ID.Valid:
		return true, s.provisionNew(ctx, q, in, host, count)
	}
	return false, s.provisionExisting(ctx, q, in, host, row)
}

func (s *Service) provisionNew(ctx context.Context, q *dbsqlc.Queries, in Input, host string, count int) error {
	if count >= MaxSources {
		return ErrTooMany
	}
	id := newID()
	params := dbsqlc.InsertContextSourceParams{ID: id, Uid: in.UID, Name: in.Name, Description: text(in.Description), Url: in.URL, Audience: string(in.Audience), SecretHeader: text(in.SecretHeader), Provisioned: true, AllowPrivate: in.AllowPrivate}
	if in.Secret != "" {
		var err error
		if params.Secret, err = s.seal(id, in.Secret); err != nil {
			return err
		}
	}
	row, err := q.InsertContextSource(ctx, params)
	if err != nil {
		return err
	}
	return recordCreated(ctx, q, row, host)
}

func (s *Service) provisionExisting(ctx context.Context, q *dbsqlc.Queries, in Input, host string, row dbsqlc.ContextSource) error {
	current, _ := s.open(row)
	credentials := current.Reveal() != in.Secret || row.SecretHeader.String != in.SecretHeader
	next := revision{host: host, credentials: credentials, params: dbsqlc.UpdateContextSourceParams{
		Uid: row.Uid, Name: in.Name, Description: text(in.Description), Url: in.URL, Audience: string(in.Audience), SecretHeader: text(in.SecretHeader), Secret: row.Secret, Provisioned: true, AllowPrivate: in.AllowPrivate,
	}}
	if credentials {
		next.params.Secret = nil
		if in.Secret != "" {
			var err error
			if next.params.Secret, err = s.seal(row.ID, in.Secret); err != nil {
				return err
			}
		}
	}
	if !credentials && sameEntry(in, row) {
		return nil
	}
	next.params.ResetCheck = in.URL != row.Url || credentials || in.AllowPrivate != row.AllowPrivate
	updated, err := q.UpdateContextSource(ctx, next.params)
	if err != nil {
		return err
	}
	if in.URL != row.Url {
		if err := q.DeleteContextSourceTools(ctx, row.ID); err != nil {
			return err
		}
	}
	return recordUpdated(ctx, q, row, updated, next, hostOfURL(row.Url))
}

func sameEntry(in Input, row dbsqlc.ContextSource) bool {
	return in.Name == row.Name && in.Description == row.Description.String && in.URL == row.Url &&
		string(in.Audience) == row.Audience && in.AllowPrivate == row.AllowPrivate && row.Provisioned
}
