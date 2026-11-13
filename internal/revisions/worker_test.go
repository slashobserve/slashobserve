package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	defaultPoll      = 5 * time.Second
	listenBackoff    = time.Second
	maxListenBackoff = 30 * time.Second
)

type Result struct {
	Version     int
	Suggestion  json.RawMessage
	FindingID   string
	ProposalID  string
	Usage       *Usage
	FailureCode FailureCode
}

type Reviser interface {
	Revise(ctx context.Context, req Request) (Result, error)
}

type WorkerOptions struct {
	Reviser Reviser
	Workers int
	Poll    time.Duration
}

type Worker struct {
	service  *Service
	listen   events.Listener
	logger   *slog.Logger
	opts     WorkerOptions
	claimant Claimant
}

func NewWorker(service *Service, listen events.Listener, logger *slog.Logger, opts WorkerOptions) *Worker {
	if opts.Workers < 1 {
		opts.Workers = 1
	}
	if opts.Poll <= 0 {
		opts.Poll = defaultPoll
	}
	return &Worker{service: service, listen: listen, logger: logger, opts: opts, claimant: Claimant{Kind: AssignBuiltin}}
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
	return w.process(ctx, claim), nil
}

func (w *Worker) process(ctx context.Context, claim Claimed) bool {
	claimed := claim.Row
	result, err := w.opts.Reviser.Revise(origin.With(ctx, claim.Request.Origin), claim.Request)
	if errors.Is(err, ErrLeaseLost) {
		return false
	}
	if err == nil {
		w.service.finishLogged(ctx, claimed, successOutcome(Source(claimed.Source), result))
		return true
	}
	return w.settleFailure(ctx, claimed, result, err)
}

func (w *Worker) settleFailure(ctx context.Context, claimed dbsqlc.AgentRequest, result Result, err error) bool {
	if claimed.Source != string(SourceSuggest) {
		if settled, progressed := w.service.settleSaved(ctx, claimed, result.Usage); settled {
			return progressed
		}
	}
	if ctx.Err() != nil {
		w.service.releaseLogged(ctx, claimed, false)
		return false
	}
	failure := result.FailureCode
	if failure == "" {
		failure = FailureInternal
	}
	w.service.finishLogged(ctx, claimed, outcome{message: err.Error(), code: "agent_run_failed", failure: failure, cause: err, usage: result.Usage})
	return true
}

func successOutcome(source Source, result Result) outcome {
	switch {
	case source == SourceSuggest:
		return suggestionOutcome(result.Suggestion, result.Usage)
	case source == SourceInvestigate && result.FindingID == "":
		return outcome{message: noFindingMessage, code: "agent_no_finding", failure: FailureNoFinding, usage: result.Usage}
	case result.FindingID != "":
		return outcome{finding: true, usage: result.Usage}
	case result.ProposalID != "":
		return outcome{proposal: true, usage: result.Usage}
	}
	return outcome{version: &result.Version, usage: result.Usage}
}
