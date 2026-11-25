package tools

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
)

type searchRig struct {
	registry *Registry
	db       *dbtest.Database
}

func newSearchRig(t *testing.T) *searchRig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	datasources, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: fakeUID, Name: "Metrics", Datasource: &fakeDS{}},
		datasource.Instance{UID: "logs", Name: "Logs", Datasource: &fakeDS{typ: "loki"}},
		datasource.Instance{UID: "traces", Name: "Traces", Datasource: &fakeDS{typ: "tempo"}},
		datasource.Instance{UID: "linted", Name: "Linted", Datasource: &fakeDS{typ: "loki", lintFn: func(_ context.Context, query string) ([]datasource.Diagnostic, error) {
			if query == "{" {
				return []datasource.Diagnostic{{Severity: datasource.SeverityError, Message: "unexpected end of input"}}, nil
			}
			return []datasource.Diagnostic{{Severity: datasource.SeverityWarning, Message: "slow regex"}}, nil
		}}},
		datasource.Instance{UID: "lintdown", Name: "Unreachable", Datasource: &fakeDS{typ: "loki", lintFn: func(context.Context, string) ([]datasource.Diagnostic, error) {
			return nil, errors.New("connection refused")
		}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	registry := New(Deps{
		Datasources:   datasources,
		Runner:        queryproxy.NewRunner(datasources, logger),
		Folders:       access,
		SavedSearches: savedsearches.New(database.DB, access, datasources, nil, logger),
		Logger:        logger,
	})
	return &searchRig{registry: registry, db: database}
}

func (r *searchRig) call(t *testing.T, run *Run, by auth.Principal, tool, input string) Result {
	t.Helper()
	ctx := auth.WithPrincipal(t.Context(), by)
	if run != nil {
		ctx = WithRun(ctx, *run)
	}
	res, err := r.registry.Call(ctx, tool, []byte(input))
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return res
}

func (r *searchRig) newFolder(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	folder, err := r.db.Queries().CreateFolder(reading(t.Context()), dbsqlc.CreateFolderParams{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return folder.ID
}

func TestSavedSearchToolsInARunStayWithinItsWriteScope(t *testing.T) {
	r := newSearchRig(t)
	chat := &Run{Mode: ModeCreate, Build: true, RequestID: ownRunID, ChatID: "chat"}
	saved := r.call(t, chat, keyCaller, nameSaveSearch, `{"kind":"logs","title":"Errors","datasource":"logs","query":"{app=\"x\"}"}`)
	out, ok := saved.Content.(savedsearches.SavedSearchJSON)
	if saved.IsError || !ok || len(saved.Created) != 1 || saved.Created[0] != out.ID {
		t.Fatalf("save_search in a chat run: %#v, created %v", saved.Content, saved.Created)
	}
	if list := r.call(t, chat, keyCaller, nameListSavedSearches, `{}`); list.IsError {
		t.Fatalf("list_saved_searches in a run: %#v", list.Content)
	}
	if code := errorCode(t, r.call(t, chat, keyCaller, nameUpdateSavedSearch, `{"id":"99999999-9999-4999-8999-999999999999","from":"now-2h"}`)); code != codeOutOfScope {
		t.Errorf("update_saved_search of a search the run did not save answered %s, want %s", code, codeOutOfScope)
	}
	private := &Run{Mode: ModeInvestigate, RequestID: ownRunID, ChatID: "chat", DashboardID: subjectID}
	if code := errorCode(t, r.call(t, private, keyCaller, nameSaveSearch, `{"kind":"logs","title":"Errors","datasource":"logs","query":"{app=\"x\"}"}`)); code != codeOutOfScope {
		t.Errorf("save_search in a private ask answered %s, want %s", code, codeOutOfScope)
	}
}

func TestSavedSearchesRoundTripThroughSaveAndList(t *testing.T) {
	r := newSearchRig(t)
	logs := r.call(t, nil, keyCaller, nameSaveSearch, `{"kind":"logs","title":"  Checkout errors ","datasource_uid":"logs","query":"{app=\"checkout\"} |= \"error\"","filters":{"columns":["level","msg"]},"from":"now-6h"}`)
	savedLogs, ok := logs.Content.(savedsearches.SavedSearchJSON)
	if logs.IsError || !ok || savedLogs.Title != "Checkout errors" || savedLogs.From != "now-6h" || savedLogs.To != "now" || savedLogs.Folder.Title != "General" {
		t.Fatalf("logs search: %#v", logs.Content)
	}
	if !strings.HasPrefix(savedLogs.URL, "/logs/logs?") || !strings.Contains(savedLogs.URL, "cols=level%2Cmsg") {
		t.Errorf("logs url = %q", savedLogs.URL)
	}
	traces := r.call(t, nil, keyCaller, nameSaveSearch, `{"kind":"traces","title":"Slow checkout","datasource_uid":"traces","filters":{"service":"checkout","min":"250ms","errors":true}}`)
	savedTraces, ok := traces.Content.(savedsearches.SavedSearchJSON)
	if traces.IsError || !ok || savedTraces.Query != nil || !strings.HasPrefix(savedTraces.URL, "/traces/traces?") {
		t.Fatalf("traces search: %#v", traces.Content)
	}

	metrics := r.call(t, nil, keyCaller, nameSaveSearch, `{"kind":"metrics","title":"Request rate","datasource_uid":"fake","query":"sum(rate(http_requests_total[5m]))","filters":{"mode":"table"}}`)
	savedMetrics, ok := metrics.Content.(savedsearches.SavedSearchJSON)
	if metrics.IsError || !ok || !strings.HasPrefix(savedMetrics.URL, "/explore/metrics/fake?") || !strings.Contains(savedMetrics.URL, "mode=table") {
		t.Fatalf("metrics search: %#v", metrics.Content)
	}

	res := r.call(t, nil, viewerCaller, nameListSavedSearches, `{}`)
	list, ok := res.Content.(savedSearchListOut)
	if res.IsError || !ok || len(list.SavedSearches) != 3 || list.NextCursor != nil {
		t.Fatalf("list: %#v", res.Content)
	}
	byID := map[string]savedSearchItemOut{}
	for _, item := range list.SavedSearches {
		byID[item.ID] = item
	}
	got := byID[savedLogs.ID]
	if got.Kind != savedsearches.KindLogs || got.Title != "Checkout errors" || got.DatasourceUID != "logs" || got.Query == nil ||
		got.From != "now-6h" || got.To != "now" || len(got.Filters.Columns) != 2 || got.Folder != "General" || got.FolderID != savedLogs.Folder.ID || got.URL != savedLogs.URL {
		t.Errorf("listed logs search = %#v", got)
	}

	onlyTraces, ok := r.call(t, nil, viewerCaller, nameListSavedSearches, `{"kind":"traces"}`).Content.(savedSearchListOut)
	if !ok || len(onlyTraces.SavedSearches) != 1 || onlyTraces.SavedSearches[0].ID != savedTraces.ID {
		t.Errorf("list by kind: %#v", onlyTraces)
	}
	onlyMetrics, ok := r.call(t, nil, viewerCaller, nameListSavedSearches, `{"kind":"metrics"}`).Content.(savedSearchListOut)
	if !ok || len(onlyMetrics.SavedSearches) != 1 || onlyMetrics.SavedSearches[0].ID != savedMetrics.ID || onlyMetrics.SavedSearches[0].Filters.Mode != "table" {
		t.Errorf("list metrics: %#v", onlyMetrics)
	}
	inFolder, ok := r.call(t, nil, viewerCaller, nameListSavedSearches, `{"folder_id":"`+savedLogs.Folder.ID+`"}`).Content.(savedSearchListOut)
	if !ok || len(inFolder.SavedSearches) != 3 {
		t.Errorf("list by folder: %#v", inFolder)
	}
}

func TestListSavedSearchesPagesByCursor(t *testing.T) {
	r := newSearchRig(t)
	for _, title := range []string{"one", "two", "three"} {
		if res := r.call(t, nil, keyCaller, nameSaveSearch, `{"kind":"logs","title":"`+title+`","datasource_uid":"logs","query":"{a=\"b\"}"}`); res.IsError {
			t.Fatalf("save %s: %#v", title, res.Content)
		}
	}
	first, ok := r.call(t, nil, keyCaller, nameListSavedSearches, `{"limit":2}`).Content.(savedSearchListOut)
	if !ok || len(first.SavedSearches) != 2 || first.NextCursor == nil {
		t.Fatalf("first page: %#v", first)
	}
	second, ok := r.call(t, nil, keyCaller, nameListSavedSearches, `{"limit":2,"cursor":"`+*first.NextCursor+`"}`).Content.(savedSearchListOut)
	if !ok || len(second.SavedSearches) != 1 || second.NextCursor != nil || second.SavedSearches[0].Title != "one" {
		t.Fatalf("second page: %#v", second)
	}
}

func TestSaveSearchRefusesWhatCannotBeSaved(t *testing.T) {
	r := newSearchRig(t)
	restricted := foldertest.Restricted(t, r.db, hiddenFolder, privateGroup).String()
	missing := "99999999-9999-4999-8999-999999999999"
	save := func(extra string) string {
		return `{"kind":"logs","title":"Errors","datasource_uid":"logs","query":"{a=\"b\"}"` + extra + `}`
	}
	viewerKey := auth.Principal{Kind: auth.KindServiceAccount, ID: "88888888-8888-4888-8888-888888888888", Name: "ci-ro", Role: auth.RoleViewer}
	cases := []struct {
		name  string
		by    auth.Principal
		input string
		code  string
	}{
		{"an unknown datasource", keyCaller, `{"kind":"logs","title":"Errors","datasource_uid":"nope","query":"{a=\"b\"}"}`, codeInvalidRequest},
		{"a datasource of the wrong kind", keyCaller, `{"kind":"logs","title":"Errors","datasource_uid":"traces","query":"{a=\"b\"}"}`, codeInvalidRequest},
		{"a metrics search without a query", keyCaller, `{"kind":"metrics","title":"Rate","datasource_uid":"fake"}`, codeInvalidRequest},
		{"a metrics search on a logs datasource", keyCaller, `{"kind":"metrics","title":"Rate","datasource_uid":"logs","query":"up"}`, codeInvalidRequest},
		{"a logs search without a query", keyCaller, `{"kind":"logs","title":"Errors","datasource_uid":"logs"}`, codeInvalidRequest},
		{"a filter key of the other kind", keyCaller, save(`,"filters":{"service":"checkout"}`), codeInvalidRequest},
		{"a range that does not resolve", keyCaller, save(`,"from":"yesterday-ish"`), codeInvalidRequest},
		{"a folder id that is not a uuid", keyCaller, save(`,"folder_id":"nope"`), codeInvalidRequest},
		{"an unknown folder", keyCaller, save(`,"folder_id":"` + missing + `"`), codeNotFound},
		{"a folder the key cannot see", keyCaller, save(`,"folder_id":"` + restricted + `"`), codeNotFound},
		{"a Viewer key with no folder to save in", viewerKey, save(""), codeForbiddenRole},
		{"a folder the caller can see but not create in", groupCaller, save(`,"folder_id":"` + restricted + `"`), codeForbiddenRole},
	}
	for _, c := range cases {
		if res := r.call(t, nil, c.by, nameSaveSearch, c.input); errorCode(t, res) != c.code {
			t.Errorf("%s: %#v, want %s", c.name, res.Content, c.code)
		}
	}
	for name, input := range map[string]string{
		"a kind that is not one":         `{"kind":"sql"}`,
		"a folder id that is not a uuid": `{"folder_id":"nope"}`,
		"a cursor that is not valid":     `{"cursor":"nope"}`,
	} {
		if res := r.call(t, nil, keyCaller, nameListSavedSearches, input); errorCode(t, res) != codeInvalidRequest {
			t.Errorf("list with %s: %#v", name, res.Content)
		}
	}
	if res := r.call(t, nil, keyCaller, nameListSavedSearches, `{"folder_id":"`+restricted+`"}`); errorCode(t, res) != codeNotFound {
		t.Errorf("listing a folder the key cannot see: %#v", res.Content)
	}
}

func TestUpdateSavedSearchReplacesWhatItRunsAndKeepsTheRest(t *testing.T) {
	r := newSearchRig(t)
	saved, ok := r.call(t, nil, keyCaller, nameSaveSearch, `{"kind":"logs","title":"Errors","datasource_uid":"logs","query":"{a=\"b\"}","filters":{"columns":["level"]}}`).Content.(savedsearches.SavedSearchJSON)
	if !ok {
		t.Fatal("save failed")
	}

	res := r.call(t, nil, keyCaller, nameUpdateSavedSearch, `{"id":"`+saved.ID+`","query":"{a=\"c\"}","filters":{"columns":["timestamp","line"]},"from":"now-6h","to":"now-1h"}`)
	got, ok := res.Content.(savedsearches.SavedSearchJSON)
	if res.IsError || !ok || got.Query == nil || *got.Query != `{a="c"}` || got.From != "now-6h" || got.To != "now-1h" || len(got.Filters.Columns) != 2 {
		t.Fatalf("update: %#v", res.Content)
	}
	if got.Title != "Errors" || got.DatasourceUID != "logs" || got.Folder.ID != saved.Folder.ID || !strings.Contains(got.URL, "cols=timestamp%2Cline") {
		t.Errorf("title, datasource, folder or url after the update: %#v", got)
	}

	listed, ok := r.call(t, nil, keyCaller, nameListSavedSearches, `{}`).Content.(savedSearchListOut)
	if !ok || len(listed.SavedSearches) != 1 || listed.SavedSearches[0].From != "now-6h" {
		t.Errorf("list after the update: %#v", listed)
	}

	cleared, ok := r.call(t, nil, keyCaller, nameUpdateSavedSearch, `{"id":"`+saved.ID+`","filters":null}`).Content.(savedsearches.SavedSearchJSON)
	if !ok || len(cleared.Filters.Columns) != 0 {
		t.Errorf("clearing filters: %#v", cleared)
	}
}

func TestUpdateSavedSearchRefusesWhatCannotBeChanged(t *testing.T) {
	r := newSearchRig(t)
	saved, ok := r.call(t, nil, keyCaller, nameSaveSearch, `{"kind":"logs","title":"Errors","datasource_uid":"logs","query":"{a=\"b\"}"}`).Content.(savedsearches.SavedSearchJSON)
	if !ok {
		t.Fatal("save failed")
	}
	missing := "99999999-9999-4999-8999-999999999999"
	cases := []struct {
		name  string
		by    auth.Principal
		input string
		code  string
	}{
		{"an id that is not a uuid", keyCaller, `{"id":"nope","from":"now-2h"}`, codeInvalidRequest},
		{"nothing to change", keyCaller, `{"id":"` + saved.ID + `"}`, codeInvalidRequest},
		{"a search that does not exist", keyCaller, `{"id":"` + missing + `","from":"now-2h"}`, codeNotFound},
		{"a blank query", keyCaller, `{"id":"` + saved.ID + `","query":" "}`, codeInvalidRequest},
		{"a trace filter on a logs search", keyCaller, `{"id":"` + saved.ID + `","filters":{"service":"checkout"}}`, codeInvalidRequest},
		{"a range that does not resolve", keyCaller, `{"id":"` + saved.ID + `","to":"yesterday-ish"}`, codeInvalidRequest},
		{"a title, which this tool does not change", keyCaller, `{"id":"` + saved.ID + `","title":"x"}`, codeInvalidRequest},
		{"a caller who can view the folder but not edit in it", groupCaller, `{"id":"` + saved.ID + `","from":"now-2h"}`, codeForbiddenRole},
	}
	for _, c := range cases {
		if res := r.call(t, nil, c.by, nameUpdateSavedSearch, c.input); errorCode(t, res) != c.code {
			t.Errorf("%s: %#v, want %s", c.name, res.Content, c.code)
		}
	}
}

func TestSaveSearchRefusesAQueryThatFailsTheLintCheckAndKeepsWarnings(t *testing.T) {
	r := newSearchRig(t)
	input := func(query string) string {
		return `{"kind":"logs","title":"Errors","datasource":"linted","query":"` + query + `"}`
	}
	res := r.call(t, nil, keyCaller, nameSaveSearch, input("{"))
	eb, ok := res.Content.(errorBody)
	if !res.IsError || !ok || eb.Code != codeInvalidRequest || !strings.Contains(eb.Message, "unexpected end of input") {
		t.Fatalf("a failing query: %#v", res.Content)
	}
	if list := r.call(t, nil, keyCaller, nameListSavedSearches, `{}`).Content.(savedSearchListOut); len(list.SavedSearches) != 0 {
		t.Fatalf("a refused search was saved: %+v", list)
	}
	if res := r.call(t, nil, keyCaller, nameSaveSearch, input("{a=\\\"b\\\"}")); res.IsError {
		t.Fatalf("a query with only a warning: %#v", res.Content)
	}
}

func TestSaveSearchTwiceReturnsTheSearchAlreadySaved(t *testing.T) {
	r := newSearchRig(t)
	input := `{"kind":"logs","title":"Checkout errors","datasource_uid":"logs","query":"{app=\"checkout\"}","filters":{"columns":["level"]}}`
	first, ok := r.call(t, nil, keyCaller, nameSaveSearch, input).Content.(savedsearches.SavedSearchJSON)
	if !ok {
		t.Fatal("the first save did not save")
	}
	again, ok := r.call(t, nil, keyCaller, nameSaveSearch, input).Content.(savedSearchReplayOut)
	if !ok || again.ID != first.ID || !again.Replayed {
		t.Fatalf("second save = %#v, want the first search replayed", again)
	}
	changed, ok := r.call(t, nil, keyCaller, nameSaveSearch, strings.Replace(input, `"level"`, `"level","msg"`, 1)).Content.(savedsearches.SavedSearchJSON)
	if !ok || changed.ID == first.ID {
		t.Fatalf("a different search was treated as the same: %#v", changed)
	}
	list, _ := r.call(t, nil, viewerCaller, nameListSavedSearches, `{}`).Content.(savedSearchListOut)
	if len(list.SavedSearches) != 2 {
		t.Fatalf("saved searches = %d, want 2", len(list.SavedSearches))
	}
}

func TestSaveSearchReplaysOnlyWithinTheFolderItWouldBeSavedIn(t *testing.T) {
	r := newSearchRig(t)
	ops := r.newFolder(t, "Ops")
	body := `"kind":"logs","title":"Checkout errors","datasource_uid":"logs","query":"{app=\"checkout\"}"`
	inOps := r.call(t, nil, keyCaller, nameSaveSearch, `{`+body+`,"folder_id":"`+ops.String()+`"}`)
	savedInOps, ok := inOps.Content.(savedsearches.SavedSearchJSON)
	if !ok || savedInOps.Folder.Title != "Ops" {
		t.Fatalf("save into Ops: %#v", inOps.Content)
	}
	byDefault, ok := r.call(t, nil, keyCaller, nameSaveSearch, `{`+body+`}`).Content.(savedsearches.SavedSearchJSON)
	if !ok || byDefault.ID == savedInOps.ID || byDefault.Folder.ID == savedInOps.Folder.ID {
		t.Fatalf("a save without folder_id returned the Ops search: %#v", byDefault)
	}
	again, ok := r.call(t, nil, keyCaller, nameSaveSearch, `{`+body+`}`).Content.(savedSearchReplayOut)
	if !ok || again.ID != byDefault.ID || !again.Replayed {
		t.Fatalf("a repeat without folder_id = %#v, want the default folder's search replayed", again)
	}
	inOpsAgain, ok := r.call(t, nil, keyCaller, nameSaveSearch, `{`+body+`,"folder_id":"`+ops.String()+`"}`).Content.(savedSearchReplayOut)
	if !ok || inOpsAgain.ID != savedInOps.ID {
		t.Fatalf("a repeat into Ops = %#v, want the Ops search replayed", inOpsAgain)
	}
}

func TestSaveSearchStillSavesWhenTheLintCheckCannotRun(t *testing.T) {
	r := newSearchRig(t)
	res := r.call(t, nil, keyCaller, nameSaveSearch, `{"kind":"logs","title":"Errors","datasource":"lintdown","query":"{app=\"x\"}"}`)
	if saved, ok := res.Content.(savedsearches.SavedSearchJSON); res.IsError || !ok || saved.DatasourceUID != "lintdown" {
		t.Fatalf("a save while the lint check is down: %#v", res.Content)
	}
}
