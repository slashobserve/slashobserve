package jaeger

import (
	"context"
	"net/url"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var _ datasource.DependencySource = (*Datasource)(nil)

type dependenciesResponse struct {
	Data []struct {
		Parent    string `json:"parent"`
		Child     string `json:"child"`
		CallCount int64  `json:"callCount"`
	} `json:"data"`
}

func (d *Datasource) Dependencies(ctx context.Context, r datasource.TimeRange) ([]datasource.ServiceDependency, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := datasource.Bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	params := url.Values{
		"endTs":    {strconv.FormatInt(r.To.UnixMilli(), 10)},
		"lookback": {strconv.FormatInt(r.To.Sub(r.From).Milliseconds(), 10)},
	}
	var resp dependenciesResponse
	if err := d.getJSON(ctx, "/api/dependencies", params, &resp); err != nil {
		return nil, err
	}
	out := make([]datasource.ServiceDependency, len(resp.Data))
	for i, e := range resp.Data {
		out[i] = datasource.ServiceDependency{Parent: e.Parent, Child: e.Child, Calls: e.CallCount}
	}
	return out, nil
}
