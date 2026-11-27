package tempo

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	spansPerSpanSet     = 100
	durationTraceLimit  = 5000
	missingSelectValue  = "nil"
	errorStatusSelected = "error"
)

var _ datasource.TraceDurationSource = (*Datasource)(nil)

type spanSetResponse struct {
	Traces []struct {
		TraceID  string `json:"traceID"`
		SpanSets []struct {
			Spans []struct {
				Name       string     `json:"name"`
				Start      unixNanos  `json:"startTimeUnixNano"`
				Duration   unixNanos  `json:"durationNanos"`
				Attributes []keyValue `json:"attributes"`
			} `json:"spans"`
			Matched int `json:"matched"`
		} `json:"spanSets"`
	} `json:"traces"`
	Metrics searchMetrics `json:"metrics"`
}

type durationRead struct {
	spans    []datasource.SpanDuration
	seen     map[string]bool
	sampled  bool
	maxSpans int
}

func (d *Datasource) TraceDurations(ctx context.Context, req datasource.TraceDurationRequest) (datasource.TraceDurations, error) {
	q := strings.TrimSpace(req.Query)
	if !strings.HasPrefix(q, "{") || !strings.HasSuffix(q, "}") {
		return datasource.TraceDurations{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", statsNeedsFilter)
	}
	groupBy := strings.TrimSpace(req.GroupBy)
	if groupBy != "" && !validGroupBy(groupBy) {
		return datasource.TraceDurations{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", groupByNeedsAttribute)
	}
	if err := rangeStartsInFuture(req.Range); err != nil {
		return datasource.TraceDurations{}, err
	}
	lim := d.opts.Limits.Tighten(req.Limits)
	ctx, cancel := datasource.Bound(ctx, lim.Timeout)
	defer cancel()
	read := &durationRead{seen: map[string]bool{}, maxSpans: cmpPositive(req.MaxSpans, datasource.TraceDurationSpanBudget)}
	text := q + " | " + selectClause(groupBy)
	var pacer datasource.Pacer
	for _, w := range datasource.NewestFirst(req.Range, firstSearchWindow) {
		if len(read.spans) >= read.maxSpans || len(read.seen) >= durationTraceLimit {
			read.sampled = true
			break
		}
		win, ok := pacer.Start(ctx, w.To.Sub(w.From))
		if !ok {
			read.sampled = true
			break
		}
		resp, err := d.searchSpanSets(win.Ctx, text, w, durationTraceLimit-len(read.seen))
		if ranOut := win.End(err); err != nil {
			if !ranOut {
				return datasource.TraceDurations{}, err
			}
			read.sampled = true
			break
		}
		read.add(resp, req.Range, groupBy, durationTraceLimit-len(read.seen))
	}
	window, groups, truncated := datasource.FoldDurations(read.spans, groupBy != "", datasource.MaxDurationGroups)
	return datasource.TraceDurations{Window: window, Groups: groups, GroupsTruncated: truncated, Exact: !read.sampled}, nil
}

func cmpPositive(n, fallback int) int {
	if n > 0 {
		return n
	}
	return fallback
}

func selectClause(groupBy string) string {
	fields := []string{"status"}
	if groupBy != "" && groupBy != "status" {
		fields = append(fields, groupBy)
	}
	return "select(" + strings.Join(fields, ", ") + ")"
}

func (d *Datasource) searchSpanSets(ctx context.Context, text string, r datasource.TimeRange, limit int) (spanSetResponse, error) {
	sent := newestFirst(text)
	params := url.Values{
		"q":     {sent},
		"start": {strconv.FormatInt(r.From.Unix(), 10)},
		"end":   {strconv.FormatInt(ceilSeconds(r.To), 10)},
		"limit": {strconv.Itoa(limit)},
		"spss":  {strconv.Itoa(spansPerSpanSet)},
	}
	var resp spanSetResponse
	if err := d.getJSON(ctx, "/api/search", params, &resp); err != nil {
		return spanSetResponse{}, traceQLProblem(err, text, sent)
	}
	return resp, nil
}

func (r *durationRead) add(resp spanSetResponse, window datasource.TimeRange, groupBy string, limit int) {
	if len(resp.Traces) >= limit || resp.Metrics.partial() {
		r.sampled = true
	}
	key := selectedKey(groupBy)
	for _, t := range resp.Traces {
		id := strings.ToLower(t.TraceID)
		if r.seen[id] {
			continue
		}
		r.seen[id] = true
		for _, set := range t.SpanSets {
			if set.Matched > len(set.Spans) {
				r.sampled = true
			}
			for _, s := range set.Spans {
				start := time.Unix(0, int64(s.Start))
				if start.Before(window.From) || !start.Before(window.To) {
					continue
				}
				if len(r.spans) >= r.maxSpans {
					r.sampled = true
					return
				}
				attrs := attributeMap(s.Attributes)
				sd := datasource.SpanDuration{Nanos: int64(s.Duration), Error: fmt.Sprint(attrs["status"]) == errorStatusSelected}
				if groupBy == "name" {
					sd.Group, sd.Grouped = s.Name, s.Name != ""
				} else if v, ok := attrs[key]; ok && v != nil {
					sd.Group = fmt.Sprint(v)
					sd.Grouped = sd.Group != "" && sd.Group != missingSelectValue
				}
				r.spans = append(r.spans, sd)
			}
		}
	}
}

func selectedKey(attribute string) string {
	if strings.HasPrefix(attribute, ".") {
		return attribute[1:]
	}
	for _, scope := range []string{"resource.", "span.", "event.", "link.", "instrumentation."} {
		if strings.HasPrefix(attribute, scope) {
			return attribute[len(scope):]
		}
	}
	return attribute
}
