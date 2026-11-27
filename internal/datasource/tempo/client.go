package tempo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var limitWords = []string{"limit", "too many", "maximum", "exceed"}

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
		return 0, nil, datasource.TransportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, d.opts.MaxResponseBytes+1))
	if err != nil {
		return 0, nil, datasource.TransportError(ctx, err)
	}
	if int64(len(raw)) > d.opts.MaxResponseBytes {
		return 0, nil, datasource.Errorf(datasource.ErrLimitExceeded, "response larger than %d bytes", d.opts.MaxResponseBytes)
	}
	return resp.StatusCode, raw, nil
}

func (d *Datasource) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	status, raw, err := d.fetch(ctx, path, params)
	if err != nil {
		return nil, err
	}
	if err := statusError(status, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (d *Datasource) getJSON(ctx context.Context, path string, params url.Values, into any) error {
	raw, err := d.get(ctx, path, params)
	if err != nil {
		return err
	}
	if json.Unmarshal(raw, into) != nil {
		return datasource.Unreadable()
	}
	return nil
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
	text := datasource.UpstreamText(raw)
	if limit, ok := searchRangeLimit(text); ok {
		return datasource.Errorf(datasource.ErrRangeTooLong, "Tempo searches at most %s at a time; pick a shorter time range", limit)
	}
	lower := strings.ToLower(text)
	kind, fallback := datasource.ErrUnavailable, fmt.Sprintf("upstream returned HTTP %d", status)
	switch {
	case status == http.StatusBadRequest && slices.ContainsFunc(limitWords, func(w string) bool { return strings.Contains(lower, w) }):
		kind, fallback = datasource.ErrLimitExceeded, "query limit exceeded"
	case status == http.StatusBadRequest:
		kind, fallback = datasource.ErrInvalidQuery, "query rejected by the datasource"
	case status == http.StatusGatewayTimeout || strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		kind, fallback = datasource.ErrTimeout, "query timed out"
	}
	if text == "" || datasource.NamesAddress(text) {
		text = fallback
	}
	return datasource.ErrorfStatus(kind, status, "%s", text)
}

var searchRangeExceeds = regexp.MustCompile(`range specified by start and end exceeds ((?:\d+h)?(?:\d+m)?\d+s)`)

func searchRangeLimit(text string) (string, bool) {
	match := searchRangeExceeds.FindStringSubmatch(text)
	if match == nil {
		return "", false
	}
	limit, err := time.ParseDuration(match[1])
	if err != nil || limit <= 0 {
		return "", false
	}
	switch {
	case limit >= 24*time.Hour && limit%(24*time.Hour) == 0:
		return plural(int(limit/(24*time.Hour)), "day"), true
	case limit >= time.Hour && limit%time.Hour == 0:
		return plural(int(limit/time.Hour), "hour"), true
	case limit >= time.Minute:
		return plural(int(limit/time.Minute), "minute"), true
	}
	return "", false
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
