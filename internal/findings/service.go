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
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const savedDashboardSummary = "Saved from a finding"

var ErrNoObservations = errors.New("findings need an observation service")

type Service struct {
	store        *store
	access       *folders.Access
	annotations  *annotations.Service
	dashboards   *dashboards.Service
	observations Observations
	logger       *slog.Logger
}

func New(database *db.DB, access *folders.Access, ann *annotations.Service, dash *dashboards.Service, observations Observations, logger *slog.Logger) (*Service, error) {
	if observations == nil {
		return nil, ErrNoObservations
	}
	return &Service{store: &store{db: database}, access: access, annotations: ann, dashboards: dash, observations: observations, logger: logger}, nil
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
	}
	if in.Evidence == nil {
		in.Evidence = []spec.Panel{}
	}
	conclusion := Conclusion{
		RunID: in.AgentRequestID, DashboardID: in.DashboardID, FolderID: in.FolderID,
		Title: in.Title, Markdown: in.Markdown, Evidence: in.Evidence, From: in.From, To: in.To,
	}
	return s.store.insert(ctx, actor.Of(by, origin.From(ctx), in.AgentRequestID.String()), in, func(q *dbsqlc.Queries) (pgtype.UUID, bool, error) {
		return s.observations.WriteConclusion(ctx, q, by, conclusion)
	})
}

func (s *Service) Get(ctx context.Context, by auth.Principal, id pgtype.UUID) (Finding, error) {
	f, err := s.store.get(ctx, id)
	if err != nil {
		return Finding{}, err
	}
	folder, abilities, err := s.place(ctx, by, f)
	if err != nil {
		return Finding{}, err
	}
	f.Folder, f.Abilities = &folder, abilities
	return f, nil
}

func (s *Service) ForRequest(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (Finding, bool, error) {
	f, found, err := s.store.forRequest(ctx, requestID)
	if err != nil || !found {
		return Finding{}, false, err
	}
	_, _, err = s.place(ctx, by, f)
	switch {
	case errors.Is(err, ErrNotFound):
		return Finding{}, false, nil
	case err != nil:
		return Finding{}, false, err
	}
	return f, true, nil
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
	visible, err := s.visibleFolders(ctx, by)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.listRows(ctx, by, filter, visible, after, limit+1)
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

func (s *Service) listRows(ctx context.Context, by auth.Principal, filter ListFilter, visible []pgtype.UUID, after *httpapi.TimeCursor, limit int) ([]Finding, error) {
	switch {
	case filter.DashboardID.Valid:
		if err := s.viewable(ctx, by, filter.DashboardID); err != nil {
			return nil, err
		}
		return s.store.listByDashboard(ctx, filter.DashboardID, visible, after, limit)
	case filter.FolderID.Valid:
		if _, _, err := s.access.Folder(ctx, by, filter.FolderID); err != nil {
			if errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrForbidden) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		return s.store.listInFolder(ctx, filter.FolderID, visible, after, limit)
	}
	return s.store.listBy(ctx, by, visible, after, limit)
}

func (s *Service) visibleFolders(ctx context.Context, by auth.Principal) ([]pgtype.UUID, error) {
	levels, err := s.access.Levels(ctx, by)
	if errors.Is(err, folders.ErrForbidden) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return levels.Visible(), nil
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
	observationID, err := db.ParseUUID(f.ObservationID)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	won, err := s.store.setAnnotation(ctx, actor.Of(by, origin.From(ctx), ""), id, dashboardID, observationID, annotationID)
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
	folder, abilities, err := s.place(ctx, by, f)
	if err != nil {
		return Placement{}, err
	}
	observationID, err := db.ParseUUID(f.ObservationID)
	if err != nil {
		return Placement{}, err
	}
	var dashboardID pgtype.UUID
	if f.DashboardID != "" {
		if dashboardID, err = db.ParseUUID(f.DashboardID); err != nil {
			return Placement{}, err
		}
	}
	return Placement{Dashboard: dashboardID, Folder: &folder, Activity: activity.OnObservation(observationID), Abilities: abilities}, nil
}

func (s *Service) place(ctx context.Context, by auth.Principal, f Finding) (folders.Folder, folders.Abilities, error) {
	observationID, err := db.ParseUUID(f.ObservationID)
	if err != nil {
		return folders.Folder{}, folders.Abilities{}, err
	}
	return s.observations.Placement(ctx, by, observationID)
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
