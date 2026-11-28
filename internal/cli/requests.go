package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slashobserve/slashobserve/internal/anchor"
)

var (
	requestScopes = []string{"mine", "all"}
	requestStates = []string{"queued", "running", "canceling", "succeeded", "failed", "canceled"}
)

type requestRow struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Title          string   `json:"title"`
	Intent         string   `json:"intent"`
	State          string   `json:"state"`
	DashboardID    *string  `json:"dashboard_id"`
	DashboardTitle *string  `json:"dashboard_title"`
	CreatedAt      string   `json:"created_at"`
	AllowedActions []string `json:"allowed_actions"`

	Anchor  *anchor.Anchor `json:"anchor"`
	BlockID *string        `json:"block_id"`
}

type requestListBody struct {
	Requests   []requestRow `json:"requests"`
	NextCursor *string      `json:"next_cursor"`
	Summary    *struct {
		Total int `json:"total"`
	} `json:"summary"`
}

type requestListOptions struct {
	scope       string
	states      []string
	query       string
	pinned      bool
	dashboard   string
	observation string
	anchored    bool
}

func (a *app) requestsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "requests",
		Short: "List, pin, rename and archive agent requests",
		Long:  "requests works the agent requests you started: prompts, follow-ups, comment revisions and investigations.\nobserve status shows one request, and observe cancel, continue and retry act on it.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.requestsListCommand(), a.requestsPinCommand(true), a.requestsPinCommand(false), a.requestsRenameCommand(), a.requestsArchiveCommand())
	return cmd
}

func (a *app) requestsListCommand() *cobra.Command {
	var opts requestListOptions
	var page pageOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agent requests, newest first",
		Long:  "list prints your agent requests, newest first and pinned ones first, with their state, intent, dashboard and, in AT, the spot a question points at.\nArchived requests are left out.\n--scope all lists every principal's requests and needs the Admin role.\n--state may repeat or take a comma-separated list.\n--anchored keeps only your own requests that point at a spot, and --observation only the requests asked about one observation or written into it; --anchored with --scope all is refused.",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := page.check(); err != nil {
				return err
			}
			query := page.query()
			if err := opts.apply(cmd, query); err != nil {
				return err
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			data, err := c.getPages(cmd.Context(), "observe", query, page.all)
			if err != nil {
				return err
			}
			out := a.output()
			return render(out, data, func(list requestListBody) error {
				rows := make([][]string, 0, len(list.Requests))
				for _, r := range list.Requests {
					rows = append(rows, []string{text(r.ID), requestStateWord(r.State), text(r.Intent), orDash(r.DashboardTitle), anchorText(r.Anchor), text(r.Title)})
				}
				out.table([]string{"ID", "STATE", "INTENT", "DASHBOARD", "AT", "TITLE"}, rows)
				var total *int
				if list.Summary != nil {
					total = &list.Summary.Total
				}
				pageFooter(out, len(list.Requests), total, list.NextCursor)
				return nil
			})
		},
	}
	addPageFlags(cmd, &page, maxRequestPage)
	flags := cmd.Flags()
	flags.StringVar(&opts.scope, "scope", "", "mine (default) or all, which needs the Admin role")
	flags.StringSliceVar(&opts.states, "state", nil, "keep these states: queued, running, canceling, succeeded, failed or canceled")
	flags.StringVar(&opts.query, "q", "", "keep requests whose title or prompt contains this text")
	flags.BoolVar(&opts.pinned, "pinned", false, "keep only pinned requests, or only unpinned ones with --pinned=false")
	flags.StringVar(&opts.dashboard, "dashboard", "", "keep only the requests about this dashboard id")
	flags.StringVar(&opts.observation, "observation", "", "keep only the requests asked about or written into this observation id")
	flags.BoolVar(&opts.anchored, "anchored", false, "keep only your own requests that point at a spot; needs --scope mine, which is the default")
	return cmd
}

func (o requestListOptions) apply(cmd *cobra.Command, query url.Values) error {
	if o.scope != "" {
		if !slices.Contains(requestScopes, o.scope) {
			return usagef("--scope must be one of %s", strings.Join(requestScopes, ", "))
		}
		query.Set("scope", o.scope)
	}
	for _, state := range o.states {
		if !slices.Contains(requestStates, state) {
			return usagef("--state must be one of %s", strings.Join(requestStates, ", "))
		}
	}
	for _, state := range o.states {
		query.Add("state", state)
	}
	if o.query != "" {
		query.Set("q", o.query)
	}
	if cmd.Flags().Changed("pinned") {
		query.Set("pinned", strconv.FormatBool(o.pinned))
	}
	if o.dashboard != "" {
		id, err := parseID(o.dashboard)
		if err != nil {
			return err
		}
		query.Set("dashboard_id", id)
	}
	if o.observation != "" {
		id, err := parseID(o.observation)
		if err != nil {
			return err
		}
		query.Set("observation_id", id)
	}
	if o.anchored {
		query.Set("anchored", "true")
	}
	return nil
}

func anchorText(a *anchor.Anchor) string {
	if a == nil {
		return "-"
	}
	return text(a.Describe())
}

func (a *app) requestsArchiveCommand() *cobra.Command {
	var withObservation, yes bool
	cmd := &cobra.Command{
		Use:   "archive <request-id> [--delete-observation --yes]",
		Short: "Archive one of your agent requests so it leaves the list",
		Long:  "archive removes one of your agent requests from requests list.\nThe request stays readable with observe status, and archiving it again changes nothing.\nOnly the person who made the request can archive it.\n--delete-observation first deletes the observation the request saved, for everyone and for good, as observe o delete does and with its permission, and archives the request only when that succeeds; it needs --yes.",
		Args:  exactArgs(1, "one request id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if withObservation && !yes {
				return usagef("--delete-observation deletes the observation request %s saved for everyone, with its blocks, RCA revisions and discussion, and cannot be undone; rerun with --yes", id)
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			out := a.output()
			if withObservation {
				observation, err := savedObservation(cmd.Context(), c, id)
				if err != nil {
					return err
				}
				if _, err := c.do(cmd.Context(), http.MethodDelete, "observations/"+observation, nil, nil); err != nil {
					return fmt.Errorf("observation %s was not deleted, so request %s was not archived: %w", observation, id, err)
				}
				if !a.json {
					out.confirm("Deleted observation %s", text(observation))
				}
			}
			data, err := c.do(cmd.Context(), http.MethodDelete, "observe/"+id, nil, nil)
			if err != nil {
				return err
			}
			if a.json {
				out.body(data)
				return nil
			}
			out.confirm("Archived request %s", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&withObservation, "delete-observation", false, "also delete the observation the request saved, for everyone")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm that the observation is deleted for everyone")
	return cmd
}

func savedObservation(ctx context.Context, c *client, id string) (string, error) {
	data, err := c.get(ctx, "observe/"+id, nil)
	if err != nil {
		return "", err
	}
	envelope, err := decode[requestEnvelope](data)
	if err != nil {
		return "", err
	}
	if envelope.Request.ObservationID == nil {
		return "", fmt.Errorf("request %s saved no observation, so there is none to delete; rerun without --delete-observation", id)
	}
	return *envelope.Request.ObservationID, nil
}

func (a *app) requestsPinCommand(pin bool) *cobra.Command {
	use, short, long := "pin", "Pin one of your agent requests to the top of the list", "pin keeps one of your agent requests at the top of requests list.\nPinning a pinned request changes nothing.\nOnly the person who made the request can pin it."
	if !pin {
		use, short, long = "unpin", "Unpin one of your agent requests", "unpin returns one of your agent requests to its place by date in requests list.\nUnpinning an unpinned request changes nothing.\nOnly the person who made the request can unpin it."
	}
	return &cobra.Command{
		Use:   use + " <request-id>",
		Short: short,
		Long:  long,
		Args:  exactArgs(1, "one request id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.updateRequest(cmd, args[0], map[string]any{"pinned": pin}, use+"ned")
		},
	}
}

func (a *app) requestsRenameCommand() *cobra.Command {
	var clearTitle bool
	cmd := &cobra.Command{
		Use:   "rename <request-id> <title>",
		Short: "Give one of your agent requests a title",
		Long:  "rename sets the title requests list shows for one of your agent requests, 1 to 120 characters on one line.\n--clear takes the title back to the start of the prompt.\nOnly the person who made the request can rename it.\nRenaming the first request of a chat with no dashboard also renames the one observation made from that chat, when you may edit it.",
		Args: func(cmd *cobra.Command, args []string) error {
			if clearTitle && len(args) == 1 || !clearTitle && len(args) >= 2 {
				return nil
			}
			return usagef("%s takes a request id and a title, or a request id and --clear", cmd.CommandPath())
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			title := any(nil)
			if !clearTitle {
				title = strings.Join(args[1:], " ")
			}
			return a.updateRequest(cmd, args[0], map[string]any{"title": title}, "renamed")
		},
	}
	cmd.Flags().BoolVar(&clearTitle, "clear", false, "take the title back to the start of the prompt")
	return cmd
}

func (a *app) updateRequest(cmd *cobra.Command, rawID string, fields map[string]any, done string) error {
	id, err := parseID(rawID)
	if err != nil {
		return err
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("encode the request: %w", err)
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.do(cmd.Context(), http.MethodPatch, "observe/"+id, nil, body)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	changed, err := decode[struct {
		Request struct {
			ObservationRename string `json:"observation_rename"`
		} `json:"request"`
	}](data)
	if err != nil {
		return err
	}
	out.confirm("Request %s %s%s", id, done, observationRenameNote[changed.Request.ObservationRename])
	return nil
}

var observationRenameNote = map[string]string{
	"renamed": ", and the observation made from it",
	"kept":    "; the observation made from it keeps its title because you may not edit it",
}
