package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/pkg/grafana"
)

const (
	codeDatasourcesUnresolved = "datasources_unresolved"
	codeTooManyImports        = "too_many_imports"
	codeAlreadyImported       = "already_imported"
	codeNotImported           = "not_imported"

	statusImported   grafana.Status = "imported"
	statusUnverified grafana.Status = "unverified"
)

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
}

type importErrorOut struct {
	Error string `json:"error"`
	Code  string `json:"code"`
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
	return failure(errorStep(nameImportGrafana, code), importErrorOut{Error: message, Code: code})
}

func (r *Registry) importGrafana(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, run, scoped, err := r.saveRun(ctx)
	if err != nil {
		return Result{}, err
	}
	var in struct {
		JSON    string            `json:"json"`
		Mapping map[string]string `json:"mapping"`
		Folder  *string           `json:"folder"`

		UpdateDashboard *string `json:"update_dashboard"`
		ImportAsNew     bool    `json:"import_as_new"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if len(in.JSON) > imports.MaxBodyBytes {
		return importFailure(codeTooLarge, fmt.Sprintf("json is larger than %d MiB", imports.MaxBodyBytes>>20)), nil
	}
	if scoped {
		if res, ok, err := r.importRunAllowed(ctx, run, in.JSON); err != nil || !ok {
			return res, err
		}
		if in.UpdateDashboard != nil {
			return invalid("this run creates a new dashboard and may not save a version of an existing one; leave out update_dashboard, or set import_as_new to make another dashboard"), nil
		}
	}
	folder, res, ok := parseImportFolder(in.Folder)
	if !ok {
		return res, nil
	}
	requestID, err := requestIDOf(run, scoped)
	if err != nil {
		return Result{}, err
	}
	req := imports.Request{JSON: []byte(in.JSON), Mapping: in.Mapping, Folder: folder, RequestID: requestID, RefuseExisting: !in.ImportAsNew}
	if in.UpdateDashboard != nil {
		if in.ImportAsNew {
			return invalid("update_dashboard and import_as_new cannot be used together"), nil
		}
		if req.Update, err = db.ParseUUID(*in.UpdateDashboard); err != nil {
			return invalid("update_dashboard must be the id of a dashboard an import made"), nil
		}
	}
	if r.deps.Imports == nil {
		return Result{}, errImportsNotConfigured
	}
	imported, err := r.deps.Imports.Import(ctx, principal, req)
	if err != nil {
		return r.importFailed(err, scoped)
	}
	step := "Imported " + plural(imported.Record.Report.Counts.Total, "panel", "panels") + " from Grafana"
	switch {
	case imported.Replayed:
		step = "Imported again"
	case imported.Updated:
		step = fmt.Sprintf("Updated the dashboard from Grafana to version %d", imported.Version.Number)
	}
	return success(step, newImportedOut(r.deps.PublicURL, imported)), nil
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
	}
	envelope, err := json.Marshal(out)
	if err != nil {
		return out
	}
	out.Attention, out.Truncated = fit(attentionOf(imported.Record.Report), listBudget-len(envelope))
	return out
}

func alreadyImportedMessage(e imports.Existing, scoped bool) string {
	if scoped {
		return fmt.Sprintf("this Grafana dashboard is already imported as %q (version %d); tell the person where it is, or set import_as_new true to make another dashboard", e.Title, e.Version)
	}
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

func (r *Registry) importFailed(err error, scoped bool) (Result, error) {
	var input *grafana.InputError
	var unresolved *imports.UnresolvedError
	var already *imports.AlreadyImportedError
	switch {
	case errors.As(err, &already):
		return failure(errorStep(nameImportGrafana, codeAlreadyImported), alreadyImportedOut{
			importErrorOut: importErrorOut{Error: alreadyImportedMessage(already.Existing, scoped), Code: codeAlreadyImported},
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
			importErrorOut: importErrorOut{Error: "map each Grafana datasource to one of ours in mapping", Code: codeDatasourcesUnresolved},
			Datasources:    rows,
			Truncated:      truncated,
		}), nil
	case errors.Is(err, imports.ErrBusy):
		return importFailure(codeTooManyImports, "another import is running; try again shortly"), nil
	}
	if res, ok := createFolderFailure(err); ok {
		return res, nil
	}
	return saveFailure(err)
}
