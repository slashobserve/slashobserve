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
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func suggestStat(id, title, query string, y int) string {
	return fmt.Sprintf(`{"id":%q,"type":"stat","title":%q,"gridPos":{"x":0,"y":%d,"w":6,"h":4},"datasource":%q,"queries":[{"query":%q}]}`, id, title, y, fakeUID, query)
}

func suggestText(id, content string, y int) string {
	return fmt.Sprintf(`{"id":%q,"type":"text","title":"Notes","gridPos":{"x":6,"y":%d,"w":6,"h":4},"content":%q}`, id, y, content)
}

type suggestRig struct {
	registry    *Registry
	base        *spec.Dashboard
	ctx         context.Context
	dashboardID string

	mu      sync.Mutex
	queries []string
}

func newSuggestRig(t *testing.T) *suggestRig {
	t.Helper()
	rig := &suggestRig{ctx: auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "Ada", Role: auth.RoleEditor})}
	frames := func(text string) ([]datasource.Frame, error) {
		rig.mu.Lock()
		rig.queries = append(rig.queries, text)
		rig.mu.Unlock()
		switch {
		case strings.Contains(text, "broken"):
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "parse error")
		case strings.Contains(text, "empty"):
			return nil, nil
		}
		return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
	}
	ds := &fakeDS{
		rangeFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
			return frames(q.Text)
		},
		instantFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]datasource.Frame, error) {
			return frames(q.Text)
		},
	}
	rig.registry = newTestRegistry(t, ds, nil)
	database := dbtest.New(t).DB
	rig.registry.deps.Dashboards = dashboards.New(database, rig.registry.deps.Runner, folders.NewAccess(database), slog.New(slog.DiscardHandler))
	base, err := spec.Parse([]byte(specOf(suggestStat("p1", "One", "count(up)", 0), suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "hello", 0))))
	if err != nil {
		t.Fatal(err)
	}
	rig.base = base
	saved, _, err := rig.registry.deps.Dashboards.Create(rig.ctx, auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "Ada", Role: auth.RoleEditor}, pgtype.UUID{}, []byte(specOf(suggestStat("p1", "One", "count(up)", 0), suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "hello", 0))), "seed")
	if err != nil {
		t.Fatal(err)
	}
	rig.dashboardID = saved.ID
	return rig
}

func (r *suggestRig) suggest(t *testing.T, panelID, tool, input string) (Result, []string) {
	t.Helper()
	r.mu.Lock()
	r.queries = nil
	r.mu.Unlock()
	ctx := WithRun(r.ctx, Run{Mode: ModeSuggest, DashboardID: r.dashboardID, ParentVersion: 1, PanelID: panelID, Base: r.base})
	res, err := r.registry.Call(ctx, tool, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return res, slices.Clone(r.queries)
}

func suggestUpdate(model string) string {
	return `{"dashboard_id":"` + dashB + `","parent_version":99,"spec":` + model + `}`
}

func TestSuggestMode(t *testing.T) {
	overlapping := `{"id":"p1","type":"stat","title":"One","gridPos":{"x":0,"y":4,"w":6,"h":4},"datasource":"` + fakeUID + `","queries":[{"query":"count(up)"}]}`
	nulTitle := `{"id":"p1","type":"stat","title":"One\u0000x","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"` + fakeUID + `","queries":[{"query":"count(up)"}]}`
	nulContent := `{"id":"t1","type":"text","title":"Notes","gridPos":{"x":6,"y":0,"w":6,"h":4},"content":"a\u0000b"}`
	withVariable := `{"specVersion":"0","title":"Probe","time":{"from":"now-15m","to":"now"},"variables":[{"name":"svc","type":"custom","options":["a","b"]}],"panels":[` +
		suggestStat("p1", "One", `count(up{svc="$svc"})`, 0) + `,` + suggestStat("p2", "Two", "sum(up)", 4) + `,` + suggestText("t1", "hello", 0) + `]}`

	tests := []struct {
		name        string
		panelID     string
		tool        string
		input       string
		wantCode    string
		wantIssue   spec.IssueCode
		wantQueries []string
		wantTitle   string
	}{
		{
			name:        "only the target panel runs and other panels are ignored",
			panelID:     "p1",
			input:       suggestUpdate(specOf(suggestStat("p1", "One", "count(up) by (x)", 0), suggestStat("p2", "CHANGED", "broken", 4), suggestText("t1", "hello", 0))),
			wantQueries: []string{"count(up) by (x)"},
			wantTitle:   "One",
		},
		{
			name:        "text panels skip the query checks",
			panelID:     "t1",
			input:       suggestUpdate(specOf(suggestStat("p1", "One", "broken", 0), suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "new notes", 0))),
			wantQueries: nil,
			wantTitle:   "Notes",
		},
		{
			name:        "a text panel over the old size guard is accepted",
			panelID:     "t1",
			input:       suggestUpdate(specOf(suggestStat("p1", "One", "count(up)", 0), suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", strings.Repeat("a", 18000), 0))),
			wantQueries: nil,
			wantTitle:   "Notes",
		},
		{
			name:     "a variable only in the model spec is unknown",
			panelID:  "p1",
			input:    suggestUpdate(withVariable),
			wantCode: codeInvalidSpec, wantIssue: spec.CodeUnknownVariable,
		},
		{
			name:     "dropping the panel is repairable",
			panelID:  "p1",
			input:    suggestUpdate(specOf(suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "hello", 0))),
			wantCode: codeInvalidRequest,
		},
		{
			name:     "overlapping another panel is repairable",
			panelID:  "p1",
			input:    suggestUpdate(specOf(overlapping, suggestStat("p2", "Two", "sum(up)", 8), suggestText("t1", "hello", 0))),
			wantCode: codeInvalidSpec, wantIssue: spec.CodeGridOverlap,
		},
		{
			name:     "a placeholder is repairable",
			panelID:  "p1",
			input:    suggestUpdate(specOf(placeholderPanel("p1", 0, 0, "no worldmap"), suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "hello", 0))),
			wantCode: codeInvalidSpec, wantIssue: codePlaceholderNotAllowed,
		},
		{
			name:     "html in a suggested text panel is rejected",
			panelID:  "t1",
			input:    suggestUpdate(specOf(suggestStat("p1", "One", "count(up)", 0), suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "<b>bold</b>", 0))),
			wantCode: codeInvalidSpec, wantIssue: codeHTMLNotAllowed,
		},
		{
			name:        "an autolink in a suggested text panel is accepted",
			panelID:     "t1",
			input:       suggestUpdate(specOf(suggestStat("p1", "One", "count(up)", 0), suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "see <https://example.com>", 0))),
			wantQueries: nil,
			wantTitle:   "Notes",
		},
		{
			name:    "a canvas action in a suggested panel is rejected",
			panelID: "p1",
			input: suggestUpdate(specOf(
				strings.Replace(canvasPanelJSON("p1", "Deploy", deployActions), `"x":6,"y":0`, `"x":0,"y":0`, 1),
				suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "hello", 0))),
			wantCode: codeInvalidSpec, wantIssue: codeActionNotAllowed,
		},
		{
			name:     "a row is repairable",
			panelID:  "p1",
			input:    suggestUpdate(specOf(rowPanel("p1", 12), suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "hello", 0))),
			wantCode: codeInvalidRequest,
		},
		{
			name:     "create_dashboard is refused",
			panelID:  "p1",
			tool:     nameCreateDashboard,
			input:    `{"spec":` + specOf() + `}`,
			wantCode: codeInvalidRequest,
		},
		{
			name:        "a failing query is repairable",
			panelID:     "p1",
			input:       suggestUpdate(specOf(suggestStat("p1", "One", "broken", 0), suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "hello", 0))),
			wantCode:    codeInvalidSpec,
			wantIssue:   queryproxy.CodeQueryInvalid,
			wantQueries: []string{"broken"},
		},
		{
			name:        "an empty result is repairable",
			panelID:     "p1",
			input:       suggestUpdate(specOf(suggestStat("p1", "One", "empty", 0), suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "hello", 0))),
			wantCode:    codeInvalidSpec,
			wantIssue:   queryproxy.CodeQueryEmpty,
			wantQueries: []string{"empty"},
		},
		{
			name:        "NUL in a title is rejected",
			panelID:     "p1",
			input:       suggestUpdate(specOf(nulTitle, suggestStat("p2", "Two", "sum(up)", 4), suggestText("t1", "hello", 0))),
			wantCode:    codeInvalidSpec,
			wantIssue:   spec.CodeSchema,
			wantQueries: nil,
		},
		{
			name:     "NUL in text content is rejected",
			panelID:  "t1",
			input:    suggestUpdate(specOf(suggestStat("p1", "One", "count(up)", 0), suggestStat("p2", "Two", "sum(up)", 4), nulContent)),
			wantCode: codeInvalidSpec, wantIssue: spec.CodeSchema,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rig := newSuggestRig(t)
			tool := tc.tool
			if tool == "" {
				tool = nameUpdateDashboard
			}
			res, queries := rig.suggest(t, tc.panelID, tool, tc.input)
			if !slices.Equal(queries, tc.wantQueries) {
				t.Errorf("queries = %q, want %q", queries, tc.wantQueries)
			}
			if tc.wantCode == "" {
				out, ok := res.Content.(suggestedOut)
				if res.IsError || !ok || !out.Suggested || out.PanelID != tc.panelID {
					t.Fatalf("expected a proposal, got %#v", res.Content)
				}
				var panel spec.Panel
				if err := json.Unmarshal(out.Panel, &panel); err != nil {
					t.Fatal(err)
				}
				if panel.ID != tc.panelID || panel.Title != tc.wantTitle {
					t.Errorf("panel = %s", out.Panel)
				}
				return
			}
			if got := errorCodeOf(t, res); got != tc.wantCode {
				t.Fatalf("code = %q, want %q (%#v)", got, tc.wantCode, res.Content)
			}
			if tc.wantIssue != "" {
				body, ok := res.Content.(invalidSpecOut)
				if !ok || !slices.ContainsFunc(body.Issues, func(i spec.Issue) bool { return i.Code == tc.wantIssue }) {
					t.Errorf("issues do not include %q: %#v", tc.wantIssue, res.Content)
				}
			}
		})
	}
}

func errorCodeOf(t *testing.T, res Result) string {
	t.Helper()
	switch body := res.Content.(type) {
	case errorBody:
		return body.Code
	case invalidSpecOut:
		return body.Code
	}
	t.Fatalf("result is not an error: %#v", res.Content)
	return ""
}
