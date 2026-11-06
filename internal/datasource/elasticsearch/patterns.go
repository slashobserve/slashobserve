package elasticsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	patternsAgg          = "patterns"
	overTimeAgg          = "over_time"
	maxPatternCategories = 100
)

var _ datasource.LogPatternSource = (*Datasource)(nil)

func (d *Datasource) LogPatterns(ctx context.Context, req datasource.PatternRequest) ([]datasource.NativePattern, error) {
	if d.opts.Flavor == OpenSearch {
		return nil, datasource.Errorf(datasource.ErrNotFound, "opensearch has no pattern aggregation")
	}
	if err := req.Range.Validate(); err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	prep, err := d.prepare(ctx, req.Query, req.Range, instantInterval(req.Range, req.Query.MinInterval), lim)
	if err != nil {
		return nil, err
	}
	if prep.user.aggs() != nil {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "patterns need a KQL, Lucene or filter-only query body")
	}
	roles, err := prep.mapping.roles(d.opts.Fields)
	if err != nil {
		return nil, err
	}
	if roles.Message == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "no message field in the mapping; set fields.message")
	}
	step, err := histogramStep(req.Range, req.Step, lim)
	if err != nil {
		return nil, err
	}
	body := buildBody(prep.user, d.opts.TimeField, req.Range, prep.adhoc, plan{kind: searchLines, paged: true, timeout: lim.Timeout, totalHits: false})
	delete(body, "sort")
	body["aggs"] = map[string]any{patternsAgg: map[string]any{
		"categorize_text": map[string]any{"field": roles.Message, "size": maxPatternCategories, "similarity_threshold": 70, "max_matched_tokens": 5},
		"aggs":            map[string]any{overTimeAgg: map[string]any{"date_histogram": map[string]any{"field": d.opts.TimeField, "fixed_interval": fixedInterval(step)}}},
	}}
	status, raw, err := d.fetch(ctx, http.MethodPost, d.indexPath("/_search"), lenientIndices, body)
	if err != nil {
		return nil, err
	}
	if categorizeUnavailable(status, raw) {
		return nil, datasource.Errorf(datasource.ErrNotFound, "this cluster has no categorize_text aggregation")
	}
	if err := statusError(status, raw); err != nil {
		return nil, err
	}
	return nativePatterns(raw)
}

func categorizeUnavailable(status int, raw []byte) bool {
	e := parseError(raw)
	text := strings.ToLower(e.text)
	switch status {
	case http.StatusForbidden:
		return e.is("security_exception") && strings.Contains(text, "categorize-text-agg")
	case http.StatusBadRequest:
		return strings.Contains(text, "unknown aggregation") && strings.Contains(text, "categorize_text")
	}
	return false
}

func nativePatterns(raw []byte) ([]datasource.NativePattern, error) {
	resp, err := parseSearch(raw)
	if err != nil {
		return nil, err
	}
	var tree struct {
		Buckets []struct {
			Key      string `json:"key"`
			OverTime struct {
				Buckets []struct {
					Key      int64 `json:"key"`
					DocCount int64 `json:"doc_count"`
				} `json:"buckets"`
			} `json:"over_time"`
		} `json:"buckets"`
	}
	if raw := resp.Aggregations[patternsAgg]; len(raw) > 0 && json.Unmarshal(raw, &tree) != nil {
		return nil, unreadable()
	}
	patterns := make([]datasource.NativePattern, 0, len(tree.Buckets))
	for _, bucket := range tree.Buckets {
		if bucket.Key == "" {
			continue
		}
		pattern := datasource.NativePattern{Pattern: bucket.Key}
		for _, b := range bucket.OverTime.Buckets {
			if b.DocCount > 0 {
				pattern.Samples = append(pattern.Samples, datasource.PatternSample{At: time.UnixMilli(b.Key).UTC(), Count: b.DocCount})
			}
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}
