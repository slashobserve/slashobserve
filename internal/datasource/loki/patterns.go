package loki

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logpatterns"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	patternsNeedLogQuery = "patterns need a log query that starts with a stream selector"
	patternsPath         = "/loki/api/v1/patterns"
	lokiWildcard         = "<_>"
)

var _ datasource.LogPatternSource = (*Datasource)(nil)

type nativePattern struct {
	Pattern string          `json:"pattern"`
	Samples []patternSample `json:"samples"`
}

type patternSample struct {
	at    time.Time
	count int64
}

func (s *patternSample) UnmarshalJSON(b []byte) error {
	var pair [2]json.Number
	if err := json.Unmarshal(b, &pair); err != nil {
		return err
	}
	at, err := pair[0].Float64()
	if err != nil {
		return err
	}
	count, err := pair[1].Float64()
	if err != nil {
		return err
	}
	s.at = time.Unix(int64(math.Round(at)), 0).UTC()
	s.count = int64(math.Round(count))
	return nil
}

func (d *Datasource) LogPatterns(ctx context.Context, req datasource.PatternRequest) ([]datasource.NativePattern, error) {
	if err := req.Range.Validate(); err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := expand(req.Query, req.Range, instantInterval(req.Range, req.Query.MinInterval))
	if err != nil {
		return nil, err
	}
	if !spec.LogQLReturnsLines(text) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", patternsNeedLogQuery)
	}
	selector := selectorOf(text)
	if selector != strings.TrimSpace(text) {
		return nil, datasource.Errorf(datasource.ErrNotFound, "the pattern API answers only a stream selector")
	}
	params := url.Values{
		"query": {selector},
		"start": {strconv.FormatInt(req.Range.From.UnixNano(), 10)},
		"end":   {strconv.FormatInt(req.Range.To.UnixNano(), 10)},
		"step":  {strconv.FormatInt(int64(max(req.Step, time.Second)/time.Second), 10) + "s"},
	}
	status, raw, err := d.fetch(ctx, patternsPath, params)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, datasource.Errorf(datasource.ErrNotFound, "the pattern API is not enabled")
	}
	if err := statusError(status, raw); err != nil {
		return nil, err
	}
	var env envelope
	if json.Unmarshal(raw, &env) != nil || env.Status != "success" {
		return nil, unreadable()
	}
	if env.Data == nil {
		return nil, datasource.Errorf(datasource.ErrNotFound, "the pattern API answered without data")
	}
	var found []nativePattern
	if err := json.Unmarshal(env.Data, &found); err != nil {
		return nil, unreadable()
	}
	out := make([]datasource.NativePattern, len(found))
	for i, p := range found {
		samples := make([]datasource.PatternSample, len(p.Samples))
		for j, s := range p.Samples {
			samples[j] = datasource.PatternSample{At: s.at, Count: s.count}
		}
		out[i] = datasource.NativePattern{Pattern: strings.ReplaceAll(p.Pattern, lokiWildcard, logpatterns.Wildcard), Samples: samples}
	}
	return out, nil
}
