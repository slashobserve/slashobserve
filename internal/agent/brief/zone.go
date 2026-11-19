package brief

import (
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func zoneNote(zone string, at time.Time) string {
	loc, err := spec.LoadZone(zone)
	if zone == "" || err != nil {
		return ""
	}
	label := tools.ZoneLabel(loc, at)
	return fmt.Sprintf("Time zone: the person reads times in %s, labelled %s. Tool results give UTC; write every time in the answer in %s with its label, such as 14:05 %s, not in UTC.", zone, label, zone, label)
}
