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
)

const (
	importTimeout  = 120 * time.Second
	maxImportBytes = 8 << 20
	reasonRunes    = 120
	statusImported = "imported"
	unresolvedCode = "datasources_unresolved"
	standardInput  = "-"
)

type importOptions struct {
	maps    []string
	preview bool
	folder  string
}

type importRequest struct {
	JSON    string            `json:"json"`
	Mapping map[string]string `json:"mapping,omitempty"`
	Folder  string            `json:"folder,omitempty"`
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

type importPreview struct {
	Summary     importSummary      `json:"summary"`
	Datasources []importDatasource `json:"datasources"`
	Ready       bool               `json:"ready"`
	Report      importReport       `json:"report"`
}

type importedDashboard struct {
	Dashboard struct {
		ID     string `json:"id"`
		Folder struct {
			Title string `json:"title"`
		} `json:"folder"`
	} `json:"dashboard"`
	Import struct {
		Title  string       `json:"title"`
		Report importReport `json:"report"`
	} `json:"import"`
	FolderNote *string `json:"folder_note"`
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
			"  observe import dashboard.json --preview",
		Args: exactArgs(1, "one file, or - for standard input"),
		RunE: a.runImport,
	}
	cmd.Flags().StringArrayVar(&a.importing.maps, "map", nil, "use our datasource <uid> for a Grafana datasource: <grafana>=<uid> (repeatable)")
	cmd.Flags().BoolVar(&a.importing.preview, "preview", false, "show the report without saving anything")
	cmd.Flags().StringVar(&a.importing.folder, "folder", "", "put the dashboard in the folder with this title (default: the folder named in the Grafana export, else your default folder)")
	return cmd
}

func (a *app) runImport(cmd *cobra.Command, args []string) error {
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
	path, folder := "imports", a.importing.folder
	if a.importing.preview {
		path, folder = "imports/preview", ""
	}
	data, err := c.withTimeout(importTimeout).post(cmd.Context(), path, importRequest{JSON: source, Mapping: mapping, Folder: folder})
	if err != nil {
		return unresolvedFrom(err)
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
		out.fields(previewFields(preview))
		out.line("")
		out.line("%s", datasourcesTable(preview.Datasources))
		out.line("%s", preview.Report.Counts.line())
		if table := preview.Report.attention(); table != "" {
			out.line("")
			out.line("%s", table)
		}
		return nil
	}
	imported, err := decode[importedDashboard](data)
	if err != nil {
		return err
	}
	out.line("%s", c.base.JoinPath("dashboards", imported.Dashboard.ID).String())
	out.notice("Imported %q: %s", text(imported.Import.Title), imported.Import.Report.Counts.line())
	out.notice("Folder: %s", text(imported.Dashboard.Folder.Title))
	if imported.FolderNote != nil {
		out.warn("%s", text(*imported.FolderNote))
	}
	if table := imported.Import.Report.attention(); table != "" {
		out.notice("%s", table)
	}
	return nil
}

func unresolvedFrom(err error) error {
	var api *apiError
	if !errors.As(err, &api) || api.status != 422 || api.code != unresolvedCode {
		return err
	}
	body, decodeErr := decode[unresolvedBody](api.raw)
	if decodeErr != nil {
		return err
	}
	return &unresolvedError{api: api, datasources: body.Datasources}
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
	if name == standardInput {
		label = "standard input"
	} else {
		file, err := os.Open(name) //nolint:gosec // reading the file the user named is the command's purpose
		if err != nil {
			return "", fmt.Errorf("read %s: %w", label, err)
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxImportBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
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
