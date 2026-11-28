package jaeger

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var limitWords = []string{"limit", "too many", "maximum", "exceed", "search depth"}

func (d *Datasource) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	status, raw, err := d.http.Fetch(ctx, path, params)
	if err != nil {
		return nil, err
	}
	if err := statusError(path, status, raw); err != nil {
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

func statusError(path string, status int, raw []byte) error {
	switch {
	case status >= 200 && status <= 299:
		return nil
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return datasource.Errorf(datasource.ErrAuthFailed, "authentication failed (HTTP %d)", status)
	case status == http.StatusTooManyRequests:
		return datasource.Errorf(datasource.ErrUnavailable, "rate limited by upstream")
	case status == http.StatusNotFound && !jsonBody(raw):
		return datasource.Errorf(datasource.ErrUnavailable, "this server does not serve %s; SlashObserve reads Jaeger 2.21 or later through its /api/v3 query API", path)
	}
	text := upstreamText(raw)
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

func jsonBody(raw []byte) bool {
	return json.Valid(bytes.TrimSpace(raw))
}

func upstreamText(raw []byte) string {
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	text := string(bytes.TrimSpace(raw))
	if json.Unmarshal(raw, &body) == nil {
		text = cmp.Or(body.Error.Message, body.Message)
	}
	return datasource.TruncateText(text)
}
