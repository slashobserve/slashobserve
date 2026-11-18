package cli

import (
	"errors"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

var errDatasourceUnhealthy = errors.New("the datasource is still unhealthy")

func (a *app) datasourcesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "datasources",
		Short: "List, recheck and manage datasources",
		Long:  "datasources lists the datasources the server queries and runs a fresh health check on one.\nget, add, edit, test and remove manage the datasources added in the app and need an Admin key; a datasource from the datasources file cannot be changed here.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.datasourcesListCommand(), a.datasourcesRecheckCommand(), a.datasourcesGetCommand(), a.datasourcesAddCommand(), a.datasourcesEditCommand(), a.datasourcesTestCommand(), a.datasourcesRemoveCommand())
	return cmd
}

func (a *app) datasourcesListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the datasources with their health",
		Long:  "list prints each datasource's uid, type, health, the dashboards that use it and when it was last checked.\nAn unhealthy one shows why.",
		Args:  noArgs,
		RunE:  a.runDatasourcesList,
	}
}

func (a *app) datasourcesRecheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "recheck <uid>",
		Short: "Run a fresh health check on one datasource",
		Long:  "recheck skips the server's 10 second health cache and prints the new status.\nIt exits 1 when the datasource is still unhealthy.",
		Args:  exactArgs(1, "a datasource uid"),
		RunE:  a.runDatasourcesRecheck,
	}
}

func (a *app) runDatasourcesList(cmd *cobra.Command, _ []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "datasources", nil)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	list, err := decode[datasourceList](data)
	if err != nil {
		return err
	}
	if len(list.Datasources) == 0 {
		out.notice("No datasources yet")
		return nil
	}
	rows := make([][]string, 0, len(list.Datasources))
	for _, ds := range list.Datasources {
		rows = append(rows, []string{text(ds.UID), text(ds.Type), text(ds.Name), datasourceHealth(ds), text(ds.Source), strconv.Itoa(ds.DashboardCount), when(ds.CheckedAt)})
	}
	out.table([]string{"UID", "TYPE", "NAME", "HEALTH", "SOURCE", "DASHBOARDS", "CHECKED"}, rows)
	return nil
}

func (a *app) runDatasourcesRecheck(cmd *cobra.Command, args []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "datasources/"+url.PathEscape(args[0])+"/recheck", struct{}{})
	if err != nil {
		return err
	}
	ds, err := decode[datasourceEntry](data)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
	} else {
		out.fields([][2]string{
			{"UID", text(ds.UID)},
			{"Type", text(ds.Type)},
			{"Health", datasourceHealth(ds)},
			{"Checked", when(ds.CheckedAt)},
		})
	}
	if !ds.Healthy {
		return errDatasourceUnhealthy
	}
	return nil
}

func datasourceHealth(ds datasourceEntry) string {
	if ds.Healthy {
		return "healthy"
	}
	health := "unhealthy"
	if message := orEmpty(ds.Message); message != "" {
		health += ": " + text(message)
	}
	if detail := orEmpty(ds.Detail); detail != "" {
		health += " (" + text(detail) + ")"
	}
	return health
}
