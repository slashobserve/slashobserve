package cli

import (
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

type savedSearchListOptions struct {
	pageOptions
	kind   string
	folder string
}

type savedSearchSaveOptions struct {
	kind       string
	title      string
	datasource string
	folder     string
	from       string
	to         string
	query      string
	columns    []string
	service    string
	span       string
	min        string
	max        string
	errors     bool
	mode       string
}

type savedSearchUpdateOptions struct {
	savedSearchSaveOptions
	clearFilters bool
}

type savedSearchMoveOptions struct {
	folder string
}

type savedSearchRow struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	DatasourceUID string `json:"datasource_uid"`
	Folder        struct {
		Title string `json:"title"`
	} `json:"folder"`
	UpdatedAt string `json:"updated_at"`
	URL       string `json:"url"`
	Replayed  bool   `json:"replayed"`
}

type savedSearchListBody struct {
	SavedSearches []savedSearchRow `json:"saved_searches"`
	NextCursor    *string          `json:"next_cursor"`
}

var searchFlagKinds = []struct {
	kind  string
	flags []string
}{
	{"logs", []string{"columns"}},
	{"traces", []string{"service", "span", "min", "max", "errors"}},
	{"metrics", []string{"mode"}},
}

func (a *app) savedSearchesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "saved-searches",
		Short: "List, save, change, rename, move and delete saved logs, trace and metrics searches",
		Long:  "saved-searches manages the logs, trace and metrics searches filed in folders.\nEveryone who can view a search's folder can read it, and the web app reopens it from the Logs, Traces and Metrics pages.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.savedSearchesListCommand(), a.savedSearchesSaveCommand(), a.savedSearchesUpdateCommand(), a.savedSearchesRenameCommand(), a.savedSearchesMoveCommand(), a.savedSearchesDeleteCommand())
	return cmd
}

func searchKind(value string, required bool) (string, error) {
	switch value {
	case "logs", "traces", "metrics":
		return value, nil
	case "":
		if !required {
			return "", nil
		}
		return "", usagef("--kind logs|traces|metrics is required")
	}
	return "", usagef("--kind must be logs, traces or metrics, not %q", text(value))
}

func optionalID(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return parseID(value)
}

func (a *app) savedSearchesListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [--kind logs|traces|metrics] [--folder <id>]",
		Short: "List the saved searches you can view, most recently changed first",
		Args:  noArgs,
		RunE:  a.runSavedSearchesList,
	}
	addPageFlags(cmd, &a.savedSearchList.pageOptions, maxSavedSearchPage)
	cmd.Flags().StringVar(&a.savedSearchList.kind, "kind", "", "only searches of this kind: logs, traces or metrics")
	cmd.Flags().StringVar(&a.savedSearchList.folder, "folder", "", "only searches saved in this folder id")
	return cmd
}

func (o savedSearchListOptions) query() (url.Values, error) {
	kind, err := searchKind(o.kind, false)
	if err != nil {
		return nil, err
	}
	folder, err := optionalID(o.folder)
	if err != nil {
		return nil, err
	}
	query := o.pageOptions.query()
	if kind != "" {
		query.Set("kind", kind)
	}
	if folder != "" {
		query.Set("folder_id", folder)
	}
	return query, nil
}

func (a *app) runSavedSearchesList(cmd *cobra.Command, _ []string) error {
	if err := a.savedSearchList.check(); err != nil {
		return err
	}
	query, err := a.savedSearchList.query()
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.getPages(cmd.Context(), "saved-searches", query, a.savedSearchList.all)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	page, err := decode[savedSearchListBody](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(page.SavedSearches))
	for _, s := range page.SavedSearches {
		rows = append(rows, []string{text(s.ID), text(s.Kind), text(s.Title), text(s.Folder.Title), text(s.DatasourceUID), when(s.UpdatedAt)})
	}
	out.table([]string{"ID", "KIND", "TITLE", "FOLDER", "DATASOURCE", "UPDATED"}, rows)
	pageFooter(out, len(page.SavedSearches), nil, page.NextCursor)
	return nil
}

func (a *app) savedSearchesSaveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "save --kind logs|traces|metrics --title <title> --datasource <uid> [flags]",
		Short: "Save a logs, trace or metrics search in a folder and print its address",
		Long:  "save files a search where its readers can reopen it, by default in your My Dashboards.\nA service-account key has no My Dashboards, so with a key the search lands in the folder General, which everyone who can see General can read; pass --folder to file it elsewhere.\nThe id goes to standard error and the address to standard output, so -q still prints the address.\nA logs search needs --query and may set --columns.\nA trace search may set --service, --span, --min, --max and --errors, and --query on Tempo (TraceQL), Jaeger (key=value pairs) or ClickHouse (SQL, which reopens in the SQL view).\nA metrics search needs --query on a Prometheus datasource and may set --mode table to reopen as a table.\nFlags of another kind are refused.",
		Example: "  observe saved-searches save --kind logs --title \"Checkout errors\" --datasource loki --query '{app=\"checkout\"} |= \"error\"' --columns timestamp,line\n" +
			"  observe saved-searches save --kind traces --title \"Slow checkout\" --datasource tempo --service checkout --min 2s --errors --folder <folder-id>\n" +
			"  observe saved-searches save --kind metrics --title \"Request rate\" --datasource prom --query 'sum(rate(http_requests_total[5m]))' --mode table",
		Args: noArgs,
		RunE: a.runSavedSearchesSave,
	}
	flags := cmd.Flags()
	opts := &a.savedSearchSave
	flags.StringVar(&opts.kind, "kind", "", "logs, traces or metrics")
	flags.StringVar(&opts.title, "title", "", "the title, 1 to 120 characters")
	flags.StringVar(&opts.datasource, "datasource", "", "the datasource uid (observe doctor lists them)")
	flags.StringVar(&opts.folder, "folder", "", "the folder id to save in; default your My Dashboards, or General with a service-account key")
	flags.StringVar(&opts.from, "from", "now-1h", "start of the range, kept as written")
	flags.StringVar(&opts.to, "to", "now", "end of the range, kept as written")
	addSearchContentFlags(cmd, opts, "the query; required for logs and metrics")
	return cmd
}

func addSearchContentFlags(cmd *cobra.Command, opts *savedSearchSaveOptions, queryHelp string) {
	flags := cmd.Flags()
	flags.StringVar(&opts.query, "query", "", queryHelp)
	flags.StringSliceVar(&opts.columns, "columns", nil, "logs only: the visible columns, comma separated")
	flags.StringVar(&opts.service, "service", "", "traces only: the service name")
	flags.StringVar(&opts.span, "span", "", "traces only: the span name")
	flags.StringVar(&opts.min, "min", "", "traces only: the shortest span duration, such as 250ms")
	flags.StringVar(&opts.max, "max", "", "traces only: the longest span duration, such as 2s")
	flags.BoolVar(&opts.errors, "errors", false, "traces only: only traces with an error")
	flags.StringVar(&opts.mode, "mode", "", "metrics only: graph or table, how the page reopens the query")
}

func refuseFlagsOfOtherKind(cmd *cobra.Command, kind string) error {
	for _, group := range searchFlagKinds {
		if group.kind == kind {
			continue
		}
		for _, name := range group.flags {
			if cmd.Flags().Changed(name) {
				return usagef("--%s applies to %s searches, not %s", name, group.kind, kind)
			}
		}
	}
	return nil
}

func (o savedSearchSaveOptions) filters() map[string]any {
	filters := map[string]any{}
	if len(o.columns) > 0 {
		filters["columns"] = o.columns
	}
	for name, value := range map[string]string{"service": o.service, "span": o.span, "min": o.min, "max": o.max} {
		if value != "" {
			filters[name] = value
		}
	}
	if o.errors {
		filters["errors"] = true
	}
	if o.mode != "" {
		filters["mode"] = o.mode
	}
	return filters
}

func (a *app) runSavedSearchesSave(cmd *cobra.Command, _ []string) error {
	opts := a.savedSearchSave
	kind, err := searchKind(opts.kind, true)
	if err != nil {
		return err
	}
	if err := refuseFlagsOfOtherKind(cmd, kind); err != nil {
		return err
	}
	if strings.TrimSpace(opts.title) == "" {
		return usagef("--title <title> is required")
	}
	if opts.datasource == "" {
		return usagef("--datasource <uid> is required; observe doctor lists them")
	}
	folder, err := optionalID(opts.folder)
	if err != nil {
		return err
	}
	payload := map[string]any{"kind": kind, "title": opts.title, "datasource_uid": opts.datasource, "from": opts.from, "to": opts.to}
	if opts.query != "" {
		payload["query"] = opts.query
	}
	if filters := opts.filters(); len(filters) > 0 {
		payload["filters"] = filters
	}
	if folder != "" {
		payload["folder_id"] = folder
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "saved-searches", payload)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	saved, err := decode[savedSearchRow](data)
	if err != nil {
		return err
	}
	if saved.Replayed {
		out.notice("Search %s was already saved", text(saved.ID))
	} else {
		out.notice("Saved search %s", text(saved.ID))
	}
	out.line("%s", text(saved.URL))
	return nil
}

func (a *app) savedSearchesUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <saved-search-id> [flags]",
		Short: "Change the query, filters or range of a saved search",
		Long:  "update replaces what the saved search runs and keeps its kind, datasource, title and folder.\nOnly the flags you give change: --query and --from or --to replace one value each, and any filter flag (--columns, --service, --span, --min, --max, --errors, --mode) replaces all the filters, so repeat the ones to keep.\n--clear-filters removes every filter, and --query \"\" removes the query of a trace search.\nThe result must pass the checks a save does, and it needs Edit on the saved search.",
		Example: "  observe saved-searches update <id> --query '{app=\"checkout\"} |= \"timeout\"' --from now-6h\n" +
			"  observe saved-searches update <id> --columns timestamp,line\n" +
			"  observe saved-searches update <id> --min 500ms --errors",
		Args: exactArgs(1, "one saved search id"),
		RunE: a.runSavedSearchesUpdate,
	}
	flags := cmd.Flags()
	opts := &a.savedSearchUpdate
	flags.StringVar(&opts.from, "from", "", "start of the range, kept as written")
	flags.StringVar(&opts.to, "to", "", "end of the range, kept as written")
	addSearchContentFlags(cmd, &opts.savedSearchSaveOptions, "the new query")
	flags.BoolVar(&opts.clearFilters, "clear-filters", false, "remove every filter")
	return cmd
}

var searchFilterFlags = []string{"columns", "service", "span", "min", "max", "errors", "mode"}

func (o savedSearchUpdateOptions) payload(cmd *cobra.Command) (map[string]any, error) {
	payload := map[string]any{}
	for name, value := range map[string]string{"query": o.query, "from": o.from, "to": o.to} {
		if cmd.Flags().Changed(name) {
			payload[name] = value
		}
	}
	filtersGiven := false
	for _, name := range searchFilterFlags {
		filtersGiven = filtersGiven || cmd.Flags().Changed(name)
	}
	switch {
	case o.clearFilters && filtersGiven:
		return nil, usagef("--clear-filters cannot be combined with a filter flag")
	case o.clearFilters:
		payload["filters"] = nil
	case filtersGiven:
		payload["filters"] = o.filters()
	}
	if len(payload) == 0 {
		return nil, usagef("give at least one of --query, --from, --to, a filter flag or --clear-filters")
	}
	return payload, nil
}

func (a *app) runSavedSearchesUpdate(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	payload, err := a.savedSearchUpdate.payload(cmd)
	if err != nil {
		return err
	}
	_, data, err := a.patchSavedSearch(cmd, id, payload)
	if err != nil {
		return err
	}
	a.reportSavedSearch(data, func(out *output) { out.confirm("Updated saved search %s", text(id)) })
	return nil
}

func (a *app) savedSearchesRenameCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename [flags] <saved-search-id> <title...>",
		Short: "Rename a saved search",
		Long:  "Everything after the id is the new title, flag-like words included, so flags such as --json or -q must come before the id.\nIt needs Edit on the saved search.",
		Args:  cobra.ArbitraryArgs,
		RunE:  a.runSavedSearchesRename,
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func (a *app) runSavedSearchesRename(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return usagef("%s takes a saved search id and the new title", cmd.CommandPath())
	}
	title := strings.TrimSpace(strings.Join(args[1:], " "))
	if title == "-h" || title == "--help" {
		return cmd.Help()
	}
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	if title == "" {
		return usagef("rename needs a title")
	}
	renamed, data, err := a.patchSavedSearch(cmd, id, map[string]any{"title": title})
	if err != nil {
		return err
	}
	a.reportSavedSearch(data, func(out *output) { out.confirm("Renamed saved search %s to %s", text(id), text(renamed.Title)) })
	return nil
}

func (a *app) savedSearchesMoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "move <saved-search-id> --folder <folder-id>",
		Short: "Move a saved search into another folder",
		Long:  "move files the saved search in the folder, which shares it with that folder's readers.\nIt needs Edit on the saved search and on the folder, and My Dashboards takes no moves; save a search there instead.",
		Args:  exactArgs(1, "one saved search id"),
		RunE:  a.runSavedSearchesMove,
	}
	cmd.Flags().StringVar(&a.savedSearchMove.folder, "folder", "", "the folder id to move the saved search into")
	return cmd
}

func (a *app) runSavedSearchesMove(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	if a.savedSearchMove.folder == "" {
		return usagef("--folder <folder-id> is required")
	}
	folder, err := parseID(a.savedSearchMove.folder)
	if err != nil {
		return err
	}
	moved, data, err := a.patchSavedSearch(cmd, id, map[string]any{"folder_id": folder})
	if err != nil {
		return err
	}
	a.reportSavedSearch(data, func(out *output) {
		out.confirm("Moved saved search %s to %s", text(moved.Title), text(moved.Folder.Title))
	})
	return nil
}

func (a *app) patchSavedSearch(cmd *cobra.Command, id string, payload map[string]any) (savedSearchRow, []byte, error) {
	c, err := a.newClient()
	if err != nil {
		return savedSearchRow{}, nil, err
	}
	data, err := c.patch(cmd.Context(), "saved-searches/"+id, payload)
	if err != nil {
		return savedSearchRow{}, nil, err
	}
	row, err := decode[savedSearchRow](data)
	return row, data, err
}

func (a *app) reportSavedSearch(data []byte, confirm func(*output)) {
	out := a.output()
	if a.json {
		out.body(data)
		return
	}
	confirm(out)
}

func (a *app) savedSearchesDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <saved-search-id>",
		Short: "Delete a saved search",
		Long:  "delete removes the saved search for everyone who could read it; the deletion stays in the folder's activity.\nIt needs Edit on the saved search.",
		Args:  exactArgs(1, "one saved search id"),
		RunE:  a.runSavedSearchesDelete,
	}
}

func (a *app) runSavedSearchesDelete(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	if _, err := c.delete(cmd.Context(), "saved-searches/"+id); err != nil {
		return err
	}
	if !a.json {
		a.output().confirm("Deleted saved search %s", text(id))
	}
	return nil
}
