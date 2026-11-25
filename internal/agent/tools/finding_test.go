package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
	"github.com/slashobserve/slashobserve/internal/logtest"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var findingWindowTo = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

type findingRig struct {
	registry   *Registry
	findings   *findings.Service
	dashboards *dashboards.Service
	db         *dbtest.Database

	observations *observations.Service
	logs         *logtest.Capture
}

func newFindingRig(t *testing.T) *findingRig {
	t.Helper()
	database := dbtest.New(t)
	logger, logs := logtest.New()
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
	obs := observations.New(database.DB, access, snapshotFreezer{}, nil, logger)
	found, err := findings.New(database.DB, access, annotations.New(database.DB, access), service, obs, logger)
	if err != nil {
		t.Fatal(err)
	}
	registry := New(Deps{Datasources: datasources, Runner: runner, Dashboards: service, Folders: access, Findings: found, Observations: obs, Logger: logger})
	return &findingRig{registry: registry, findings: found, dashboards: service, db: database, observations: obs, logs: logs}
}

func investigateRun(from, to time.Time) Run {
	return Run{Mode: ModeInvestigate, Prompt: "why are checkout errors up\nsince the deploy", From: from, To: to, Investigation: true}
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
	editor := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: editorCaller.ID, Role: auth.RoleEditor})
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
		{"save_finding outside a run", editor, nameSaveFinding, `{}`, true, "save_finding runs inside the agent; MCP clients use create_observation"},
		{"create_dashboard in an investigation", investigate, nameCreateDashboard, `{}`, true, "an investigate run never edits a dashboard's panels, variables or settings and ends with save_finding; it cannot use create_dashboard, so if the request asks for such a change, say in the finding that it has to be asked for as a change on the dashboard, not as an investigation"},
		{"update_panels in an investigation", investigate, nameUpdatePanels, `{}`, true, "an investigate run never edits a dashboard's panels, variables or settings and ends with save_finding; it cannot use update_panels, so if the request asks for such a change, say in the finding that it has to be asked for as a change on the dashboard, not as an investigation"},
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
		{nameListAnnotations, modes},
		{nameListComments, modes},
		{nameCreateObservation, []Mode{ModeCreate, ModeRevise, ModeInvestigate}},
		{nameMoveDashboard, []Mode{ModeCreate, ModeRevise, ModeInvestigate}},
		{nameCreateComment, nil},
		{nameStartRun, nil},
		{nameRestoreDashboardVersion, nil},
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
		{"control character", findingBody("checkout\u0007 failed"), codeInvalidRequest, "markdown must not contain control characters other than line breaks and tabs; remove them"},
		{"override character", findingBody("checkout\u202e failed"), codeInvalidRequest, "markdown must not contain bidirectional override characters (U+202A to U+202E, U+2066 to U+2069); remove them"},
		{"invisible only", findingBody("\u200b"), codeInvalidRequest, "markdown has no visible character; enter 1 to 4000 characters of visible text"},
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
	rejected, ok := res.Content.(patchRejectedOut)
	if !res.IsError || !ok || rejected.Code != codeInvalidSpec || len(rejected.Issues) != 1 || !strings.HasPrefix(rejected.Issues[0].Path, "/panels/1") || !slices.Equal(rejected.FailingPanels, []string{"evidence-2"}) {
		t.Fatalf("a panel without data: %#v", res.Content)
	}
}

func TestSaveFindingStoresTheEvidenceInTheRunsWindow(t *testing.T) {
	r := newFindingRig(t)
	from := findingWindowTo.Add(-2 * time.Hour)
	run := investigateRun(from, findingWindowTo)
	res := r.callIn(t, &run, editorCaller, nameSaveFinding, findingBody("checkout-0 throws 5xx since 09:12", evidencePanel("Errors", "sum(rate(errors[5m]))"), evidencePanel("Latency", "histogram_quantile(0.99, x)")))
	saved, ok := res.Content.(findingSavedOut)
	if res.IsError || !ok || saved.FindingID == "" || saved.Evidence != 2 || saved.URL == "" {
		t.Fatalf("save: %#v", res.Content)
	}
	if res.Step != "Saved finding "+saved.FindingID {
		t.Errorf("step = %q", res.Step)
	}
	id, err := db.ParseUUID(saved.FindingID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := r.findings.Get(reading(t.Context()), editorCaller, id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.URL != "/o/"+f.ObservationID {
		t.Errorf("url = %q, want the finding's observation %s", saved.URL, f.ObservationID)
	}
	if len(f.Evidence) != 2 || f.Evidence[0].ID != "evidence-1" || f.Evidence[1].ID != "evidence-2" {
		t.Errorf("evidence = %+v", f.Evidence)
	}
	if !f.From.Equal(from) || !f.To.Equal(findingWindowTo) || f.Title != "Finding" || f.DashboardID != "" {
		t.Errorf("finding = %s to %s %q dashboard %q", f.From, f.To, f.Title, f.DashboardID)
	}
}

func TestSaveFindingUsesTheShortTitleTheAgentGave(t *testing.T) {
	r := newFindingRig(t)
	run := Run{Mode: ModeCreate, Prompt: "@observe why are server errors elevated, and which pod is responsible for them right now?"}
	res := r.callIn(t, &run, editorCaller, nameSaveFinding, `{"title":"Checkout 5xx errors by pod","markdown":"checkout-0 throws 5xx"}`)
	saved, ok := res.Content.(findingSavedOut)
	if res.IsError || !ok {
		t.Fatalf("save: %#v", res.Content)
	}
	id, err := db.ParseUUID(saved.FindingID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := r.findings.Get(reading(t.Context()), editorCaller, id)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "Checkout 5xx errors by pod" {
		t.Errorf("title = %q", f.Title)
	}
}

func TestSaveFindingNamesAnUntitledAnswerWithoutThePrompt(t *testing.T) {
	r := newFindingRig(t)
	for name, c := range map[string]struct {
		investigation bool
		want          string
	}{
		"an ask":           {false, "Answer"},
		"an investigation": {true, "Finding"},
	} {
		run := Run{Mode: ModeCreate, Prompt: "@observe why are server errors elevated, and which pod is responsible for them right now?", Investigation: c.investigation}
		res := r.callIn(t, &run, editorCaller, nameSaveFinding, `{"markdown":"checkout-0 throws 5xx"}`)
		saved, ok := res.Content.(findingSavedOut)
		if res.IsError || !ok {
			t.Fatalf("%s: save: %#v", name, res.Content)
		}
		id, err := db.ParseUUID(saved.FindingID)
		if err != nil {
			t.Fatal(err)
		}
		f, err := r.findings.Get(reading(t.Context()), editorCaller, id)
		if err != nil {
			t.Fatal(err)
		}
		if f.Title != c.want {
			t.Errorf("%s: title = %q, want %q", name, f.Title, c.want)
		}
	}
}

func TestSaveFindingDropsTheStructuredFieldsOutsideAnInvestigation(t *testing.T) {
	r := newFindingRig(t)
	body := `{"markdown":"checkout-0 throws 5xx","impact":"checkout fails for 12 percent of orders","confidence":"high","next_steps":["roll back the deploy"]}`
	for name, c := range map[string]struct {
		investigation bool
		kept          bool
	}{
		"an ask":           {false, false},
		"an investigation": {true, true},
	} {
		run := Run{Mode: ModeCreate, Prompt: "why are server errors elevated", Investigation: c.investigation}
		res := r.callIn(t, &run, editorCaller, nameSaveFinding, body)
		saved, ok := res.Content.(findingSavedOut)
		if res.IsError || !ok {
			t.Fatalf("%s: save: %#v", name, res.Content)
		}
		if got := saved.Note != ""; got != !c.kept {
			t.Errorf("%s: note = %q, want a note = %t", name, saved.Note, !c.kept)
		}
		id, err := db.ParseUUID(saved.FindingID)
		if err != nil {
			t.Fatal(err)
		}
		f, err := r.findings.Get(reading(t.Context()), editorCaller, id)
		if err != nil {
			t.Fatal(err)
		}
		if got := f.Structured.Impact != "" && f.Structured.Confidence == "high" && len(f.Structured.NextSteps) == 1; got != c.kept {
			t.Errorf("%s: structured fields kept = %t, want %t", name, got, c.kept)
		}
	}
}

func TestAnUntitledPrivateAnswerSavedAsAnObservationShowsNoWordOfItsQuestion(t *testing.T) {
	const secret = "quasarbudget"
	r := newFindingRig(t)
	queue := revisions.NewService(r.db.DB, slog.New(slog.DiscardHandler), revisions.WithBuiltInAgent(), revisions.WithAccess(folders.NewAccess(r.db.DB)))
	board, _, err := r.dashboards.Create(reading(t.Context()), adminCaller, pgtype.UUID{}, []byte(boardSpec("Checkout")), "create")
	if err != nil {
		t.Fatal(err)
	}
	prompt := "is the " + secret + " plan why checkout fails"
	started, err := queue.Start(reading(t.Context()), editorCaller, revisions.StartRequest{Prompt: &prompt, DashboardID: &board.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !revisions.PrivateAsk(started.AgentRequest) {
		t.Fatalf("run %+v is not a private ask", started.AgentRequest)
	}
	run, err := RunFor(revisions.RequestOf(started.AgentRequest))
	if err != nil {
		t.Fatal(err)
	}
	run.From, run.To = findingWindowTo.Add(-time.Hour), findingWindowTo
	if res := r.callIn(t, &run, editorCaller, nameSaveFinding, findingBody("checkout-0 throws 5xx")); res.IsError {
		t.Fatalf("save: %#v", res.Content)
	}
	res := r.callIn(t, nil, editorCaller, nameSaveRunAnswer, `{"run_id":"`+started.ID.String()+`"}`)
	filed, ok := res.Content.(runAnswerSavedOut)
	if res.IsError || !ok {
		t.Fatalf("save answer: %#v", res.Content)
	}

	reader := auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Bea", Role: auth.RoleEditor}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	observations.NewHandler(r.observations, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), reader))
	})))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequestWithContext(reading(t.Context()), http.MethodGet, "/api/v1/observations/"+filed.ObservationID, nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), secret) || !strings.Contains(rec.Body.String(), `"title":"Answer"`) {
		t.Errorf("REST read of the saved answer: %d %s", rec.Code, rec.Body)
	}
	read := r.callIn(t, nil, reader, nameGetObservation, `{"observation_id":"`+filed.ObservationID+`"}`)
	raw, err := json.Marshal(read.Content)
	if err != nil {
		t.Fatal(err)
	}
	if read.IsError || strings.Contains(string(raw), secret) || !strings.Contains(string(raw), `"title":"Answer"`) {
		t.Errorf("get_observation of the saved answer: %s", raw)
	}
}

func (r *findingRig) investigation(t *testing.T, by auth.Principal, from, to time.Time) (Run, dbsqlc.AgentRequest) {
	t.Helper()
	byID, err := db.ParseUUID(by.ID)
	if err != nil {
		t.Fatal(err)
	}
	row, err := r.db.Queries().EnqueueInvestigateRequest(reading(t.Context()), dbsqlc.EnqueueInvestigateRequestParams{
		Prompt: "why are checkout errors up\nsince the deploy", RequestedByKind: string(by.Kind), RequestedByID: byID, RequestedByName: by.Name,
	})
	if err != nil {
		t.Fatalf("enqueue investigation: %v", err)
	}
	err = r.db.InTx(reading(t.Context()), func(q *dbsqlc.Queries) error {
		id, err := r.observations.OpenForRun(reading(t.Context()), q, observations.RunOpen{Run: row, By: by, Strict: true})
		if err != nil {
			return err
		}
		row, err = q.SetRequestObservation(reading(t.Context()), dbsqlc.SetRequestObservationParams{ID: row.ID, ObservationID: id})
		return err
	})
	if err != nil {
		t.Fatalf("open observation: %v", err)
	}
	run := investigateRun(from, to)
	run.RequestID = row.ID.String()
	return run, row
}

func TestSaveFindingWritesTheConclusionIntoTheRunsObservation(t *testing.T) {
	r := newFindingRig(t)
	from := findingWindowTo.Add(-2 * time.Hour)
	run, row := r.investigation(t, editorCaller, from, findingWindowTo)
	body := findingBody("checkout-0 throws 5xx since 09:12", evidencePanel("Errors", "sum(rate(errors[5m]))"), evidencePanel("Latency", "histogram_quantile(0.99, x)"))
	res := r.callIn(t, &run, editorCaller, nameSaveFinding, body)
	saved, ok := res.Content.(findingSavedOut)
	if res.IsError || !ok || saved.Replayed || saved.Evidence != 2 {
		t.Fatalf("save: %#v", res.Content)
	}
	o, err := r.observations.Get(reading(t.Context()), editorCaller, row.ObservationID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := make([]observations.Kind, len(o.Blocks))
	for i, b := range o.Blocks {
		kinds[i] = b.Kind
	}
	if want := []observations.Kind{observations.KindRun, observations.KindText, observations.KindPanel, observations.KindPanel}; !slices.Equal(kinds, want) {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	var text observations.TextBody
	if err := json.Unmarshal(o.Blocks[1].Body, &text); err != nil || text.Markdown != "## Finding\n\ncheckout-0 throws 5xx since 09:12" {
		t.Errorf("text %q, %v", text.Markdown, err)
	}
	var panel observations.PanelBody
	if err := json.Unmarshal(o.Blocks[2].Body, &panel); err != nil || panel.From == nil || !panel.From.Equal(from) || !panel.To.Equal(findingWindowTo) {
		t.Errorf("panel %s, %v", o.Blocks[2].Body, err)
	}
	var cited struct {
		RunID string   `json:"run_id"`
		Cites []string `json:"cites"`
	}
	if err := json.Unmarshal(o.Blocks[0].Body, &cited); err != nil || cited.RunID != row.ID.String() || !slices.Equal(cited.Cites, []string{o.Blocks[1].ID, o.Blocks[2].ID, o.Blocks[3].ID}) {
		t.Errorf("run block %s, %v", o.Blocks[0].Body, err)
	}
	for _, b := range o.Blocks[1:] {
		if b.State != observations.BlockLive || b.RunID != row.ID.String() {
			t.Errorf("block %+v", b)
		}
	}

	again := r.callIn(t, &run, editorCaller, nameSaveFinding, body)
	if replayed, ok := again.Content.(findingSavedOut); again.IsError || !ok || !replayed.Replayed || replayed.FindingID != saved.FindingID {
		t.Fatalf("second save: %#v", again.Content)
	}
	after, err := r.observations.Get(reading(t.Context()), editorCaller, row.ObservationID)
	if err != nil || len(after.Blocks) != 4 {
		t.Errorf("a second save left %d blocks, %v", len(after.Blocks), err)
	}
}

func (r *findingRig) answerRun(t *testing.T, by auth.Principal, dashboard pgtype.UUID) (Run, dbsqlc.AgentRequest) {
	t.Helper()
	byID, err := db.ParseUUID(by.ID)
	if err != nil {
		t.Fatal(err)
	}
	row, err := r.db.Queries().EnqueueAgentRequest(reading(t.Context()), dbsqlc.EnqueueAgentRequestParams{
		DashboardID: dashboard, Intent: "apply", Prompt: "why is this panel flat", BaseVersion: 1,
		RequestedByKind: string(by.Kind), RequestedByID: byID, RequestedByName: by.Name,
	})
	if err != nil {
		t.Fatalf("enqueue answer run: %v", err)
	}
	run := Run{Mode: ModeRevise, Prompt: row.Prompt, DashboardID: dashboard.String(), RequestID: row.ID.String(), From: findingWindowTo.Add(-time.Hour), To: findingWindowTo}
	return run, row
}

func TestAnAnswerRunKeepsItsFindingInTheChatUntilItsRequesterSavesItAsAnObservation(t *testing.T) {
	r := newFindingRig(t)
	board, _, err := r.dashboards.Create(reading(t.Context()), adminCaller, pgtype.UUID{}, []byte(boardSpec("Checkout")), "create")
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := db.ParseUUID(board.ID)
	if err != nil {
		t.Fatal(err)
	}
	created, err := r.db.Queries().CreateFolder(reading(t.Context()), dbsqlc.CreateFolderParams{Title: "Payments"})
	if err != nil {
		t.Fatal(err)
	}
	folder := created.ID
	foldertest.Move(t, r.db, boardID, folder)
	run, row := r.answerRun(t, adminCaller, boardID)

	res := r.callIn(t, &run, adminCaller, nameSaveFinding, findingBody("checkout-0 throws 5xx", evidencePanel("Errors", "sum(rate(errors[5m]))")))
	saved, ok := res.Content.(findingSavedOut)
	if res.IsError || !ok {
		t.Fatalf("save: %#v", res)
	}
	if saved.URL != "/c/"+row.ID.String() {
		t.Errorf("url = %q, want the chat", saved.URL)
	}
	if after, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), row.ID); err != nil || after.ObservationID.Valid {
		t.Fatalf("run observation %v, %v", after.ObservationID, err)
	}

	res = r.callIn(t, nil, editorCaller, nameSaveRunAnswer, `{"run_id":"`+row.ID.String()+`"}`)
	if body, ok := res.Content.(errorBody); !res.IsError || !ok || body.Code != codeNoAnswer {
		t.Fatalf("another person saves it: %#v", res.Content)
	}
	res = r.callIn(t, nil, adminCaller, nameSaveRunAnswer, `{"run_id":"`+row.ID.String()+`"}`)
	filed, ok := res.Content.(runAnswerSavedOut)
	if res.IsError || !ok || filed.Replayed {
		t.Fatalf("save answer: %#v", res.Content)
	}
	after, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), row.ID)
	if err != nil || !after.ObservationID.Valid || filed.URL != "/o/"+after.ObservationID.String() {
		t.Fatalf("run observation %v, url %q, %v", after.ObservationID, filed.URL, err)
	}
	o, err := r.observations.Get(reading(t.Context()), adminCaller, after.ObservationID)
	if err != nil || o.FolderID != folder.String() {
		t.Fatalf("observation folder %q, %v, want %s", o.FolderID, err, folder)
	}
	again := r.callIn(t, nil, adminCaller, nameSaveRunAnswer, `{"run_id":"`+row.ID.String()+`"}`)
	if repeat, ok := again.Content.(runAnswerSavedOut); !ok || !repeat.Replayed || repeat.ObservationID != filed.ObservationID {
		t.Fatalf("repeat: %#v", again.Content)
	}
}

func TestSaveFindingFailsWhenTheRunsObservationIsOutOfReach(t *testing.T) {
	r := newFindingRig(t)
	run, row := r.investigation(t, editorCaller, findingWindowTo.Add(-time.Hour), findingWindowTo)
	hidden := foldertest.Restricted(t, r.db, "Payroll", "payroll-team")
	r.db.Exec(t, `UPDATE observations SET folder_id = $1 WHERE id = $2`, hidden, row.ObservationID)

	res := r.callIn(t, &run, editorCaller, nameSaveFinding, findingBody("checkout-0 throws 5xx", evidencePanel("Errors", "sum(rate(errors[5m]))")))
	if !res.IsError {
		t.Fatalf("save: %#v", res.Content)
	}
	var findingRows int
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT count(*) FROM findings`).Scan(&findingRows); err != nil || findingRows != 0 {
		t.Errorf("findings left behind: %d, %v", findingRows, err)
	}
	if strings.Contains(r.logs.String(), "checkout-0 throws") {
		t.Error("the finding text was logged")
	}
}

func TestSaveFindingInARunWithoutAnObservationStaysInTheChat(t *testing.T) {
	r := newFindingRig(t)
	run, row := r.investigation(t, editorCaller, findingWindowTo.Add(-time.Hour), findingWindowTo)
	r.db.Exec(t, `UPDATE agent_requests SET observation_id = NULL WHERE id = $1`, row.ID)
	r.db.Exec(t, `DELETE FROM observations WHERE id = $1`, row.ObservationID)
	res := r.callIn(t, &run, editorCaller, nameSaveFinding, findingBody("checkout-0 throws 5xx"))
	saved, ok := res.Content.(findingSavedOut)
	if res.IsError || !ok {
		t.Fatalf("save: %#v", res)
	}
	after, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), row.ID)
	if err != nil || after.ObservationID.Valid || saved.URL != "/c/"+row.ID.String() {
		t.Errorf("run observation %v url %q, %v", after.ObservationID, saved.URL, err)
	}
}

func TestSuggestedChangeIsOneSentenceOnADashboardAskOnly(t *testing.T) {
	onDashboard := Run{Mode: ModeInvestigate, DashboardID: "00000000-0000-4000-8000-000000000001"}
	cases := []struct {
		name string
		raw  string
		run  Run
		want string
		ok   bool
	}{
		{"none anywhere", "", Run{Mode: ModeCreate}, "", true},
		{"a sentence on a dashboard ask", "  Split the error panel by pod. ", onDashboard, "Split the error panel by pod.", true},
		{"on Home", "Add a panel.", Run{Mode: ModeCreate}, "", false},
		{"on an observation ask", "Add a panel.", Run{Mode: ModeInvestigate}, "", false},
		{"in a revision", "Add a panel.", Run{Mode: ModeRevise, DashboardID: onDashboard.DashboardID}, "", false},
		{"two lines", "Add a panel.\nThen another.", onDashboard, "", false},
		{"too long", strings.Repeat("a", maxSuggestedChangeRunes+1), onDashboard, "", false},
	}
	for _, c := range cases {
		got, res, ok := suggestedChange(c.raw, c.run)
		if ok != c.ok || got != c.want || ok == res.IsError {
			t.Errorf("%s: got %q ok %v res %#v", c.name, got, ok, res.Content)
		}
	}
}

func TestAFindingSavedAfterRefusedReadsSaysReadingStoppedEarly(t *testing.T) {
	if got, _, ok := findingMarkdown("checkout is slow", 0, nil); !ok || got != "checkout is slow" {
		t.Fatalf("no refused read changed the text: %q", got)
	}
	one, _, _ := findingMarkdown("checkout is slow", 1, nil)
	if !strings.HasPrefix(one, "checkout is slow\n\n_Reading stopped early: 1 read was refused") || !strings.Contains(one, "counts above may be partial") {
		t.Fatalf("one refused read: %q", one)
	}
	if many, _, _ := findingMarkdown("checkout is slow", 3, nil); !strings.Contains(many, "3 reads were refused") {
		t.Fatalf("three refused reads: %q", many)
	}
}

func TestAFindingSaysWhichDatasourcesCouldNotBeChecked(t *testing.T) {
	got, _, ok := findingMarkdown("Prometheus shows checkout errors at 2%.", 0, []Unchecked{{Datasource: "loki", Code: "query_timeout"}, {Datasource: "tempo", Code: "datasource_unavailable"}})
	want := "Prometheus shows checkout errors at 2%.\n\n_Could not check loki (a read timed out), tempo (the datasource did not answer): those reads did not finish, so this answer does not show that their data is missing._"
	if !ok || got != want {
		t.Fatalf("finding = %q, want %q", got, want)
	}
	if _, res, ok := findingMarkdown(strings.Repeat("word ", 390), 0, []Unchecked{{Datasource: "loki", Code: "query_timeout"}}); ok || errorCode(t, res) != codeTooLong {
		t.Fatalf("390 words with the could-not-check note: ok %v, res %#v", ok, res.Content)
	}
}

func TestSaveFindingAddsTheCouldNotCheckNoteFromTheRun(t *testing.T) {
	r := newFindingRig(t)
	run, row := r.investigation(t, editorCaller, findingWindowTo.Add(-time.Hour), findingWindowTo)
	ctx := WithUnchecked(WithRun(auth.WithPrincipal(t.Context(), editorCaller), run), []Unchecked{{Datasource: "loki", Code: "query_timeout"}})
	res, err := r.registry.Call(ctx, nameSaveFinding, []byte(findingBody("Checkout errors rose after the deploy.")))
	if err != nil || res.IsError {
		t.Fatalf("save: %#v, %v", res.Content, err)
	}
	o, err := r.observations.Get(reading(t.Context()), editorCaller, row.ObservationID)
	if err != nil {
		t.Fatal(err)
	}
	var text observations.TextBody
	if err := json.Unmarshal(o.Blocks[1].Body, &text); err != nil || !strings.HasSuffix(text.Markdown, "Checkout errors rose after the deploy.\n\n_Could not check loki (a read timed out): those reads did not finish, so this answer does not show that their data is missing._") {
		t.Fatalf("text %q, %v", text.Markdown, err)
	}
}

func TestSaveFindingWantsTimesInThePersonsZone(t *testing.T) {
	r := newFindingRig(t)
	run, _ := r.investigation(t, editorCaller, findingWindowTo.Add(-time.Hour), findingWindowTo)
	run.TimeZone = "Asia/Kolkata"
	ctx := WithRun(auth.WithPrincipal(t.Context(), editorCaller), run)
	res, err := r.registry.Call(ctx, nameSaveFinding, []byte(findingBody("Checkout errors rose at 14:22 UTC.")))
	if err != nil || !res.IsError || errorCode(t, res) != codeWrongZone {
		t.Fatalf("a UTC finding for a person in Kolkata: %#v, %v", res.Content, err)
	}
	res, err = r.registry.Call(ctx, nameSaveFinding, []byte(findingBody("Checkout errors rose at 19:52 IST.")))
	if err != nil || res.IsError {
		t.Fatalf("a finding in IST: %#v, %v", res.Content, err)
	}
}

func TestATooLongFindingStepSaysWhatToDoInWords(t *testing.T) {
	_, res, ok := findingMarkdown(strings.Repeat("word ", 401), 0, nil)
	if ok || res.Step != "Finding was 401 words, over the 400 word limit; the agent must shorten it" {
		t.Fatalf("ok %v, step %q", ok, res.Step)
	}
}

func TestAFindingThatFitsOnlyWithoutTheReadsNoteIsSentBackNotLost(t *testing.T) {
	r := newFindingRig(t)
	run := investigateRun(findingWindowTo.Add(-time.Hour), findingWindowTo)
	ctx := WithReadsRefused(WithRun(auth.WithPrincipal(t.Context(), editorCaller), run), 2)
	nearTheLimit := "Checkout errors rose after the deploy. " + strings.Repeat("checkout-payments-0 ", 200)[:3840]
	res, err := r.registry.Call(ctx, nameSaveFinding, []byte(findingBody(nearTheLimit)))
	if err != nil {
		t.Fatalf("a finding near the limit with refused reads ended the run with %v", err)
	}
	if !res.IsError || errorCode(t, res) != codeInvalidRequest {
		t.Fatalf("res = %#v, want the model told to shorten it", res.Content)
	}
	short := "Checkout errors rose after the deploy. " + strings.Repeat("checkout-payments-0 ", 40)
	res, err = r.registry.Call(ctx, nameSaveFinding, []byte(findingBody(short)))
	if err != nil || res.IsError {
		t.Fatalf("a short finding with refused reads: %#v, %v", res.Content, err)
	}
}

func TestTheReadsNoteIsCountedAgainstTheWordLimit(t *testing.T) {
	_, res, ok := findingMarkdown(strings.Repeat("word ", 390), 2, nil)
	if ok || errorCode(t, res) != codeTooLong {
		t.Fatalf("390 words with a 34 word note: ok %v, res %#v", ok, res.Content)
	}
	if got, _, ok := findingMarkdown(strings.Repeat("word ", 300), 2, nil); !ok || !strings.Contains(got, "Reading stopped early") {
		t.Fatalf("300 words with the note: ok %v, got %q", ok, got)
	}
}

func TestTheSuggestedChangePropertyIsOmittedWhenNoneIsNeededAndNotRepeatedInMarkdown(t *testing.T) {
	schema := string(toolsByName(t)[nameSaveFinding].InputSchema)
	for _, want := range []string{"omitted when none is needed", "never repeated in markdown"} {
		if !strings.Contains(schema, want) {
			t.Errorf("save_finding schema lacks %q: %s", want, schema)
		}
	}
}

func TestTheFindingToolNamesNoCallLimitThatIsOnlyRightForSomeRuns(t *testing.T) {
	tool := toolsByName(t)[nameSaveFinding]
	for _, word := range []string{"six", "twelve", "6 ", "12 "} {
		if strings.Contains(tool.Description, word) {
			t.Errorf("save_finding names a call limit %q, but a build run has six and an investigation twelve: %s", word, tool.Description)
		}
	}
}

func (r *findingRig) homeRun(t *testing.T, by auth.Principal) (Run, dbsqlc.AgentRequest) {
	t.Helper()
	byID, err := db.ParseUUID(by.ID)
	if err != nil {
		t.Fatal(err)
	}
	row, err := r.db.Queries().EnqueueObserveRequest(reading(t.Context()), dbsqlc.EnqueueObserveRequestParams{
		Prompt: "change the checkout dashboard", RequestedByKind: string(by.Kind), RequestedByID: byID, RequestedByName: by.Name,
	})
	if err != nil {
		t.Fatalf("enqueue home run: %v", err)
	}
	return Run{Mode: ModeCreate, Prompt: row.Prompt, RequestID: row.ID.String(), Build: true}, row
}

func (r *findingRig) boardOf(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	board, _, err := r.dashboards.Create(reading(t.Context()), adminCaller, pgtype.UUID{}, []byte(boardSpec(title)), "create")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(board.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func askOnBody(markdown, dashboardID string) string {
	return `{"markdown":` + strconv.Quote(markdown) + `,"ask_on_dashboard_id":` + strconv.Quote(dashboardID) + `}`
}

func TestAHomeRunsFindingCarriesTheDashboardItResolved(t *testing.T) {
	r := newFindingRig(t)
	board := r.boardOf(t, "Checkout")
	run, row := r.homeRun(t, adminCaller)
	res := r.callIn(t, &run, adminCaller, nameSaveFinding, askOnBody("Checkout is the dashboard you mean; changing it is asked for there.", board.String()))
	if _, ok := res.Content.(findingSavedOut); res.IsError || !ok {
		t.Fatalf("save: %#v", res)
	}
	f, found, err := r.findings.ForRequest(reading(t.Context()), adminCaller, row.ID)
	if err != nil || !found {
		t.Fatalf("finding: found %v, %v", found, err)
	}
	if f.AskOnID != board.String() || f.AskOnTitle != "Checkout" {
		t.Errorf("ask on = %q %q, want %s Checkout", f.AskOnID, f.AskOnTitle, board.String())
	}
}

func TestAnAskTargetTheCallerCannotViewIsNotFoundAndSavesNothing(t *testing.T) {
	r := newFindingRig(t)
	board := r.boardOf(t, "Payroll")
	foldertest.Move(t, r.db, board, foldertest.Restricted(t, r.db, "Payroll", "payroll-team"))
	run, _ := r.homeRun(t, editorCaller)
	res := r.callIn(t, &run, editorCaller, nameSaveFinding, askOnBody("Payroll is the dashboard.", board.String()))
	if !res.IsError || !strings.Contains(fmt.Sprint(res.Content), "dashboard not found") {
		t.Fatalf("save: %#v", res)
	}
	var rows int
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT count(*) FROM findings`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("findings left behind: %d, %v", rows, err)
	}
}

func TestAnAskTargetThatIsNotAnIDIsRefused(t *testing.T) {
	r := newFindingRig(t)
	run, _ := r.homeRun(t, adminCaller)
	res := r.callIn(t, &run, adminCaller, nameSaveFinding, askOnBody("The checkout dashboard.", "Checkout"))
	if !res.IsError || !strings.Contains(fmt.Sprint(res.Content), "ask_on_dashboard_id") {
		t.Fatalf("save: %#v", res)
	}
}

func TestAnAskTargetOnlyBelongsToARunWithoutADashboard(t *testing.T) {
	r := newFindingRig(t)
	board := r.boardOf(t, "Checkout")
	other := r.boardOf(t, "Search")
	revise, _ := r.answerRun(t, adminCaller, board)
	investigate := investigateRun(findingWindowTo.Add(-time.Hour), findingWindowTo)
	for name, run := range map[string]Run{"a revision": revise, "an investigation": investigate} {
		res := r.callIn(t, &run, adminCaller, nameSaveFinding, askOnBody("Search is the dashboard.", other.String()))
		if !res.IsError || !strings.Contains(fmt.Sprint(res.Content), "ask_on_dashboard_id") {
			t.Errorf("%s: save: %#v", name, res)
		}
	}
}

func TestAFindingOfAnAsOfRunSaysItsNowAndEndsItsDefaultWindowThere(t *testing.T) {
	r := newFindingRig(t)
	at := time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
	for _, asOf := range []time.Time{at, {}} {
		run := investigateRun(time.Time{}, time.Time{})
		run.AsOf = asOf
		res := r.callIn(t, &run, editorCaller, nameSaveFinding, findingBody("checkout-0 throws 5xx since 09:12"))
		saved, ok := res.Content.(findingSavedOut)
		if res.IsError || !ok {
			t.Fatalf("save: %#v", res.Content)
		}
		id, err := db.ParseUUID(saved.FindingID)
		if err != nil {
			t.Fatal(err)
		}
		f, err := r.findings.Get(reading(t.Context()), editorCaller, id)
		if err != nil {
			t.Fatal(err)
		}
		if asOf.IsZero() {
			if f.AsOf != nil {
				t.Errorf("a live run's finding says as_of %s", f.AsOf)
			}
			continue
		}
		if f.AsOf == nil || !f.AsOf.Equal(at) || !f.To.Equal(at) || !f.From.Equal(at.Add(-defaultWindow)) {
			t.Errorf("finding as of %v covers %s to %s, want the default window ending at %s", f.AsOf, f.From, f.To, at)
		}
	}
}

func TestWithoutSuggestedChange(t *testing.T) {
	fenced := "Errors doubled.\n\n```promql\n# suggested_change: none\nsum(rate(errors[5m]))\n```\n\nSee the panel."
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no label", "Errors doubled.\n\nSee the panel.", "Errors doubled.\n\nSee the panel."},
		{"trailing label line", "Errors doubled.\n\nSuggested change: Add a p99 panel.", "Errors doubled."},
		{"bold label then text on the next lines", "Errors doubled.\n\n**Suggested change:**\n\nAdd a p99 panel.", "Errors doubled."},
		{"italic label", "Errors doubled.\n\n_Suggested change:_ Add a p99 panel.", "Errors doubled."},
		{"plural label", "Errors doubled.\n\nSuggested changes:\n- Add a p99 panel.\n- Split by pod.", "Errors doubled."},
		{"lower case label", "Errors doubled.\nsuggested change: Add a p99 panel.", "Errors doubled."},
		{"field name as the label", "Errors doubled.\nsuggested_change: Add a p99 panel.", "Errors doubled."},
		{"trailing blank lines after the label", "Errors doubled.\nSuggested change: Add a p99 panel.\n\n\n", "Errors doubled."},
		{"leading indentation kept", "  Errors doubled.\n\n    indented line\n\nSuggested change: Add a p99 panel.", "  Errors doubled.\n\n    indented line"},
		{"a fenced block naming the field survives", fenced, fenced},
		{"a label after a fenced block is removed", fenced + "\n\nSuggested change: Add a p99 panel.", fenced},
		{"a tilde fence is skipped too", "Errors doubled.\n~~~\nSuggested change: x\n~~~", "Errors doubled.\n~~~\nSuggested change: x\n~~~"},
		{"the field name inside a sentence stays", "The change is in suggested_change.\nSee the panel.", "The change is in suggested_change.\nSee the panel."},
		{"the phrase inside a sentence stays", "Errors doubled. A suggested change: none was needed.\nSee the panel.", "Errors doubled. A suggested change: none was needed.\nSee the panel."},
		{"a label that is all there is keeps the original", "Suggested change: Add a p99 panel.", "Suggested change: Add a p99 panel."},
		{"nothing dropped keeps trailing blank lines", "Errors doubled.\n\n", "Errors doubled.\n\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withoutSuggestedChange(c.in); got != c.want {
				t.Errorf("withoutSuggestedChange(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMarkdownWithoutSuggestedChangeOnlyWhenTheFieldIsSet(t *testing.T) {
	in := "Errors doubled.\n\nSuggested change: Add a p99 panel."
	if got := markdownWithoutSuggestedChange(in, ""); got != in {
		t.Errorf("field unset: got %q, want the original", got)
	}
	if got := markdownWithoutSuggestedChange(in, "Add a p99 panel."); got != "Errors doubled." {
		t.Errorf("field set: got %q", got)
	}
}
