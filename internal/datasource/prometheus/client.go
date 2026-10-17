package prometheus

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const maxUpstreamTextBytes = 512

type timeoutKey struct{}

type envelope struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType"`
	Error     string          `json:"error"`
}

func bound(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	reported := timeout
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= timeout {
		reported = 0
	}
	ctx = context.WithValue(ctx, timeoutKey{}, reported)
	return context.WithTimeout(ctx, timeout)
}

func contextError(ctx context.Context, err error) error {
	reported, _ := ctx.Value(timeoutKey{}).(time.Duration)
	return datasource.ContextError(ctx, err, reported)
}

func transportError(ctx context.Context, err error) error {
	if cerr := contextError(ctx, err); cerr != nil {
		return cerr
	}
	return datasource.Errorf(datasource.ErrUnavailable, "connection failed")
}

func (d *Datasource) newRequest(ctx context.Context, method, path string, form url.Values) (*http.Request, error) {
	target := d.prefix + path
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(form.Encode())
	} else if len(form) > 0 {
		target += "?" + form.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json")
	for name, value := range d.opts.Headers {
		req.Header.Set(name, value)
	}
	switch {
	case d.opts.BasicAuthUser != "":
		req.SetBasicAuth(d.opts.BasicAuthUser, d.opts.BasicAuthPassword.Reveal())
	case !d.opts.BearerToken.IsZero():
		req.Header.Set("Authorization", "Bearer "+d.opts.BearerToken.Reveal())
	}
	return req, nil
}

func (d *Datasource) do(ctx context.Context, method, path string, form url.Values) (json.RawMessage, error) {
	req, err := d.newRequest(ctx, method, path, form)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, d.opts.MaxResponseBytes+1))
	if err != nil {
		return nil, transportError(ctx, err)
	}
	if int64(len(raw)) > d.opts.MaxResponseBytes {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "response larger than %d bytes", d.opts.MaxResponseBytes)
	}
	return decodeResponse(resp.StatusCode, raw)
}

func decodeResponse(status int, raw []byte) (json.RawMessage, error) {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, datasource.Errorf(datasource.ErrUnavailable, "authentication failed (HTTP %d)", status)
	case http.StatusTooManyRequests:
		return nil, datasource.Errorf(datasource.ErrUnavailable, "rate limited by upstream")
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil || (env.Status != "success" && env.Status != "error") {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "upstream returned HTTP %d", status)
	}
	if env.Status == "error" {
		return nil, upstreamError(env)
	}
	if status < 200 || status > 299 {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "upstream returned HTTP %d", status)
	}
	return env.Data, nil
}

func upstreamError(env envelope) error {
	text := truncateText(env.Error)
	if text == "" {
		text = "upstream error"
	}
	lower := strings.ToLower(env.Error)
	switch {
	case env.ErrorType == "bad_data":
		return datasource.Errorf(datasource.ErrInvalidQuery, "%s", text)
	case env.ErrorType == "timeout":
		return datasource.Errorf(datasource.ErrTimeout, "%s", text)
	case env.ErrorType == "execution" && (strings.Contains(lower, "too many") || strings.Contains(lower, "limit")):
		return datasource.Errorf(datasource.ErrLimitExceeded, "%s", text)
	}
	return datasource.Errorf(datasource.ErrUnavailable, "%s", text)
}

func truncateText(s string) string {
	if len(s) > maxUpstreamTextBytes {
		s = s[:maxUpstreamTextBytes]
	}
	return strings.ToValidUTF8(s, "")
}

func promDuration(d time.Duration) string {
	return strconv.FormatInt(max(d.Milliseconds(), 1), 10) + "ms"
}

func unixSeconds(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixMilli())/1000, 'f', 3, 64)
}
