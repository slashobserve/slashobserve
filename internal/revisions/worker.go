package revisions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
)

const (
	defaultWorkers    = 2
	defaultPoll       = 5 * time.Second
	defaultRunTimeout = 3 * time.Minute
	finishTimeout     = 15 * time.Second
	listenBackoff     = time.Second
	maxListenBackoff  = 30 * time.Second

	maxCheckFailures   = 3
	checkFailedMessage = "could not check the request's saved version"
	timedOutMessage    = "the agent run timed out"
	crashedMessage     = "the agent run crashed"
)

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
}

func NewWorker(service *Service, listen events.Listener, logger *slog.Logger, opts WorkerOptions) *Worker {
	if opts.Workers <= 0 {
		opts.Workers = defaultWorkers
	}
	if opts.Poll <= 0 {
		opts.Poll = defaultPoll
	}
	if opts.RunTimeout <= 0 {
		opts.RunTimeout = defaultRunTimeout
	}
	return &Worker{service: service, listen: listen, logger: logger, opts: opts}
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
		w.logger.Warn("agent request listener failed", slog.Any("error", err), slog.Duration("retry_in", backoff))
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
			w.logger.Error("agent request worker step failed", slog.Any("error", err))
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
	if settled, progressed := w.settleSaved(ctx, claimed, true, nil); settled {
		return progressed
	}

	runCtx, cancel := context.WithTimeout(ctx, w.opts.RunTimeout)
	result, err := w.revise(runCtx, requestFrom(claimed))
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	cancel()

	if err == nil {
		w.finish(ctx, claimed, outcome{version: &result.Version, usage: result.Usage})
		return true
	}
	if settled, progressed := w.settleSaved(ctx, claimed, false, result.Usage); settled {
		return progressed
	}
	switch {
	case ctx.Err() != nil:
		w.logger.InfoContext(ctx, "agent request interrupted; releasing it", slog.String("agent_request_id", claimed.ID.String()))
		w.release(ctx, claimed, false)
		return false
	case timedOut:
		w.finish(ctx, claimed, outcome{message: timedOutMessage, usage: result.Usage})
	default:
		w.finish(ctx, claimed, outcome{message: err.Error(), usage: result.Usage})
	}
	return true
}

func (w *Worker) settleSaved(ctx context.Context, claimed dbsqlc.AgentRequest, beforeRun bool, usage *Usage) (settled, progressed bool) {
	checkCtx, cancel := finishContext(ctx)
	defer cancel()
	version, saved, err := w.service.existingVersion(checkCtx, claimed)
	switch {
	case err != nil:
		w.logger.ErrorContext(ctx, "check agent request result", slog.String("agent_request_id", claimed.ID.String()), slog.Any("error", err))
		if !beforeRun {
			return false, true
		}
		if int(claimed.CheckFailures)+1 >= maxCheckFailures {
			w.finish(ctx, claimed, outcome{message: checkFailedMessage})
			return true, true
		}
		w.release(ctx, claimed, true)
		return true, false
	case saved:
		w.finish(ctx, claimed, outcome{version: &version, usage: usage})
		return true, true
	}
	return false, true
}

func (w *Worker) revise(ctx context.Context, req Request) (result Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			w.logger.ErrorContext(ctx, "agent run panicked", slog.String("agent_request_id", req.ID), slog.String("panic_type", fmt.Sprintf("%T", p)))
			result, err = Result{}, errors.New(crashedMessage)
		}
	}()
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
		w.logger.ErrorContext(ctx, "release agent request; its lease will expire", append(attrs, slog.Any("error", err))...)
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
		w.logger.ErrorContext(ctx, "finish agent request", append(attrs, slog.Any("error", err))...)
	case !finished:
		w.logger.WarnContext(ctx, "agent request was finished elsewhere; dropping this result", attrs...)
	case o.version != nil:
		w.logger.InfoContext(ctx, "agent request succeeded", append(attrs, slog.Int("version", *o.version), slog.Int("attempt", int(claimed.Attempts)))...)
	default:
		w.logger.WarnContext(ctx, "agent request failed", append(attrs, slog.String("error", truncateError(o.message)), slog.Int("attempt", int(claimed.Attempts)))...)
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
		ID:          row.ID.String(),
		DashboardID: row.DashboardID.String(),
		Source:      Source(row.Source),
		Prompt:      row.Prompt,
		PanelID:     row.PanelID.String,
		BaseVersion: int(row.BaseVersion.Int32),
		Attempt:     int(row.Attempts),
		RequestedBy: requesterOf(row),
	}
}
