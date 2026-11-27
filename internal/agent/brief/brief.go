package brief

import (
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var clock = time.Now

type Task struct {
	Mode          tools.Mode
	Source        revisions.Source
	Prompt        string
	PanelID       string
	From, To      *time.Time
	ContinueNote  string
	ThreadContext string
	Continues     bool
	Attempt       int
	Investigation bool
	Observations  []string
	TimeZone      string
	AsOf          *time.Time
	Withheld      bool
	Pointer       Pointer
}

type Pointer struct {
	Anchor        *anchor.Anchor
	BlockID       string
	ObservationID string
	ThreadAsk     bool
}

type View struct {
	Current *Revision
	Build   *dashboards.Build
}

type Revision struct {
	DashboardID string
	Number      int
	Spec        *spec.Dashboard
	Unverified  dashboards.Unverified
	Conversion  []tools.Attention
}

func (t Task) now() time.Time {
	if t.AsOf != nil {
		return *t.AsOf
	}
	return clock()
}

func guideTypes(statuses []datasource.Status) []spec.DatasourceType {
	var types []spec.DatasourceType
	for _, s := range statuses {
		for _, t := range prompts.GuideTypes(s.Type, s.Source == datasource.SourceBuiltin) {
			if !slices.Contains(types, t) {
				types = append(types, t)
			}
		}
	}
	return types
}
