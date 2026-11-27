package elasticsearch

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
	m, err := d.Mapping(ctx)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	roles, err := m.roles(d.opts.Fields)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	list := datasource.ServiceList{Signal: signal, Field: roles.Service, Values: []string{}}
	if roles.Service == "" {
		return list, nil
	}
	field, err := m.termsField(roles.Service)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	values, _, err := d.termsValues(ctx, field, req.Range, req.Limit, nil)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	kept, _ := datasource.FilterSortLimit(values, "", req.Limit)
	list.Values = kept
	return list, nil
}
