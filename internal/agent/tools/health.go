package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/health"
)

const (
	nameListDashboardHealth = "list_dashboard_health"

	defaultListHealth   = 20
	maxListHealth       = 50
	maxProblemTextBytes = 500

	descListDashboardHealth = "List dashboards with health problems in folders you can see: panels whose queries failed or returned no data over the last 7 days at the last daily check, variables that failed, duplicate titles in a folder, and dashboards nobody opened in 90 days. Failing queries or variables come first, then no data, duplicate titles and unopened dashboards; within each, the most recently opened first. The first page carries summary, the count of dashboards per problem in the folders asked for whatever the kind, and each dashboard carries its latest repair. Fix failing panels with get_panels and update_panels. Page with next_cursor."
)

var schemaListDashboardHealth = `{"type":"object","properties":{` +
	`"folder_id":{"type":"string","format":"uuid","description":"Only dashboards in this folder, from list_folders."},` +
	`"kind":{"type":"string","enum":` + healthKindsJSON() + `,"description":"Only this kind of problem."},` +
	`"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Dashboards per page. Default 20."},` +
	`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`

var errHealthNotConfigured = errors.New("dashboard health is not configured")

func healthKindsJSON() string {
	raw, err := json.Marshal(health.Kinds)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func (r *Registry) healthEntries() []entry {
	return []entry{
		{Tool{Name: nameListDashboardHealth, Description: descListDashboardHealth, InputSchema: json.RawMessage(schemaListDashboardHealth), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.listDashboardHealth},
	}
}

type listHealthInput struct {
	FolderID string `json:"folder_id"`
	Kind     string `json:"kind"`
	Limit    *int   `json:"limit"`
	Cursor   string `json:"cursor"`
}

type healthProblemOut struct {
	Kind       health.Kind `json:"kind"`
	PanelID    string      `json:"panel_id,omitempty"`
	PanelTitle string      `json:"panel_title,omitempty"`
	Variable   string      `json:"variable,omitempty"`
	Code       string      `json:"code,omitempty"`
	Message    string      `json:"message,omitempty"`
}

type healthItemOut struct {
	DashboardID       string             `json:"dashboard_id"`
	Title             string             `json:"title"`
	URL               string             `json:"url"`
	FolderID          string             `json:"folder_id"`
	FolderTitle       string             `json:"folder_title"`
	CheckedAt         string             `json:"checked_at,omitempty"`
	CheckedVersion    *int               `json:"checked_version,omitempty"`
	Skipped           string             `json:"skipped,omitempty"`
	Problems          []healthProblemOut `json:"problems"`
	ProblemsTruncated bool               `json:"problems_truncated,omitempty"`
	Repair            *healthRepairOut   `json:"repair,omitempty"`
}

type healthRepairOut struct {
	State   string `json:"state"`
	Version *int   `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type healthSummaryOut struct {
	Dashboards      int `json:"dashboards"`
	Failing         int `json:"failing"`
	QueryFailed     int `json:"query_failed"`
	VariableFailed  int `json:"variable_failed"`
	NoData          int `json:"no_data"`
	DuplicateTitles int `json:"duplicate_titles"`
	Unopened        int `json:"unopened"`
}

type healthJobOut struct {
	LastStartedAt  string `json:"last_started_at,omitempty"`
	LastFinishedAt string `json:"last_finished_at,omitempty"`
	LastOutcome    string `json:"last_outcome,omitempty"`
	Running        bool   `json:"running"`
}

type healthListOut struct {
	Dashboards []healthItemOut   `json:"dashboards"`
	NextCursor *string           `json:"next_cursor"`
	Summary    *healthSummaryOut `json:"summary,omitempty"`
	Job        healthJobOut      `json:"job"`
}

func (r *Registry) listDashboardHealth(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Health == nil {
		return Result{}, errHealthNotConfigured
	}
	var in listHealthInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	params, res, ok := healthParams(in)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	for {
		page, err := r.deps.Health.List(ctx, principal, params)
		switch {
		case errors.Is(err, health.ErrFolderNotFound):
			return toolError(codeNotFound, "folder not found"), nil
		case errors.Is(err, health.ErrInvalidCursor):
			return invalid("cursor is not valid"), nil
		case err != nil:
			return Result{}, err
		}
		out, smaller, retry := healthPage(page)
		if !retry {
			return success("Listed "+plural(len(out.Dashboards), "unhealthy dashboard", "unhealthy dashboards"), out), nil
		}
		params.Limit = smaller
	}
}

func healthParams(in listHealthInput) (health.ListParams, Result, bool) {
	limit, res, ok := boundedInt(in.Limit, defaultListHealth, 1, maxListHealth, "limit")
	if !ok {
		return health.ListParams{}, res, false
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return health.ListParams{}, res, false
	}
	p := health.ListParams{Cursor: in.Cursor, Limit: limit}
	if in.FolderID != "" {
		id, err := db.ParseUUID(in.FolderID)
		if err != nil {
			return health.ListParams{}, invalid("folder_id must be a UUID from list_folders"), false
		}
		p.FolderID = id
	}
	if in.Kind != "" {
		kind, valid := health.ValidKind(in.Kind)
		if !valid {
			return health.ListParams{}, invalid("kind must be one of %s", healthKindList()), false
		}
		p.Kind = kind
	}
	return p, Result{}, true
}

func healthKindList() string {
	names := make([]string, len(health.Kinds))
	for i, k := range health.Kinds {
		names[i] = string(k)
	}
	return strings.Join(names, ", ")
}

func healthPage(page health.Page) (healthListOut, int, bool) {
	out := healthListOut{Dashboards: make([]healthItemOut, 0, len(page.Items)), Job: healthJobOf(page.Job)}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	if page.Summary != nil {
		out.Summary = (*healthSummaryOut)(page.Summary)
	}
	budget := listBudget - jsonSize(out)
	for _, it := range page.Items {
		out.Dashboards = append(out.Dashboards, healthItemOf(it))
	}
	kept, cut := fit(out.Dashboards, budget)
	switch {
	case cut && len(out.Dashboards) > 1:
		return out, max(len(kept), 1), true
	case cut:
		out.Dashboards[0] = trimProblems(out.Dashboards[0], budget)
	}
	return out, 0, false
}

func trimProblems(it healthItemOut, budget int) healthItemOut {
	problems := it.Problems
	it.Problems, it.ProblemsTruncated = []healthProblemOut{}, true
	it.Problems, _ = fit(problems, budget-jsonSize(it))
	return it
}

func healthItemOf(it health.Item) healthItemOut {
	out := healthItemOut{
		DashboardID:    it.ID,
		Title:          it.Title,
		URL:            it.URL,
		FolderID:       it.Folder.ID.String(),
		FolderTitle:    it.Folder.Title,
		CheckedAt:      rfc3339(it.CheckedAt),
		CheckedVersion: it.CheckedVersion,
		Skipped:        it.Skipped,
		Problems:       make([]healthProblemOut, 0, len(it.Problems)),
	}
	for _, p := range it.Problems {
		out.Problems = append(out.Problems, healthProblemOut{
			Kind:       p.Kind,
			PanelID:    p.PanelID,
			PanelTitle: clip(p.PanelTitle, maxProblemTextBytes),
			Variable:   p.Variable,
			Code:       p.Code,
			Message:    clip(p.Message, maxProblemTextBytes),
		})
	}
	if r := it.Repair; r != nil {
		out.Repair = &healthRepairOut{State: r.State, Version: r.Version}
		if r.Error != nil {
			out.Repair.Error = clip(*r.Error, maxProblemTextBytes)
		}
	}
	return out
}

func healthJobOf(j health.JobStatus) healthJobOut {
	return healthJobOut{
		LastStartedAt:  rfc3339(j.LastStartedAt),
		LastFinishedAt: rfc3339(j.LastFinishedAt),
		LastOutcome:    j.LastOutcome,
		Running:        j.Running,
	}
}

func rfc3339(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
