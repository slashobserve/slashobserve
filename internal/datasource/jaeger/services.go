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
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	values, err := d.services(ctx)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	return datasource.ServiceList{Signal: signal, Field: ServiceKey, Values: filterSortLimit(values, "", req.Limit)}, nil
}
