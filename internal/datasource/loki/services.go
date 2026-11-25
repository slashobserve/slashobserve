package loki

import (
	"context"
	"net/url"
	"slices"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var ServiceLabels = datasource.ServiceLabels

var _ datasource.ServiceSource = (*Datasource)(nil)

func (d *Datasource) Services(ctx context.Context, req datasource.ServiceRequest) (datasource.ServiceList, error) {
	signal, err := datasource.ResolveServiceSignal(d.Type(), req.Signal)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	params := url.Values{"start": {lokiTime(req.Range.From)}, "end": {lokiTime(req.Range.To)}}
	label := ServiceLabels[0]
	values, err := d.getStrings(ctx, labelValuesPath(label), params)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	if len(values) == 0 {
		if label, values, err = d.otherServiceLabel(ctx, params); err != nil {
			return datasource.ServiceList{}, err
		}
	}
	return datasource.ServiceList{Signal: signal, Field: label, Values: filterSortLimit(values, "", req.Limit)}, nil
}

func (d *Datasource) otherServiceLabel(ctx context.Context, params url.Values) (string, []string, error) {
	names, err := d.getStrings(ctx, "/loki/api/v1/labels", params)
	if err != nil {
		return "", nil, err
	}
	for _, label := range ServiceLabels[1:] {
		if !slices.Contains(names, label) {
			continue
		}
		values, err := d.getStrings(ctx, labelValuesPath(label), params)
		if err != nil || len(values) > 0 {
			return label, values, err
		}
	}
	return ServiceLabels[0], nil, nil
}
