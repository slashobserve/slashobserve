package brief

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func Structured(statuses []datasource.Status, everyone func(uid string) bool, t Task, v View, draft json.RawMessage) tools.RunBrief {
	out := tools.RunBrief{
		Request:     structuredRequest(t),
		Datasources: make([]tools.BriefDatasource, len(statuses)),
		Withheld:    t.Withheld,
		Guide:       tools.BriefGuide{Text: prompts.GuideFor(guideTypes(statuses)), Schema: prompts.Schema()},
	}
	for i, s := range statuses {
		out.Datasources[i] = tools.BriefDatasource{Status: s, Everyone: everyone != nil && everyone(s.UID)}
	}
	if c := v.Current; c != nil {
		out.Current = &tools.BriefRevision{DashboardID: c.DashboardID, Number: c.Number, Spec: c.Spec, Unverified: c.Unverified, Conversion: slices.Clone(c.Conversion)}
	}
	if b := v.Build; b != nil {
		p := b.Progress()
		out.Build = &tools.BriefBuild{
			BaseVersion: b.BaseVersion, Spec: b.Spec, Unverified: b.Unverified,
			Progress: tools.BuildProgress{Saves: b.Saves, Panels: p.Panels, PlannedPanels: p.PlannedPanels},
		}
	}
	if t.Mode == tools.ModeSuggest && len(draft) > 0 {
		out.Draft = slices.Clone(draft)
	}
	return out
}

func structuredRequest(t Task) tools.BriefRequest {
	return tools.BriefRequest{
		Mode: t.Mode, Source: string(t.Source), Prompt: t.Prompt, PanelID: nonEmpty(t.PanelID),
		From: inUTC(t.From), To: inUTC(t.To), TimeZone: nonEmpty(t.TimeZone), AsOf: inUTC(t.AsOf),
		ContinueNote: nonEmpty(t.ContinueNote), ThreadContext: nonEmpty(t.ThreadContext), Continues: t.Continues, Attempt: t.Attempt,
		Investigation: t.Investigation,
		Pointer: tools.BriefPointer{
			Anchor: t.Pointer.Anchor, BlockID: nonEmpty(t.Pointer.BlockID), ObservationID: nonEmpty(t.Pointer.ObservationID), ThreadAsk: t.Pointer.ThreadAsk,
		},
	}
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func inUTC(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	at := t.UTC()
	return &at
}
