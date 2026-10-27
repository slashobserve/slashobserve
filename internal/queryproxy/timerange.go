package queryproxy

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const MaxRange = 400 * 24 * time.Hour

func PanelRange(p *spec.Panel, rng datasource.TimeRange, now time.Time) datasource.TimeRange {
	if p.TimeFrom != "" {
		if from, err := spec.ResolveTime(p.TimeFrom, now, false); err == nil {
			rng = datasource.TimeRange{From: from, To: now.UTC()}
		}
	}
	if p.TimeShift != "" {
		if back, err := spec.ResolveTime("now-"+p.TimeShift, now, false); err == nil {
			shift := now.UTC().Sub(back)
			rng = datasource.TimeRange{From: rng.From.Add(-shift), To: rng.To.Add(-shift)}
		}
	}
	return rng
}
