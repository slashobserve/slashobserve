package queryproxy

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var DraftLimits = datasource.Limits{Timeout: 15 * time.Second, MaxSeries: 200, MaxPoints: 100_000, MaxRows: 1000}

const DraftGrace = 5 * time.Second
