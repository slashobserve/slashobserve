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
	local := at.In(loc)
	return fmt.Sprintf("Time zone: the person reads times in %s, labelled %s. Tool results give UTC; write every time in the answer in %s with its label, such as 14:05 %s, not in UTC. It is now %s %s on %s, which is %s UTC; take your window from that time and state the window you queried.", zone, label, zone, label, local.Format("15:04"), label, local.Format("2006-01-02"), at.UTC().Format("2006-01-02 15:04"))
}
