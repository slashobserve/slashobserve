package elasticsearch

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	maxFacetFields     = 20
	defaultFacetValues = 10
	cardinalitySuffix  = "__card"
	unknownFacetKind   = "unknown"
)

var (
	_ datasource.LogFacetSource = (*Datasource)(nil)

	defaultFacetKinds = []FieldKind{KindKeyword, KindText, KindBoolean, KindIP, KindNumber}
	countableKinds    = []FieldKind{KindKeyword, KindText, KindBoolean, KindIP, KindNumber, KindFlattened}
)

type facetTarget struct {
	name string
	kind FieldKind
	agg  string
}

func (t facetTarget) countable() bool { return t.agg != "" }

func (d *Datasource) LogFacets(ctx context.Context, req datasource.FacetRequest) (datasource.Facets, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.Facets{}, err
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	prep, err := d.prepare(ctx, req.Query, req.Range, instantInterval(req.Range, req.Query.MinInterval), lim)
	if err != nil {
		return datasource.Facets{}, err
	}
	if prep.user.aggs() != nil {
		return datasource.Facets{}, datasource.Errorf(datasource.ErrInvalidQuery, "facets need a KQL, Lucene or filter-only query body")
	}
	roles, err := prep.mapping.roles(d.opts.Fields)
	if err != nil {
		return datasource.Facets{}, err
	}
	targets, truncated := facetTargets(prep.mapping, roles, req.Fields)
	facets := datasource.Facets{Fields: []datasource.FacetField{}, Truncated: truncated}
	counted := slices.DeleteFunc(slices.Clone(targets), func(t facetTarget) bool { return !t.countable() })
	var resp searchResponse
	if len(counted) > 0 {
		raw, err := d.search(ctx, facetsBody(prep, d.opts.TimeField, req.Range, lim, counted, facetValues(req.Limit)))
		if err != nil {
			return datasource.Facets{}, err
		}
		if resp, err = parseSearch(raw); err != nil {
			return datasource.Facets{}, err
		}
	}
	for _, t := range targets {
		field := datasource.FacetField{Name: t.name, Kind: string(t.kind), Values: []datasource.FacetValue{}}
		if t.countable() {
			if err := fillFacetField(&field, resp); err != nil {
				return datasource.Facets{}, err
			}
		}
		facets.Fields = append(facets.Fields, field)
	}
	return facets, nil
}

func facetValues(limit int) int {
	if limit < 1 {
		return defaultFacetValues
	}
	return limit
}

func facetsBody(prep prepared, timeField string, r datasource.TimeRange, lim datasource.Limits, targets []facetTarget, size int) map[string]any {
	body := countingBody(prep, timeField, r, time.Second, lim)
	aggs := make(map[string]any, 2*len(targets))
	for _, t := range targets {
		aggs[t.name] = map[string]any{"terms": map[string]any{"field": t.agg, "size": size}}
		aggs[t.name+cardinalitySuffix] = map[string]any{"cardinality": map[string]any{"field": t.agg}}
	}
	body["aggs"] = aggs
	return body
}

func facetTargets(m *Mapping, roles FieldRoles, requested []string) (targets []facetTarget, truncated bool) {
	if requested != nil {
		for _, name := range requested {
			target, ok := facetTargetOf(m, name)
			if !ok {
				target = facetTarget{name: name, kind: unknownFacetKind}
			}
			targets = append(targets, target)
		}
		return targets, false
	}
	candidates := []string{roles.Level, roles.Service}
	for _, name := range m.Names() {
		if slices.Contains(defaultFacetKinds, m.Fields[name].Kind) {
			candidates = append(candidates, name)
		}
	}
	counted := map[string]bool{}
	for _, name := range candidates {
		target, ok := facetTargetOf(m, name)
		if !ok || !target.countable() || counted[target.agg] {
			continue
		}
		counted[target.agg] = true
		targets = append(targets, target)
	}
	if len(targets) > maxFacetFields {
		return targets[:maxFacetFields], true
	}
	return targets, false
}

func facetTargetOf(m *Mapping, name string) (facetTarget, bool) {
	f, ok := m.Fields[name]
	if !ok {
		return facetTarget{}, false
	}
	target := facetTarget{name: name, kind: f.Kind}
	if f.Conflict || !slices.Contains(countableKinds, f.Kind) {
		return target, true
	}
	if agg, err := m.termsField(name); err == nil {
		target.agg = agg
	}
	return target, true
}

type termsAggregation struct {
	DocCountErrorUpperBound int64 `json:"doc_count_error_upper_bound"`
	SumOtherDocCount        int64 `json:"sum_other_doc_count"`
	Buckets                 []struct {
		Key         json.RawMessage `json:"key"`
		KeyAsString string          `json:"key_as_string"`
		DocCount    int64           `json:"doc_count"`
	} `json:"buckets"`
}

func fillFacetField(field *datasource.FacetField, resp searchResponse) error {
	var terms termsAggregation
	var distinct struct {
		Value float64 `json:"value"`
	}
	termsRaw, hasTerms := resp.Aggregations[field.Name]
	cardRaw, hasCard := resp.Aggregations[field.Name+cardinalitySuffix]
	if !hasTerms || !hasCard {
		return nil
	}
	if json.Unmarshal(termsRaw, &terms) != nil || json.Unmarshal(cardRaw, &distinct) != nil {
		return unreadable()
	}
	for _, b := range terms.Buckets {
		value := b.KeyAsString
		if value == "" {
			value = keyText(b.Key)
		}
		field.Values = append(field.Values, datasource.FacetValue{Value: value, Count: b.DocCount})
	}
	field.Exact = terms.DocCountErrorUpperBound == 0 && !resp.partial()
	field.Cardinality = int(distinct.Value)
	field.Truncated = terms.SumOtherDocCount > 0
	return nil
}
