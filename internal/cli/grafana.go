package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var errGrafanaImportInterrupted = errors.New("interrupted; the import continues on the server")

type grafanaImportOptions struct {
	folders string
	all     bool
}

type grafanaFolder struct {
	UID        string `json:"uid"`
	Title      string `json:"title"`
	Dashboards int    `json:"dashboards"`
}

type grafanaImportResult struct {
	UID              string  `json:"uid"`
	Title            string  `json:"title"`
	Outcome          string  `json:"outcome"`
	UnverifiedPanels int     `json:"unverified_panels"`
	Error            *string `json:"error"`
}

type grafanaImport struct {
	ID      string                `json:"id"`
	Status  string                `json:"status"`
	Total   int                   `json:"total"`
	Done    int                   `json:"done"`
	Failed  int                   `json:"failed"`
	Skipped int                   `json:"skipped"`
	Error   *string               `json:"error"`
	Results []grafanaImportResult `json:"results"`
}

func (a *app) grafanaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grafana",
		Short: "Import the dashboards of a connected Grafana",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.grafanaImportCommand())
	return cmd
}

func (a *app) grafanaImportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <connection id> [--folders <uid,...> | --all]",
		Short: "Import the dashboards of a connected Grafana",
		Long:  "import brings the dashboards of the chosen Grafana folders in, with their folders, and waits until it finishes.\nWithout --folders or --all it lists the folders to choose from.",
		Example: "  observe grafana import <connection-id>\n" +
			"  observe grafana import <connection-id> --folders platform,checkout\n" +
			"  observe grafana import <connection-id> --all",
		Args: exactArgs(1, "a connection id"),
		RunE: a.runGrafanaImport,
	}
	cmd.Flags().StringVar(&a.grafanaImport.folders, "folders", "", "comma-separated Grafana folder uids, each with its subfolders; general is the dashboards outside any folder")
	cmd.Flags().BoolVar(&a.grafanaImport.all, "all", false, "import every folder")
	return cmd
}

func (a *app) runGrafanaImport(cmd *cobra.Command, args []string) error {
	opts := a.grafanaImport
	connID, err := parseID(args[0])
	if err != nil {
		return err
	}
	if opts.all && opts.folders != "" {
		return usagef("choose --folders or --all, not both")
	}
	folders := splitList(opts.folders)
	if opts.folders != "" && len(folders) == 0 {
		return usagef("--folders needs at least one folder uid")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	out := a.output()
	if !opts.all && len(folders) == 0 {
		return listGrafanaFolders(ctx, c, out, connID)
	}
	body := map[string]any{"folders": "all"}
	if !opts.all {
		body["folders"] = folders
	}
	data, err := c.post(ctx, "grafana/"+connID+"/imports", body)
	if err != nil {
		return err
	}
	started, err := decode[grafanaImport](data)
	if err != nil {
		return err
	}
	final := started
	if started.Status == "running" {
		data, final, err = pollGrafanaImport(ctx, c, out, connID, started.ID, pollInterval)
		if err != nil {
			return err
		}
	}
	if a.json {
		out.body(data)
	} else {
		reportGrafanaImport(out, final)
	}
	return importOutcome(final)
}

func listGrafanaFolders(ctx context.Context, c *client, out *output, connID string) error {
	data, err := c.get(ctx, "grafana/"+connID+"/folders", nil)
	if err != nil {
		return err
	}
	if out.json {
		out.body(data)
	} else {
		folders, err := decode[[]grafanaFolder](data)
		if err != nil {
			return err
		}
		rows := make([][]string, 0, len(folders))
		for _, f := range folders {
			rows = append(rows, []string{text(f.UID), text(f.Title), strconv.Itoa(f.Dashboards)})
		}
		out.table([]string{"UID", "TITLE", "DASHBOARDS"}, rows)
	}
	return usagef("choose folders with --folders <uid,...> or import everything with --all")
}

func pollGrafanaImport(ctx context.Context, c *client, out *output, connID, syncID string, interval time.Duration) ([]byte, grafanaImport, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	live := out.terminal && !out.quiet && !out.json
	defer func() {
		if live {
			_, _ = fmt.Fprint(out.out, clearLine)
		}
	}()
	failures := 0
	for {
		data, current, err := readGrafanaImport(ctx, c, connID, syncID)
		switch {
		case ctx.Err() != nil:
			return nil, grafanaImport{}, errGrafanaImportInterrupted
		case err != nil:
			failures++
			if !isTransient(err) || failures >= maxPollFailures {
				return nil, grafanaImport{}, err
			}
		case current.Status != "running":
			return data, current, nil
		default:
			failures = 0
			if live {
				_, _ = fmt.Fprintf(out.out, "%s%s", clearLine, out.color.dim(fmt.Sprintf("Imported %d of %d", current.Done, current.Total)))
			}
		}
		select {
		case <-ctx.Done():
			return nil, grafanaImport{}, errGrafanaImportInterrupted
		case <-ticker.C:
		}
	}
}

func readGrafanaImport(ctx context.Context, c *client, connID, syncID string) ([]byte, grafanaImport, error) {
	data, err := c.get(ctx, "grafana/"+connID+"/imports/"+syncID, nil)
	var api *apiError
	if errors.As(err, &api) && api.status == http.StatusNotFound {
		return nil, grafanaImport{}, fmt.Errorf("the server no longer has import %s; a connection keeps only its last 20 imports", syncID)
	}
	if err != nil {
		return nil, grafanaImport{}, err
	}
	current, err := decode[grafanaImport](data)
	return data, current, err
}

func reportGrafanaImport(out *output, final grafanaImport) {
	out.line("%d done, %d skipped, %d failed of %d", final.Done, final.Skipped, final.Failed, final.Total)
	var failed [][]string
	unverified := 0
	for _, r := range final.Results {
		if r.Outcome == "failed" {
			failed = append(failed, []string{text(r.UID), text(r.Title), orDash(r.Error)})
		}
		if r.UnverifiedPanels > 0 {
			unverified++
		}
	}
	if len(failed) > 0 {
		out.table([]string{"UID", "TITLE", "ERROR"}, failed)
	}
	if unverified > 0 {
		out.notice("%d %s panels that did not verify", unverified, plural(unverified, "dashboard has", "dashboards have"))
	}
}

func importOutcome(final grafanaImport) error {
	switch final.Status {
	case "failed", "canceled":
		reason := "the import " + final.Status
		if final.Error != nil && *final.Error != "" {
			reason += ": " + text(*final.Error)
		}
		return errors.New(reason)
	}
	if final.Failed > 0 {
		return fmt.Errorf("%d %s failed to import", final.Failed, plural(final.Failed, "dashboard", "dashboards"))
	}
	return nil
}
