package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	dashA = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	dashB = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"

	lookalikeID = "0000000000000x0000x0000x000000000000"
)

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatalf("bad uuid %q: %v", raw, err)
	}
	return id
}

func TestUpdateTarget(t *testing.T) {
	two, zero, big := 2, 0, 1<<40
	revise := Run{Mode: ModeRevise, DashboardID: dashA, ParentVersion: 4}
	tests := []struct {
		name       string
		in         saveInput
		run        Run
		scoped     bool
		wantID     string
		wantParent int
		wantTool   string
		wantErr    bool
	}{
		{name: "mcp needs both", in: saveInput{DashboardID: dashA, ParentVersion: &two}, wantID: dashA, wantParent: 2},
		{name: "mcp missing id", in: saveInput{ParentVersion: &two}, wantTool: "dashboard_id is required"},
		{name: "mcp missing parent", in: saveInput{DashboardID: dashA}, wantTool: "parent_version is required"},
		{name: "mcp bad id", in: saveInput{DashboardID: "nope", ParentVersion: &two}, wantTool: "dashboard_id must be a UUID"},
		{name: "mcp lookalike id", in: saveInput{DashboardID: lookalikeID, ParentVersion: &two}, wantTool: "dashboard_id must be a UUID"},
		{name: "mcp undashed id", in: saveInput{DashboardID: strings.ReplaceAll(dashA, "-", ""), ParentVersion: &two}, wantTool: "dashboard_id must be a UUID"},
		{name: "mcp parent zero", in: saveInput{DashboardID: dashA, ParentVersion: &zero}, wantTool: "parent_version must be between"},
		{name: "mcp parent huge", in: saveInput{DashboardID: dashA, ParentVersion: &big}, wantTool: "parent_version must be between"},
		{name: "mcp upper case id", in: saveInput{DashboardID: strings.ToUpper(dashA), ParentVersion: &two}, wantID: dashA, wantParent: 2},

		{name: "create run", in: saveInput{DashboardID: dashA, ParentVersion: &two}, run: Run{Mode: ModeCreate}, scoped: true, wantTool: "this run creates a new dashboard"},
		{name: "revise fills from run", run: revise, scoped: true, wantID: dashA, wantParent: 4},
		{name: "revise explicit matching id", in: saveInput{DashboardID: dashA}, run: revise, scoped: true, wantID: dashA, wantParent: 4},
		{name: "revise id differs by case", in: saveInput{DashboardID: strings.ToUpper(dashA)}, run: revise, scoped: true, wantID: dashA, wantParent: 4},
		{name: "revise run id upper case", in: saveInput{DashboardID: dashA}, run: Run{Mode: ModeRevise, DashboardID: strings.ToUpper(dashA), ParentVersion: 4}, scoped: true, wantID: dashA, wantParent: 4},
		{name: "revise other id", in: saveInput{DashboardID: dashB}, run: revise, scoped: true, wantTool: "dashboard_id must match it"},
		{name: "revise malformed id", in: saveInput{DashboardID: "nope"}, run: revise, scoped: true, wantTool: "dashboard_id must be a UUID"},
		{name: "revise explicit parent", in: saveInput{ParentVersion: &two}, run: revise, scoped: true, wantID: dashA, wantParent: 2},
		{name: "revise lookalike id", in: saveInput{DashboardID: lookalikeID}, run: revise, scoped: true, wantTool: "dashboard_id must be a UUID"},
		{name: "revise parent zero", in: saveInput{ParentVersion: &zero}, run: revise, scoped: true, wantTool: "parent_version must be between"},

		{name: "empty mode", run: Run{}, scoped: true, wantErr: true},
		{name: "unknown mode", run: Run{Mode: "edit", DashboardID: dashA, ParentVersion: 1}, scoped: true, wantErr: true},
		{name: "revise without id", run: Run{Mode: ModeRevise, ParentVersion: 1}, scoped: true, wantErr: true},
		{name: "revise with lookalike run id", run: Run{Mode: ModeRevise, DashboardID: lookalikeID, ParentVersion: 1}, scoped: true, wantErr: true},
		{name: "revise with malformed run id", run: Run{Mode: ModeRevise, DashboardID: "x", ParentVersion: 1}, scoped: true, wantErr: true},
		{name: "revise without parent", run: Run{Mode: ModeRevise, DashboardID: dashA}, scoped: true, wantErr: true},
		{name: "revise negative parent", run: Run{Mode: ModeRevise, DashboardID: dashA, ParentVersion: -1}, scoped: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, res, ok, err := updateTarget(tt.in, tt.run, tt.scoped)
			switch {
			case tt.wantErr:
				if !errors.Is(err, errInvalidRun) || ok {
					t.Fatalf("ok = %v err = %v, want an invalid run error", ok, err)
				}
			case tt.wantTool != "":
				if err != nil || ok {
					t.Fatalf("ok = %v err = %v, want a tool error", ok, err)
				}
				if msg := res.Content.(errorBody).Message; !strings.Contains(msg, tt.wantTool) {
					t.Fatalf("message = %q, want it to contain %q", msg, tt.wantTool)
				}
				if errorCode(t, res) != codeInvalidRequest {
					t.Fatalf("code = %q", errorCode(t, res))
				}
			default:
				if err != nil || !ok {
					t.Fatalf("ok = %v err = %v res = %#v", ok, err, res.Content)
				}
				if target.id != mustUUID(t, tt.wantID) || target.parent != tt.wantParent {
					t.Fatalf("target = %+v, want %s/%d", target, tt.wantID, tt.wantParent)
				}
			}
		})
	}
}

func TestSaveToolsRunScopeThroughCall(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleEditor})
	spec := `{"spec":{"title":"t"}}`

	t.Run("create in a revise run", func(t *testing.T) {
		res, err := r.Call(WithRun(ctx, Run{Mode: ModeRevise, DashboardID: dashA, ParentVersion: 1}), nameCreateDashboard, []byte(spec))
		if err != nil {
			t.Fatal(err)
		}
		if errorCode(t, res) != codeInvalidRequest || !strings.Contains(res.Content.(errorBody).Message, dashA) {
			t.Fatalf("res = %#v", res.Content)
		}
		if !strings.HasPrefix(res.Step, "Rejected create_dashboard input: this run revises dashboard") {
			t.Fatalf("step = %q", res.Step)
		}
	})
	t.Run("update in a create run", func(t *testing.T) {
		res, err := r.Call(WithRun(ctx, Run{Mode: ModeCreate}), nameUpdateDashboard, []byte(spec))
		if err != nil {
			t.Fatal(err)
		}
		if errorCode(t, res) != codeInvalidRequest || !strings.Contains(res.Content.(errorBody).Message, "call create_dashboard") {
			t.Fatalf("res = %#v", res.Content)
		}
	})
	t.Run("update for another dashboard", func(t *testing.T) {
		in := `{"dashboard_id":"` + dashB + `","spec":{"title":"t"}}`
		res, err := r.Call(WithRun(ctx, Run{Mode: ModeRevise, DashboardID: dashA, ParentVersion: 1}), nameUpdateDashboard, []byte(in))
		if err != nil || errorCode(t, res) != codeInvalidRequest {
			t.Fatalf("res = %#v err = %v", res, err)
		}
	})
	for _, run := range []Run{{}, {Mode: "edit"}, {Mode: ModeRevise}, {Mode: ModeRevise, DashboardID: dashA}} {
		for _, tool := range []string{nameCreateDashboard, nameUpdateDashboard} {
			res, err := r.Call(WithRun(ctx, run), tool, []byte(spec))
			if err == nil || res.Content != nil || res.IsError {
				t.Errorf("%s with %+v: res = %#v err = %v, want an internal error", tool, run, res, err)
			}
		}
	}
	t.Run("request id", func(t *testing.T) {
		_, err := r.Call(WithRun(ctx, Run{Mode: ModeCreate, RequestID: "r1"}), nameCreateDashboard, []byte(spec))
		if !errors.Is(err, errInvalidRun) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no principal", func(t *testing.T) {
		if _, err := r.Call(WithRun(reading(context.Background()), Run{Mode: ModeCreate}), nameCreateDashboard, []byte(spec)); err == nil {
			t.Fatal("expected an internal error")
		}
	})
	t.Run("spec required", func(t *testing.T) {
		res, err := r.Call(ctx, nameCreateDashboard, []byte(`{}`))
		if err != nil || errorCode(t, res) != codeInvalidRequest {
			t.Fatalf("res = %#v err = %v", res, err)
		}
	})
	t.Run("mcp update needs both fields", func(t *testing.T) {
		res, err := r.Call(ctx, nameUpdateDashboard, []byte(spec))
		if err != nil || errorCode(t, res) != codeInvalidRequest {
			t.Fatalf("res = %#v err = %v", res, err)
		}
	})
}

func TestASaveFailureStepNamesTheFirstIssuePath(t *testing.T) {
	long := "/panels/0/" + strings.Repeat("x", 200)
	tests := []struct {
		name   string
		issues []spec.Issue
		want   string
	}{
		{"a path", []spec.Issue{{Path: "/panels/0/fieldConfig/defaults/unit", Code: spec.CodeSchema}, {Path: "/title", Code: spec.CodeSchema}}, "Save rejected: 2 issues (schema) at /panels/0/fieldConfig/defaults/unit"},
		{"no path", []spec.Issue{{Code: spec.CodeSchema}}, "Save rejected: 1 issue (schema)"},
		{"a long path is clipped", []spec.Issue{{Path: long, Code: spec.CodeSchema}}, "Save rejected: 1 issue (schema) at " + long[:maxStepReasonBytes] + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := saveFailure(&dashboards.InvalidSpecError{Issues: tt.issues})
			if err != nil || res.Step != tt.want {
				t.Fatalf("step = %q err = %v, want %q", res.Step, err, tt.want)
			}
		})
	}
}

func TestGetDashboardInABuildRunPointsAtGetPanels(t *testing.T) {
	rig := newPanelsRig(t)
	run := rig.createRun(t)
	res, _ := run.run(t, nameGetDashboard, `{"dashboard_id":"build"}`)
	want := "dashboard_id must be a UUID from list_dashboards; your build has no id until you finish, so read its panels with get_panels without dashboard_id"
	if got := refusal(t, res); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}

	r := newTestRegistry(t, &fakeDS{}, nil)
	outside, err := call(t, r, nameGetDashboard, `{"dashboard_id":"build"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := refusal(t, outside), "dashboard_id must be a UUID from list_dashboards or create_dashboard"; got != want {
		t.Errorf("message outside a run = %q, want %q", got, want)
	}
}

func TestListDashboardsRejectsAnOrderItDoesNotOffer(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, nameListDashboards, `{"order":"title"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := refusal(t, res), "order must be updated, views or name"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestListDashboardsSchemaOffersEveryOrder(t *testing.T) {
	var schema struct {
		Properties struct {
			Order struct {
				Enum []string `json:"enum"`
			} `json:"order"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(mcpWording.replacer.Replace(schemaListDashboards)), &schema); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(schema.Properties.Order.Enum, ","); got != "updated,views,name" {
		t.Errorf("order enum = %q, want updated,views,name", got)
	}
	for _, text := range []string{descListDashboards, descGetDashboard} {
		if !strings.Contains(text, "view") {
			t.Errorf("description does not mention views: %s", text)
		}
	}
}

func TestGetDashboardNamesTheViewWindowLikeTheAPI(t *testing.T) {
	r := newAccessRig(t)
	access := folders.NewAccess(r.db.DB)
	logger := slog.New(slog.DiscardHandler)
	withHealth := *r
	withHealth.registry = New(Deps{Dashboards: r.service, Folders: access, Health: health.New(health.Deps{DB: r.db.DB, Access: access, Logger: logger, Job: "dashboard_health"}), Logger: logger})
	eve := r.person(t, "eve", auth.RoleViewer)
	id := r.seed(t, "Board")

	res := withHealth.call(t, eve, nameGetDashboard, `{"dashboard_id":"`+id.String()+`"}`)
	raw, err := json.Marshal(res.Content)
	if err != nil || res.IsError {
		t.Fatalf("get_dashboard = %s (%v)", textOf(t, res), err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["view_count"]) != "0" || string(fields["window_days"]) != "30" {
		t.Errorf("view_count = %s, window_days = %s, want 0 and 30", fields["view_count"], fields["window_days"])
	}
	if _, old := fields["view_window_days"]; old {
		t.Error("get_dashboard still answers view_window_days")
	}
}

func TestListDashboardsExcludesTheAuditFolderOnlyWhenAsked(t *testing.T) {
	r := newAccessRig(t)
	r.seed(t, "Board")
	trail := r.newFolder(t, "Trail")
	if _, _, err := r.service.Create(reading(t.Context()), adminCaller, trail, []byte(boardSpec("Sign-ins")), "create"); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, folders.AuditStarterKey, trail)

	for _, tt := range []struct {
		input string
		want  int
	}{
		{`{}`, 2},
		{`{"exclude_audit":false}`, 2},
		{`{"exclude_audit":true}`, 1},
		{`{"exclude_audit":true,"order":"name","limit":1}`, 1},
	} {
		list, ok := r.call(t, adminCaller, nameListDashboards, tt.input).Content.(dashboardListOut)
		if !ok || len(list.Dashboards) != tt.want {
			t.Errorf("list_dashboards %s = %+v, want %d dashboards", tt.input, list, tt.want)
		}
	}
	if list := r.call(t, adminCaller, nameListDashboards, `{"exclude_audit":true}`).Content.(dashboardListOut); list.Dashboards[0].Title != "Board" {
		t.Errorf("the dashboard left is %q, want Board", list.Dashboards[0].Title)
	}
}

func TestListFoldersPrintsTheAuditFlagOnEveryRow(t *testing.T) {
	r := newAccessRig(t)
	trail := r.newFolder(t, "Trail")
	r.newFolder(t, "Payments")
	r.db.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, folders.AuditStarterKey, trail)

	list, ok := r.call(t, adminCaller, nameListFolders, `{}`).Content.(folderListOut)
	if !ok || len(list.Folders) < 3 {
		t.Fatalf("list_folders = %+v", list)
	}
	for _, f := range list.Folders {
		raw, err := json.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		want := `"audit":false`
		if f.ID == trail.String() {
			want = `"audit":true`
		}
		if !strings.Contains(string(raw), want) {
			t.Errorf("folder %s = %s, want %s", f.Title, raw, want)
		}
	}
}
