package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

const (
	checkOK   = "ok"
	checkWarn = "warn"
	checkFail = "fail"
	checkSkip = "skip"
)

var releasePattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

type checkResult struct {
	Check  string `json:"check"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type doctorReport struct {
	OK     bool          `json:"ok"`
	Checks []checkResult `json:"checks"`
}

type readiness struct {
	Checks map[string]string `json:"checks"`
}

type keyOwner struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type instanceInfo struct {
	Version string `json:"version"`
}

type agentInfo struct {
	Available bool    `json:"available"`
	Provider  *string `json:"provider"`
	Model     *string `json:"model"`
	Effort    *string `json:"effort"`
}

type datasourceEntry struct {
	UID            string  `json:"uid"`
	Type           string  `json:"type"`
	Name           string  `json:"name"`
	Healthy        bool    `json:"healthy"`
	Message        *string `json:"message"`
	DashboardCount int     `json:"dashboard_count"`
}

type datasourceList struct {
	Datasources []datasourceEntry `json:"datasources"`
}

var keyChecks = []string{"role", "version", "datasource", "model"}

func (a *app) doctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the server, the key, the datasources and the model",
		Long:  "doctor checks that the server is ready, the key works, the CLI and server versions match, every datasource is healthy and a model is configured.\nIt exits 1 when any check fails; a warning never fails it.",
		Args:  noArgs,
		RunE:  a.runDoctor,
	}
}

func (a *app) runDoctor(cmd *cobra.Command, _ []string) error {
	base, err := a.serverAddress()
	if err != nil {
		return err
	}
	report := a.diagnose(cmd.Context(), base.String(), a.keylessClient(base))
	out := a.output()
	if a.json {
		data, err := json.Marshal(report)
		if err != nil {
			return err
		}
		out.body(data)
	} else {
		rows := make([][]string, 0, len(report.Checks))
		for _, check := range report.Checks {
			rows = append(rows, []string{check.Check, paintStatus(out.color, check.Status), text(check.Detail)})
		}
		out.table([]string{"CHECK", "STATUS", "DETAIL"}, rows)
	}
	if failed := report.failed(); failed > 0 {
		return fmt.Errorf("%d %s failed", failed, plural(failed, "check", "checks"))
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func paintStatus(p palette, status string) string {
	switch status {
	case checkOK:
		return p.green(status)
	case checkFail:
		return p.red(status)
	case checkWarn:
		return p.yellow(status)
	}
	return status
}

func (a *app) diagnose(ctx context.Context, server string, keyless *client) doctorReport {
	var checks []checkResult
	add := func(check, status, detail string) { checks = append(checks, checkResult{check, status, detail}) }
	skip := func(names ...string) {
		for _, name := range names {
			add(name, checkSkip, "")
		}
	}

	if status, detail := serverReady(ctx, keyless, server); status != checkOK {
		add("server", status, detail)
		skip(append([]string{"key"}, keyChecks...)...)
		return reportOf(checks)
	}
	add("server", checkOK, server+" is ready")

	c, me, detail := a.checkKey(ctx, keyless)
	if c == nil {
		add("key", checkFail, detail)
		skip(keyChecks...)
		return reportOf(checks)
	}
	add("key", checkOK, fmt.Sprintf("%s (%s)", me.Name, me.Kind))

	switch me.Role {
	case "editor", "admin":
		add("role", checkOK, me.Role)
	default:
		add("role", checkWarn, me.Role+": this role cannot create dashboards or annotations")
	}

	status, detail := a.checkVersion(ctx, c)
	add("version", status, detail)

	for _, row := range checkDatasources(ctx, c) {
		add("datasource", row.Status, row.Detail)
	}

	status, detail = checkModel(ctx, c)
	add("model", status, detail)
	return reportOf(checks)
}

func reportOf(checks []checkResult) doctorReport {
	report := doctorReport{Checks: checks}
	report.OK = report.failed() == 0
	return report
}

func (r doctorReport) failed() int {
	n := 0
	for _, check := range r.Checks {
		if check.Status == checkFail {
			n++
		}
	}
	return n
}

func serverReady(ctx context.Context, c *client, server string) (string, string) {
	status, data, err := c.probe(ctx, "readyz")
	if err != nil {
		return checkFail, err.Error()
	}
	switch status {
	case http.StatusOK:
		return checkOK, ""
	case http.StatusServiceUnavailable:
		var body readiness
		_ = json.Unmarshal(data, &body)
		var failing []string
		for name, state := range body.Checks {
			if state == "error" {
				failing = append(failing, name)
			}
		}
		slices.Sort(failing)
		if len(failing) == 0 {
			return checkFail, server + " is not ready"
		}
		return checkFail, server + " is not ready: " + strings.Join(failing, ", ") + " failing"
	}
	return checkFail, fmt.Sprintf("%s answered HTTP %d on /readyz", server, status)
}

func (a *app) checkKey(ctx context.Context, keyless *client) (*client, keyOwner, string) {
	resolved, err := a.resolve()
	if err != nil {
		return nil, keyOwner{}, err.Error()
	}
	if resolved.token == "" {
		return nil, keyOwner{}, "no key; run observe login"
	}
	c, err := a.clientFor(keyless.base, resolved.token)
	if err != nil {
		return nil, keyOwner{}, err.Error()
	}
	data, err := c.get(ctx, "me", nil)
	if errors.Is(err, errUnauthorized) {
		return nil, keyOwner{}, "the server refused the key; run observe login"
	}
	if err != nil {
		return nil, keyOwner{}, err.Error()
	}
	me, err := decode[keyOwner](data)
	if err != nil {
		return nil, keyOwner{}, err.Error()
	}
	return c, me, ""
}

func (a *app) checkVersion(ctx context.Context, c *client) (string, string) {
	data, err := c.get(ctx, "instance", nil)
	if err != nil {
		return checkFail, err.Error()
	}
	info, err := decode[instanceInfo](data)
	if err != nil {
		return checkFail, err.Error()
	}
	server := cmpOrUnknown(info.Version)
	if releasePattern.MatchString(a.version) && releasePattern.MatchString(info.Version) && a.version != info.Version {
		return checkWarn, fmt.Sprintf("the CLI is %s and the server %s", a.version, server)
	}
	return checkOK, fmt.Sprintf("CLI %s, server %s", a.version, server)
}

func cmpOrUnknown(version string) string {
	if version == "" {
		return "unknown"
	}
	return version
}

func listDatasources(ctx context.Context, c *client) ([]datasourceEntry, error) {
	data, err := c.get(ctx, "datasources", nil)
	if err != nil {
		return nil, err
	}
	list, err := decode[datasourceList](data)
	if err != nil {
		return nil, err
	}
	return list.Datasources, nil
}

func checkDatasources(ctx context.Context, c *client) []checkResult {
	list, err := listDatasources(ctx, c)
	if err != nil {
		return []checkResult{{Status: checkFail, Detail: err.Error()}}
	}
	if len(list) == 0 {
		return []checkResult{{Status: checkWarn, Detail: "no datasources are configured"}}
	}
	rows := make([]checkResult, 0, len(list))
	for _, ds := range list {
		label := fmt.Sprintf("%s (%s)", ds.UID, ds.Type)
		if ds.Healthy {
			rows = append(rows, checkResult{Status: checkOK, Detail: label})
			continue
		}
		message := "unhealthy"
		if ds.Message != nil && *ds.Message != "" {
			message = *ds.Message
		}
		rows = append(rows, checkResult{Status: checkFail, Detail: label + ": " + message})
	}
	return rows
}

func checkModel(ctx context.Context, c *client) (string, string) {
	data, err := c.get(ctx, "agent", nil)
	if err != nil {
		return checkFail, err.Error()
	}
	info, err := decode[agentInfo](data)
	if err != nil {
		return checkFail, err.Error()
	}
	if !info.Available {
		return checkFail, "no model is configured; prompts cannot run"
	}
	detail := strings.TrimSpace(orEmpty(info.Provider) + " " + orEmpty(info.Model))
	if info.Effort != nil && *info.Effort != "" {
		detail += ", effort " + *info.Effort
	}
	return checkOK, detail
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
