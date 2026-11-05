package loki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const maxUpstreamTextBytes = 512

type timeoutKey struct{}

type envelope struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

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

func (d *Datasource) fetch(ctx context.Context, path string, params url.Values) (int, []byte, error) {
	target := d.prefix + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, nil, datasource.Errorf(datasource.ErrUnavailable, "connection failed")
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

func (d *Datasource) get(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	status, raw, err := d.fetch(ctx, path, params)
	if err != nil {
		return nil, err
	}
	if err := statusError(status, raw); err != nil {
		return nil, err
	}
	var env envelope
	if json.Unmarshal(raw, &env) != nil || env.Status != "success" {
		return nil, unreadable()
	}
	return env.Data, nil
}

func (d *Datasource) ready(ctx context.Context) error {
	status, raw, err := d.fetch(ctx, "/ready", nil)
	if err != nil {
		return err
	}
	return statusError(status, raw)
}

func statusError(status int, raw []byte) error {
	switch {
	case status >= 200 && status <= 299:
		return nil
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return datasource.Errorf(datasource.ErrAuthFailed, "authentication failed (HTTP %d)", status)
	case status == http.StatusTooManyRequests:
		return datasource.Errorf(datasource.ErrUnavailable, "rate limited by upstream")
	}
	text := upstreamText(raw)
	lower := strings.ToLower(text)
	kind, fallback := datasource.ErrUnavailable, fmt.Sprintf("upstream returned HTTP %d", status)
	switch {
	case status == http.StatusBadRequest && (strings.Contains(lower, "limit") || strings.Contains(lower, "too many") || strings.Contains(lower, "maximum")):
		kind, fallback = datasource.ErrLimitExceeded, "query limit exceeded"
	case status == http.StatusBadRequest:
		kind, fallback = datasource.ErrInvalidQuery, "query rejected by the datasource"
	case status == http.StatusGatewayTimeout || strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		kind, fallback = datasource.ErrTimeout, "query timed out"
	}
	if text == "" || datasource.NamesAddress(text) {
		text = fallback
	}
	return datasource.Errorf(kind, "%s", text)
}

func upstreamText(raw []byte) string {
	var body struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	text := string(bytes.TrimSpace(raw))
	if json.Unmarshal(raw, &body) == nil {
		text = body.Message
		if text == "" {
			text = body.Error
		}
	}
	return truncateText(text)
}

func truncateText(s string) string {
	if len(s) > maxUpstreamTextBytes {
		s = s[:maxUpstreamTextBytes]
	}
	return strings.ToValidUTF8(s, "")
}

func unreadable() error {
	return datasource.Errorf(datasource.ErrUnavailable, "upstream returned an unreadable result")
}
