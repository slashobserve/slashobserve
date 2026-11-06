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
)

var _ datasource.LogFacetSource = (*Datasource)(nil)

type detectedField struct {
	Label       string `json:"label"`
	Cardinality int    `json:"cardinality"`
}

func (d *Datasource) LogFacets(ctx context.Context, req datasource.FacetRequest) (datasource.Facets, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.Facets{}, err
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := expand(req.Query, req.Range, instantInterval(req.Range, req.Query.MinInterval))
	if err != nil {
		return datasource.Facets{}, err
	}
	if !spec.LogQLReturnsLines(text) {
		return datasource.Facets{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", facetsNeedLogQuery)
	}
	selector := selectorOf(text)
	window := url.Values{"start": {lokiTime(req.Range.From)}, "end": {lokiTime(req.Range.To)}}
	indexLabels, err := d.getStrings(ctx, "/loki/api/v1/labels", withQuery(window, selector))
	if err != nil {
		return datasource.Facets{}, err
	}
	indexLabels = slices.DeleteFunc(slices.Sorted(slices.Values(indexLabels)), func(name string) bool { return strings.HasPrefix(name, "__") })
	indexLabels = slices.Compact(indexLabels)
	labels, requestedFields := splitFacetFields(indexLabels, req.Fields)
	out := datasource.Facets{Fields: []datasource.FacetField{}}
	if len(labels) > maxFacetLabels {
		labels, out.Truncated = labels[:maxFacetLabels], true
	}
	needSample := req.Fields == nil || len(requestedFields) > 0

	labelFields := make([]datasource.FacetField, len(labels))
	var (
		detected   []detectedField
		noDetected bool
		frames     []datasource.Frame
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(facetConcurrency)
	for i, name := range labels {
		group.Go(func() error {
			field, err := d.labelFacet(groupCtx, selector, window, name, req.Limit)
			labelFields[i] = field
			return err
		})
	}
	if needSample {
		group.Go(func() error {
			var err error
			detected, noDetected, err = d.detectedFields(groupCtx, text, window)
			return err
		})
		group.Go(func() error {
			var err error
			frames, err = d.run(groupCtx, "/loki/api/v1/query_range", sampleParams(text, window, req.Sample), lim, req.Sample)
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return datasource.Facets{}, err
	}
	out.Fields = append(out.Fields, labelFields...)
	if !needSample {
		return out, nil
	}
	out.Sampled = true
	out.LinesSampled = lineCount(frames)
	names, cardinality := sampledFieldNames(detected, noDetected, frames, indexLabels, requestedFields)
	if len(detected) >= maxDetectedFields {
		out.Truncated = true
	}
	for _, name := range names {
		out.Fields = append(out.Fields, sampleFacet(frames, name, cardinality[name], req.Limit))
	}
	return out, nil
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
		return datasource.FacetField{}, unreadable()
	}
	field := datasource.FacetField{Name: name, Kind: datasource.FacetKindLabel, Exact: true, Unit: facetBytesUnit, Values: []datasource.FacetValue{}}
	err = decodeEach(data.Result, 0, func(s series) error {
		value, ok := s.Metric[name]
		if !ok || s.Value == nil {
			return nil
		}
		n, err := strconv.ParseFloat(s.Value.value, 64)
		if err != nil {
			return unreadable()
		}
		field.Values = append(field.Values, datasource.FacetValue{Value: value, Count: int64(n)})
		return nil
	})
	if err != nil {
		return datasource.FacetField{}, err
	}
	field.Cardinality = len(field.Values)
	if len(field.Values) > limit {
		field.Values, field.Truncated = field.Values[:limit], true
	}
	return field, nil
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
		return nil, false, unreadable()
	}
	return body.Fields, false, nil
}

func sampledFieldNames(detected []detectedField, noDetected bool, frames []datasource.Frame, indexLabels, requested []string) ([]string, map[string]int) {
	cardinality := make(map[string]int, len(detected))
	for _, f := range detected {
		cardinality[f.Label] = f.Cardinality
	}
	if requested != nil {
		return slices.Compact(slices.Sorted(slices.Values(requested))), cardinality
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
	return slices.Compact(names), cardinality
}

func sampleFacet(frames []datasource.Frame, name string, cardinality, limit int) datasource.FacetField {
	counts := map[string]int64{}
	for _, f := range frames {
		value, ok := f.Labels[name]
		if !ok {
			continue
		}
		counts[value] += int64(len(f.Fields[0].Values))
	}
	field := datasource.FacetField{Name: name, Kind: datasource.FacetKindField, Values: make([]datasource.FacetValue, 0, len(counts))}
	for _, value := range slices.Sorted(maps.Keys(counts)) {
		field.Values = append(field.Values, datasource.FacetValue{Value: value, Count: counts[value]})
	}
	slices.SortStableFunc(field.Values, func(a, b datasource.FacetValue) int { return cmp.Compare(b.Count, a.Count) })
	field.Cardinality = max(cardinality, len(counts))
	if len(field.Values) > limit {
		field.Values, field.Truncated = field.Values[:limit], true
	}
	return field
}

func lineCount(frames []datasource.Frame) int {
	n := 0
	for _, f := range frames {
		n += len(f.Fields[0].Values)
	}
	return n
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
