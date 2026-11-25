package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/origin"
)

type ReleaseUpdate struct {
	DashboardID      pgtype.UUID
	Parent           int
	Spec             json.RawMessage
	Summary          string
	Home             pgtype.UUID
	EditedCopySuffix string
	InTx             func(q *dbsqlc.Queries, version int) error
}

func (s *Service) UpdateProvisioned(ctx context.Context, by auth.Principal, in ReleaseUpdate) (SaveResult, error) {
	before, err := s.beforeRelease(ctx, by, in)
	if err != nil {
		return SaveResult{}, err
	}
	return s.persist(withAcceptedEmpty(ctx), by, SaveInput{
		DashboardID: in.DashboardID,
		Parent:      in.Parent,
		Spec:        in.Spec,
		Summary:     in.Summary,
		InTx:        in.InTx,
		before:      before,
		keepLayout:  true,
	})
}

func (s *Service) beforeRelease(ctx context.Context, by auth.Principal, in ReleaseUpdate) (func(context.Context, *dbsqlc.Queries) (bool, error), error) {
	_, source, err := s.store.latest(ctx, in.DashboardID)
	if err != nil {
		return nil, err
	}
	var kept *newVersion
	if in.EditedCopySuffix != "" {
		summary, parsed, canonical, err := duplicateDraft(source, source.Spec.Title+in.EditedCopySuffix)
		if err != nil {
			return nil, err
		}
		kept = &newVersion{spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), origin: origin.From(ctx), unverified: source.Unverified}
	}
	moved := actor.In(ctx, by, "")
	return func(ctx context.Context, q *dbsqlc.Queries) (bool, error) {
		current, err := q.DashboardFolder(ctx, in.DashboardID)
		if err != nil {
			return false, err
		}
		if kept != nil {
			if err := s.keepEditedCopy(ctx, q, *kept, current, in.Home); err != nil {
				return false, err
			}
		}
		if current == in.Home {
			return false, nil
		}
		err = s.store.moveIn(ctx, q, in.DashboardID, current, in.Home, moved)
		if folders.LinkedRefusal(err) {
			s.logger.WarnContext(ctx, "starter dashboard shared by public link stays in its folder",
				slog.String("dashboard_id", in.DashboardID.String()), slog.String("folder_id", current.String()))
			return true, nil
		}
		return false, err
	}, nil
}

func (s *Service) keepEditedCopy(ctx context.Context, q *dbsqlc.Queries, kept newVersion, where, home pgtype.UUID) error {
	kept.folder = folders.Folder{ID: where}
	_, _, err := s.store.createIn(ctx, q, kept)
	var restricted *folders.RestrictedContentError
	if errors.As(err, &restricted) && where != home {
		kept.folder = folders.Folder{ID: home}
		_, _, err = s.store.createIn(ctx, q, kept)
	}
	return err
}
