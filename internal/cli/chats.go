package cli

import (
	"net/http"
	"net/url"
	"strconv"

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
	query       string
	pinned      bool
	dashboard   string
	observation string
}

func (a *app) chatsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chats",
		Short: "List, read and archive your chats with the agent",
		Long:  "chats works the chats you had with the built-in agent: each prompt you send is a turn of one.\nOnly you can read your chats; an Admin cannot read another person's.\nobserve --chat <id> asks the next turn of a chat.",
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
		Short: "List your chats, the one most recently asked in first",
		Long:  "list prints your chats, the one most recently asked in first, with how many turns each has and the state of its last turn.\nArchived chats are left out.",
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
				rows = append(rows, []string{text(chat.ID), state, strconv.Itoa(chat.Turns), when(chat.LastTurnAt), firstLine(chat.Title)})
			}
			out.table([]string{"ID", "LAST TURN", "TURNS", "ASKED", "TITLE"}, rows)
			pageFooter(out, len(list.Chats), nil, list.NextCursor)
			return nil
		},
	}
	addPageFlags(cmd, &page, maxChatPage)
	flags := cmd.Flags()
	flags.StringVar(&opts.query, "q", "", "keep chats whose title or any prompt contains this text")
	flags.BoolVar(&opts.pinned, "pinned", false, "keep only pinned chats, or only unpinned ones with --pinned=false")
	flags.StringVar(&opts.dashboard, "dashboard", "", "keep only the chats about this dashboard id")
	flags.StringVar(&opts.observation, "observation", "", "keep only the chats about this observation id")
	return cmd
}

func (o chatListOptions) apply(cmd *cobra.Command, query url.Values) error {
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
		Short: "Print one of your chats with every turn, oldest first",
		Long:  "show prints one of your chats and its turns, oldest first, each with its request id, state and prompt.\nobserve status <request-id> prints one turn in full.",
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
	return &cobra.Command{
		Use:   "archive <chat-id>",
		Short: "Archive one of your chats and its turns so they leave your lists",
		Long:  "archive removes one of your chats, and every request in it, from chats list and requests list.\nThe requests stay readable with observe status, and the dashboards and observations they made are kept.\nOnly the owner of a chat can archive it.",
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
}
