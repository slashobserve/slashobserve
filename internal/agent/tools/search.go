package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/search"
)

const (
	nameSearchWorkspace = "search_workspace"

	descSearchWorkspace = "Search everything this key may read in the workspace by words: dashboards (title, tags, description, panel titles, query text), findings, annotations, comments and saved logs and trace searches. Every word must match, exactly or by its English stem; \"quoted words\" match as a phrase, or joins alternatives, -word excludes, and the last word also matches as a prefix. Results are ranked by relevance, newer first among near-equals, each with a snippet and its url; read the object with its own tool for more. Page with next_cursor."
)

var schemaSearchWorkspace = `{"type":"object","properties":{` +
	`"q":{"type":"string","minLength":1,"maxLength":` + strconv.Itoa(search.MaxQueryRunes) + `,"description":"The words to search for, such as checkout latency or \"connection reset\" -staging."},` +
	`"kinds":{"type":"array","maxItems":` + strconv.Itoa(len(search.Kinds())) + `,"items":{"type":"string","enum":["` + joinKinds(`","`) + `"]},"description":"Only results of any of these kinds. Default all."},` +
	`"limit":{"type":"integer","minimum":1,"maximum":` + strconv.Itoa(search.MaxLimit) + `,"description":"Results per page. Default ` + strconv.Itoa(search.DefaultLimit) + `."},` +
	`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"required":["q"],"additionalProperties":false}`

var errSearchNotConfigured = errors.New("search is not configured")

func joinKinds(separator string) string {
	names := make([]string, len(search.Kinds()))
	for i, kind := range search.Kinds() {
		names[i] = string(kind)
	}
	return strings.Join(names, separator)
}

func init() { registerEntries((*Registry).searchEntries) }

func (r *Registry) searchEntries() []entry {
	return []entry{
		{Tool{Name: nameSearchWorkspace, Description: descSearchWorkspace, InputSchema: json.RawMessage(schemaSearchWorkspace), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.searchWorkspace},
	}
}

type searchWorkspaceInput struct {
	Q      string   `json:"q"`
	Kinds  []string `json:"kinds"`
	Limit  *int     `json:"limit"`
	Cursor string   `json:"cursor"`
}

type searchFolderOut struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type searchTargetOut struct {
	Kind  search.Kind `json:"kind"`
	ID    string      `json:"id"`
	Title string      `json:"title"`
}

type searchResultOut struct {
	Kind      search.Kind      `json:"kind"`
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	Snippet   string           `json:"snippet"`
	URL       *string          `json:"url"`
	Folder    *searchFolderOut `json:"folder"`
	Target    *searchTargetOut `json:"target"`
	ThreadID  *string          `json:"thread_id"`
	UpdatedAt string           `json:"updated_at"`
}

type searchListOut struct {
	Results    []searchResultOut `json:"results"`
	NextCursor *string           `json:"next_cursor"`
}

func (r *Registry) searchWorkspace(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Search == nil {
		return Result{}, errSearchNotConfigured
	}
	var in searchWorkspaceInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	limit, res, ok := boundedInt(in.Limit, search.DefaultLimit, 1, search.MaxLimit, "limit")
	if !ok {
		return res, nil
	}
	kinds := make([]search.Kind, len(in.Kinds))
	for i, name := range in.Kinds {
		kinds[i] = search.Kind(name)
		if !kinds[i].Valid() {
			return invalid("kinds must each be one of %s", joinKinds(", ")), nil
		}
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	for {
		page, err := r.deps.Search.Search(ctx, principal, search.Request{Q: in.Q, Kinds: kinds, Limit: limit, Cursor: in.Cursor})
		if err != nil {
			return searchFailure(err)
		}
		out := searchListOut{Results: make([]searchResultOut, 0, len(page.Results))}
		for _, result := range page.Results {
			out.Results = append(out.Results, searchResultOf(result))
		}
		if page.Next != "" {
			out.NextCursor = &page.Next
		}
		kept, cut := fit(out.Results, listBudget-jsonSize(searchListOut{Results: []searchResultOut{}, NextCursor: out.NextCursor}))
		if !cut || len(out.Results) == 1 {
			return success("Found "+plural(len(out.Results), "result", "results"), out), nil
		}
		limit = max(len(kept), 1)
	}
}

func searchResultOf(r search.Result) searchResultOut {
	out := searchResultOut{
		Kind:      r.Kind,
		ID:        r.ID,
		Title:     r.Title,
		Snippet:   search.PlainText(r.Snippet),
		UpdatedAt: r.ChangedAt.UTC().Format(time.RFC3339),
	}
	if r.URL != "" {
		out.URL = &r.URL
	}
	if r.Folder != nil {
		out.Folder = &searchFolderOut{ID: r.Folder.ID, Title: r.Folder.Title}
	}
	if r.Target != nil {
		out.Target = &searchTargetOut{Kind: r.Target.Kind, ID: r.Target.ID, Title: r.Target.Title}
		out.ThreadID = &r.ThreadID
	}
	return out
}

func searchFailure(err error) (Result, error) {
	var query *search.InvalidQueryError
	switch {
	case errors.As(err, &query):
		return invalid("%s", query.Message), nil
	case errors.Is(err, search.ErrInvalidCursor):
		return invalid("cursor is not valid"), nil
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, roleDeniedMessage), nil
	}
	return Result{}, err
}
