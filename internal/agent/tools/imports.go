package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/pkg/grafana"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	codeDatasourcesUnresolved = "datasources_unresolved"
	codeTooManyImports        = "too_many_imports"
	codeImportContent         = "import_content_not_allowed"
	codeAlreadyImported       = "already_imported"
	codeNotImported           = "not_imported"
	codeGrafanaUIDMismatch    = "grafana_uid_mismatch"

	statusImported   grafana.Status = "imported"
	statusUnverified grafana.Status = "unverified"
)

const maxImportIssuesShown = 5

var errImportsNotConfigured = errors.New("imports are not configured")

func init() { registerEntries((*Registry).importEntries) }

func (r *Registry) importEntries() []entry {
	return []entry{
		{Tool{Name: nameImportGrafana, Description: descImportGrafana, InputSchema: json.RawMessage(schemaImportGrafana), MinRole: auth.RoleViewer}, r.importGrafana},
	}
}

type Attention struct {
	PanelID string         `json:"panel_id"`
	Title   string         `json:"title"`
	Status  grafana.Status `json:"status"`
	Reason  string         `json:"reason"`
}

type importedOut struct {
	DashboardID string         `json:"dashboard_id"`
	Version     int            `json:"version"`
	URL         string         `json:"url"`
	Title       string         `json:"title"`
	Folder      string         `json:"folder"`
	Replayed    bool           `json:"replayed"`
	Updated     bool           `json:"updated"`
	Counts      grafana.Counts `json:"counts"`
	Attention   []Attention    `json:"attention"`
	Truncated   bool           `json:"truncated"`
	FolderNote  *string        `json:"folder_note,omitempty"`
	Warning     string         `json:"warning,omitempty"`
}

type importErrorOut struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

type existingOut struct {
	DashboardID string `json:"dashboard_id"`
	Title       string `json:"title"`
	Version     int    `json:"version"`
	URL         string `json:"url"`
	CanUpdate   bool   `json:"can_update"`
}

type alreadyImportedOut struct {
	importErrorOut
	Existing existingOut `json:"existing"`
}

type unresolvedRowOut struct {
	Key        string   `json:"key"`
	Name       *string  `json:"name"`
	Type       *string  `json:"type"`
	Kind       *string  `json:"kind"`
	Panels     int      `json:"panels"`
	Candidates []string `json:"candidates"`
}

type unresolvedOut struct {
	importErrorOut
	Datasources []unresolvedRowOut `json:"datasources"`
	Truncated   bool               `json:"truncated,omitempty"`
}

func attentionOf(report grafana.Report) []Attention {
	out := []Attention{}
	for _, line := range report.Panels {
		if line.Status == statusImported {
			continue
		}
		row := Attention{PanelID: line.PanelID, Title: line.Title, Status: line.Status}
		if len(line.Reasons) > 0 {
			row.Reason = line.Reasons[0]
		}
		out = append(out, row)
	}
	return out
}

func ConversionAttention(report grafana.Report) []Attention {
	return slices.DeleteFunc(attentionOf(report), func(a Attention) bool { return a.Status == statusUnverified })
}

func unresolvedRows(list []grafana.SourceDatasource) []unresolvedRowOut {
	rows := []unresolvedRowOut{}
	for _, s := range list {
		if s.Kind == nil || s.Match != nil {
			continue
		}
		rows = append(rows, unresolvedRowOut{Key: s.Key, Name: s.Name, Type: s.Type, Kind: s.Kind, Panels: s.Panels, Candidates: s.Candidates})
	}
	return rows
}

func importFailure(code, message string) Result {
	return failure(errorStep(nameImportGrafana, code), importErrorOut{Message: message, Code: code})
}

type importArgs struct {
	JSON    string            `json:"json"`
	Mapping map[string]string `json:"mapping"`
	Folder  *string           `json:"folder"`

	UpdateDashboard *string `json:"update_dashboard"`
	ImportAsNew     bool    `json:"import_as_new"`
}

func (in importArgs) request(folder string, requestID pgtype.UUID, scoped bool) (imports.Request, Result, bool) {
	req := imports.Request{JSON: []byte(in.JSON), Mapping: in.Mapping, Folder: folder, RequestID: requestID, RefuseExisting: !in.ImportAsNew && !scoped}
	if in.UpdateDashboard == nil {
		return req, Result{}, true
	}
	if in.ImportAsNew {
		return req, invalid("update_dashboard and import_as_new cannot be used together"), false
	}
	id, err := db.ParseUUID(*in.UpdateDashboard)
	if err != nil {
		return req, invalid("update_dashboard must be the id of a dashboard an import made"), false
	}
	req.Update = id
	return req, Result{}, true
}

func (r *Registry) importScope(ctx context.Context, run Run, scoped bool, in importArgs) (Result, bool, error) {
	if !scoped {
		return Result{}, true, nil
	}
	if res, ok, err := r.importRunAllowed(ctx, run, in.JSON); err != nil || !ok {
		return res, false, err
	}
	if in.UpdateDashboard != nil {
		return invalid("this run creates a new dashboard and may not save a version of an existing one; leave out update_dashboard, or set import_as_new to make another dashboard"), false, nil
	}
	return Result{}, true, nil
}

func importStep(imported imports.Imported) string {
	switch {
	case imported.Replayed:
		return "Imported again"
	case imported.Updated:
		return fmt.Sprintf("Updated the dashboard from Grafana to version %d", imported.Version.Number)
	}
	return "Imported " + plural(imported.Record.Report.Counts.Total, "panel", "panels") + " from Grafana"
}

func (r *Registry) importGrafana(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, run, scoped, err := r.saveRun(ctx)
	if err != nil {
		return Result{}, err
	}
	var in importArgs
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if len(in.JSON) > imports.MaxBodyBytes {
		return importFailure(codeTooLarge, fmt.Sprintf("json is larger than %d MiB", imports.MaxBodyBytes>>20)), nil
	}
	if res, ok, err := r.importScope(ctx, run, scoped, in); err != nil || !ok {
		return res, err
	}
	folder, res, ok := parseImportFolder(in.Folder)
	if !ok {
		return res, nil
	}
	requestID, err := requestIDOf(run, scoped)
	if err != nil {
		return Result{}, err
	}
	req, res, ok := in.request(folder, requestID, scoped)
	if !ok {
		return res, nil
	}
	if r.deps.Imports == nil {
		return Result{}, errImportsNotConfigured
	}
	converted, err := r.deps.Imports.Preview(ctx, principal, req)
	if err != nil {
		return r.importFailed(ctx, err)
	}
	if res, refused := importContentRefusal(converted.Spec); refused {
		return res, nil
	}
	imported, err := r.deps.Imports.Import(ctx, principal, req)
	if err != nil {
		return r.importFailed(ctx, err)
	}
	return success(importStep(imported), newImportedOut(r.deps.PublicURL, imported)), nil
}

func importContentRefusal(converted *spec.Dashboard) (Result, bool) {
	if converted == nil {
		return Result{}, false
	}
	issues := importOnlyIssues(converted, nil)
	if len(issues) == 0 {
		return Result{}, false
	}
	shown := issues[:min(len(issues), maxImportIssuesShown)]
	lines := make([]string, len(shown))
	for i, issue := range shown {
		lines[i] = issue.Path + ": " + string(issue.Code)
	}
	if more := len(issues) - len(shown); more > 0 {
		lines = append(lines, fmt.Sprintf("%d more", more))
	}
	message := fmt.Sprintf("this Grafana JSON converts to content the agent cannot save (%s); remove those panels from the JSON, or ask a person to import it in the web app", strings.Join(lines, "; "))
	return importFailure(codeImportContent, message), true
}

func (r *Registry) importRunAllowed(ctx context.Context, run Run, jsonText string) (Result, bool, error) {
	if run.Mode == ModeSuggest {
		return invalid("this run proposes a panel change; importing is not available here"), false, nil
	}
	if _, err := validateRun(run); err != nil {
		return Result{}, false, err
	}
	if run.Mode == ModeRevise {
		return invalid("this run revises dashboard %s; importing creates a new dashboard and is not available here", run.DashboardID), false, nil
	}
	if run.Build {
		found, err := r.hasBuild(ctx, run)
		if err != nil {
			return Result{}, false, err
		}
		if found {
			return invalid("%s", importInBuildRun), false, nil
		}
	}
	if text := strings.TrimSpace(jsonText); text == "" || !strings.Contains(run.Prompt, text) {
		return invalid("the request does not contain this Grafana JSON; build the dashboard with create_dashboard instead"), false, nil
	}
	return Result{}, true, nil
}

func newImportedOut(publicURL *url.URL, imported imports.Imported) importedOut {
	out := importedOut{
		DashboardID: imported.Dashboard.ID,
		Version:     imported.Version.Number,
		URL:         links.Dashboard(publicURL, imported.Dashboard.ID, imported.Dashboard.Title),
		Title:       imported.Dashboard.Title,
		Folder:      imported.Dashboard.Folder.Title,
		Replayed:    imported.Replayed,
		Updated:     imported.Updated,
		Counts:      imported.Record.Report.Counts,
		Attention:   []Attention{},
		FolderNote:  httpapi.OptionalString(imported.FolderNote),
		Warning:     imported.Dashboard.ManagedBy.EditWarning(),
	}
	envelope, err := json.Marshal(out)
	if err != nil {
		return out
	}
	out.Attention, out.Truncated = fit(attentionOf(imported.Record.Report), listBudget-len(envelope))
	return out
}

func alreadyImportedMessage(e imports.Existing) string {
	if e.CanUpdate {
		return fmt.Sprintf("this Grafana dashboard is already imported as %q (version %d); set this tool's update_dashboard argument to %s to save this JSON as its next version, or import_as_new true to make another dashboard", e.Title, e.Version, e.DashboardID)
	}
	return fmt.Sprintf("this Grafana dashboard is already imported as %q (version %d) and you cannot edit it; pass import_as_new true to make your own copy", e.Title, e.Version)
}

func parseImportFolder(raw *string) (string, Result, bool) {
	if raw == nil {
		return "", Result{}, true
	}
	title, err := folders.CleanTitle(*raw)
	if err != nil {
		return "", invalid("folder: %s", err.Error()), false
	}
	return title, Result{}, true
}

func (r *Registry) importFailed(ctx context.Context, err error) (Result, error) {
	var input *grafana.InputError
	var unresolved *imports.UnresolvedError
	var already *imports.AlreadyImportedError
	var mismatch *imports.UIDMismatchError
	switch {
	case errors.As(err, &mismatch):
		return importFailure(codeGrafanaUIDMismatch, mismatch.Error()), nil
	case errors.As(err, &already):
		return failure(errorStep(nameImportGrafana, codeAlreadyImported), alreadyImportedOut{
			importErrorOut: importErrorOut{Message: alreadyImportedMessage(already.Existing), Code: codeAlreadyImported},
			Existing: existingOut{
				DashboardID: already.Existing.DashboardID,
				Title:       already.Existing.Title,
				Version:     already.Existing.Version,
				URL:         links.Dashboard(r.deps.PublicURL, already.Existing.DashboardID, already.Existing.Title),
				CanUpdate:   already.Existing.CanUpdate,
			},
		}), nil
	case errors.Is(err, imports.ErrNotFound):
		return importFailure(codeNotImported, "that dashboard was not made by an import; change it with update_dashboard"), nil
	case errors.As(err, &input):
		return importFailure(input.Code, input.Message), nil
	case errors.As(err, &unresolved):
		rows, truncated := fit(unresolvedRows(unresolved.Datasources), listBudget)
		return failure(errorStep(nameImportGrafana, codeDatasourcesUnresolved), unresolvedOut{
			importErrorOut: importErrorOut{Message: "map each Grafana datasource to one of ours in mapping", Code: codeDatasourcesUnresolved},
			Datasources:    rows,
			Truncated:      truncated,
		}), nil
	case errors.Is(err, imports.ErrBusy):
		return importFailure(codeTooManyImports, "another import is running; try again shortly"), nil
	}
	if res, ok := createFolderFailure(err); ok {
		return res, nil
	}
	return saveFailure(ctx, err)
}
