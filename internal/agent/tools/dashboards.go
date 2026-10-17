package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	defaultListDashboards = 20
	maxListDashboards     = 50
	uuidTextBytes         = 36
	maxStepCodes          = 3

	codeInvalidSpec     = "invalid_spec"
	codeVersionConflict = "version_conflict"
	codeNotFound        = "not_found"

	defaultCreateSummary = "Create dashboard"
	defaultUpdateSummary = "Update dashboard"
)

var errRequestIDUnsupported = errors.New("request-id saves need plan 036")

func (r *Registry) dashboardEntries() []entry {
	return []entry{
		{Tool{Name: nameListDashboards, Description: descListDashboards, InputSchema: json.RawMessage(schemaListDashboards), ReadOnly: true}, r.listDashboards},
		{Tool{Name: nameGetDashboard, Description: descGetDashboard, InputSchema: json.RawMessage(schemaGetDashboard), ReadOnly: true}, r.getDashboard},
		{Tool{Name: nameCreateDashboard, Description: descCreateDashboard, InputSchema: json.RawMessage(schemaCreateDashboard)}, r.createDashboard},
		{Tool{Name: nameUpdateDashboard, Description: descUpdateDashboard, InputSchema: json.RawMessage(schemaUpdateDashboard)}, r.updateDashboard},
	}
}

type dashboardItemOut struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Tags          []string `json:"tags"`
	LatestVersion int      `json:"latest_version"`
	UpdatedAt     string   `json:"updated_at"`
	URL           string   `json:"url"`
}

type dashboardListOut struct {
	Dashboards []dashboardItemOut `json:"dashboards"`
	NextCursor *string            `json:"next_cursor"`
}

func (r *Registry) listDashboards(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Limit  *int   `json:"limit"`
		Cursor string `json:"cursor"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	limit, res, ok := boundedInt(in.Limit, defaultListDashboards, 1, maxListDashboards, "limit")
	if !ok {
		return res, nil
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return res, nil
	}
	var (
		items []dashboardItemOut
		next  string
	)
	for {
		list, cursor, err := r.deps.Dashboards.List(ctx, limit, in.Cursor)
		if errors.Is(err, dashboards.ErrInvalidCursor) {
			return invalid("cursor is not valid"), nil
		}
		if err != nil {
			return Result{}, err
		}
		items, next = r.dashboardItems(list), cursor
		kept, cut := fit(items, listBudget)
		if !cut || limit <= 1 {
			break
		}
		limit = max(len(kept), 1)
	}
	out := dashboardListOut{Dashboards: items}
	if next != "" {
		out.NextCursor = &next
	}
	return success("Listed "+plural(len(items), "dashboard", "dashboards"), out), nil
}

func (r *Registry) dashboardItems(list []dashboards.Dashboard) []dashboardItemOut {
	items := make([]dashboardItemOut, len(list))
	for i, d := range list {
		items[i] = dashboardItemOut{
			ID:            d.ID,
			Title:         d.Title,
			Tags:          nonNilStrings(d.Tags),
			LatestVersion: d.LatestVersion,
			UpdatedAt:     d.UpdatedAt.UTC().Format(time.RFC3339),
			URL:           DashboardURL(r.deps.PublicURL, d.ID),
		}
	}
	return items
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func parseDashboardID(raw string) (pgtype.UUID, Result, bool) {
	var id pgtype.UUID
	if len(raw) != uuidTextBytes || id.Scan(raw) != nil {
		return id, invalid("dashboard_id must be a UUID from list_dashboards or create_dashboard"), false
	}
	return id, Result{}, true
}

type dashboardOut struct {
	DashboardID string          `json:"dashboard_id"`
	Version     int             `json:"version"`
	Title       string          `json:"title"`
	URL         string          `json:"url"`
	Spec        *spec.Dashboard `json:"spec"`
}

func (r *Registry) getDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		DashboardID string `json:"dashboard_id"`
		Version     *int   `json:"version"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseDashboardID(in.DashboardID)
	if !ok {
		return res, nil
	}
	version, res, ok := boundedInt(in.Version, 0, 1, math.MaxInt32, "version")
	if !ok {
		return res, nil
	}
	var (
		dash dashboards.Dashboard
		ver  dashboards.Version
		err  error
	)
	if in.Version == nil {
		dash, ver, err = r.deps.Dashboards.Get(ctx, id)
	} else {
		dash, ver, err = r.deps.Dashboards.GetVersion(ctx, id, version)
	}
	if errors.Is(err, dashboards.ErrNotFound) {
		return toolError(codeNotFound, "dashboard or version not found"), nil
	}
	if err != nil {
		return Result{}, err
	}
	out := dashboardOut{DashboardID: dash.ID, Version: ver.Number, Title: ver.Spec.Title, URL: DashboardURL(r.deps.PublicURL, dash.ID), Spec: ver.Spec}
	body, err := json.Marshal(out)
	if err != nil {
		return Result{}, err
	}
	if len(body) > maxContentBytes {
		return toolError(codeTooLarge, fmt.Sprintf("the spec is %d bytes, over the %d byte tool result limit", len(body), maxContentBytes)), nil
	}
	return success("Read version "+strconv.Itoa(ver.Number), out), nil
}

type panelOut struct {
	ID    string         `json:"id"`
	Title string         `json:"title"`
	Type  spec.PanelType `json:"type"`
}

type savedOut struct {
	DashboardID string     `json:"dashboard_id"`
	Version     int        `json:"version"`
	URL         string     `json:"url"`
	Replayed    *bool      `json:"replayed,omitempty"`
	Panels      []panelOut `json:"panels"`
	Truncated   bool       `json:"truncated,omitempty"`
}

type invalidSpecOut struct {
	Code      string       `json:"code"`
	Issues    []spec.Issue `json:"issues"`
	Truncated bool         `json:"truncated,omitempty"`
}

type conflictOut struct {
	Code          string `json:"code"`
	LatestVersion int    `json:"latest_version"`
}

type saveInput struct {
	DashboardID   string          `json:"dashboard_id"`
	ParentVersion *int            `json:"parent_version"`
	Spec          json.RawMessage `json:"spec"`
	Summary       string          `json:"summary"`
}

type createInput struct {
	Spec    json.RawMessage `json:"spec"`
	Summary string          `json:"summary"`
}

func (r *Registry) saveRun(ctx context.Context) (auth.Principal, Run, bool, error) {
	principal, ok := auth.FromContext(ctx)
	if !ok {
		return auth.Principal{}, Run{}, false, errors.New("no principal in context")
	}
	run, scoped := RunFrom(ctx)
	if scoped && run.RequestID != "" {
		return auth.Principal{}, Run{}, false, errRequestIDUnsupported
	}
	return principal, run, scoped, nil
}

func checkSpecPresent(raw json.RawMessage) (Result, bool) {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return invalid("spec is required; see get_spec_guide"), false
	}
	return Result{}, true
}

func (r *Registry) createDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, run, scoped, err := r.saveRun(ctx)
	if err != nil {
		return Result{}, err
	}
	var in createInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkSpecPresent(in.Spec); !ok {
		return res, nil
	}
	if scoped && run.Mode == ModeRevise {
		return invalid("this run revises dashboard %s; call update_dashboard", run.DashboardID), nil
	}
	dash, ver, err := r.deps.Dashboards.Create(ctx, principal, in.Spec, summaryOr(in.Summary, defaultCreateSummary))
	if err != nil {
		return saveFailure(err)
	}
	return success("Saved version "+strconv.Itoa(ver.Number), r.savedOut(dash, ver, nil)), nil
}

func (r *Registry) updateDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, run, scoped, err := r.saveRun(ctx)
	if err != nil {
		return Result{}, err
	}
	var in saveInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkSpecPresent(in.Spec); !ok {
		return res, nil
	}
	rawID, parent, res, ok := updateTarget(in, run, scoped)
	if !ok {
		return res, nil
	}
	id, res, ok := parseDashboardID(rawID)
	if !ok {
		return res, nil
	}
	dash, ver, replayed, err := r.deps.Dashboards.AddVersion(ctx, principal, id, parent, in.Spec, summaryOr(in.Summary, defaultUpdateSummary))
	if err != nil {
		return saveFailure(err)
	}
	return success("Saved version "+strconv.Itoa(ver.Number), r.savedOut(dash, ver, &replayed)), nil
}

func updateTarget(in saveInput, run Run, scoped bool) (id string, parent int, res Result, ok bool) {
	id = in.DashboardID
	if in.ParentVersion != nil {
		if *in.ParentVersion < 1 || *in.ParentVersion > math.MaxInt32 {
			return "", 0, invalid("parent_version must be between 1 and %d", math.MaxInt32), false
		}
		parent = *in.ParentVersion
	}
	if !scoped {
		switch {
		case id == "":
			return "", 0, invalid("dashboard_id is required"), false
		case in.ParentVersion == nil:
			return "", 0, invalid("parent_version is required"), false
		}
		return id, parent, Result{}, true
	}
	if run.Mode == ModeCreate {
		return "", 0, invalid("this run creates a new dashboard; call create_dashboard"), false
	}
	if id != "" && id != run.DashboardID {
		return "", 0, invalid("this run revises dashboard %s; dashboard_id must match it", run.DashboardID), false
	}
	if in.ParentVersion == nil {
		parent = run.ParentVersion
	}
	return run.DashboardID, parent, Result{}, true
}

func summaryOr(summary, fallback string) string {
	if strings.TrimSpace(summary) == "" {
		return fallback
	}
	return summary
}

func (r *Registry) savedOut(dash dashboards.Dashboard, ver dashboards.Version, replayed *bool) savedOut {
	panels := make([]panelOut, len(ver.Spec.Panels))
	for i, p := range ver.Spec.Panels {
		panels[i] = panelOut{ID: p.ID, Title: p.Title, Type: p.Type}
	}
	panels, truncated := fit(panels, listBudget)
	return savedOut{
		DashboardID: dash.ID,
		Version:     ver.Number,
		URL:         DashboardURL(r.deps.PublicURL, dash.ID),
		Replayed:    replayed,
		Panels:      panels,
		Truncated:   truncated,
	}
}

func saveFailure(err error) (Result, error) {
	var invalidSpec *dashboards.InvalidSpecError
	var conflict *dashboards.ConflictError
	switch {
	case errors.As(err, &invalidSpec):
		issues, truncated := fit(slices.Clone(invalidSpec.Issues), listBudget)
		step := "Save rejected: " + plural(len(invalidSpec.Issues), "issue", "issues") + issueCodes(invalidSpec.Issues)
		return failure(step, invalidSpecOut{Code: codeInvalidSpec, Issues: issues, Truncated: truncated}), nil
	case errors.As(err, &conflict):
		return failure("Save rejected: version conflict", conflictOut{Code: codeVersionConflict, LatestVersion: conflict.Latest}), nil
	case errors.Is(err, dashboards.ErrNotFound):
		return failure("Save rejected: dashboard not found", errorBody{Code: codeNotFound}), nil
	}
	return Result{}, err
}

func issueCodes(issues []spec.Issue) string {
	var codes []string
	for _, issue := range issues {
		if !slices.Contains(codes, string(issue.Code)) {
			codes = append(codes, string(issue.Code))
		}
		if len(codes) == maxStepCodes {
			break
		}
	}
	if len(codes) == 0 {
		return ""
	}
	return " (" + strings.Join(codes, ", ") + ")"
}
