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
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/slashobserve/slashobserve/internal/auth"
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
	upstreamWait         = 5 * time.Second
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
	Exemplars     bool
	User          datasource.QueryUser
	Admit         func(index int, q QueryResult) error // an *AbortError stops the other queries and fails the request with its Err

	skip []bool
}

func UserFor(p auth.Principal) datasource.QueryUser {
	return datasource.QueryUser{ID: p.ID, Login: cmp.Or(p.Email, p.Name), Email: p.Email, Name: p.Name}
}

const (
	ExemplarsOK          = "ok"
	ExemplarsTruncated   = "truncated"
	ExemplarsUnavailable = "unavailable"
	ExemplarsUnsupported = "unsupported"

	maxExemplars = 2000
)

type QueryResult struct {
	Ref            string
	Kind           Kind
	Step           time.Duration
	Frames         []datasource.Frame
	Exemplars      []datasource.Frame
	ExemplarStatus string
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

type AbortError struct{ Err error }

func (e *AbortError) Error() string { return e.Err.Error() }
func (e *AbortError) Unwrap() error { return e.Err }

type Runner struct {
	reg    *datasource.Registry
	logger *slog.Logger
	sem    *semaphore.Weighted
	values *valueCache

	panelDeadline time.Duration
	upstreamWait  time.Duration
}

type RunnerOption func(*Runner)

func WithValueCache() RunnerOption {
	return func(r *Runner) { r.values = newValueCache(r.logger) }
}

func WithUpstreamWait(d time.Duration) RunnerOption {
	return func(r *Runner) { r.upstreamWait = d }
}

func NewRunner(reg *datasource.Registry, logger *slog.Logger, opts ...RunnerOption) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Runner{reg: reg, logger: logger, sem: upstreamCalls, panelDeadline: panelDeadline, upstreamWait: upstreamWait}
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
	if spec.InstantByDefault(p.Type) {
		return KindInstant
	}
	return KindRange
}

func PanelKind(p *spec.Panel) Kind {
	for j, kind := range queryKinds(p) {
		if !p.Queries[j].Hide && kind == KindRange {
			return KindRange
		}
	}
	return KindInstant
}

func QueryStep(base, minimum time.Duration, factor int) time.Duration {
	return max(base, minimum) * time.Duration(max(1, factor))
}

func (r *Runner) minInterval(ctx context.Context, req Request, st *varState, panel *spec.Panel, q spec.Query) (time.Duration, error) {
	text := cmp.Or(q.Interval, panel.Interval)
	if text == "" {
		return 0, nil
	}
	if name, ok := spec.IntervalVariableName(text); ok {
		resolved, err := r.intervalSelection(ctx, req, st, name)
		if err != nil {
			return 0, err
		}
		text = resolved
	}
	minimum, err := spec.ParseInterval(text)
	if err != nil {
		return 0, fmt.Errorf("%w: min step: %s", ErrInvalidRequest, err.Error())
	}
	return minimum, nil
}

func (r *Runner) intervalSelection(ctx context.Context, req Request, st *varState, name string) (string, error) {
	idx := variableIndex(req.Dashboard, name)
	if idx < 0 || req.Dashboard.Variables[idx].Type != spec.IntervalVariable {
		return "", fmt.Errorf("%w: min step %q is not an interval variable", ErrInvalidRequest, name)
	}
	sel, ok := req.Selections[name]
	if !ok {
		return "", fmt.Errorf("%w: no selection for variable %q", ErrInvalidRequest, name)
	}
	if err := r.checkSelection(ctx, req.Dashboard, idx, req.Range, req.Selections, req.User, st, sel); err != nil {
		return "", err
	}
	variable, err := r.variableFor(ctx, req.Dashboard, idx, req.Range, queryUse{}, req.Selections, req.User, st)
	if err != nil {
		return "", err
	}
	if len(variable.Values) != 1 {
		return "", fmt.Errorf("%w: variable %q must select one step", ErrInvalidRequest, name)
	}
	return variable.Values[0], nil
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
	_, res, errs, err := r.runQueries(ctx, req, newVarState(), true)
	if err != nil {
		return Result{}, err
	}
	if i := slices.IndexFunc(errs, func(err error) bool { return err != nil }); i >= 0 {
		return Result{}, errs[i]
	}
	return res, nil
}

func (r *Runner) runQueries(ctx context.Context, req Request, st *varState, requireAll bool) (panel *spec.Panel, res Result, errs []error, err error) {
	panel, err = checkRequest(req, requireAll)
	if err != nil {
		return nil, Result{}, nil, err
	}

	baseStep := StepFor(req.Range, req.MaxDataPoints)
	wanted := wantedQueries(req, panel)
	errs = make([]error, len(panel.Queries))
	prepared := make([]*preparedQuery, len(panel.Queries))
	for j, q := range panel.Queries {
		if !wanted[j] || q.IsExpression() {
			continue
		}
		pq, err := r.prepare(ctx, req, st, panel, j, baseStep)
		if err != nil {
			errs[j] = &QueryError{Ref: panel.QueryRef(j), Index: j, Err: err}
			continue
		}
		prepared[j] = pq
	}

	outs, abortErr := r.runPrepared(ctx, req, panel, prepared, errs)
	if abortErr != nil {
		return panel, Result{}, nil, abortErr
	}

	results := make([]QueryResult, len(panel.Queries))
	have := make([]bool, len(panel.Queries))
	for j, pq := range prepared {
		if pq != nil && errs[j] == nil {
			results[j], have[j] = pq.result(panel.QueryRef(j), outs[j]), true
		}
	}
	if abortErr := evalExpressions(req, panel, wanted, results, have, errs); abortErr != nil {
		return panel, Result{}, nil, abortErr
	}

	res = Result{Kind: PanelKind(panel), Range: req.Range}
	if res.Kind == KindRange {
		res.Step = baseStep
	}
	for j := range panel.Queries {
		if have[j] && !panel.Queries[j].Hide {
			res.Queries = append(res.Queries, results[j])
		}
	}
	return panel, res, errs, nil
}

func (r *Runner) runPrepared(ctx context.Context, req Request, panel *spec.Panel, prepared []*preparedQuery, errs []error) (outs []fetched, abortErr error) {
	outs = make([]fetched, len(panel.Queries))
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	var (
		group   errgroup.Group
		abortMu sync.Mutex
	)
	group.SetLimit(queryParallelism)
	for j, pq := range prepared {
		if pq == nil {
			continue
		}
		group.Go(func() error {
			out, err := r.runQuery(runCtx, req, panel, j, pq)
			var abort *AbortError
			if errors.As(err, &abort) {
				abortMu.Lock()
				if abortErr == nil {
					abortErr = abort.Err
				}
				abortMu.Unlock()
				stop()
				return nil
			}
			if err != nil {
				errs[j] = &QueryError{Ref: panel.QueryRef(j), Index: j, Err: err}
				return nil
			}
			outs[j] = out
			return nil
		})
	}
	_ = group.Wait()
	return outs, abortErr
}

type fetched struct {
	frames    []datasource.Frame
	exemplars []datasource.Frame
	status    string
}

func (r *Runner) runQuery(ctx context.Context, req Request, panel *spec.Panel, j int, pq *preparedQuery) (fetched, error) {
	ref := panel.QueryRef(j)
	out, err := upstream(ctx, r, panel.ID, pq.inst, func(ctx context.Context) (fetched, error) {
		frames, err := pq.call(ctx)
		if err != nil {
			return fetched{}, err
		}
		exemplars, status, err := r.fetchExemplars(ctx, panel.ID, ref, pq)
		return fetched{frames: frames, exemplars: exemplars, status: status}, err
	})
	if err != nil {
		return fetched{}, err
	}
	if req.Admit != nil && !panel.Queries[j].Hide {
		if err := req.Admit(j, pq.result(ref, out)); err != nil {
			return fetched{}, err
		}
	}
	return out, nil
}

func (r *Runner) fetchExemplars(ctx context.Context, subject, ref string, pq *preparedQuery) ([]datasource.Frame, string, error) {
	if !pq.asksExemplars {
		return nil, "", nil
	}
	if pq.exemplars == nil {
		return nil, ExemplarsUnsupported, nil
	}
	frames, err := pq.exemplars(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", err
		}
		_, code, _ := Classify(err)
		r.logger.WarnContext(ctx, "exemplar fetch failed", append([]any{
			slog.String("subject", subject),
			slog.String("datasource", pq.inst.UID),
			slog.String("query_ref", ref),
		}, logattrs.Failure(code, err)...)...)
		return nil, ExemplarsUnavailable, nil
	}
	frames, truncated := capExemplars(thinExemplars(frames, pq.from, pq.step), maxExemplars)
	if truncated {
		return frames, ExemplarsTruncated, nil
	}
	return frames, ExemplarsOK, nil
}

func thinExemplars(frames []datasource.Frame, from time.Time, step time.Duration) []datasource.Frame {
	var out []datasource.Frame
	for _, f := range frames {
		if thinned, ok := thinExemplarFrame(f, from, step); ok {
			out = append(out, thinned)
		}
	}
	return out
}

func thinExemplarFrame(f datasource.Frame, from time.Time, step time.Duration) (datasource.Frame, bool) {
	if len(f.Fields) < 2 {
		return datasource.Frame{}, false
	}
	best := map[int64]int{}
	for i := range f.Fields[0].Values {
		at, ok := f.Fields[0].Values[i].(time.Time)
		if !ok {
			continue
		}
		bucket := int64(at.Sub(from) / step)
		if kept, seen := best[bucket]; !seen || higher(f.Fields[1].Values[i], f.Fields[1].Values[kept]) {
			best[bucket] = i
		}
	}
	rows := make([]int, 0, len(best))
	for _, i := range best {
		rows = append(rows, i)
	}
	slices.Sort(rows)
	return selectRows(f, rows), len(rows) > 0
}

func higher(a, b any) bool {
	x, aok := a.(float64)
	y, bok := b.(float64)
	return aok && (!bok || x > y)
}

func selectRows(f datasource.Frame, rows []int) datasource.Frame {
	out := datasource.Frame{Name: f.Name, Labels: f.Labels, Fields: make([]datasource.Field, len(f.Fields))}
	for k, field := range f.Fields {
		values := make([]any, len(rows))
		for n, i := range rows {
			values[n] = field.Values[i]
		}
		out.Fields[k] = datasource.Field{Name: field.Name, Type: field.Type, Values: values}
	}
	return out
}

func capExemplars(frames []datasource.Frame, limit int) ([]datasource.Frame, bool) {
	type row struct {
		frame, index int
		at           time.Time
	}
	var all []row
	for fi, f := range frames {
		for i, v := range f.Fields[0].Values {
			all = append(all, row{fi, i, v.(time.Time)})
		}
	}
	if len(all) <= limit {
		return frames, false
	}
	slices.SortStableFunc(all, func(a, b row) int { return b.at.Compare(a.at) })
	keep := make([][]int, len(frames))
	for _, r := range all[:limit] {
		keep[r.frame] = append(keep[r.frame], r.index)
	}
	var out []datasource.Frame
	for fi, rows := range keep {
		if len(rows) == 0 {
			continue
		}
		slices.Sort(rows)
		out = append(out, selectRows(frames[fi], rows))
	}
	return out, true
}

func checkRequest(req Request, requireAll bool) (*spec.Panel, error) {
	panel, ok := dataPanel(req.Dashboard, req.PanelID)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrPanelNotFound, req.PanelID)
	}
	if _, reuses := panel.DashboardSource(); reuses {
		return nil, fmt.Errorf("%w: panel %q reuses another panel's results; run that panel", ErrInvalidRequest, req.PanelID)
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
	from time.Time
	call func(context.Context) ([]datasource.Frame, error)

	asksExemplars bool
	exemplars     func(context.Context) ([]datasource.Frame, error)
}

func (pq *preparedQuery) result(ref string, out fetched) QueryResult {
	return QueryResult{Ref: ref, Kind: pq.kind, Step: pq.step, Frames: out.frames, Exemplars: out.exemplars, ExemplarStatus: out.status}
}

func (r *Runner) prepare(ctx context.Context, req Request, st *varState, panel *spec.Panel, j int, baseStep time.Duration) (*preparedQuery, error) {
	q := panel.Queries[j]
	inst, err := r.selectedDatasource(ctx, req.Dashboard, panel.QueryDatasource(j), req.Range, req.Selections, req.User, st)
	if err != nil {
		return nil, err
	}
	use := scanQueryUse(q.Query)
	vars, err := r.variablesFor(ctx, req.Dashboard, len(req.Dashboard.Variables), use, req.Range, req.Selections, req.User, st)
	if err != nil {
		return nil, err
	}
	dq := datasource.Query{Text: q.Query, Format: q.Format, Variables: vars, Limits: req.Limits, Dashboard: req.Dashboard.Title, User: req.User}
	if applies := implicitScope(inst.Datasource.Type(), use); applies != nil {
		if dq.Filters, err = r.implicitFilters(st, req.Dashboard, req.Selections, inst.UID, applies); err != nil {
			return nil, err
		}
	}
	minimum, err := r.minInterval(ctx, req, st, panel, q)
	if err != nil {
		return nil, err
	}
	pq := &preparedQuery{inst: inst, kind: QueryKind(panel, q), asksExemplars: req.Exemplars && q.Exemplar}
	if pq.kind == KindRange {
		pq.step = QueryStep(baseStep, minimum, q.IntervalFactor)
		rng := req.Range
		if pq.step > baseStep {
			rng = alignToStep(rng, pq.step)
		}
		pq.from = rng.From
		pq.call = func(ctx context.Context) ([]datasource.Frame, error) {
			return inst.Datasource.QueryRange(ctx, dq, rng, pq.step)
		}
		if src, ok := inst.Datasource.(datasource.ExemplarSource); ok {
			pq.exemplars = func(ctx context.Context) ([]datasource.Frame, error) {
				return src.QueryExemplars(ctx, dq, rng, pq.step)
			}
		}
	} else {
		dq.MinInterval = minimum
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
