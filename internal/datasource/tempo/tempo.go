package tempo

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/otlphttp"
)

type Options = otlphttp.Options

type Datasource struct {
	opts Options
	http *otlphttp.Client
}

var _ datasource.Datasource = (*Datasource)(nil)

func New(opts Options) (*Datasource, error) {
	c, err := otlphttp.New("tempo", opts)
	if err != nil {
		return nil, err
	}
	return &Datasource{opts: c.Options(), http: c}, nil
}

func (d *Datasource) Type() string { return "tempo" }

func (d *Datasource) Close() error { return d.http.Close() }

func (d *Datasource) CheckHealth(ctx context.Context) error {
	ctx, cancel := datasource.Bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	_, err := d.get(ctx, "/ready", nil)
	return err
}
