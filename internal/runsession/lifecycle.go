package runsession

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/brief"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	stopTool      = "agent"
	internalStep  = "internal error"
	conflictNote  = "The dashboard changed while you were revising it, so your build was discarded. Read the dashboard again and make the change again on its latest version."
	outcomeFailed = "failed"
	outcomeDone   = "succeeded"
)

var (
	loopFailures = []revisions.FailureCode{
		revisions.FailureCallTimeout, revisions.FailureRateLimited, revisions.FailureProviderError, revisions.FailureOutputLimit,
		revisions.FailureNoToolCall, revisions.FailureNoFinding, revisions.FailureRefused, revisions.FailureNoFixture, revisions.FailureInternal,
	}
)

type endSpec struct {
	failure revisions.FailureCode
	message string
	step    string
}

func (sess *session) end(ctx context.Context, e endSpec) error {
	if e.step != "" {
		sess.record(revisions.Step{At: time.Now(), Phase: tools.PhaseSave, Tool: stopTool, Status: "error", Message: e.step})
	}
	if e.message == "" {
		e.message = revisions.FailureMessage(e.failure, sess.req.Source)
		if e.failure == revisions.FailureValidationExhausted {
			e.message += revisions.IssuesSuffix(sess.state.IssueCodes)
		}
	}
	row, err := sess.currentRow(ctx)
	if err == nil {
		err = sess.finish(ctx, row, revisions.Ending{Failure: e.failure, Message: e.message, Usage: sess.state.storedUsage(sess.s.prices)}, &e)
	}
	if errors.Is(err, revisions.ErrLeaseLost) {
		return sess.markEnded(ctx, row, &e)
	}
	return err
}

func (sess *session) currentRow(ctx context.Context) (dbsqlc.AgentRequest, error) {
	row, err := sess.s.runs.GetByID(ctx, sess.id)
	if err != nil {
		return row, err
	}
	if int(row.Attempts) != sess.req.Attempt {
		return row, revisions.ErrLeaseLost
	}
	return row, nil
}

func (sess *session) finish(ctx context.Context, row dbsqlc.AgentRequest, final revisions.Ending, e *endSpec) error {
	ended, err := sess.s.runs.End(ctx, row, final)
	if err != nil {
		return err
	}
	sess.s.claims.DropTokens(ctx, row)
	sess.markDone(ended, e)
	return nil
}

func (sess *session) markEnded(ctx context.Context, row dbsqlc.AgentRequest, e *endSpec) error {
	stored, err := sess.s.runs.GetByID(ctx, row.ID)
	if err != nil {
		return err
	}
	sess.markDone(stored, e)
	return nil
}

func (sess *session) markDone(row dbsqlc.AgentRequest, e *endSpec) {
	sess.ended, sess.endedBy, sess.endedRow = true, e, row
	sess.state.Settled = Settled{Kind: settledEnded}
}

func (sess *session) endForLoad(ctx context.Context, err error) error {
	var load *brief.LoadError
	switch {
	case errors.Is(err, dashboards.ErrNotFound):
		return sess.end(ctx, endSpec{failure: revisions.FailureNotFound})
	case errors.Is(err, brief.ErrDraftInvalid):
		return sess.end(ctx, endSpec{failure: revisions.FailureDraftInvalid, message: err.Error()})
	case errors.Is(err, brief.ErrInvalidTask):
		return sess.end(ctx, endSpec{failure: revisions.FailureInvalidTask, message: err.Error()})
	case errors.As(err, &load):
		return sess.end(ctx, endSpec{failure: load.Code, message: load.Message})
	}
	return err
}

func (sess *session) dashboardID() (id pgtype.UUID, ok bool, err error) {
	if sess.req.DashboardID == "" {
		return id, false, nil
	}
	id, err = db.ParseUUID(sess.req.DashboardID)
	return id, err == nil, err
}

func (sess *session) loadCurrent(ctx context.Context) (*brief.Revision, error) {
	id, has, err := sess.dashboardID()
	if err != nil {
		return nil, fmt.Errorf("run session: dashboard id: %w", err)
	}
	var current *brief.Revision
	switch sess.state.Guard.Mode {
	case tools.ModeRevise:
		current, err = sess.s.loader.Latest(ctx, id, sess.req.ID)
	case tools.ModeSuggest:
		current, err = sess.s.loader.Base(ctx, id, sess.req.BaseVersion, sess.req.PanelID, sess.req.Draft, sess.req.ID)
	case tools.ModeInvestigate:
		if has {
			current, err = sess.s.loader.Latest(ctx, id, sess.req.ID)
		}
	}
	if err != nil {
		return nil, sess.endForLoad(ctx, err)
	}
	return current, nil
}

func (sess *session) loadBuild(ctx context.Context) (*dashboards.Build, error) {
	if sess.state.Guard.Mode != tools.ModeCreate && sess.state.Guard.Mode != tools.ModeRevise {
		return nil, nil
	}
	build, found, err := sess.s.builds.Build(ctx, sess.id)
	if err != nil || !found {
		return nil, sess.failed(ctx, "could not load the build", err)
	}
	return &build, nil
}

func (sess *session) failed(ctx context.Context, what string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, dashboards.ErrNotFound) {
		return sess.end(ctx, endSpec{failure: revisions.FailureNotFound})
	}
	sess.s.failure(ctx, "run session build store failed", "run_session_build_failed", err, sess.req)
	return fmt.Errorf("%s: %w", what, err)
}

func (sess *session) refreshBuild(ctx context.Context) {
	build, found, err := sess.s.builds.Build(ctx, sess.id)
	switch {
	case err != nil:
		sess.s.failure(ctx, "run session could not read the build", "run_session_build_failed", err, sess.req)
	case found:
		sess.state.Build = buildProgress(build)
	default:
		sess.state.Build = nil
	}
}

func (sess *session) rebase(ctx context.Context) (bool, error) {
	onto, rebased, err := sess.s.builds.RebaseBuild(ctx, sess.req.RequestedBy, sess.id, sess.req.Attempt)
	if err != nil {
		return false, sess.failed(ctx, "could not rebase the build", err)
	}
	if rebased {
		sess.record(revisions.Step{At: time.Now(), Phase: tools.PhaseSave, Tool: stopTool, Status: "ok", Message: fmt.Sprintf("Rebased onto version %d", onto)})
	}
	return rebased, nil
}

func (sess *session) rebaseAndCommit(ctx context.Context) (commit dashboards.Commit, committed bool, err error) {
	rebased, err := sess.rebase(ctx)
	if err != nil || sess.ended || !rebased {
		return dashboards.Commit{}, false, err
	}
	commit, err = sess.s.builds.CommitBuild(ctx, sess.req.RequestedBy, sess.id, sess.req.Attempt)
	var conflict *dashboards.ConflictError
	var required *dashboards.ProposalRequiredError
	switch {
	case errors.As(err, &conflict):
		return dashboards.Commit{}, false, nil
	case errors.As(err, &required):
		return dashboards.Commit{}, false, err
	case err != nil:
		return dashboards.Commit{}, false, sess.failed(ctx, "could not save the build", err)
	}
	return commit, true, nil
}

func settledByCommit(commit dashboards.Commit) Settled {
	if commit.Proposal != nil {
		return Settled{Kind: settledProposed}
	}
	return Settled{Kind: settledSaved, Version: commit.Version.Number}
}

func (sess *session) restartFromLatest(ctx context.Context) error {
	if err := sess.s.builds.DiscardBuild(ctx, sess.id, sess.req.Attempt); err != nil {
		return sess.failed(ctx, "could not discard the build", err)
	}
	if sess.state.Restarts >= maxRestarts {
		return sess.end(ctx, endSpec{failure: revisions.FailureConflict, step: "Stopped: " + revisions.FailureMessage(revisions.FailureConflict, sess.req.Source)})
	}
	id, _, err := sess.dashboardID()
	if err != nil {
		return fmt.Errorf("run session: dashboard id: %w", err)
	}
	latest, err := sess.s.dashboards.LatestVersion(ctx, id)
	if err != nil {
		return sess.failed(ctx, "could not read the latest version", err)
	}
	sess.state.Restarts++
	sess.state.Guard.Restart()
	sess.state.Guard.BeginSession(false, latest)
	sess.state.Build = nil
	return nil
}

func (sess *session) resolveConflict(ctx context.Context) (restarted bool, err error) {
	commit, committed, err := sess.rebaseAndCommit(ctx)
	if err != nil || sess.ended {
		return false, err
	}
	if committed {
		sess.state.Settled = settledByCommit(commit)
		sess.state.Build = nil
		return false, nil
	}
	if err := sess.restartFromLatest(ctx); err != nil {
		return false, err
	}
	return !sess.ended, nil
}

func (sess *session) commit(ctx context.Context) (restarted bool, err error) {
	commit, err := sess.s.builds.CommitBuild(ctx, sess.req.RequestedBy, sess.id, sess.req.Attempt)
	var conflict *dashboards.ConflictError
	var required *dashboards.ProposalRequiredError
	switch {
	case err == nil:
		sess.state.Settled = settledByCommit(commit)
		sess.state.Build = nil
		return false, nil
	case errors.Is(err, dashboards.ErrBuildNotFound):
		return false, nil
	case errors.As(err, &required):
		return false, err
	case !errors.As(err, &conflict):
		return false, sess.failed(ctx, "could not save the build", err)
	}
	return sess.resolveConflict(ctx)
}

func briefTask(req revisions.Request, mode tools.Mode) brief.Task {
	return brief.Task{
		Mode: mode, Source: req.Source, Prompt: strings.TrimSpace(req.Prompt), PanelID: req.PanelID,
		From: req.From, To: req.To, ContinueNote: req.ContinueNote, ThreadContext: req.ThreadContext, Continues: req.Continues, Attempt: req.Attempt,
	}
}

func (s *Service) Brief(ctx context.Context, req revisions.Request, reason string) (tools.BriefAnswer, error) {
	why, ok := brief.ParseReason(reason)
	if !ok {
		return tools.BriefAnswer{}, tools.ErrInvalidReason
	}
	var answer tools.BriefAnswer
	ended := false
	err := s.with(ctx, req, func(ctx context.Context, sess *session) error {
		var err error
		answer, err = sess.brief(ctx, why)
		ended = sess.ended
		return err
	})
	if errors.Is(err, errRunNotRunning) || (err == nil && ended) {
		return tools.BriefAnswer{}, tools.ErrRunEnded
	}
	return answer, err
}

func (sess *session) brief(ctx context.Context, why brief.Reason) (tools.BriefAnswer, error) {
	mode := sess.state.Guard.Mode
	task := briefTask(sess.req, mode)
	current, build, restarted, err := sess.settleBuildOnLatest(ctx)
	if err != nil || sess.ended {
		return tools.BriefAnswer{}, err
	}
	switch {
	case restarted:
		why = brief.Conflict
	case build != nil && why == brief.Fresh:
		why = brief.ResumeReason(task)
	}
	parent := 0
	if current != nil {
		parent = current.Number
	}
	sess.state.Guard.BeginSession(build != nil, parent)
	sess.state.Build = nil
	if build != nil {
		sess.state.Build = buildProgress(*build)
	}
	message, err := brief.Render(sess.s.statuses.Health(ctx), task, brief.View{Current: current, Build: build}, why, sess.state.Digest.Text())
	if err != nil {
		sess.s.failure(ctx, "run session could not render the brief", "run_session_brief_failed", err, sess.req)
		return tools.BriefAnswer{}, fmt.Errorf("render the brief: %w", err)
	}
	return tools.BriefAnswer{System: message.System, Message: message.User, Reason: string(why), Build: sess.state.Build}, nil
}

func (sess *session) settleBuildOnLatest(ctx context.Context) (current *brief.Revision, build *dashboards.Build, restarted bool, err error) {
	for {
		if current, err = sess.loadCurrent(ctx); err != nil || sess.ended {
			return nil, nil, false, err
		}
		if build, err = sess.loadBuild(ctx); err != nil || sess.ended {
			return nil, nil, false, err
		}
		if sess.state.Guard.Mode != tools.ModeRevise || build == nil || current == nil || build.BaseVersion == current.Number {
			return current, build, restarted, nil
		}
		rebased, err := sess.rebase(ctx)
		if err != nil || sess.ended {
			return nil, nil, false, err
		}
		if rebased {
			continue
		}
		if err := sess.restartFromLatest(ctx); err != nil || sess.ended {
			return nil, nil, false, err
		}
		restarted = true
	}
}

func (s *Service) NoteUsage(ctx context.Context, req revisions.Request, u revisions.Usage) (stop bool, err error) {
	err = s.with(ctx, req, func(ctx context.Context, sess *session) error {
		sess.state.noteModel(u)
		if !s.overCostLimit(sess.state.usage()) {
			return nil
		}
		stop = true
		return sess.end(ctx, endSpec{failure: revisions.FailureBudgetExhausted, step: "Stopped: " + revisions.FailureMessage(revisions.FailureBudgetExhausted, req.Source)})
	})
	if errors.Is(err, errRunNotRunning) {
		return true, nil
	}
	return stop, err
}

func (s *Service) Finish(ctx context.Context, req revisions.Request, in tools.RunFinish) (tools.RunFinished, error) {
	failure := revisions.FailureCode(in.FailureCode)
	switch {
	case in.Outcome != outcomeDone && in.Outcome != outcomeFailed:
		return tools.RunFinished{}, claims.ErrInvalidOutcome
	case failure != "" && (in.Outcome != outcomeFailed || !slices.Contains(loopFailures, failure)):
		return tools.RunFinished{}, tools.ErrInvalidFailureCode
	case in.Commit && in.Outcome != outcomeDone:
		return tools.RunFinished{}, tools.ErrCommitNeedsSuccess
	}
	var out tools.RunFinished
	err := s.with(ctx, req, func(ctx context.Context, sess *session) error {
		var err error
		out, err = sess.finishRun(ctx, failure, in)
		return err
	})
	if errors.Is(err, errRunNotRunning) {
		return s.settledAnswer(ctx, req)
	}
	return out, err
}

func (s *Service) settledAnswer(ctx context.Context, req revisions.Request) (tools.RunFinished, error) {
	id, err := db.ParseUUID(req.ID)
	if err != nil {
		return tools.RunFinished{}, claims.ErrNotYourRun
	}
	row, err := s.runs.GetByID(ctx, id)
	if err != nil {
		return tools.RunFinished{}, err
	}
	if !revisions.State(row.State).Terminal() || int(row.Attempts) != req.Attempt {
		return tools.RunFinished{}, claims.ErrNotYourRun
	}
	return finishedOf(row, false), nil
}

func (sess *session) finishRun(ctx context.Context, failure revisions.FailureCode, in tools.RunFinish) (tools.RunFinished, error) {
	row, err := sess.currentRow(ctx)
	if errors.Is(err, revisions.ErrLeaseLost) {
		return sess.settled(ctx)
	}
	if err != nil {
		return tools.RunFinished{}, err
	}
	if in.Usage != nil {
		sess.state.noteModel(*in.Usage)
	}
	if in.Commit && sess.state.Guard.Mode != tools.ModeInvestigate && sess.state.Guard.Mode != tools.ModeSuggest {
		restarted, err := sess.commit(ctx)
		switch {
		case err != nil:
			return tools.RunFinished{}, err
		case sess.ended:
			return finishedOf(sess.endedRow, false), nil
		case restarted:
			return finishedOf(row, true), nil
		}
	}
	if err := sess.finish(ctx, row, sess.endingOf(failure, in), nil); err != nil {
		if errors.Is(err, revisions.ErrLeaseLost) {
			return sess.settled(ctx)
		}
		return tools.RunFinished{}, err
	}
	return finishedOf(sess.endedRow, false), nil
}

func (sess *session) endingOf(failure revisions.FailureCode, in tools.RunFinish) revisions.Ending {
	ending := revisions.Ending{Usage: sess.state.storedUsage(sess.s.prices)}
	switch {
	case in.Outcome == outcomeFailed && failure != "":
		ending.Failure, ending.Message = failure, revisions.FailureMessage(failure, sess.req.Source)
	case in.Outcome == outcomeFailed:
		ending.Failure = revisions.FailureAgentFailed
		if ending.Message = claims.CleanText(in.Message, maxMessageRunes); ending.Message == "" {
			ending.Message = silentFailure
		}
	case sess.state.Guard.Mode == tools.ModeSuggest:
		ending.Suggestion = sess.state.Suggestion
	}
	return ending
}

func (sess *session) settled(ctx context.Context) (tools.RunFinished, error) {
	out, err := sess.s.settledAnswer(ctx, sess.req)
	if err == nil {
		sess.ended = true
	}
	return out, err
}

func finishedOf(row dbsqlc.AgentRequest, restarted bool) tools.RunFinished {
	out := tools.RunFinished{State: row.State, ResultKind: row.ResultKind, Restarted: restarted}
	if row.ResultVersion.Valid {
		version := int(row.ResultVersion.Int32)
		out.ResultVersion = &version
	}
	if row.FailureCode.Valid {
		out.FailureCode = &row.FailureCode.String
	}
	return out
}
