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

type envelope struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	Warnings  []string        `json:"warnings"`
	ErrorType string          `json:"errorType"`
	Error     string          `json:"error"`
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
	raw, _, err := d.doWithWarnings(ctx, method, path, form)
	return raw, err
}

func (d *Datasource) doWithWarnings(ctx context.Context, method, path string, form url.Values) (json.RawMessage, []string, error) {
	req, err := d.newRequest(ctx, method, path, form)
	if err != nil {
		return nil, nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, nil, datasource.TransportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, d.opts.MaxResponseBytes+1))
	if err != nil {
		return nil, nil, datasource.TransportError(ctx, err)
	}
	if int64(len(raw)) > d.opts.MaxResponseBytes {
		return nil, nil, datasource.Errorf(datasource.ErrLimitExceeded, "response larger than %d bytes", d.opts.MaxResponseBytes)
	}
	return decodeResponse(resp.StatusCode, raw)
}

func decodeResponse(status int, raw []byte) (json.RawMessage, []string, error) {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, nil, datasource.Errorf(datasource.ErrAuthFailed, "authentication failed (HTTP %d)", status)
	case http.StatusTooManyRequests:
		return nil, nil, datasource.Errorf(datasource.ErrUnavailable, "rate limited by upstream")
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil || (env.Status != "success" && env.Status != "error") {
		return nil, nil, datasource.ErrorfStatus(datasource.ErrUnavailable, status, "upstream returned HTTP %d", status)
	}
	if env.Status == "error" {
		return nil, nil, upstreamError(env)
	}
	if status < 200 || status > 299 {
		return nil, nil, datasource.ErrorfStatus(datasource.ErrUnavailable, status, "upstream returned HTTP %d", status)
	}
	return env.Data, upstreamWarnings(env.Warnings), nil
}

func upstreamWarnings(raw []string) []string {
	var out []string
	for _, w := range raw {
		w = datasource.TruncateText(w)
		if w == "" {
			continue
		}
		if datasource.NamesAddress(w) {
			w = "the datasource reported a warning"
		}
		out = append(out, w)
	}
	return out
}

func upstreamError(env envelope) error {
	kind, fallback := datasource.ErrUnavailable, "upstream error"
	lower := strings.ToLower(env.Error)
	switch {
	case env.ErrorType == "bad_data":
		kind, fallback = datasource.ErrInvalidQuery, "query rejected by the datasource"
	case env.ErrorType == "timeout":
		kind, fallback = datasource.ErrTimeout, "query timed out"
	case env.ErrorType == "execution" && (strings.Contains(lower, "too many") || strings.Contains(lower, "limit")):
		kind, fallback = datasource.ErrLimitExceeded, "query limit exceeded"
	}
	text := datasource.TruncateText(env.Error)
	if text == "" || datasource.NamesAddress(env.Error) {
		text = fallback
	}
	return datasource.Errorf(kind, "%s", text)
}

func promDuration(d time.Duration) string {
	return strconv.FormatInt(max(d.Milliseconds(), 1), 10) + "ms"
}

func unixSeconds(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixMilli())/1000, 'f', 3, 64)
}
