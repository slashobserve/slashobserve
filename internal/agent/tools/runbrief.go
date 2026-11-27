package tools

import (
	"encoding/json"
	"time"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type RunBrief struct {
	Request     BriefRequest      `json:"request"`
	Current     *BriefRevision    `json:"current"`
	Build       *BriefBuild       `json:"build"`
	Datasources []BriefDatasource `json:"datasources"`
	Withheld    bool              `json:"withheld"`
	Guide       BriefGuide        `json:"guide"`
	Draft       json.RawMessage   `json:"draft"`
}

type BriefRequest struct {
	Mode          Mode         `json:"mode"`
	Source        string       `json:"source"`
	Prompt        string       `json:"prompt"`
	PanelID       *string      `json:"panel_id"`
	From          *time.Time   `json:"from"`
	To            *time.Time   `json:"to"`
	TimeZone      *string      `json:"time_zone"`
	AsOf          *time.Time   `json:"as_of"`
	ContinueNote  *string      `json:"continue_note"`
	ThreadContext *string      `json:"thread_context"`
	Continues     bool         `json:"continues"`
	Attempt       int          `json:"attempt"`
	Investigation bool         `json:"investigation"`
	Pointer       BriefPointer `json:"pointer"`
}

type BriefPointer struct {
	Anchor        *anchor.Anchor `json:"anchor"`
	BlockID       *string        `json:"block_id"`
	ObservationID *string        `json:"observation_id"`
	ThreadAsk     bool           `json:"thread_ask"`
}

type BriefRevision struct {
	DashboardID string                `json:"dashboard_id"`
	Number      int                   `json:"number"`
	Spec        *spec.Dashboard       `json:"spec"`
	Unverified  dashboards.Unverified `json:"unverified"`
	Conversion  []Attention           `json:"conversion"`
}

type BriefBuild struct {
	BaseVersion int                   `json:"base_version"`
	Spec        *spec.Dashboard       `json:"spec"`
	Unverified  dashboards.Unverified `json:"unverified"`
	Progress    BuildProgress         `json:"progress"`
}

type BriefDatasource struct {
	datasource.Status
	Everyone bool `json:"everyone"`
}

type BriefGuide struct {
	Text   string          `json:"text"`
	Schema json.RawMessage `json:"schema"`
}
