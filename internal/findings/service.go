package findings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

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
	Title, Markdown string
	Evidence        []spec.Panel
	From, To        time.Time
}

func (s *Service) Save(ctx context.Context, by auth.Principal, in SaveInput) (Finding, bool, error) {
	if in.DashboardID.Valid {
		if err := s.viewable(ctx, by, in.DashboardID); err != nil {
			return Finding{}, false, err
		}
	}
	if in.Evidence == nil {
		in.Evidence = []spec.Panel{}
	}
	return s.store.insert(ctx, actor.Of(by, origin.From(ctx), in.AgentRequestID.String()), in)
}

func (s *Service) Get(ctx context.Context, by auth.Principal, id pgtype.UUID) (Finding, error) {
	f, err := s.store.get(ctx, id)
	if err != nil {
		return Finding{}, err
	}
	if err := s.visible(ctx, by, f); err != nil {
		return Finding{}, err
	}
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

func (s *Service) List(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID, limit int, cursor string) ([]Finding, string, error) {
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	var rows []Finding
	var err error
	if dashboardID.Valid {
		if err := s.viewable(ctx, by, dashboardID); err != nil {
			return nil, "", err
		}
		rows, err = s.store.listByDashboard(ctx, dashboardID, after, limit+1)
	} else {
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
	won, err := s.store.setAnnotation(ctx, id, annotationID)
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

func (s *Service) visible(ctx context.Context, by auth.Principal, f Finding) error {
	if f.DashboardID == "" {
		if (string(by.Kind) == f.CreatedBy.Kind && by.ID == f.CreatedBy.ID) || by.Role == auth.RoleAdmin {
			return nil
		}
		return ErrNotFound
	}
	dashboardID, err := db.ParseUUID(f.DashboardID)
	if err != nil {
		return err
	}
	return s.viewable(ctx, by, dashboardID)
}

func (s *Service) viewable(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	access, err := s.access.Dashboard(ctx, by, dashboardID)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden):
		return ErrNotFound
	case err != nil:
		return err
	case !access.Abilities.View:
		return ErrNotFound
	}
	return nil
}
