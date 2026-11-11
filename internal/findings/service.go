package findings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const savedDashboardSummary = "Saved from a finding"

type Service struct {
	store       *store
	access      *folders.Access
	annotations *annotations.Service
	dashboards  *dashboards.Service
	logger      *slog.Logger
}

func New(database *db.DB, access *folders.Access, ann *annotations.Service, dash *dashboards.Service, logger *slog.Logger) *Service {
	return &Service{store: &store{db: database}, access: access, annotations: ann, dashboards: dash, logger: logger}
}

type SaveInput struct {
	AgentRequestID  pgtype.UUID
	DashboardID     pgtype.UUID
	FolderID        pgtype.UUID
	Title, Markdown string
	Evidence        []spec.Panel
	From, To        time.Time
}

func (s *Service) Save(ctx context.Context, by auth.Principal, in SaveInput) (Finding, bool, error) {
	if in.DashboardID.Valid && in.FolderID.Valid {
		return Finding{}, false, ErrPlacement
	}
	if in.DashboardID.Valid {
		if err := s.viewable(ctx, by, in.DashboardID); err != nil {
			return Finding{}, false, err
		}
	} else {
		folder, err := s.creationFolder(ctx, by, in.FolderID)
		if err != nil {
			return Finding{}, false, err
		}
		in.FolderID = folder
	}
	if in.Evidence == nil {
		in.Evidence = []spec.Panel{}
	}
	return s.store.insert(ctx, actor.Of(by, origin.From(ctx), in.AgentRequestID.String()), in)
}

func (s *Service) creationFolder(ctx context.Context, by auth.Principal, requested pgtype.UUID) (pgtype.UUID, error) {
	folder, err := s.access.CreationFolder(ctx, by, requested)
	switch {
	case errors.Is(err, folders.ErrNotFound):
		return pgtype.UUID{}, ErrFolderNotFound
	case errors.Is(err, folders.ErrForbidden) && !requested.Valid:
		return pgtype.UUID{}, nil
	case err != nil:
		return pgtype.UUID{}, err
	}
	return folder.ID, nil
}

func (s *Service) Get(ctx context.Context, by auth.Principal, id pgtype.UUID) (Finding, error) {
	f, err := s.store.get(ctx, id)
	if err != nil {
		return Finding{}, err
	}
	where, err := s.locate(ctx, by, f)
	if err != nil {
		return Finding{}, err
	}
	f.Folder, f.Abilities = where.folder, where.abilities
	return f, nil
}

func (s *Service) ForRequest(ctx context.Context, requestID pgtype.UUID) (Finding, bool, error) {
	return s.store.forRequest(ctx, requestID)
}

func (s *Service) EvidenceDashboard(ctx context.Context, by auth.Principal, id pgtype.UUID) (*spec.Dashboard, error) {
	f, err := s.Get(ctx, by, id)
	if err != nil {
		return nil, err
	}
	return f.Spec(), nil
}

type ListFilter struct {
	DashboardID pgtype.UUID
	FolderID    pgtype.UUID
}

func (s *Service) List(ctx context.Context, by auth.Principal, filter ListFilter, limit int, cursor string) ([]Finding, string, error) {
	if filter.DashboardID.Valid && filter.FolderID.Valid {
		return nil, "", ErrPlacement
	}
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	var rows []Finding
	var err error
	switch {
	case filter.DashboardID.Valid:
		if err := s.viewable(ctx, by, filter.DashboardID); err != nil {
			return nil, "", err
		}
		rows, err = s.store.listByDashboard(ctx, filter.DashboardID, after, limit+1)
	case filter.FolderID.Valid:
		if _, _, err := s.access.Folder(ctx, by, filter.FolderID); err != nil {
			if errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrForbidden) {
				return nil, "", ErrNotFound
			}
			return nil, "", err
		}
		rows, err = s.store.listInFolder(ctx, filter.FolderID, after, limit+1)
	default:
		var levels folders.Levels
		levels, err = s.access.Levels(ctx, by)
		if errors.Is(err, folders.ErrForbidden) {
			return nil, "", ErrNotFound
		}
		if err == nil {
			rows, err = s.store.listBy(ctx, by, levels.Visible(), after, limit+1)
		}
	}
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = httpapi.EncodeTimeCursor(rows[limit-1].CreatedAt, rows[limit-1].ID)
	}
	return rows, next, nil
}

func (s *Service) Pin(ctx context.Context, by auth.Principal, id pgtype.UUID) (annotations.Annotation, bool, error) {
	f, err := s.Get(ctx, by, id)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	if f.DashboardID == "" {
		return annotations.Annotation{}, false, ErrNoDashboard
	}
	if f.AnnotationID != "" {
		a, err := s.pinned(ctx, f)
		if !errors.Is(err, ErrNotFound) {
			return a, false, err
		}
	}
	dashboardID, err := db.ParseUUID(f.DashboardID)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	at := f.CreatedAt
	if f.RequestTo != nil {
		at = *f.RequestTo
	}
	a, err := s.annotations.Create(ctx, by, annotations.CreateInput{DashboardID: dashboardID, From: at, Text: PinText(f), Tags: []string{PinTag}})
	switch {
	case errors.Is(err, annotations.ErrNotFound):
		return annotations.Annotation{}, false, ErrNotFound
	case err != nil:
		return annotations.Annotation{}, false, err
	}
	annotationID, err := db.ParseUUID(a.ID)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	won, err := s.store.setAnnotation(ctx, actor.Of(by, origin.From(ctx), ""), id, dashboardID, annotationID)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	if won {
		return a, true, nil
	}
	if err := s.annotations.Delete(ctx, by, annotationID); err != nil {
		s.logger.WarnContext(ctx, "delete the annotation of a pin that lost the race", logattrs.Failure("finding_pin_cleanup_failed", err, slog.String("finding_id", f.ID), slog.String("annotation_id", a.ID))...)
	}
	winner, err := s.store.get(ctx, id)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	a, err = s.pinned(ctx, winner)
	return a, false, err
}

func (s *Service) pinned(ctx context.Context, f Finding) (annotations.Annotation, error) {
	if f.AnnotationID == "" {
		return annotations.Annotation{}, ErrNotFound
	}
	id, err := db.ParseUUID(f.AnnotationID)
	if err != nil {
		return annotations.Annotation{}, err
	}
	return s.store.annotation(ctx, id)
}

func (s *Service) SaveAsDashboard(ctx context.Context, by auth.Principal, id pgtype.UUID) (dashboards.Dashboard, dashboards.Version, error) {
	f, err := s.Get(ctx, by, id)
	if err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, err
	}
	if len(f.Evidence) == 0 {
		return dashboards.Dashboard{}, dashboards.Version{}, ErrNoEvidence
	}
	raw, err := json.Marshal(f.Spec())
	if err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, fmt.Errorf("encode evidence dashboard: %w", err)
	}
	return s.dashboards.Create(ctx, by, pgtype.UUID{}, raw, savedDashboardSummary)
}

type Placement struct {
	Dashboard pgtype.UUID
	Folder    *folders.Folder
	Activity  activity.Owner
	Abilities folders.Abilities
}

func (s *Service) Access(ctx context.Context, by auth.Principal, id pgtype.UUID) (Placement, error) {
	f, err := s.store.get(ctx, id)
	if err != nil {
		return Placement{}, err
	}
	where, err := s.locate(ctx, by, f)
	if err != nil {
		return Placement{}, err
	}
	var dashboardID pgtype.UUID
	if f.DashboardID != "" {
		if dashboardID, err = db.ParseUUID(f.DashboardID); err != nil {
			return Placement{}, err
		}
	}
	return Placement{Dashboard: dashboardID, Folder: where.folder, Activity: activityOwner(dashboardID, id), Abilities: where.abilities}, nil
}

type location struct {
	folder    *folders.Folder
	abilities folders.Abilities
}

func (s *Service) locate(ctx context.Context, by auth.Principal, f Finding) (location, error) {
	switch {
	case f.DashboardID != "":
		dashboardID, err := db.ParseUUID(f.DashboardID)
		if err != nil {
			return location{}, err
		}
		access, err := s.dashboardAccess(ctx, by, dashboardID)
		if err != nil {
			return location{}, err
		}
		return location{folder: &access.Folder, abilities: access.Abilities}, nil
	case f.FolderID != "":
		folderID, err := db.ParseUUID(f.FolderID)
		if err != nil {
			return location{}, err
		}
		folder, abilities, err := s.access.Folder(ctx, by, folderID)
		switch {
		case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrForbidden):
			return location{}, ErrNotFound
		case err != nil:
			return location{}, err
		case !abilities.View:
			return location{}, ErrNotFound
		}
		return location{folder: &folder, abilities: abilities}, nil
	}
	creator := string(by.Kind) == f.CreatedBy.Kind && by.ID == f.CreatedBy.ID
	abilities := folders.Unfiled(by, creator)
	if !abilities.View {
		return location{}, ErrNotFound
	}
	return location{abilities: abilities}, nil
}

func (s *Service) viewable(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	_, err := s.dashboardAccess(ctx, by, dashboardID)
	return err
}

func (s *Service) dashboardAccess(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) (folders.DashboardAccess, error) {
	access, err := s.access.Dashboard(ctx, by, dashboardID)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden):
		return folders.DashboardAccess{}, ErrNotFound
	case err != nil:
		return folders.DashboardAccess{}, err
	case !access.Abilities.View:
		return folders.DashboardAccess{}, ErrNotFound
	}
	return access, nil
}
