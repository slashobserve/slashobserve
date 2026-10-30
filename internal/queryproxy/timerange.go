package queryproxy

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const MaxRange = 400 * 24 * time.Hour

func PanelRange(p *spec.Panel, rng datasource.TimeRange, now time.Time, opts spec.TimeOptions) datasource.TimeRange {
	end := now.Add(-opts.NowDelay).UTC()
	if p.TimeFrom != "" {
		if from, err := spec.ResolveTime(p.TimeFrom, now, false, opts); err == nil {
			rng = datasource.TimeRange{From: from, To: end}
		}
	}
	if p.TimeShift != "" {
		if back, err := spec.ResolveTime("now-"+p.TimeShift, now, false, opts); err == nil {
			shift := end.Sub(back)
			rng = datasource.TimeRange{From: rng.From.Add(-shift), To: rng.To.Add(-shift)}
		}
	}
	return rng
}
