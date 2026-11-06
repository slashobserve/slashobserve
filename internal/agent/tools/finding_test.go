package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

var findingWindowTo = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

type findingRig struct {
	registry   *Registry
	findings   *findings.Service
	dashboards *dashboards.Service
	db         *dbtest.Database
}

func newFindingRig(t *testing.T) *findingRig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	frames := func(q datasource.Query) ([]datasource.Frame, error) {
		if strings.Contains(q.Text, "empty") {
			return nil, nil
		}
		return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
	}
	ds := &fakeDS{
		rangeFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
			return frames(q)
		},
		instantFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]datasource.Frame, error) {
			return frames(q)
		},
	}
	datasources, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(datasources, logger)
	access := folders.NewAccess(database.DB)
	service := dashboards.New(database.DB, runner, access, logger)
	found := findings.New(database.DB, access, annotations.New(database.DB, access), service, logger)
	registry := New(Deps{Datasources: datasources, Runner: runner, Dashboards: service, Folders: access, Findings: found, Logger: logger})
	return &findingRig{registry: registry, findings: found, dashboards: service, db: database}
}

func investigateRun(from, to time.Time) Run {
	return Run{Mode: ModeInvestigate, Prompt: "why are checkout errors up\nsince the deploy", From: from, To: to}
}

func (r *findingRig) callIn(t *testing.T, run *Run, by auth.Principal, tool, input string) Result {
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

func evidencePanel(title, query string) string {
	return fmt.Sprintf(`{"type":"timeseries","title":%q,"datasource":%q,"queries":[{"query":%q}]}`, title, fakeUID, query)
}

func findingBody(markdown string, panels ...string) string {
	encoded, err := json.Marshal(markdown)
	if err != nil {
		panic(err)
	}
	return `{"markdown":` + string(encoded) + `,"evidence":[` + strings.Join(panels, ",") + `]}`
}

func TestFindingToolsOnlyRunWhereTheyBelong(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	editor := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, ID: editorCaller.ID, Role: auth.RoleEditor})
	investigate := WithRun(editor, investigateRun(time.Time{}, time.Time{}))
	create := WithRun(editor, Run{Mode: ModeCreate})
	revise := WithRun(editor, Run{Mode: ModeRevise})
	suggest := WithRun(editor, Run{Mode: ModeSuggest})
	cases := []struct {
		name    string
		ctx     context.Context
		tool    string
		input   string
		refused bool
		message string
	}{
		{"save_finding outside a run", editor, nameSaveFinding, `{}`, true, "save_finding runs inside the agent; MCP clients use create_finding"},
		{"create_dashboard in an investigation", investigate, nameCreateDashboard, `{}`, true, "an investigate run only reads data and ends with save_finding; it cannot use create_dashboard"},
		{"update_panels in an investigation", investigate, nameUpdatePanels, `{}`, true, "an investigate run only reads data and ends with save_finding; it cannot use update_panels"},
		{"query_range in an investigation", investigate, nameQueryRange, `{"datasource":"fake","query":"up"}`, false, ""},
		{"save_finding in a create run", create, nameSaveFinding, `{}`, false, ""},
		{"save_finding in a revise run", revise, nameSaveFinding, `{}`, false, ""},
		{"save_finding in a suggest run", suggest, nameSaveFinding, `{}`, true, "save_finding cannot end a suggest run; propose the panel instead"},
	}
	for _, c := range cases {
		res, err := r.Call(c.ctx, c.tool, []byte(c.input))
		if !c.refused && errors.Is(err, errFindingsNotConfigured) {
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		body, isError := res.Content.(errorBody)
		refused := res.IsError && isError && body.Code == codeNotInThisRun
		if refused != c.refused || (c.refused && (body.Message != c.message || res.Step != "Refused "+c.tool+": not_in_this_run")) {
			t.Errorf("%s: %#v step %q", c.name, res.Content, res.Step)
		}
	}
}

func TestOfferedInSplitsTheToolsByMode(t *testing.T) {
	byName := map[string]Tool{}
	for _, tool := range newTestRegistry(t, &fakeDS{}, nil).List() {
		byName[tool.Name] = tool
	}
	modes := []Mode{ModeCreate, ModeRevise, ModeSuggest, ModeInvestigate}
	cases := []struct {
		tool    string
		offered []Mode
	}{
		{nameQueryRange, modes},
		{nameGetDashboard, modes},
		{nameCreateDashboard, []Mode{ModeCreate, ModeRevise, ModeSuggest}},
		{nameUpdatePanels, []Mode{ModeCreate, ModeRevise, ModeSuggest}},
		{nameImportGrafana, []Mode{ModeCreate, ModeRevise, ModeSuggest}},
		{nameSaveFinding, []Mode{ModeCreate, ModeRevise, ModeInvestigate}},
		{nameListAnnotations, []Mode{ModeInvestigate}},
		{nameListFindings, modes},
		{nameCreateFinding, nil},
		{nameListComments, nil},
	}
	for _, c := range cases {
		tool := byName[c.tool]
		for _, mode := range modes {
			want := slices.Contains(c.offered, mode)
			if tool.OfferedIn(mode) != want {
				t.Errorf("%s offered in %s = %t", c.tool, mode, !want)
			}
		}
	}
}

func TestSaveFindingRefusesWhatAFindingCannotHold(t *testing.T) {
	r := newFindingRig(t)
	run := investigateRun(time.Time{}, time.Time{})
	five := []string{}
	for i := range 5 {
		five = append(five, evidencePanel(fmt.Sprintf("P%d", i), "up"))
	}
	cases := []struct {
		name, input, code, message string
	}{
		{"no markdown", findingBody("  "), codeInvalidRequest, "markdown is required"},
		{"401 words", findingBody(strings.Repeat("word ", 401)), codeTooLong, "the finding is 401 words; keep it to 400 words and move detail into evidence panels"},
		{"html", findingBody("checkout <script>x</script> failed"), codeInvalidRequest, "markdown must be plain Markdown without HTML"},
		{"control character", findingBody("checkout\u0007 failed"), codeInvalidRequest, "markdown must not contain control or bidirectional override characters"},
		{"five panels", findingBody("checkout failed", five...), codeInvalidRequest, "evidence takes at most 4 panels"},
		{"a text panel", findingBody("checkout failed", `{"type":"text","title":"Notes","content":"hi"}`), codeInvalidRequest, "evidence[0] must be a data panel with queries, not text"},
		{"half a window", `{"markdown":"checkout failed","from":"now-1h"}`, codeInvalidRequest, "from and to go together"},
	}
	for _, c := range cases {
		res := r.callIn(t, &run, editorCaller, nameSaveFinding, c.input)
		body, ok := res.Content.(errorBody)
		if !res.IsError || !ok || body.Code != c.code || body.Message != c.message {
			t.Errorf("%s: %#v", c.name, res.Content)
		}
	}
	res := r.callIn(t, &run, editorCaller, nameSaveFinding, findingBody("checkout failed", evidencePanel("Errors", "up"), evidencePanel("Nothing", "empty")))
	rejected, ok := res.Content.(invalidSpecOut)
	if !res.IsError || !ok || rejected.Code != codeInvalidSpec || len(rejected.Issues) != 1 || !strings.HasPrefix(rejected.Issues[0].Path, "/panels/1") {
		t.Fatalf("a panel without data: %#v", res.Content)
	}
}

func TestSaveFindingStoresTheEvidenceInTheRunsWindow(t *testing.T) {
	r := newFindingRig(t)
	from := findingWindowTo.Add(-2 * time.Hour)
	run := investigateRun(from, findingWindowTo)
	res := r.callIn(t, &run, editorCaller, nameSaveFinding, findingBody("checkout-0 throws 5xx since 09:12", evidencePanel("Errors", "sum(rate(errors[5m]))"), evidencePanel("Latency", "histogram_quantile(0.99, x)")))
	saved, ok := res.Content.(findingSavedOut)
	if res.IsError || !ok || saved.Code != "saved" || saved.FindingID == "" || saved.Evidence != 2 || saved.URL != "/findings/"+saved.FindingID {
		t.Fatalf("save: %#v", res.Content)
	}
	if res.Step != "Saved finding "+saved.FindingID {
		t.Errorf("step = %q", res.Step)
	}
	id, err := db.ParseUUID(saved.FindingID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := r.findings.Get(t.Context(), editorCaller, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Evidence) != 2 || f.Evidence[0].ID != "evidence-1" || f.Evidence[1].ID != "evidence-2" {
		t.Errorf("evidence = %+v", f.Evidence)
	}
	if !f.From.Equal(from) || !f.To.Equal(findingWindowTo) || f.Title != "why are checkout errors up" || f.DashboardID != "" {
		t.Errorf("finding = %s to %s %q dashboard %q", f.From, f.To, f.Title, f.DashboardID)
	}
}

func TestCreateFindingKeepsHiddenDashboardsHidden(t *testing.T) {
	r := newFindingRig(t)
	board, _, err := r.dashboards.Create(t.Context(), adminCaller, pgtype.UUID{}, []byte(boardSpec(hiddenTitle)), "create")
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := db.ParseUUID(board.ID)
	if err != nil {
		t.Fatal(err)
	}
	foldertest.Move(t, r.db, hidden, foldertest.Restricted(t, r.db, hiddenFolder, privateGroup))
	input := `{"markdown":"payroll is late","dashboard_id":"` + board.ID + `"}`
	if res := r.callIn(t, nil, editorCaller, nameCreateFinding, input); errorCode(t, res) != codeNotFound {
		t.Fatalf("hidden dashboard: %#v", res.Content)
	}
	res := r.callIn(t, nil, adminCaller, nameCreateFinding, `{"markdown":"payroll is late","title":"Payroll","dashboard_id":"`+board.ID+`","from":"now-3h","to":"now"}`)
	created, ok := res.Content.(findings.FindingJSON)
	if res.IsError || !ok || created.Title != "Payroll" || created.DashboardID == nil || *created.DashboardID != board.ID || created.AgentRequestID != nil {
		t.Fatalf("visible dashboard: %#v", res.Content)
	}
	if res := r.callIn(t, nil, editorCaller, nameCreateFinding, `{"markdown":"x","title":"two\nlines"}`); errorCode(t, res) != codeInvalidRequest {
		t.Errorf("a two-line title: %#v", res.Content)
	}
	if res := r.callIn(t, nil, viewerCaller, nameCreateFinding, `{"markdown":"x"}`); errorCode(t, res) != codeForbiddenRole {
		t.Errorf("a viewer: %#v", res.Content)
	}
}

func TestListFindingsPagesTheCallersFindings(t *testing.T) {
	r := newFindingRig(t)
	for i := range 3 {
		if res := r.callIn(t, nil, editorCaller, nameCreateFinding, fmt.Sprintf(`{"markdown":"finding %d"}`, i)); res.IsError {
			t.Fatalf("create: %#v", res.Content)
		}
	}
	if res := r.callIn(t, nil, keyCaller, nameCreateFinding, `{"markdown":"someone else"}`); res.IsError {
		t.Fatalf("create: %#v", res.Content)
	}
	var seen []string
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 3 {
			t.Fatal("the list did not end")
		}
		input := `{"limit":2}`
		if cursor != "" {
			input = `{"limit":2,"cursor":"` + cursor + `"}`
		}
		res := r.callIn(t, nil, viewerCaller, nameListFindings, input)
		if res.IsError {
			t.Fatalf("list: %#v", res.Content)
		}
		if len(res.Content.(findingListOut).Findings) != 0 {
			t.Fatalf("a viewer sees another principal's findings: %#v", res.Content)
		}
		res = r.callIn(t, nil, editorCaller, nameListFindings, input)
		page := res.Content.(findingListOut)
		for _, f := range page.Findings {
			seen = append(seen, f.Markdown)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if want := []string{"finding 2", "finding 1", "finding 0"}; strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Errorf("listed %v, want %v", seen, want)
	}
	if res := r.callIn(t, nil, editorCaller, nameListFindings, `{"cursor":"nope"}`); errorCode(t, res) != codeInvalidRequest {
		t.Errorf("a bad cursor: %#v", res.Content)
	}
}
