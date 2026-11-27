package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const maxDatasourceSecretBytes = 4096

var errDatasourceTestFailed = errors.New("the datasource did not pass the check")

type datasourceFlags struct {
	kind, name, audience, address, username, database, tenantID       string
	scrapeInterval                                                    string
	index, timeField, queryLanguage, spans                            string
	fieldMessage, fieldLevel, fieldService, fieldTraceID, fieldSpanID string
	timeout                                                           string
	tls                                                               bool
	maxSeries, maxPoints, maxRows, maxConcurrent                      int
	grafanaUIDs                                                       []string
	passwordFile, bearerTokenFile, apiKeyFile                         string
	headers                                                           []string
	clear                                                             []string
	saved                                                             bool
}

type datasourceConfig struct {
	UID        string   `json:"uid"`
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	Audience   string   `json:"audience"`
	Address    string   `json:"address"`
	Username   string   `json:"username"`
	Database   string   `json:"database"`
	TLS        bool     `json:"tls"`
	TenantID   string   `json:"tenant_id"`
	Scrape     string   `json:"scrape_interval"`
	Index      string   `json:"index"`
	TimeField  string   `json:"time_field"`
	Language   string   `json:"query_language"`
	Spans      string   `json:"spans"`
	GrafanaIDs []string `json:"grafana_uids"`
	SecretsSet []string `json:"secrets_set"`
	UpdatedAt  string   `json:"updated_at"`
}

type datasourceTestResult struct {
	Healthy bool    `json:"healthy"`
	Message *string `json:"message"`
	Detail  *string `json:"detail"`
}

type datasourceInUse struct {
	UsedBy []struct {
		Kind  string `json:"kind"`
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"used_by"`
	Hidden int `json:"hidden"`
}

func (f *datasourceFlags) register(cmd *cobra.Command, withType bool) {
	flags := cmd.Flags()
	if withType {
		flags.StringVar(&f.kind, "type", "", "prometheus, loki, tempo, jaeger, clickhouse, elasticsearch or opensearch")
	}
	flags.StringVar(&f.name, "name", "", "the display name; the uid when empty")
	flags.StringVar(&f.audience, "audience", "", "who can query it: everyone, the default, or admins")
	flags.StringVar(&f.address, "address", "", "an http or https URL, or host:port for ClickHouse")
	flags.StringVar(&f.username, "username", "", "the basic-auth user, or the ClickHouse user")
	flags.StringVar(&f.database, "database", "", "the ClickHouse database")
	flags.BoolVar(&f.tls, "tls", false, "connect to ClickHouse over TLS")
	flags.StringVar(&f.tenantID, "tenant-id", "", "the Loki tenant")
	flags.StringVar(&f.scrapeInterval, "scrape-interval", "", "the Prometheus scrape interval, such as 15s")
	flags.StringVar(&f.index, "index", "", "the Elasticsearch or OpenSearch index pattern")
	flags.StringVar(&f.timeField, "time-field", "", "the time field of the index")
	flags.StringVar(&f.queryLanguage, "query-language", "", "kql or lucene")
	flags.StringVar(&f.spans, "spans", "", "jaeger when the index holds spans Jaeger wrote")
	flags.StringVar(&f.fieldMessage, "field-message", "", "the index field that holds the log message")
	flags.StringVar(&f.fieldLevel, "field-level", "", "the index field that holds the log level")
	flags.StringVar(&f.fieldService, "field-service", "", "the index field that holds the service name")
	flags.StringVar(&f.fieldTraceID, "field-trace-id", "", "the index field that holds the trace id")
	flags.StringVar(&f.fieldSpanID, "field-span-id", "", "the index field that holds the span id")
	flags.StringVar(&f.timeout, "timeout", "", "the query timeout, such as 30s")
	flags.IntVar(&f.maxSeries, "max-series", 0, "the most series a query may return")
	flags.IntVar(&f.maxPoints, "max-points", 0, "the most points a query may return")
	flags.IntVar(&f.maxRows, "max-rows", 0, "the most rows a query may return")
	flags.IntVar(&f.maxConcurrent, "max-concurrent-queries", 0, "the most queries that run at once across the fleet")
	flags.StringSliceVar(&f.grafanaUIDs, "grafana-uid", nil, "a Grafana datasource uid this one stands for on import; repeat for several")
	flags.StringVar(&f.passwordFile, "password-file", "", "read the password from this file, or - for standard input")
	flags.StringVar(&f.bearerTokenFile, "bearer-token-file", "", "read the bearer token from this file, or - for standard input")
	flags.StringVar(&f.apiKeyFile, "api-key-file", "", "read the API key from this file, or - for standard input")
	flags.StringArrayVar(&f.headers, "header", nil, "NAME=FILE: send header NAME with the value read from FILE, or - for standard input; repeat for several")
}

func (f *datasourceFlags) registerClear(cmd *cobra.Command) {
	cmd.Flags().StringArrayVar(&f.clear, "clear", nil, "remove a stored credential: password, bearer_token, api_key or header:<name>; repeat for several")
}

func (a *app) readDatasourceSecret(ctx context.Context, path string, stdinUsed *bool) (string, error) {
	var data []byte
	var err error
	if path == "-" {
		if *stdinUsed {
			return "", usagef("only one credential can be read from standard input")
		}
		*stdinUsed = true
		file, interactive := a.interactiveInput()
		data, err = readInterruptible(ctx, a.std.Err, file, interactive, func() ([]byte, error) {
			return io.ReadAll(io.LimitReader(a.std.In, maxDatasourceSecretBytes+1))
		})
		if errors.Is(err, errInterrupted) {
			return "", err
		}
	} else {
		data, err = readLimitedFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("read the credential: %w", err)
	}
	value := strings.TrimRight(string(data), "\r\n")
	switch {
	case value == "":
		return "", usagef("the credential in %s is empty", pathLabel(path))
	case len(value) > maxDatasourceSecretBytes:
		return "", usagef("the credential in %s is longer than %d bytes", pathLabel(path), maxDatasourceSecretBytes)
	}
	return value, nil
}

func pathLabel(path string) string {
	if path == "-" {
		return "standard input"
	}
	return path
}

func readLimitedFile(path string) ([]byte, error) {
	file, err := os.Open(path) //nolint:gosec // the operator names the credential file
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(io.LimitReader(file, maxDatasourceSecretBytes+1))
}

func (f *datasourceFlags) body(cmd *cobra.Command, a *app) (map[string]any, error) {
	flags := cmd.Flags()
	body := map[string]any{}
	set := func(flag, key string, value any) {
		if flags.Changed(flag) {
			body[key] = value
		}
	}
	set("type", "type", f.kind)
	set("name", "name", f.name)
	set("audience", "audience", f.audience)
	set("address", "address", f.address)
	set("username", "username", f.username)
	set("database", "database", f.database)
	set("tls", "tls", f.tls)
	set("tenant-id", "tenant_id", f.tenantID)
	set("scrape-interval", "scrape_interval", f.scrapeInterval)
	set("index", "index", f.index)
	set("time-field", "time_field", f.timeField)
	set("query-language", "query_language", f.queryLanguage)
	set("spans", "spans", f.spans)
	set("grafana-uid", "grafana_uids", f.grafanaUIDs)
	roles := map[string]any{}
	for flag, key := range map[string]string{"field-message": "message", "field-level": "level", "field-service": "service", "field-trace-id": "trace_id", "field-span-id": "span_id"} {
		if flags.Changed(flag) {
			value, _ := flags.GetString(flag)
			roles[key] = value
		}
	}
	if len(roles) > 0 {
		body["fields"] = roles
	}
	limits := map[string]any{}
	if flags.Changed("timeout") {
		limits["timeout"] = f.timeout
	}
	for flag, key := range map[string]string{"max-series": "max_series", "max-points": "max_points", "max-rows": "max_rows", "max-concurrent-queries": "max_concurrent_queries"} {
		if flags.Changed(flag) {
			value, _ := flags.GetInt(flag)
			limits[key] = value
		}
	}
	if len(limits) > 0 {
		body["limits"] = limits
	}
	secrets, err := f.secrets(cmd, a)
	if err != nil {
		return nil, err
	}
	if len(secrets) > 0 {
		body["secrets"] = secrets
	}
	if len(f.clear) > 0 {
		body["clear_secrets"] = f.clear
	}
	return body, nil
}

func (f *datasourceFlags) secrets(cmd *cobra.Command, a *app) (map[string]any, error) {
	secrets := map[string]any{}
	stdinUsed := false
	for _, s := range []struct{ path, key string }{{f.passwordFile, "password"}, {f.bearerTokenFile, "bearer_token"}, {f.apiKeyFile, "api_key"}} {
		if s.path == "" {
			continue
		}
		value, err := a.readDatasourceSecret(cmd.Context(), s.path, &stdinUsed)
		if err != nil {
			return nil, err
		}
		secrets[s.key] = value
	}
	headers := map[string]string{}
	for _, entry := range f.headers {
		name, path, ok := strings.Cut(entry, "=")
		if !ok || name == "" || path == "" {
			return nil, usagef("--header takes NAME=FILE, got %q", entry)
		}
		value, err := a.readDatasourceSecret(cmd.Context(), path, &stdinUsed)
		if err != nil {
			return nil, err
		}
		headers[name] = value
	}
	if len(headers) > 0 {
		secrets["headers"] = headers
	}
	return secrets, nil
}

func (a *app) datasourcesGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <uid>",
		Short: "Show the definition of a datasource added in the app",
		Long:  "get prints the address, login and settings of a datasource an Admin added in the app, and which credentials are stored.\nIt never prints a credential's value.",
		Args:  exactArgs(1, "a datasource uid"),
		RunE:  a.runDatasourcesGet,
	}
}

func (a *app) datasourcesAddCommand() *cobra.Command {
	flags := &datasourceFlags{}
	cmd := &cobra.Command{
		Use:   "add <uid> --type <type> --address <address> [flags]",
		Short: "Add a datasource in the app",
		Long: "add saves a datasource, encrypts its credentials with the server's secret key and starts using it at once.\n" +
			"Credentials are read from files or standard input so they stay out of the process list and the shell history.\n" +
			"The address must pass the outbound allowlist; it needs an Admin key.",
		Example: "  observe datasources add metrics --type prometheus --address https://prom.example.com --bearer-token-file ./token\n" +
			"  printf '%s' \"$TOKEN\" | observe datasources add logs --type loki --address https://loki.example.com --bearer-token-file -",
		Args: exactArgs(1, "a datasource uid"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.kind == "" || flags.address == "" {
				return usagef("pass --type and --address")
			}
			body, err := flags.body(cmd, a)
			if err != nil {
				return err
			}
			body["uid"] = args[0]
			return a.sendDatasource(cmd, "Added", func(c *client) ([]byte, error) { return c.post(cmd.Context(), "datasources", body) })
		},
	}
	flags.register(cmd, true)
	return cmd
}

func (a *app) datasourcesEditCommand() *cobra.Command {
	flags := &datasourceFlags{}
	cmd := &cobra.Command{
		Use:   "edit <uid> [flags]",
		Short: "Change a datasource added in the app",
		Long: "edit changes only the settings you pass; a credential you do not pass keeps its stored value.\n" +
			"--clear removes a stored credential.\n" +
			"The change applies at once on the server you reach and within 15 seconds on the others.",
		Example: "  observe datasources edit metrics --address https://prom2.example.com\n  observe datasources edit metrics --clear bearer_token",
		Args:    exactArgs(1, "a datasource uid"),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := flags.body(cmd, a)
			if err != nil {
				return err
			}
			if len(body) == 0 {
				return usagef("pass at least one setting to change")
			}
			return a.sendDatasource(cmd, "Changed", func(c *client) ([]byte, error) {
				return c.patch(cmd.Context(), "datasources/"+url.PathEscape(args[0]), body)
			})
		},
	}
	flags.register(cmd, false)
	flags.registerClear(cmd)
	return cmd
}

func (a *app) sendDatasource(cmd *cobra.Command, verb string, send func(*client) ([]byte, error)) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := send(c)
	if err != nil {
		return err
	}
	return render(a.output(), data, func(cfg datasourceConfig) error {
		out := a.output()
		out.line("%s datasource %s", verb, text(cfg.UID))
		out.line("Check it now: observe datasources recheck %s", text(cfg.UID))
		return nil
	})
}

func (a *app) datasourcesTestCommand() *cobra.Command {
	flags := &datasourceFlags{}
	cmd := &cobra.Command{
		Use:   "test <uid> [--saved] [flags]",
		Short: "Check a datasource definition without saving it",
		Long: "test runs the real health check against a draft definition and saves nothing.\n" +
			"With --saved the draft starts from the saved datasource and your flags change only what they name, so stored credentials are used.\n" +
			"It exits 1 when the check fails.",
		Example: "  observe datasources test metrics --type prometheus --address https://prom.example.com\n  observe datasources test metrics --saved --address https://prom2.example.com",
		Args:    exactArgs(1, "a datasource uid"),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := flags.body(cmd, a)
			if err != nil {
				return err
			}
			body["uid"] = args[0]
			var query url.Values
			if flags.saved {
				query = url.Values{"base": {args[0]}}
			}
			return a.runDatasourcesTest(cmd, query, body)
		},
	}
	flags.register(cmd, true)
	cmd.Flags().BoolVar(&flags.saved, "saved", false, "start from the saved datasource of this uid")
	return cmd
}

func (a *app) runDatasourcesTest(cmd *cobra.Command, query url.Values, body map[string]any) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode the request: %w", err)
	}
	data, err := c.do(cmd.Context(), http.MethodPost, "datasources/test", query, payload)
	if err != nil {
		return err
	}
	result, err := decode[datasourceTestResult](data)
	if err != nil {
		return err
	}
	out := a.output()
	switch {
	case a.json:
		out.body(data)
	case result.Healthy:
		out.line("The datasource answered the health check")
	default:
		reason := text(orEmpty(result.Message))
		if detail := orEmpty(result.Detail); detail != "" {
			reason += " (" + text(detail) + ")"
		}
		out.line("The check failed: %s", reason)
	}
	if !result.Healthy {
		return errDatasourceTestFailed
	}
	return nil
}

func (a *app) datasourcesRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <uid>",
		Short: "Remove a datasource added in the app",
		Long:  "remove deletes a datasource an Admin added in the app.\nIt is refused, with the list of what uses it, while a dashboard or saved search uses the datasource.",
		Args:  exactArgs(1, "a datasource uid"),
		RunE:  a.runDatasourcesRemove,
	}
}

func (a *app) runDatasourcesGet(cmd *cobra.Command, args []string) error {
	data, err := a.fetch(cmd.Context(), "datasources/"+url.PathEscape(args[0])+"/config", nil)
	if err != nil {
		return err
	}
	return render(a.output(), data, func(cfg datasourceConfig) error {
		pairs := [][2]string{{"UID", text(cfg.UID)}, {"Type", text(cfg.Type)}, {"Name", text(cfg.Name)}, {"Audience", text(cfg.Audience)}, {"Address", text(cfg.Address)}}
		for _, optional := range [][2]string{{"Username", cfg.Username}, {"Database", cfg.Database}, {"Tenant", cfg.TenantID}, {"Scrape interval", cfg.Scrape}, {"Index", cfg.Index}, {"Time field", cfg.TimeField}, {"Query language", cfg.Language}, {"Spans", cfg.Spans}} {
			if optional[1] != "" {
				pairs = append(pairs, [2]string{optional[0], text(optional[1])})
			}
		}
		credentials := "none"
		if len(cfg.SecretsSet) > 0 {
			credentials = text(strings.Join(cfg.SecretsSet, ", "))
		}
		pairs = append(pairs, [2]string{"Credentials set", credentials}, [2]string{"Updated", when(cfg.UpdatedAt)})
		a.output().fields(pairs)
		return nil
	})
}

func (a *app) runDatasourcesRemove(cmd *cobra.Command, args []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	if _, err := c.delete(cmd.Context(), "datasources/"+url.PathEscape(args[0])); err != nil {
		return datasourceInUseDetail(err)
	}
	if !a.json {
		a.output().confirm("Removed datasource %s", text(args[0]))
	}
	return nil
}

func datasourceInUseDetail(err error) error {
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.code != "datasource_in_use" {
		return err
	}
	var inUse datasourceInUse
	if json.Unmarshal(apiErr.raw, &inUse) != nil {
		return err
	}
	lines := []string{err.Error(), "Used by:"}
	for _, u := range inUse.UsedBy {
		lines = append(lines, fmt.Sprintf("  %s %s %s", text(u.Kind), text(u.ID), text(u.Title)))
	}
	if inUse.Hidden > 0 {
		lines = append(lines, fmt.Sprintf("  and %d more in folders you cannot see", inUse.Hidden))
	}
	return errors.New(strings.Join(lines, "\n"))
}
