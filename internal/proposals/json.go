package proposals

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type authorJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SummaryJSON struct {
	ID             string      `json:"id"`
	DashboardID    string      `json:"dashboard_id"`
	State          string      `json:"state"`
	Title          string      `json:"title"`
	Revision       int         `json:"revision"`
	BaseVersion    int         `json:"base_version"`
	Summary        string      `json:"summary"`
	Author         authorJSON  `json:"author"`
	Origin         *string     `json:"origin"`
	OriginClient   *string     `json:"origin_client"`
	RunID          *string     `json:"run_id"`
	CreatedAt      string      `json:"created_at"`
	UpdatedAt      string      `json:"updated_at"`
	ClosedAt       *string     `json:"closed_at"`
	ClosedBy       *authorJSON `json:"closed_by"`
	MergedVersion  *int        `json:"merged_version"`
	Behind         bool        `json:"behind"`
	LatestVersion  int         `json:"latest_version"`
	AllowedActions []string    `json:"allowed_actions"`
}

type ProposalJSON struct {
	SummaryJSON
	Spec      *spec.Dashboard   `json:"spec"`
	Changes   []spec.UnitChange `json:"changes"`
	Conflicts []spec.Conflict   `json:"conflicts"`
}

type listJSON struct {
	Proposals  []SummaryJSON `json:"proposals"`
	NextCursor *string       `json:"next_cursor"`
}

type changedJSON struct {
	Error    string `json:"error"`
	Code     string `json:"code"`
	Revision int    `json:"revision"`
}

func newAuthorJSON(a Author) authorJSON {
	return authorJSON(a)
}

func timeText(t *time.Time) *string {
	if t == nil {
		return nil
	}
	return httpapi.OptionalString(httpapi.FormatTime(*t))
}

func NewSummaryJSON(i Item) SummaryJSON {
	actions := make([]string, len(i.AllowedActions))
	for n, a := range i.AllowedActions {
		actions[n] = string(a)
	}
	out := SummaryJSON{
		ID:             i.ID,
		DashboardID:    i.DashboardID,
		State:          string(i.State),
		Title:          i.Title,
		Revision:       i.Revision,
		BaseVersion:    i.BaseVersion,
		Summary:        i.Summary,
		Author:         newAuthorJSON(i.Author),
		Origin:         httpapi.OptionalString(i.Origin),
		OriginClient:   httpapi.OptionalString(i.OriginClient),
		RunID:          httpapi.OptionalString(i.RunID),
		CreatedAt:      httpapi.FormatTime(i.CreatedAt),
		UpdatedAt:      httpapi.FormatTime(i.UpdatedAt),
		ClosedAt:       timeText(i.ClosedAt),
		MergedVersion:  i.MergedVersion,
		Behind:         i.Behind,
		LatestVersion:  i.LatestVersion,
		AllowedActions: actions,
	}
	if i.ClosedBy != nil {
		closer := newAuthorJSON(*i.ClosedBy)
		out.ClosedBy = &closer
	}
	return out
}

func NewProposalJSON(p Proposal) ProposalJSON {
	return ProposalJSON{SummaryJSON: NewSummaryJSON(p.Item), Spec: p.Spec, Changes: p.Changes, Conflicts: p.Conflicts}
}
