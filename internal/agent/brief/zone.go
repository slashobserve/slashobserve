package brief

import (
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const windowAdvice = "take your window from that time and state the window you queried."

func zoneAt(name string, at time.Time) *tools.BriefZone {
	loc, err := spec.LoadZone(name)
	if name == "" || err != nil {
		return nil
	}
	_, offset := at.In(loc).Zone()
	return &tools.BriefZone{Label: tools.ZoneLabel(loc, at), OffsetSeconds: offset}
}

func clockNote(name string, at time.Time) string {
	utc := at.UTC().Format("2006-01-02 15:04")
	zone := zoneAt(name, at)
	if zone == nil {
		return fmt.Sprintf("It is now %s UTC; %s", utc, windowAdvice)
	}
	local := at.In(time.FixedZone(zone.Label, zone.OffsetSeconds))
	note := fmt.Sprintf("Time zone: the person reads times in %s, labelled %s. Tool results give UTC; write every time in the answer in %s with its label, such as 14:05 %s, not in UTC. It is now %s %s on %s", name, zone.Label, name, zone.Label, local.Format("15:04"), zone.Label, local.Format("2006-01-02"))
	if zone.Label == "UTC" {
		return note + "; " + windowAdvice
	}
	return fmt.Sprintf("%s, which is %s UTC; %s", note, utc, windowAdvice)
}
