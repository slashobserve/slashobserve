package loki

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	facetsNeedLogQuery = "facets need a log query that starts with a stream selector"
	facetBytesUnit     = "bytes"
	maxFacetLabels     = 20
	maxDetectedFields  = 50
	facetConcurrency   = 4
	firstSampleWindow  = 5 * time.Minute
)

var _ datasource.LogFacetSource = (*Datasource)(nil)

type detectedField struct {
	Label       string   `json:"label"`
	Cardinality int      `json:"cardinality"`
	Parsers     []string `json:"parsers"`
}

func facetKind(field detectedField, detected bool) string {
	if detected && len(field.Parsers) == 0 {
		return datasource.FacetKindMetadata
	}
	return datasource.FacetKindField
}

func (d *Datasource) LogFacets(ctx context.Context, req datasource.FacetRequest) (datasource.Facets, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.Facets{}, err
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := datasource.Bound(ctx, lim.Timeout)
	defer cancel()
	text, err := expand(req.Query, req.Range, datasource.InstantInterval(req.Range, req.Query.MinInterval))
	if err != nil {
		return datasource.Facets{}, err
	}
	if !spec.LogQLReturnsLines(text) {
		return datasource.Facets{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", facetsNeedLogQuery)
	}
	selector := selectorOf(text)
	window := windowOf(req.Range)
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(facetConcurrency)
	var sample newestSample
	takeSample := func() {
		sample.taken = true
		group.Go(func() error { return sample.read(groupCtx, d, text, req, lim) })
	}
	if req.Fields == nil {
		takeSample()
	}
	indexLabels, err := d.indexLabelsInTime(groupCtx, withQuery(window, selector))
	if err != nil {
		return datasource.Facets{}, firstFailure(groupCtx, err, cancel, group)
	}
	labels, requestedFields := splitFacetFields(indexLabels, req.Fields)
	out := datasource.Facets{Fields: []datasource.FacetField{}}
	if len(labels) > maxFacetLabels {
		labels, out.Truncated = labels[:maxFacetLabels], true
	}
	if len(requestedFields) > 0 {
		takeSample()
	}
	labelFields := make([]datasource.FacetField, len(labels))
	counted := make([]bool, len(labels))
	for i, name := range labels {
		group.Go(func() error {
			field, ok, err := d.labelFacetInTime(groupCtx, selector, window, name, req.Limit)
			labelFields[i], counted[i] = field, ok
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return datasource.Facets{}, err
	}
	for i, name := range labels {
		if !counted[i] {
			labelFields[i] = sampleFacet(sample.frames, name, datasource.FacetKindLabel, 0, req.Limit)
		}
	}
	out.Fields = append(out.Fields, labelFields...)
	if !sample.taken {
		return out, nil
	}
	out.Sampled = true
	out.LinesSampled = lineCount(sample.frames)
	out.Fields = append(out.Fields, sample.fields(indexLabels, requestedFields, req.Limit)...)
	out.Truncated = out.Truncated || len(sample.detected) >= maxDetectedFields
	return out, nil
}

func (s *newestSample) fields(indexLabels, requested []string, limit int) []datasource.FacetField {
	names, byName := sampledFieldNames(s.detected, s.noDetected, s.frames, indexLabels, requested)
	out := make([]datasource.FacetField, 0, len(names))
	for _, name := range names {
		field, known := byName[name]
		out = append(out, sampleFacet(s.frames, name, facetKind(field, known), field.Cardinality, limit))
	}
	return out
}

func firstFailure(groupCtx context.Context, err error, cancel context.CancelFunc, group *errgroup.Group) error {
	groupFailedFirst := groupCtx.Err() != nil
	cancel()
	if groupErr := group.Wait(); groupFailedFirst && groupErr != nil {
		return groupErr
	}
	return err
}

type newestSample struct {
	taken      bool
	frames     []datasource.Frame
	detected   []detectedField
	noDetected bool
}

func (s *newestSample) read(ctx context.Context, d *Datasource, text string, req datasource.FacetRequest, lim datasource.Limits) error {
	frames, sampled, err := d.sampleNewest(ctx, text, req.Range, req.Sample, lim)
	if err != nil {
		return err
	}
	s.frames = frames
	s.detected, s.noDetected, err = d.detectedFieldsInTime(ctx, text, sampled)
	return err
}

func (d *Datasource) indexLabelsInTime(ctx context.Context, params url.Values) ([]string, error) {
	win, ok := (&datasource.Pacer{}).Start(ctx, 0)
	if !ok {
		return nil, nil
	}
	names, err := d.getStrings(win.Ctx, "/loki/api/v1/labels", params)
	if ranOut := win.End(err); ranOut {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names = slices.DeleteFunc(slices.Sorted(slices.Values(names)), func(name string) bool { return strings.HasPrefix(name, "__") })
	return slices.Compact(names), nil
}

func (d *Datasource) labelFacetInTime(ctx context.Context, selector string, window url.Values, name string, limit int) (datasource.FacetField, bool, error) {
	win, ok := (&datasource.Pacer{}).Start(ctx, 0)
	if !ok {
		return datasource.FacetField{}, false, nil
	}
	field, err := d.labelFacet(win.Ctx, selector, window, name, limit)
	if ranOut := win.End(err); ranOut {
		return datasource.FacetField{}, false, nil
	}
	return field, err == nil, err
}

func windowOf(r datasource.TimeRange) url.Values {
	return url.Values{"start": {lokiTime(r.From)}, "end": {lokiTime(r.To)}}
}

func (d *Datasource) sampleNewest(ctx context.Context, text string, r datasource.TimeRange, sample int, lim datasource.Limits) ([]datasource.Frame, datasource.TimeRange, error) {
	var (
		frames []datasource.Frame
		pacer  datasource.Pacer
	)
	sampled := datasource.TimeRange{From: r.To, To: r.To}
	for _, w := range datasource.NewestFirst(r, firstSampleWindow) {
		win, ok := pacer.Start(ctx, w.To.Sub(w.From))
		if !ok {
			break
		}
		wanted := sample - lineCount(frames)
		got, err := d.run(win.Ctx, "/loki/api/v1/query_range", sampleParams(text, windowOf(w), wanted), lim, wanted)
		if ranOut := win.End(err); err != nil {
			if !ranOut {
				return nil, datasource.TimeRange{}, err
			}
			break
		}
		frames, sampled.From = append(frames, got...), w.From
		if lineCount(frames) >= sample {
			break
		}
	}
	return frames, sampled, nil
}

func (d *Datasource) detectedFieldsInTime(ctx context.Context, text string, sampled datasource.TimeRange) ([]detectedField, bool, error) {
	if !sampled.From.Before(sampled.To) {
		return nil, true, nil
	}
	win, ok := (&datasource.Pacer{}).Start(ctx, sampled.To.Sub(sampled.From))
	if !ok {
		return nil, true, nil
	}
	detected, noDetected, err := d.detectedFields(win.Ctx, text, windowOf(sampled))
	if ranOut := win.End(err); ranOut {
		return nil, true, nil
	}
	return detected, noDetected, err
}

func withQuery(window url.Values, query string) url.Values {
	params := maps.Clone(window)
	params.Set("query", query)
	return params
}

func sampleParams(text string, window url.Values, sample int) url.Values {
	params := withQuery(window, text)
	params.Set("limit", strconv.Itoa(sample))
	params.Set("direction", "backward")
	return params
}

func splitFacetFields(indexLabels, requested []string) (labels, fields []string) {
	if requested == nil {
		return indexLabels, nil
	}
	for _, name := range requested {
		if slices.Contains(indexLabels, name) {
			labels = append(labels, name)
		} else {
			fields = append(fields, name)
		}
	}
	return labels, fields
}

func (d *Datasource) labelFacet(ctx context.Context, selector string, window url.Values, name string, limit int) (datasource.FacetField, error) {
	params := withQuery(window, selector)
	params.Set("targetLabels", name)
	params.Set("limit", strconv.Itoa(limit+1))
	raw, err := d.get(ctx, "/loki/api/v1/index/volume", params)
	if err != nil {
		return datasource.FacetField{}, err
	}
	var data queryData
	if err := json.Unmarshal(raw, &data); err != nil {
		return datasource.FacetField{}, datasource.Unreadable()
	}
	field := datasource.FacetField{Name: name, Kind: datasource.FacetKindLabel, Exact: true, Unit: facetBytesUnit, Values: []datasource.FacetValue{}}
	err = decodeEach(data.Result, 0, func(s series) error {
		value, ok := s.Metric[name]
		if !ok || s.Value == nil {
			return nil
		}
		n, err := strconv.ParseFloat(s.Value.value, 64)
		if err != nil {
			return datasource.Unreadable()
		}
		field.Values = append(field.Values, datasource.FacetValue{Value: value, Count: int64(n)})
		return nil
	})
	if err != nil {
		return datasource.FacetField{}, err
	}
	field.Cardinality = len(field.Values)
	field.Values, field.Truncated = rankValues(field.Values, limit)
	return field, nil
}

func rankValues(values []datasource.FacetValue, limit int) (ranked []datasource.FacetValue, truncated bool) {
	slices.SortFunc(values, func(a, b datasource.FacetValue) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Value, b.Value))
	})
	if len(values) > limit {
		return values[:limit], true
	}
	return values, false
}

func (d *Datasource) detectedFields(ctx context.Context, text string, window url.Values) ([]detectedField, bool, error) {
	params := withQuery(window, text)
	params.Set("limit", strconv.Itoa(maxDetectedFields))
	status, raw, err := d.fetch(ctx, "/loki/api/v1/detected_fields", params)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, true, nil
	}
	if err := statusError(status, raw); err != nil {
		return nil, false, err
	}
	var body struct {
		Fields []detectedField `json:"fields"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, false, datasource.Unreadable()
	}
	return body.Fields, false, nil
}

func sampledFieldNames(detected []detectedField, noDetected bool, frames []datasource.Frame, indexLabels, requested []string) ([]string, map[string]detectedField) {
	byName := make(map[string]detectedField, len(detected))
	for _, f := range detected {
		byName[f.Label] = f
	}
	if requested != nil {
		return slices.Compact(slices.Sorted(slices.Values(requested))), byName
	}
	names := make([]string, 0, len(detected))
	if noDetected {
		seen := map[string]struct{}{}
		for _, f := range frames {
			for name := range f.Labels {
				seen[name] = struct{}{}
			}
		}
		names = slices.Sorted(maps.Keys(seen))
	} else {
		for _, f := range detected {
			names = append(names, f.Label)
		}
	}
	names = slices.DeleteFunc(names, func(name string) bool {
		return strings.HasPrefix(name, "__") || slices.Contains(indexLabels, name)
	})
	return slices.Compact(names), byName
}

func sampleFacet(frames []datasource.Frame, name, kind string, cardinality, limit int) datasource.FacetField {
	counts := map[string]int64{}
	for _, f := range frames {
		value, ok := f.Labels[name]
		if !ok {
			continue
		}
		counts[value] += int64(len(f.Fields[0].Values))
	}
	field := datasource.FacetField{Name: name, Kind: kind, Values: make([]datasource.FacetValue, 0, len(counts))}
	for value, count := range counts {
		field.Values = append(field.Values, datasource.FacetValue{Value: value, Count: count})
	}
	field.Cardinality = max(cardinality, len(counts))
	field.Values, field.Truncated = rankValues(field.Values, limit)
	return field
}

func selectorOf(text string) string {
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '"', '`':
			end := stringEnd(text, i)
			if end < 0 {
				return strings.TrimSpace(text)
			}
			i = end - 1
		case '|':
			return strings.TrimSpace(text[:i])
		}
	}
	return strings.TrimSpace(text)
}
