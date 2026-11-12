package tools

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/search"
)

type workspaceRig struct {
	registry *Registry
	db       *dbtest.Database
}

func newWorkspaceRig(t *testing.T) workspaceRig {
	t.Helper()
	database := dbtest.New(t)
	base := &url.URL{Scheme: "https", Host: "observe.example.com"}
	service := search.New(database.DB, folders.NewAccess(database.DB), base)
	return workspaceRig{registry: New(Deps{Search: service, Logger: slog.New(slog.DiscardHandler)}), db: database}
}

func (r workspaceRig) text(t *testing.T, query string, args ...any) string {
	t.Helper()
	var out string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r workspaceRig) dashboard(t *testing.T, title, description string, folder pgtype.UUID) string {
	t.Helper()
	id := r.text(t, `INSERT INTO dashboards (title, latest_version, folder_id, created_by_kind, created_by_id, created_by_name, created_via)
VALUES ($1, 1, coalesce($2::uuid, general_folder_id()), 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`, title, folder)
	r.db.Exec(t, `INSERT INTO dashboard_versions (dashboard_id, version, spec, summary, author_kind, author_id, author_name)
VALUES ($1::uuid, 1, jsonb_build_object('title', $2::text, 'description', $3::text), 'seed', 'user', gen_random_uuid(), 'seed')`, id, title, description)
	return id
}

func (r workspaceRig) comment(t *testing.T, dashboard, body string) (thread string) {
	t.Helper()
	thread = r.text(t, `INSERT INTO threads (dashboard_id, dashboard_version) VALUES ($1::uuid, 1) RETURNING id::text`, dashboard)
	r.db.Exec(t, `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name) VALUES ($1::uuid, $2, 'user', gen_random_uuid(), 'Ada')`, thread, body)
	return thread
}

func (r workspaceRig) search(t *testing.T, by auth.Principal, input string) (searchListOut, Result) {
	t.Helper()
	res, err := r.registry.Call(auth.WithPrincipal(t.Context(), by), nameSearchWorkspace, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		return searchListOut{}, res
	}
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	var out searchListOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("content %s: %v", raw, err)
	}
	return out, res
}

func titlesOfSearch(out searchListOut) []string {
	titles := make([]string, len(out.Results))
	for i, r := range out.Results {
		titles[i] = r.Title
	}
	slices.Sort(titles)
	return titles
}

func TestSearchWorkspaceAnswersWithPlainSnippetsAndPlaces(t *testing.T) {
	r := newWorkspaceRig(t)
	board := r.dashboard(t, "Checkout latency", "Slow checkout requests by pod", pgtype.UUID{})
	thread := r.comment(t, board, "the checkout p99 doubled after the deploy")

	out, res := r.search(t, viewerCaller, `{"q":"checkout"}`)

	if res.IsError || len(out.Results) != 2 {
		t.Fatalf("results = %+v, %+v", out.Results, res)
	}
	byKind := map[search.Kind]searchResultOut{}
	for _, result := range out.Results {
		byKind[result.Kind] = result
	}
	dash := byKind[search.KindDashboard]
	if dash.ID != board || dash.Title != "Checkout latency" || dash.URL == nil || !strings.HasPrefix(*dash.URL, "https://observe.example.com/") {
		t.Errorf("dashboard result = %+v", dash)
	}
	if dash.Folder == nil || dash.Folder.Title != "General" || dash.Target != nil || dash.ThreadID != nil {
		t.Errorf("dashboard place = %+v", dash)
	}
	comment := byKind[search.KindComment]
	if comment.Snippet != "checkout p99 doubled after the deploy" {
		t.Errorf("comment snippet = %q", comment.Snippet)
	}
	if comment.Target == nil || comment.Target.Kind != search.KindDashboard || comment.Target.ID != board || comment.ThreadID == nil || *comment.ThreadID != thread {
		t.Errorf("comment target = %+v, thread %v", comment.Target, comment.ThreadID)
	}
	for _, result := range out.Results {
		if strings.ContainsAny(result.Snippet, "\x02\x03") || result.UpdatedAt == "" {
			t.Errorf("result = %+v", result)
		}
	}
}

func TestSearchWorkspaceReturnsAnEmptyArrayWhenNothingMatches(t *testing.T) {
	r := newWorkspaceRig(t)

	_, res := r.search(t, viewerCaller, `{"q":"nothing"}`)

	if got := string(contentJSONOf(t, res)); got != `{"results":[],"next_cursor":null}` {
		t.Fatalf("empty search = %s", got)
	}
}

func contentJSONOf(t *testing.T, res Result) []byte {
	t.Helper()
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSearchWorkspaceFindsOnlyWhatTheCallerMayRead(t *testing.T) {
	r := newWorkspaceRig(t)
	restricted := foldertest.Restricted(t, r.db, "Payroll", privateGroup)
	r.dashboard(t, "Payroll checkout", "", restricted)
	hidden := r.dashboard(t, "Payroll runs", "", restricted)
	r.comment(t, hidden, "payroll checkout totals")
	r.dashboard(t, "Open checkout", "", pgtype.UUID{})

	cases := []struct {
		name string
		by   auth.Principal
		want []string
	}{
		{"viewer", viewerCaller, []string{"Open checkout"}},
		{"key", keyCaller, []string{"Open checkout"}},
		{"group member", groupCaller, []string{"Open checkout", "Payroll checkout", "Payroll runs"}},
		{"admin", adminCaller, []string{"Open checkout", "Payroll checkout", "Payroll runs"}},
	}
	for _, c := range cases {
		out, res := r.search(t, c.by, `{"q":"checkout"}`)
		if res.IsError {
			t.Fatalf("%s: %+v", c.name, res)
		}
		if got := titlesOfSearch(out); !slices.Equal(got, c.want) {
			t.Errorf("%s finds %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSearchWorkspaceNarrowsByKindAndPages(t *testing.T) {
	r := newWorkspaceRig(t)
	first := r.dashboard(t, "Checkout one", "", pgtype.UUID{})
	second := r.dashboard(t, "Checkout two", "", pgtype.UUID{})
	r.comment(t, first, "checkout comment")

	onlyDashboards, _ := r.search(t, viewerCaller, `{"q":"checkout","kinds":["dashboard"]}`)
	page, _ := r.search(t, viewerCaller, `{"q":"checkout","kinds":["dashboard"],"limit":1}`)

	if len(onlyDashboards.Results) != 2 {
		t.Errorf("dashboards only = %+v", onlyDashboards.Results)
	}
	if len(page.Results) != 1 || page.NextCursor == nil {
		t.Fatalf("first page = %+v, next %v", page.Results, page.NextCursor)
	}
	cursor, err := json.Marshal(*page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	last, _ := r.search(t, viewerCaller, `{"q":"checkout","kinds":["dashboard"],"limit":1,"cursor":`+string(cursor)+`}`)
	if len(last.Results) != 1 || last.NextCursor != nil {
		t.Fatalf("second page = %+v, next %v", last.Results, last.NextCursor)
	}
	if got := []string{page.Results[0].ID, last.Results[0].ID}; !slices.Contains(got, first) || !slices.Contains(got, second) {
		t.Errorf("pages hold %v, want %s and %s", got, first, second)
	}
}

func TestSearchWorkspaceRefusesBadInput(t *testing.T) {
	r := newWorkspaceRig(t)
	cases := []struct {
		name    string
		input   string
		message string
	}{
		{"no q", `{}`, "q must be 1 to 200 characters"},
		{"empty q", `{"q":"   "}`, "q must be 1 to 200 characters"},
		{"q too long", `{"q":"` + strings.Repeat("a", search.MaxQueryRunes+1) + `"}`, "q must be 1 to 200 characters"},
		{"only an exclusion", `{"q":"-staging"}`, "q needs a word to search for"},
		{"limit 0", `{"q":"a","limit":0}`, "limit must be between 1 and 50"},
		{"limit 51", `{"q":"a","limit":51}`, "limit must be between 1 and 50"},
		{"unknown kind", `{"q":"a","kinds":["folder"]}`, "kinds must each be one of dashboard, finding, annotation, observation, comment, saved_search"},
		{"cursor not a cursor", `{"q":"a","cursor":"nope"}`, "cursor is not valid"},
		{"cursor too long", `{"q":"a","cursor":"` + strings.Repeat("c", maxNameBytes+1) + `"}`, "cursor must be at most 256 bytes"},
	}
	for _, c := range cases {
		_, res := r.search(t, viewerCaller, c.input)
		body, _ := res.Content.(errorBody)
		if !res.IsError || body.Code != codeInvalidRequest || body.Message != c.message {
			t.Errorf("%s: error %v, %+v", c.name, res.IsError, body)
		}
	}
}

func TestSearchWorkspaceIsMCPOnlyAndNeedsAViewer(t *testing.T) {
	var tool Tool
	for _, candidate := range New(Deps{}).List() {
		if candidate.Name == nameSearchWorkspace {
			tool = candidate
		}
	}
	if !tool.MCPOnly || !tool.ReadOnly || tool.MinRole != auth.RoleViewer {
		t.Fatalf("search_workspace = %+v", tool)
	}
	if len(tool.Description) >= 2000 {
		t.Errorf("description is %d bytes", len(tool.Description))
	}
	for _, mode := range []Mode{ModeCreate, ModeRevise, ModeSuggest, ModeInvestigate} {
		if tool.OfferedIn(mode) {
			t.Errorf("search_workspace is offered to the agent in %s", mode)
		}
	}
}
