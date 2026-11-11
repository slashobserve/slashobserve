package cli

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

const maxContentColumn = 80

type observationRow struct {
	ID        string                `json:"id"`
	Title     string                `json:"title"`
	State     string                `json:"state"`
	Summary   *string               `json:"summary"`
	URL       string                `json:"url"`
	UpdatedAt string                `json:"updated_at"`
	Blocks    []observationBlockRow `json:"blocks"`
}

type observationBlockRow struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	State    string          `json:"state"`
	Position int             `json:"position"`
	Width    string          `json:"width"`
	Version  int             `json:"version"`
	Body     json.RawMessage `json:"body"`
}

type observationEnvelope struct {
	Observation observationRow `json:"observation"`
}

type observationListBody struct {
	Observations []observationRow `json:"observations"`
	NextCursor   *string          `json:"next_cursor"`
}

type blockEnvelope struct {
	Block observationBlockRow `json:"block"`
}

type textSource struct {
	text string
	file string
}

func (s textSource) markdown() (string, bool, error) {
	switch {
	case s.text != "" && s.file != "":
		return "", false, usagef("--text and --text-file cannot be used together")
	case s.file != "":
		raw, err := readUserFile(s.file)
		return string(raw), true, err
	}
	return s.text, s.text != "", nil
}

func readUserFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // reading the file the user named is the command's purpose
	if err != nil {
		return nil, usagef("cannot read %s: %v", text(path), err)
	}
	return raw, nil
}

func (a *app) observationsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "o",
		Short: "Work with observations",
		Long:  "o creates, reads, extends and resolves observations: pages of Markdown, panel and snapshot blocks about one question.\nA block that someone who can only comment adds is requested until a person who can edit keeps or discards it.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.observationNewCommand(), a.observationShowCommand(), a.observationAddCommand(), a.observationResolveCommand(), a.observationListCommand(), a.observationKeepCommand(), a.observationDiscardCommand())
	return cmd
}

func (a *app) observationNewCommand() *cobra.Command {
	var folder, from, to string
	var source textSource
	cmd := &cobra.Command{
		Use:   "new <title...> [--folder <id>] [--from <time> --to <time>] [--text <markdown> | --text-file <path>]",
		Short: "Start an observation",
		Long:  "new starts an observation in a folder, by default your My Dashboards, and prints its address.\nWith --text or --text-file it also adds the first text block.\n--from and --to, RFC 3339 times given together, set the range its panels show.",
		Args:  minimumArgs(1, "a title"),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"title": strings.Join(args, " ")}
			if folder != "" {
				id, err := parseID(folder)
				if err != nil {
					return err
				}
				body["folder_id"] = id
			}
			if (from == "") != (to == "") {
				return usagef("--from and --to are given together")
			}
			if from != "" {
				body["from"], body["to"] = from, to
			}
			markdown, given, err := source.markdown()
			if err != nil {
				return err
			}
			if given {
				body["first_block"] = map[string]any{"kind": "text", "body": map[string]string{"markdown": markdown}}
			}
			return a.observationCall(cmd, http.MethodPost, "observations", body, func(out *output, data []byte) error {
				created, err := decode[observationEnvelope](data)
				if err != nil {
					return err
				}
				out.line("Created %s %s", text(created.Observation.Title), text(created.Observation.URL))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&folder, "folder", "", "the folder id to file it in")
	cmd.Flags().StringVar(&from, "from", "", "start of the range its panels show, an RFC 3339 time")
	cmd.Flags().StringVar(&to, "to", "", "end of the range its panels show, an RFC 3339 time")
	cmd.Flags().StringVar(&source.text, "text", "", "Markdown for a first text block")
	cmd.Flags().StringVar(&source.file, "text-file", "", "read the Markdown of a first text block from this file")
	return cmd
}

func (a *app) observationShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <observation-id>",
		Short: "Show an observation and its blocks",
		Long:  "show prints the title, state, summary and address of an observation, then one line for each block with its position, id, kind, state, version, width and, for text, its first line.",
		Args:  exactArgs(1, "one observation id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			data, err := c.get(cmd.Context(), "observations/"+id, nil)
			if err != nil {
				return err
			}
			out := a.output()
			if a.json {
				out.body(data)
				return nil
			}
			shown, err := decode[observationEnvelope](data)
			if err != nil {
				return err
			}
			printObservation(out, shown.Observation)
			return nil
		},
	}
}

func printObservation(out *output, o observationRow) {
	summary := "-"
	if o.Summary != nil {
		summary = text(*o.Summary)
	}
	out.fields([][2]string{{"Title", text(o.Title)}, {"State", text(o.State)}, {"Summary", summary}, {"URL", text(o.URL)}})
	rows := make([][]string, 0, len(o.Blocks))
	for _, b := range o.Blocks {
		rows = append(rows, []string{strconv.Itoa(b.Position), text(b.ID), text(b.Kind), text(b.State), strconv.Itoa(b.Version), text(b.Width), blockContent(b)})
	}
	if len(rows) > 0 {
		out.line("")
		out.table([]string{"POS", "ID", "KIND", "STATE", "VERSION", "WIDTH", "CONTENT"}, rows)
	}
}

func blockContent(b observationBlockRow) string {
	var body struct {
		Markdown string `json:"markdown"`
		Panel    struct {
			Title string `json:"title"`
		} `json:"panel"`
	}
	if json.Unmarshal(b.Body, &body) != nil {
		return "-"
	}
	switch {
	case b.Kind == "text":
		return firstLine(body.Markdown)
	case body.Panel.Title != "":
		return firstLine(body.Panel.Title)
	}
	return "-"
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	line = text(strings.TrimSpace(line))
	if utf8.RuneCountInString(line) <= maxContentColumn {
		return line
	}
	return string([]rune(line)[:maxContentColumn-3]) + "..."
}

type addBlockOptions struct {
	source     textSource
	panelFile  string
	snapshotOf string
	from, to   string
	half       bool
	position   int
}

func (o addBlockOptions) body(cmd *cobra.Command) (map[string]any, string, error) {
	markdown, hasText, err := o.source.markdown()
	if err != nil {
		return nil, "", err
	}
	given := 0
	for _, present := range []bool{hasText, o.panelFile != "", o.snapshotOf != ""} {
		if present {
			given++
		}
	}
	if given != 1 {
		return nil, "", usagef("give exactly one of --text, --text-file, --panel-file and --snapshot-of")
	}
	if (o.from != "" || o.to != "") && o.panelFile == "" {
		return nil, "", usagef("--from and --to go with --panel-file")
	}
	body := map[string]any{}
	var kind string
	switch {
	case hasText:
		kind, body["body"] = "text", map[string]string{"markdown": markdown}
	case o.panelFile != "":
		kind = "panel"
		panel, err := o.panel()
		if err != nil {
			return nil, "", err
		}
		body["body"] = panel
	default:
		id, err := parseID(o.snapshotOf)
		if err != nil {
			return nil, "", err
		}
		kind, body["from_block_id"] = "snapshot", id
	}
	body["kind"] = kind
	if o.half {
		body["width"] = "half"
	}
	if cmd.Flags().Changed("position") {
		body["position"] = o.position
	}
	return body, kind, nil
}

func (o addBlockOptions) panel() (map[string]any, error) {
	raw, err := readUserFile(o.panelFile)
	if err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, usagef("%s must hold one JSON object, the panel", text(o.panelFile))
	}
	if (o.from == "") != (o.to == "") {
		return nil, usagef("--from and --to are given together")
	}
	body := map[string]any{"panel": json.RawMessage(raw)}
	if o.from != "" {
		body["from"], body["to"] = o.from, o.to
	}
	return body, nil
}

func (a *app) observationAddCommand() *cobra.Command {
	var opts addBlockOptions
	cmd := &cobra.Command{
		Use:   "add <observation-id> (--text <markdown> | --text-file <path> | --panel-file <path> [--from <time> --to <time>] | --snapshot-of <block-id>) [--half] [--position <n>]",
		Short: "Add a block to an observation",
		Long:  "add adds one block: Markdown text, a panel from a JSON file, whose query must return data, or a snapshot that freezes the data of a panel block.\nThe block is live when you can edit the observation and requested when you can only comment.\n--from and --to, RFC 3339 times given together, give a panel its own range.",
		Args:  exactArgs(1, "one observation id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			body, kind, err := opts.body(cmd)
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodPost, "observations/"+id+"/blocks", body, func(out *output, data []byte) error {
				added, err := decode[blockEnvelope](data)
				if err != nil {
					return err
				}
				out.confirm("Added %s block %s (%s)", kind, text(added.Block.ID), blockStateWord(added.Block.State))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&opts.source.text, "text", "", "the Markdown of a text block")
	cmd.Flags().StringVar(&opts.source.file, "text-file", "", "read the Markdown of a text block from this file")
	cmd.Flags().StringVar(&opts.panelFile, "panel-file", "", "read the panel of a panel block, a JSON object, from this file")
	cmd.Flags().StringVar(&opts.snapshotOf, "snapshot-of", "", "the id of the panel block to freeze into a snapshot block")
	cmd.Flags().StringVar(&opts.from, "from", "", "start of the panel's own range, an RFC 3339 time")
	cmd.Flags().StringVar(&opts.to, "to", "", "end of the panel's own range, an RFC 3339 time")
	cmd.Flags().BoolVar(&opts.half, "half", false, "make the block half width")
	cmd.Flags().IntVar(&opts.position, "position", 0, "place the block at this position, by default the end")
	return cmd
}

func blockStateWord(state string) string {
	if state == "requested" {
		return "requested"
	}
	return "live"
}

func (a *app) observationResolveCommand() *cobra.Command {
	var summary string
	cmd := &cobra.Command{
		Use:   "resolve <observation-id> --summary <line>",
		Short: "Close an observation with a one-line summary",
		Args:  exactArgs(1, "one observation id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if summary == "" {
				return usagef("--summary <line> is required")
			}
			return a.observationCall(cmd, http.MethodPost, "observations/"+id+"/resolve", map[string]string{"summary": summary}, func(out *output, data []byte) error {
				resolved, err := decode[observationEnvelope](data)
				if err != nil {
					return err
				}
				out.confirm("Resolved %s", text(resolved.Observation.Title))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&summary, "summary", "", "one line on what was found")
	return cmd
}

func (a *app) observationListCommand() *cobra.Command {
	var folder, state string
	var page pageOptions
	cmd := &cobra.Command{
		Use:   "list [--folder <id>] [--state open|resolved]",
		Short: "List observations, most recently changed first",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := page.query()
			if folder != "" {
				id, err := parseID(folder)
				if err != nil {
					return err
				}
				query.Set("folder_id", id)
			}
			if state != "" {
				if state != "open" && state != "resolved" {
					return usagef("--state must be open or resolved")
				}
				query.Set("state", state)
			}
			return a.observationList(cmd, query)
		},
	}
	addPageFlags(cmd, &page)
	cmd.Flags().StringVar(&folder, "folder", "", "only observations in this folder id")
	cmd.Flags().StringVar(&state, "state", "", "only open or resolved observations")
	return cmd
}

func (a *app) observationList(cmd *cobra.Command, query url.Values) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "observations", query)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	page, err := decode[observationListBody](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(page.Observations))
	for _, o := range page.Observations {
		rows = append(rows, []string{text(o.ID), text(o.Title), text(o.State), when(o.UpdatedAt)})
	}
	out.table([]string{"ID", "TITLE", "STATE", "UPDATED"}, rows)
	nextPageHint(out, page.NextCursor)
	return nil
}

func (a *app) observationKeepCommand() *cobra.Command {
	var version int
	cmd := &cobra.Command{
		Use:   "keep <block-id> --version <n>",
		Short: "Keep a requested block",
		Long:  "keep makes a requested block live for everyone who sees the observation.\n--version is the block's version from observe o show; a block that changed since answers version_conflict.",
		Args:  exactArgs(1, "one block id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if version < 1 {
				return usagef("--version <n> is required, the block's version from observe o show")
			}
			return a.observationCall(cmd, http.MethodPost, "blocks/"+id+"/keep", map[string]int{"version": version}, func(out *output, data []byte) error {
				kept, err := decode[blockEnvelope](data)
				if err != nil {
					return err
				}
				out.confirm("Kept %s block %s", text(kept.Block.Kind), text(kept.Block.ID))
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&version, "version", 0, "the block's version")
	return cmd
}

func (a *app) observationDiscardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "discard <block-id>",
		Short: "Discard a requested block",
		Long:  "discard deletes a requested block.\nAn editor may discard any requested block, and the one who asked for it may discard theirs.",
		Args:  exactArgs(1, "one block id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodPost, "blocks/"+id+"/discard", nil, func(out *output, _ []byte) error {
				out.confirm("Discarded block %s", id)
				return nil
			})
		},
	}
}

func (a *app) observationCall(cmd *cobra.Command, method, path string, payload any, show func(*output, []byte) error) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	var data []byte
	if payload == nil {
		data, err = c.do(cmd.Context(), method, path, nil, nil)
	} else {
		data, err = c.send(cmd.Context(), method, path, payload)
	}
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	return show(out, data)
}
