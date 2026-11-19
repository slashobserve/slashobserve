package clickhouse

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var _ datasource.LagSource = (*Datasource)(nil)

func (d *Datasource) TailLag() time.Duration { return datasource.DefaultTailLag }
