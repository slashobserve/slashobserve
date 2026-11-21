package cli

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const maxChatPage = 50

type chatTurnRow struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Prompt string `json:"prompt"`
}

type chatRow struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	DashboardID   *string      `json:"dashboard_id"`
	ObservationID *string      `json:"observation_id"`
	PinnedAt      *string      `json:"pinned_at"`
	LastTurnAt    string       `json:"last_turn_at"`
	Turns         int          `json:"turns"`
	LastTurn      *chatTurnRow `json:"last_turn"`
	Owner         chatOwnerRow `json:"owner"`
	ReadOnly      bool         `json:"read_only"`
}

type chatOwnerRow struct {
	Name string `json:"name"`
}

type chatListBody struct {
	Chats      []chatRow `json:"chats"`
	NextCursor *string   `json:"next_cursor"`
}

type chatDetailBody struct {
	Chat  chatRow       `json:"chat"`
	Turns []chatTurnRow `json:"turns"`
}

type chatListOptions struct {
	scope       string
	query       string
	pinned      bool
	dashboard   string
	observation string
}

func (a *app) chatsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chats",
		Short: "List, read and archive your chats with the agent",
		Long:  "chats works the chats you had with the built-in agent: each prompt you send is a turn of one.\nYou and Admins can read your chats, and nobody else sees them; an Admin reads anyone's read only.\nobserve --chat <id> asks the next turn of a chat.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.chatsListCommand(), a.chatsShowCommand(), a.chatsArchiveCommand())
	return cmd
}

func (a *app) chatsListCommand() *cobra.Command {
	var opts chatListOptions
	var page pageOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your chats, pinned ones first, then the one most recently asked in",
		Long:  "list prints your chats, pinned ones first and then the one most recently asked in, with how many turns each has and the state of its last turn.\n--scope all orders everyone's chats by their last turn only.\nArchived chats are left out.",
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
			data, err := c.getPages(cmd.Context(), "chats", query, page.all)
			if err != nil {
				return err
			}
			out := a.output()
			if a.json {
				out.body(data)
				return nil
			}
			list, err := decode[chatListBody](data)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(list.Chats))
			for _, chat := range list.Chats {
				state := "-"
				if chat.LastTurn != nil {
					state = requestStateWord(chat.LastTurn.State)
				}
				row := []string{text(chat.ID)}
				if opts.everyone() {
					row = append(row, text(chat.Owner.Name))
				}
				rows = append(rows, append(row, state, strconv.Itoa(chat.Turns), when(chat.LastTurnAt), firstLine(chat.Title)))
			}
			header := []string{"ID", "LAST TURN", "TURNS", "ASKED", "TITLE"}
			if opts.everyone() {
				header = []string{"ID", "OWNER", "LAST TURN", "TURNS", "ASKED", "TITLE"}
			}
			out.table(header, rows)
			pageFooter(out, len(list.Chats), nil, list.NextCursor)
			return nil
		},
	}
	addPageFlags(cmd, &page, maxChatPage)
	flags := cmd.Flags()
	flags.StringVar(&opts.scope, "scope", "", "mine (default) or all, every person's chats, which needs the Admin role")
	flags.StringVar(&opts.query, "q", "", "keep chats whose title or any prompt contains this text")
	flags.BoolVar(&opts.pinned, "pinned", false, "keep only pinned chats, or only unpinned ones with --pinned=false")
	flags.StringVar(&opts.dashboard, "dashboard", "", "keep only the chats about this dashboard id")
	flags.StringVar(&opts.observation, "observation", "", "keep only the chats about this observation id")
	return cmd
}

func (o chatListOptions) everyone() bool {
	return o.scope == "all"
}

func (o chatListOptions) apply(cmd *cobra.Command, query url.Values) error {
	if o.scope != "" {
		if !slices.Contains(requestScopes, o.scope) {
			return usagef("--scope must be one of %s", strings.Join(requestScopes, ", "))
		}
		query.Set("scope", o.scope)
	}
	if o.query != "" {
		query.Set("q", o.query)
	}
	if cmd.Flags().Changed("pinned") {
		query.Set("pinned", strconv.FormatBool(o.pinned))
	}
	for field, raw := range map[string]string{"dashboard_id": o.dashboard, "observation_id": o.observation} {
		if raw == "" {
			continue
		}
		id, err := parseID(raw)
		if err != nil {
			return err
		}
		query.Set(field, id)
	}
	return nil
}

func (a *app) chatsShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <chat-id>",
		Short: "Print one of your chats, or as an Admin anyone's, with every turn, oldest first",
		Long:  "show prints one of your chats and its turns, oldest first, each with its request id, state and prompt.\nAn Admin may show anyone's chat, read only, and the read is recorded in the audit trail.\nobserve status <request-id> prints one turn in full.",
		Args:  exactArgs(1, "one chat id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			data, err := c.get(cmd.Context(), "chats/"+id, nil)
			if err != nil {
				return err
			}
			out := a.output()
			if a.json {
				out.body(data)
				return nil
			}
			detail, err := decode[chatDetailBody](data)
			if err != nil {
				return err
			}
			out.fields([][2]string{
				{"Chat", text(detail.Chat.ID)},
				{"Owner", text(detail.Chat.Owner.Name)},
				{"Title", firstLine(detail.Chat.Title)},
				{"Dashboard", orDash(detail.Chat.DashboardID)},
				{"Observation", orDash(detail.Chat.ObservationID)},
				{"Pinned", orDash(detail.Chat.PinnedAt)},
			})
			rows := make([][]string, 0, len(detail.Turns))
			for _, turn := range detail.Turns {
				rows = append(rows, []string{text(turn.ID), requestStateWord(turn.State), firstLine(turn.Prompt)})
			}
			out.line("")
			out.table([]string{"REQUEST", "STATE", "PROMPT"}, rows)
			return nil
		},
	}
}

func (a *app) chatsArchiveCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "archive <chat-id> --yes",
		Short: "Archive one of your chats and its turns so they leave your lists",
		Long:  "archive removes one of your chats, and every request in it, from chats list and requests list.\nThe requests stay readable with observe status, and the dashboards and observations they made are kept.\nOnly the owner of a chat can archive it; an Admin who can read it gets chat_read_only.\nIt needs --yes, because nothing brings an archived chat back to the lists.",
		Args:  exactArgs(1, "one chat id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if !yes {
				return usagef("archiving chat %s removes it and every request in it from your lists and cannot be undone; rerun with --yes", id)
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			data, err := c.do(cmd.Context(), http.MethodDelete, "chats/"+id, nil, nil)
			if err != nil {
				return err
			}
			out := a.output()
			if a.json {
				out.body(data)
				return nil
			}
			out.confirm("Archived chat %s", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm that the chat and its requests leave your lists")
	return cmd
}
