package prometheus

import (
	"context"
	"net/url"
	"slices"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var _ datasource.ServiceSource = (*Datasource)(nil)

func (d *Datasource) Services(ctx context.Context, req datasource.ServiceRequest) (datasource.ServiceList, error) {
	signal, err := datasource.ResolveServiceSignal(d.Type(), req.Signal)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	ctx, cancel := datasource.Bound(ctx, discoverTimeout)
	defer cancel()
	form := url.Values{"start": {unixSeconds(req.Range.From)}, "end": {unixSeconds(req.Range.To)}}
	names, err := d.getStrings(ctx, "/api/v1/labels", form)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	for _, label := range datasource.ServiceLabels {
		if !slices.Contains(names, label) {
			continue
		}
		values, err := d.getStrings(ctx, "/api/v1/label/"+url.PathEscape(label)+"/values", form)
		if err != nil {
			return datasource.ServiceList{}, err
		}
		if len(values) > 0 {
			shown, _ := datasource.FilterSortLimit(values, "", req.Limit)
			return datasource.ServiceList{Signal: signal, Field: label, Values: shown}, nil
		}
	}
	return datasource.ServiceList{Signal: signal}, nil
}
