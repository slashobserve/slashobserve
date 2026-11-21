package savedsearches_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const payrollGroup = "payroll-team"

var (
	editor      = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve", Role: auth.RoleEditor}
	otherEditor = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Ed", Role: auth.RoleEditor}
	viewer      = auth.Principal{Kind: auth.KindUser, ID: "33333333-3333-4333-8333-333333333333", Name: "Vic", Role: auth.RoleViewer}
	payroll     = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Pat", Role: auth.RoleEditor, Groups: []string{payrollGroup}}
	pipeline    = auth.Principal{Kind: auth.KindServiceAccount, ID: "66666666-6666-4666-8666-666666666666", Name: "pipeline", Role: auth.RoleEditor}
)

type fakeSource struct {
	datasource.Datasource
	kind string
}

func (f fakeSource) Type() string { return f.kind }

type rig struct {
	db      *dbtest.Database
	service *savedsearches.Service
}

func newRegistry(t *testing.T) *datasource.Registry {
	t.Helper()
	var instances []datasource.Instance
	for _, kind := range []string{"loki", "tempo", "clickhouse", "prometheus"} {
		instances = append(instances, datasource.Instance{UID: kind, Name: kind, Datasource: fakeSource{kind: kind}})
	}
	registry, err := datasource.NewRegistry(slog.New(slog.DiscardHandler), instances...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newRig(t *testing.T) rig {
	t.Helper()
	d := dbtest.New(t)
	for _, p := range []auth.Principal{editor, otherEditor, viewer, payroll} {
		d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $2 || '@example.com', $3)`, p.ID, p.Name, p.Name)
	}
	d.Exec(t, `INSERT INTO service_accounts (id, name) VALUES ($1::uuid, $2)`, pipeline.ID, pipeline.Name)
	service := savedsearches.New(d.DB, folders.NewAccess(d.DB), newRegistry(t), nil, slog.New(slog.DiscardHandler))
	return rig{db: d, service: service}
}

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r rig) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var out string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), query, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r rig) general(t *testing.T) pgtype.UUID {
	t.Helper()
	return mustUUID(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
}

func (r rig) personal(t *testing.T, p auth.Principal) pgtype.UUID {
	t.Helper()
	return mustUUID(t, r.scalar(t, `SELECT id::text FROM folders WHERE personal AND owner_id = $1::uuid`, p.ID))
}

func (r rig) folder(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	return mustUUID(t, r.scalar(t, `INSERT INTO folders (title) VALUES ($1) RETURNING id::text`, title))
}

func logsSearch(title string) savedsearches.CreateInput {
	query := `{app="checkout"} |= "error"`
	return savedsearches.CreateInput{Kind: savedsearches.KindLogs, Title: title, DatasourceUID: "loki", Query: &query, Filters: []byte(`{"columns":["level","msg"]}`)}
}

func traceSearch(title string) savedsearches.CreateInput {
	return savedsearches.CreateInput{Kind: savedsearches.KindTraces, Title: title, DatasourceUID: "tempo", Filters: []byte(`{"service":"checkout","errors":true}`), From: "now-6h"}
}

func (r rig) save(t *testing.T, by auth.Principal, in savedsearches.CreateInput) savedsearches.SavedSearch {
	t.Helper()
	got, err := r.service.Create(reading(t.Context()), by, in)
	if err != nil {
		t.Fatalf("create %q: %v", in.Title, err)
	}
	return got
}

type event struct {
	Type, Folder, Data string
}

func (r rig) events(t *testing.T, id string) []event {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(reading(t.Context()), `SELECT type, COALESCE(folder_id::text, ''), data::text FROM activity WHERE saved_search_id = $1::uuid ORDER BY seq`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.Type, &e.Folder, &e.Data); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

func types(events []event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}

func TestASavedSearchLandsInMyDashboardsOrGeneral(t *testing.T) {
	r := newRig(t)

	mine := r.save(t, editor, logsSearch("Checkout errors"))
	if !mine.Folder.Personal || mine.Folder.ID != r.personal(t, editor) || !mine.Abilities.Edit {
		t.Fatalf("a user's search: folder %+v abilities %+v", mine.Folder, mine.Abilities)
	}
	if mine.Actor.Name != editor.Name || mine.From != "now-1h" || mine.To != "now" || mine.Query == nil || len(mine.Filters.Columns) != 2 {
		t.Fatalf("stored search %+v", mine)
	}
	if want := "/logs/loki?cols=level%2Cmsg&from=now-1h&q=%7Bapp%3D%22checkout%22%7D+%7C%3D+%22error%22&to=now"; mine.URL != want {
		t.Fatalf("url %q, want %q", mine.URL, want)
	}

	shared := r.save(t, pipeline, traceSearch("Slow checkout"))
	if !shared.Folder.General || shared.Folder.ID != r.general(t) {
		t.Fatalf("a service account's search went to %+v", shared.Folder)
	}
	if shared.Query != nil || shared.From != "now-6h" || !strings.HasPrefix(shared.URL, "/traces/tempo?") {
		t.Fatalf("trace search %+v", shared)
	}

	events := r.events(t, mine.ID)
	if len(events) != 1 || events[0].Type != "saved_search.created" || events[0].Folder != mine.Folder.ID.String() {
		t.Fatalf("events %+v", events)
	}
	var data struct{ Title, Kind string }
	if err := json.Unmarshal([]byte(events[0].Data), &data); err != nil || data.Title != "Checkout errors" || data.Kind != "logs" {
		t.Fatalf("event data %s: %v", events[0].Data, err)
	}
}

func TestAClickHouseTraceSearchKeepsItsSQLAndReopensWithIt(t *testing.T) {
	r := newRig(t)
	query := "SELECT TraceId, min(Timestamp) AS StartTime FROM otel_traces WHERE $__timeFilter(Timestamp) GROUP BY TraceId"
	in := traceSearch("Slowest traces")
	in.DatasourceUID, in.Query = "clickhouse", &query

	got := r.save(t, editor, in)
	if got.Query == nil || *got.Query != query {
		t.Fatalf("stored query %v", got.Query)
	}
	reopened, err := url.Parse(got.URL)
	if err != nil || reopened.Path != "/traces/clickhouse" || reopened.Query().Get("q") != query {
		t.Fatalf("url %q: %v", got.URL, err)
	}
}

func TestACreateIsRefusedWhereTheCallerMayNotCreate(t *testing.T) {
	r := newRig(t)
	restricted := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)

	cases := []struct {
		name   string
		by     auth.Principal
		folder pgtype.UUID
		want   error
	}{
		{"a folder the caller cannot view", editor, restricted, savedsearches.ErrFolderNotFound},
		{"a folder the caller can only view", viewer, r.general(t), folders.ErrForbidden},
		{"a folder that does not exist", editor, mustUUID(t, "5f3c1a52-8a0e-4b8c-9a55-0a6f3e4b7d10"), savedsearches.ErrFolderNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := logsSearch("x")
			in.FolderID = c.folder
			if _, err := r.service.Create(reading(t.Context()), c.by, in); !errors.Is(err, c.want) {
				t.Fatalf("Create = %v, want %v", err, c.want)
			}
		})
	}
	if n := r.scalar(t, `SELECT count(*)::text FROM saved_searches`); n != "0" {
		t.Fatalf("%s rows after the refusals", n)
	}
}

func TestACreateChecksTheDatasourceAgainstTheKind(t *testing.T) {
	r := newRig(t)
	cases := []struct {
		name string
		edit func(*savedsearches.CreateInput)
		code string
	}{
		{"an unknown datasource", func(in *savedsearches.CreateInput) { in.DatasourceUID = "nowhere" }, savedsearches.CodeUnknownDatasource},
		{"a trace datasource for logs", func(in *savedsearches.CreateInput) { in.DatasourceUID = "tempo" }, savedsearches.CodeUnsupportedDatasource},
		{"a metrics datasource for logs", func(in *savedsearches.CreateInput) { in.DatasourceUID = "prometheus" }, savedsearches.CodeUnsupportedDatasource},
		{"a log datasource for traces", func(in *savedsearches.CreateInput) {
			in.Kind, in.Query, in.DatasourceUID = savedsearches.KindTraces, nil, "loki"
		}, savedsearches.CodeUnsupportedDatasource},
		{"a field that breaks a rule", func(in *savedsearches.CreateInput) { in.Title = "" }, savedsearches.CodeInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := logsSearch("x")
			in.Filters = nil
			c.edit(&in)
			var invalid *savedsearches.InvalidInputError
			if _, err := r.service.Create(reading(t.Context()), editor, in); !errors.As(err, &invalid) || invalid.Code != c.code {
				t.Fatalf("Create = %v, want code %s", err, c.code)
			}
		})
	}
	in := logsSearch("ClickHouse logs")
	in.DatasourceUID = "clickhouse"
	r.save(t, editor, in)
}

func TestListShowsOnlyVisibleFoldersAndPagesByCursor(t *testing.T) {
	r := newRig(t)
	restricted := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)
	hidden := logsSearch("Payroll errors")
	hidden.FolderID = restricted
	r.save(t, payroll, hidden)
	first := r.save(t, editor, logsSearch("One"))
	r.save(t, editor, traceSearch("Two"))
	third := r.save(t, editor, logsSearch("Three"))
	r.save(t, otherEditor, logsSearch("Other's private"))

	titles := func(items []savedsearches.SavedSearch) []string {
		out := make([]string, len(items))
		for i, s := range items {
			out[i] = s.Title
		}
		return out
	}

	page, next, err := r.service.List(reading(t.Context()), editor, savedsearches.ListFilter{}, 2, "")
	if err != nil || next == "" || strings.Join(titles(page), ",") != "Three,Two" {
		t.Fatalf("first page %v next %q err %v", titles(page), next, err)
	}
	page, next, err = r.service.List(reading(t.Context()), editor, savedsearches.ListFilter{}, 2, next)
	if err != nil || next != "" || strings.Join(titles(page), ",") != "One" {
		t.Fatalf("second page %v next %q err %v", titles(page), next, err)
	}

	kind, _, err := r.service.List(reading(t.Context()), editor, savedsearches.ListFilter{Kind: savedsearches.KindTraces}, 10, "")
	if err != nil || strings.Join(titles(kind), ",") != "Two" {
		t.Fatalf("traces only %v err %v", titles(kind), err)
	}
	inFolder, _, err := r.service.List(reading(t.Context()), editor, savedsearches.ListFilter{FolderID: first.Folder.ID}, 10, "")
	if err != nil || len(inFolder) != 3 || inFolder[0].ID != third.ID {
		t.Fatalf("one folder %v err %v", titles(inFolder), err)
	}
	if _, _, err := r.service.List(reading(t.Context()), editor, savedsearches.ListFilter{FolderID: restricted}, 10, ""); !errors.Is(err, savedsearches.ErrFolderNotFound) {
		t.Fatalf("a hidden folder: %v", err)
	}
	if _, _, err := r.service.List(reading(t.Context()), editor, savedsearches.ListFilter{}, 10, "nope"); !errors.Is(err, savedsearches.ErrInvalidCursor) {
		t.Fatalf("a bad cursor: %v", err)
	}
	seen, _, err := r.service.List(reading(t.Context()), payroll, savedsearches.ListFilter{}, 10, "")
	if err != nil || strings.Join(titles(seen), ",") != "Payroll errors" {
		t.Fatalf("the payroll member sees %v err %v", titles(seen), err)
	}
}

func TestRenameAndMoveAppendOneRowEachAndMoveTakesEarlierRowsAlong(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, editor, logsSearch("Errors"))
	shared := r.folder(t, "Shared")
	renamed := "Checkout errors"

	got, err := r.service.Update(reading(t.Context()), editor, mustUUID(t, saved.ID), savedsearches.UpdateInput{Title: &renamed})
	if err != nil || got.Title != renamed || !got.UpdatedAt.After(saved.UpdatedAt) {
		t.Fatalf("rename = %+v, %v", got, err)
	}
	same := renamed
	if _, err := r.service.Update(reading(t.Context()), editor, mustUUID(t, saved.ID), savedsearches.UpdateInput{Title: &same, FolderID: saved.Folder.ID}); err != nil {
		t.Fatalf("a change to what it already has: %v", err)
	}
	if got := types(r.events(t, saved.ID)); strings.Join(got, ",") != "saved_search.created,saved_search.updated" {
		t.Fatalf("events after rename %v", got)
	}

	moved, err := r.service.Update(reading(t.Context()), editor, mustUUID(t, saved.ID), savedsearches.UpdateInput{FolderID: shared})
	if err != nil || moved.Folder.ID != shared || moved.Title != renamed {
		t.Fatalf("move = %+v, %v", moved, err)
	}
	events := r.events(t, saved.ID)
	if got := strings.Join(types(events), ","); got != "saved_search.created,saved_search.updated,saved_search.moved" {
		t.Fatalf("events after move %v", got)
	}
	for _, e := range events {
		if e.Folder != shared.String() {
			t.Fatalf("event %s is in folder %s, want %s", e.Type, e.Folder, shared)
		}
	}
	var data struct {
		From string `json:"from_folder_id"`
		To   string `json:"to_folder_id"`
	}
	if err := json.Unmarshal([]byte(events[2].Data), &data); err != nil || data.From != saved.Folder.ID.String() || data.To != shared.String() {
		t.Fatalf("move data %s: %v", events[2].Data, err)
	}

	if got, err := url.ParseQuery(strings.TrimPrefix(moved.URL, "/logs/loki?")); err != nil || got.Get("cols") == "" {
		t.Fatalf("url %q: %v", moved.URL, err)
	}
}

func TestAContentUpdateReplacesTheQueryFiltersAndRangeAndAppendsOneEvent(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, editor, logsSearch("Errors"))
	id := mustUUID(t, saved.ID)
	query := `{app="payments"} |= "timeout"`
	from, to := "now-6h", "now-1h"

	got, err := r.service.Update(reading(t.Context()), editor, id, savedsearches.UpdateInput{
		Query: &query, Filters: []byte(`{"columns":["timestamp","line"]}`), From: &from, To: &to,
	})
	if err != nil || got.Query == nil || *got.Query != query || got.From != from || got.To != to || strings.Join(got.Filters.Columns, ",") != "timestamp,line" {
		t.Fatalf("content update = %+v, %v", got, err)
	}
	if got.Title != "Errors" || got.Folder.ID != saved.Folder.ID || !got.UpdatedAt.After(saved.UpdatedAt) {
		t.Fatalf("title, folder or time after a content update: %+v", got)
	}
	events := r.events(t, saved.ID)
	if strings.Join(types(events), ",") != "saved_search.created,saved_search.updated" {
		t.Fatalf("events %v", types(events))
	}
	var data struct {
		Changed []string `json:"changed"`
		Title   string   `json:"title"`
	}
	if err := json.Unmarshal([]byte(events[1].Data), &data); err != nil || strings.Join(data.Changed, ",") != "query,filters,range" || data.Title != "Errors" {
		t.Fatalf("event data %s: %v", events[1].Data, err)
	}
	if strings.Contains(events[1].Data, "timeout") {
		t.Fatalf("event carries query text: %s", events[1].Data)
	}

	if _, err := r.service.Update(reading(t.Context()), editor, id, savedsearches.UpdateInput{Query: &query, From: &from}); err != nil {
		t.Fatalf("a content update to what it already has: %v", err)
	}
	if got := types(r.events(t, saved.ID)); len(got) != 2 {
		t.Fatalf("an unchanged content update appended an event: %v", got)
	}

	cleared, err := r.service.Update(reading(t.Context()), editor, id, savedsearches.UpdateInput{Filters: []byte(`null`)})
	if err != nil || len(cleared.Filters.Columns) != 0 || *cleared.Query != query {
		t.Fatalf("clearing filters = %+v, %v", cleared, err)
	}

	title, newQuery := "Payments timeouts", `{app="payments"}`
	both, err := r.service.Update(reading(t.Context()), editor, id, savedsearches.UpdateInput{Title: &title, Query: &newQuery})
	if err != nil || both.Title != title || *both.Query != newQuery {
		t.Fatalf("title and query together = %+v, %v", both, err)
	}
	events = r.events(t, saved.ID)
	last := events[len(events)-1]
	if err := json.Unmarshal([]byte(last.Data), &data); err != nil || strings.Join(data.Changed, ",") != "title,query" {
		t.Fatalf("last event %s: %v", last.Data, err)
	}
}

func TestAContentUpdateKeepsTheKindAndDatasourceAndChecksThemAgainstTheNewContent(t *testing.T) {
	r := newRig(t)
	logs := r.save(t, editor, logsSearch("Errors"))
	traces := r.save(t, editor, traceSearch("Slow"))
	blank, huge, badRange, empty := " ", strings.Repeat("x", 8001), "yesterday-ish", ""
	tempoQuery := `{ status = error }`

	cases := []struct {
		name string
		id   string
		in   savedsearches.UpdateInput
	}{
		{"a blank query", logs.ID, savedsearches.UpdateInput{Query: &blank}},
		{"a query over the limit", logs.ID, savedsearches.UpdateInput{Query: &huge}},
		{"clearing the query of a logs search", logs.ID, savedsearches.UpdateInput{Query: &empty}},
		{"a trace filter on a logs search", logs.ID, savedsearches.UpdateInput{Filters: []byte(`{"service":"checkout"}`)}},
		{"columns on a traces search", traces.ID, savedsearches.UpdateInput{Filters: []byte(`{"columns":["a"]}`)}},
		{"an unknown filter key", logs.ID, savedsearches.UpdateInput{Filters: []byte(`{"colums":[]}`)}},
		{"a range that does not parse", logs.ID, savedsearches.UpdateInput{From: &badRange}},
		{"an empty range end", logs.ID, savedsearches.UpdateInput{To: &empty}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := r.service.Update(reading(t.Context()), editor, mustUUID(t, c.id), c.in)
			var invalid *savedsearches.InvalidInputError
			if !errors.As(err, &invalid) || invalid.Code != savedsearches.CodeInvalid {
				t.Fatalf("Update = %v, want %s", err, savedsearches.CodeInvalid)
			}
		})
	}
	got, err := r.service.Update(reading(t.Context()), editor, mustUUID(t, traces.ID), savedsearches.UpdateInput{Query: &tempoQuery})
	if err != nil || got.Query == nil || *got.Query != tempoQuery || got.Kind != savedsearches.KindTraces || got.DatasourceUID != "tempo" {
		t.Fatalf("a Tempo query = %+v, %v", got, err)
	}
	cleared, err := r.service.Update(reading(t.Context()), editor, mustUUID(t, traces.ID), savedsearches.UpdateInput{Query: &empty})
	if err != nil || cleared.Query != nil {
		t.Fatalf("clearing a Tempo query = %+v, %v", cleared, err)
	}
	for _, id := range []string{logs.ID, traces.ID} {
		if got := types(r.events(t, id)); len(got) > 3 {
			t.Fatalf("refusals appended events: %v", got)
		}
	}
}

func TestAContentUpdateNeedsEditOnTheSearchLikeARename(t *testing.T) {
	r := newRig(t)
	shared := r.folder(t, "Shared")
	in := logsSearch("In shared")
	in.FolderID = shared
	inShared := r.save(t, editor, in)
	private := r.save(t, editor, logsSearch("Private"))
	query := `{app="other"}`

	if _, err := r.service.Update(reading(t.Context()), viewer, mustUUID(t, inShared.ID), savedsearches.UpdateInput{Query: &query}); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("a viewer's content update = %v, want %v", err, folders.ErrForbidden)
	}
	if _, err := r.service.Update(reading(t.Context()), otherEditor, mustUUID(t, private.ID), savedsearches.UpdateInput{Query: &query}); !errors.Is(err, savedsearches.ErrNotFound) {
		t.Fatalf("another person's private search = %v, want %v", err, savedsearches.ErrNotFound)
	}
	if got := types(r.events(t, inShared.ID)); len(got) != 1 {
		t.Fatalf("refusals appended events: %v", got)
	}
}

func TestUpdateRefusals(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, editor, logsSearch("Errors"))
	id := mustUUID(t, saved.ID)
	shared := r.folder(t, "Shared")
	inShared := r.save(t, editor, func() savedsearches.CreateInput {
		in := logsSearch("In shared")
		in.FolderID = shared
		return in
	}())
	title := "x"
	empty := ""

	cases := []struct {
		name string
		by   auth.Principal
		id   pgtype.UUID
		in   savedsearches.UpdateInput
		want error
	}{
		{"nothing to change", editor, id, savedsearches.UpdateInput{}, savedsearches.ErrNothingToUpdate},
		{"an unknown search", editor, mustUUID(t, "5f3c1a52-8a0e-4b8c-9a55-0a6f3e4b7d10"), savedsearches.UpdateInput{Title: &title}, savedsearches.ErrNotFound},
		{"another person's private search", otherEditor, id, savedsearches.UpdateInput{Title: &title}, savedsearches.ErrNotFound},
		{"a search in a folder the caller can only view", viewer, mustUUID(t, inShared.ID), savedsearches.UpdateInput{Title: &title}, folders.ErrForbidden},
		{"an empty title", editor, id, savedsearches.UpdateInput{Title: &empty}, nil},
		{"a move into a personal folder", editor, mustUUID(t, inShared.ID), savedsearches.UpdateInput{FolderID: r.personal(t, editor)}, folders.ErrMoveIntoPersonal},
		{"a move into a folder that does not exist", editor, id, savedsearches.UpdateInput{FolderID: mustUUID(t, "5f3c1a52-8a0e-4b8c-9a55-0a6f3e4b7d10")}, savedsearches.ErrFolderNotFound},
		{"a move by a caller who can only view", viewer, mustUUID(t, inShared.ID), savedsearches.UpdateInput{FolderID: r.general(t)}, folders.ErrForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := r.service.Update(reading(t.Context()), c.by, c.id, c.in)
			var invalid *savedsearches.InvalidInputError
			if c.want == nil {
				if !errors.As(err, &invalid) || invalid.Code != savedsearches.CodeInvalid {
					t.Fatalf("Update = %v, want %s", err, savedsearches.CodeInvalid)
				}
				return
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("Update = %v, want %v", err, c.want)
			}
		})
	}
	if got := types(r.events(t, saved.ID)); len(got) != 1 {
		t.Fatalf("refusals appended events: %v", got)
	}
}

func TestDeleteAppendsTheEventThenRemovesTheRow(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, editor, logsSearch("Errors"))
	id := mustUUID(t, saved.ID)

	if err := r.service.Delete(reading(t.Context()), otherEditor, id); !errors.Is(err, savedsearches.ErrNotFound) {
		t.Fatalf("another person deleting: %v", err)
	}
	if err := r.service.Delete(reading(t.Context()), editor, mustUUID(t, "5f3c1a52-8a0e-4b8c-9a55-0a6f3e4b7d10")); !errors.Is(err, savedsearches.ErrNotFound) {
		t.Fatalf("an unknown search: %v", err)
	}
	if err := r.service.Delete(reading(t.Context()), editor, id); err != nil {
		t.Fatal(err)
	}
	if err := r.service.Delete(reading(t.Context()), editor, id); !errors.Is(err, savedsearches.ErrNotFound) {
		t.Fatalf("deleting twice: %v", err)
	}
	events := r.events(t, saved.ID)
	if got := strings.Join(types(events), ","); got != "saved_search.created,saved_search.deleted" || events[1].Folder != saved.Folder.ID.String() {
		t.Fatalf("events %+v", events)
	}
	if n := r.scalar(t, `SELECT count(*)::text FROM saved_searches`); n != "0" {
		t.Fatalf("%s rows left", n)
	}
}

func TestDeleteIsRefusedWithoutEditOnTheFolder(t *testing.T) {
	r := newRig(t)
	shared := r.folder(t, "Shared")
	in := logsSearch("In shared")
	in.FolderID = shared
	saved := r.save(t, editor, in)

	if err := r.service.Delete(reading(t.Context()), viewer, mustUUID(t, saved.ID)); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("a viewer deleting: %v", err)
	}
	if got := types(r.events(t, saved.ID)); len(got) != 1 {
		t.Fatalf("a refused delete appended %v", got)
	}
}

func TestDeletingAUserDeletesTheSearchesInTheirPersonalFolder(t *testing.T) {
	r := newRig(t)
	r.save(t, editor, logsSearch("Mine"))
	kept := r.save(t, otherEditor, logsSearch("Theirs"))

	r.db.Exec(t, `DELETE FROM users WHERE id = $1::uuid`, editor.ID)
	if got := r.scalar(t, `SELECT string_agg(id::text, ',') FROM saved_searches`); got != kept.ID {
		t.Fatalf("saved searches left: %s, want only %s", got, kept.ID)
	}
}
