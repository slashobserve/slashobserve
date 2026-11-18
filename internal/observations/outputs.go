package observations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const madeDashboardSummary = "Made from an observation"

var (
	ErrNoDashboard   = errors.New("the observation names no dashboard to pin to")
	ErrNoPanels      = errors.New("the observation has no live panel blocks to make a dashboard from")
	ErrRunInProgress = errors.New("an investigation is still running on this observation; delete it once the run ends or is stopped")
)

type Dashboards interface {
	Create(ctx context.Context, by auth.Principal, folderID pgtype.UUID, rawSpec []byte, summary string) (dashboards.Dashboard, dashboards.Version, error)
	Get(ctx context.Context, id pgtype.UUID) (dashboards.Dashboard, dashboards.Version, error)
	Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error
}

type Annotations interface {
	Create(ctx context.Context, by auth.Principal, in annotations.CreateInput) (annotations.Annotation, error)
	Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error
}

type Option func(*Service)

func WithOutputs(d Dashboards, a Annotations) Option {
	return func(s *Service) { s.dashboards, s.annotations = d, a }
}

type Pin struct {
	DashboardID  string
	AnnotationID string
}

func (s *Service) runDashboard(ctx context.Context, id pgtype.UUID) (pgtype.UUID, error) {
	dashboardID, err := s.store.db.Queries().GetObservationRunDashboard(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return pgtype.UUID{}, nil
	case err != nil:
		return pgtype.UUID{}, fmt.Errorf("get the dashboard of an observation's run: %w", err)
	}
	return dashboardID, nil
}

func (s *Service) pin(ctx context.Context, id, dashboardID pgtype.UUID) (Pin, bool, error) {
	row, err := s.store.db.Queries().GetObservationPin(ctx, dbsqlc.GetObservationPinParams{ObservationID: id, DashboardID: dashboardID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Pin{}, false, nil
	case err != nil:
		return Pin{}, false, fmt.Errorf("get observation pin: %w", err)
	}
	return Pin{DashboardID: dashboardID.String(), AnnotationID: row.AnnotationID.String()}, true, nil
}

func (s *Service) Pin(ctx context.Context, by auth.Principal, id, dashboardID pgtype.UUID) (Pin, bool, error) {
	row, _, _, err := s.open(ctx, by, id)
	if err != nil {
		return Pin{}, false, err
	}
	if !dashboardID.Valid {
		if dashboardID, err = s.runDashboard(ctx, id); err != nil {
			return Pin{}, false, err
		}
	}
	if !dashboardID.Valid {
		return Pin{}, false, ErrNoDashboard
	}
	if existing, ok, err := s.pin(ctx, id, dashboardID); err != nil || ok {
		return existing, false, err
	}
	at := time.Now().UTC()
	if row.TimeTo.Valid {
		at = row.TimeTo.Time
	}
	a, err := s.annotations.Create(ctx, by, annotations.CreateInput{
		DashboardID: dashboardID, From: at, Text: row.Title + " " + links.Observation(nil, id.String()), Tags: []string{findings.PinTag},
	})
	switch {
	case errors.Is(err, annotations.ErrNotFound):
		return Pin{}, false, ErrNotFound
	case errors.Is(err, annotations.ErrForbidden):
		return Pin{}, false, folders.ErrForbidden
	case err != nil:
		return Pin{}, false, err
	}
	annotationID, err := db.ParseUUID(a.ID)
	if err != nil {
		return Pin{}, false, err
	}
	won, err := s.store.db.Queries().InsertObservationPin(ctx, dbsqlc.InsertObservationPinParams{ObservationID: id, DashboardID: dashboardID, AnnotationID: annotationID})
	if err != nil {
		return Pin{}, false, fmt.Errorf("record observation pin: %w", err)
	}
	if won == 1 {
		return Pin{DashboardID: dashboardID.String(), AnnotationID: a.ID}, true, nil
	}
	if err := s.annotations.Delete(ctx, by, annotationID); err != nil {
		s.logger.WarnContext(ctx, "delete the annotation of a pin that lost the race", logattrs.Failure("observation_pin_cleanup_failed", err, slog.String("observation_id", id.String()), slog.String("annotation_id", a.ID))...)
	}
	existing, _, err := s.pin(ctx, id, dashboardID)
	return existing, false, err
}

type outputKey struct {
	Observation, Block pgtype.UUID
	Kind               string
	Principal          pgtype.UUID
}

func (s *Service) outputKeyFor(by auth.Principal, observationID, blockID pgtype.UUID) (outputKey, error) {
	who := actor.Of(by, origin.Origin{}, "")
	principal, err := db.ParseUUID(who.ID)
	if err != nil {
		return outputKey{}, folders.ErrForbidden
	}
	return outputKey{Observation: observationID, Block: blockID, Kind: string(who.Kind), Principal: principal}, nil
}

func (s *Service) madeDashboard(ctx context.Context, by auth.Principal, key outputKey) (dashboards.Dashboard, dashboards.Version, bool, error) {
	q := s.store.db.Queries()
	dashboardID, err := q.GetObservationDashboard(ctx, dbsqlc.GetObservationDashboardParams{ObservationID: key.Observation, BlockID: key.Block, CreatedByKind: key.Kind, CreatedByID: key.Principal})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dashboards.Dashboard{}, dashboards.Version{}, false, nil
	case err != nil:
		return dashboards.Dashboard{}, dashboards.Version{}, false, fmt.Errorf("get made dashboard: %w", err)
	}
	access, err := s.access.Dashboard(ctx, by, dashboardID)
	if err != nil || !access.Abilities.View {
		return dashboards.Dashboard{}, dashboards.Version{}, false, nil
	}
	d, v, err := s.dashboards.Get(ctx, dashboardID)
	if err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, false, err
	}
	return d, v, true, nil
}

func (s *Service) panelsSpec(parent dbsqlc.Observation, blocks []dbsqlc.ObservationBlock) (*spec.Dashboard, error) {
	panels := make([]json.RawMessage, 0, len(blocks))
	var variables []spec.Variable
	seen := map[string]bool{}
	var from, to time.Time
	now := time.Now().UTC()
	for _, b := range blocks {
		var body PanelBody
		if err := json.Unmarshal(b.Body, &body); err != nil {
			return nil, fmt.Errorf("decode panel block: %w", err)
		}
		panels = append(panels, body.Panel)
		for _, v := range body.Variables {
			if !seen[v.Name] {
				seen[v.Name] = true
				variables = append(variables, v)
			}
		}
		if from.IsZero() {
			from, to = blockRange(body, timePtr(parent.TimeFrom), timePtr(parent.TimeTo), now)
		}
	}
	if parent.TimeFrom.Valid && parent.TimeTo.Valid {
		from, to = parent.TimeFrom.Time, parent.TimeTo.Time
	}
	return findings.EvidenceSpecWith(parent.Title, from.Truncate(storedPrecision), to.Truncate(storedPrecision), panels, variables)
}

func (s *Service) MakeDashboard(ctx context.Context, by auth.Principal, id, blockID pgtype.UUID, again bool) (dashboards.Dashboard, dashboards.Version, bool, error) {
	none := func(err error) (dashboards.Dashboard, dashboards.Version, bool, error) {
		return dashboards.Dashboard{}, dashboards.Version{}, false, err
	}
	if blockID.Valid {
		block, err := s.store.block(ctx, blockID)
		if err != nil {
			return none(err)
		}
		if id.Valid && id != block.ObservationID {
			return none(ErrNotFound)
		}
		id = block.ObservationID
	}
	parent, _, abilities, err := s.open(ctx, by, id)
	if err != nil {
		return none(err)
	}
	rows, err := s.store.blocks(ctx, id)
	if err != nil {
		return none(err)
	}
	viewer := actor.Of(by, origin.Origin{}, "")
	var panels []dbsqlc.ObservationBlock
	for _, b := range rows {
		if Kind(b.Kind) == KindPanel && BlockState(b.State) == BlockLive && visibleTo(b, abilities, viewer) && (!blockID.Valid || b.ID == blockID) {
			panels = append(panels, b)
		}
	}
	switch {
	case blockID.Valid && len(panels) == 0:
		return none(ErrNotFound)
	case len(panels) == 0:
		return none(ErrNoPanels)
	}
	key, err := s.outputKeyFor(by, id, blockID)
	if err != nil {
		return none(err)
	}
	if !again {
		if d, v, ok, err := s.madeDashboard(ctx, by, key); err != nil || ok {
			return d, v, false, err
		}
	}
	doc, err := s.panelsSpec(parent, panels)
	if err != nil {
		return none(fmt.Errorf("build the dashboard of an observation: %w", err))
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return none(fmt.Errorf("encode the dashboard of an observation: %w", err))
	}
	d, v, err := s.dashboards.Create(ctx, by, pgtype.UUID{}, raw, madeDashboardSummary)
	if err != nil {
		return none(err)
	}
	dashboardID, err := db.ParseUUID(d.ID)
	if err != nil {
		return none(err)
	}
	q := s.store.db.Queries()
	if again {
		if err := q.DeleteObservationDashboard(ctx, dbsqlc.DeleteObservationDashboardParams{ObservationID: key.Observation, BlockID: key.Block, CreatedByKind: key.Kind, CreatedByID: key.Principal}); err != nil {
			return none(fmt.Errorf("replace made dashboard: %w", err))
		}
	}
	won, err := q.InsertObservationDashboard(ctx, dbsqlc.InsertObservationDashboardParams{ObservationID: key.Observation, BlockID: key.Block, DashboardID: dashboardID, CreatedByKind: key.Kind, CreatedByID: key.Principal})
	if err != nil {
		return none(fmt.Errorf("record made dashboard: %w", err))
	}
	if won == 1 {
		return d, v, true, nil
	}
	if err := s.dashboards.Delete(ctx, by, dashboardID); err != nil {
		s.logger.WarnContext(ctx, "delete the dashboard that lost the race", logattrs.Failure("observation_dashboard_cleanup_failed", err, slog.String("observation_id", id.String()), slog.String("dashboard_id", d.ID))...)
	}
	winner, wv, ok, err := s.madeDashboard(ctx, by, key)
	if err != nil || !ok {
		return none(err)
	}
	return winner, wv, false, nil
}
