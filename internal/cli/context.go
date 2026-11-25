package cli

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type contextWriteOptions struct {
	name, description, address, audience, secretHeader string
	secretStdin, clearSecret                           bool
}

type contextApproveOptions struct {
	allReadOnly bool
}

type contextSource struct {
	UID          string  `json:"uid"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	URL          string  `json:"url"`
	Audience     string  `json:"audience"`
	SecretHeader *string `json:"secret_header"`
	SecretSet    bool    `json:"secret_set"`
	Provisioned  bool    `json:"provisioned"`
	Private      bool    `json:"private"`
	State        string  `json:"state"`
	Failure      *string `json:"failure"`
	CheckedAt    *string `json:"checked_at"`
	Tools        struct {
		Offered  int `json:"offered"`
		Approved int `json:"approved"`
		Changed  int `json:"changed"`
		Gone     int `json:"gone"`
		Refused  int `json:"refused"`
	} `json:"tools"`
	UpdatedAt string `json:"updated_at"`
}

type contextToolDefinition struct {
	Hash string `json:"hash"`
}

type contextTool struct {
	Name        string                 `json:"name"`
	State       string                 `json:"state"`
	Refusal     *string                `json:"refusal"`
	Description string                 `json:"description"`
	Hash        string                 `json:"hash"`
	Seen        *contextToolDefinition `json:"seen"`
}

func (t contextTool) offeredHash() string {
	if t.Seen != nil {
		return t.Seen.Hash
	}
	return t.Hash
}

func (a *app) contextCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Register MCP servers as context sources and approve their tools",
		Long:  "context manages the MCP servers the agent may use for outside context.\nEvery command needs an Admin key.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.contextListCommand(), a.contextAddCommand(), a.contextEditCommand(), a.contextRemoveCommand(), a.contextDiscoverCommand(), a.contextToolsCommand(), a.contextApproveCommand(), a.contextRevokeCommand())
	return cmd
}

func (o *contextWriteOptions) register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&o.name, "name", "", "the display name; the uid when empty")
	flags.StringVar(&o.description, "description", "", "what the source is for, on one line; an empty value removes it on edit")
	flags.StringVar(&o.address, "url", "", "the https address of the MCP server")
	flags.StringVar(&o.audience, "audience", "", "who can use its tools: everyone, the default, or admins")
	flags.StringVar(&o.secretHeader, "secret-header", "", "the header that carries the secret, such as Authorization")
	flags.BoolVar(&o.secretStdin, "secret-stdin", false, "read the secret from standard input")
}

func (a *app) contextListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list [uid]",
		Short: "List the context sources, or show one",
		Args:  rangeArgs(0, 1, "at most one context source uid"),
		RunE:  a.runContextList,
	}
}

func (a *app) contextAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add <uid> --url <https address> [--secret-header <name> --secret-stdin]",
		Short:   "Register an MCP server",
		Long:    "add registers an MCP server.\nIt does not contact the server: run discover, then approve its tools.\nThe server's domain must be on the outbound domain list.",
		Example: "  printf '%s' \"$TOKEN\" | observe context add docs --url https://docs.example.com/mcp --secret-header Authorization --secret-stdin",
		Args:    exactArgs(1, "a context source uid"),
		RunE:    a.runContextAdd,
	}
	a.contextAdd.register(cmd)
	return cmd
}

func (a *app) contextEditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <uid>",
		Short: "Change a context source",
		Long:  "edit changes the flags you pass and leaves the rest.\nA new --url drops the approved tools, so approvals never carry over to another server.",
		Args:  exactArgs(1, "a context source uid"),
		RunE:  a.runContextEdit,
	}
	a.contextEdit.register(cmd)
	cmd.Flags().BoolVar(&a.contextEdit.clearSecret, "clear-secret", false, "remove the stored secret")
	return cmd
}

func (a *app) contextRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <uid>",
		Short: "Remove a context source, its approved tools and its secret",
		Args:  exactArgs(1, "a context source uid"),
		RunE:  a.runContextRemove,
	}
}

func (a *app) contextDiscoverCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "discover <uid>",
		Short: "Ask the server for its tools",
		Long:  "discover lists the server's tools and compares them with what was approved.\nA tool that changed stops being served until you approve it again.\nNothing new is approved.",
		Args:  exactArgs(1, "a context source uid"),
		RunE:  a.runContextDiscover,
	}
}

func (a *app) contextToolsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tools <uid>",
		Short: "List the tools discovery found",
		Args:  exactArgs(1, "a context source uid"),
		RunE:  a.runContextTools,
	}
}

func (a *app) contextApproveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve <uid> [tool | --all-read-only]",
		Short: "Approve one tool, or every read-only tool",
		Long:  "approve pins a tool as it is described now.\nWith --all-read-only it approves every offered tool the server marks read-only.\nA tool marked destructive, or not marked at all, is refused.",
		Example: "  observe context approve docs search\n" +
			"  observe context approve docs --all-read-only",
		Args: rangeArgs(1, 2, "a context source uid and a tool name, or --all-read-only"),
		RunE: a.runContextApprove,
	}
	cmd.Flags().BoolVar(&a.contextApprove.allReadOnly, "all-read-only", false, "approve every offered read-only tool")
	return cmd
}

func (a *app) contextRevokeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <uid> <tool>",
		Short: "Withdraw the approval of a tool",
		Args:  exactArgs(2, "a context source uid and a tool name"),
		RunE:  a.runContextRevoke,
	}
}

func contextPath(uid string, rest ...string) string {
	return strings.Join(append([]string{"context-sources", url.PathEscape(uid)}, rest...), "/")
}

func (a *app) runContextList(cmd *cobra.Command, args []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	if len(args) == 1 {
		data, err := c.get(cmd.Context(), contextPath(args[0]), nil)
		if err != nil {
			return err
		}
		if a.json {
			a.output().body(data)
			return nil
		}
		source, err := decode[contextSource](data)
		if err != nil {
			return err
		}
		a.output().fields(contextFields(source))
		return nil
	}
	data, err := c.get(cmd.Context(), "context-sources", nil)
	if err != nil {
		return err
	}
	if a.json {
		a.output().body(data)
		return nil
	}
	list, err := decode[struct {
		Sources []contextSource `json:"context_sources"`
	}](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(list.Sources))
	for _, s := range list.Sources {
		rows = append(rows, []string{text(s.UID), text(s.Name), text(s.State), text(s.Audience), contextToolSummary(s), text(s.URL)})
	}
	a.output().table([]string{"UID", "NAME", "STATE", "AUDIENCE", "TOOLS", "URL"}, rows)
	return nil
}

func contextToolSummary(s contextSource) string {
	parts := []string{strconv.Itoa(s.Tools.Approved) + " approved"}
	for _, extra := range []struct {
		count int
		label string
	}{{s.Tools.Offered, "offered"}, {s.Tools.Changed, "changed"}, {s.Tools.Gone, "gone"}, {s.Tools.Refused, "refused"}} {
		if extra.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", extra.count, extra.label))
		}
	}
	return strings.Join(parts, ", ")
}

func contextFields(s contextSource) [][2]string {
	secret := "none"
	if s.SecretSet {
		secret = "set"
		if s.SecretHeader != nil {
			secret += " in " + text(*s.SecretHeader)
		}
	}
	pairs := [][2]string{{"UID", text(s.UID)}, {"Name", text(s.Name)}}
	if s.Description != nil {
		pairs = append(pairs, [2]string{"Description", text(*s.Description)})
	}
	pairs = append(pairs,
		[2]string{"URL", text(s.URL)}, [2]string{"Audience", text(s.Audience)},
		[2]string{"State", text(s.State)}, [2]string{"Tools", contextToolSummary(s)}, [2]string{"Secret", secret},
	)
	if s.Failure != nil {
		pairs = append(pairs, [2]string{"Failure", text(*s.Failure)})
	}
	if s.CheckedAt != nil {
		pairs = append(pairs, [2]string{"Checked", when(*s.CheckedAt)})
	}
	if s.Provisioned {
		pairs = append(pairs, [2]string{"Managed by", "the provisioning file"})
	}
	if s.Private {
		pairs = append(pairs, [2]string{"Network", "private addresses allowed by the provisioning file"})
	}
	return append(pairs, [2]string{"Updated", when(s.UpdatedAt)})
}

func (a *app) contextBody(cmd *cobra.Command, opts contextWriteOptions) (map[string]any, error) {
	flags := cmd.Flags()
	body := map[string]any{}
	for flag, entry := range map[string]struct {
		key   string
		value string
	}{
		"name": {"name", opts.name}, "description": {"description", opts.description}, "url": {"url", opts.address}, "audience": {"audience", opts.audience}, "secret-header": {"secret_header", opts.secretHeader},
	} {
		if flags.Changed(flag) {
			body[entry.key] = entry.value
		}
	}
	if opts.secretStdin {
		stdinUsed := false
		secret, err := a.readDatasourceSecret(cmd.Context(), "-", &stdinUsed)
		if err != nil {
			return nil, err
		}
		body["secret"] = secret
	}
	return body, nil
}

func (a *app) runContextAdd(cmd *cobra.Command, args []string) error {
	opts := a.contextAdd
	if !cmd.Flags().Changed("url") {
		return usagef("pass the server's address with --url")
	}
	if opts.secretStdin != cmd.Flags().Changed("secret-header") {
		return usagef("--secret-header and --secret-stdin go together")
	}
	body, err := a.contextBody(cmd, opts)
	if err != nil {
		return err
	}
	body["uid"] = args[0]
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "context-sources", body)
	if err != nil {
		return err
	}
	if a.json {
		a.output().body(data)
		return nil
	}
	a.output().confirm("Registered context source %s; run \"observe context discover %s\" to list its tools", text(args[0]), text(args[0]))
	return nil
}

func (a *app) runContextEdit(cmd *cobra.Command, args []string) error {
	opts := a.contextEdit
	if opts.clearSecret && opts.secretStdin {
		return usagef("choose --secret-stdin or --clear-secret, not both")
	}
	body, err := a.contextBody(cmd, opts)
	if err != nil {
		return err
	}
	if opts.clearSecret {
		body["clear_secret"] = true
	}
	if len(body) == 0 {
		return usagef("pass at least one flag to change")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.patch(cmd.Context(), contextPath(args[0]), body)
	if err != nil {
		return err
	}
	if a.json {
		a.output().body(data)
		return nil
	}
	a.output().confirm("Updated context source %s", text(args[0]))
	return nil
}

func (a *app) runContextRemove(cmd *cobra.Command, args []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	if _, err := c.delete(cmd.Context(), contextPath(args[0])); err != nil {
		return err
	}
	if !a.json {
		a.output().confirm("Removed context source %s", text(args[0]))
	}
	return nil
}

func (a *app) runContextDiscover(cmd *cobra.Command, args []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), contextPath(args[0], "discover"), struct{}{})
	if err != nil {
		return err
	}
	if a.json {
		a.output().body(data)
		return nil
	}
	found, err := decode[struct {
		Source contextSource `json:"source"`
		Tally  struct {
			Found   int `json:"found"`
			New     int `json:"new"`
			Changed int `json:"changed"`
			Gone    int `json:"gone"`
			Ignored int `json:"ignored"`
		} `json:"tally"`
	}](data)
	if err != nil {
		return err
	}
	out := a.output()
	if found.Source.Failure != nil {
		return fmt.Errorf("discovery failed: %s", text(*found.Source.Failure))
	}
	out.line("Found %d tools: %d new, %d changed, %d gone, %d ignored", found.Tally.Found, found.Tally.New, found.Tally.Changed, found.Tally.Gone, found.Tally.Ignored)
	if found.Tally.Changed > 0 {
		out.notice("%d changed %s stopped being served until you approve %s again", found.Tally.Changed, plural(found.Tally.Changed, "tool", "tools"), plural(found.Tally.Changed, "it", "them"))
	}
	return nil
}

func (a *app) fetchContextTools(cmd *cobra.Command, c *client, uid string) ([]byte, []contextTool, error) {
	data, err := c.get(cmd.Context(), contextPath(uid, "tools"), nil)
	if err != nil {
		return nil, nil, err
	}
	list, err := decode[struct {
		Tools []contextTool `json:"tools"`
	}](data)
	return data, list.Tools, err
}

func (a *app) runContextTools(cmd *cobra.Command, args []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, tools, err := a.fetchContextTools(cmd, c, args[0])
	if err != nil {
		return err
	}
	if a.json {
		a.output().body(data)
		return nil
	}
	rows := make([][]string, 0, len(tools))
	for _, t := range tools {
		note := t.Description
		if t.Refusal != nil {
			note = "refused: " + *t.Refusal
		}
		rows = append(rows, []string{text(t.Name), text(t.State), firstLine(note)})
	}
	a.output().table([]string{"TOOL", "STATE", "NOTE"}, rows)
	return nil
}

func (a *app) runContextApprove(cmd *cobra.Command, args []string) error {
	all := a.contextApprove.allReadOnly
	switch {
	case all && len(args) != 1:
		return usagef("--all-read-only takes a context source uid and no tool name")
	case !all && len(args) != 2:
		return usagef("pass a tool name, or --all-read-only to approve every read-only tool")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if all {
		data, err := c.post(ctx, contextPath(args[0], "tools", "approve-read-only"), struct{}{})
		if err != nil {
			return err
		}
		if a.json {
			a.output().body(data)
			return nil
		}
		done, err := decode[struct {
			Approved []contextTool `json:"approved"`
		}](data)
		if err != nil {
			return err
		}
		a.output().confirm("Approved %d %s", len(done.Approved), plural(len(done.Approved), "tool", "tools"))
		return nil
	}
	_, tools, err := a.fetchContextTools(cmd, c, args[0])
	if err != nil {
		return err
	}
	var target *contextTool
	for i := range tools {
		if tools[i].Name == args[1] {
			target = &tools[i]
		}
	}
	if target == nil {
		return fmt.Errorf("context source %s has no tool %s; run \"observe context discover %s\" first", text(args[0]), text(args[1]), text(args[0]))
	}
	data, err := c.post(ctx, contextPath(args[0], "tools", url.PathEscape(args[1]), "approve"), map[string]string{"hash": target.offeredHash()})
	if err != nil {
		return contextRefusal(err)
	}
	if a.json {
		a.output().body(data)
		return nil
	}
	a.output().confirm("Approved %s", text(args[1]))
	return nil
}

func contextRefusal(err error) error {
	var api *apiError
	if !errors.As(err, &api) || api.code != "tool_refused" {
		return err
	}
	return fmt.Errorf("%w\nA tool the server marks destructive, or does not mark read-only, cannot be approved", err)
}

func (a *app) runContextRevoke(cmd *cobra.Command, args []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), contextPath(args[0], "tools", url.PathEscape(args[1]), "revoke"), struct{}{})
	if err != nil {
		return err
	}
	if a.json {
		a.output().body(data)
		return nil
	}
	a.output().confirm("Withdrew the approval of %s", text(args[1]))
	return nil
}
