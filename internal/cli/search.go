package cli

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 50
)

var searchKinds = []string{"dashboard", "finding", "annotation", "comment", "saved_search"}

type searchOptions struct {
	kinds  []string
	limit  int
	cursor string
}

type searchSegment struct {
	Text string `json:"text"`
}

type searchRow struct {
	Kind    string          `json:"kind"`
	Title   string          `json:"title"`
	Snippet []searchSegment `json:"snippet"`
	URL     *string         `json:"url"`
	Folder  *struct {
		Title string `json:"title"`
	} `json:"folder"`
}

type searchBody struct {
	Results    []searchRow `json:"results"`
	NextCursor *string     `json:"next_cursor"`
}

func (a *app) searchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <words...> [--kind <kind>]... [--limit 20]",
		Short: "Search dashboards, findings, annotations, comments and saved searches",
		Long:  "search finds what you can read in the workspace by words, ranked by relevance.\nEvery word must match; \"quoted words\" match as a phrase, or joins alternatives, -word excludes, and the last word also matches as a prefix.\nPut words that start with - after --.\n--kind narrows it to dashboard, finding, annotation, comment or saved_search and may be repeated.",
		Example: "  observe search checkout latency\n" +
			"  observe search \"connection reset\" --kind comment --kind finding -- -staging",
		Args: cobra.MinimumNArgs(1),
		RunE: a.runSearch,
	}
	flags := cmd.Flags()
	flags.StringArrayVar(&a.searching.kinds, "kind", nil, "only this kind: "+strings.Join(searchKinds, ", ")+"; repeat it for several")
	flags.IntVar(&a.searching.limit, "limit", defaultSearchLimit, "results per page, 1 to 50")
	flags.StringVar(&a.searching.cursor, "cursor", "", "continue from a next_cursor")
	return cmd
}

func (o searchOptions) query(words []string) (url.Values, error) {
	if o.limit < 1 || o.limit > maxSearchLimit {
		return nil, usagef("--limit must be 1 to %d", maxSearchLimit)
	}
	for _, kind := range o.kinds {
		if !slices.Contains(searchKinds, kind) {
			return nil, usagef("--kind must be one of %s, not %q", strings.Join(searchKinds, ", "), text(kind))
		}
	}
	query := url.Values{"q": {strings.Join(words, " ")}, "limit": {strconv.Itoa(o.limit)}}
	if len(o.kinds) > 0 {
		query["kind"] = o.kinds
	}
	if o.cursor != "" {
		query.Set("cursor", o.cursor)
	}
	return query, nil
}

func (a *app) runSearch(cmd *cobra.Command, words []string) error {
	query, err := a.searching.query(words)
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "search", query)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	page, err := decode[searchBody](data)
	if err != nil {
		return err
	}
	if len(page.Results) == 0 {
		out.line("No results.")
		return nil
	}
	for _, r := range page.Results {
		folder, address := "no folder", "no page"
		if r.Folder != nil {
			folder = text(r.Folder.Title)
		}
		if r.URL != nil {
			address = text(*r.URL)
		}
		out.line("%s  %s  %s  %s", text(r.Kind), text(r.Title), folder, address)
		if snippet := snippetText(r.Snippet); snippet != "" {
			out.line("    %s", snippet)
		}
	}
	nextPageHint(out, page.NextCursor)
	return nil
}

func snippetText(segments []searchSegment) string {
	var b strings.Builder
	for _, s := range segments {
		b.WriteString(s.Text)
	}
	return strings.TrimSpace(text(b.String()))
}
