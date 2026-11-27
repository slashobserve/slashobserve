package tools

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func (r *Registry) SweepRange(ctx context.Context, uid, query string, rng datasource.TimeRange, points int) ([]datasource.Frame, error) {
	run, inRun := RunFrom(ctx)
	principal, _ := auth.FromContext(ctx)
	ctx = attributed(callAccess(ctx, principal, run, inRun), nameQueryRange, run, inRun)
	inst, err := r.deps.Datasources.For(ctx, uid)
	if err != nil {
		return nil, err
	}
	if res, ok := checkQueryText(query); !ok {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", errorText(res))
	}
	dash, selections, res, ok := adhocDashboard(inst, spec.Timeseries, instantInput{Datasource: uid, Query: query}, "")
	if !ok {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", errorText(res))
	}
	noteRange(ctx, rng)
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	out, err := r.deps.Runner.RunPanel(ctx, queryproxy.Request{
		Dashboard:     dash,
		PanelID:       adhocPanelID,
		Range:         rng,
		Selections:    selections,
		MaxDataPoints: points,
		Limits:        queryproxy.DraftLimits,
		Explore:       true,
		User:          queryproxy.UserFor(principal),
	})
	if err != nil {
		return nil, err
	}
	return out.Frames(), nil
}

func errorText(res Result) string {
	if body, ok := res.Content.(errorBody); ok {
		return body.Message
	}
	return "the query is not valid"
}
