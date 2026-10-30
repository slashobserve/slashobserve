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
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	defaultListDashboards = 20
	maxListDashboards     = 50
	maxStepCodes          = 3

	roleDeniedMessage = "your role does not allow changing this dashboard"

	codeInvalidSpec     = "invalid_spec"
	codeVersionConflict = "version_conflict"
	codeNotFound        = "not_found"

	defaultCreateSummary = "Create dashboard"
	defaultUpdateSummary = "Update dashboard"
)

var (
	errInvalidRun = errors.New("invalid run scope")
)

func (r *Registry) dashboardEntries() []entry {
	return []entry{
		{Tool{Name: nameListDashboards, Description: descListDashboards, InputSchema: json.RawMessage(schemaListDashboards), ReadOnly: true, MinRole: auth.RoleViewer}, r.listDashboards},
		{Tool{Name: nameGetDashboard, Description: descGetDashboard, InputSchema: json.RawMessage(schemaGetDashboard), ReadOnly: true, MinRole: auth.RoleViewer}, r.getDashboard},
		{Tool{Name: nameCreateDashboard, Description: descCreateDashboard, InputSchema: json.RawMessage(schemaCreateDashboard), MinRole: auth.RoleEditor}, r.createDashboard},
		{Tool{Name: nameUpdateDashboard, Description: descUpdateDashboard, InputSchema: json.RawMessage(schemaUpdateDashboard), MinRole: auth.RoleEditor}, r.updateDashboard},
	}
}

type dashboardItemOut struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Tags          []string `json:"tags"`
	LatestVersion int      `json:"latest_version"`
	UpdatedAt     string   `json:"updated_at"`
	URL           string   `json:"url"`
	Folder        string   `json:"folder"`
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
	principal, _ := auth.FromContext(ctx)
	var (
		items []dashboardItemOut
		next  string
	)
	for {
		page, err := r.deps.Dashboards.List(ctx, principal, dashboards.ListParams{Limit: limit, Cursor: in.Cursor, SkipTotal: true})
		if errors.Is(err, dashboards.ErrInvalidCursor) {
			return invalid("cursor is not valid"), nil
		}
		if err != nil {
			return Result{}, err
		}
		items, next = r.dashboardItems(page.Items), page.NextCursor
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

func (r *Registry) dashboardItems(list []dashboards.ListItem) []dashboardItemOut {
	items := make([]dashboardItemOut, len(list))
	for i, d := range list {
		items[i] = dashboardItemOut{
			ID:            d.ID,
			Title:         d.Title,
			Tags:          nonNilStrings(d.Tags),
			LatestVersion: d.LatestVersion,
			UpdatedAt:     d.UpdatedAt.UTC().Format(time.RFC3339),
			URL:           DashboardURL(r.deps.PublicURL, d.ID),
			Folder:        d.Folder.Title,
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
	id, err := db.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, invalid("dashboard_id must be a UUID from list_dashboards or create_dashboard"), false
	}
	return id, Result{}, true
}

type dashboardOut struct {
	DashboardID              string          `json:"dashboard_id"`
	Version                  int             `json:"version"`
	Title                    string          `json:"title"`
	URL                      string          `json:"url"`
	Folder                   string          `json:"folder"`
	Spec                     *spec.Dashboard `json:"spec"`
	Unverified               *unverifiedOut  `json:"unverified,omitempty"`
	ImportAttention          []Attention     `json:"import_attention,omitempty"`
	ImportAttentionTruncated bool            `json:"import_attention_truncated,omitempty"`
}

type unverifiedOut struct {
	Panels    []unverifiedPanelOut    `json:"panels"`
	Variables []unverifiedVariableOut `json:"variables"`
}

type unverifiedPanelOut struct {
	PanelID string       `json:"panel_id"`
	Title   string       `json:"title"`
	Issues  []spec.Issue `json:"issues"`
}

type unverifiedVariableOut struct {
	Name   string       `json:"name"`
	Issues []spec.Issue `json:"issues"`
}

func newUnverifiedOut(d *spec.Dashboard, u dashboards.Unverified) *unverifiedOut {
	if u.Empty() {
		return nil
	}
	out := &unverifiedOut{Panels: make([]unverifiedPanelOut, len(u.Panels)), Variables: make([]unverifiedVariableOut, len(u.Variables))}
	for i, p := range u.Panels {
		title, _ := panelTitleIn(d, p.PanelID)
		out.Panels[i] = unverifiedPanelOut{PanelID: p.PanelID, Title: title, Issues: p.Issues}
	}
	for i, v := range u.Variables {
		out.Variables[i] = unverifiedVariableOut{Name: v.Name, Issues: v.Issues}
	}
	return out
}

func panelTitleIn(d *spec.Dashboard, id string) (string, bool) {
	entry, ok := d.FindPanel(id)
	if !ok {
		return "", false
	}
	return entry.Panel.Title, true
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
	if res, ok, err := r.viewableDashboard(ctx, id, "dashboard or version not found"); err != nil || !ok {
		return res, err
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
	out := dashboardOut{DashboardID: dash.ID, Version: ver.Number, Title: ver.Spec.Title, URL: DashboardURL(r.deps.PublicURL, dash.ID), Folder: dash.Folder.Title, Spec: ver.Spec, Unverified: newUnverifiedOut(ver.Spec, ver.Unverified)}
	if out.ImportAttention, out.ImportAttentionTruncated, err = r.importAttention(ctx, id, ver.Number); err != nil {
		return Result{}, err
	}
	body, err := json.Marshal(out)
	if err != nil {
		return Result{}, err
	}
	if len(body) > MaxModelSpecBytes {
		return toolError(codeTooLarge, fmt.Sprintf("the spec is %d bytes, over the %d byte tool result limit", len(body), MaxModelSpecBytes)), nil
	}
	return success("Read version "+strconv.Itoa(ver.Number), out), nil
}

func (r *Registry) importAttention(ctx context.Context, id pgtype.UUID, version int) ([]Attention, bool, error) {
	if r.deps.Imports == nil {
		return nil, false, nil
	}
	record, err := r.deps.Imports.ForDashboard(ctx, id)
	if errors.Is(err, imports.ErrNotFound) || errors.Is(err, dashboards.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if record.Version != version {
		return nil, false, nil
	}
	rows, truncated := fit(ConversionAttention(record.Report), listBudget)
	return rows, truncated, nil
}

type panelOut struct {
	ID    string         `json:"id"`
	Title string         `json:"title"`
	Type  spec.PanelType `json:"type"`
}

type savedOut struct {
	DashboardID      string     `json:"dashboard_id"`
	Version          int        `json:"version"`
	URL              string     `json:"url"`
	Replayed         *bool      `json:"replayed,omitempty"`
	Panels           []panelOut `json:"panels"`
	Truncated        bool       `json:"truncated,omitempty"`
	UnverifiedPanels []string   `json:"unverified_panels,omitempty"`
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
	Spec     json.RawMessage `json:"spec"`
	Summary  string          `json:"summary"`
	FolderID string          `json:"folder_id"`
}

func (r *Registry) saveRun(ctx context.Context) (auth.Principal, Run, bool, error) {
	principal, ok := auth.FromContext(ctx)
	if !ok {
		return auth.Principal{}, Run{}, false, errors.New("no principal in context")
	}
	run, scoped := RunFrom(ctx)
	return principal, run, scoped, nil
}

func parseFolderID(raw string) (pgtype.UUID, Result, bool) {
	if raw == "" {
		return pgtype.UUID{}, Result{}, true
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, invalid("folder_id must be a UUID from list_folders"), false
	}
	return id, Result{}, true
}

func validateRun(run Run) (pgtype.UUID, error) {
	switch run.Mode {
	case ModeCreate:
		return pgtype.UUID{}, nil
	case ModeRevise:
		id, err := db.ParseUUID(run.DashboardID)
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("%w: revise needs a dashboard UUID", errInvalidRun)
		}
		if run.ParentVersion < 1 {
			return pgtype.UUID{}, fmt.Errorf("%w: revise needs a parent version of at least 1", errInvalidRun)
		}
		return id, nil
	}
	return pgtype.UUID{}, fmt.Errorf("%w: unknown mode %q", errInvalidRun, string(run.Mode))
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
	if scoped {
		if run.Mode == ModeSuggest {
			return invalid("this run proposes a change to panel %s; it cannot create dashboards", run.PanelID), nil
		}
		if _, err := validateRun(run); err != nil {
			return Result{}, err
		}
		if run.Mode == ModeRevise {
			return invalid("this run revises dashboard %s; call update_dashboard", run.DashboardID), nil
		}
	}
	folderID, res, ok := parseFolderID(in.FolderID)
	if !ok {
		return res, nil
	}
	requestID, err := requestIDOf(run, scoped)
	if err != nil {
		return Result{}, err
	}
	if parsed, err := spec.Parse(in.Spec); err == nil {
		if issues := importOnlyIssues(parsed, nil); len(issues) > 0 {
			return importOnlyFailure(issues), nil
		}
	}
	dash, ver, replayed, err := r.deps.Dashboards.CreateRequestDashboard(ctx, principal, folderID, in.Spec, summaryOr(in.Summary, defaultCreateSummary), requestID)
	if res, ok := createFolderFailure(err); ok {
		return res, nil
	}
	if err != nil {
		return saveFailure(err)
	}
	return success("Saved version "+strconv.Itoa(ver.Number), r.savedOut(dash, ver, &replayed)), nil
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
	if scoped && run.Mode == ModeSuggest {
		return r.suggestPanel(ctx, principal, run, in.Spec)
	}
	target, res, ok, err := updateTarget(in, run, scoped)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return res, nil
	}
	requestID, err := requestIDOf(run, scoped)
	if err != nil {
		return Result{}, err
	}
	if res, ok, err := r.guardImportOnly(ctx, in.Spec, target); err != nil {
		return Result{}, err
	} else if !ok {
		return res, nil
	}
	dash, ver, replayed, err := r.deps.Dashboards.AddRequestVersion(ctx, principal, target.id, target.parent, in.Spec, summaryOr(in.Summary, defaultUpdateSummary), requestID)
	if err != nil {
		return saveFailure(err)
	}
	return success("Saved version "+strconv.Itoa(ver.Number), r.savedOut(dash, ver, &replayed)), nil
}

func (r *Registry) guardImportOnly(ctx context.Context, raw json.RawMessage, target saveTarget) (Result, bool, error) {
	parsed, err := spec.Parse(raw)
	if err != nil || !needsImportOnlyCheck(parsed) {
		return Result{}, true, nil
	}
	_, parentVersion, err := r.deps.Dashboards.GetVersion(ctx, target.id, target.parent)
	if errors.Is(err, dashboards.ErrNotFound) {
		return Result{}, true, nil
	}
	if err != nil {
		return Result{}, false, err
	}
	parent := parentVersion.Spec
	if parent == nil {
		parent = &spec.Dashboard{}
	}
	if issues := importOnlyIssues(parsed, parent); len(issues) > 0 {
		return importOnlyFailure(issues), false, nil
	}
	return Result{}, true, nil
}

func importOnlyFailure(issues []spec.Issue) Result {
	step := "Save rejected: " + plural(len(issues), "issue", "issues") + issueCodes(issues)
	kept, truncated := fit(slices.Clone(issues), listBudget)
	return failure(step, invalidSpecOut{Code: codeInvalidSpec, Issues: kept, Truncated: truncated})
}

func requestIDOf(run Run, scoped bool) (pgtype.UUID, error) {
	if !scoped || run.RequestID == "" {
		return pgtype.UUID{}, nil
	}
	id, err := db.ParseUUID(run.RequestID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("%w: the request id is not a UUID", errInvalidRun)
	}
	return id, nil
}

type saveTarget struct {
	id     pgtype.UUID
	parent int
}

func updateTarget(in saveInput, run Run, scoped bool) (saveTarget, Result, bool, error) {
	var parent int
	if in.ParentVersion != nil {
		if *in.ParentVersion < 1 || *in.ParentVersion > math.MaxInt32 {
			return saveTarget{}, invalid("parent_version must be between 1 and %d", math.MaxInt32), false, nil
		}
		parent = *in.ParentVersion
	}
	if !scoped {
		switch {
		case in.DashboardID == "":
			return saveTarget{}, invalid("dashboard_id is required"), false, nil
		case in.ParentVersion == nil:
			return saveTarget{}, invalid("parent_version is required"), false, nil
		}
		id, res, ok := parseDashboardID(in.DashboardID)
		return saveTarget{id: id, parent: parent}, res, ok, nil
	}
	runID, err := validateRun(run)
	if err != nil {
		return saveTarget{}, Result{}, false, err
	}
	if run.Mode == ModeCreate {
		return saveTarget{}, invalid("this run creates a new dashboard; call create_dashboard"), false, nil
	}
	if in.DashboardID != "" {
		id, res, ok := parseDashboardID(in.DashboardID)
		if !ok {
			return saveTarget{}, res, false, nil
		}
		if id != runID {
			return saveTarget{}, invalid("this run revises dashboard %s; dashboard_id must match it", run.DashboardID), false, nil
		}
	}
	if in.ParentVersion == nil {
		parent = run.ParentVersion
	}
	return saveTarget{id: runID, parent: parent}, Result{}, true, nil
}

func summaryOr(summary, fallback string) string {
	if strings.TrimSpace(summary) == "" {
		return fallback
	}
	return summary
}

func (r *Registry) savedOut(dash dashboards.Dashboard, ver dashboards.Version, replayed *bool) savedOut {
	entries := ver.Spec.AllPanels()
	panels := make([]panelOut, len(entries))
	for i, e := range entries {
		panels[i] = panelOut{ID: e.Panel.ID, Title: e.Panel.Title, Type: e.Panel.Type}
	}
	panels, truncated := fit(panels, listBudget)
	var unverified []string
	for _, p := range ver.Unverified.Panels {
		unverified = append(unverified, p.PanelID)
	}
	return savedOut{
		DashboardID:      dash.ID,
		Version:          ver.Number,
		URL:              DashboardURL(r.deps.PublicURL, dash.ID),
		Replayed:         replayed,
		Panels:           panels,
		Truncated:        truncated,
		UnverifiedPanels: unverified,
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
	case errors.Is(err, dashboards.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, dashboards.ErrFolderNotFound):
		return failure("Save rejected: dashboard not found", errorBody{Code: codeNotFound}), nil
	case errors.Is(err, folders.ErrForbidden):
		return failure("Save rejected: "+codeForbiddenRole, errorBody{Code: codeForbiddenRole, Message: roleDeniedMessage}), nil
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
