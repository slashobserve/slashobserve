package tools

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
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

func TestSavedSearchToolsOnlyRunOutsideAgentRuns(t *testing.T) {
	r := newSearchRig(t)
	for name, input := range map[string]string{
		nameSaveSearch:        `{"kind":"logs","title":"Errors","datasource_uid":"logs","query":"{app=\"x\"}"}`,
		nameListSavedSearches: `{}`,
	} {
		res := r.call(t, &Run{Mode: ModeCreate}, keyCaller, name, input)
		body, ok := res.Content.(errorBody)
		if errorCode(t, res) != codeInvalidRequest || !ok || body.Message != name+" is for MCP clients; a run saves only dashboards or a finding" {
			t.Errorf("%s in a run: %#v", name, res.Content)
		}
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

	res := r.call(t, nil, viewerCaller, nameListSavedSearches, `{}`)
	list, ok := res.Content.(savedSearchListOut)
	if res.IsError || !ok || len(list.SavedSearches) != 2 || list.NextCursor != nil {
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
	inFolder, ok := r.call(t, nil, viewerCaller, nameListSavedSearches, `{"folder_id":"`+savedLogs.Folder.ID+`"}`).Content.(savedSearchListOut)
	if !ok || len(inFolder.SavedSearches) != 2 {
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
		"a kind that is not logs or traces": `{"kind":"metrics"}`,
		"a folder id that is not a uuid":    `{"folder_id":"nope"}`,
		"a cursor that is not valid":        `{"cursor":"nope"}`,
	} {
		if res := r.call(t, nil, keyCaller, nameListSavedSearches, input); errorCode(t, res) != codeInvalidRequest {
			t.Errorf("list with %s: %#v", name, res.Content)
		}
	}
	if res := r.call(t, nil, keyCaller, nameListSavedSearches, `{"folder_id":"`+restricted+`"}`); errorCode(t, res) != codeNotFound {
		t.Errorf("listing a folder the key cannot see: %#v", res.Content)
	}
}
