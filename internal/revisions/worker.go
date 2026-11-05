package revisions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	defaultWorkers   = 2
	defaultPoll      = 5 * time.Second
	heartbeatEvery   = 5 * time.Second
	finishTimeout    = 15 * time.Second
	listenBackoff    = time.Second
	maxListenBackoff = 30 * time.Second

	maxCheckFailures   = 3
	checkFailedMessage = "could not check the request's saved version"
	timedOutMessage    = "the agent run timed out"
	crashedMessage     = "the agent run crashed"

	requesterGoneMessage = "the person or key that asked for this run no longer exists"

	noSuggestionMessage = "the agent did not propose a panel"
	noFindingMessage    = "the agent finished without saving a finding"
)

var errCrashed = errors.New(crashedMessage)

type WorkerOptions struct {
	Workers    int
	Poll       time.Duration
	RunTimeout time.Duration
}

type Worker struct {
	service *Service
	listen  events.Listener
	logger  *slog.Logger
	opts    WorkerOptions

	heartbeat time.Duration
}

func NewWorker(service *Service, listen events.Listener, logger *slog.Logger, opts WorkerOptions) *Worker {
	if service.principals == nil {
		panic("revisions: NewWorker needs a Service built with WithPrincipals")
	}
	if service.access == nil {
		panic("revisions: NewWorker needs a Service built with WithAccess")
	}
	if opts.Workers <= 0 {
		opts.Workers = defaultWorkers
	}
	if opts.Poll <= 0 {
		opts.Poll = defaultPoll
	}
	return &Worker{service: service, listen: listen, logger: logger, opts: opts, heartbeat: heartbeatEvery}
}

func (w *Worker) Run(ctx context.Context) {
	wakes := make([]chan struct{}, w.opts.Workers)
	for i := range wakes {
		wakes[i] = make(chan struct{}, 1)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.relayWakeups(ctx, wakes)
	}()
	for _, wake := range wakes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx, wake)
		}()
	}
	wg.Wait()
}

func (w *Worker) relayWakeups(ctx context.Context, wakes []chan struct{}) {
	backoff := listenBackoff
	for ctx.Err() == nil {
		established := false
		err := w.listen(ctx, WakeChannel, func() {
			established = true
			signal(wakes)
		}, func(string) { signal(wakes) })
		if ctx.Err() != nil {
			return
		}
		w.logger.Warn("agent request listener failed", logattrs.Failure("agent_request_listener_failed", err, slog.Duration("retry_in", backoff))...)
		if established {
			backoff = listenBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxListenBackoff)
	}
}

func signal(wakes []chan struct{}) {
	for _, wake := range wakes {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func (w *Worker) loop(ctx context.Context, wake <-chan struct{}) {
	for ctx.Err() == nil {
		worked, err := w.step(ctx)
		if err != nil && ctx.Err() == nil {
			w.logger.Error("agent request poll failed", logattrs.Failure("agent_request_poll_failed", err)...)
		}
		if worked && err == nil {
			continue
		}
		timer := time.NewTimer(w.opts.Poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (w *Worker) step(ctx context.Context) (bool, error) {
	claimed, found, err := w.service.claim(ctx)
	if err != nil || !found {
		return false, err
	}
	return w.process(ctx, claimed), nil
}

func (w *Worker) process(ctx context.Context, claimed dbsqlc.AgentRequest) bool {
	source := Source(claimed.Source)
	if source != SourceSuggest {
		if settled, progressed := w.settleSaved(ctx, claimed, nil); settled {
			return progressed
		}
	}

	if claimed.CancelRequestedAt.Valid {
		w.finish(ctx, claimed, canceledOutcome(nil, nil))
		return true
	}

	req := requestFrom(claimed)
	requester, err := w.service.principals.Principal(ctx, auth.PrincipalKind(claimed.RequestedByKind), claimed.RequestedByID.String())
	switch {
	case errors.Is(err, auth.ErrPrincipalGone):
		w.finish(ctx, claimed, outcome{message: requesterGoneMessage, code: "agent_requester_gone", failure: FailureRequesterGone})
		return true
	case err != nil:
		w.logger.ErrorContext(ctx, "load agent request requester", logattrs.Failure("agent_requester_load_failed", err, logAttrs(claimed)...)...)
		w.release(ctx, claimed, true)
		return false
	}
	req.RequestedBy = requester
	switch err := w.service.checkRun(ctx, Source(claimed.Source), claimed.DashboardID, requester); {
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrDashboardNotFound):
		w.finish(ctx, claimed, outcome{message: requesterForbiddenMessage, code: "agent_requester_forbidden", failure: FailureRequesterForbidden})
		return true
	case err != nil:
		w.logger.ErrorContext(ctx, "check agent request requester access", logattrs.Failure("agent_requester_access_failed", err, logAttrs(claimed)...)...)
		w.release(ctx, claimed, true)
		return false
	}

	result, run, err := w.runLeased(ctx, claimed, req)
	if run.lost || errors.Is(err, ErrLeaseLost) {
		w.logger.WarnContext(ctx, "agent request lease lost; dropping this attempt's work", logattrs.Failure("agent_request_lease_lost", ErrLeaseLost, append(logAttrs(claimed), slog.Int("attempt", int(claimed.Attempts)))...)...)
		return false
	}
	if err == nil {
		w.finish(ctx, claimed, successOutcome(source, result))
		return true
	}
	return w.settleFailure(ctx, claimed, run, result, err)
}

func (w *Worker) settleFailure(ctx context.Context, claimed dbsqlc.AgentRequest, run runEnd, result Result, err error) bool {
	if claimed.Source != string(SourceSuggest) {
		if settled, progressed := w.settleSaved(ctx, claimed, result.Usage); settled {
			return progressed
		}
	}
	crashed := errors.Is(err, errCrashed)
	switch settlementFor(run, ctx.Err() != nil, crashed) {
	case settleCanceled:
		w.finish(ctx, claimed, canceledOutcome(err, result.Usage))
	case settleRelease:
		w.logger.InfoContext(ctx, "agent request interrupted; releasing it", slog.String("agent_request_id", claimed.ID.String()))
		w.release(ctx, claimed, false)
		return false
	case settleTimedOut:
		code, failure := "agent_run_timed_out", FailureRunTimeout
		if crashed {
			code, failure = "agent_run_panicked", FailureCrashed
		}
		w.finish(ctx, claimed, outcome{message: timedOutMessage, code: code, failure: failure, cause: err, usage: result.Usage})
	default:
		code, failure := "agent_run_failed", result.FailureCode
		if crashed {
			code, failure = "agent_run_panicked", FailureCrashed
		}
		if failure == "" {
			failure = FailureInternal
		}
		w.finish(ctx, claimed, outcome{message: err.Error(), code: code, failure: failure, cause: err, usage: result.Usage})
	}
	return true
}

type settlement int

const (
	settleFailed settlement = iota
	settleCanceled
	settleRelease
	settleTimedOut
)

func settlementFor(run runEnd, shuttingDown, crashed bool) settlement {
	switch {
	case run.canceled && !crashed:
		return settleCanceled
	case shuttingDown:
		return settleRelease
	case run.timedOut:
		return settleTimedOut
	}
	return settleFailed
}

func canceledOutcome(cause error, usage *Usage) outcome {
	return outcome{message: CanceledMessage, code: "agent_run_canceled", failure: FailureCanceled, cause: cause, usage: usage}
}

type runEnd struct {
	timedOut bool
	lost     bool
	canceled bool
}

func (w *Worker) runLeased(ctx context.Context, claimed dbsqlc.AgentRequest, req Request) (Result, runEnd, error) {
	runCtx, cancel := w.runContext(ctx)
	defer cancel()
	var lost, canceled atomic.Bool
	stopHeartbeat := w.holdLease(ctx, claimed, cancel, &lost, &canceled)
	result, err := w.revise(runCtx, req)
	stopHeartbeat()
	return result, runEnd{timedOut: errors.Is(runCtx.Err(), context.DeadlineExceeded), lost: lost.Load(), canceled: canceled.Load()}, err
}

func (w *Worker) runContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if w.opts.RunTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, w.opts.RunTimeout)
}

func (w *Worker) holdLease(ctx context.Context, claimed dbsqlc.AgentRequest, cancelRun context.CancelFunc, lost, canceled *atomic.Bool) (stop func()) {
	beatCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(w.heartbeat)
		defer ticker.Stop()
		renewedAt := time.Now()
		for {
			select {
			case <-beatCtx.Done():
				return
			case <-ticker.C:
			}
			renewCtx, cancelRenew := context.WithTimeout(beatCtx, w.service.leaseTTL)
			cancelRequested, err := w.service.renewLease(renewCtx, claimed)
			cancelRenew()
			switch {
			case err == nil:
				renewedAt = time.Now()
				if cancelRequested && canceled.CompareAndSwap(false, true) {
					cancelRun()
				}
			case beatCtx.Err() != nil:
				return
			case errors.Is(err, ErrLeaseLost), time.Since(renewedAt) > w.service.leaseTTL:
				lost.Store(true)
				cancelRun()
				return
			default:
				w.logger.WarnContext(ctx, "renew agent request lease; retrying at the next beat", logattrs.Failure("agent_lease_renew_failed", err, logAttrs(claimed)...)...)
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func successOutcome(source Source, result Result) outcome {
	switch source {
	case SourceSuggest:
		return suggestionOutcome(result)
	case SourceInvestigate:
		if result.FindingID == "" {
			return outcome{message: noFindingMessage, code: "agent_no_finding", failure: FailureNoFinding, usage: result.Usage}
		}
		return outcome{finding: true, usage: result.Usage}
	}
	return outcome{version: &result.Version, usage: result.Usage}
}

func suggestionOutcome(result Result) outcome {
	proposed := bytes.TrimSpace(result.Suggestion)
	if len(proposed) == 0 || proposed[0] != '{' || !json.Valid(proposed) {
		return outcome{message: noSuggestionMessage, code: "agent_no_suggestion", failure: FailureNoSuggestion, usage: result.Usage}
	}
	return outcome{suggestion: proposed, usage: result.Usage}
}

func (w *Worker) settleSaved(ctx context.Context, claimed dbsqlc.AgentRequest, usage *Usage) (settled, progressed bool) {
	checkCtx, cancel := finishContext(ctx)
	defer cancel()
	if claimed.Source == string(SourceInvestigate) {
		found, err := w.service.hasFinding(checkCtx, claimed)
		return w.settleChecked(ctx, claimed, err, found, outcome{finding: true, usage: usage})
	}
	version, saved, err := w.service.existingVersion(checkCtx, claimed)
	return w.settleChecked(ctx, claimed, err, saved, outcome{version: &version, usage: usage})
}

func (w *Worker) settleChecked(ctx context.Context, claimed dbsqlc.AgentRequest, err error, saved bool, success outcome) (settled, progressed bool) {
	switch {
	case err != nil:
		w.logger.ErrorContext(ctx, "check agent request result", logattrs.Failure("agent_request_check_failed", err, slog.String("agent_request_id", claimed.ID.String()))...)
		if int(claimed.CheckFailures)+1 >= maxCheckFailures {
			w.finish(ctx, claimed, outcome{message: checkFailedMessage, code: "agent_request_check_failed", failure: FailureCheckFailed, cause: err})
			return true, true
		}
		w.release(ctx, claimed, true)
		return true, false
	case saved:
		w.finish(ctx, claimed, success)
		return true, true
	}
	return false, true
}

func (w *Worker) revise(ctx context.Context, req Request) (result Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			w.logger.ErrorContext(ctx, "agent run panicked", slog.String("agent_request_id", req.ID), slog.String("panic_type", fmt.Sprintf("%T", p)))
			result, err = Result{}, errCrashed
		}
	}()
	ctx = origin.With(ctx, req.Origin)
	return w.service.reviser.Revise(ctx, req)
}

func finishContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
}

func (w *Worker) release(ctx context.Context, claimed dbsqlc.AgentRequest, checkFailed bool) {
	releaseCtx, cancel := finishContext(ctx)
	defer cancel()
	released, err := w.service.release(releaseCtx, claimed, checkFailed)
	attrs := logAttrs(claimed)
	switch {
	case err != nil:
		w.logger.ErrorContext(ctx, "release agent request; its lease will expire", logattrs.Failure("agent_request_release_failed", err, attrs...)...)
	case !released:
		w.logger.WarnContext(ctx, "agent request was finished elsewhere; not releasing it", attrs...)
	}
}

func (w *Worker) finish(ctx context.Context, claimed dbsqlc.AgentRequest, o outcome) {
	finishCtx, cancel := finishContext(ctx)
	defer cancel()
	finished, err := w.service.finish(finishCtx, claimed, o)
	attrs := logAttrs(claimed)
	switch {
	case err != nil:
		w.logger.ErrorContext(ctx, "finish agent request", logattrs.Failure("agent_request_finish_failed", err, attrs...)...)
	case !finished:
		w.logger.WarnContext(ctx, "agent request was finished elsewhere; dropping this result", attrs...)
	case o.suggestion != nil, o.finding:
		w.logger.InfoContext(ctx, "agent request succeeded", append(attrs, slog.Int("attempt", int(claimed.Attempts)))...)
	case o.version != nil:
		w.logger.InfoContext(ctx, "agent request succeeded", append(attrs, slog.Int("version", *o.version), slog.Int("attempt", int(claimed.Attempts)))...)
	default:
		w.logger.WarnContext(ctx, "agent request failed", logattrs.Failure(o.code, o.cause, append(attrs, slog.Int("attempt", int(claimed.Attempts)))...)...)
	}
}

func logAttrs(claimed dbsqlc.AgentRequest) []any {
	attrs := []any{slog.String("agent_request_id", claimed.ID.String())}
	if claimed.DashboardID.Valid {
		attrs = append(attrs, slog.String("dashboard_id", claimed.DashboardID.String()))
	}
	return attrs
}

func requestFrom(row dbsqlc.AgentRequest) Request {
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
