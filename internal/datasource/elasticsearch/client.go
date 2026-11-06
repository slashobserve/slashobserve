package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type timeoutKey struct{}

func bound(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	reported := timeout
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= timeout {
		reported = 0
	}
	ctx = context.WithValue(ctx, timeoutKey{}, reported)
	return context.WithTimeout(ctx, timeout)
}

func transportError(ctx context.Context, err error) error {
	reported, _ := ctx.Value(timeoutKey{}).(time.Duration)
	if cerr := datasource.ContextError(ctx, err, reported); cerr != nil {
		return cerr
	}
	return datasource.Errorf(datasource.ErrUnavailable, "connection failed")
}

func (d *Datasource) fetch(ctx context.Context, method, path string, params url.Values, body any) (int, []byte, error) {
	target := d.prefix + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, datasource.Errorf(datasource.ErrInvalidQuery, "request cannot be encoded")
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, nil, datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range d.opts.Headers {
		req.Header.Set(name, value)
	}
	switch {
	case d.opts.BasicAuthUser != "":
		req.SetBasicAuth(d.opts.BasicAuthUser, d.opts.BasicAuthPassword.Reveal())
	case !d.opts.BearerToken.IsZero():
		req.Header.Set("Authorization", "Bearer "+d.opts.BearerToken.Reveal())
	case !d.opts.APIKey.IsZero():
		req.Header.Set("Authorization", "ApiKey "+d.opts.APIKey.Reveal())
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, nil, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, d.opts.MaxResponseBytes+1))
	if err != nil {
		return 0, nil, transportError(ctx, err)
	}
	if int64(len(raw)) > d.opts.MaxResponseBytes {
		return 0, nil, datasource.Errorf(datasource.ErrLimitExceeded, "response larger than %d bytes", d.opts.MaxResponseBytes)
	}
	return resp.StatusCode, raw, nil
}

func (d *Datasource) call(ctx context.Context, method, path string, params url.Values, body any) ([]byte, error) {
	status, raw, err := d.fetch(ctx, method, path, params, body)
	if err != nil {
		return nil, err
	}
	if err := statusError(status, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (d *Datasource) indexPath(suffix string) string {
	return "/" + url.PathEscape(d.opts.Index) + suffix
}

var lenientIndices = url.Values{"ignore_unavailable": {"true"}, "allow_no_indices": {"true"}}

func (d *Datasource) search(ctx context.Context, body map[string]any) ([]byte, error) {
	return d.call(ctx, http.MethodPost, d.indexPath("/_search"), lenientIndices, body)
}

func (d *Datasource) rangeQuery(r datasource.TimeRange) map[string]any {
	return map[string]any{"range": map[string]any{d.opts.TimeField: map[string]any{
		"gte":    r.From.UnixMilli(),
		"lte":    r.To.UnixMilli(),
		"format": "epoch_millis",
	}}}
}
