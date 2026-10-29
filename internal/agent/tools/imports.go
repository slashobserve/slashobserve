package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/pkg/grafana"
)

const (
	codeDatasourcesUnresolved = "datasources_unresolved"
	codeTooManyImports        = "too_many_imports"

	statusImported   grafana.Status = "imported"
	statusUnverified grafana.Status = "unverified"
)

var errImportsNotConfigured = errors.New("imports are not configured")

func (r *Registry) importEntries() []entry {
	return []entry{
		{Tool{Name: nameImportGrafana, Description: descImportGrafana, InputSchema: json.RawMessage(schemaImportGrafana)}, r.importGrafana},
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
	Replayed    bool           `json:"replayed"`
	Counts      grafana.Counts `json:"counts"`
	Attention   []Attention    `json:"attention"`
	Truncated   bool           `json:"truncated"`
}

type importErrorOut struct {
	Error string `json:"error"`
	Code  string `json:"code"`
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
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if len(in.JSON) > imports.MaxBodyBytes {
		return importFailure(codeTooLarge, fmt.Sprintf("json is larger than %d MiB", imports.MaxBodyBytes>>20)), nil
	}
	if scoped {
		if run.Mode == ModeSuggest {
			return invalid("this run proposes a panel change; importing is not available here"), nil
		}
		if _, err := validateRun(run); err != nil {
			return Result{}, err
		}
		if run.Mode == ModeRevise {
			return invalid("this run revises dashboard %s; importing creates a new dashboard and is not available here", run.DashboardID), nil
		}
		if text := strings.TrimSpace(in.JSON); text == "" || !strings.Contains(run.Prompt, text) {
			return invalid("the request does not contain this Grafana JSON; build the dashboard with create_dashboard instead"), nil
		}
	}
	requestID, err := requestIDOf(run, scoped)
	if err != nil {
		return Result{}, err
	}
	if r.deps.Imports == nil {
		return Result{}, errImportsNotConfigured
	}
	imported, err := r.deps.Imports.Import(ctx, principal, imports.Request{JSON: []byte(in.JSON), Mapping: in.Mapping, RequestID: requestID})
	if err != nil {
		return r.importFailed(err)
	}
	step := "Imported " + plural(imported.Record.Report.Counts.Total, "panel", "panels") + " from Grafana"
	if imported.Replayed {
		step = "Imported again"
	}
	attention, truncated := fit(attentionOf(imported.Record.Report), listBudget)
	return success(step, importedOut{
		DashboardID: imported.Dashboard.ID,
		Version:     imported.Version.Number,
		URL:         DashboardURL(r.deps.PublicURL, imported.Dashboard.ID),
		Title:       imported.Dashboard.Title,
		Replayed:    imported.Replayed,
		Counts:      imported.Record.Report.Counts,
		Attention:   attention,
		Truncated:   truncated,
	}), nil
}

func (r *Registry) importFailed(err error) (Result, error) {
	var input *grafana.InputError
	var unresolved *imports.UnresolvedError
	switch {
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
	return saveFailure(err)
}
