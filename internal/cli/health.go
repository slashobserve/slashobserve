package cli

import (
	"errors"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

const (
	defaultHealthLimit = 50
	maxHealthLimit     = 100
	maxRepairIDs       = 50
)

var errNothingQueued = errors.New("no repair was queued")

type healthOptions struct {
	folder string
	kind   string
	limit  int
}

type healthFolder struct {
	Title string `json:"title"`
}

type healthProblem struct {
	Kind       string  `json:"kind"`
	PanelID    *string `json:"panel_id"`
	PanelTitle *string `json:"panel_title"`
	Variable   *string `json:"variable"`
}

type healthDashboard struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Folder    healthFolder    `json:"folder"`
	CheckedAt *string         `json:"checked_at"`
	Problems  []healthProblem `json:"problems"`
}

type healthJob struct {
	LastStartedAt  *string `json:"last_started_at"`
	LastFinishedAt *string `json:"last_finished_at"`
	LastOutcome    *string `json:"last_outcome"`
	Running        bool    `json:"running"`
}

type healthList struct {
	Dashboards []healthDashboard `json:"dashboards"`
	NextCursor *string           `json:"next_cursor"`
	Job        healthJob         `json:"job"`
}

type repairOutcome struct {
	Queued []struct {
		DashboardID string `json:"dashboard_id"`
		RequestID   string `json:"request_id"`
	} `json:"queued"`
	Skipped []struct {
		DashboardID string `json:"dashboard_id"`
		Reason      string `json:"reason"`
	} `json:"skipped"`
}

func (a *app) healthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "List dashboards with health problems, and repair them",
		Long: "health lists the dashboards with problems in the folders you can see: panels whose queries failed or returned no data at the last daily check, variables that failed, duplicate titles in a folder and dashboards nobody opened in 90 days.\n" +
			"It exits 0 whatever it finds.",
		Example: "  observe health\n" +
			"  observe health --kind no_data --folder <folder-id>\n" +
			"  observe health repair <dashboard-id> <dashboard-id>\n" +
			"  observe health run",
		Args: noArgs,
		RunE: a.runHealth,
	}
	cmd.Flags().StringVar(&a.health.folder, "folder", "", "only dashboards in this folder id")
	cmd.Flags().StringVar(&a.health.kind, "kind", "", "only this kind of problem: no_data, query_failed, variable_failed, duplicate_title or unopened")
	cmd.Flags().IntVar(&a.health.limit, "limit", defaultHealthLimit, "dashboards to list, up to 100")
	cmd.AddCommand(a.healthRepairCommand(), a.healthRunCommand())
	return cmd
}

func (o healthOptions) query() (url.Values, error) {
	query := url.Values{"limit": {strconv.Itoa(o.limit)}}
	if o.folder != "" {
		id, err := parseID(o.folder)
		if err != nil {
			return nil, err
		}
		query.Set("folder_id", id)
	}
	if o.kind != "" {
		query.Set("kind", o.kind)
	}
	return query, nil
}

func (a *app) runHealth(cmd *cobra.Command, _ []string) error {
	query, err := a.health.query()
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "health", query)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	list, err := decode[healthList](data)
	if err != nil {
		return err
	}
	if len(list.Dashboards) == 0 {
		out.line("No dashboard has a health problem.")
	} else {
		out.table([]string{"ID", "DASHBOARD", "FOLDER", "KIND", "PANEL OR VARIABLE", "CHECKED"}, healthRows(list.Dashboards))
	}
	out.line("%s", jobLine(list.Job))
	if list.NextCursor != nil {
		out.notice("more dashboards have problems; raise --limit (up to %d) or narrow with --folder or --kind", maxHealthLimit)
	}
	return nil
}

func healthRows(dashboards []healthDashboard) [][]string {
	var rows [][]string
	for _, d := range dashboards {
		checked := "-"
		if d.CheckedAt != nil {
			checked = when(*d.CheckedAt)
		}
		for _, p := range d.Problems {
			rows = append(rows, []string{text(d.ID), text(d.Title), text(d.Folder.Title), text(p.Kind), problemTarget(p), checked})
		}
	}
	return rows
}

func problemTarget(p healthProblem) string {
	switch {
	case p.Variable != nil && *p.Variable != "":
		return "$" + text(*p.Variable)
	case p.PanelTitle != nil && *p.PanelTitle != "" && p.PanelID != nil:
		return text(*p.PanelTitle) + " (" + text(*p.PanelID) + ")"
	}
	return orDash(p.PanelID)
}

func jobLine(j healthJob) string {
	switch {
	case j.Running:
		return "A health check is running now."
	case j.LastFinishedAt != nil:
		return "Last check finished " + when(*j.LastFinishedAt) + ": " + orDash(j.LastOutcome) + "."
	case j.LastStartedAt != nil:
		return "Last check started " + when(*j.LastStartedAt) + " and did not finish."
	}
	return "No health check has run yet."
}

func (a *app) healthRepairCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "repair <dashboard-id>...",
		Short: "Ask the agent to repair the failing panels of dashboards",
		Long: "repair queues one agent revision per dashboard that fixes the panels and variables the last health check found failing.\n" +
			"It prints queued or the reason each dashboard was skipped, and exits 1 when it queued none.",
		Example: "  observe health repair <dashboard-id> <dashboard-id>",
		Args:    repairArgs,
		RunE:    a.runHealthRepair,
	}
}

func repairArgs(cmd *cobra.Command, args []string) error {
	if len(args) < 1 || len(args) > maxRepairIDs {
		return usagef("%s takes 1 to %d dashboard ids", cmd.CommandPath(), maxRepairIDs)
	}
	for _, arg := range args {
		if _, err := parseID(arg); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) runHealthRepair(cmd *cobra.Command, args []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "health/repairs", map[string][]string{"dashboard_ids": args})
	if err != nil {
		return err
	}
	outcome, err := decode[repairOutcome](data)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
	} else {
		out.table([]string{"DASHBOARD", "RESULT", "REQUEST"}, repairRows(args, outcome))
	}
	if len(outcome.Queued) == 0 {
		return errNothingQueued
	}
	return nil
}

func repairRows(ids []string, outcome repairOutcome) [][]string {
	results := map[string][2]string{}
	for _, q := range outcome.Queued {
		results[q.DashboardID] = [2]string{"queued", text(q.RequestID)}
	}
	for _, s := range outcome.Skipped {
		results[s.DashboardID] = [2]string{text(s.Reason), "-"}
	}
	var rows [][]string
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		result, ok := results[id]
		if !ok {
			result = [2]string{"-", "-"}
		}
		rows = append(rows, []string{text(id), result[0], result[1]})
	}
	return rows
}

func (a *app) healthRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Start a health check of every dashboard now (Admin)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := a.newClient()
			if err != nil {
				return err
			}
			if _, err := c.post(cmd.Context(), "health/run", struct{}{}); err != nil {
				return err
			}
			if !a.json {
				a.output().confirm("Started a health check; results appear as each dashboard is checked.")
			}
			return nil
		},
	}
}
