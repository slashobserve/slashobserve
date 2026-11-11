package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const maxGrafanaNameRunes = 100

var errGrafanaImportInterrupted = errors.New("interrupted; the import continues on the server")

type grafanaConnectOptions struct {
	name       string
	tokenStdin bool
}

type grafanaImportOptions struct {
	folders string
	all     bool
}

type grafanaTest struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type grafanaConnection struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	LastTest *grafanaTest `json:"last_test"`
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
		Short: "Connect to a Grafana and import its dashboards",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.grafanaConnectCommand(), a.grafanaImportCommand())
	return cmd
}

func (a *app) grafanaConnectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connect <url> --token-stdin",
		Short:   "Store a Grafana address and token and test them",
		Long:    "connect stores a Grafana address with a Grafana service-account token read from standard input, then tests it.\nIt needs an Admin key.",
		Example: "  printf '%s' \"$GRAFANA_TOKEN\" | observe grafana connect https://grafana.example.com --token-stdin",
		Args:    exactArgs(1, "the Grafana URL"),
		RunE:    a.runGrafanaConnect,
	}
	cmd.Flags().BoolVar(&a.grafanaConnect.tokenStdin, "token-stdin", false, "read the Grafana token from standard input")
	cmd.Flags().StringVar(&a.grafanaConnect.name, "name", "", "the connection's name (default the URL's host)")
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

func (a *app) runGrafanaConnect(cmd *cobra.Command, args []string) error {
	opts := a.grafanaConnect
	if !opts.tokenStdin {
		return usagef("pass the Grafana token on standard input with --token-stdin")
	}
	address, name, err := grafanaConnectTarget(args[0], opts.name)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	token, err := a.readGrafanaToken(ctx)
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(ctx, "grafana", map[string]string{"name": name, "url": address, "token": token})
	if err != nil {
		return err
	}
	created, err := decode[grafanaConnection](data)
	if err != nil {
		return err
	}
	stored := fmt.Sprintf("the connection %s is stored; fix or remove it on the Grafana settings page", text(created.ID))
	data, err = c.post(ctx, "grafana/"+created.ID+"/test", struct{}{})
	if err != nil {
		return fmt.Errorf("%w\n%s", err, stored)
	}
	tested, err := decode[grafanaConnection](data)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
	} else {
		out.line("Connected %s: %s", text(tested.Name), text(tested.ID))
	}
	if err := connectionTestFailure(tested.LastTest, stored); err != nil {
		return err
	}
	if !a.json {
		out.line("%s", text(tested.LastTest.Message))
	}
	return nil
}

func grafanaConnectTarget(raw, name string) (string, string, error) {
	address := strings.TrimSpace(raw)
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", "", usagef("%q is not an http or https URL", text(address))
	}
	name = cmp.Or(name, parsed.Hostname())
	if runes := []rune(name); len(runes) > maxGrafanaNameRunes {
		name = string(runes[:maxGrafanaNameRunes])
	}
	return address, name, nil
}

func connectionTestFailure(test *grafanaTest, stored string) error {
	switch {
	case test == nil:
		return fmt.Errorf("the server did not report a test result\n%s", stored)
	case !test.OK:
		return fmt.Errorf("the connection test failed: %s\n%s", text(test.Message), stored)
	}
	return nil
}

func (a *app) readGrafanaToken(ctx context.Context) (string, error) {
	file, interactive := a.interactiveInput()
	data, err := readInterruptible(ctx, a.std.Err, file, interactive, func() ([]byte, error) {
		return io.ReadAll(io.LimitReader(a.std.In, maxKeyBytes+1))
	})
	if errors.Is(err, errInterrupted) {
		return "", err
	}
	if err != nil {
		return "", fmt.Errorf("read the Grafana token from standard input: %w", err)
	}
	token := strings.TrimSpace(string(data))
	switch {
	case token == "":
		return "", usagef("standard input held no Grafana token")
	case len(token) > maxKeyBytes:
		return "", usagef("the Grafana token is longer than %d bytes", maxKeyBytes)
	}
	return token, nil
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
