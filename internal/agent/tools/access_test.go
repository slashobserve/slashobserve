package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

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
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/threads"
)

const (
	privateGroup = "payroll-team"
	hiddenTitle  = "Payroll board"
	hiddenTag    = "payroll-tag"
	hiddenFolder = "Payroll"
	openTitle    = "Open board"
)

var (
	adminCaller     = auth.Principal{Kind: auth.KindUser, ID: "77777777-7777-4777-8777-777777777777", Name: "Root", Email: "root@example.com", Role: auth.RoleAdmin}
	editorCaller    = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Eve", Email: "eve@example.com", Role: auth.RoleEditor}
	commenterCaller = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Cam", Email: "cam@example.com", Role: auth.RoleCommenter}
	viewerCaller    = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Vic", Email: "vic@example.com", Role: auth.RoleViewer}
	groupCaller     = auth.Principal{Kind: auth.KindUser, ID: "66666666-6666-4666-8666-666666666666", Name: "Gia", Email: "gia@example.com", Role: auth.RoleViewer, Groups: []string{privateGroup}}
	keyCaller       = auth.Principal{Kind: auth.KindServiceAccount, ID: "33333333-3333-4333-8333-333333333333", Name: "ci", Role: auth.RoleEditor}
)

type accessRig struct {
	registry *Registry
	service  *dashboards.Service
	threads  *threads.Service
	notes    *annotations.Service
	findings *findings.Service
	db       *dbtest.Database
}

func newAccessRig(t *testing.T) *accessRig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	frames := func() []datasource.Frame { return []datasource.Frame{timeNumberFrame(nil, 1, 2)} }
	ds := &fakeDS{
		rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
			return frames(), nil
		},
		instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return frames(), nil
		},
	}
	datasources, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(datasources, logger)
	access := folders.NewAccess(database.DB)
	service := dashboards.New(database.DB, runner, access, logger)
	notes := annotations.New(database.DB, access)
	found := findings.New(database.DB, access, notes, service, logger)
	commentThreads := threads.NewService(database.DB, access, service, found, notes, observations.New(database.DB, access, nil, nil, logger))
	registry := New(Deps{Datasources: datasources, Runner: runner, Dashboards: service, Threads: commentThreads, Folders: access, Annotations: notes, Logger: logger})
	return &accessRig{registry: registry, service: service, threads: commentThreads, notes: notes, findings: found, db: database}
}

func boardSpec(title string, tags ...string) string {
	encoded, err := json.Marshal(append([]string{}, tags...))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`{"specVersion":"0","title":%q,"tags":%s,"time":{"from":"now-15m","to":"now"},"panels":[%s]}`, title, encoded, statPanel("p_up", 0))
}

func (r *accessRig) call(t *testing.T, by auth.Principal, tool, input string) Result {
	t.Helper()
	res, err := r.registry.Call(auth.WithPrincipal(t.Context(), by), tool, []byte(input))
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return res
}

func textOf(t *testing.T, res Result) string {
	t.Helper()
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (r *accessRig) seed(t *testing.T, title string, tags ...string) pgtype.UUID {
	t.Helper()
	dash, _, err := r.service.Create(t.Context(), adminCaller, pgtype.UUID{}, []byte(boardSpec(title, tags...)), "create")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *accessRig) newFolder(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	folder, err := r.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return folder.ID
}

type hiddenBoards struct {
	open, hidden pgtype.UUID
	folder       pgtype.UUID
	commentID    string
}

func (r *accessRig) seedHidden(t *testing.T) hiddenBoards {
	t.Helper()
	b := hiddenBoards{open: r.seed(t, openTitle, "open-tag"), hidden: r.seed(t, hiddenTitle, hiddenTag)}
	b.folder = foldertest.Restricted(t, r.db, hiddenFolder, privateGroup)
	foldertest.Move(t, r.db, b.hidden, b.folder)
	_, message, err := r.threads.Open(t.Context(), adminCaller, threads.OpenInput{Target: threads.Target{Kind: threads.KindDashboard, ID: b.hidden}, Body: "who gets paid what"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b.commentID = message.ID
	return b
}

func TestRestrictedFolderDashboardsNeverLeakThroughTools(t *testing.T) {
	r := newAccessRig(t)
	b := r.seedHidden(t)
	hiddenID := b.hidden.String()
	leaks := []string{hiddenTitle, hiddenTag, hiddenFolder, hiddenID, "who gets paid what"}
	writes := []struct {
		tool, input string
		floor       auth.Role
	}{
		{nameListComments, `{"dashboard_id":"` + hiddenID + `"}`, auth.RoleViewer},
		{nameReplyToComment, `{"comment_id":"` + b.commentID + `","body":"hello"}`, auth.RoleCommenter},
		{nameResolveComment, `{"comment_id":"` + b.commentID + `"}`, auth.RoleCommenter},
		{nameUpdateDashboard, `{"dashboard_id":"` + hiddenID + `","parent_version":1,"spec":` + boardSpec("Changed") + `}`, auth.RoleEditor},
		{nameCreateDashboard, `{"folder_id":"` + b.folder.String() + `","spec":` + boardSpec("Sneaky") + `}`, auth.RoleEditor},
	}

	for _, by := range []auth.Principal{editorCaller, commenterCaller, viewerCaller, keyCaller} {
		t.Run(by.Name, func(t *testing.T) {
			list := r.call(t, by, nameListDashboards, `{}`)
			listed, ok := list.Content.(dashboardListOut)
			if list.IsError || !ok || len(listed.Dashboards) != 1 || listed.Dashboards[0].Title != openTitle {
				t.Fatalf("list_dashboards = %s, want only %q", textOf(t, list), openTitle)
			}
			if folderList := r.call(t, by, nameListFolders, `{}`); folderList.IsError || strings.Contains(textOf(t, folderList), hiddenFolder) {
				t.Errorf("list_folders = %s", textOf(t, folderList))
			}
			if got := r.call(t, by, nameGetDashboard, `{"dashboard_id":"`+b.open.String()+`"}`); got.IsError {
				t.Errorf("get_dashboard of the open dashboard = %s", textOf(t, got))
			}

			refusals := []Result{
				r.call(t, by, nameGetDashboard, `{"dashboard_id":"`+hiddenID+`"}`),
				r.call(t, by, nameGetDashboard, `{"dashboard_id":"`+hiddenID+`","version":1}`),
			}
			for _, w := range writes {
				if by.Role.AtLeast(w.floor) {
					refusals = append(refusals, r.call(t, by, w.tool, w.input))
				}
			}
			for _, res := range refusals {
				if code := errorCode(t, res); code != codeNotFound {
					t.Errorf("a call on the hidden dashboard answered %s, want not_found", textOf(t, res))
				}
				for _, leak := range leaks {
					if strings.Contains(textOf(t, res), leak) {
						t.Errorf("a refusal leaks %q: %s", leak, textOf(t, res))
					}
				}
			}
		})
	}

	for _, by := range []auth.Principal{groupCaller, adminCaller} {
		t.Run(by.Name+" has access", func(t *testing.T) {
			list, ok := r.call(t, by, nameListDashboards, `{}`).Content.(dashboardListOut)
			if !ok || len(list.Dashboards) != 2 {
				t.Fatalf("list_dashboards = %+v, want both dashboards", list)
			}
			get := r.call(t, by, nameGetDashboard, `{"dashboard_id":"`+hiddenID+`"}`)
			if out, ok := get.Content.(dashboardOut); get.IsError || !ok || out.Title != hiddenTitle || out.Folder != hiddenFolder {
				t.Fatalf("get_dashboard = %s", textOf(t, get))
			}
			if folderList := r.call(t, by, nameListFolders, `{}`); !strings.Contains(textOf(t, folderList), hiddenFolder) {
				t.Errorf("list_folders = %s, want %s", textOf(t, folderList), hiddenFolder)
			}
			if comments := r.call(t, by, nameListComments, `{"dashboard_id":"`+hiddenID+`"}`); comments.IsError || !strings.Contains(textOf(t, comments), "who gets paid what") {
				t.Errorf("list_comments = %s", textOf(t, comments))
			}
		})
	}
}

func (r *accessRig) createdIn(t *testing.T, res Result) string {
	t.Helper()
	saved, ok := res.Content.(savedOut)
	if res.IsError || !ok {
		t.Fatalf("create_dashboard = %s", textOf(t, res))
	}
	id, err := db.ParseUUID(saved.DashboardID)
	if err != nil {
		t.Fatal(err)
	}
	dash, _, err := r.service.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return dash.Folder.Title
}

func (r *accessRig) create(t *testing.T, by auth.Principal, title string, folder pgtype.UUID) Result {
	t.Helper()
	input := `{"spec":` + boardSpec(title) + `}`
	if folder.Valid {
		input = `{"folder_id":"` + folder.String() + `","spec":` + boardSpec(title) + `}`
	}
	return r.call(t, by, nameCreateDashboard, input)
}

func (r *accessRig) createdSaved(t *testing.T, res Result) string {
	t.Helper()
	saved, ok := res.Content.(savedOut)
	if res.IsError || !ok {
		t.Fatalf("create_dashboard = %s", textOf(t, res))
	}
	return saved.DashboardID
}

func (r *accessRig) person(t *testing.T, subject string, role auth.Role) auth.Principal {
	t.Helper()
	u, err := r.db.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{
		OidcIssuer: "i", OidcSubject: subject, Email: subject + "@example.com", Name: subject,
	})
	if err != nil {
		t.Fatal(err)
	}
	return auth.Principal{Kind: auth.KindUser, ID: u.ID.String(), Name: subject, Role: role}
}

func TestCreateDashboardWithoutAFolderUsesTheDefaultFolder(t *testing.T) {
	t.Run("a key lands in General even after an explicit create in another folder", func(t *testing.T) {
		r := newAccessRig(t)
		ops := r.newFolder(t, "Ops")
		if got := r.createdIn(t, r.create(t, keyCaller, "In ops", ops)); got != "Ops" {
			t.Fatalf("explicit folder: landed in %q, want Ops", got)
		}
		if got := r.createdIn(t, r.create(t, keyCaller, "Next", pgtype.UUID{})); got != "General" {
			t.Fatalf("no folder after Ops: landed in %q, want General", got)
		}
	})

	t.Run("a person lands in their My Dashboards even after an explicit create elsewhere", func(t *testing.T) {
		r := newAccessRig(t)
		eve := r.person(t, "eve", auth.RoleEditor)
		ops := r.newFolder(t, "Ops")
		if got := r.createdIn(t, r.create(t, eve, "In ops", ops)); got != "Ops" {
			t.Fatalf("explicit folder: landed in %q, want Ops", got)
		}
		if got := r.createdIn(t, r.create(t, eve, "Next", pgtype.UUID{})); got != "My Dashboards" {
			t.Fatalf("no folder: landed in %q, want My Dashboards", got)
		}
	})

	t.Run("each person gets their own My Dashboards", func(t *testing.T) {
		r := newAccessRig(t)
		eve, bob := r.person(t, "eve", auth.RoleEditor), r.person(t, "bob", auth.RoleEditor)
		mine := r.create(t, eve, "Mine", pgtype.UUID{})
		theirs := r.create(t, bob, "Theirs", pgtype.UUID{})
		if folderOf(t, r, mine) == folderOf(t, r, theirs) {
			t.Fatal("two people landed in the same personal folder")
		}
	})

	t.Run("a principal with no person record lands in General", func(t *testing.T) {
		r := newAccessRig(t)
		if got := r.createdIn(t, r.create(t, editorCaller, "Mine", pgtype.UUID{})); got != "General" {
			t.Fatalf("landed in %q, want General", got)
		}
	})

	t.Run("an agent run saves in the requester's My Dashboards", func(t *testing.T) {
		r := newAccessRig(t)
		eve := r.person(t, "eve", auth.RoleEditor)
		ops := r.newFolder(t, "Ops")
		r.create(t, eve, "In ops", ops)
		ctx := WithRun(auth.WithPrincipal(t.Context(), eve), Run{Mode: ModeCreate})
		res, err := r.registry.Call(ctx, nameCreateDashboard, []byte(`{"spec":`+boardSpec("From a run")+`}`))
		if err != nil {
			t.Fatal(err)
		}
		if got := r.createdIn(t, res); got != "My Dashboards" {
			t.Fatalf("landed in %q, want My Dashboards", got)
		}
	})

	t.Run("a key that cannot create in General is refused", func(t *testing.T) {
		r := newAccessRig(t)
		viewerKey := auth.Principal{Kind: auth.KindServiceAccount, ID: "99999999-9999-4999-8999-999999999999", Name: "reader", Role: auth.RoleViewer}
		ctx := WithRun(auth.WithPrincipal(t.Context(), viewerKey), Run{Mode: ModeCreate})
		res, err := r.registry.Call(ctx, nameCreateDashboard, []byte(`{"spec":`+boardSpec("Refused")+`}`))
		if err != nil {
			t.Fatal(err)
		}
		if code := errorCode(t, res); code != codeForbiddenRole {
			t.Fatalf("error code = %s, want %s", code, codeForbiddenRole)
		}
	})
}

func folderOf(t *testing.T, r *accessRig, res Result) pgtype.UUID {
	t.Helper()
	saved, ok := res.Content.(savedOut)
	if res.IsError || !ok {
		t.Fatalf("create_dashboard = %s", textOf(t, res))
	}
	id, err := db.ParseUUID(saved.DashboardID)
	if err != nil {
		t.Fatal(err)
	}
	folderID, err := r.db.Queries().DashboardFolder(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return folderID
}
