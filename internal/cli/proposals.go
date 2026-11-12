package cli

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var (
	proposalStates = []string{"open", "merged", "discarded", "all"}
	choiceKinds    = []string{"panel", "variable", "setting"}
	choiceSides    = []string{"ours", "theirs"}
)

type proposalRow struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Title  string `json:"title"`
	Author struct {
		Name string `json:"name"`
	} `json:"author"`
	OriginClient   *string  `json:"origin_client"`
	DashboardID    string   `json:"dashboard_id"`
	Revision       int      `json:"revision"`
	BaseVersion    int      `json:"base_version"`
	LatestVersion  int      `json:"latest_version"`
	Summary        string   `json:"summary"`
	MergedVersion  *int     `json:"merged_version"`
	Behind         bool     `json:"behind"`
	AllowedActions []string `json:"allowed_actions"`
}

type proposalDetail struct {
	proposalRow
	Changes []struct {
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Change string `json:"change"`
	} `json:"changes"`
	Conflicts []struct {
		Kind    string `json:"kind"`
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"conflicts"`
}

type proposalListBody struct {
	Proposals  []proposalRow `json:"proposals"`
	NextCursor *string       `json:"next_cursor"`
}

type mergeChoice struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Take string `json:"take"`
}

type proposalMergeBody struct {
	Revision int           `json:"revision"`
	Choices  []mergeChoice `json:"choices,omitempty"`
}

func (a *app) proposalsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proposals",
		Short: "List, read, merge and discard proposed changes to dashboards",
		Long:  "proposals works the changes that someone who cannot edit a dashboard, or whose connection is in propose mode, saved for review.\nA person who can edit merges a proposal into the dashboard as a new version, or discards it.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.proposalsListCommand(), a.proposalsShowCommand(), a.proposalsMergeCommand(), a.proposalsDiscardCommand())
	return cmd
}

func (a *app) proposalsListCommand() *cobra.Command {
	var state string
	var page pageOptions
	cmd := &cobra.Command{
		Use:   "list <dashboard-id> [--state open|merged|discarded|all]",
		Short: "List the proposals on a dashboard, newest first",
		Long:  "list prints the open proposals of a dashboard, newest first, with their state, revision and whether the dashboard has moved on since they were based on it.\n--state merged, discarded or all shows the closed ones as well.",
		Args:  exactArgs(1, "one dashboard id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			query := page.query()
			if state != "" {
				if !slices.Contains(proposalStates, state) {
					return usagef("--state must be one of %s", strings.Join(proposalStates, ", "))
				}
				query.Set("state", state)
			}
			return a.proposalsList(cmd, id, query)
		},
	}
	addPageFlags(cmd, &page)
	cmd.Flags().StringVar(&state, "state", "", "open, merged, discarded or all; default open")
	return cmd
}

func (a *app) proposalsList(cmd *cobra.Command, dashboardID string, query url.Values) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "dashboards/"+dashboardID+"/proposals", query)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	page, err := decode[proposalListBody](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(page.Proposals))
	for _, p := range page.Proposals {
		rows = append(rows, []string{text(p.ID), text(p.State), strconv.Itoa(p.Revision), behindText(p.Behind), text(p.Title)})
	}
	out.table([]string{"ID", "STATE", "REVISION", "BEHIND", "TITLE"}, rows)
	nextPageHint(out, page.NextCursor)
	return nil
}

func behindText(behind bool) string {
	if behind {
		return "behind main"
	}
	return "-"
}

func (a *app) proposalsShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <proposal-id>",
		Short: "Show a proposal with its changes and conflicts",
		Long:  "show prints a proposal's summary, what it adds, changes, removes or moves compared with the version it is based on, the conflicts a merge onto the latest version would hit, and the actions you may take now.\nA merge needs the revision shown here.",
		Args:  exactArgs(1, "one proposal id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodGet, "proposals/"+id, nil, func(out *output, data []byte) error {
				shown, err := decode[proposalDetail](data)
				if err != nil {
					return err
				}
				printProposal(out, shown)
				return nil
			})
		},
	}
}

func printProposal(out *output, p proposalDetail) {
	base := "version " + strconv.Itoa(p.BaseVersion)
	if p.Behind {
		base += " (latest " + strconv.Itoa(p.LatestVersion) + ", behind main)"
	}
	pairs := [][2]string{
		{"Title", text(p.Title)},
		{"ID", text(p.ID)},
		{"Dashboard", text(p.DashboardID)},
		{"State", text(p.State)},
		{"Revision", strconv.Itoa(p.Revision)},
		{"Based on", base},
		{"Author", text(p.Author.Name)},
	}
	if p.OriginClient != nil {
		pairs = append(pairs, [2]string{"Via", text(*p.OriginClient)})
	}
	pairs = append(pairs, [2]string{"Summary", text(p.Summary)})
	if p.MergedVersion != nil {
		pairs = append(pairs, [2]string{"Merged as", "version " + strconv.Itoa(*p.MergedVersion)})
	}
	out.fields(pairs)
	if len(p.Changes) > 0 {
		out.line("")
		out.line("Changes")
		for _, c := range p.Changes {
			out.line("  %s %s %s", text(c.Change), text(c.Kind), text(c.ID))
		}
	}
	if len(p.Conflicts) > 0 {
		out.line("")
		out.line("Conflicts")
		for _, c := range p.Conflicts {
			out.line("  %s %s: %s", text(c.Kind), text(c.ID), text(c.Message))
		}
	}
	allowed := "-"
	if len(p.AllowedActions) > 0 {
		allowed = text(strings.Join(p.AllowedActions, ", "))
	}
	out.line("")
	out.line("Allowed actions: %s", allowed)
}

func (a *app) proposalsMergeCommand() *cobra.Command {
	var revision int
	var takes []string
	cmd := &cobra.Command{
		Use:   "merge <proposal-id> --revision <n> [--take <kind>:<id>=ours|theirs]...",
		Short: "Merge a proposal into its dashboard as a new version",
		Long:  "merge saves the proposal as the next version of its dashboard; it needs Edit on the dashboard and a connection that is not in propose mode.\n--revision is the revision from observe proposals show; a proposal that gained a newer revision answers proposal_changed.\nWhere the dashboard and the proposal changed the same unit, --take names the side to keep: ours keeps the dashboard's unit and theirs takes the proposal's, for example --take panel:p1=theirs.\nThe merge runs the queries of the panels it changes before it saves.",
		Args:  exactArgs(1, "one proposal id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if revision < 1 {
				return usagef("--revision <n> is required, the revision from observe proposals show")
			}
			choices := make([]mergeChoice, len(takes))
			for i, raw := range takes {
				if choices[i], err = parseTake(raw); err != nil {
					return err
				}
			}
			return a.observationCall(cmd, http.MethodPost, "proposals/"+id+"/merge", proposalMergeBody{Revision: revision, Choices: choices}, func(out *output, data []byte) error {
				merged, err := decode[proposalRow](data)
				if err != nil {
					return err
				}
				if merged.MergedVersion == nil {
					out.confirm("Already in the dashboard")
					return nil
				}
				out.confirm("Merged as version %d", *merged.MergedVersion)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&revision, "revision", 0, "the proposal's revision from observe proposals show")
	cmd.Flags().StringArrayVar(&takes, "take", nil, "which side wins a conflict, <kind>:<id>=ours|theirs; repeat for each conflict")
	return cmd
}

func parseTake(raw string) (mergeChoice, error) {
	bad := usagef("--take %q is not <kind>:<id>=ours|theirs with kind %s", text(raw), strings.Join(choiceKinds, ", "))
	kind, rest, found := strings.Cut(raw, ":")
	if !found {
		return mergeChoice{}, bad
	}
	cut := strings.LastIndex(rest, "=")
	if cut < 1 {
		return mergeChoice{}, bad
	}
	id, take := rest[:cut], rest[cut+1:]
	if !slices.Contains(choiceKinds, kind) || !slices.Contains(choiceSides, take) {
		return mergeChoice{}, bad
	}
	return mergeChoice{Kind: kind, ID: id, Take: take}, nil
}

func (a *app) proposalsDiscardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "discard <proposal-id>",
		Short: "Close a proposal without saving it",
		Long:  "discard closes an open proposal without saving its change.\nIts author may, and so may someone who can edit the dashboard on a connection that is not in propose mode.",
		Args:  exactArgs(1, "one proposal id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodPost, "proposals/"+id+"/discard", nil, func(out *output, _ []byte) error {
				out.confirm("Discarded")
				return nil
			})
		},
	}
}
