package tempo

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const ServiceTag = serviceTag

var _ datasource.ServiceSource = (*Datasource)(nil)

func (d *Datasource) Services(ctx context.Context, req datasource.ServiceRequest) (datasource.ServiceList, error) {
	signal, err := datasource.ResolveServiceSignal(d.Type(), req.Signal)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	ctx, cancel := datasource.Bound(ctx, discoverTimeout)
	defer cancel()
	values, err := d.tagValues(ctx, serviceTag, "", req.Range)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	kept, _ := datasource.FilterSortLimit(values, "", req.Limit)
	return datasource.ServiceList{Signal: signal, Field: serviceTag, Values: kept}, nil
}
