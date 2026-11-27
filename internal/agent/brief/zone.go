package brief

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func zoneAt(name string, at time.Time) *tools.BriefZone {
	loc, err := spec.LoadZone(name)
	if name == "" || err != nil {
		return nil
	}
	_, offset := at.In(loc).Zone()
	return &tools.BriefZone{Label: tools.ZoneLabel(loc, at), OffsetSeconds: offset}
}
