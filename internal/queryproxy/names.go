package queryproxy

import (
	"context"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	DefaultNamesLimit = 100
	MaxNamesLimit     = 500

	maxNamesSearchBytes = 200
	maxNamesFieldBytes  = 256
	maxHelpBytes        = 160
	namesTimeout        = 10 * time.Second
	clickHouseType      = "clickhouse"
)

type NamesRequest struct {
	Datasource string
	Kind       datasource.DiscoverKind
	Search     string
	Metric     string
	Label      string
	Table      string
	Scope      string
	Inventory  bool
	Limit      int // 0 means DefaultNamesLimit
}

type Names struct {
	Datasource      string
	Label           string
	Metrics         []datasource.Metric
	Labels          []string
	Values          []string
	Families        []datasource.Family
	FamiliesTotal   int
	RuntimeFamilies int
	Truncated       bool
}

type invalidNames struct{ message string }

func (e *invalidNames) Error() string        { return e.message }
func (e *invalidNames) Is(target error) bool { return target == ErrInvalidRequest }

func invalidNamesf(format string, args ...any) error {
	return &invalidNames{message: fmt.Sprintf(format, args...)}
}

func (req NamesRequest) validate() (limit int, err error) {
	limit = req.Limit
	if limit == 0 {
		limit = DefaultNamesLimit
	}
	if limit < 1 || limit > MaxNamesLimit {
		return 0, invalidNamesf("limit must be between 1 and %d", MaxNamesLimit)
	}
	for _, f := range []struct {
		field, value string
		max          int
	}{
		{"search", req.Search, maxNamesSearchBytes},
		{"metric", req.Metric, maxNamesFieldBytes},
		{"label", req.Label, maxNamesFieldBytes},
		{"table", req.Table, maxNamesFieldBytes},
		{"scope", req.Scope, maxNamesFieldBytes},
	} {
		if len(f.value) > f.max {
			return 0, invalidNamesf("%s must be at most %d bytes", f.field, f.max)
		}
	}
	if req.Kind == datasource.DiscoverLabelValues && req.Label == "" {
		return 0, invalidNamesf("label is required")
	}
	return limit, nil
}

func (req NamesRequest) kind() datasource.DiscoverKind {
	if req.Scope != "" || req.Inventory {
		return datasource.DiscoverInventory
	}
	return req.Kind
}

func (r *Runner) Names(ctx context.Context, req NamesRequest) (Names, error) {
	limit, err := req.validate()
	if err != nil {
		return Names{}, err
	}
	inst, err := r.instance(req.Datasource)
	if err != nil {
		return Names{}, err
	}
	needsTable := req.Kind == datasource.DiscoverLabels || req.Kind == datasource.DiscoverLabelValues
	if needsTable && inst.Datasource.Type() == clickHouseType && req.Table == "" && req.Metric == "" {
		return Names{}, invalidNamesf("table or metric is required for ClickHouse labels")
	}
	release, err := r.acquireUpstreamBounded(ctx)
	if err != nil {
		return Names{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, namesTimeout)
	defer cancel()
	cat, err := inst.Datasource.Discover(ctx, datasource.DiscoverRequest{
		Kind:   req.kind(),
		Search: req.Search,
		Metric: req.Metric,
		Label:  req.Label,
		Table:  req.Table,
		Scope:  req.Scope,
		Limit:  limit + 1,
	})
	if err != nil {
		return Names{}, err
	}
	if cat == nil {
		cat = &datasource.Catalog{}
	}
	return newNames(req, cat, limit), nil
}

func (r *Runner) acquireUpstreamBounded(ctx context.Context) (release func(), err error) {
	waitCtx, cancel := context.WithTimeout(ctx, r.upstreamWait)
	defer cancel()
	release, err = r.AcquireUpstream(waitCtx)
	switch {
	case err == nil:
		return release, nil
	case ctx.Err() != nil:
		return nil, ctx.Err()
	}
	return nil, ErrUpstreamBusy
}

func newNames(req NamesRequest, cat *datasource.Catalog, limit int) Names {
	out := Names{
		Datasource: req.Datasource,
		Label:      req.Label,
		Metrics:    make([]datasource.Metric, 0, len(cat.Metrics)),
		Labels:     append([]string{}, cat.Labels...),
		Values:     append([]string{}, cat.Values...),
		Families:   make([]datasource.Family, 0, len(cat.Families)),
		Truncated:  cat.Truncated,
	}
	for _, m := range cat.Metrics {
		m.Help = ClipBytes(m.Help, maxHelpBytes)
		out.Metrics = append(out.Metrics, m)
	}
	for _, f := range cat.Families {
		f.Help = ClipBytes(f.Help, maxHelpBytes)
		out.Families = append(out.Families, f)
	}
	out.FamiliesTotal = max(cat.FamiliesTotal, len(out.Families))
	out.RuntimeFamilies = cat.RuntimeFamilies
	out.Metrics, out.Truncated = truncate(out.Metrics, limit, out.Truncated)
	out.Labels, out.Truncated = truncate(out.Labels, limit, out.Truncated)
	out.Values, out.Truncated = truncate(out.Values, limit, out.Truncated)
	return out
}

func truncate[T any](items []T, limit int, already bool) ([]T, bool) {
	if len(items) <= limit {
		return items, already
	}
	return items[:limit], true
}

func ClipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + "..."
}
