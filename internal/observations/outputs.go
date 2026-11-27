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
	"github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	madeDashboardSummary = "Made from an observation"
	discardTimeout       = 10 * time.Second
)

var (
	ErrNoDashboard       = errors.New("the observation names no dashboard to pin to")
	ErrDashboardNotFound = errors.New("dashboard not found")
	ErrNoPanels          = errors.New("the observation has no live panel blocks to make a dashboard from")
	ErrRunInProgress     = errors.New("an investigation is still running on this observation; delete it once the run ends or is stopped")
)

type Dashboards interface {
	CreateCopy(ctx context.Context, by auth.Principal, rawSpec []byte, summary string, from placement.Copied) (dashboards.Dashboard, dashboards.Version, error)
	Get(ctx context.Context, id pgtype.UUID) (dashboards.Dashboard, dashboards.Version, error)
	Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error
}

type Annotations interface {
	Create(ctx context.Context, by auth.Principal, in annotations.CreateInput) (annotations.Annotation, error)
	Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error
}

type Option func(*Service)

func WithPlacement(g *placement.Guard) Option {
	return func(s *Service) { s.guard = g }
}

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

func (s *Service) pinned(ctx context.Context, id, dashboardID pgtype.UUID) (bool, error) {
	_, err := s.store.db.Queries().GetObservationPin(ctx, dbsqlc.GetObservationPinParams{ObservationID: id, DashboardID: dashboardID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("get observation pin: %w", err)
	}
	return true, nil
}

func (s *Service) mayEdit(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) (bool, error) {
	access, err := s.access.Dashboard(ctx, by, dashboardID)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("read access to the observation's dashboard: %w", err)
	}
	return access.Abilities.Edit, nil
}

func (s *Service) unpin(ctx context.Context, by auth.Principal, id pgtype.UUID, annotationIDs []pgtype.UUID) {
	for _, annotationID := range annotationIDs {
		if err := s.annotations.Delete(ctx, by, annotationID); err != nil {
			s.logger.WarnContext(ctx, "delete the pin of a deleted observation", logattrs.Failure("observation_unpin_failed", err, slog.String("observation_id", id.String()), slog.String("annotation_id", annotationID.String()))...)
		}
	}
}

func pinOf(dashboardID, annotationID pgtype.UUID) Pin {
	return Pin{DashboardID: dashboardID.String(), AnnotationID: annotationID.String()}
}

func (s *Service) locked(ctx context.Context, name string, fn func(q *dbsqlc.Queries) error) error {
	return s.store.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockObservationOutput(ctx, name); err != nil {
			return fmt.Errorf("lock %s: %w", name, err)
		}
		return fn(q)
	})
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
	var pin Pin
	var created bool
	var createdAnnotation pgtype.UUID
	err = s.locked(ctx, "pin:"+id.String()+":"+dashboardID.String(), func(q *dbsqlc.Queries) error {
		existing, err := q.GetObservationPin(ctx, dbsqlc.GetObservationPinParams{ObservationID: id, DashboardID: dashboardID})
		switch {
		case err == nil:
			pin, created = pinOf(dashboardID, existing.AnnotationID), false
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("get observation pin: %w", err)
		}
		at := time.Now().UTC()
		if row.TimeTo.Valid {
			at = row.TimeTo.Time
		}
		a, err := s.annotations.Create(ctx, by, annotations.CreateInput{
			DashboardID: dashboardID, From: at, Text: row.Title + " " + links.Observation(nil, id.String()), Tags: []string{findings.PinTag},
			CopyOf: placement.Copied{Kind: folders.ContentObservation, ID: id},
		})
		switch {
		case errors.Is(err, annotations.ErrNotFound):
			return ErrDashboardNotFound
		case errors.Is(err, annotations.ErrForbidden):
			return folders.ErrForbidden
		case err != nil:
			return err
		}
		annotationID, err := db.ParseUUID(a.ID)
		if err != nil {
			return err
		}
		createdAnnotation = annotationID
		if err := q.InsertObservationPin(ctx, dbsqlc.InsertObservationPinParams{ObservationID: id, DashboardID: dashboardID, AnnotationID: annotationID}); err != nil {
			return fmt.Errorf("record observation pin: %w", err)
		}
		pin, created = pinOf(dashboardID, annotationID), true
		return nil
	})
	if err != nil {
		if createdAnnotation.Valid {
			s.discardAnnotation(ctx, by, id, createdAnnotation)
		}
		return Pin{}, false, err
	}
	return pin, created, nil
}

func (s *Service) discardAnnotation(ctx context.Context, by auth.Principal, id, annotationID pgtype.UUID) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), discardTimeout)
	defer cancel()
	if err := s.annotations.Delete(ctx, by, annotationID); err != nil {
		s.logger.WarnContext(ctx, "delete the annotation of a pin that was not recorded", logattrs.Failure("observation_pin_cleanup_failed", err, slog.String("observation_id", id.String()), slog.String("annotation_id", annotationID.String()))...)
	}
}

func (s *Service) discardDashboard(ctx context.Context, by auth.Principal, id pgtype.UUID, dashboardID string) {
	made, err := db.ParseUUID(dashboardID)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), discardTimeout)
		defer cancel()
		err = s.dashboards.Delete(ctx, by, made)
	}
	if err != nil {
		s.logger.WarnContext(ctx, "delete the dashboard made from an observation that was not recorded", logattrs.Failure("observation_dashboard_cleanup_failed", err, slog.String("observation_id", id.String()), slog.String("dashboard_id", dashboardID))...)
	}
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

func (k outputKey) lockName() string {
	return fmt.Sprintf("dashboard:%s:%s:%s:%s", k.Observation, k.Block, k.Kind, k.Principal)
}

func (s *Service) madeDashboard(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, key outputKey) (dashboards.Dashboard, dashboards.Version, bool, error) {
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
	return d.WithAccess(access), v, true, nil
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
	parent, panels, err := s.panelsToMake(ctx, by, id, blockID)
	if err != nil {
		return none(err)
	}
	key, err := s.outputKeyFor(by, parent.ID, blockID)
	if err != nil {
		return none(err)
	}
	doc, err := s.panelsSpec(parent, panels)
	if err != nil {
		return none(fmt.Errorf("build the dashboard of an observation: %w", err))
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return none(fmt.Errorf("encode the dashboard of an observation: %w", err))
	}
	var made dashboards.Dashboard
	var version dashboards.Version
	var created bool
	err = s.locked(ctx, key.lockName(), func(q *dbsqlc.Queries) error {
		var err error
		made, version, created, err = s.makeOrReadDashboard(ctx, q, by, key, raw, again)
		return err
	})
	if err != nil {
		if created {
			s.discardDashboard(ctx, by, key.Observation, made.ID)
		}
		return none(err)
	}
	s.attachRunChat(ctx, by, runToAttach(parent, blockID, panels), made.ID)
	return made, version, created, nil
}

func (s *Service) panelsToMake(ctx context.Context, by auth.Principal, id, blockID pgtype.UUID) (dbsqlc.Observation, []dbsqlc.ObservationBlock, error) {
	if blockID.Valid {
		block, err := s.store.block(ctx, blockID)
		if err != nil {
			return dbsqlc.Observation{}, nil, err
		}
		if id.Valid && id != block.ObservationID {
			return dbsqlc.Observation{}, nil, ErrNotFound
		}
		id = block.ObservationID
	}
	parent, _, abilities, err := s.open(ctx, by, id)
	if err != nil {
		return dbsqlc.Observation{}, nil, err
	}
	rows, err := s.store.blocks(ctx, id)
	if err != nil {
		return dbsqlc.Observation{}, nil, err
	}
	panels := livePanels(rows, abilities, actor.Of(by, origin.Origin{}, ""), blockID)
	switch {
	case blockID.Valid && len(panels) == 0:
		return dbsqlc.Observation{}, nil, ErrNotFound
	case len(panels) == 0:
		return dbsqlc.Observation{}, nil, ErrNoPanels
	}
	return parent, panels, nil
}

func livePanels(rows []dbsqlc.ObservationBlock, abilities folders.Abilities, viewer actor.Actor, only pgtype.UUID) []dbsqlc.ObservationBlock {
	var panels []dbsqlc.ObservationBlock
	for _, b := range rows {
		if Kind(b.Kind) == KindPanel && BlockState(b.State) == BlockLive && visibleTo(b, abilities, viewer) && (!only.Valid || b.ID == only) {
			panels = append(panels, b)
		}
	}
	return panels
}

func (s *Service) makeOrReadDashboard(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, key outputKey, raw []byte, again bool) (dashboards.Dashboard, dashboards.Version, bool, error) {
	if !again {
		d, v, ok, err := s.madeDashboard(ctx, q, by, key)
		if err != nil || ok {
			return d, v, false, err
		}
	}
	if err := q.DeleteObservationDashboard(ctx, dbsqlc.DeleteObservationDashboardParams{ObservationID: key.Observation, BlockID: key.Block, CreatedByKind: key.Kind, CreatedByID: key.Principal}); err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, false, fmt.Errorf("replace made dashboard: %w", err)
	}
	d, v, err := s.dashboards.CreateCopy(ctx, by, raw, madeDashboardSummary, placement.Copied{Kind: folders.ContentObservation, ID: key.Observation})
	if err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, false, err
	}
	dashboardID, err := db.ParseUUID(d.ID)
	if err != nil {
		return d, v, true, err
	}
	if err := q.InsertObservationDashboard(ctx, dbsqlc.InsertObservationDashboardParams{ObservationID: key.Observation, BlockID: key.Block, DashboardID: dashboardID, CreatedByKind: key.Kind, CreatedByID: key.Principal}); err != nil {
		return d, v, true, fmt.Errorf("record made dashboard: %w", err)
	}
	return d, v, true, nil
}

func (s *Service) attachRunChat(ctx context.Context, by auth.Principal, runID pgtype.UUID, dashboardID string) {
	if !runID.Valid {
		return
	}
	dashboard, dashErr := db.ParseUUID(dashboardID)
	owner, ownerErr := db.ParseUUID(by.ID)
	err := errors.Join(dashErr, ownerErr)
	if err == nil {
		err = s.store.db.Queries().AttachRunChatToDashboard(ctx, dbsqlc.AttachRunChatToDashboardParams{DashboardID: dashboard, RunID: runID, OwnerKind: string(by.Kind), OwnerID: owner})
	}
	if err != nil {
		s.logger.WarnContext(ctx, "attach the chat to the dashboard made from its observation", logattrs.Failure("chat_attach_failed", err, slog.String("dashboard_id", dashboardID), slog.String("run_id", runID.String()))...)
	}
}

func runToAttach(parent dbsqlc.Observation, blockID pgtype.UUID, panels []dbsqlc.ObservationBlock) pgtype.UUID {
	if blockID.Valid && len(panels) == 1 && panels[0].RunID.Valid {
		return panels[0].RunID
	}
	return parent.RunID
}
