package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	createdOut, ok := res.Content.(findingCreatedOut)
	created := createdOut.FindingJSON
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

func TestCreateFindingFilesInAFolderTheCallerMayWriteTo(t *testing.T) {
	r := newFindingRig(t)
	open, err := r.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Incidents"})
	if err != nil {
		t.Fatal(err)
	}
	folderID := open.ID.String()
	res := r.callIn(t, nil, editorCaller, nameCreateFinding, `{"markdown":"disk is full","folder_id":"`+folderID+`"}`)
	createdOut, ok := res.Content.(findingCreatedOut)
	created := createdOut.FindingJSON
	if res.IsError || !ok || createdOut.Notice != noticeCreateFinding || created.ObservationID != created.ID || created.FolderID == nil || *created.FolderID != folderID || created.DashboardID != nil {
		t.Fatalf("filed finding: %#v", res.Content)
	}
	res = r.callIn(t, nil, editorCaller, nameListFindings, `{"folder_id":"`+folderID+`"}`)
	list, ok := res.Content.(findingListOut)
	if res.IsError || !ok || len(list.Findings) != 1 || list.Findings[0].ID != created.ID || list.Findings[0].FolderID == nil || *list.Findings[0].FolderID != folderID {
		t.Fatalf("folder list: %#v", res.Content)
	}
	res = r.callIn(t, nil, editorCaller, nameListFindings, `{}`)
	if all, ok := res.Content.(findingListOut); res.IsError || !ok || len(all.Findings) != 1 {
		t.Fatalf("own list: %#v", res.Content)
	}
}

func TestCreateFindingRefusesWhatTheFolderCannotTake(t *testing.T) {
	r := newFindingRig(t)
	board, _, err := r.dashboards.Create(t.Context(), adminCaller, pgtype.UUID{}, []byte(boardSpec("Checkout")), "create")
	if err != nil {
		t.Fatal(err)
	}
	restricted := foldertest.Restricted(t, r.db, hiddenFolder, privateGroup)
	missing := "99999999-9999-4999-8999-999999999999"
	cases := []struct {
		name  string
		by    auth.Principal
		input string
		code  string
	}{
		{"a dashboard and a folder", editorCaller, `{"markdown":"x","dashboard_id":"` + board.ID + `","folder_id":"` + missing + `"}`, codeInvalidRequest},
		{"a folder id that is not a uuid", editorCaller, `{"markdown":"x","folder_id":"nope"}`, codeInvalidRequest},
		{"an unknown folder", editorCaller, `{"markdown":"x","folder_id":"` + missing + `"}`, codeNotFound},
		{"a folder the caller cannot see", editorCaller, `{"markdown":"x","folder_id":"` + restricted.String() + `"}`, codeNotFound},
	}
	for _, c := range cases {
		if res := r.callIn(t, nil, c.by, nameCreateFinding, c.input); errorCode(t, res) != c.code {
			t.Errorf("%s: %#v, want %s", c.name, res.Content, c.code)
		}
	}
	if res := r.callIn(t, nil, editorCaller, nameListFindings, `{"folder_id":"`+missing+`"}`); errorCode(t, res) != codeNotFound {
		t.Errorf("listing an unknown folder: %#v", res.Content)
	}
	if res := r.callIn(t, nil, editorCaller, nameListFindings, `{"folder_id":"`+missing+`","dashboard_id":"`+board.ID+`"}`); errorCode(t, res) != codeInvalidRequest {
		t.Errorf("listing by dashboard and folder: %#v", res.Content)
	}
}

func TestMoveFindingSharesAFindingWithAnotherFolder(t *testing.T) {
	r := newFindingRig(t)
	board, _, err := r.dashboards.Create(t.Context(), adminCaller, pgtype.UUID{}, []byte(boardSpec("Checkout")), "create")
	if err != nil {
		t.Fatal(err)
	}
	onBoard := r.callIn(t, nil, adminCaller, nameCreateFinding, `{"markdown":"on a board","dashboard_id":"`+board.ID+`"}`).Content.(findingCreatedOut).FindingJSON
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', 'eve', 'eve@example.com', 'Eve')`, editorCaller.ID)
	filed := r.callIn(t, nil, editorCaller, nameCreateFinding, `{"markdown":"disk is full"}`).Content.(findingCreatedOut).FindingJSON
	target, err := r.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Payments"})
	if err != nil {
		t.Fatal(err)
	}
	elsewhere, err := r.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Platform"})
	if err != nil {
		t.Fatal(err)
	}
	shared := r.callIn(t, nil, adminCaller, nameCreateFinding, `{"markdown":"shared","folder_id":"`+target.ID.String()+`"}`).Content.(findingCreatedOut).FindingJSON
	var personal string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM folders WHERE personal AND owner_id = $1::uuid`, editorCaller.ID).Scan(&personal); err != nil {
		t.Fatal(err)
	}
	missing := "99999999-9999-4999-8999-999999999999"
	move := func(by auth.Principal, run *Run, finding, folder string) Result {
		return r.callIn(t, run, by, nameMoveFinding, `{"finding_id":"`+finding+`","folder_id":"`+folder+`"}`)
	}

	cases := []struct {
		name   string
		result Result
		code   string
	}{
		{"a finding on a dashboard", move(adminCaller, nil, onBoard.ID, target.ID.String()), codeInvalidRequest},
		{"an unknown finding", move(editorCaller, nil, missing, target.ID.String()), codeNotFound},
		{"an unknown folder", move(editorCaller, nil, filed.ID, missing), codeNotFound},
		{"My Dashboards", move(editorCaller, nil, filed.ID, personal), codeInvalidRequest},
		{"a Viewer on a finding it cannot edit", move(viewerCaller, nil, shared.ID, elsewhere.ID.String()), codeForbiddenRole},
		{"a Commenter on a finding it cannot edit", move(commenterCaller, nil, shared.ID, elsewhere.ID.String()), codeForbiddenRole},
		{"a finding id that is not a uuid", move(editorCaller, nil, "nope", target.ID.String()), codeInvalidRequest},
		{"a folder id that is not a uuid", move(editorCaller, nil, filed.ID, "nope"), codeInvalidRequest},
		{"inside a run", move(editorCaller, &Run{Mode: ModeCreate}, filed.ID, target.ID.String()), codeInvalidRequest},
	}
	for _, c := range cases {
		if got := errorCode(t, c.result); got != c.code {
			t.Errorf("%s: %#v, want %s", c.name, c.result.Content, c.code)
		}
	}
	if body := cases[len(cases)-1].result.Content.(errorBody); body.Message != "move_finding is for MCP clients; a run saves only dashboards or a finding" {
		t.Errorf("in-run message = %q", body.Message)
	}

	res := move(editorCaller, nil, filed.ID, target.ID.String())
	out, ok := res.Content.(findingMovedOut)
	if res.IsError || !ok || out.ID != filed.ID || out.Notice != noticeMoveFinding || out.Folder != "Payments" || out.FolderID == nil || *out.FolderID != target.ID.String() {
		t.Fatalf("move: %#v", res.Content)
	}
	if again := move(editorCaller, nil, filed.ID, target.ID.String()); again.IsError {
		t.Errorf("moving into the folder it is in: %#v", again.Content)
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

func TestSaveFindingInAnAnswerRunOnADashboardSitsInTheDashboardsFolder(t *testing.T) {
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
	after, err := r.db.Queries().GetAgentRequestByID(t.Context(), row.ID)
	if err != nil || !after.ObservationID.Valid {
		t.Fatalf("run observation %v, %v", after.ObservationID, err)
	}
	if saved.URL != "/o/"+after.ObservationID.String() {
		t.Errorf("url = %q", saved.URL)
	}
	o, err := r.observations.Get(t.Context(), adminCaller, after.ObservationID)
	if err != nil || o.FolderID != folder.String() {
		t.Fatalf("observation folder %q, %v, want %s", o.FolderID, err, folder)
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

func TestSaveFindingInARunWithoutAnObservationCreatesIt(t *testing.T) {
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
	if err != nil || !after.ObservationID.Valid || saved.URL != "/o/"+after.ObservationID.String() {
		t.Errorf("run observation %v url %q, %v", after.ObservationID, saved.URL, err)
	}
}

func TestCreateFindingTellsTheCallerItIsDeprecated(t *testing.T) {
	r := newFindingRig(t)
	res := r.callIn(t, nil, editorCaller, nameCreateFinding, `{"markdown":"disk is full"}`)
	created, ok := res.Content.(findingCreatedOut)
	if res.IsError || !ok || created.Notice != noticeCreateFinding || created.ObservationID != created.ID || created.URL != "/o/"+created.ObservationID {
		t.Fatalf("create: %#v", res.Content)
	}
	list, ok := r.callIn(t, nil, editorCaller, nameListFindings, `{}`).Content.(findingListOut)
	if !ok || list.Notice != noticeListFindings || len(list.Findings) != 1 || list.Findings[0].ObservationID != created.ObservationID {
		t.Fatalf("list: %#v", list)
	}
}
