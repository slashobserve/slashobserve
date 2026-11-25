package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
)

const (
	nameSaveSearch        = "save_search"
	nameListSavedSearches = "list_saved_searches"
	nameUpdateSavedSearch = "update_saved_search"

	defaultListSavedSearches = 20
	maxListSavedSearches     = 50

	descSaveSearch        = "Save a logs, trace or metrics search with a title in a folder (default your My Dashboards) so people can open it from its page. A query with a lint_query error is not saved; draft_query writes one. A repeat returns the saved search with replayed true. Returns its url."
	descUpdateSavedSearch = "Change what a saved logs, trace or metrics search runs: its query, filters or range, which each replace the saved value (filters replace all the filters, so send every one to keep). Its kind, datasource, title and folder stay. Needs Edit on the search's folder; get the id and the current values from list_saved_searches. Check a new query with lint_query or draft_query first. Returns the search with its url."
	descListSavedSearches = "List the saved logs, trace and metrics searches this key can see, most recently changed first, optionally of one kind or in one folder. Each has its datasource, query, filters, range and url. Page with next_cursor."

	propSearchKind = `"kind":{"type":"string","enum":["logs","traces","metrics"],"description":"Which page opens the search."}`

	propSearchFilters = `"columns":{"type":"array","maxItems":20,"items":{"type":"string","minLength":1,"maxLength":256},"description":"Logs: columns to show."},` +
		`"service":{"type":"string","maxLength":200,"description":"Traces: this service."},` +
		`"span":{"type":"string","maxLength":200,"description":"Traces: spans with this name."},` +
		`"min":{"type":"string","description":"Traces: shortest span, such as 250ms."},` +
		`"max":{"type":"string","description":"Traces: longest span, such as 2s."},` +
		`"errors":{"type":"boolean","description":"Traces: only traces with errors."},` +
		`"mode":{"type":"string","enum":["graph","table"],"description":"Metrics: graph (default) or table."}`

	schemaSaveSearch = `{"type":"object","properties":{` + propSearchKind + `,` +
		`"title":{"type":"string","minLength":1,"maxLength":120,"description":"Short title."},` +
		`"datasource":{"type":"string","description":"From list_datasources: Loki, Elasticsearch, OpenSearch or ClickHouse (logs), Tempo, Jaeger or ClickHouse (traces), Prometheus (metrics)."},` +
		`"query":{"type":"string","minLength":1,"maxLength":8000,"description":"The query as it runs. Required for logs and metrics (PromQL); optional for traces: TraceQL on Tempo, key=value on Jaeger, SQL on ClickHouse."},` +
		`"filters":{"type":"object","properties":{` + propSearchFilters + `},"additionalProperties":false},` +
		`"from":{"type":"string","description":"Start of the range, such as now-1h or RFC 3339. Default now-1h."},` +
		`"to":{"type":"string","description":"End of the range, such as now or RFC 3339. Default now."},` +
		`"folder_id":{"type":"string","format":"uuid","description":"The folder, from list_folders. Default your My Dashboards, else General."}},` +
		`"required":["kind","title","datasource"],"additionalProperties":false}`

	schemaUpdateSavedSearch = `{"type":"object","properties":{` +
		`"id":{"type":"string","format":"uuid","description":"The saved search, from list_saved_searches."},` +
		`"query":{"type":"string","maxLength":8000,"description":"The new query as it runs. An empty string removes the query of a trace search and is refused for a logs or metrics search."},` +
		`"filters":{"type":["object","null"],"properties":{` + propSearchFilters + `},` +
		`"additionalProperties":false,"description":"Replaces all the filters. A logs search takes only columns, a traces search the trace keys, a metrics search only mode; null removes every filter."},` +
		`"from":{"type":"string","description":"New start of the saved range, such as now-1h or an RFC 3339 time."},` +
		`"to":{"type":"string","description":"New end of the saved range, such as now or an RFC 3339 time."}},` +
		`"required":["id"],"additionalProperties":false}`

	schemaListSavedSearches = `{"type":"object","properties":{` +
		`"kind":{"type":"string","enum":["logs","traces","metrics"],"description":"Only searches of this kind."},` +
		`"folder_id":{"type":"string","format":"uuid","description":"Only searches in this folder, from list_folders."},` +
		`"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Searches per page. Default 20."},` +
		`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`
)

var errSavedSearchesNotConfigured = errors.New("saved searches are not configured")

func init() { registerEntries((*Registry).savedSearchEntries) }

func (r *Registry) savedSearchEntries() []entry {
	return []entry{
		{Tool{Name: nameSaveSearch, Description: descSaveSearch, InputSchema: json.RawMessage(schemaSaveSearch), MCPOnly: true, MinRole: auth.RoleViewer}, r.saveSearch},
		{Tool{Name: nameUpdateSavedSearch, Description: descUpdateSavedSearch, InputSchema: json.RawMessage(schemaUpdateSavedSearch), MCPOnly: true, MinRole: auth.RoleViewer}, r.updateSavedSearch},
		{Tool{Name: nameListSavedSearches, Description: descListSavedSearches, InputSchema: json.RawMessage(schemaListSavedSearches), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.listSavedSearches},
	}
}

type saveSearchInput struct {
	Kind       string          `json:"kind"`
	Title      string          `json:"title"`
	Datasource string          `json:"datasource"`
	Query      *string         `json:"query"`
	Filters    json.RawMessage `json:"filters"`
	From       string          `json:"from"`
	To         string          `json:"to"`
	FolderID   string          `json:"folder_id"`
}

type updateSavedSearchInput struct {
	ID      string          `json:"id"`
	Query   *string         `json:"query"`
	Filters json.RawMessage `json:"filters"`
	From    *string         `json:"from"`
	To      *string         `json:"to"`
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
	leftOutOut
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
	create := savedsearches.CreateInput{
		Kind:          savedsearches.Kind(in.Kind),
		Title:         in.Title,
		DatasourceUID: in.Datasource,
		Query:         in.Query,
		Filters:       in.Filters,
		From:          in.From,
		To:            in.To,
		FolderID:      folderID,
	}
	principal, _ := auth.FromContext(ctx)
	existing, err := r.GuardSavedSearch(ctx, principal, create)
	if err != nil {
		return savedSearchFailure(ctx, err)
	}
	if existing != nil {
		return success("Found the search already saved as "+existing.ID, savedSearchReplayOut{SavedSearchJSON: savedsearches.JSONOf(*existing), Replayed: true}), nil
	}
	saved, err := r.deps.SavedSearches.Create(ctx, principal, create)
	if err != nil {
		return savedSearchFailure(ctx, err)
	}
	res = success("Saved search "+saved.ID, savedsearches.JSONOf(saved))
	res.Created = []string{saved.ID}
	return res, nil
}

func (r *Registry) updateSavedSearch(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, scoped := rejectInRun(ctx, nameUpdateSavedSearch); scoped {
		return res, nil
	}
	if r.deps.SavedSearches == nil {
		return Result{}, errSavedSearchesNotConfigured
	}
	var in updateSavedSearchInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, err := db.ParseUUID(in.ID)
	if err != nil {
		return invalid("id must be a saved search id from list_saved_searches"), nil
	}
	principal, _ := auth.FromContext(ctx)
	saved, err := r.deps.SavedSearches.Update(ctx, principal, id, savedsearches.UpdateInput{Query: in.Query, Filters: in.Filters, From: in.From, To: in.To})
	if errors.Is(err, savedsearches.ErrNothingToUpdate) {
		return invalid("give at least one of query, filters, from or to"), nil
	}
	if err != nil {
		return savedSearchFailure(ctx, err)
	}
	return success("Updated saved search "+saved.ID, savedsearches.JSONOf(saved)), nil
}

type savedSearchReplayOut struct {
	savedsearches.SavedSearchJSON
	Replayed bool `json:"replayed"`
}

func (r *Registry) GuardSavedSearch(ctx context.Context, by auth.Principal, in savedsearches.CreateInput) (*savedsearches.SavedSearch, error) {
	if err := r.searchQueryPasses(ctx, in.DatasourceUID, in.Query); err != nil {
		return nil, err
	}
	return r.alreadySavedSearch(ctx, by, in)
}

func (r *Registry) alreadySavedSearch(ctx context.Context, principal auth.Principal, in savedsearches.CreateInput) (*savedsearches.SavedSearch, error) {
	wanted, err := searchFilters(in.Filters)
	if err != nil || r.deps.Folders == nil || r.deps.SavedSearches == nil {
		return nil, nil
	}
	folder, err := r.deps.Folders.CreationFolder(ctx, principal, in.FolderID)
	if err != nil {
		return nil, nil
	}
	from, to := savedsearches.RangeOrDefault(in.From, in.To)
	items, _, err := r.deps.SavedSearches.List(ctx, principal, savedsearches.ListFilter{Kind: in.Kind, FolderID: folder.ID}, identicalSearchLimit, "")
	if err != nil {
		return nil, nil
	}
	for _, item := range items {
		if sameSearch(item, principal, in, from, to) && hasFilters(item, wanted) {
			return &item, nil
		}
	}
	return nil, nil
}

func sameSearch(item savedsearches.SavedSearch, by auth.Principal, in savedsearches.CreateInput, from, to string) bool {
	return item.Actor.Kind == by.Kind && item.Actor.ID == by.ID &&
		item.Title == strings.TrimSpace(in.Title) && item.DatasourceUID == in.DatasourceUID &&
		ptrEqual(item.Query, in.Query) && item.From == from && item.To == to
}

func hasFilters(item savedsearches.SavedSearch, wanted string) bool {
	have, err := searchFilters(mustJSON(item.Filters))
	return err == nil && have == wanted
}

func searchFilters(raw []byte) (string, error) {
	var f savedsearches.Filters
	if len(bytes.TrimSpace(raw)) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &f); err != nil {
			return "", err
		}
	}
	out, err := json.Marshal(f)
	return string(out), err
}

func mustJSON(v any) json.RawMessage {
	out, _ := json.Marshal(v)
	return out
}

func ptrEqual(a, b *string) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

func (r *Registry) searchQueryPasses(ctx context.Context, uid string, query *string) error {
	if query == nil || *query == "" || r.deps.Datasources == nil {
		return nil
	}
	inst, err := r.deps.Datasources.For(ctx, uid)
	if err != nil {
		return nil
	}
	diagnostics, _, ran, err := r.lint(ctx, inst, *query)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || !ran {
		return nil
	}
	for _, d := range diagnostics {
		if d.Severity == datasource.SeverityError {
			return &savedsearches.InvalidInputError{Code: savedsearches.CodeQueryInvalid, Message: "the query does not pass the lint check, so nothing was saved: " + clip(d.Message, maxCellBytes)}
		}
	}
	return nil
}

type listSavedSearchesInput struct {
	Kind     string `json:"kind"`
	FolderID string `json:"folder_id"`
	Limit    *int   `json:"limit"`
	Cursor   string `json:"cursor"`
}

func (in listSavedSearchesInput) parse() (savedsearches.ListFilter, int, Result, bool) {
	filter := savedsearches.ListFilter{Kind: savedsearches.Kind(in.Kind)}
	if in.Kind != "" && !filter.Kind.Valid() {
		return filter, 0, invalid("kind must be logs, traces or metrics"), false
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
	titles, res, ok, err := r.restrictedTitles(ctx, false)
	if err != nil || !ok {
		return res, err
	}
	principal, _ := auth.FromContext(ctx)
	for {
		list, next, err := r.deps.SavedSearches.List(ctx, principal, filter, limit, in.Cursor)
		if err != nil {
			return savedSearchFailure(ctx, err)
		}
		list = keepReadable(titles, list, func(s savedsearches.SavedSearch) (string, string) { return s.Folder.ID.String(), s.ID })
		out := savedSearchListOut{SavedSearches: make([]savedSearchItemOut, len(list)), leftOutOut: titles.report("saved search", "saved searches")}
		for i, s := range list {
			out.SavedSearches[i] = savedSearchItem(s)
		}
		if next != "" {
			out.NextCursor = &next
		}
		kept, cut := fit(out.SavedSearches, listBudget-jsonSize(savedSearchListOut{SavedSearches: []savedSearchItemOut{}, NextCursor: out.NextCursor}))
		if !cut || len(list) == 1 {
			titles.done(ctx)
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

func savedSearchFailure(ctx context.Context, err error) (Result, error) {
	if res, ok := restrictedRefusal(ctx, err); ok {
		return res, nil
	}
	var invalidInput *savedsearches.InvalidInputError
	switch {
	case errors.As(err, &invalidInput):
		return invalid("%s", invalidInput.Message), nil
	case errors.Is(err, savedsearches.ErrInvalidCursor):
		return invalid("cursor is not valid"), nil
	case errors.Is(err, savedsearches.ErrNotFound):
		return toolError(codeNotFound, "saved search not found"), nil
	case errors.Is(err, savedsearches.ErrFolderNotFound), errors.Is(err, folders.ErrNoAccess):
		return toolError(codeNotFound, "folder not found"), nil
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, "you cannot save or change searches in this folder"), nil
	}
	return Result{}, err
}
