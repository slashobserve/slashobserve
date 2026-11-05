package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	OutcomeSucceeded   = "succeeded"
	OutcomeFailed      = "failed"
	OutcomeInterrupted = "interrupted"

	HeartbeatJob = "scheduler_heartbeat"
)

const (
	tick           = 15 * time.Second
	maxLease       = 10 * time.Minute
	recordTimeout  = 5 * time.Second
	heartbeatEvery = 5 * time.Minute
)

var (
	ErrUnknownJob = errors.New("no job is registered under that name")
	ErrJobLeased  = errors.New("the job is running under another lease")
)

type job struct {
	name  string
	every time.Duration
	run   func(context.Context) error
}

type Scheduler struct {
	logger  *slog.Logger
	store   *store
	metrics Metrics
	jobs    map[string]job
}

func New(logger *slog.Logger, database *db.DB, metrics Metrics, instanceID string) *Scheduler {
	s := &Scheduler{
		logger:  logger,
		store:   &store{db: database, instanceID: instanceID},
		metrics: metrics,
		jobs:    map[string]job{},
	}
	s.Register(HeartbeatJob, heartbeatEvery, func(context.Context) error { return nil })
	return s
}

func (s *Scheduler) Register(name string, every time.Duration, fn func(context.Context) error) {
	if every < time.Second || every%time.Second != 0 {
		panic("scheduler: job " + name + " must run every whole number of seconds")
	}
	if _, taken := s.jobs[name]; taken {
		panic("scheduler: job " + name + " is registered twice")
	}
	s.jobs[name] = job{name: name, every: every, run: fn}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	synced := false
	for {
		if !synced {
			synced = s.sync(ctx)
		}
		if synced {
			s.runDue(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) RunNow(ctx context.Context, name string) error {
	j, ok := s.jobs[name]
	if !ok {
		return ErrUnknownJob
	}
	if err := s.store.upsert(ctx, j); err != nil {
		return err
	}
	c, err := s.store.claim(ctx, []string{name}, true)
	if errors.Is(err, errNothingToClaim) {
		return ErrJobLeased
	}
	if err != nil {
		return err
	}
	s.run(ctx, j, c)
	return nil
}

func (s *Scheduler) names() []string {
	names := make([]string, 0, len(s.jobs))
	for name := range s.jobs {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (s *Scheduler) sync(ctx context.Context) bool {
	for _, name := range s.names() {
		if err := s.store.upsert(ctx, s.jobs[name]); err != nil {
			if ctx.Err() == nil {
				s.logger.WarnContext(ctx, "register scheduled job", logattrs.Failure("job_register_failed", err, slog.String("job", name))...)
			}
			return false
		}
	}
	return true
}

func (s *Scheduler) runDue(ctx context.Context) {
	names := s.names()
	for ctx.Err() == nil {
		c, err := s.store.claim(ctx, names, false)
		if errors.Is(err, errNothingToClaim) {
			return
		}
		if err != nil {
			if ctx.Err() == nil {
				s.logger.WarnContext(ctx, "claim scheduled job", logattrs.Failure("job_claim_failed", err)...)
			}
			return
		}
		s.run(ctx, s.jobs[c.name], c)
	}
}

func (s *Scheduler) run(ctx context.Context, j job, c claim) {
	s.metrics.JobStarted(j.name)
	started := time.Now()
	runCtx, cancel := context.WithDeadline(ctx, c.deadline)
	err := call(runCtx, j.run)
	cancel()
	outcome := outcomeOf(ctx, err)

	recordCtx, cancelRecord := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancelRecord()
	var held bool
	var recordErr error
	if outcome == OutcomeInterrupted {
		held, recordErr = s.store.release(recordCtx, c)
	} else {
		held, recordErr = s.store.finish(recordCtx, c, outcome, summary(err))
	}
	s.metrics.JobFinished(j.name, outcome)

	attrs := []any{slog.String("job", j.name), slog.String("outcome", outcome), slog.Int64("duration_ms", time.Since(started).Milliseconds())}
	switch {
	case recordErr != nil:
		s.logger.WarnContext(ctx, "record scheduled job outcome", logattrs.Failure("job_record_failed", recordErr, attrs...)...)
	case !held:
		s.logger.WarnContext(ctx, "scheduled job lost its lease", attrs...)
	}
	switch outcome {
	case OutcomeFailed:
		s.logger.ErrorContext(ctx, "scheduled job failed", logattrs.Failure("job_failed", err, append(attrs, panicAttrs(err)...)...)...)
	case OutcomeInterrupted:
		s.logger.InfoContext(ctx, "scheduled job interrupted", attrs...)
	default:
		s.logger.DebugContext(ctx, "scheduled job finished", attrs...)
	}
}

func outcomeOf(ctx context.Context, err error) string {
	switch {
	case err == nil:
		return OutcomeSucceeded
	case ctx.Err() != nil:
		return OutcomeInterrupted
	}
	return OutcomeFailed
}

type panicError struct {
	valueType string
}

func (e panicError) Error() string { return "scheduled job panicked with a " + e.valueType }

func call(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = panicError{valueType: fmt.Sprintf("%T", p)}
		}
	}()
	return fn(ctx)
}

func panicAttrs(err error) []any {
	var p panicError
	if errors.As(err, &p) {
		return []any{slog.String("panic_type", p.valueType)}
	}
	return nil
}

func summary(err error) string {
	if err == nil {
		return ""
	}
	var parts []string
	for _, a := range append(logattrs.Safe(err), panicAttrs(err)...) {
		if attr, ok := a.(slog.Attr); ok {
			parts = append(parts, attr.Key+"="+attr.Value.String())
		}
	}
	return strings.Join(parts, " ")
}
