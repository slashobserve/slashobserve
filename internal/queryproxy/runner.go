package queryproxy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type Kind string

const (
	KindRange   Kind = "range"
	KindInstant Kind = "instant"
)

const (
	maxUpstreamCalls     = 32
	queryParallelism     = 4
	defaultMaxDataPoints = 1000
	minMaxDataPoints     = 10
	maxMaxDataPoints     = 11000
	panelDeadline        = 55 * time.Second
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
	IncludeHidden bool

	skip []bool
}

type QueryResult struct {
	Ref    string
	Kind   Kind
	Step   time.Duration
	Frames []datasource.Frame
}

type Result struct {
	Queries []QueryResult
	Kind    Kind
	Step    time.Duration
	Range   datasource.TimeRange
}

func (r Result) Frames() []datasource.Frame {
	var frames []datasource.Frame
	for _, q := range r.Queries {
		frames = append(frames, q.Frames...)
	}
	return frames
}

type QueryError struct {
	Ref   string
	Index int
	Err   error
}

func (e *QueryError) Error() string { return e.Err.Error() }
func (e *QueryError) Unwrap() error { return e.Err }

type Runner struct {
	reg    *datasource.Registry
	logger *slog.Logger
	sem    *semaphore.Weighted
	values *valueCache

	panelDeadline time.Duration
}

type RunnerOption func(*Runner)

func WithValueCache() RunnerOption {
	return func(r *Runner) { r.values = newValueCache() }
}

func NewRunner(reg *datasource.Registry, logger *slog.Logger, opts ...RunnerOption) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Runner{reg: reg, logger: logger, sem: upstreamCalls, panelDeadline: panelDeadline}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Runner) AcquireUpstream(ctx context.Context) (release func(), err error) {
	if err := r.sem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	return func() { r.sem.Release(1) }, nil
}

func QueryKind(p *spec.Panel, q spec.Query) Kind {
	if q.Instant {
		return KindInstant
	}
	switch p.Type {
	case spec.Timeseries, spec.Heatmap, spec.StateTimeline, spec.Histogram:
		return KindRange
	}
	return KindInstant
}

func PanelKind(p *spec.Panel) Kind {
	for _, q := range p.Queries {
		if !q.Hide && QueryKind(p, q) == KindRange {
			return KindRange
		}
	}
	return KindInstant
}

func QueryStep(base time.Duration, p *spec.Panel, q spec.Query) time.Duration {
	text := cmp.Or(q.Interval, p.Interval)
	if text == "" {
		return base
	}
	minimum, err := spec.ParseInterval(text)
	if err != nil {
		return base
	}
	return max(base, minimum)
}

func Floor(t time.Time, unit time.Duration) time.Time {
	ms, u := t.UnixMilli(), unit.Milliseconds()
	return time.UnixMilli(ms - ((ms%u)+u)%u).UTC()
}

func alignToStep(r datasource.TimeRange, step time.Duration) datasource.TimeRange {
	from, to := Floor(r.From, step), Floor(r.To, step)
	if !to.After(from) {
		to = from.Add(step)
	}
	return datasource.TimeRange{From: from, To: to}
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
	ctx, cancel := context.WithTimeout(ctx, r.panelDeadline)
	defer cancel()
	_, res, errs, err := r.runQueries(ctx, req, map[string]allList{}, true)
	if err != nil {
		return Result{}, err
	}
	if i := slices.IndexFunc(errs, func(err error) bool { return err != nil }); i >= 0 {
		return Result{}, errs[i]
	}
	return res, nil
}

func (r *Runner) runQueries(ctx context.Context, req Request, lists map[string]allList, requireAll bool) (panel *spec.Panel, res Result, errs []error, err error) {
	panel, err = checkRequest(req, requireAll)
	if err != nil {
		return nil, Result{}, nil, err
	}

	baseStep := StepFor(req.Range, req.MaxDataPoints)
	errs = make([]error, len(panel.Queries))
	prepared := make([]*preparedQuery, len(panel.Queries))
	for j, q := range panel.Queries {
		if skipped(req, j, q) {
			continue
		}
		pq, err := r.prepare(ctx, req, lists, panel, j, baseStep)
		if err != nil {
			errs[j] = &QueryError{Ref: panel.QueryRef(j), Index: j, Err: err}
			continue
		}
		prepared[j] = pq
	}

	frames := make([][]datasource.Frame, len(panel.Queries))
	var group errgroup.Group
	group.SetLimit(queryParallelism)
	for j, pq := range prepared {
		if pq == nil {
			continue
		}
		group.Go(func() error {
			out, err := upstream(ctx, r, panel.ID, pq.inst, pq.call)
			if err != nil {
				errs[j] = &QueryError{Ref: panel.QueryRef(j), Index: j, Err: err}
				return nil
			}
			frames[j] = out
			return nil
		})
	}
	_ = group.Wait()

	res = Result{Kind: PanelKind(panel), Range: req.Range}
	if res.Kind == KindRange {
		res.Step = baseStep
	}
	for j, pq := range prepared {
		if pq == nil || errs[j] != nil || panel.Queries[j].Hide {
			continue
		}
		res.Queries = append(res.Queries, QueryResult{Ref: panel.QueryRef(j), Kind: pq.kind, Step: pq.step, Frames: frames[j]})
	}
	return panel, res, errs, nil
}

func checkRequest(req Request, requireAll bool) (*spec.Panel, error) {
	panel, ok := dataPanel(req.Dashboard, req.PanelID)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrPanelNotFound, req.PanelID)
	}
	if err := req.Range.Validate(); err != nil {
		return nil, invalidRangeError(err)
	}
	for name := range req.Selections {
		if variableIndex(req.Dashboard, name) < 0 {
			return nil, fmt.Errorf("%w: %q", ErrUnknownVariable, name)
		}
	}
	if requireAll {
		for _, v := range req.Dashboard.Variables {
			if _, ok := req.Selections[v.Name]; !ok {
				return nil, fmt.Errorf("%w: no selection for variable %q", ErrInvalidRequest, v.Name)
			}
		}
	}
	return panel, nil
}

type preparedQuery struct {
	inst datasource.Instance
	kind Kind
	step time.Duration
	call func(context.Context) ([]datasource.Frame, error)
}

func skipped(req Request, j int, q spec.Query) bool {
	return (q.Hide && !req.IncludeHidden) || (j < len(req.skip) && req.skip[j])
}

func (r *Runner) prepare(ctx context.Context, req Request, lists map[string]allList, panel *spec.Panel, j int, baseStep time.Duration) (*preparedQuery, error) {
	q := panel.Queries[j]
	inst, err := r.instance(panel.QueryDatasource(j))
	if err != nil {
		return nil, err
	}
	vars, err := r.variablesFor(ctx, req.Dashboard, len(req.Dashboard.Variables), q.Query, req.Range, req.Selections, lists)
	if err != nil {
		return nil, err
	}
	dq := datasource.Query{Text: q.Query, Variables: vars, Limits: req.Limits}
	pq := &preparedQuery{inst: inst, kind: QueryKind(panel, q)}
	if pq.kind == KindRange {
		pq.step = QueryStep(baseStep, panel, q)
		rng := req.Range
		if pq.step > baseStep {
			rng = alignToStep(rng, pq.step)
		}
		pq.call = func(ctx context.Context) ([]datasource.Frame, error) {
			return inst.Datasource.QueryRange(ctx, dq, rng, pq.step)
		}
	} else {
		pq.call = func(ctx context.Context) ([]datasource.Frame, error) {
			return inst.Datasource.QueryInstant(ctx, dq, req.Range)
		}
	}
	return pq, nil
}

func invalidRangeError(err error) error {
	var ue *datasource.UpstreamError
	if errors.As(err, &ue) {
		return fmt.Errorf("%w: %s", ErrInvalidRequest, ue.Message)
	}
	return fmt.Errorf("%w: %s", ErrInvalidRequest, err.Error())
}

func (r *Runner) instance(uid string) (datasource.Instance, error) {
	inst, ok := r.reg.Get(uid)
	if !ok {
		return datasource.Instance{}, fmt.Errorf("%w: %q", ErrUnknownDatasource, uid)
	}
	return inst, nil
}

func dataPanel(d *spec.Dashboard, id string) (*spec.Panel, bool) {
	if d == nil {
		return nil, false
	}
	entry, ok := d.FindDataPanel(id)
	if !ok {
		return nil, false
	}
	return entry.Panel, true
}

func upstream[T any](ctx context.Context, r *Runner, subject string, inst datasource.Instance, call func(context.Context) (T, error)) (out T, err error) {
	if err := r.sem.Acquire(ctx, 1); err != nil {
		return out, err
	}
	defer r.sem.Release(1)
	defer func() {
		if p := recover(); p != nil {
			r.logger.ErrorContext(ctx, "datasource call panicked",
				slog.String("subject", subject),
				slog.String("datasource", inst.UID),
				slog.String("panic_type", fmt.Sprintf("%T", p)),
				slog.String("stack", string(debug.Stack())))
			var zero T
			out, err = zero, errInternal
		}
	}()
	out, err = call(ctx)
	if err != nil {
		if status, code, _ := Classify(err); status == http.StatusInternalServerError {
			r.logger.ErrorContext(ctx, "datasource call failed", append([]any{
				slog.String("subject", subject),
				slog.String("datasource", inst.UID),
				slog.String("datasource_type", inst.Datasource.Type()),
				slog.String("error_code", code),
			}, logattrs.Safe(err)...)...)
		}
	}
	return out, err
}
