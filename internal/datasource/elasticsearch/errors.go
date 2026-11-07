package elasticsearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const maxUpstreamTextBytes = 512

var invalidQueryTypes = []string{"parsing_exception", "query_shard_exception", "illegal_argument_exception", "x_content_parse_exception"}

type upstreamError struct {
	types []string
	text  string
}

func (e upstreamError) is(typ string) bool { return slices.Contains(e.types, typ) }

func (e upstreamError) isAny(types []string) bool {
	return slices.ContainsFunc(e.types, func(t string) bool { return slices.Contains(types, t) })
}

func parseError(raw []byte) upstreamError {
	var body struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &body) != nil || len(body.Error) == 0 {
		return upstreamError{text: truncateText(string(bytes.TrimSpace(raw)))}
	}
	var text string
	if json.Unmarshal(body.Error, &text) == nil {
		return upstreamError{text: truncateText(text)}
	}
	var detail struct {
		Type      string `json:"type"`
		Reason    string `json:"reason"`
		RootCause []struct {
			Type   string `json:"type"`
			Reason string `json:"reason"`
		} `json:"root_cause"`
	}
	if json.Unmarshal(body.Error, &detail) != nil {
		return upstreamError{}
	}
	e := upstreamError{types: []string{detail.Type}, text: detail.Reason}
	if len(detail.RootCause) > 0 {
		e.types = append(e.types, detail.RootCause[0].Type)
		if detail.RootCause[0].Reason != "" {
			e.text = detail.RootCause[0].Reason
		}
	}
	e.text = truncateText(e.text)
	return e
}

func statusError(status int, raw []byte) error {
	switch {
	case status >= 200 && status <= 299:
		return nil
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return datasource.Errorf(datasource.ErrAuthFailed, "authentication failed (HTTP %d)", status)
	}
	e := parseError(raw)
	kind, fallback := datasource.ErrUnavailable, fmt.Sprintf("upstream returned HTTP %d", status)
	switch {
	case e.is("index_not_found_exception"):
		kind, fallback = datasource.ErrNoIndex, "the index pattern matches no index"
	case e.is("too_many_buckets_exception"), e.is("circuit_breaking_exception"):
		kind, fallback = datasource.ErrLimitExceeded, "query limit exceeded"
	case e.isAny(invalidQueryTypes):
		kind, fallback = datasource.ErrInvalidQuery, "query rejected by the datasource"
	case status == http.StatusTooManyRequests:
		return datasource.Errorf(datasource.ErrUnavailable, "rate limited by upstream")
	case status == http.StatusRequestTimeout, status == http.StatusGatewayTimeout:
		kind, fallback = datasource.ErrTimeout, "query timed out"
	case status == http.StatusBadRequest:
		kind, fallback = datasource.ErrInvalidQuery, "query rejected by the datasource"
	}
	text := e.text
	if text == "" || datasource.NamesAddress(text) {
		text = fallback
	}
	return datasource.Errorf(kind, "%s", text)
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
