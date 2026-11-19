package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
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
	"github.com/slashobserve/slashobserve/internal/logtest"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
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
		{"save_finding outside a run", editor, nameSaveFinding, `{}`, true, "save_finding runs inside the agent; MCP clients use create_observation"},
		{"create_dashboard in an investigation", investigate, nameCreateDashboard, `{}`, true, "an investigate run only reads data and ends with save_finding; it cannot use create_dashboard, so if the request asks for a change, say in the finding that it has to be asked for as a change on the dashboard, not as an investigation"},
		{"update_panels in an investigation", investigate, nameUpdatePanels, `{}`, true, "an investigate run only reads data and ends with save_finding; it cannot use update_panels, so if the request asks for a change, say in the finding that it has to be asked for as a change on the dashboard, not as an investigation"},
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
	if res.IsError || !ok || saved.Code != "saved" || saved.FindingID == "" || saved.Evidence != 2 || saved.URL == "" {
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
	if saved.URL != "/o/"+f.ObservationID {
		t.Errorf("url = %q, want the finding's observation %s", saved.URL, f.ObservationID)
	}
	if len(f.Evidence) != 2 || f.Evidence[0].ID != "evidence-1" || f.Evidence[1].ID != "evidence-2" {
		t.Errorf("evidence = %+v", f.Evidence)
	}
	if !f.From.Equal(from) || !f.To.Equal(findingWindowTo) || f.Title != "why are checkout errors up" || f.DashboardID != "" {
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
	f, err := r.findings.Get(t.Context(), editorCaller, id)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "Checkout 5xx errors by pod" {
		t.Errorf("title = %q", f.Title)
	}
}

func TestSaveFindingShortensThePromptWhenTheAgentGaveNoTitle(t *testing.T) {
	r := newFindingRig(t)
	run := Run{Mode: ModeCreate, Prompt: "@observe why are server errors elevated, and which pod is responsible for them right now?"}
	res := r.callIn(t, &run, editorCaller, nameSaveFinding, `{"markdown":"checkout-0 throws 5xx"}`)
	saved, ok := res.Content.(findingSavedOut)
	if res.IsError || !ok {
		t.Fatalf("save: %#v", res.Content)
	}
	id, err := db.ParseUUID(saved.FindingID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := r.findings.Get(t.Context(), editorCaller, id)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "why are server errors elevated, and which pod is responsible..." {
		t.Errorf("title = %q", f.Title)
	}
}

func (r *findingRig) investigation(t *testing.T, by auth.Principal, from, to time.Time) (Run, dbsqlc.AgentRequest) {
	t.Helper()
	byID, err := db.ParseUUID(by.ID)
	if err != nil {
		t.Fatal(err)
	}
	row, err := r.db.Queries().EnqueueInvestigateRequest(t.Context(), dbsqlc.EnqueueInvestigateRequestParams{
		Prompt: "why are checkout errors up\nsince the deploy", RequestedByKind: string(by.Kind), RequestedByID: byID, RequestedByName: by.Name,
	})
	if err != nil {
		t.Fatalf("enqueue investigation: %v", err)
	}
	err = r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		id, err := r.observations.OpenForRun(t.Context(), q, observations.RunOpen{Run: row, By: by, Strict: true})
		if err != nil {
			return err
		}
		row, err = q.SetRequestObservation(t.Context(), dbsqlc.SetRequestObservationParams{ID: row.ID, ObservationID: id})
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
	o, err := r.observations.Get(t.Context(), editorCaller, row.ObservationID)
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
	if err := json.Unmarshal(o.Blocks[1].Body, &text); err != nil || text.Markdown != "## why are checkout errors up\n\ncheckout-0 throws 5xx since 09:12" {
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
	after, err := r.observations.Get(t.Context(), editorCaller, row.ObservationID)
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
	row, err := r.db.Queries().EnqueueAgentRequest(t.Context(), dbsqlc.EnqueueAgentRequestParams{
		DashboardID: dashboard, Source: "prompt", Prompt: "why is this panel flat", BaseVersion: 1,
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
	board, _, err := r.dashboards.Create(t.Context(), adminCaller, pgtype.UUID{}, []byte(boardSpec("Checkout")), "create")
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := db.ParseUUID(board.ID)
	if err != nil {
		t.Fatal(err)
	}
	created, err := r.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Payments"})
	if err != nil {
		t.Fatal(err)
	}
	folder := created.ID
	foldertest.Move(t, r.db, boardID, folder)
	run, row := r.answerRun(t, adminCaller, boardID)

	res := r.callIn(t, &run, adminCaller, nameSaveFinding, findingBody("checkout-0 throws 5xx", evidencePanel("Errors", "sum(rate(errors[5m]))")))
	saved, ok := res.Content.(findingSavedOut)
	if res.IsError || !ok || saved.Code != "saved" {
		t.Fatalf("save: %#v", res)
	}
	if saved.URL != "/c/"+row.ID.String() {
		t.Errorf("url = %q, want the chat", saved.URL)
	}
	if after, err := r.db.Queries().GetAgentRequestByID(t.Context(), row.ID); err != nil || after.ObservationID.Valid {
		t.Fatalf("run observation %v, %v", after.ObservationID, err)
	}

	res = r.callIn(t, nil, editorCaller, nameSaveRunAnswer, `{"run_id":"`+row.ID.String()+`"}`)
	if body, ok := res.Content.(errorBody); !res.IsError || !ok || body.Code != codeNoAnswer {
		t.Fatalf("another person saves it: %#v", res.Content)
	}
	res = r.callIn(t, nil, adminCaller, nameSaveRunAnswer, `{"run_id":"`+row.ID.String()+`"}`)
	filed, ok := res.Content.(runAnswerSavedOut)
	if res.IsError || !ok || filed.Code != "saved" || filed.Replayed {
		t.Fatalf("save answer: %#v", res.Content)
	}
	after, err := r.db.Queries().GetAgentRequestByID(t.Context(), row.ID)
	if err != nil || !after.ObservationID.Valid || filed.URL != "/o/"+after.ObservationID.String() {
		t.Fatalf("run observation %v, url %q, %v", after.ObservationID, filed.URL, err)
	}
	o, err := r.observations.Get(t.Context(), adminCaller, after.ObservationID)
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
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM findings`).Scan(&findingRows); err != nil || findingRows != 0 {
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
	if res.IsError || !ok || saved.Code != "saved" {
		t.Fatalf("save: %#v", res)
	}
	after, err := r.db.Queries().GetAgentRequestByID(t.Context(), row.ID)
	if err != nil || after.ObservationID.Valid || saved.URL != "/c/"+row.ID.String() {
		t.Errorf("run observation %v url %q, %v", after.ObservationID, saved.URL, err)
	}
}

func TestAFindingSavedAfterRefusedReadsSaysReadingStoppedEarly(t *testing.T) {
	if got, _, ok := findingMarkdown("checkout is slow", 0); !ok || got != "checkout is slow" {
		t.Fatalf("no refused read changed the text: %q", got)
	}
	one, _, _ := findingMarkdown("checkout is slow", 1)
	if !strings.HasPrefix(one, "checkout is slow\n\n_Reading stopped early: 1 read was refused") || !strings.Contains(one, "counts above may be partial") {
		t.Fatalf("one refused read: %q", one)
	}
	if many, _, _ := findingMarkdown("checkout is slow", 3); !strings.Contains(many, "3 reads were refused") {
		t.Fatalf("three refused reads: %q", many)
	}
}

func TestATooLongFindingStepSaysWhatToDoInWords(t *testing.T) {
	_, res, ok := findingMarkdown(strings.Repeat("word ", 401), 0)
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
	_, res, ok := findingMarkdown(strings.Repeat("word ", 390), 2)
	if ok || errorCode(t, res) != codeTooLong {
		t.Fatalf("390 words with a 34 word note: ok %v, res %#v", ok, res.Content)
	}
	if got, _, ok := findingMarkdown(strings.Repeat("word ", 300), 2); !ok || !strings.Contains(got, "Reading stopped early") {
		t.Fatalf("300 words with the note: ok %v, got %q", ok, got)
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
	row, err := r.db.Queries().EnqueueObserveRequest(t.Context(), dbsqlc.EnqueueObserveRequestParams{
		Prompt: "change the checkout dashboard", RequestedByKind: string(by.Kind), RequestedByID: byID, RequestedByName: by.Name,
	})
	if err != nil {
		t.Fatalf("enqueue home run: %v", err)
	}
	return Run{Mode: ModeCreate, Prompt: row.Prompt, RequestID: row.ID.String(), Build: true}, row
}

func (r *findingRig) boardOf(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	board, _, err := r.dashboards.Create(t.Context(), adminCaller, pgtype.UUID{}, []byte(boardSpec(title)), "create")
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
	if saved, ok := res.Content.(findingSavedOut); res.IsError || !ok || saved.Code != "saved" {
		t.Fatalf("save: %#v", res)
	}
	f, found, err := r.findings.ForRequest(t.Context(), adminCaller, row.ID)
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
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM findings`).Scan(&rows); err != nil || rows != 0 {
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
