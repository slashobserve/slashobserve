package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/slashobserve/slashobserve/internal/links"
)

const (
	importTimeout  = 120 * time.Second
	maxImportBytes = 8 << 20
	reasonRunes    = 120
	statusImported = "imported"
	unresolvedCode = "datasources_unresolved"

	alreadyImportedCode = "already_imported"
	standardInput       = "-"
)

type importOptions struct {
	maps    []string
	preview bool
	folder  string
	update  string
	asNew   bool
}

type importRequest struct {
	JSON    string            `json:"json"`
	Mapping map[string]string `json:"mapping,omitempty"`
	Folder  string            `json:"folder,omitempty"`

	UpdateDashboard string `json:"update_dashboard,omitempty"`
	RefuseExisting  bool   `json:"refuse_existing,omitempty"`
}

type importCounts struct {
	Total       int `json:"total"`
	Imported    int `json:"imported"`
	Partial     int `json:"partial"`
	Placeholder int `json:"placeholder"`
	Unverified  int `json:"unverified"`
}

type importPanelLine struct {
	Title      string   `json:"title"`
	SourceType string   `json:"source_type"`
	Status     string   `json:"status"`
	Reasons    []string `json:"reasons"`
}

type importReport struct {
	Counts importCounts      `json:"counts"`
	Panels []importPanelLine `json:"panels"`
}

type importSummary struct {
	Format        string `json:"format"`
	Title         string `json:"title"`
	SchemaVersion int    `json:"schema_version"`
	Panels        int    `json:"panels"`
	Rows          int    `json:"rows"`
	Variables     int    `json:"variables"`
}

type importDatasource struct {
	Key        string   `json:"key"`
	Type       *string  `json:"type"`
	Panels     int      `json:"panels"`
	Match      *string  `json:"match"`
	MatchedBy  *string  `json:"matched_by"`
	Candidates []string `json:"candidates"`
}

type importExisting struct {
	DashboardID string `json:"dashboard_id"`
	Title       string `json:"title"`
	Version     int    `json:"version"`
	CanUpdate   bool   `json:"can_update"`
}

func (e importExisting) nextStep() string {
	if e.CanUpdate {
		return fmt.Sprintf("Run it again with --update %s to save it as version %d, or with --as-new to make another dashboard.", e.DashboardID, e.Version+1)
	}
	return "You cannot edit it; run it again with --as-new to make your own copy."
}

type alreadyImportedBody struct {
	Existing importExisting `json:"existing"`
}

type alreadyImportedError struct {
	api      *apiError
	existing importExisting
}

func (e *alreadyImportedError) Error() string { return e.api.Error() + "\n" + e.existing.nextStep() }

func (e *alreadyImportedError) Unwrap() error { return e.api }

type importPreview struct {
	Summary     importSummary      `json:"summary"`
	Datasources []importDatasource `json:"datasources"`
	Ready       bool               `json:"ready"`
	Report      importReport       `json:"report"`
	Existing    *importExisting    `json:"existing"`
}

type importedDashboard struct {
	Dashboard struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Folder struct {
			Title string `json:"title"`
		} `json:"folder"`
	} `json:"dashboard"`
	Import struct {
		Title  string       `json:"title"`
		Report importReport `json:"report"`
	} `json:"import"`
	FolderNote *string `json:"folder_note"`
	Warning    string  `json:"warning"`
}

type unresolvedBody struct {
	Datasources []importDatasource `json:"datasources"`
}

type unresolvedError struct {
	api         *apiError
	datasources []importDatasource
}

func (e *unresolvedError) Error() string {
	var b strings.Builder
	b.WriteString(e.api.Error())
	b.WriteString("\n")
	b.WriteString(datasourcesTable(e.datasources))
	b.WriteString("\n")
	for _, d := range e.datasources {
		if d.Match != nil {
			continue
		}
		candidates := "-"
		if len(d.Candidates) > 0 {
			candidates = text(strings.Join(d.Candidates, ", "))
		}
		fmt.Fprintf(&b, "Map each one with --map '<grafana>=<uid>'; candidates: %s: %s\n", text(d.Key), candidates)
	}
	return b.String()
}

func (e *unresolvedError) Unwrap() error { return e.api }

func (a *app) importCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [flags] <file>",
		Short: "Import a Grafana dashboard",
		Long:  "Import a Grafana dashboard JSON file ('-' reads standard input) and print the address of the new dashboard.\nThe import report goes to standard error.",
		Example: "  observe import dashboard.json\n" +
			"  observe import dashboard.json --map prometheus=prom\n" +
			"  observe import dashboard.json --preview\n" +
			"  observe import dashboard.json --update <dashboard id>",
		Args: exactArgs(1, "one file, or - for standard input"),
		RunE: a.runImport,
	}
	cmd.Flags().StringArrayVar(&a.importing.maps, "map", nil, "use our datasource <uid> for a Grafana datasource: <grafana>=<uid> (repeatable)")
	cmd.Flags().BoolVar(&a.importing.preview, "preview", false, "show the report without saving anything")
	cmd.Flags().StringVar(&a.importing.folder, "folder", "", "put the dashboard in the folder with this title (default: the folder named in the Grafana export, else your default folder)")
	cmd.Flags().StringVar(&a.importing.update, "update", "", "save the file as the next version of this dashboard, which an earlier import made, instead of making a new one")
	cmd.Flags().BoolVar(&a.importing.asNew, "as-new", false, "make a new dashboard even when this Grafana dashboard was already imported")
	a.addOpenFlag(cmd)
	return cmd
}

func (a *app) runImport(cmd *cobra.Command, args []string) error {
	if a.importing.update != "" && a.importing.asNew {
		return usagef("--update and --as-new cannot be used together")
	}
	mapping, err := parseMappings(a.importing.maps)
	if err != nil {
		return err
	}
	source, err := readImportSource(a.std.In, args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	path, request := a.importRequest(source, mapping)
	data, err := c.withTimeout(importTimeout).post(cmd.Context(), path, request)
	if err != nil {
		return importFailureFrom(err)
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	if a.importing.preview {
		preview, err := decode[importPreview](data)
		if err != nil {
			return err
		}
		printPreview(out, preview)
		return nil
	}
	imported, err := decode[importedDashboard](data)
	if err != nil {
		return err
	}
	address := links.Dashboard(c.base, imported.Dashboard.ID, imported.Dashboard.Title)
	out.line("%s", address)
	verb := "Imported"
	if a.importing.update != "" {
		verb = "Updated"
	}
	out.notice("%s %q: %s", verb, text(imported.Import.Title), imported.Import.Report.Counts.line())
	out.notice("Folder: %s", text(imported.Dashboard.Folder.Title))
	if imported.FolderNote != nil {
		out.warn("%s", text(*imported.FolderNote))
	}
	out.warnOfEdit(imported.Warning)
	if table := imported.Import.Report.attention(); table != "" {
		out.notice("%s", table)
	}
	a.openLink(out, address)
	return nil
}

func (a *app) importRequest(source string, mapping map[string]string) (string, importRequest) {
	if a.importing.preview {
		return "imports/preview", importRequest{JSON: source, Mapping: mapping}
	}
	return "imports", importRequest{JSON: source, Mapping: mapping, Folder: a.importing.folder, UpdateDashboard: a.importing.update, RefuseExisting: !a.importing.asNew}
}

func printPreview(out *output, preview importPreview) {
	out.fields(previewFields(preview))
	if preview.Existing != nil {
		out.line("")
		out.line("Already imported as %q (dashboard %s, version %d). %s", text(preview.Existing.Title), preview.Existing.DashboardID, preview.Existing.Version, preview.Existing.nextStep())
	}
	out.line("")
	out.line("%s", datasourcesTable(preview.Datasources))
	out.line("%s", preview.Report.Counts.line())
	if table := preview.Report.attention(); table != "" {
		out.line("")
		out.line("%s", table)
	}
}

func importFailureFrom(err error) error {
	var api *apiError
	if !errors.As(err, &api) {
		return err
	}
	switch {
	case api.status == 422 && api.code == unresolvedCode:
		if body, decodeErr := decode[unresolvedBody](api.raw); decodeErr == nil {
			return &unresolvedError{api: api, datasources: body.Datasources}
		}
	case api.status == 409 && api.code == alreadyImportedCode:
		if body, decodeErr := decode[alreadyImportedBody](api.raw); decodeErr == nil {
			return &alreadyImportedError{api: api, existing: body.Existing}
		}
	}
	return err
}

func parseMappings(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	mapping := make(map[string]string, len(values))
	for _, value := range values {
		cut := strings.LastIndex(value, "=")
		if cut <= 0 || cut == len(value)-1 {
			return nil, usagef("--map wants <grafana datasource>=<our uid>, got %q", value)
		}
		key, uid := value[:cut], value[cut+1:]
		if _, taken := mapping[key]; taken {
			return nil, usagef("--map names %q twice", key)
		}
		mapping[key] = uid
	}
	return mapping, nil
}

func readImportSource(stdin io.Reader, name string) (string, error) {
	label := text(name)
	reader := stdin
	unreadable := func(err error) error { return fmt.Errorf("read %s: %w", label, err) }
	if name == standardInput {
		label = "standard input"
	} else {
		unreadable = func(err error) error { return usagef("read %s: %v", label, err) }
		file, err := os.Open(name) //nolint:gosec // reading the file the user named is the command's purpose
		if err != nil {
			return "", unreadable(err)
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxImportBytes+1))
	if err != nil {
		return "", unreadable(err)
	}
	if len(data) > maxImportBytes {
		return "", fmt.Errorf("%s is larger than 8 MiB, the import limit", label)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%s is empty", label)
	}
	return string(data), nil
}

func previewFields(p importPreview) [][2]string {
	ready := "no"
	if p.Ready {
		ready = "yes"
	}
	return [][2]string{
		{"Title", orDash(&p.Summary.Title)},
		{"Format", orDash(&p.Summary.Format)},
		{"Schema version", strconv.Itoa(p.Summary.SchemaVersion)},
		{"Panels", strconv.Itoa(p.Summary.Panels)},
		{"Rows", strconv.Itoa(p.Summary.Rows)},
		{"Variables", strconv.Itoa(p.Summary.Variables)},
		{"Ready", ready},
	}
}

func datasourcesTable(datasources []importDatasource) string {
	rows := make([][]string, 0, len(datasources))
	for _, d := range datasources {
		rows = append(rows, []string{text(d.Key), orDash(d.Type), strconv.Itoa(d.Panels), orDash(d.Match), orDash(d.MatchedBy)})
	}
	return tableText([]string{"GRAFANA", "TYPE", "PANELS", "MATCH", "BY"}, rows)
}

func (c importCounts) line() string {
	return fmt.Sprintf("%d panels, %d imported, %d partial, %d placeholders, %d unverified", c.Total, c.Imported, c.Partial, c.Placeholder, c.Unverified)
}

func (r importReport) attention() string {
	var rows [][]string
	for _, panel := range r.Panels {
		if panel.Status == statusImported {
			continue
		}
		reason := ""
		if len(panel.Reasons) > 0 {
			reason = shortReason(panel.Reasons[0])
		}
		rows = append(rows, []string{text(panel.Status), orDash(&panel.Title), orDash(&panel.SourceType), orDash(&reason)})
	}
	if len(rows) == 0 {
		return ""
	}
	return tableText([]string{"STATUS", "PANEL", "TYPE", "REASON"}, rows)
}

func shortReason(reason string) string {
	runes := []rune(text(reason))
	if len(runes) > reasonRunes {
		return string(runes[:reasonRunes]) + "..."
	}
	return string(runes)
}

func tableText(headers []string, rows [][]string) string {
	var b strings.Builder
	(&output{out: &b}).table(headers, rows)
	return strings.TrimRight(b.String(), "\n")
}
