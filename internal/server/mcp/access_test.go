package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
	"github.com/slashobserve/slashobserve/internal/threads"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const (
	hiddenTitle  = "Payroll board"
	hiddenTag    = "payroll-tag"
	hiddenFolder = "Payroll"
	openTitle    = "Open board"
	commentText  = "who gets paid what"
	privateGroup = "payroll-team"
	forbidden    = "forbidden_role"
)

func serviceKeyOf(role auth.Role, id string) auth.Principal {
	return auth.Principal{Kind: auth.KindServiceAccount, ID: id, Name: string(role) + "-key", Role: role}
}

var (
	adminKey     = serviceKeyOf(auth.RoleAdmin, "a0000000-0000-4000-8000-000000000001")
	editorKey    = serviceKeyOf(auth.RoleEditor, "a0000000-0000-4000-8000-000000000002")
	commenterKey = serviceKeyOf(auth.RoleCommenter, "a0000000-0000-4000-8000-000000000003")
	viewerKey    = serviceKeyOf(auth.RoleViewer, "a0000000-0000-4000-8000-000000000004")
	creatorKey   = auth.Principal{Kind: auth.KindServiceAccount, ID: "a0000000-0000-4000-8000-000000000005", Name: "creator-key", Role: auth.RoleEditor}
)

type keyAuth map[string]Credential

func (k keyAuth) Authenticate(_ context.Context, header string) (Credential, bool, error) {
	c, ok := k[header]
	return c, ok, nil
}

func bearer(p auth.Principal) string { return "Bearer " + p.Name }

type promStub struct{}

func (promStub) Type() string                      { return "prometheus" }
func (promStub) CheckHealth(context.Context) error { return nil }
func (promStub) Close() error                      { return nil }
func (promStub) Lint(context.Context, string) ([]datasource.Diagnostic, error) {
	return nil, nil
}

func (promStub) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}

func (promStub) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return []string{"v"}, nil
}

func (promStub) QueryRange(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
	return stubFrames(), nil
}

func (promStub) QueryInstant(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
	return stubFrames(), nil
}

type lokiStub struct{ promStub }

func (lokiStub) Type() string { return "loki" }

func stubFrames() []datasource.Frame {
	return []datasource.Frame{{Fields: []datasource.Field{
		{Name: "time", Type: datasource.FieldTime, Values: []any{1.0}},
		{Name: "value", Type: datasource.FieldNumber, Values: []any{2.0}},
	}}}
}

type accessFixture struct {
	*fixture
	db       *dbtest.Database
	service  *dashboards.Service
	findings *findings.Service
	threads  *threads.Service
	authn    keyAuth
}

func newAccessFixture(t *testing.T) *accessFixture {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	datasources, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: promStub{}}, datasource.Instance{UID: "loki", Datasource: lokiStub{}})
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(datasources, logger)
	access := folders.NewAccess(database.DB)
	service := dashboards.New(database.DB, runner, access, logger)
	notes := annotations.New(database.DB, access)
	found := findings.New(database.DB, access, notes, service, logger)
	commentThreads := threads.NewService(database.DB, access, service, found, notes)
	base, err := url.Parse(publicURL)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.New(tools.Deps{Datasources: datasources, Runner: runner, Dashboards: service, Findings: found, SavedSearches: savedsearches.New(database.DB, access, datasources, base, logger), Threads: commentThreads, Folders: access, Annotations: notes, PublicURL: base, Logger: logger})
	authn := keyAuth{}
	for _, p := range []auth.Principal{adminKey, editorKey, commenterKey, viewerKey, creatorKey} {
		authn[bearer(p)] = Credential{Principal: p}
	}
	f := &fixture{handler: NewHandler(reg, authn, Options{PublicURL: base, Version: "test", Logger: logger}), tools: offeredOverMCP(reg)}
	return &accessFixture{fixture: f, db: database, service: service, findings: found, threads: commentThreads, authn: authn}
}

func (f *accessFixture) writtenRows(t *testing.T) int {
	t.Helper()
	var n int
	err := f.db.SQLDB(t).QueryRowContext(t.Context(), "SELECT (SELECT count(*) FROM dashboards) + (SELECT count(*) FROM dashboard_versions) + (SELECT count(*) FROM annotations) + (SELECT count(*) FROM findings) + (SELECT count(*) FROM saved_searches) + (SELECT count(*) FROM activity)").Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *accessFixture) person(t *testing.T, name string, role auth.Role) auth.Principal {
	t.Helper()
	u, err := f.db.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: name, Email: name + "@example.com", Name: name})
	if err != nil {
		t.Fatal(err)
	}
	p := auth.Principal{Kind: auth.KindUser, ID: u.ID.String(), Name: name, Role: role}
	f.authn[bearer(p)] = Credential{Principal: p, OAuth: true, Client: "Claude Code"}
	return p
}

func boardSpec(title string, tags ...string) string {
	quoted := make([]string, len(tags))
	for i, tag := range tags {
		quoted[i] = fmt.Sprintf("%q", tag)
	}
	return fmt.Sprintf(`{"specVersion":"0","title":%q,"tags":[%s],"time":{"from":"now-15m","to":"now"},"panels":[{"id":"p","type":"timeseries","title":"p","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]}]}`, title, strings.Join(quoted, ","))
}

func (f *accessFixture) seed(t *testing.T, by auth.Principal, folder pgtype.UUID, title string, tags ...string) string {
	t.Helper()
	dash, _, err := f.service.Create(t.Context(), by, folder, []byte(boardSpec(title, tags...)), "create")
	if err != nil {
		t.Fatal(err)
	}
	return dash.ID
}

func (f *accessFixture) as(t *testing.T, by auth.Principal, tool string, args map[string]any) toolResult {
	t.Helper()
	params := map[string]any{"name": tool, "arguments": args}
	rec := f.post(rpc(1, "tools/call", params), map[string]string{"Authorization": bearer(by)})
	resp := decodeRPC(t, rec)
	if rec.Code != 200 || resp.Error != nil {
		t.Fatalf("tools/call %s as %s: status %d: %s", tool, by.Name, rec.Code, rec.Body.String())
	}
	var out toolResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func text(res toolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		b.WriteString(c.Text)
	}
	return b.String()
}

func TestKeysNeverSeeADashboardInARestrictedFolderTheyHaveNoClaimOn(t *testing.T) {
	f := newAccessFixture(t)
	open := f.seed(t, adminKey, pgtype.UUID{}, openTitle, "open-tag")
	hidden := f.seed(t, adminKey, pgtype.UUID{}, hiddenTitle, hiddenTag)
	hiddenID, err := db.ParseUUID(hidden)
	if err != nil {
		t.Fatal(err)
	}
	foldertest.Move(t, f.db, hiddenID, foldertest.Restricted(t, f.db, hiddenFolder, privateGroup))
	_, comment, err := f.threads.Open(t.Context(), adminKey, threads.OpenInput{Target: threads.Target{Kind: threads.KindDashboard, ID: hiddenID}, Body: commentText}, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, by := range []auth.Principal{editorKey, commenterKey, viewerKey} {
		t.Run(by.Name, func(t *testing.T) {
			list := f.as(t, by, "list_dashboards", map[string]any{})
			if list.IsError || !strings.Contains(text(list), openTitle) {
				t.Fatalf("list_dashboards = %s", text(list))
			}
			if folderList := f.as(t, by, "list_folders", map[string]any{}); folderList.IsError || strings.Contains(text(folderList), hiddenFolder) {
				t.Errorf("list_folders = %s", text(folderList))
			}
			if got := f.as(t, by, "get_dashboard", map[string]any{"dashboard_id": open}); got.IsError {
				t.Errorf("get_dashboard of the open dashboard = %s", text(got))
			}

			refusals := map[string]toolResult{
				"get_dashboard":   f.as(t, by, "get_dashboard", map[string]any{"dashboard_id": hidden}),
				"list_comments":   f.as(t, by, "list_comments", map[string]any{"dashboard_id": hidden}),
				"get version":     f.as(t, by, "get_dashboard", map[string]any{"dashboard_id": hidden, "version": 1}),
				"reply":           f.as(t, by, "reply_to_comment", map[string]any{"comment_id": comment.ID, "body": "hello"}),
				"resolve":         f.as(t, by, "resolve_comment", map[string]any{"comment_id": comment.ID}),
				"reopen":          f.as(t, by, "reopen_comment", map[string]any{"comment_id": comment.ID}),
				"edit":            f.as(t, by, "edit_comment", map[string]any{"comment_id": comment.ID, "body": "changed"}),
				"create":          f.as(t, by, "create_comment", map[string]any{"target": "dashboard:" + hidden, "body": "hello"}),
				"list by target":  f.as(t, by, "list_comments", map[string]any{"target": "dashboard:" + hidden}),
				"update":          f.as(t, by, "update_dashboard", map[string]any{"dashboard_id": hidden, "parent_version": 1, "spec": json.RawMessage(boardSpec("Sneaky"))}),
				"create in there": f.as(t, by, "create_dashboard", map[string]any{"folder_id": f.folderID(t, hiddenFolder), "spec": json.RawMessage(boardSpec("Sneaky"))}),
			}
			for name, res := range refusals {
				if !res.IsError {
					t.Errorf("%s succeeded: %s", name, text(res))
				}
				for _, leak := range []string{hiddenTitle, hiddenTag, hiddenFolder, hidden, commentText} {
					if strings.Contains(text(res), leak) {
						t.Errorf("%s leaks %q: %s", name, leak, text(res))
					}
				}
			}
			for _, name := range []string{"get_dashboard", "get version", "list_comments", "list by target"} {
				if code := refusals[name].StructuredContent["code"]; code != "not_found" {
					t.Errorf("%s code = %v, want not_found", name, code)
				}
			}
		})
	}

	t.Run("the Admin key sees it", func(t *testing.T) {
		list := f.as(t, adminKey, "list_dashboards", map[string]any{})
		if !strings.Contains(text(list), hiddenTitle) || !strings.Contains(text(list), openTitle) {
			t.Fatalf("list_dashboards = %s", text(list))
		}
		if got := f.as(t, adminKey, "get_dashboard", map[string]any{"dashboard_id": hidden}); got.IsError || !strings.Contains(text(got), hiddenTitle) {
			t.Errorf("get_dashboard = %s", text(got))
		}
	})
}

func TestAKeySeesARestrictedFolderItCreated(t *testing.T) {
	f := newAccessFixture(t)
	folder, err := f.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{
		Title:         hiddenFolder,
		CreatedByKind: pgtype.Text{String: string(auth.KindServiceAccount), Valid: true},
		CreatedByID:   mustUUID(t, creatorKey.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.db.Queries().InsertFolderPermission(t.Context(), dbsqlc.InsertFolderPermissionParams{
		FolderID: folder.ID, GroupName: pgtype.Text{String: privateGroup, Valid: true}, Level: "view",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := f.seed(t, creatorKey, folder.ID, hiddenTitle)

	if got := f.as(t, creatorKey, "get_dashboard", map[string]any{"dashboard_id": id}); got.IsError || !strings.Contains(text(got), hiddenTitle) {
		t.Errorf("the creator's get_dashboard = %s", text(got))
	}
	if got := f.as(t, editorKey, "get_dashboard", map[string]any{"dashboard_id": id}); !got.IsError || strings.Contains(text(got), hiddenTitle) {
		t.Errorf("another key's get_dashboard = %s", text(got))
	}
}

func TestKeysBelowEditorQueryButCannotWrite(t *testing.T) {
	f := newAccessFixture(t)
	id := f.seed(t, adminKey, pgtype.UUID{}, openTitle)
	dashID := mustUUID(t, id)
	_, comment, err := f.threads.Open(t.Context(), adminKey, threads.OpenInput{Target: threads.Target{Kind: threads.KindDashboard, ID: dashID}, Body: commentText}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reads := map[string]map[string]any{
		"list_dashboards":  {},
		"get_dashboard":    {"dashboard_id": id},
		"get_spec_guide":   {},
		"list_folders":     {},
		"list_comments":    {"dashboard_id": id},
		"list_datasources": {},
		"query_instant":    {"datasource": "prom", "query": "up"},
	}
	panel := json.RawMessage(`{"id":"p","type":"timeseries","title":"changed","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]}`)
	filed, _, err := f.findings.Save(t.Context(), adminKey, findings.SaveInput{Title: "Slow pod", Markdown: "checkout-0 is slow", From: time.Now().Add(-time.Hour), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	target, err := f.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Payments"})
	if err != nil {
		t.Fatal(err)
	}
	writes := map[string]map[string]any{
		"move_finding":      {"finding_id": filed.ID, "folder_id": target.ID.String()},
		"save_search":       {"kind": "logs", "title": "Errors", "datasource_uid": "loki", "query": "{a=\"b\"}"},
		"create_dashboard":  {"spec": json.RawMessage(boardSpec("Mine"))},
		"update_panels":     {"dashboard_id": id, "parent_version": 1, "summary": "change", "upsert": []json.RawMessage{panel}},
		"create_annotation": {"text": "x"},
	}

	for _, by := range []auth.Principal{commenterKey, viewerKey} {
		t.Run(by.Name, func(t *testing.T) {
			for _, tool := range f.tools {
				if tool.MinRole != auth.RoleEditor {
					continue
				}
				res := f.as(t, by, tool.Name, map[string]any{})
				if !res.IsError || res.StructuredContent["code"] != forbidden {
					t.Errorf("%s = %s, want %s", tool.Name, text(res), forbidden)
				}
			}
			for name, args := range reads {
				if res := f.as(t, by, name, args); res.IsError {
					t.Errorf("%s = %s, want it to work", name, text(res))
				}
			}
			before := f.writtenRows(t)
			for name, args := range writes {
				if res := f.as(t, by, name, args); !res.IsError || res.StructuredContent["code"] != forbidden {
					t.Errorf("%s = %s, want %s", name, text(res), forbidden)
				}
			}
			if after := f.writtenRows(t); after != before {
				t.Errorf("refused writes changed the rows: %d before, %d after", before, after)
			}
		})
	}

	t.Run("a Commenter key replies and resolves, a Viewer key does not", func(t *testing.T) {
		reply := map[string]any{"comment_id": comment.ID, "body": "looking"}
		if res := f.as(t, viewerKey, "reply_to_comment", reply); !res.IsError || res.StructuredContent["code"] != forbidden {
			t.Errorf("Viewer reply = %s, want %s", text(res), forbidden)
		}
		if res := f.as(t, viewerKey, "resolve_comment", map[string]any{"comment_id": comment.ID}); !res.IsError || res.StructuredContent["code"] != forbidden {
			t.Errorf("Viewer resolve = %s, want %s", text(res), forbidden)
		}
		if res := f.as(t, commenterKey, "reply_to_comment", reply); res.IsError {
			t.Errorf("Commenter reply = %s", text(res))
		}
		if res := f.as(t, commenterKey, "resolve_comment", map[string]any{"comment_id": comment.ID}); res.IsError {
			t.Errorf("Commenter resolve = %s", text(res))
		}
		if res := f.as(t, viewerKey, "reopen_comment", map[string]any{"comment_id": comment.ID}); !res.IsError || res.StructuredContent["code"] != forbidden {
			t.Errorf("Viewer reopen = %s, want %s", text(res), forbidden)
		}
		if res := f.as(t, commenterKey, "reopen_comment", map[string]any{"comment_id": comment.ID}); res.IsError {
			t.Errorf("Commenter reopen = %s", text(res))
		}
		if res := f.as(t, viewerKey, "create_comment", map[string]any{"target": "dashboard:" + id, "body": "hi"}); !res.IsError || res.StructuredContent["code"] != forbidden {
			t.Errorf("Viewer create_comment = %s, want %s", text(res), forbidden)
		}
		created := f.as(t, commenterKey, "create_comment", map[string]any{"target": "dashboard:" + id, "body": "my own"})
		if created.IsError {
			t.Fatalf("Commenter create_comment = %s", text(created))
		}
		var made struct {
			Comment struct {
				ID string `json:"id"`
			} `json:"comment"`
		}
		if err := json.Unmarshal([]byte(text(created)), &made); err != nil {
			t.Fatal(err)
		}
		if res := f.as(t, commenterKey, "edit_comment", map[string]any{"comment_id": made.Comment.ID, "body": "my own, edited"}); res.IsError {
			t.Errorf("Commenter edit_comment = %s", text(res))
		}
		if res := f.as(t, editorKey, "edit_comment", map[string]any{"comment_id": made.Comment.ID, "body": "not mine"}); !res.IsError || res.StructuredContent["code"] != "not_author" {
			t.Errorf("another key's edit_comment = %s, want not_author", text(res))
		}
	})

	t.Run("an Editor key writes", func(t *testing.T) {
		if res := f.as(t, editorKey, "create_dashboard", map[string]any{"spec": json.RawMessage(boardSpec("Mine"))}); res.IsError {
			t.Errorf("Editor create_dashboard = %s", text(res))
		}
	})
}

func TestMoveFindingFollowsWhoMayReadAndEditTheFinding(t *testing.T) {
	f := newAccessFixture(t)
	saved, _, err := f.findings.Save(t.Context(), viewerKey, findings.SaveInput{Title: "Slow pod", Markdown: "checkout-0 is slow", From: time.Now().Add(-time.Hour), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := f.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Shared"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	move := func(folder pgtype.UUID) map[string]any {
		return map[string]any{"finding_id": saved.ID, "folder_id": folder.String()}
	}

	if res := f.as(t, editorKey, "move_finding", move(shared.ID)); !res.IsError || res.StructuredContent["code"] != "not_found" {
		t.Errorf("an Editor key moving a finding in no folder that it did not save = %s, want not_found", text(res))
	}
	moved := f.as(t, adminKey, "move_finding", move(shared.ID))
	if moved.IsError || moved.StructuredContent["folder"] != "Shared" || moved.StructuredContent["folder_id"] != shared.ID.String() {
		t.Fatalf("an Admin key's move = %s", text(moved))
	}
	if res := f.as(t, editorKey, "move_finding", move(other.ID)); res.IsError || res.StructuredContent["folder"] != "Other" {
		t.Errorf("an Editor key moving a finding filed in a folder it can edit = %s", text(res))
	}
}

func TestSavedSearchToolsFollowTheKeysRole(t *testing.T) {
	f := newAccessFixture(t)
	input := map[string]any{"kind": "logs", "title": "Errors", "datasource_uid": "loki", "query": "{a=\"b\"}"}
	onMetrics := map[string]any{"kind": "logs", "title": "Errors", "datasource_uid": "prom", "query": "{a=\"b\"}"}

	if res := f.as(t, viewerKey, "save_search", input); !res.IsError || res.StructuredContent["code"] != forbidden {
		t.Errorf("a Viewer key's save_search = %s, want %s", text(res), forbidden)
	}
	if res := f.as(t, editorKey, "save_search", onMetrics); !res.IsError || res.StructuredContent["code"] != "invalid_request" {
		t.Errorf("an Editor key saving on a metrics datasource = %s, want invalid_request", text(res))
	}
	if res := f.as(t, viewerKey, "list_saved_searches", map[string]any{}); res.IsError {
		t.Errorf("a Viewer key's list_saved_searches = %s", text(res))
	}
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *accessFixture) folderID(t *testing.T, title string) string {
	t.Helper()
	rows, err := f.db.Queries().FolderFacts(t.Context(), dbsqlc.FolderFactsParams{Groups: []string{}, Kind: string(auth.KindUser)})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Title == title {
			return row.ID.String()
		}
	}
	t.Fatalf("no folder %q", title)
	return ""
}

func TestAViewerOverOAuthDoesWhatTheWebAllows(t *testing.T) {
	f := newAccessFixture(t)
	vera := f.person(t, "vera", auth.RoleViewer)
	general := f.seed(t, adminKey, pgtype.UUID{}, openTitle)
	panel := json.RawMessage(`{"id":"p","type":"timeseries","title":"changed","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]}`)
	patch := func(id string) map[string]any {
		return map[string]any{"dashboard_id": id, "parent_version": 1, "summary": "change", "upsert": []json.RawMessage{panel}}
	}
	refused := func(t *testing.T, res toolResult) {
		t.Helper()
		if !res.IsError || res.StructuredContent["code"] != forbidden {
			t.Errorf("got %s, want %s", text(res), forbidden)
		}
	}

	if res := f.as(t, vera, "query_instant", map[string]any{"datasource": "prom", "query": "up"}); res.IsError {
		t.Fatalf("query_instant = %s", text(res))
	}
	created := f.as(t, vera, "create_dashboard", map[string]any{"spec": json.RawMessage(boardSpec("Mine"))})
	if created.IsError {
		t.Fatalf("create_dashboard = %s", text(created))
	}
	var made struct {
		DashboardID string `json:"dashboard_id"`
	}
	if err := json.Unmarshal([]byte(text(created)), &made); err != nil {
		t.Fatal(err)
	}
	dash, _, err := f.service.Get(t.Context(), mustUUID(t, made.DashboardID))
	if err != nil || dash.Folder.Title != "My Dashboards" {
		t.Fatalf("the dashboard landed in %q, %v; want My Dashboards", dash.Folder.Title, err)
	}

	if res := f.as(t, vera, "update_panels", patch(made.DashboardID)); res.IsError {
		t.Errorf("update_panels on her own dashboard = %s", text(res))
	}
	if res := f.as(t, vera, "create_comment", map[string]any{"target": "dashboard:" + made.DashboardID, "body": "note"}); res.IsError {
		t.Errorf("create_comment on her own dashboard = %s", text(res))
	}
	refused(t, f.as(t, vera, "update_panels", patch(general)))
	refused(t, f.as(t, vera, "create_comment", map[string]any{"target": "dashboard:" + general, "body": "note"}))

	finding := f.as(t, vera, "create_finding", map[string]any{"markdown": "x"})
	refused(t, finding)
	if want := "your role (viewer) cannot use create_finding; an admin can change your role"; !strings.Contains(text(finding), want) {
		t.Errorf("create_finding refusal = %s, want %q", text(finding), want)
	}

	versions, _, err := f.service.ListVersions(t.Context(), mustUUID(t, made.DashboardID), 10, "")
	if err != nil || len(versions) == 0 {
		t.Fatalf("versions = %v, %v", versions, err)
	}
	for _, v := range versions {
		if v.Origin != "mcp" || v.OriginClient != "Claude Code" {
			t.Errorf("version %d origin = %q client %q, want mcp and Claude Code", v.Number, v.Origin, v.OriginClient)
		}
	}
}
