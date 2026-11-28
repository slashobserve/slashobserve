package jaeger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/otlphttp"
)

type traceResponse struct {
	Result struct {
		ResourceSpans []otlphttp.ResourceSpans `json:"resourceSpans"`
	} `json:"result"`
}

func (d *Datasource) Trace(ctx context.Context, id string, r datasource.TimeRange, maxSpans int) (datasource.Frame, error) {
	ctx, cancel := datasource.Bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	return d.trace(ctx, id, r, maxSpans)
}

func (d *Datasource) trace(ctx context.Context, id string, r datasource.TimeRange, maxSpans int) (datasource.Frame, error) {
	rows, payloadBytes, err := d.fetchSpans(ctx, id, r)
	if err != nil {
		return datasource.Frame{}, err
	}
	f := otlphttp.Frame(rows[:min(len(rows), maxSpans)])
	f.Meta = &datasource.FrameMeta{Bytes: int64(payloadBytes)}
	if len(rows) > maxSpans {
		f.Meta.Truncated, f.Meta.RowsTotal = true, len(rows)
	}
	return f, nil
}

func (d *Datasource) fetchSpans(ctx context.Context, id string, r datasource.TimeRange) ([]otlphttp.Row, int, error) {
	params := url.Values{
		"startTime": {r.From.UTC().Format(timeParamLayout)},
		"endTime":   {r.To.UTC().Format(timeParamLayout)},
	}
	path := "/api/v3/traces/" + id
	status, raw, err := d.http.Fetch(ctx, path, params)
	if err != nil {
		return nil, 0, err
	}
	if status == http.StatusNotFound && jsonBody(raw) {
		return nil, 0, traceNotFound()
	}
	if err := statusError(path, status, raw); err != nil {
		return nil, 0, err
	}
	var resp traceResponse
	if json.Unmarshal(raw, &resp) != nil {
		return nil, 0, datasource.Unreadable()
	}
	rows, err := otlphttp.Rows(resp.Result.ResourceSpans)
	if err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 {
		return nil, 0, traceNotFound()
	}
	return rows, len(raw), nil
}

func traceNotFound() error {
	return datasource.Errorf(datasource.ErrNotFound, "trace not found")
}
