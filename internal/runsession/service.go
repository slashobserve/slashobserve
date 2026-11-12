package runsession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/brief"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/llm/pricing"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const maxRestarts = 1

var (
	errRunNotRunning = errors.New("the run is not running at this attempt")
)

type Builds interface {
	Build(ctx context.Context, requestID pgtype.UUID) (dashboards.Build, bool, error)
	CommitBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID, attempt int) (dashboards.Dashboard, dashboards.Version, bool, error)
	DiscardBuild(ctx context.Context, requestID pgtype.UUID, attempt int) error
	RebaseBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID, attempt int) (int, bool, error)
}

type Dashboards interface {
	brief.DashboardReader
	LatestVersion(ctx context.Context, id pgtype.UUID) (int, error)
}

type Statuses interface {
	Health(ctx context.Context) []datasource.Status
}

type Config struct {
	DB            *db.DB
	Runs          *revisions.Service
	Claims        *claims.Service
	Builds        Builds
	Dashboards    Dashboards
	Imports       brief.ImportReader
	Statuses      Statuses
	Steps         revisions.StepRecorder
	Prices        pricing.Table
	MaxRunCostUSD float64
	Logger        *slog.Logger
}

type Service struct {
	db         *db.DB
	runs       *revisions.Service
	claims     *claims.Service
	builds     Builds
	dashboards Dashboards
	loader     brief.Loader
	statuses   Statuses
	steps      revisions.StepRecorder
	prices     pricing.Table
	maxRunCost float64
	logger     *slog.Logger
}

func New(c Config) *Service {
	if c.DB == nil || c.Runs == nil || c.Claims == nil || c.Builds == nil || c.Dashboards == nil || c.Statuses == nil || c.Steps == nil {
		panic("runsession.New needs a database, the runs and claims services, builds, dashboards, datasource statuses and a step recorder")
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return &Service{
		db: c.DB, runs: c.Runs, claims: c.Claims, builds: c.Builds, dashboards: c.Dashboards,
		loader:   brief.Loader{Dashboards: c.Dashboards, Imports: c.Imports, Logger: c.Logger},
		statuses: c.Statuses, steps: c.Steps, prices: c.Prices, maxRunCost: c.MaxRunCostUSD, logger: c.Logger,
	}
}

type session struct {
	s        *Service
	q        *dbsqlc.Queries
	req      revisions.Request
	id       pgtype.UUID
	state    State
	loaded   []byte
	steps    []revisions.Step
	ended    bool
	endedBy  *endSpec
	endedRow dbsqlc.AgentRequest
}

func (s *Service) with(ctx context.Context, req revisions.Request, fn func(ctx context.Context, sess *session) error) error {
	id, err := db.ParseUUID(req.ID)
	if err != nil {
		return fmt.Errorf("run session: run id: %w", err)
	}
	ctx = auth.WithPrincipal(ctx, req.RequestedBy)
	var steps []revisions.Step
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		sess, err := s.open(ctx, q, req, id)
		if err != nil {
			return err
		}
		if err := fn(ctx, sess); err != nil {
			return err
		}
		steps = sess.steps
		return sess.store(ctx)
	})
	if err != nil {
		return err
	}
	for _, step := range steps {
		s.steps.RecordStep(context.WithoutCancel(ctx), req, step)
	}
	return nil
}

func (s *Service) open(ctx context.Context, q *dbsqlc.Queries, req revisions.Request, id pgtype.UUID) (*session, error) {
	key := dbsqlc.GetRunSessionForUpdateParams{AgentRequestID: id, Attempt: httpapi.Int32(req.Attempt)}
	raw, err := q.GetRunSessionForUpdate(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		raw, err = s.create(ctx, q, req, id)
	}
	if err != nil {
		return nil, err
	}
	sess := &session{s: s, q: q, req: req, id: id, loaded: raw}
	if err := json.Unmarshal(raw, &sess.state); err != nil {
		return nil, fmt.Errorf("decode run session: %w", err)
	}
	return sess, nil
}

func (s *Service) create(ctx context.Context, q *dbsqlc.Queries, req revisions.Request, id pgtype.UUID) ([]byte, error) {
	row, err := q.GetAgentRequestByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (revisions.State(row.State).Terminal() || int(row.Attempts) != req.Attempt)) {
		return nil, errRunNotRunning
	}
	if err != nil {
		return nil, fmt.Errorf("get run for its session: %w", err)
	}
	initial, err := s.initial(ctx, req, id)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(initial)
	if err != nil {
		return nil, fmt.Errorf("encode run session: %w", err)
	}
	attempt := httpapi.Int32(req.Attempt)
	if _, err := q.InsertRunSession(ctx, dbsqlc.InsertRunSessionParams{AgentRequestID: id, Attempt: attempt, State: raw}); err != nil {
		return nil, fmt.Errorf("insert run session: %w", err)
	}
	if err := q.DeleteOlderRunSessions(ctx, dbsqlc.DeleteOlderRunSessionsParams{AgentRequestID: id, Attempt: attempt}); err != nil {
		return nil, fmt.Errorf("delete older run sessions: %w", err)
	}
	return q.GetRunSessionForUpdate(ctx, dbsqlc.GetRunSessionForUpdateParams{AgentRequestID: id, Attempt: attempt})
}

func (s *Service) initial(ctx context.Context, req revisions.Request, id pgtype.UUID) (State, error) {
	run, err := tools.RunFor(req)
	if err != nil {
		return State{}, err
	}
	state := State{Guard: guard.New(run.Mode)}
	parent := 0
	switch {
	case run.Mode == tools.ModeSuggest:
		parent = req.BaseVersion
	case run.DashboardID != "":
		dashboardID, err := db.ParseUUID(run.DashboardID)
		if err != nil {
			return State{}, fmt.Errorf("run session: dashboard id: %w", err)
		}
		if parent, err = s.dashboards.LatestVersion(ctx, dashboardID); err != nil {
			return State{}, err
		}
	}
	building := false
	if run.Build {
		build, found, err := s.builds.Build(ctx, id)
		if err != nil {
			return State{}, fmt.Errorf("get the run's build: %w", err)
		}
		if found {
			building, state.Build = true, buildProgress(build)
		}
	}
	state.Guard.BeginSession(building, parent)
	return state, nil
}

func buildProgress(b dashboards.Build) *tools.BuildProgress {
	p := b.Progress()
	return &tools.BuildProgress{Saves: b.Saves, Panels: p.Panels, PlannedPanels: p.PlannedPanels}
}

func (sess *session) store(ctx context.Context) error {
	if sess.ended {
		if err := sess.q.DeleteRunSession(ctx, dbsqlc.DeleteRunSessionParams{AgentRequestID: sess.id, Attempt: httpapi.Int32(sess.req.Attempt)}); err != nil {
			return fmt.Errorf("delete run session: %w", err)
		}
		return nil
	}
	raw, err := json.Marshal(sess.state)
	if err != nil {
		return fmt.Errorf("encode run session: %w", err)
	}
	if bytes.Equal(raw, sess.loaded) {
		return nil
	}
	if err := sess.q.UpsertRunSession(ctx, dbsqlc.UpsertRunSessionParams{AgentRequestID: sess.id, Attempt: httpapi.Int32(sess.req.Attempt), State: raw}); err != nil {
		return fmt.Errorf("store run session: %w", err)
	}
	return nil
}

func (sess *session) record(step revisions.Step) {
	sess.steps = append(sess.steps, step)
}

func (s *Service) Scope(ctx context.Context, req revisions.Request) (tools.Run, error) {
	run, err := tools.RunFor(req)
	if err != nil {
		return tools.Run{}, err
	}
	ended := false
	err = s.with(ctx, req, func(ctx context.Context, sess *session) error {
		run.ParentVersion = sess.state.Guard.ParentVersion
		if run.Mode != tools.ModeSuggest {
			return nil
		}
		current, err := sess.loadCurrent(ctx)
		if err != nil || sess.ended {
			ended = sess.ended
			return err
		}
		run.Base = current.Spec
		return nil
	})
	if errors.Is(err, errRunNotRunning) || (err == nil && ended) {
		return tools.Run{}, tools.ErrRunEnded
	}
	return run, err
}

func (s *Service) Progress(ctx context.Context, req revisions.Request) (*tools.BuildProgress, error) {
	run, err := tools.RunFor(req)
	if err != nil || !run.Build {
		return nil, err
	}
	id, err := db.ParseUUID(req.ID)
	if err != nil {
		return nil, fmt.Errorf("run session: run id: %w", err)
	}
	build, found, err := s.builds.Build(ctx, id)
	if err != nil || !found {
		return nil, err
	}
	return buildProgress(build), nil
}

func (s *Service) failure(ctx context.Context, message string, code string, err error, req revisions.Request) {
	s.logger.ErrorContext(ctx, message, logattrs.Failure(code, err, slog.String("agent_request_id", req.ID))...)
}
