package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

var placeholderUser = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Email: "ada@example.com"}

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
	service := dashboards.New(database.DB, runner, logger)
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
	dash, _, err := r.service.Create(r.ctx, placeholderUser, []byte(specOf(panels...)), "import")
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
	list, _, err := r.service.List(r.ctx, 10, "")
	if err != nil || len(list) != 0 {
		t.Fatalf("saved dashboards = %v err = %v", list, err)
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
