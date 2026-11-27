package runsession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/brief"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/agent/triage"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type Sweeper interface {
	Sweep(ctx context.Context, run tools.Run) triage.Outcome
}

type Triage struct {
	Block      string        `json:"block"`
	Leads      []triage.Lead `json:"leads,omitempty"`
	Calls      int           `json:"calls"`
	DurationMS int64         `json:"duration_ms"`
}

func (s *Service) SweepWith(sw Sweeper) { s.sweeper = sw }

func sweeps(run tools.Run) bool {
	return run.Mode == tools.ModeInvestigate && run.Investigation
}

func (s *Service) preflightOnFirstBrief(ctx context.Context, req revisions.Request, why brief.Reason, access datasource.Access) error {
	if why != brief.Fresh {
		return nil
	}
	err := s.preflight(ctx, req, access)
	if errors.Is(err, errRunNotRunning) {
		return tools.ErrRunEnded
	}
	return err
}

func (s *Service) preflight(ctx context.Context, req revisions.Request, access datasource.Access) error {
	if s.sweeper == nil {
		return nil
	}
	run, err := tools.RunFor(req)
	if err != nil || !sweeps(run) {
		return err
	}
	needed := false
	if err := s.with(ctx, req, func(_ context.Context, sess *session) error {
		needed = sess.state.Triage == nil && !sess.state.AgentHarness
		return nil
	}); err != nil || !needed {
		return err
	}
	run.Access = access
	swept := origin.With(auth.WithPrincipal(ctx, req.RequestedBy), req.WriteOrigin(origin.From(ctx)))
	out := s.sweeper.Sweep(swept, run)
	return s.with(ctx, req, func(_ context.Context, sess *session) error {
		if sess.state.Triage != nil {
			return nil
		}
		sess.state.Triage = &Triage{Block: out.Block, Leads: out.Leads, Calls: out.Calls, DurationMS: out.Duration.Milliseconds()}
		sess.state.seedLedger()
		sess.record(revisions.Step{
			At: time.Now(), Phase: tools.PhaseQuery, Tool: stopTool, Status: "ok", DurationMS: out.Duration.Milliseconds(),
			Message: fmt.Sprintf("Swept the datasources before the first model call: %d reads in %s", out.Calls, out.Duration.Round(100*time.Millisecond)),
		})
		return nil
	})
}

func (sess *session) triageBlock() string {
	if sess.state.Triage == nil {
		return ""
	}
	return sess.state.Triage.Block
}

type prior struct {
	Triage *Triage           `json:"triage"`
	Agent  *tools.AgentState `json:"agent_state"`
}

func priorSession(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID, attempt int32) (prior, error) {
	raw, err := q.GetPriorRunSession(ctx, dbsqlc.GetPriorRunSessionParams{AgentRequestID: id, Attempt: attempt})
	if errors.Is(err, pgx.ErrNoRows) {
		return prior{}, nil
	}
	if err != nil {
		return prior{}, fmt.Errorf("get the prior run session: %w", err)
	}
	var p prior
	if err := json.Unmarshal(raw, &p); err != nil {
		return prior{}, fmt.Errorf("decode the prior run session: %w", err)
	}
	return p, nil
}

func (p prior) carried() *tools.AgentState {
	if p.Agent == nil || isNull(p.Agent.Carry) {
		return nil
	}
	return &tools.AgentState{Carry: p.Agent.Carry}
}

func isNull(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}
