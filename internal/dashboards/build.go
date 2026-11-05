package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var (
	ErrBuildLost       = errors.New("the build no longer belongs to this attempt")
	ErrBuildNotFound   = errors.New("build not found")
	ErrBuildEmpty      = errors.New("the build has no panels")
	ErrBuildNotSavable = errors.New("the request is no longer a failed request with an unsaved build")
)

type Build struct {
	RequestID     pgtype.UUID
	DashboardID   pgtype.UUID
	BaseVersion   int
	Spec          *spec.Dashboard
	Unverified    Unverified
	PlannedPanels int
	Summary       string
	Saves         int
	UpdatedAt     time.Time

	requestedByKind string
	requestedByID   pgtype.UUID
	origin          origin.Origin
}

func (b Build) Progress() BuildProgress {
	return buildProgress(b.Spec, b.PlannedPanels)
}

type BuildSave struct {
	RequestID     pgtype.UUID
	Attempt       int
	DashboardID   pgtype.UUID
	BaseVersion   int
	Spec          []byte
	Summary       string
	PlannedPanels *int
}

func (s *Service) SaveBuild(ctx context.Context, by auth.Principal, in BuildSave) (Build, error) {
	revision := in.DashboardID.Valid
	fallback := defaultCreateSummary
	if revision {
		if _, err := s.need(ctx, by, in.DashboardID, folders.NeedEdit); err != nil {
			return Build{}, err
		}
		fallback = defaultUpdateSummary
	} else if _, err := s.creationTarget(ctx, by, pgtype.UUID{}); err != nil {
		return Build{}, err
	}
	summary, parsed, canonical, err := validateSave(in.Spec, in.Summary, fallback)
	if err != nil {
		return Build{}, err
	}
	if in.PlannedPanels != nil {
		if issues := validatePlannedPanels(*in.PlannedPanels); len(issues) > 0 {
			return Build{}, &InvalidSpecError{Issues: issues}
		}
	}
	stored, found, err := s.Build(ctx, in.RequestID)
	if err != nil {
		return Build{}, err
	}
	planned := stored.PlannedPanels
	if in.PlannedPanels != nil {
		planned = *in.PlannedPanels
	}
	prev, prevUnverified, err := s.buildParent(ctx, in, stored, found)
	if err != nil {
		return Build{}, err
	}
	carried, err := s.verify(ctx, by, prev, prevUnverified, parsed, prev != nil)
	if err != nil {
		return Build{}, err
	}
	progress := buildProgress(parsed, planned)
	row, err := s.store.saveBuild(ctx, in, canonical, carried, summary, progress)
	if err != nil {
		return Build{}, err
	}
	return Build{
		RequestID:     in.RequestID,
		DashboardID:   in.DashboardID,
		BaseVersion:   in.BaseVersion,
		Spec:          parsed,
		Unverified:    carried,
		PlannedPanels: planned,
		Summary:       summary,
		Saves:         int(row.Saves),
		UpdatedAt:     row.UpdatedAt.Time.UTC(),
	}, nil
}

func (s *Service) buildParent(ctx context.Context, in BuildSave, stored Build, found bool) (*spec.Dashboard, Unverified, error) {
	switch {
	case found:
		return stored.Spec, stored.Unverified, nil
	case in.DashboardID.Valid:
		prev, err := s.VersionSpec(ctx, in.DashboardID, in.BaseVersion)
		if err != nil {
			return nil, Unverified{}, err
		}
		unverified, err := s.store.unverified(ctx, in.DashboardID, in.BaseVersion)
		return prev, unverified, err
	}
	return nil, Unverified{}, nil
}

func (s *Service) Build(ctx context.Context, requestID pgtype.UUID) (Build, bool, error) {
	row, err := s.store.db.Queries().GetAgentRequestBuild(ctx, requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Build{}, false, nil
	}
	if err != nil {
		return Build{}, false, fmt.Errorf("get agent request build: %w", err)
	}
	build, err := buildOf(row.AgentRequestID, row.DashboardID, row.BaseVersion, row.Spec, row.Unverified, int(row.PlannedPanels), row.Summary, row.Saves, row.UpdatedAt)
	return build, err == nil, err
}

func (s *Service) viewRow(ctx context.Context, requestID pgtype.UUID) (Build, bool, error) {
	row, err := s.store.db.Queries().GetAgentRequestBuildView(ctx, requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Build{}, false, nil
	}
	if err != nil {
		return Build{}, false, fmt.Errorf("get agent request build view: %w", err)
	}
	build, err := buildOf(row.AgentRequestID, row.DashboardID, row.BaseVersion, row.Spec, row.Unverified, int(row.PlannedPanels), row.Summary, row.Saves, row.UpdatedAt)
	if err != nil {
		return Build{}, false, err
	}
	build.requestedByKind = row.RequestedByKind
	build.requestedByID = row.RequestedByID
	build.origin = origin.Origin{Kind: origin.Kind(row.Origin.String), Client: row.OriginClient.String}
	return build, true, nil
}

func buildOf(requestID, dashboardID pgtype.UUID, baseVersion pgtype.Int4, rawSpec, rawUnverified []byte, plannedPanels int, summary string, saves int32, updatedAt pgtype.Timestamptz) (Build, error) {
	d, err := decodeSpec(rawSpec)
	if err != nil {
		return Build{}, err
	}
	unverified, err := decodeUnverified(rawUnverified)
	if err != nil {
		return Build{}, err
	}
	return Build{
		RequestID:     requestID,
		DashboardID:   dashboardID,
		BaseVersion:   int(baseVersion.Int32),
		Spec:          d,
		Unverified:    unverified,
		PlannedPanels: plannedPanels,
		Summary:       summary,
		Saves:         int(saves),
		UpdatedAt:     updatedAt.Time.UTC(),
	}, nil
}

func (s *Service) ViewBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (Build, error) {
	build, found, err := s.viewRow(ctx, requestID)
	if err != nil {
		return Build{}, err
	}
	if !found || build.requestedByKind != string(by.Kind) || build.requestedByID.String() != by.ID {
		return Build{}, ErrBuildNotFound
	}
	if !build.DashboardID.Valid {
		return build, nil
	}
	access, err := s.access.Dashboard(ctx, by, build.DashboardID)
	if errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrNoAccess) {
		return Build{}, ErrBuildNotFound
	}
	if err != nil {
		return Build{}, err
	}
	if !access.Abilities.Has(folders.NeedView) {
		return Build{}, ErrBuildNotFound
	}
	return build, nil
}

func (s *Service) CommitBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID, attempt int) (Dashboard, Version, bool, error) {
	dash, ver, saved, err := s.store.savedByRequest(ctx, requestID, pgtype.UUID{})
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	if saved {
		if err := s.DiscardBuild(ctx, requestID, attempt); err != nil {
			return Dashboard{}, Version{}, false, err
		}
		return dash, ver, true, nil
	}
	build, found, err := s.Build(ctx, requestID)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	if !found {
		return Dashboard{}, Version{}, false, ErrBuildNotFound
	}
	return s.commitBuild(ctx, by, build, build.Summary, func(q *dbsqlc.Queries, _ pgtype.UUID, _ int) error {
		n, err := q.DeleteAgentRequestBuild(ctx, dbsqlc.DeleteAgentRequestBuildParams{AgentRequestID: requestID, Attempt: httpapi.Int32(attempt)})
		if err != nil {
			return fmt.Errorf("delete agent request build: %w", err)
		}
		if n == 0 {
			return ErrBuildLost
		}
		return nil
	})
}

func (s *Service) DiscardBuild(ctx context.Context, requestID pgtype.UUID, attempt int) error {
	_, err := s.store.db.Queries().DeleteAgentRequestBuild(ctx, dbsqlc.DeleteAgentRequestBuildParams{AgentRequestID: requestID, Attempt: httpapi.Int32(attempt)})
	if err != nil {
		return fmt.Errorf("delete agent request build: %w", err)
	}
	return nil
}

func (s *Service) SaveStoppedBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	if dash, ver, found, err := s.store.savedByRequest(ctx, requestID, pgtype.UUID{}); err != nil || found {
		return dash, ver, found, err
	}
	build, found, err := s.viewRow(ctx, requestID)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	if !found {
		return Dashboard{}, Version{}, false, ErrBuildNotFound
	}
	progress := build.Progress()
	if progress.Panels == 0 {
		return Dashboard{}, Version{}, false, ErrBuildEmpty
	}
	if build.origin.Kind != "" {
		ctx = origin.With(ctx, build.origin)
	}
	return s.commitBuild(ctx, by, build, stoppedSummary(progress), func(q *dbsqlc.Queries, dashboardID pgtype.UUID, version int) error {
		n, err := q.MarkAgentRequestBuildSaved(ctx, dbsqlc.MarkAgentRequestBuildSavedParams{ResultVersion: httpapi.Int32(version), DashboardID: dashboardID, ID: requestID})
		if err != nil {
			return fmt.Errorf("mark agent request build saved: %w", err)
		}
		if n == 0 {
			return ErrBuildNotSavable
		}
		deleted, err := q.DeleteStoppedAgentRequestBuild(ctx, requestID)
		if err != nil {
			return fmt.Errorf("delete stopped agent request build: %w", err)
		}
		if deleted != 1 {
			return ErrBuildNotSavable
		}
		return nil
	})
}

func (s *Service) commitBuild(ctx context.Context, by auth.Principal, build Build, summary string, inTx func(q *dbsqlc.Queries, dashboardID pgtype.UUID, version int) error) (Dashboard, Version, bool, error) {
	if !build.DashboardID.Valid {
		closed, err := closeLines(build.Spec)
		if err != nil {
			s.logger.ErrorContext(ctx, "could not close the lines of a build", logattrs.Failure("close_lines_failed", err, slog.String("request_id", build.RequestID.String()))...)
		} else {
			build.Spec = closed
		}
	}
	canonical, err := json.Marshal(build.Spec)
	if err != nil {
		return Dashboard{}, Version{}, false, fmt.Errorf("marshal spec: %w", err)
	}
	in := newVersion{spec: build.Spec, canonical: canonical, summary: summary, author: authorOf(by), requestID: build.RequestID, origin: origin.From(ctx), unverified: build.Unverified}
	if !build.DashboardID.Valid {
		return s.commitCreate(ctx, by, in, inTx)
	}
	return s.commitRevision(ctx, by, build, in, inTx)
}

func (s *Service) commitCreate(ctx context.Context, by auth.Principal, in newVersion, inTx func(q *dbsqlc.Queries, dashboardID pgtype.UUID, version int) error) (Dashboard, Version, bool, error) {
	target, err := s.creationTarget(ctx, by, pgtype.UUID{})
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	in.folder = target.folder
	dash, ver, err := s.store.createWith(ctx, in, func(q *dbsqlc.Queries, dash Dashboard, ver Version) error {
		id, err := db.ParseUUID(dash.ID)
		if err != nil {
			return fmt.Errorf("dashboard id: %w", err)
		}
		return inTx(q, id, ver.Number)
	})
	if errors.Is(err, errRequestSaved) {
		return s.replayedCreate(ctx, in.requestID)
	}
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	dash.Abilities = target.abilities
	return dash, ver, false, nil
}

func (s *Service) commitRevision(ctx context.Context, by auth.Principal, build Build, in newVersion, inTx func(q *dbsqlc.Queries, dashboardID pgtype.UUID, version int) error) (Dashboard, Version, bool, error) {
	access, err := s.need(ctx, by, build.DashboardID, folders.NeedEdit)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	prev, err := s.VersionSpec(ctx, build.DashboardID, build.BaseVersion)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	if freezeResized(prev, in.spec) {
		if in.canonical, err = json.Marshal(in.spec); err != nil {
			return Dashboard{}, Version{}, false, fmt.Errorf("marshal spec: %w", err)
		}
	}
	in.dashboardID = build.DashboardID
	in.parent = build.BaseVersion
	dash, ver, err := s.store.addVersionWith(ctx, in, func(q *dbsqlc.Queries, _ Dashboard, ver Version) error {
		return inTx(q, build.DashboardID, ver.Number)
	})
	switch {
	case errors.Is(err, errStaleParent):
		return Dashboard{}, Version{}, false, s.conflict(ctx, build.DashboardID)
	case errors.Is(err, errRequestSaved):
		dash, ver, found, err := s.store.savedByRequest(ctx, in.requestID, build.DashboardID)
		if err != nil {
			return Dashboard{}, Version{}, false, err
		}
		if !found {
			return Dashboard{}, Version{}, false, fmt.Errorf("revise dashboard: %w", errRequestSaved)
		}
		return withAccess(dash, access), ver, true, nil
	case err != nil:
		return Dashboard{}, Version{}, false, err
	}
	return withAccess(dash, access), ver, false, nil
}

func (st store) saveBuild(ctx context.Context, in BuildSave, canonical []byte, unverified Unverified, summary string, progress BuildProgress) (dbsqlc.AgentRequestBuild, error) {
	params := dbsqlc.UpsertAgentRequestBuildParams{
		AgentRequestID: in.RequestID,
		DashboardID:    in.DashboardID,
		Spec:           canonical,
		Summary:        summary,
		PlannedPanels:  httpapi.Int32(progress.PlannedPanels),
		Panels:         httpapi.Int32(progress.Panels),
		Attempt:        httpapi.Int32(in.Attempt),
	}
	if in.DashboardID.Valid {
		params.BaseVersion = pgtype.Int4{Int32: httpapi.Int32(in.BaseVersion), Valid: true}
	}
	if !unverified.Empty() {
		var err error
		if params.Unverified, err = json.Marshal(unverified); err != nil {
			return dbsqlc.AgentRequestBuild{}, fmt.Errorf("marshal unverified: %w", err)
		}
	}
	var row dbsqlc.AgentRequestBuild
	err := st.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		saved, err := q.UpsertAgentRequestBuild(ctx, params)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBuildLost
		}
		if err != nil {
			return fmt.Errorf("upsert agent request build: %w", err)
		}
		row = saved
		if !in.DashboardID.Valid {
			return nil
		}
		payload, err := json.Marshal(events.Event{
			Type:        events.TypeRevisionUpdated,
			DashboardID: in.DashboardID.String(),
			RevisionID:  in.RequestID.String(),
			Status:      "running",
			BuildSaves:  int(row.Saves),
		})
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}
		if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: events.Channel, Payload: string(payload)}); err != nil {
			return fmt.Errorf("notify event: %w", err)
		}
		return nil
	})
	return row, err
}
