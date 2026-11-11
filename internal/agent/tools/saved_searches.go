package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
)

const (
	nameSaveSearch        = "save_search"
	nameListSavedSearches = "list_saved_searches"

	defaultListSavedSearches = 20
	maxListSavedSearches     = 50

	descSaveSearch        = "Save a logs or trace search with a title in a folder, by default your My Dashboards, so people can open it from the logs or traces page. Check the query with lint_query or draft_query first. Returns its url."
	descListSavedSearches = "List the saved logs and trace searches this key can see, most recently changed first, optionally of one kind or in one folder. Each has its datasource, query, filters, range and url. Page with next_cursor."

	propSearchKind = `"kind":{"type":"string","enum":["logs","traces"],"description":"Which page opens the search."}`

	schemaSaveSearch = `{"type":"object","properties":{` + propSearchKind + `,` +
		`"title":{"type":"string","minLength":1,"maxLength":120,"description":"A short title on one line."},` +
		`"datasource_uid":{"type":"string","description":"The datasource to search, from list_datasources: Loki, ClickHouse, Elasticsearch or OpenSearch for logs, Tempo or ClickHouse for traces."},` +
		`"query":{"type":"string","minLength":1,"maxLength":8000,"description":"The query as it runs. Required for logs; optional TraceQL for Tempo traces; not allowed for ClickHouse traces."},` +
		`"filters":{"type":"object","properties":{` +
		`"columns":{"type":"array","maxItems":20,"items":{"type":"string","minLength":1,"maxLength":256},"description":"Logs: the columns to show."},` +
		`"service":{"type":"string","maxLength":200,"description":"Traces: only this service."},` +
		`"span":{"type":"string","maxLength":200,"description":"Traces: only spans with this name."},` +
		`"min":{"type":"string","description":"Traces: the shortest duration, such as 250ms."},` +
		`"max":{"type":"string","description":"Traces: the longest duration, such as 2s."},` +
		`"errors":{"type":"boolean","description":"Traces: true for only traces with errors."}},` +
		`"additionalProperties":false,"description":"A logs search takes only columns, a traces search the other keys."},` +
		`"from":{"type":"string","description":"Start of the saved range, such as now-1h or an RFC 3339 time. Default now-1h."},` +
		`"to":{"type":"string","description":"End of the saved range, such as now or an RFC 3339 time. Default now."},` +
		`"folder_id":{"type":"string","format":"uuid","description":"The folder to save it in, from list_folders. Default your My Dashboards, else General."}},` +
		`"required":["kind","title","datasource_uid"],"additionalProperties":false}`

	schemaListSavedSearches = `{"type":"object","properties":{` +
		`"kind":{"type":"string","enum":["logs","traces"],"description":"Only searches of this kind."},` +
		`"folder_id":{"type":"string","format":"uuid","description":"Only searches in this folder, from list_folders."},` +
		`"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Searches per page. Default 20."},` +
		`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`
)

var errSavedSearchesNotConfigured = errors.New("saved searches are not configured")

func init() { registerEntries((*Registry).savedSearchEntries) }

func (r *Registry) savedSearchEntries() []entry {
	return []entry{
		{Tool{Name: nameSaveSearch, Description: descSaveSearch, InputSchema: json.RawMessage(schemaSaveSearch), MCPOnly: true, MinRole: auth.RoleViewer}, r.saveSearch},
		{Tool{Name: nameListSavedSearches, Description: descListSavedSearches, InputSchema: json.RawMessage(schemaListSavedSearches), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.listSavedSearches},
	}
}

type saveSearchInput struct {
	Kind          string          `json:"kind"`
	Title         string          `json:"title"`
	DatasourceUID string          `json:"datasource_uid"`
	Query         *string         `json:"query"`
	Filters       json.RawMessage `json:"filters"`
	From          string          `json:"from"`
	To            string          `json:"to"`
	FolderID      string          `json:"folder_id"`
}

type savedSearchItemOut struct {
	ID            string                `json:"id"`
	Kind          savedsearches.Kind    `json:"kind"`
	Title         string                `json:"title"`
	DatasourceUID string                `json:"datasource_uid"`
	Query         *string               `json:"query"`
	Filters       savedsearches.Filters `json:"filters"`
	From          string                `json:"from"`
	To            string                `json:"to"`
	FolderID      string                `json:"folder_id"`
	Folder        string                `json:"folder"`
	URL           string                `json:"url"`
}

type savedSearchListOut struct {
	SavedSearches []savedSearchItemOut `json:"saved_searches"`
	NextCursor    *string              `json:"next_cursor"`
}

func (r *Registry) saveSearch(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, scoped := rejectInRun(ctx, nameSaveSearch); scoped {
		return res, nil
	}
	if r.deps.SavedSearches == nil {
		return Result{}, errSavedSearchesNotConfigured
	}
	var in saveSearchInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	folderID, res, ok := parseFolderID(in.FolderID)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	saved, err := r.deps.SavedSearches.Create(ctx, principal, savedsearches.CreateInput{
		Kind:          savedsearches.Kind(in.Kind),
		Title:         in.Title,
		DatasourceUID: in.DatasourceUID,
		Query:         in.Query,
		Filters:       in.Filters,
		From:          in.From,
		To:            in.To,
		FolderID:      folderID,
	})
	if err != nil {
		return savedSearchFailure(err)
	}
	return success("Saved search "+saved.ID, savedsearches.JSONOf(saved)), nil
}

type listSavedSearchesInput struct {
	Kind     string `json:"kind"`
	FolderID string `json:"folder_id"`
	Limit    *int   `json:"limit"`
	Cursor   string `json:"cursor"`
}

func (in listSavedSearchesInput) parse() (savedsearches.ListFilter, int, Result, bool) {
	filter := savedsearches.ListFilter{Kind: savedsearches.Kind(in.Kind)}
	if in.Kind != "" && filter.Kind != savedsearches.KindLogs && filter.Kind != savedsearches.KindTraces {
		return filter, 0, invalid("kind must be logs or traces"), false
	}
	folderID, res, ok := parseFolderID(in.FolderID)
	if !ok {
		return filter, 0, res, false
	}
	filter.FolderID = folderID
	limit, res, ok := boundedInt(in.Limit, defaultListSavedSearches, 1, maxListSavedSearches, "limit")
	if !ok {
		return filter, 0, res, false
	}
	res, ok = checkLength("cursor", in.Cursor, maxNameBytes)
	return filter, limit, res, ok
}

func (r *Registry) listSavedSearches(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, scoped := rejectInRun(ctx, nameListSavedSearches); scoped {
		return res, nil
	}
	if r.deps.SavedSearches == nil {
		return Result{}, errSavedSearchesNotConfigured
	}
	var in listSavedSearchesInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	filter, limit, res, ok := in.parse()
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	for {
		list, next, err := r.deps.SavedSearches.List(ctx, principal, filter, limit, in.Cursor)
		if err != nil {
			return savedSearchFailure(err)
		}
		out := savedSearchListOut{SavedSearches: make([]savedSearchItemOut, len(list))}
		for i, s := range list {
			out.SavedSearches[i] = savedSearchItem(s)
		}
		if next != "" {
			out.NextCursor = &next
		}
		kept, cut := fit(out.SavedSearches, listBudget-jsonSize(savedSearchListOut{SavedSearches: []savedSearchItemOut{}, NextCursor: out.NextCursor}))
		if !cut || len(list) == 1 {
			return success("Listed "+plural(len(list), "saved search", "saved searches"), out), nil
		}
		limit = max(len(kept), 1)
	}
}

func savedSearchItem(s savedsearches.SavedSearch) savedSearchItemOut {
	return savedSearchItemOut{
		ID:            s.ID,
		Kind:          s.Kind,
		Title:         s.Title,
		DatasourceUID: s.DatasourceUID,
		Query:         s.Query,
		Filters:       s.Filters,
		From:          s.From,
		To:            s.To,
		FolderID:      s.Folder.ID.String(),
		Folder:        s.Folder.Title,
		URL:           s.URL,
	}
}

func rejectInRun(ctx context.Context, name string) (Result, bool) {
	if _, scoped := RunFrom(ctx); scoped {
		return invalid("%s is for MCP clients; a run saves only dashboards or a finding", name), true
	}
	return Result{}, false
}

func savedSearchFailure(err error) (Result, error) {
	var invalidInput *savedsearches.InvalidInputError
	switch {
	case errors.As(err, &invalidInput):
		return invalid("%s", invalidInput.Message), nil
	case errors.Is(err, savedsearches.ErrInvalidCursor):
		return invalid("cursor is not valid"), nil
	case errors.Is(err, savedsearches.ErrFolderNotFound), errors.Is(err, folders.ErrNoAccess):
		return toolError(codeNotFound, "folder not found"), nil
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, "you cannot save searches in this folder"), nil
	}
	return Result{}, err
}
