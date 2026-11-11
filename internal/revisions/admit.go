package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	maxClaimsPerCall = 8

	actsForAnotherMessage = "this agent acts for another person"
	noSaveMessage         = "the agent finished without saving a dashboard"
)

type Claimed struct {
	Row        dbsqlc.AgentRequest
	Request    Request
	FirstClaim bool
}

type admission int

const (
	admitted admission = iota
	settled
	released
)

type Ending struct {
	Failure    FailureCode
	Message    string
	Usage      *Usage
	Suggestion json.RawMessage
}

func (s *Service) Claim(ctx context.Context, c Claimant) (Claimed, bool, error) {
	for range maxClaimsPerCall {
		result, found, err := s.claim(ctx, c)
		if err != nil || !found {
			return Claimed{}, false, err
		}
		req, outcome := s.admit(ctx, c, result.request)
		switch outcome {
		case admitted:
			return Claimed{Row: result.request, Request: req, FirstClaim: result.firstClaim}, true, nil
		case released:
			return Claimed{}, false, nil
		}
	}
	return Claimed{}, false, nil
}

func (s *Service) admit(ctx context.Context, c Claimant, row dbsqlc.AgentRequest) (Request, admission) {
	if Source(row.Source) != SourceSuggest {
		if done, progressed := s.settleSaved(ctx, row, nil); done {
			if progressed {
				return Request{}, settled
			}
			return Request{}, released
		}
	}
	requester, err := s.Requester(ctx, row)
	switch {
	case errors.Is(err, auth.ErrPrincipalGone):
		s.finishLogged(ctx, row, outcome{message: requesterGoneMessage, code: "agent_requester_gone", failure: FailureRequesterGone})
		return Request{}, settled
	case err != nil:
		s.logger.ErrorContext(ctx, "load agent request requester", logattrs.Failure("agent_requester_load_failed", err, logAttrs(row)...)...)
		s.releaseLogged(ctx, row, true)
		return Request{}, released
	}
	switch err := s.checkRun(ctx, Source(row.Source), row.DashboardID, requester); {
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrDashboardNotFound):
		s.finishLogged(ctx, row, outcome{message: requesterForbiddenMessage, code: "agent_requester_forbidden", failure: FailureRequesterForbidden})
		return Request{}, settled
	case err != nil:
		s.logger.ErrorContext(ctx, "check agent request requester access", logattrs.Failure("agent_requester_access_failed", err, logAttrs(row)...)...)
		s.releaseLogged(ctx, row, true)
		return Request{}, released
	}
	if !actsFor(c, requester) {
		s.finishLogged(ctx, row, outcome{message: actsForAnotherMessage, code: "agent_acts_for_another", failure: FailureRequesterForbidden})
		return Request{}, settled
	}
	req := RequestOf(row)
	req.RequestedBy = requester
	return req, admitted
}

func (s *Service) Requester(ctx context.Context, row dbsqlc.AgentRequest) (auth.Principal, error) {
	return s.principals.Principal(ctx, auth.PrincipalKind(row.RequestedByKind), row.RequestedByID.String())
}

func (s *Service) Release(ctx context.Context, row dbsqlc.AgentRequest) {
	s.releaseLogged(ctx, row, false)
}

func (s *Service) RenewLease(ctx context.Context, row dbsqlc.AgentRequest) (canceled bool, err error) {
	cancelRequestedAt, err := s.db.Queries().RenewAgentRequestLease(ctx, dbsqlc.RenewAgentRequestLeaseParams{
		LeaseSeconds: s.leaseTTL.Seconds(), ID: row.ID, Attempts: row.Attempts,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, ErrLeaseLost
	case err != nil:
		return false, fmt.Errorf("renew agent request lease: %w", err)
	}
	return cancelRequestedAt.Valid, nil
}

func (s *Service) End(ctx context.Context, row dbsqlc.AgentRequest, e Ending) (dbsqlc.AgentRequest, error) {
	finishCtx, cancel := finishContext(ctx)
	defer cancel()
	o, err := s.endingOutcome(finishCtx, row, e)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	finished, err := s.finish(finishCtx, row, o)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if !finished {
		return dbsqlc.AgentRequest{}, ErrLeaseLost
	}
	stored, err := s.db.Queries().GetAgentRequestByID(finishCtx, row.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("get ended agent request: %w", err)
	}
	return stored, nil
}

func (s *Service) endingOutcome(ctx context.Context, row dbsqlc.AgentRequest, e Ending) (outcome, error) {
	if Source(row.Source) != SourceSuggest {
		success, saved, err := s.savedOutcome(ctx, row, e.Usage)
		if err != nil {
			return outcome{}, err
		}
		if saved {
			return success, nil
		}
	}
	current, err := s.db.Queries().GetAgentRequestByID(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return outcome{}, ErrRequestNotFound
	}
	if err != nil {
		return outcome{}, fmt.Errorf("get ending agent request: %w", err)
	}
	switch {
	case current.CancelRequestedAt.Valid:
		return canceledOutcome(nil, e.Usage), nil
	case e.Failure != "":
		return outcome{message: e.Message, code: "agent_run_failed", failure: e.Failure, usage: e.Usage}, nil
	case Source(row.Source) == SourceSuggest:
		return suggestionOutcome(Result{Suggestion: e.Suggestion, Usage: e.Usage}), nil
	case Source(row.Source) == SourceInvestigate:
		return outcome{message: noFindingMessage, code: "agent_no_finding", failure: FailureNoFinding, usage: e.Usage}, nil
	}
	return outcome{message: noSaveMessage, code: "agent_no_save", failure: FailureNoToolCall, usage: e.Usage}, nil
}

func (s *Service) savedOutcome(ctx context.Context, row dbsqlc.AgentRequest, usage *Usage) (success outcome, saved bool, err error) {
	finding := outcome{finding: true, usage: usage}
	if Source(row.Source) == SourceInvestigate {
		found, err := s.hasFinding(ctx, row)
		return finding, found, err
	}
	version, found, err := s.existingVersion(ctx, row)
	if err != nil {
		return outcome{}, false, err
	}
	if found {
		return outcome{version: &version, usage: usage}, true, nil
	}
	found, err = s.hasFinding(ctx, row)
	return finding, found, err
}

func (s *Service) settleSaved(ctx context.Context, row dbsqlc.AgentRequest, usage *Usage) (settled, progressed bool) {
	checkCtx, cancel := finishContext(ctx)
	defer cancel()
	success, saved, err := s.savedOutcome(checkCtx, row, usage)
	return s.settleChecked(ctx, row, err, saved, success)
}

func (s *Service) settleChecked(ctx context.Context, row dbsqlc.AgentRequest, err error, saved bool, success outcome) (settled, progressed bool) {
	switch {
	case err != nil:
		s.logger.ErrorContext(ctx, "check agent request result", logattrs.Failure("agent_request_check_failed", err, slog.String("agent_request_id", row.ID.String()))...)
		if int(row.CheckFailures)+1 >= maxCheckFailures {
			s.finishLogged(ctx, row, outcome{message: checkFailedMessage, code: "agent_request_check_failed", failure: FailureCheckFailed, cause: err})
			return true, true
		}
		s.releaseLogged(ctx, row, true)
		return true, false
	case saved:
		s.finishLogged(ctx, row, success)
		return true, true
	}
	return false, true
}

func (s *Service) releaseLogged(ctx context.Context, row dbsqlc.AgentRequest, checkFailed bool) {
	releaseCtx, cancel := finishContext(ctx)
	defer cancel()
	released, err := s.release(releaseCtx, row, checkFailed)
	attrs := logAttrs(row)
	switch {
	case err != nil:
		s.logger.ErrorContext(ctx, "release agent request; its lease will expire", logattrs.Failure("agent_request_release_failed", err, attrs...)...)
	case !released:
		s.logger.WarnContext(ctx, "agent request was finished elsewhere; not releasing it", attrs...)
	}
}

func (s *Service) finishLogged(ctx context.Context, row dbsqlc.AgentRequest, o outcome) {
	finishCtx, cancel := finishContext(ctx)
	defer cancel()
	finished, err := s.finish(finishCtx, row, o)
	attrs := logAttrs(row)
	switch {
	case err != nil:
		s.logger.ErrorContext(ctx, "finish agent request", logattrs.Failure("agent_request_finish_failed", err, attrs...)...)
	case !finished:
		s.logger.WarnContext(ctx, "agent request was finished elsewhere; dropping this result", attrs...)
	case o.suggestion != nil, o.finding:
		s.logger.InfoContext(ctx, "agent request succeeded", append(attrs, slog.Int("attempt", int(row.Attempts)))...)
	case o.version != nil:
		s.logger.InfoContext(ctx, "agent request succeeded", append(attrs, slog.Int("version", *o.version), slog.Int("attempt", int(row.Attempts)))...)
	default:
		s.logger.WarnContext(ctx, "agent request failed", logattrs.Failure(o.code, o.cause, append(attrs, slog.Int("attempt", int(row.Attempts)))...)...)
	}
}

func logAttrs(row dbsqlc.AgentRequest) []any {
	attrs := []any{slog.String("agent_request_id", row.ID.String())}
	if row.DashboardID.Valid {
		attrs = append(attrs, slog.String("dashboard_id", row.DashboardID.String()))
	}
	return attrs
}

func RequestOf(row dbsqlc.AgentRequest) Request {
	return Request{
		ID:           row.ID.String(),
		DashboardID:  row.DashboardID.String(),
		Source:       Source(row.Source),
		Prompt:       row.Prompt,
		PanelID:      row.PanelID.String,
		BaseVersion:  int(row.BaseVersion.Int32),
		Draft:        row.Draft,
		Attempt:      int(row.Attempts),
		RequestedBy:  requesterOf(row),
		Origin:       origin.Origin{Kind: origin.Kind(row.Origin.String), Client: row.OriginClient.String},
		Continues:    row.ContinuesID.Valid,
		ContinueNote: row.ContinueNote.String,
		From:         optionalTime(row.ContextFrom),
		To:           optionalTime(row.ContextTo),
	}
}
