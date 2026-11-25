package contextsources

import (
	"context"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func (s *Service) Provision(ctx context.Context, entries []config.ContextSource) error {
	return s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		rows, err := q.ListContextSources(ctx)
		if err != nil {
			return err
		}
		stored := make(map[string]dbsqlc.ContextSource, len(rows))
		for _, r := range rows {
			stored[r.Uid] = r
		}
		wanted := make(map[string]bool, len(entries))
		for _, e := range entries {
			wanted[e.UID] = true
			if err := s.provisionOne(ctx, q, e, stored[e.UID], len(rows)); err != nil {
				return fmt.Errorf("context source %q: %w", e.UID, err)
			}
		}
		for _, r := range rows {
			if wanted[r.Uid] || !r.Provisioned {
				continue
			}
			if _, err := q.DeleteContextSource(ctx, r.Uid); err != nil {
				return err
			}
			if err := recordDeleted(ctx, q, r); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) provisionOne(ctx context.Context, q *dbsqlc.Queries, e config.ContextSource, row dbsqlc.ContextSource, total int) error {
	in := Input{UID: e.UID, Name: e.Name, URL: e.URL, Audience: e.Audience, SecretHeader: e.SecretHeader, Secret: e.Secret.Reveal()}.normalized()
	host, err := in.check()
	if err != nil {
		return err
	}
	switch {
	case row.ID.Valid && !row.Provisioned:
		return ErrUIDTaken
	case !row.ID.Valid:
		return s.provisionNew(ctx, q, in, host, total)
	}
	return s.provisionExisting(ctx, q, in, host, row)
}

func (s *Service) provisionNew(ctx context.Context, q *dbsqlc.Queries, in Input, host string, total int) error {
	if total >= MaxSources {
		return ErrTooMany
	}
	id := newID()
	params := dbsqlc.InsertContextSourceParams{ID: id, Uid: in.UID, Name: in.Name, Url: in.URL, Audience: string(in.Audience), SecretHeader: text(in.SecretHeader), Provisioned: true}
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
		Uid: row.Uid, Name: in.Name, Url: in.URL, Audience: string(in.Audience), SecretHeader: text(in.SecretHeader), Secret: row.Secret, Provisioned: true,
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
	unchanged := !credentials && in.Name == row.Name && in.URL == row.Url && string(in.Audience) == row.Audience && row.Provisioned
	if unchanged {
		return nil
	}
	next.params.ResetCheck = in.URL != row.Url || credentials
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
