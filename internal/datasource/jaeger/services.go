package jaeger

import (
	"context"

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
	values, err := d.services(ctx)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	kept, _ := datasource.FilterSortLimit(values, "", req.Limit)
	return datasource.ServiceList{Signal: signal, Field: ServiceKey, Values: kept}, nil
}
