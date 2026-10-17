package queryproxy

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type Kind string

const (
	KindRange   Kind = "range"
	KindInstant Kind = "instant"
)

const (
	maxUpstreamCalls     = 32
	defaultMaxDataPoints = 1000
	minMaxDataPoints     = 10
	maxMaxDataPoints     = 11000
)

var upstreamCalls = semaphore.NewWeighted(maxUpstreamCalls)

type Selection struct {
	Values []string
	All    bool
}

type Request struct {
	Dashboard     *spec.Dashboard
	PanelID       string
	Range         datasource.TimeRange
	Selections    map[string]Selection
	MaxDataPoints int
	Limits        datasource.Limits
}

type Result struct {
	Frames []datasource.Frame
	Kind   Kind
	Step   time.Duration
	Range  datasource.TimeRange
}

type Runner struct {
	reg    *datasource.Registry
	logger *slog.Logger
}

func NewRunner(reg *datasource.Registry, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{reg: reg, logger: logger}
}

func KindFor(t spec.PanelType) Kind {
	if t == spec.Timeseries {
		return KindRange
	}
	return KindInstant
}

func StepFor(r datasource.TimeRange, maxDataPoints int) time.Duration {
	if maxDataPoints == 0 {
		maxDataPoints = defaultMaxDataPoints
	}
	maxDataPoints = min(max(maxDataPoints, minMaxDataPoints), maxMaxDataPoints)
	span := r.To.Sub(r.From)
	if span <= 0 {
		return time.Second
	}
	raw := (span + time.Duration(maxDataPoints) - 1) / time.Duration(maxDataPoints)
	whole := (raw + time.Second - 1) / time.Second * time.Second
	return max(whole, time.Second)
}

func (r *Runner) RunPanel(ctx context.Context, req Request) (Result, error) {
	panel, ok := findPanel(req.Dashboard, req.PanelID)
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrPanelNotFound, req.PanelID)
	}
	inst, err := r.instance(panel.Datasource)
	if err != nil {
		return Result{}, err
	}
	if err := req.Range.Validate(); err != nil {
		return Result{}, err
	}
	vars, err := selectionVariables(req.Dashboard.Variables, req.Selections)
	if err != nil {
		return Result{}, err
	}
	q := datasource.Query{Text: panel.Query, Variables: vars, Limits: req.Limits}
	kind := KindFor(panel.Type)
	res := Result{Kind: kind, Range: req.Range}
	if kind == KindRange {
		res.Step = StepFor(req.Range, req.MaxDataPoints)
	}
	res.Frames, err = upstream(ctx, r, panel.ID, inst.UID, func(ctx context.Context) ([]datasource.Frame, error) {
		if kind == KindRange {
			return inst.Datasource.QueryRange(ctx, q, req.Range, res.Step)
		}
		return inst.Datasource.QueryInstant(ctx, q, req.Range)
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

func (r *Runner) instance(ref spec.DatasourceRef) (datasource.Instance, error) {
	inst, ok := r.reg.Get(ref.UID)
	if !ok {
		return datasource.Instance{}, fmt.Errorf("%w: %q", ErrUnknownDatasource, ref.UID)
	}
	if got := inst.Datasource.Type(); got != string(ref.Type) {
		return datasource.Instance{}, fmt.Errorf("%w: datasource %q is %s, not %s", ErrDatasourceTypeMismatch, ref.UID, got, ref.Type)
	}
	return inst, nil
}

func findPanel(d *spec.Dashboard, id string) (spec.Panel, bool) {
	if d == nil {
		return spec.Panel{}, false
	}
	for _, p := range d.Panels {
		if p.ID == id {
			return p, true
		}
	}
	return spec.Panel{}, false
}

func upstream[T any](ctx context.Context, r *Runner, subject, uid string, call func(context.Context) (T, error)) (out T, err error) {
	if err := upstreamCalls.Acquire(ctx, 1); err != nil {
		return out, err
	}
	defer upstreamCalls.Release(1)
	defer func() {
		if p := recover(); p != nil {
			r.logger.ErrorContext(ctx, "datasource call panicked",
				slog.String("subject", subject),
				slog.String("datasource", uid),
				slog.String("panic_type", fmt.Sprintf("%T", p)),
				slog.String("stack", string(debug.Stack())))
			var zero T
			out, err = zero, errInternal
		}
	}()
	return call(ctx)
}
