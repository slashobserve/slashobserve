package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

const (
	defaultPageSize = 50
	maxAllPages     = 1000

	maxDashboardPage   = 200
	maxObservationPage = 100
	maxSavedSearchPage = 100
	maxProposalPage    = 100
	maxRequestPage     = 100
	maxThreadPage      = 500
	maxAnnotationPage  = 500
	maxPeoplePage      = 500
)

type pageOptions struct {
	limit    int
	maxLimit int
	cursor   string
	all      bool
}

func addPageFlags(cmd *cobra.Command, opts *pageOptions, maxLimit int) {
	opts.maxLimit = maxLimit
	cmd.Flags().IntVar(&opts.limit, "limit", defaultPageSize, fmt.Sprintf("rows per page, 1 to %d", maxLimit))
	cmd.Flags().StringVar(&opts.cursor, "cursor", "", "continue from the next_cursor a page ended with")
	cmd.Flags().BoolVar(&opts.all, "all", false, "read every page and print them as one list")
}

func (o pageOptions) query() url.Values {
	query := url.Values{"limit": {strconv.Itoa(o.limit)}}
	if o.cursor != "" {
		query.Set("cursor", o.cursor)
	}
	return query
}

func (o pageOptions) check() error {
	if o.limit < 1 || (o.maxLimit > 0 && o.limit > o.maxLimit) {
		return usagef("--limit must be 1 to %d", o.maxLimit)
	}
	if o.all && o.cursor != "" {
		return usagef("--all reads every page from the start, so it cannot be combined with --cursor")
	}
	return nil
}

func (c *client) getPages(ctx context.Context, path string, query url.Values, all bool) ([]byte, error) {
	data, err := c.get(ctx, path, query)
	if err != nil || !all {
		return data, err
	}
	merged, seen := data, map[string]bool{}
	for page := 1; ; page++ {
		next, err := nextCursor(merged)
		if err != nil {
			return nil, err
		}
		if next == "" {
			return merged, nil
		}
		if seen[next] || page >= maxAllPages {
			return nil, errors.New("the server kept answering with more pages; use --cursor to read them one at a time")
		}
		seen[next] = true
		query.Set("cursor", next)
		more, err := c.get(ctx, path, query)
		if err != nil {
			return nil, err
		}
		if merged, err = appendPage(merged, more); err != nil {
			return nil, err
		}
	}
}

func nextCursor(page []byte) (string, error) {
	fields, err := decode[map[string]json.RawMessage](page)
	if err != nil {
		return "", err
	}
	var next *string
	if raw, found := fields["next_cursor"]; found && json.Unmarshal(raw, &next) != nil {
		return "", errors.New("the server sent a next_cursor the CLI cannot read; check that the CLI and the server versions match")
	}
	if next == nil {
		return "", nil
	}
	return *next, nil
}

func appendPage(base, more []byte) ([]byte, error) {
	baseFields, err := decode[map[string]json.RawMessage](base)
	if err != nil {
		return nil, err
	}
	moreFields, err := decode[map[string]json.RawMessage](more)
	if err != nil {
		return nil, err
	}
	for key, raw := range moreFields {
		if key == "warnings" {
			continue
		}
		var rows []json.RawMessage
		if json.Unmarshal(raw, &rows) != nil {
			continue
		}
		var have []json.RawMessage
		if json.Unmarshal(baseFields[key], &have) != nil {
			continue
		}
		joined, err := json.Marshal(append(have, rows...))
		if err != nil {
			return nil, err
		}
		baseFields[key] = joined
	}
	baseFields["next_cursor"] = moreFields["next_cursor"]
	return json.Marshal(baseFields)
}

func pageFooter(out *output, shown int, total *int, next *string) {
	if next == nil || *next == "" {
		return
	}
	if total != nil {
		out.notice("Showing %d of %d. More: --cursor %s, or --all for every page.", shown, *total, text(*next))
		return
	}
	out.notice("Showing %d, and more exist. More: --cursor %s, or --all for every page.", shown, text(*next))
}
