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

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
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

type WorkerMetrics interface {
	RequestClaimed(source string, waited time.Duration)
}

type noWorkerMetrics struct{}

func (noWorkerMetrics) RequestClaimed(string, time.Duration) {}

type WorkerOptions struct {
	InstanceID string
	Workers    int
	Poll       time.Duration
	RunTimeout time.Duration
	Metrics    WorkerMetrics
}

type Worker struct {
	service  *Service
	listen   events.Listener
	logger   *slog.Logger
	opts     WorkerOptions
	claimant Claimant

	heartbeat time.Duration
}

func NewWorker(service *Service, listen events.Listener, logger *slog.Logger, opts WorkerOptions) *Worker {
	if service.principals == nil {
		panic("revisions: NewWorker needs a Service built with WithPrincipals")
	}
	if service.access == nil {
		panic("revisions: NewWorker needs a Service built with WithAccess")
	}
	if opts.Workers < 1 {
		panic("revisions: NewWorker needs at least one worker")
	}
	if opts.Poll <= 0 {
		opts.Poll = defaultPoll
	}
	if opts.Metrics == nil {
		opts.Metrics = noWorkerMetrics{}
	}
	return &Worker{service: service, listen: listen, logger: logger, opts: opts, heartbeat: heartbeatEvery, claimant: Claimant{Kind: AssignBuiltin, ID: opts.InstanceID}}
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
	claim, found, err := w.service.Claim(ctx, w.claimant)
	if err != nil || !found {
		return false, err
	}
	row := claim.Row
	if claim.FirstClaim && row.StartedAt.Valid && row.CreatedAt.Valid {
		w.opts.Metrics.RequestClaimed(row.Source, row.StartedAt.Time.Sub(row.CreatedAt.Time))
	}
	return w.process(ctx, claim), nil
}

func (w *Worker) process(ctx context.Context, claim Claimed) bool {
	claimed := claim.Row
	result, run, err := w.runLeased(ctx, claimed, claim.Request)
	if run.lost || errors.Is(err, ErrLeaseLost) {
		w.logger.WarnContext(ctx, "agent request lease lost; dropping this attempt's work", logattrs.Failure("agent_request_lease_lost", ErrLeaseLost, append(logAttrs(claimed), slog.Int("attempt", int(claimed.Attempts)))...)...)
		return false
	}
	if err == nil {
		w.service.finishLogged(ctx, claimed, successOutcome(Source(claimed.Source), result))
		return true
	}
	return w.settleFailure(ctx, claimed, run, result, err)
}

func (w *Worker) settleFailure(ctx context.Context, claimed dbsqlc.AgentRequest, run runEnd, result Result, err error) bool {
	if claimed.Source != string(SourceSuggest) {
		if settled, progressed := w.service.settleSaved(ctx, claimed, result.Usage); settled {
			return progressed
		}
	}
	crashed := errors.Is(err, errCrashed)
	switch settlementFor(run, ctx.Err() != nil, crashed) {
	case settleCanceled:
		w.service.finishLogged(ctx, claimed, canceledOutcome(err, result.Usage))
	case settleRelease:
		w.logger.InfoContext(ctx, "agent request interrupted; releasing it", slog.String("agent_request_id", claimed.ID.String()))
		w.service.releaseLogged(ctx, claimed, false)
		return false
	case settleTimedOut:
		code, failure := "agent_run_timed_out", FailureRunTimeout
		if crashed {
			code, failure = "agent_run_panicked", FailureCrashed
		}
		w.service.finishLogged(ctx, claimed, outcome{message: timedOutMessage, code: code, failure: failure, cause: err, usage: result.Usage})
	default:
		code, failure := "agent_run_failed", result.FailureCode
		if crashed {
			code, failure = "agent_run_panicked", FailureCrashed
		}
		if failure == "" {
			failure = FailureInternal
		}
		w.service.finishLogged(ctx, claimed, outcome{message: err.Error(), code: code, failure: failure, cause: err, usage: result.Usage})
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
			cancelRequested, err := w.service.RenewLease(renewCtx, claimed)
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
	switch {
	case source == SourceSuggest:
		return suggestionOutcome(result)
	case source == SourceInvestigate && result.FindingID == "":
		return outcome{message: noFindingMessage, code: "agent_no_finding", failure: FailureNoFinding, usage: result.Usage}
	case result.FindingID != "":
		return outcome{finding: true, usage: result.Usage}
	case result.ProposalID != "":
		return outcome{proposal: true, usage: result.Usage}
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
