package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type panelsRig struct {
	registry *Registry
	service  *dashboards.Service
	database *dbtest.Database
	ctx      context.Context

	mu  sync.Mutex
	ran []string
}

func newPanelsRig(t *testing.T) *panelsRig {
	t.Helper()
	rig := &panelsRig{database: dbtest.New(t), ctx: auth.WithPrincipal(context.Background(), placeholderUser)}
	logger := slog.New(slog.DiscardHandler)
	frames := func(text string, tr datasource.TimeRange) ([]datasource.Frame, error) {
		rig.mu.Lock()
		rig.ran = append(rig.ran, text)
		rig.mu.Unlock()
		switch {
		case strings.Contains(text, "broken"):
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "parse error")
		case strings.Contains(text, "empty"), strings.Contains(text, "short") && tr.To.Sub(tr.From) > time.Hour:
			return nil, nil
		}
		return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
	}
	ds := &fakeDS{
		rangeFn: func(_ context.Context, q datasource.Query, tr datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
			return frames(q.Text, tr)
		},
		instantFn: func(_ context.Context, q datasource.Query, tr datasource.TimeRange) ([]datasource.Frame, error) {
			return frames(q.Text, tr)
		},
	}
	datasources, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(datasources, logger)
	access := folders.NewAccess(rig.database.DB)
	rig.service = dashboards.New(rig.database.DB, runner, access, logger)
	rig.registry = New(Deps{Datasources: datasources, Runner: runner, Dashboards: rig.service, Folders: access, Logger: logger})
	return rig
}

func boardSpecOf(panels ...string) string {
	return `{"specVersion":"0","title":"Board","tags":["ops"],"time":{"from":"now-15m","to":"now"},"variables":[{"name":"svc","type":"custom","options":["a","b"]}],"panels":[` + strings.Join(panels, ",") + `]}`
}

func (r *panelsRig) seed(t *testing.T, panels ...string) string {
	t.Helper()
	dash, _, err := r.service.Create(r.ctx, placeholderUser, pgtype.UUID{}, []byte(boardSpecOf(panels...)), "seed")
	if err != nil {
		t.Fatal(err)
	}
	return dash.ID
}

func (r *panelsRig) seedStats(t *testing.T) string {
	t.Helper()
	return r.seed(t, suggestStat("p1", "One", "count(up)", 0), suggestStat("p2", "Two", "sum(up)", 4), rowPanel("r1", 8, suggestStat("p3", "Three", "max(up)", 9)))
}

type panelsCaller struct {
	rig *panelsRig
	ctx context.Context
}

func (r *panelsRig) as(ctx context.Context) panelsCaller { return panelsCaller{rig: r, ctx: ctx} }

func (c panelsCaller) run(t *testing.T, tool, input string) (Result, []string) {
	r, ctx := c.rig, c.ctx
	t.Helper()
	r.mu.Lock()
	r.ran = nil
	r.mu.Unlock()
	res, err := r.registry.Call(ctx, tool, []byte(input))
	if err != nil {
		t.Fatalf("%s %s: %v", tool, input, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return res, slices.Compact(slices.Sorted(slices.Values(r.ran)))
}

func (r *panelsRig) latest(t *testing.T, id string) dashboards.Version {
	t.Helper()
	_, ver, err := r.service.Get(r.ctx, mustUUID(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return ver
}

func (r *panelsRig) reviseCtx(id string, parent int) context.Context {
	return WithRun(r.ctx, Run{Mode: ModeRevise, DashboardID: id, ParentVersion: parent})
}

func panelPatch(id string, parent int, rest string) string {
	return fmt.Sprintf(`{"dashboard_id":%q,"parent_version":%d,"summary":"Change","upsert":%s}`, id, parent, rest)
}

func savedVersion(t *testing.T, res Result) savedOut {
	t.Helper()
	out, ok := res.Content.(savedOut)
	if res.IsError || !ok {
		t.Fatalf("expected a saved version, got %#v", res.Content)
	}
	return out
}

func rejected(t *testing.T, res Result) patchRejectedOut {
	t.Helper()
	out, ok := res.Content.(patchRejectedOut)
	if !res.IsError || !ok || out.Code != codeInvalidSpec {
		t.Fatalf("expected a rejected patch, got %#v", res.Content)
	}
	return out
}

func issueCodesIn(out patchRejectedOut) []spec.IssueCode {
	var codes []spec.IssueCode
	for _, issue := range out.Issues {
		codes = append(codes, issue.Code)
	}
	return codes
}

func TestGetPanels(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)

	t.Run("returns the panels, the variables and what is missing", func(t *testing.T) {
		res, _ := rig.as(rig.ctx).run(t, nameGetPanels, `{"dashboard_id":"`+id+`","ids":["p2","nope","p1","p2"]}`)
		out, ok := res.Content.(panelsOut)
		if res.IsError || !ok {
			t.Fatalf("res = %#v", res.Content)
		}
		var got []string
		for _, p := range out.Panels {
			got = append(got, p.ID)
		}
		if !slices.Equal(got, []string{"p2", "p1"}) || !slices.Equal(out.Missing, []string{"nope"}) || out.Truncated {
			t.Fatalf("panels = %v missing = %v truncated = %v", got, out.Missing, out.Truncated)
		}
		if out.Version != 1 || out.DashboardID != id || len(out.Variables) != 1 || out.Variables[0].Name != "svc" {
			t.Fatalf("out = %+v", out)
		}
		if out.Panels[0].Queries[0].Query != "sum(up)" {
			t.Errorf("panel = %+v", out.Panels[0])
		}
	})
	t.Run("a row comes without its children", func(t *testing.T) {
		res, _ := rig.as(rig.ctx).run(t, nameGetPanels, `{"dashboard_id":"`+id+`","ids":["r1"]}`)
		out := res.Content.(panelsOut)
		if len(out.Panels) != 1 || out.Panels[0].Type != spec.Row || len(out.Panels[0].Panels) != 0 {
			t.Fatalf("panels = %+v", out.Panels)
		}
	})
	t.Run("a nested panel is found", func(t *testing.T) {
		res, _ := rig.as(rig.ctx).run(t, nameGetPanels, `{"dashboard_id":"`+id+`","ids":["p3"]}`)
		if out := res.Content.(panelsOut); len(out.Panels) != 1 || out.Panels[0].ID != "p3" {
			t.Fatalf("panels = %+v", out.Panels)
		}
	})
	t.Run("a run reads its own dashboard by default", func(t *testing.T) {
		res, _ := rig.as(rig.reviseCtx(id, 1)).run(t, nameGetPanels, `{"ids":["p1"]}`)
		if out, ok := res.Content.(panelsOut); !ok || out.DashboardID != id || len(out.Panels) != 1 {
			t.Fatalf("res = %#v", res.Content)
		}
	})
	t.Run("input rules", func(t *testing.T) {
		twentyOne := `["` + strings.Join(strings.Split(strings.Repeat("x", 21), ""), `","`) + `"]`
		for name, input := range map[string]string{
			"no ids":        `{"dashboard_id":"` + id + `"}`,
			"empty ids":     `{"dashboard_id":"` + id + `","ids":[]}`,
			"too many ids":  `{"dashboard_id":"` + id + `","ids":` + twentyOne + `}`,
			"no dashboard":  `{"ids":["p1"]}`,
			"bad dashboard": `{"dashboard_id":"nope","ids":["p1"]}`,
			"bad version":   `{"dashboard_id":"` + id + `","version":0,"ids":["p1"]}`,
		} {
			res, _ := rig.as(rig.ctx).run(t, nameGetPanels, input)
			if got := errorCodeOf(t, res); got != codeInvalidRequest {
				t.Errorf("%s: code = %q", name, got)
			}
		}
	})
	t.Run("an unknown version is not found", func(t *testing.T) {
		res, _ := rig.as(rig.ctx).run(t, nameGetPanels, `{"dashboard_id":"`+id+`","version":9,"ids":["p1"]}`)
		if got := errorCodeOf(t, res); got != codeNotFound {
			t.Errorf("code = %q", got)
		}
	})
}

func TestGetPanelsNeverExceedsItsResultLimit(t *testing.T) {
	rig := newPanelsRig(t)
	big := strings.Repeat("a", 19000)
	id := rig.seed(t, suggestText("t1", big, 0), suggestText("t2", big, 4), suggestText("t3", big, 8), suggestText("t4", big, 12))

	res, _ := rig.as(rig.ctx).run(t, nameGetPanels, `{"dashboard_id":"`+id+`","ids":["t1","t2","t3","t4"]}`)
	out, ok := res.Content.(panelsOut)
	if res.IsError || !ok {
		t.Fatalf("res = %#v", res.Content)
	}
	if !out.Truncated || len(out.Panels) != 3 || !slices.Equal(out.Missing, []string{"t4"}) {
		t.Fatalf("truncated = %v panels = %d missing = %v", out.Truncated, len(out.Panels), out.Missing)
	}
}

func bigQueryPanel(id string, y, queries, queryBytes int) string {
	var list []string
	for range queries {
		list = append(list, fmt.Sprintf(`{"query":%q}`, strings.Repeat("a", queryBytes)))
	}
	return fmt.Sprintf(`{"id":%q,"type":"stat","title":%q,"gridPos":{"x":0,"y":%d,"w":6,"h":4},"datasource":%q,"queries":[%s]}`, id, id, y, fakeUID, strings.Join(list, ","))
}

func TestGetPanelsReturnsAPanelLargerThanTheBudgetAlone(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seed(t, bigQueryPanel("t1", 0, 4, 18000), suggestText("t2", "small", 4))

	res, _ := rig.as(rig.ctx).run(t, nameGetPanels, `{"dashboard_id":"`+id+`","ids":["t1","t2"]}`)
	out, ok := res.Content.(panelsOut)
	if res.IsError || !ok {
		t.Fatalf("res = %#v", res.Content)
	}
	if len(out.Panels) != 1 || out.Panels[0].ID != "t1" || !slices.Equal(out.Missing, []string{"t2"}) || !out.Truncated || len(out.TooLarge) != 0 {
		t.Fatalf("panels = %d missing = %v truncated = %v too_large = %v", len(out.Panels), out.Missing, out.Truncated, out.TooLarge)
	}
}

func TestGetPanelsListsAPanelTooLargeToReturn(t *testing.T) {
	doc := parseBoard(t, bigQueryPanel("huge", 0, 8, 18000), suggestText("t2", "small", 4))

	out, err := readPanels("d", 1, doc, []string{"huge", "t2"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.TooLarge, []string{"huge"}) || out.Truncated || len(out.Missing) != 0 || len(out.Panels) != 1 || out.Panels[0].ID != "t2" {
		t.Fatalf("too_large = %v truncated = %v missing = %v panels = %d", out.TooLarge, out.Truncated, out.Missing, len(out.Panels))
	}
}

func TestAnMCPPatchRejectionHasNoRepairMessage(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)

	res, _ := rig.as(rig.ctx).run(t, nameUpdatePanels, panelPatch(id, 1, `[`+suggestStat("p1", "One", "broken", 0)+`]`))
	if out := rejected(t, res); out.Message != "" {
		t.Fatalf("message = %q, want none for an MCP save", out.Message)
	}
}

func TestARejectedPatchStepNamesTheFirstIssuePath(t *testing.T) {
	issues := []spec.Issue{{Path: "/panels/0/fieldConfig/defaults/unit", Code: spec.CodeSchema, Message: "bad"}}
	for name, res := range map[string]Result{
		"an MCP patch":  patchRejected(issues, nil),
		"a build patch": buildRejected(issues, nil),
	} {
		if want := "Save rejected: 1 issue (schema) at /panels/0/fieldConfig/defaults/unit"; res.Step != want {
			t.Errorf("%s: step = %q, want %q", name, res.Step, want)
		}
	}
	if res := patchRejected([]spec.Issue{{Code: spec.CodeSchema}}, nil); res.Step != "Save rejected: 1 issue (schema)" {
		t.Errorf("an issue without a path: step = %q", res.Step)
	}
}

func TestABuildRejectionTellsTheAgentHowToRepairIt(t *testing.T) {
	issues := []spec.Issue{{Path: "/panels/0/display", Code: spec.CodeSchema, Message: "bad"}}
	out := rejected(t, buildRejected(issues, map[string]string{"/panels/0": "p1"}))
	for _, want := range []string{
		"Fix only the panels in failing_panels and save again.",
		"query_range on a failing query",
		"If an issue names a field you do not know, call get_spec_guide for the JSON Schema.",
	} {
		if !strings.Contains(out.Message, want) {
			t.Errorf("message lacks %q: %s", want, out.Message)
		}
	}
	if mcp := rejected(t, patchRejected(issues, nil)); mcp.Message != "" {
		t.Errorf("an MCP rejection has the message %q", mcp.Message)
	}
}

func TestGetPanelsHidesADashboardTheCallerCannotSee(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)
	foldertest.Move(t, rig.database, mustUUID(t, id), foldertest.Restricted(t, rig.database, "Payroll", "payroll-team"))

	res, _ := rig.as(rig.ctx).run(t, nameGetPanels, `{"dashboard_id":"`+id+`","ids":["p1"]}`)
	if got := errorCodeOf(t, res); got != codeNotFound {
		t.Fatalf("code = %q", got)
	}
	res, _ = rig.as(rig.ctx).run(t, nameUpdatePanels, panelPatch(id, 1, `[`+suggestStat("p1", "Hacked", "count(up)", 0)+`]`))
	if got := errorCodeOf(t, res); got != codeNotFound {
		t.Fatalf("update code = %q", got)
	}
	if rig.latest(t, id).Number != 1 {
		t.Fatal("a hidden dashboard was changed")
	}
}

func TestGetPanelsInSuggestModeReadsTheDraft(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)
	draft, err := spec.Parse([]byte(boardSpecOf(suggestStat("p1", "Draft title", "count(up)", 0), suggestStat("p2", "Two", "sum(up)", 4))))
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithRun(rig.ctx, Run{Mode: ModeSuggest, DashboardID: id, ParentVersion: 1, PanelID: "p1", Base: draft})

	res, _ := rig.as(ctx).run(t, nameGetPanels, `{"ids":["p1"]}`)
	if out := res.Content.(panelsOut); len(out.Panels) != 1 || out.Panels[0].Title != "Draft title" || out.Version != 1 {
		t.Fatalf("draft read = %+v", out)
	}
	res, _ = rig.as(ctx).run(t, nameGetPanels, `{"ids":["p1"],"version":1}`)
	if out := res.Content.(panelsOut); len(out.Panels) != 1 || out.Panels[0].Title != "One" {
		t.Fatalf("stored read = %+v", out)
	}
}

func TestUpdatePanelsSavesOnlyWhatChanged(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)

	res, queries := rig.as(rig.ctx).run(t, nameUpdatePanels, panelPatch(id, 1, `[`+suggestStat("p1", "One renamed", "count(up) by (job)", 0)+`]`))
	out := savedVersion(t, res)
	if out.Version != 2 || out.DashboardID != id || len(out.Panels) != 4 {
		t.Fatalf("saved = %+v", out)
	}
	if !slices.Equal(queries, []string{"count(up) by (job)"}) {
		t.Errorf("queries run = %q", queries)
	}
	saved := rig.latest(t, id).Spec
	if p, _ := saved.FindPanel("p1"); p.Panel.Title != "One renamed" {
		t.Errorf("p1 = %+v", p.Panel)
	}
	if p, _ := saved.FindPanel("p2"); p.Panel.Title != "Two" {
		t.Errorf("p2 = %+v", p.Panel)
	}
}

func TestUpdatePanelsInARunDefaultsTheParent(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)
	input := `{"summary":"Change","upsert":[` + suggestStat("p2", "Two renamed", "sum(up)", 4) + `]}`

	res, queries := rig.as(rig.reviseCtx(id, 1)).run(t, nameUpdatePanels, input)
	if out := savedVersion(t, res); out.Version != 2 {
		t.Fatalf("saved = %+v", out)
	}
	if len(queries) != 0 {
		t.Errorf("an unchanged query ran: %q", queries)
	}
}

func TestUpdatePanelsRunScopeAndConflict(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)
	other := rig.seedStats(t)
	change := `"summary":"Change","upsert":[` + suggestStat("p1", "Renamed", "count(up)", 0) + `]`

	res, _ := rig.as(rig.reviseCtx(id, 1)).run(t, nameUpdatePanels, `{"dashboard_id":"`+other+`",`+change+`}`)
	if got := errorCodeOf(t, res); got != codeInvalidRequest || !strings.Contains(res.Content.(errorBody).Message, id) {
		t.Fatalf("wrong dashboard: %#v", res.Content)
	}
	res, _ = rig.as(WithRun(rig.ctx, Run{Mode: ModeCreate})).run(t, nameUpdatePanels, `{`+change+`}`)
	if got := errorCodeOf(t, res); got != codeInvalidRequest || !strings.Contains(res.Content.(errorBody).Message, "call create_dashboard") {
		t.Fatalf("create mode: %#v", res.Content)
	}
	res, _ = rig.as(rig.ctx).run(t, nameUpdatePanels, `{"dashboard_id":"`+id+`",`+change+`}`)
	if got := errorCodeOf(t, res); got != codeInvalidRequest {
		t.Fatalf("outside a run, no parent_version: code = %q", got)
	}

	if out := savedVersion(t, firstOf(rig.as(rig.ctx).run(t, nameUpdatePanels, `{"dashboard_id":"`+id+`","parent_version":1,`+change+`}`))); out.Version != 2 {
		t.Fatalf("saved = %+v", out)
	}
	stale := `"summary":"Change","upsert":[` + suggestStat("p2", "Two renamed", "sum(up)", 4) + `]`
	res, _ = rig.as(rig.ctx).run(t, nameUpdatePanels, `{"dashboard_id":"`+id+`","parent_version":1,`+stale+`}`)
	conflict, ok := res.Content.(conflictOut)
	if !res.IsError || !ok || conflict.Code != codeVersionConflict || conflict.LatestVersion != 2 {
		t.Fatalf("stale parent: %#v", res.Content)
	}
}

func firstOf(res Result, _ []string) Result { return res }

func TestUpdatePanelsRejectionNamesTheFailingPanels(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)

	t.Run("a failing query", func(t *testing.T) {
		res, queries := rig.as(rig.ctx).run(t, nameUpdatePanels, panelPatch(id, 1, `[`+suggestStat("p1", "One", "broken", 0)+`]`))
		out := rejected(t, res)
		if !slices.Equal(out.FailingPanels, []string{"p1"}) || !slices.Equal(issueCodesIn(out), []spec.IssueCode{queryproxy.CodeQueryInvalid}) {
			t.Fatalf("out = %+v", out)
		}
		if !slices.Equal(queries, []string{"broken"}) {
			t.Errorf("queries = %q", queries)
		}
	})
	t.Run("a failing query in a row child", func(t *testing.T) {
		res, _ := rig.as(rig.ctx).run(t, nameUpdatePanels, panelPatch(id, 1, `[`+suggestStat("p3", "Three", "broken", 9)+`]`))
		if out := rejected(t, res); !slices.Equal(out.FailingPanels, []string{"p3"}) {
			t.Fatalf("out = %+v", out)
		}
	})
	t.Run("an overlap", func(t *testing.T) {
		res, _ := rig.as(rig.ctx).run(t, nameUpdatePanels, panelPatch(id, 1, `[`+suggestStat("p9", "Nine", "count(up)", 4)+`]`))
		out := rejected(t, res)
		if !slices.Contains(issueCodesIn(out), spec.CodeGridOverlap) || len(out.FailingPanels) != 1 || !slices.Contains([]string{"p2", "p9"}, out.FailingPanels[0]) {
			t.Fatalf("out = %+v", out)
		}
	})
	t.Run("an empty patch", func(t *testing.T) {
		res, _ := rig.as(rig.ctx).run(t, nameUpdatePanels, `{"dashboard_id":"`+id+`","parent_version":1,"summary":"Nothing","dashboard":{}}`)
		if out := rejected(t, res); out.FailingPanels == nil || len(out.FailingPanels) != 0 {
			t.Fatalf("out = %+v", out)
		}
	})
	if rig.latest(t, id).Number != 1 {
		t.Fatal("a rejected patch saved a version")
	}
}

func TestUpdatePanelsDashboardFields(t *testing.T) {
	t.Run("tags and title run no query", func(t *testing.T) {
		rig := newPanelsRig(t)
		id := rig.seedStats(t)
		res, queries := rig.as(rig.ctx).run(t, nameUpdatePanels, `{"dashboard_id":"`+id+`","parent_version":1,"summary":"Retag","dashboard":{"title":"Renamed","tags":["linux","ops"]}}`)
		savedVersion(t, res)
		if len(queries) != 0 {
			t.Errorf("queries run = %q", queries)
		}
		saved := rig.latest(t, id).Spec
		if saved.Title != "Renamed" || !slices.Equal(saved.Tags, []string{"linux", "ops"}) || saved.Time.From != "now-15m" {
			t.Errorf("saved = %+v", saved)
		}
	})
	t.Run("a different time range runs every query", func(t *testing.T) {
		rig := newPanelsRig(t)
		id := rig.seedStats(t)
		res, queries := rig.as(rig.ctx).run(t, nameUpdatePanels, `{"dashboard_id":"`+id+`","parent_version":1,"summary":"Six hours","dashboard":{"time":{"from":"now-6h","to":"now"}}}`)
		savedVersion(t, res)
		if want := []string{"count(up)", "max(up)", "sum(up)"}; !slices.Equal(queries, want) {
			t.Errorf("queries run = %q, want %q", queries, want)
		}
		if got := rig.latest(t, id).Spec.Time; got.From != "now-6h" || got.To != "now" {
			t.Errorf("time = %+v", got)
		}
	})
	t.Run("the same time range runs none", func(t *testing.T) {
		rig := newPanelsRig(t)
		id := rig.seedStats(t)
		res, queries := rig.as(rig.ctx).run(t, nameUpdatePanels, `{"dashboard_id":"`+id+`","parent_version":1,"summary":"Same","dashboard":{"time":{"from":"now-15m","to":"now"},"title":"Other"}}`)
		savedVersion(t, res)
		if len(queries) != 0 {
			t.Errorf("queries run = %q", queries)
		}
	})
	t.Run("an unknown key is invalid input", func(t *testing.T) {
		rig := newPanelsRig(t)
		id := rig.seedStats(t)
		res, _ := rig.as(rig.ctx).run(t, nameUpdatePanels, `{"dashboard_id":"`+id+`","parent_version":1,"summary":"Refresh","dashboard":{"refresh":"5s"}}`)
		if got := errorCodeOf(t, res); got != codeInvalidRequest || !strings.Contains(res.Content.(errorBody).Message, "refresh") {
			t.Fatalf("res = %#v", res.Content)
		}
	})
	t.Run("a bad time is rejected with its path and no panel", func(t *testing.T) {
		rig := newPanelsRig(t)
		id := rig.seedStats(t)
		res, _ := rig.as(rig.ctx).run(t, nameUpdatePanels, `{"dashboard_id":"`+id+`","parent_version":1,"summary":"Bad","dashboard":{"time":{"from":"yesterday","to":"now"}}}`)
		out := rejected(t, res)
		if len(out.Issues) == 0 || out.Issues[0].Path != "/time/from" || out.FailingPanels == nil || len(out.FailingPanels) != 0 {
			t.Fatalf("out = %+v", out)
		}
	})
	t.Run("a time range that leaves a verified panel empty is rejected", func(t *testing.T) {
		rig := newPanelsRig(t)
		id := rig.seed(t, suggestStat("p1", "One", "short(up)", 0), suggestStat("p2", "Two", "sum(up)", 4))
		res, _ := rig.as(rig.ctx).run(t, nameUpdatePanels, `{"dashboard_id":"`+id+`","parent_version":1,"summary":"Six hours","dashboard":{"time":{"from":"now-6h","to":"now"}}}`)
		out := rejected(t, res)
		if !slices.Equal(out.FailingPanels, []string{"p1"}) || !slices.Equal(issueCodesIn(out), []spec.IssueCode{queryproxy.CodeQueryEmpty}) {
			t.Fatalf("out = %+v", out)
		}
		if rig.latest(t, id).Number != 1 {
			t.Fatal("a rejected patch saved a version")
		}
	})
}

func TestUpdatePanelsKeepsTheImportOnlyRules(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seed(t,
		statPanel("p1", 0),
		canvasPanelJSON("c_map", "Deploy", deployActions),
		placeholderPanel("ph", 12, 0, "no worldmap"),
	)
	cases := []struct {
		name   string
		upsert string
		code   spec.IssueCode
		panel  string
	}{
		{"a placeholder edit", placeholderPanel("ph", 12, 0, "changed"), codePlaceholderNotAllowed, "ph"},
		{"added HTML", suggestText("t1", "<b>bold</b>", 4), codeHTMLNotAllowed, "t1"},
		{"a changed action", canvasPanelJSON("c_map", "Deploy", otherActions), codeActionNotAllowed, "c_map"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, _ := rig.as(rig.ctx).run(t, nameUpdatePanels, panelPatch(id, 1, `[`+tc.upsert+`]`))
			out := rejected(t, res)
			if !slices.Equal(issueCodesIn(out), []spec.IssueCode{tc.code}) || !slices.Equal(out.FailingPanels, []string{tc.panel}) {
				t.Fatalf("out = %+v", out)
			}
		})
	}
	if rig.latest(t, id).Number != 1 {
		t.Fatal("a rejected patch saved a version")
	}
}

func TestUpdatePanelsInSuggestMode(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)
	base := rig.latest(t, id).Spec
	ctx := WithRun(rig.ctx, Run{Mode: ModeSuggest, DashboardID: id, ParentVersion: 1, PanelID: "p1", Base: base})
	one := suggestStat("p1", "One suggested", "count(up) by (job)", 0)

	t.Run("one upsert of the target panel is proposed and nothing is saved", func(t *testing.T) {
		res, queries := rig.as(ctx).run(t, nameUpdatePanels, `{"summary":"Suggest","upsert":[`+one+`]}`)
		out, ok := res.Content.(suggestedOut)
		if res.IsError || !ok || !out.Suggested || out.PanelID != "p1" {
			t.Fatalf("res = %#v", res.Content)
		}
		var panel spec.Panel
		if err := json.Unmarshal(out.Panel, &panel); err != nil || panel.Title != "One suggested" {
			t.Errorf("panel = %s (%v)", out.Panel, err)
		}
		if !slices.Equal(queries, []string{"count(up) by (job)"}) {
			t.Errorf("queries = %q", queries)
		}
		if rig.latest(t, id).Number != 1 {
			t.Error("suggest mode saved a version")
		}
	})
	t.Run("an upsert without gridPos keeps the panel's position", func(t *testing.T) {
		bare := fmt.Sprintf(`{"id":"p1","type":"stat","title":"One moved on","datasource":%q,"queries":[{"query":"count(up)"}]}`, fakeUID)
		res, _ := rig.as(ctx).run(t, nameUpdatePanels, `{"summary":"Suggest","upsert":[`+bare+`]}`)
		out, ok := res.Content.(suggestedOut)
		if res.IsError || !ok {
			t.Fatalf("res = %#v", res.Content)
		}
		var panel spec.Panel
		if err := json.Unmarshal(out.Panel, &panel); err != nil {
			t.Fatal(err)
		}
		if panel.Title != "One moved on" || panel.GridPos != (spec.GridPos{X: 0, Y: 0, W: 6, H: 4}) {
			t.Errorf("panel = %s", out.Panel)
		}
	})
	t.Run("a failing suggestion is rejected with its issues", func(t *testing.T) {
		res, _ := rig.as(ctx).run(t, nameUpdatePanels, `{"summary":"Suggest","upsert":[`+suggestStat("p1", "One", "broken", 0)+`]}`)
		if body, ok := res.Content.(invalidSpecOut); !res.IsError || !ok || len(body.Issues) == 0 {
			t.Fatalf("res = %#v", res.Content)
		}
	})
	for name, input := range map[string]string{
		"another panel":      `{"summary":"s","upsert":[` + suggestStat("p2", "Two", "sum(up)", 4) + `]}`,
		"two upserts":        `{"summary":"s","upsert":[` + one + `,` + suggestStat("p2", "Two", "sum(up)", 4) + `]}`,
		"a delete":           `{"summary":"s","upsert":[` + one + `],"delete":["p2"]}`,
		"a layout":           `{"summary":"s","upsert":[` + one + `],"layout":[{"id":"p2","gridPos":{"x":6,"y":0,"w":6,"h":4}}]}`,
		"variables":          `{"summary":"s","upsert":[` + one + `],"variables":[]}`,
		"dashboard fields":   `{"summary":"s","upsert":[` + one + `],"dashboard":{"title":"x"}}`,
		"an empty dashboard": `{"summary":"s","upsert":[` + one + `],"dashboard":{}}`,
		"dashboard only":     `{"summary":"s","dashboard":{"title":"x"}}`,
		"a row":              `{"summary":"s","upsert":[` + rowPanel("p1", 12) + `]}`,
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			res, queries := rig.as(ctx).run(t, nameUpdatePanels, input)
			if got := errorCodeOf(t, res); got != codeInvalidRequest {
				t.Fatalf("code = %q (%#v)", got, res.Content)
			}
			if len(queries) != 0 {
				t.Errorf("queries = %q", queries)
			}
		})
	}
}

func TestUpdatePanelsInputRules(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)
	panel := suggestStat("p1", "One", "count(up)", 0)
	many := func(n int) string { return "[" + strings.TrimSuffix(strings.Repeat(panel+",", n), ",") + "]" }
	head := `{"dashboard_id":"` + id + `","parent_version":1,`
	for name, input := range map[string]string{
		"no summary":      head + `"upsert":` + many(1) + `}`,
		"a blank summary": head + `"summary":"  ","upsert":` + many(1) + `}`,
		"51 upserts":      head + `"summary":"s","upsert":` + many(51) + `}`,
		"an unknown key":  head + `"summary":"s","upsert":` + many(1) + `,"replace":[]}`,
		"a wrong type":    head + `"summary":"s","upsert":"p1"}`,
	} {
		res, _ := rig.as(rig.ctx).run(t, nameUpdatePanels, input)
		if got := errorCodeOf(t, res); got != codeInvalidRequest {
			t.Errorf("%s: code = %q", name, got)
		}
	}
}

func TestPanelToolRoleFloors(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)
	viewer := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindServiceAccount, ID: dashB, Name: "viewer-key", Role: auth.RoleViewer})

	res, _ := rig.as(viewer).run(t, nameUpdatePanels, panelPatch(id, 1, `[`+suggestStat("p1", "Hacked", "count(up)", 0)+`]`))
	if got := errorCodeOf(t, res); got != codeForbiddenRole {
		t.Fatalf("viewer update_panels: code = %q", got)
	}
	res, _ = rig.as(viewer).run(t, nameGetPanels, `{"dashboard_id":"`+id+`","ids":["p1"]}`)
	if _, ok := res.Content.(panelsOut); res.IsError || !ok {
		t.Fatalf("viewer get_panels: %#v", res.Content)
	}
}

func TestOwningPanelIDPicksTheDeepestPanel(t *testing.T) {
	ids := map[string]string{"/panels/1": "row", "/panels/1/panels/0": "child", "/panels/10": "other"}
	for path, want := range map[string]string{
		"/panels/1":                         "row",
		"/panels/1/title":                   "row",
		"/panels/1/panels/0/queries/0":      "child",
		"/panels/10/gridPos":                "other",
		"/panels/2":                         "",
		"/time/from":                        "",
		"/panels/1/panels/0":                "child",
		"/panels/1/panels/1/queries/0/expr": "row",
	} {
		got, ok := owningPanelID(path, ids)
		if got != want || ok != (want != "") {
			t.Errorf("owningPanelID(%q) = %q, %v; want %q", path, got, ok, want)
		}
	}
}
