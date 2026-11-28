package tempo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var limitWords = []string{"limit", "too many", "maximum", "exceed"}

func (d *Datasource) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	status, raw, err := d.http.Fetch(ctx, path, params)
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
