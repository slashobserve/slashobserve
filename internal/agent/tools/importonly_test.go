package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

var placeholderUser = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Email: "ada@example.com", Role: auth.RoleEditor}

type placeholderRig struct {
	registry *Registry
	service  *dashboards.Service
	ctx      context.Context
}

func newPlaceholderRig(t *testing.T) *placeholderRig {
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
	service := dashboards.New(database.DB, runner, folders.NewAccess(database.DB), logger)
	registry := New(Deps{Datasources: datasources, Runner: runner, Dashboards: service, Logger: logger})
	return &placeholderRig{registry: registry, service: service, ctx: auth.WithPrincipal(context.Background(), placeholderUser)}
}

func statPanel(id string, y int) string {
	return fmt.Sprintf(`{"id":%q,"type":"stat","title":"Up","gridPos":{"x":0,"y":%d,"w":6,"h":4},"datasource":%q,"queries":[{"query":"count(up)"}]}`, id, y, fakeUID)
}

func placeholderPanel(id string, x, y int, reason string) string {
	return fmt.Sprintf(`{"id":%q,"type":"placeholder","title":"Map","description":"worldmap","gridPos":{"x":%d,"y":%d,"w":6,"h":4},"reason":%q,"sourceType":"worldmap"}`, id, x, y, reason)
}

func rowPanel(id string, y int, children ...string) string {
	return fmt.Sprintf(`{"id":%q,"type":"row","title":"Row","gridPos":{"x":0,"y":%d,"w":24,"h":1},"panels":[%s]}`, id, y, strings.Join(children, ","))
}

func specOf(panels ...string) string {
	return fmt.Sprintf(`{"specVersion":"0","title":"Placeholder check","time":{"from":"now-15m","to":"now"},"panels":[%s]}`, strings.Join(panels, ","))
}

func (r *placeholderRig) seed(t *testing.T, panels ...string) string {
	t.Helper()
	dash, _, err := r.service.Create(r.ctx, placeholderUser, pgtype.UUID{}, []byte(specOf(panels...)), "import")
	if err != nil {
		t.Fatal(err)
	}
	return dash.ID
}

func (r *placeholderRig) create(t *testing.T, spec string) Result {
	t.Helper()
	res, err := r.registry.Call(r.ctx, nameCreateDashboard, []byte(fmt.Sprintf(`{"spec":%s}`, spec)))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (r *placeholderRig) update(t *testing.T, id, spec string) Result {
	t.Helper()
	res, err := r.registry.Call(r.ctx, nameUpdateDashboard, []byte(fmt.Sprintf(`{"dashboard_id":%q,"parent_version":1,"spec":%s}`, id, spec)))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func issueCodesOf(t *testing.T, res Result) []string {
	t.Helper()
	body, ok := res.Content.(invalidSpecOut)
	if !res.IsError || !ok || body.Code != codeInvalidSpec {
		t.Fatalf("res = %#v", res)
	}
	var codes []string
	for _, issue := range body.Issues {
		codes = append(codes, string(issue.Code))
	}
	return codes
}

func requireSaved(t *testing.T, res Result, version int) {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected a saved version, got %#v", res.Content)
	}
	if out, ok := res.Content.(savedOut); !ok || out.Version != version {
		t.Fatalf("res = %#v", res.Content)
	}
}

func TestCreateRejectsPlaceholderPanels(t *testing.T) {
	r := newPlaceholderRig(t)
	res := r.create(t, specOf(statPanel("p_up", 0), placeholderPanel("p_x", 6, 0, "not supported")))
	if codes := issueCodesOf(t, res); len(codes) != 1 || codes[0] != string(codePlaceholderNotAllowed) {
		t.Fatalf("codes = %v", codes)
	}
	if res.Step != "Save rejected: 1 issue (placeholder_not_allowed)" {
		t.Fatalf("step = %q", res.Step)
	}
	issue := res.Content.(invalidSpecOut).Issues[0]
	if issue.Path != "/panels/1" || !strings.Contains(issue.Message, "cannot create placeholder panels") {
		t.Fatalf("issue = %+v", issue)
	}
	page, err := r.service.List(r.ctx, placeholderUser, dashboards.ListParams{Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("saved dashboards = %v err = %v", page.Items, err)
	}
}

func TestCreateRejectsPlaceholderInsideRow(t *testing.T) {
	r := newPlaceholderRig(t)
	res := r.create(t, specOf(rowPanel("r_a", 0, placeholderPanel("p_x", 0, 1, "not supported"))))
	if issue := res.Content.(invalidSpecOut).Issues[0]; res.Step != "Save rejected: 1 issue (placeholder_not_allowed)" || issue.Path != "/panels/0/panels/0" {
		t.Fatalf("res = %#v", res)
	}
}

func TestCreateKeepsNormalIssuesForUnparsableSpecs(t *testing.T) {
	r := newPlaceholderRig(t)
	broken := strings.Replace(specOf(placeholderPanel("p_x", 0, 0, "not supported")), `"title":"Placeholder check"`, `"title":"Placeholder check","bogus":1`, 1)
	codes := issueCodesOf(t, r.create(t, broken))
	if len(codes) == 0 {
		t.Fatal("expected issues")
	}
	for _, code := range codes {
		if code == string(codePlaceholderNotAllowed) {
			t.Fatalf("codes = %v", codes)
		}
	}
}

func TestUpdateMovesPlaceholdersButNeverEditsOrAddsThem(t *testing.T) {
	const reason = "Grafana panel type worldmap is not supported yet"
	original := func(r *placeholderRig, t *testing.T) string {
		return r.seed(t, statPanel("p_up", 0), placeholderPanel("p_x", 6, 0, reason))
	}

	t.Run("move within the grid", func(t *testing.T) {
		r := newPlaceholderRig(t)
		id := original(r, t)
		requireSaved(t, r.update(t, id, specOf(statPanel("p_up", 0), placeholderPanel("p_x", 12, 4, reason))), 2)
	})
	t.Run("move into a row", func(t *testing.T) {
		r := newPlaceholderRig(t)
		id := original(r, t)
		requireSaved(t, r.update(t, id, specOf(statPanel("p_up", 0), rowPanel("r_a", 4, placeholderPanel("p_x", 0, 5, reason)))), 2)
	})
	t.Run("delete", func(t *testing.T) {
		r := newPlaceholderRig(t)
		id := original(r, t)
		requireSaved(t, r.update(t, id, specOf(statPanel("p_up", 0))), 2)
	})
	t.Run("move out of a row", func(t *testing.T) {
		r := newPlaceholderRig(t)
		id := r.seed(t, statPanel("p_up", 0), rowPanel("r_a", 4, placeholderPanel("p_x", 0, 5, reason)))
		requireSaved(t, r.update(t, id, specOf(statPanel("p_up", 0), rowPanel("r_a", 4), placeholderPanel("p_x", 6, 0, reason))), 2)
	})
	t.Run("replace with a real panel under the same id", func(t *testing.T) {
		r := newPlaceholderRig(t)
		id := original(r, t)
		requireSaved(t, r.update(t, id, specOf(statPanel("p_up", 0), statPanel("p_x", 4))), 2)
	})
	t.Run("edit the reason", func(t *testing.T) {
		r := newPlaceholderRig(t)
		id := original(r, t)
		res := r.update(t, id, specOf(statPanel("p_up", 0), placeholderPanel("p_x", 6, 0, "something else")))
		if codes := issueCodesOf(t, res); len(codes) != 1 || codes[0] != string(codePlaceholderNotAllowed) {
			t.Fatalf("codes = %v", codes)
		}
		if message := res.Content.(invalidSpecOut).Issues[0].Message; !strings.Contains(message, `placeholder "p_x" can only be moved or deleted`) {
			t.Fatalf("message = %q", message)
		}
	})
	t.Run("add a second placeholder", func(t *testing.T) {
		r := newPlaceholderRig(t)
		id := original(r, t)
		res := r.update(t, id, specOf(statPanel("p_up", 0), placeholderPanel("p_x", 6, 0, reason), placeholderPanel("p_y", 12, 0, reason)))
		if codes := issueCodesOf(t, res); len(codes) != 1 || codes[0] != string(codePlaceholderNotAllowed) {
			t.Fatalf("codes = %v", codes)
		}
		if path := res.Content.(invalidSpecOut).Issues[0].Path; path != "/panels/2" {
			t.Fatalf("path = %q", path)
		}
	})
	t.Run("an unparsable spec reports the normal issues", func(t *testing.T) {
		r := newPlaceholderRig(t)
		id := original(r, t)
		broken := strings.Replace(specOf(placeholderPanel("p_x", 6, 0, "other")), `"title":"Placeholder check"`, `"title":"Placeholder check","bogus":1`, 1)
		for _, code := range issueCodesOf(t, r.update(t, id, broken)) {
			if code == string(codePlaceholderNotAllowed) {
				t.Fatal("the guard must leave unparsable specs to the save")
			}
		}
	})
	t.Run("a missing parent is left to the save", func(t *testing.T) {
		r := newPlaceholderRig(t)
		id := original(r, t)
		res, err := r.registry.Call(r.ctx, nameUpdateDashboard, []byte(fmt.Sprintf(`{"dashboard_id":%q,"parent_version":9,"spec":%s}`, id, specOf(placeholderPanel("p_x", 6, 0, reason)))))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError || strings.Contains(res.Step, string(codePlaceholderNotAllowed)) {
			t.Fatalf("res = %#v", res)
		}
	})
}

func textPanelJSON(id, content, extra string, y int) string {
	return fmt.Sprintf(`{"id":%q,"type":"text","title":"Notes","gridPos":{"x":0,"y":%d,"w":6,"h":4},"content":%q%s}`, id, y, content, extra)
}

func canvasPanelJSON(id, label, actions string) string {
	return fmt.Sprintf(`{"id":%q,"type":"canvas","title":"Diagram","gridPos":{"x":6,"y":0,"w":6,"h":4},"datasource":%q,"queries":[{"query":"count(up)"}],"display":{"elements":[{"id":"btn","kind":"button","left":10,"top":10,"width":20,"height":20,"text":%q%s}]}}`, id, fakeUID, label, actions)
}

const (
	deployActions = `,"actions":[{"title":"Deploy","method":"POST","url":"https://deploy.example.com/api"}]`
	otherActions  = `,"actions":[{"title":"Deploy","method":"DELETE","url":"https://deploy.example.com/api"}]`
	importedHTML  = `,"mode":"html"`
)

func mustParse(t *testing.T, panels ...string) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse([]byte(specOf(panels...)))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestContainsHTML(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"plain sentence", false},
		{"**bold** and [a link](https://example.com)", false},
		{"see <https://example.com/path> for more", false},
		{"mail <someone@example.com>", false},
		{"1 < 2 and 3 > 2", false},
		{"a <- b", false},
		{"<div>", true},
		{"text <b>bold</b> text", true},
		{"<br/>", true},
		{"<br />", true},
		{"<my-element>", true},
		{"</div>", true},
		{"<!-- hidden -->", true},
		{"<!DOCTYPE html>", true},
		{"<![CDATA[x]]>", true},
		{"<?xml version=\"1.0\"?>", true},
		{"ends with <b", true},
		{"<script src=x>", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := containsHTML(tc.in); got != tc.want {
				t.Fatalf("containsHTML(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestImportOnlyIssuesForHTMLAndActions(t *testing.T) {
	imported := mustParse(t, textPanelJSON("t_note", "<p>Runbook</p>", importedHTML, 0), canvasPanelJSON("c_map", "Deploy", deployActions))
	plain := mustParse(t, textPanelJSON("t_note", "Runbook", "", 0), canvasPanelJSON("c_map", "Deploy", ""))
	movedCanvas := func(actions string) string {
		return strings.Replace(canvasPanelJSON("c_map", "Deploy", actions), `"x":6,"y":0`, `"x":12,"y":4`, 1)
	}
	emptyCanvas := `{"id":"c_map","type":"canvas","title":"Diagram","gridPos":{"x":6,"y":0,"w":6,"h":4},"datasource":"` + fakeUID + `","queries":[{"query":"count(up)"}],"display":{"elements":[]}}`
	renamedElement := strings.Replace(canvasPanelJSON("c_map", "Deploy", deployActions), `"id":"btn"`, `"id":"btn2"`, 1)
	actionsPath := "/panels/0/display/elements/0/actions"
	htmlCode, actionCode := []spec.IssueCode{codeHTMLNotAllowed}, []spec.IssueCode{codeActionNotAllowed}

	cases := []struct {
		name   string
		next   *spec.Dashboard
		parent *spec.Dashboard
		want   []spec.IssueCode
		paths  []string
	}{
		{"create an html panel", mustParse(t, textPanelJSON("t_new", "hello", importedHTML, 0)), nil, htmlCode, []string{"/panels/0"}},
		{"create Markdown with a tag", mustParse(t, textPanelJSON("t_new", "see <div>x</div>", "", 0)), nil, htmlCode, []string{"/panels/0"}},
		{"create a code panel holding a tag", mustParse(t, textPanelJSON("t_new", "<div>", `,"mode":"code"`, 0)), nil, htmlCode, []string{"/panels/0"}},
		{"create Markdown with an autolink", mustParse(t, textPanelJSON("t_new", "see <https://example.com>", "", 0)), nil, nil, nil},
		{"create plain Markdown", mustParse(t, textPanelJSON("t_new", "# Title\n\n- item", "", 0)), nil, nil, nil},
		{"keep an imported html panel but move it", mustParse(t, textPanelJSON("t_note", "<p>Runbook</p>", importedHTML, 8)), imported, nil, nil},
		{"edit the content of an imported html panel", mustParse(t, textPanelJSON("t_note", "<p>Changed</p>", importedHTML, 0)), imported, htmlCode, []string{"/panels/0"}},
		{"switch a plain panel to html", mustParse(t, textPanelJSON("t_note", "Runbook", importedHTML, 0)), plain, htmlCode, []string{"/panels/0"}},
		{"change the code options of an imported html panel", mustParse(t, textPanelJSON("t_note", "<p>Runbook</p>", importedHTML+`,"code":{"language":"html"}`, 0)), imported, htmlCode, []string{"/panels/0"}},
		{"replace an imported html panel with plain text", mustParse(t, textPanelJSON("t_note", "Runbook", "", 0)), imported, nil, nil},
		{"delete an imported html panel", mustParse(t, statPanel("p_up", 0)), imported, nil, nil},
		{"reuse the id of a stat panel for html", mustParse(t, textPanelJSON("p_up", "<p>x</p>", importedHTML, 0)), mustParse(t, statPanel("p_up", 0)), htmlCode, []string{"/panels/0"}},
		{"create a canvas action", mustParse(t, canvasPanelJSON("c_new", "Deploy", deployActions)), nil, actionCode, []string{actionsPath}},
		{"add an action to an element", mustParse(t, canvasPanelJSON("c_map", "Deploy", deployActions)), plain, actionCode, []string{actionsPath}},
		{"change an action", mustParse(t, canvasPanelJSON("c_map", "Deploy", otherActions)), imported, actionCode, []string{actionsPath}},
		{"keep the action and change the element text", mustParse(t, canvasPanelJSON("c_map", "Release", deployActions)), imported, nil, nil},
		{"remove the actions", mustParse(t, canvasPanelJSON("c_map", "Deploy", "")), imported, nil, nil},
		{"remove the element", mustParse(t, emptyCanvas), imported, nil, nil},
		{"move a canvas that keeps its action", mustParse(t, movedCanvas(deployActions)), imported, nil, nil},
		{"carry an action to a new element id", mustParse(t, renamedElement), imported, actionCode, []string{actionsPath}},
		{"keep a canvas action inside a row", mustParse(t, rowPanel("r_a", 0, strings.Replace(canvasPanelJSON("c_map", "Deploy", deployActions), `"x":6,"y":0`, `"x":0,"y":1`, 1))), imported, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := importOnlyIssues(tc.next, tc.parent)
			var codes []spec.IssueCode
			var paths []string
			for _, issue := range issues {
				codes = append(codes, issue.Code)
				paths = append(paths, issue.Path)
			}
			if !slices.Equal(codes, tc.want) || !slices.Equal(paths, tc.paths) {
				t.Fatalf("issues = %+v, want codes %v at %v", issues, tc.want, tc.paths)
			}
		})
	}
}

func TestImportOnlyMessagesTellTheAgentWhatToDo(t *testing.T) {
	html := importOnlyIssues(mustParse(t, textPanelJSON("t_new", "<div>", "", 0)), nil)
	if len(html) != 1 || html[0].Message != "the agent cannot write HTML; write Markdown without tags, or leave this imported panel unchanged" {
		t.Fatalf("html issues = %+v", html)
	}
	action := importOnlyIssues(mustParse(t, canvasPanelJSON("c_new", "Deploy", deployActions)), nil)
	if len(action) != 1 || action[0].Message != "the agent cannot add or change canvas actions; keep them as they are or remove the element" {
		t.Fatalf("action issues = %+v", action)
	}
}

func TestNeedsImportOnlyCheck(t *testing.T) {
	cases := []struct {
		name string
		d    *spec.Dashboard
		want bool
	}{
		{"stat and plain text", mustParse(t, statPanel("p_up", 0), textPanelJSON("t_note", "hello", "", 4)), false},
		{"autolink in text", mustParse(t, textPanelJSON("t_note", "<https://example.com>", "", 0)), false},
		{"placeholder", mustParse(t, placeholderPanel("p_x", 0, 0, "unsupported")), true},
		{"html mode", mustParse(t, textPanelJSON("t_note", "hello", importedHTML, 0)), true},
		{"tag in text", mustParse(t, textPanelJSON("t_note", "<b>x</b>", "", 0)), true},
		{"canvas without actions", mustParse(t, canvasPanelJSON("c_map", "Deploy", "")), false},
		{"canvas with an action", mustParse(t, canvasPanelJSON("c_map", "Deploy", deployActions)), true},
		{"html inside a row", mustParse(t, rowPanel("r_a", 0, textPanelJSON("t_note", "<i>x</i>", "", 1))), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsImportOnlyCheck(tc.d); got != tc.want {
				t.Fatalf("needsImportOnlyCheck = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCreateRejectsHTMLAndActions(t *testing.T) {
	r := newPlaceholderRig(t)
	for name, panel := range map[string]string{
		"html panel":        textPanelJSON("t_new", "hello", importedHTML, 0),
		"tag in Markdown":   textPanelJSON("t_new", "<script>x</script>", "", 0),
		"canvas action":     canvasPanelJSON("c_new", "Deploy", deployActions),
		"html inside a row": rowPanel("r_a", 0, textPanelJSON("t_new", "<b>x</b>", "", 1)),
	} {
		t.Run(name, func(t *testing.T) {
			codes := issueCodesOf(t, r.create(t, specOf(panel)))
			if len(codes) != 1 || (codes[0] != string(codeHTMLNotAllowed) && codes[0] != string(codeActionNotAllowed)) {
				t.Fatalf("codes = %v", codes)
			}
		})
	}
	requireSaved(t, r.create(t, specOf(textPanelJSON("t_new", "see <https://example.com>", "", 0))), 1)
}

func TestUpdateKeepsImportedHTMLAndActionsButNeverWritesThem(t *testing.T) {
	seed := func(t *testing.T) (*placeholderRig, string) {
		r := newPlaceholderRig(t)
		return r, r.seed(t, textPanelJSON("t_note", "<p>Runbook</p>", importedHTML, 0), canvasPanelJSON("c_map", "Deploy", deployActions))
	}
	t.Run("move both and change a label", func(t *testing.T) {
		r, id := seed(t)
		moved := strings.Replace(canvasPanelJSON("c_map", "Release", deployActions), `"x":6,"y":0`, `"x":12,"y":0`, 1)
		requireSaved(t, r.update(t, id, specOf(textPanelJSON("t_note", "<p>Runbook</p>", importedHTML, 4), moved)), 2)
	})
	t.Run("delete both", func(t *testing.T) {
		r, id := seed(t)
		requireSaved(t, r.update(t, id, specOf(statPanel("p_up", 0))), 2)
	})
	t.Run("edit the html", func(t *testing.T) {
		r, id := seed(t)
		res := r.update(t, id, specOf(textPanelJSON("t_note", "<p>Edited</p>", importedHTML, 0), canvasPanelJSON("c_map", "Deploy", deployActions)))
		if codes := issueCodesOf(t, res); len(codes) != 1 || codes[0] != string(codeHTMLNotAllowed) {
			t.Fatalf("codes = %v", codes)
		}
	})
	t.Run("change the action", func(t *testing.T) {
		r, id := seed(t)
		res := r.update(t, id, specOf(textPanelJSON("t_note", "<p>Runbook</p>", importedHTML, 0), canvasPanelJSON("c_map", "Deploy", otherActions)))
		if codes := issueCodesOf(t, res); len(codes) != 1 || codes[0] != string(codeActionNotAllowed) {
			t.Fatalf("codes = %v", codes)
		}
	})
	t.Run("add html to a plain dashboard", func(t *testing.T) {
		r := newPlaceholderRig(t)
		id := r.seed(t, statPanel("p_up", 0))
		res := r.update(t, id, specOf(statPanel("p_up", 0), textPanelJSON("t_new", "<b>x</b>", "", 4)))
		if codes := issueCodesOf(t, res); len(codes) != 1 || codes[0] != string(codeHTMLNotAllowed) {
			t.Fatalf("codes = %v", codes)
		}
	})
}
