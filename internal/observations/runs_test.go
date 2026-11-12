package observations_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
)

const evidencePanel = `{"type":"timeseries","title":"Rate","datasource":"prom","queries":[{"ref":"A","query":"sum(rate(http_requests_total[5m]))"}]}`

func (r rig) dashboardIn(t *testing.T, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	d, err := r.db.Queries().CreateDashboard(t.Context(), dbsqlc.CreateDashboardParams{
		Title: "Checkout", Tags: []string{}, CreatedByKind: string(editor.Kind), CreatedByID: mustUUID(t, editor.ID), CreatedByName: editor.Name,
		CreatedVia: "direct", FolderID: folder,
	})
	if err != nil {
		t.Fatalf("create dashboard: %v", err)
	}
	return d.ID
}

func (r rig) run(t *testing.T, by auth.Principal, dashboard pgtype.UUID, prompt string, from, to *time.Time) dbsqlc.AgentRequest {
	t.Helper()
	params := dbsqlc.EnqueueInvestigateRequestParams{
		DashboardID: dashboard, Prompt: prompt,
		RequestedByKind: string(by.Kind), RequestedByID: mustUUID(t, by.ID), RequestedByName: by.Name,
	}
	if from != nil {
		params.ContextFrom = pgtype.Timestamptz{Time: *from, Valid: true}
	}
	if to != nil {
		params.ContextTo = pgtype.Timestamptz{Time: *to, Valid: true}
	}
	row, err := r.db.Queries().EnqueueInvestigateRequest(t.Context(), params)
	if err != nil {
		t.Fatalf("enqueue investigation: %v", err)
	}
	return row
}

func (r rig) openForRun(t *testing.T, in observations.RunOpen) (pgtype.UUID, error) {
	t.Helper()
	var id pgtype.UUID
	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		var err error
		id, err = r.service.OpenForRun(t.Context(), q, in)
		return err
	})
	return id, err
}

func (r rig) mustOpenForRun(t *testing.T, in observations.RunOpen) observations.Observation {
	t.Helper()
	id, err := r.openForRun(t, in)
	if err != nil {
		t.Fatalf("open for run: %v", err)
	}
	if !id.Valid {
		t.Fatal("no observation was opened")
	}
	o, err := r.service.Get(t.Context(), in.By, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return o
}

func (r rig) link(t *testing.T, run dbsqlc.AgentRequest, o observations.Observation) {
	t.Helper()
	r.db.Exec(t, `UPDATE agent_requests SET observation_id = $2::uuid WHERE id = $1::uuid`, run.ID.String(), o.ID)
}

func runBlock(t *testing.T, o observations.Observation) observations.Block {
	t.Helper()
	for _, b := range o.Blocks {
		if b.Kind == observations.KindRun {
			return b
		}
	}
	t.Fatalf("no run block in %+v", o.Blocks)
	return observations.Block{}
}

func cites(t *testing.T, b observations.Block) []string {
	t.Helper()
	var body struct {
		RunID string   `json:"run_id"`
		Cites []string `json:"cites"`
	}
	if err := json.Unmarshal(b.Body, &body); err != nil {
		t.Fatal(err)
	}
	return body.Cites
}

func TestAnEditorsInvestigationOpensAnObservationInTheDashboardsFolder(t *testing.T) {
	r := newRig(t)
	dashboard := r.dashboardIn(t, r.shared)
	from, to := windowTo.Add(-2*time.Hour), windowTo
	run := r.run(t, editor, dashboard, "Why is checkout slow?\nDetails follow", &from, &to)
	o := r.mustOpenForRun(t, observations.RunOpen{Run: run, By: editor, Strict: true})
	if o.FolderID != r.shared.String() || o.Title != "Why is checkout slow?" || o.State != observations.StateOpen {
		t.Errorf("observation %+v", o)
	}
	if o.From == nil || o.To == nil || !o.From.Equal(from) || !o.To.Equal(to) {
		t.Errorf("range %v to %v", o.From, o.To)
	}
	if o.Actor.ID != editor.ID || o.RunID != run.ID.String() {
		t.Errorf("actor %+v run %q", o.Actor, o.RunID)
	}
	if len(o.Blocks) != 1 {
		t.Fatalf("blocks %+v", o.Blocks)
	}
	b := runBlock(t, o)
	if b.State != observations.BlockLive || b.Position != 0 || b.RunID != run.ID.String() || len(cites(t, b)) != 0 {
		t.Errorf("run block %+v", b)
	}
	var body map[string]any
	if err := json.Unmarshal(b.Body, &body); err != nil || body["run_id"] != run.ID.String() {
		t.Errorf("run block body %s", b.Body)
	}
	if got, want := r.activityTypes(t, o), []string{"observation.created", "observation.block_added"}; !slices.Equal(got, want) {
		t.Errorf("events %v, want %v", got, want)
	}
}

func TestAnInvestigationTitleComesFromThePromptsFirstLine(t *testing.T) {
	r := newRig(t)
	noRange := r.run(t, editor, r.dashboardIn(t, r.shared), "body only", nil, nil)
	o := r.mustOpenForRun(t, observations.RunOpen{Run: noRange, By: editor, Strict: true})
	if o.From != nil || o.To != nil || o.Title != "body only" {
		t.Errorf("observation %+v", o)
	}
	long := r.run(t, editor, r.dashboardIn(t, r.shared), strings.Repeat("é", 300), nil, nil)
	if got := r.mustOpenForRun(t, observations.RunOpen{Run: long, By: editor, Strict: true}).Title; len([]rune(got)) != observations.MaxTitleRunes {
		t.Errorf("title of %d characters", len([]rune(got)))
	}
	blank := r.run(t, editor, r.dashboardIn(t, r.shared), "\n\n\t", nil, nil)
	if got := r.mustOpenForRun(t, observations.RunOpen{Run: blank, By: editor, Strict: true}).Title; got != "Investigation" {
		t.Errorf("title %q", got)
	}
}

func TestACommenterOnTheDashboardGetsAnObservationInMyDashboards(t *testing.T) {
	r := newRig(t)
	run := r.run(t, commenter, r.dashboardIn(t, r.shared), "Why?", nil, nil)
	o := r.mustOpenForRun(t, observations.RunOpen{Run: run, By: commenter, Strict: true})
	if o.FolderID != r.personalFolder(t, commenter).String() || !o.Folder.Personal {
		t.Errorf("folder %q personal %v", o.FolderID, o.Folder.Personal)
	}
	if b := runBlock(t, o); b.State != observations.BlockLive {
		t.Errorf("a person's own folder takes live blocks, got %s", b.State)
	}
}

func TestAKeyThatCannotEditAnyFolderGetsNoObservation(t *testing.T) {
	r := newRig(t)
	viewerKey := key
	viewerKey.Role = auth.RoleViewer
	run := r.run(t, viewerKey, r.dashboardIn(t, r.shared), "Why?", nil, nil)
	id, err := r.openForRun(t, observations.RunOpen{Run: run, By: viewerKey, Strict: true})
	if err != nil || id.Valid {
		t.Errorf("a key that cannot edit General got %v, %v", id, err)
	}
	var count int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM observations`).Scan(&count); err != nil || count != 0 {
		t.Errorf("observations: %d, %v", count, err)
	}

	noDashboard := r.run(t, key, pgtype.UUID{}, "Why?", nil, nil)
	o := r.mustOpenForRun(t, observations.RunOpen{Run: noDashboard, By: key, Strict: true})
	if !o.Folder.General {
		t.Errorf("an editor key files in General, got %+v", o.Folder)
	}
}

func TestANamedObservationMustBeVisibleAndCommentableWhenStrict(t *testing.T) {
	r := newRig(t)
	secret := r.create(t, admin, r.hiddenFolder(t))
	open := r.create(t, editor, r.shared)
	run := r.run(t, viewer, pgtype.UUID{}, "Why?", nil, nil)
	cases := []struct {
		name string
		by   auth.Principal
		id   string
		want error
	}{
		{"an observation the caller cannot see", otherEditor, secret.ID, observations.ErrNotFound},
		{"an observation that does not exist", editor, "00000000-0000-4000-8000-000000000000", observations.ErrNotFound},
		{"an observation the caller may only view", viewer, open.ID, folders.ErrForbidden},
	}
	for _, c := range cases {
		_, err := r.openForRun(t, observations.RunOpen{Run: run, By: c.by, ObservationID: mustUUID(t, c.id), Strict: true})
		if !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
}

func TestANamedObservationThatCannotBeUsedFallsBackToANewOneWhenNotStrict(t *testing.T) {
	r := newRig(t)
	secret := r.create(t, admin, r.hiddenFolder(t))
	open := r.create(t, editor, r.shared)
	for name, id := range map[string]string{"an invisible one": secret.ID, "one the caller may only view": open.ID} {
		run := r.run(t, viewer, pgtype.UUID{}, "Why?", nil, nil)
		o := r.mustOpenForRun(t, observations.RunOpen{Run: run, By: viewer, ObservationID: mustUUID(t, id)})
		if o.ID == secret.ID || o.ID == open.ID || o.FolderID != r.personalFolder(t, viewer).String() {
			t.Errorf("%s: got %q in %q", name, o.ID, o.FolderID)
		}
	}
}

func TestARunBlockJoinsTheNamedObservationAtTheEnd(t *testing.T) {
	r := newRig(t)
	named := r.create(t, editor, r.shared)
	r.addText(t, editor, named, "first")
	run := r.run(t, otherEditor, pgtype.UUID{}, "Why?", nil, nil)
	o := r.mustOpenForRun(t, observations.RunOpen{Run: run, By: otherEditor, ObservationID: mustUUID(t, named.ID), Strict: true})
	if o.ID != named.ID || len(o.Blocks) != 2 || o.Blocks[1].Kind != observations.KindRun || o.Blocks[1].State != observations.BlockLive {
		t.Errorf("observation %+v", o)
	}

	asked := r.run(t, commenter, pgtype.UUID{}, "Why?", nil, nil)
	o = r.mustOpenForRun(t, observations.RunOpen{Run: asked, By: commenter, ObservationID: mustUUID(t, named.ID), Strict: true})
	if len(o.Blocks) != 3 || o.Blocks[2].State != observations.BlockRequested || o.Blocks[2].RunID != asked.ID.String() {
		t.Errorf("a commenter's run block is requested, got %+v", o.Blocks)
	}
}

func TestAFullObservationTakesNoRunBlockAndNoError(t *testing.T) {
	r := newRig(t)
	full := r.create(t, editor, r.shared)
	for range observations.MaxBlocks {
		r.addText(t, editor, full, "x")
	}
	run := r.run(t, editor, pgtype.UUID{}, "Why?", nil, nil)
	id, err := r.openForRun(t, observations.RunOpen{Run: run, By: editor, ObservationID: mustUUID(t, full.ID), Strict: true})
	if err != nil || id.Valid {
		t.Errorf("got %v, %v", id, err)
	}
}

func recordInput(run dbsqlc.AgentRequest, panels ...string) observations.RunResult {
	evidence := make([]json.RawMessage, len(panels))
	for i, p := range panels {
		evidence[i] = json.RawMessage(p)
	}
	return observations.RunResult{
		RunID: run.ID, Title: "Checkout 5xx", Markdown: "A deploy raised errors.", Evidence: evidence,
		From: windowTo.Add(-time.Hour), To: windowTo,
	}
}

func countOf(items []string, want string) int {
	n := 0
	for _, item := range items {
		if item == want {
			n++
		}
	}
	return n
}

func TestRecordingARunsResultWritesTextAndPanelsAndCitesThem(t *testing.T) {
	r := newRig(t)
	run := r.run(t, editor, r.dashboardIn(t, r.shared), "Why?", nil, nil)
	o := r.mustOpenForRun(t, observations.RunOpen{Run: run, By: editor, Strict: true})
	r.link(t, run, o)
	if err := r.service.RecordRunResult(t.Context(), editor, recordInput(run, evidencePanel, evidencePanel)); err != nil {
		t.Fatal(err)
	}
	got, err := r.service.Get(t.Context(), editor, mustUUID(t, o.ID))
	if err != nil {
		t.Fatal(err)
	}
	kinds := make([]observations.Kind, len(got.Blocks))
	for i, b := range got.Blocks {
		kinds[i] = b.Kind
	}
	if want := []observations.Kind{observations.KindRun, observations.KindText, observations.KindPanel, observations.KindPanel}; !slices.Equal(kinds, want) {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	var text struct {
		Markdown string `json:"markdown"`
	}
	if err := json.Unmarshal(got.Blocks[1].Body, &text); err != nil || text.Markdown != "## Checkout 5xx\n\nA deploy raised errors." {
		t.Errorf("text %q, %v", text.Markdown, err)
	}
	var panel observations.PanelBody
	if err := json.Unmarshal(got.Blocks[2].Body, &panel); err != nil || panel.From == nil || !panel.From.Equal(windowTo.Add(-time.Hour)) || !panel.To.Equal(windowTo) {
		t.Errorf("panel %s, %v", got.Blocks[2].Body, err)
	}
	for _, b := range got.Blocks[1:] {
		if b.State != observations.BlockLive || b.RunID != run.ID.String() {
			t.Errorf("block %+v", b)
		}
	}
	want := []string{got.Blocks[1].ID, got.Blocks[2].ID, got.Blocks[3].ID}
	if cited := cites(t, got.Blocks[0]); !slices.Equal(cited, want) {
		t.Errorf("cites %v, want %v", cited, want)
	}
	if got.Blocks[0].Version != 2 {
		t.Errorf("run block version %d", got.Blocks[0].Version)
	}
	events := r.activityTypes(t, got)
	if countOf(events, "observation.block_added") != 4 || countOf(events, "observation.block_updated") != 1 {
		t.Errorf("events %v", events)
	}
}

func TestRecordingARunsResultTwiceWritesOnce(t *testing.T) {
	r := newRig(t)
	run := r.run(t, editor, r.dashboardIn(t, r.shared), "Why?", nil, nil)
	o := r.mustOpenForRun(t, observations.RunOpen{Run: run, By: editor, Strict: true})
	r.link(t, run, o)
	for range 2 {
		if err := r.service.RecordRunResult(t.Context(), editor, recordInput(run, evidencePanel)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.service.Get(t.Context(), editor, mustUUID(t, o.ID))
	if err != nil || len(got.Blocks) != 3 {
		t.Errorf("blocks %d, %v", len(got.Blocks), err)
	}
}

func TestARunWithoutAnObservationRecordsNothing(t *testing.T) {
	r := newRig(t)
	run := r.run(t, editor, pgtype.UUID{}, "Why?", nil, nil)
	if err := r.service.RecordRunResult(t.Context(), editor, recordInput(run, evidencePanel)); err != nil {
		t.Errorf("a run with no observation: %v", err)
	}
	unknown := observations.RunResult{RunID: mustUUID(t, "00000000-0000-4000-8000-000000000000"), Title: "t", Markdown: "m"}
	if err := r.service.RecordRunResult(t.Context(), editor, unknown); err != nil {
		t.Errorf("an unknown run: %v", err)
	}
}

func TestARequesterWhoLostEditGetsRequestedBlocks(t *testing.T) {
	r := newRig(t)
	run := r.run(t, editor, r.dashboardIn(t, r.shared), "Why?", nil, nil)
	o := r.mustOpenForRun(t, observations.RunOpen{Run: run, By: editor, Strict: true})
	r.link(t, run, o)
	demoted := editor
	demoted.Role = auth.RoleCommenter
	if err := r.service.RecordRunResult(t.Context(), demoted, recordInput(run, evidencePanel)); err != nil {
		t.Fatal(err)
	}
	got, err := r.service.Get(t.Context(), editor, mustUUID(t, o.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range got.Blocks[1:] {
		if b.State != observations.BlockRequested {
			t.Errorf("block %s is %s", b.Kind, b.State)
		}
	}
	if len(got.Blocks) != 3 || len(cites(t, got.Blocks[0])) != 2 {
		t.Errorf("blocks %+v", got.Blocks)
	}

	again := r.run(t, editor, r.dashboardIn(t, r.shared), "Again?", nil, nil)
	second := r.mustOpenForRun(t, observations.RunOpen{Run: again, By: editor, Strict: true})
	r.link(t, again, second)
	viewing := editor
	viewing.Role = auth.RoleViewer
	if err := r.service.RecordRunResult(t.Context(), viewing, recordInput(again)); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a requester who can only view: %v", err)
	}
}

func TestAnInvalidEvidencePanelIsSkippedAndTheRestIsKept(t *testing.T) {
	r := newRig(t)
	run := r.run(t, editor, r.dashboardIn(t, r.shared), "Why?", nil, nil)
	o := r.mustOpenForRun(t, observations.RunOpen{Run: run, By: editor, Strict: true})
	r.link(t, run, o)
	notAPanel := `{"type":"text","title":"Notes"}`
	if err := r.service.RecordRunResult(t.Context(), editor, recordInput(run, notAPanel, evidencePanel)); err != nil {
		t.Fatal(err)
	}
	got, err := r.service.Get(t.Context(), editor, mustUUID(t, o.ID))
	if err != nil || len(got.Blocks) != 3 || got.Blocks[2].Kind != observations.KindPanel || len(cites(t, got.Blocks[0])) != 2 {
		t.Errorf("blocks %+v, %v", got.Blocks, err)
	}
}

func TestEvidenceStopsAtThePanelLimit(t *testing.T) {
	r := newRig(t)
	run := r.run(t, editor, r.dashboardIn(t, r.shared), "Why?", nil, nil)
	o := r.mustOpenForRun(t, observations.RunOpen{Run: run, By: editor, Strict: true})
	for range observations.MaxPanelBlocks - 1 {
		r.addPanel(t, editor, o)
	}
	r.link(t, run, o)
	if err := r.service.RecordRunResult(t.Context(), editor, recordInput(run, evidencePanel, evidencePanel, evidencePanel)); err != nil {
		t.Fatal(err)
	}
	got, err := r.service.Get(t.Context(), editor, mustUUID(t, o.ID))
	if err != nil {
		t.Fatal(err)
	}
	panels := 0
	for _, b := range got.Blocks {
		if b.Kind == observations.KindPanel {
			panels++
		}
	}
	if cited := cites(t, runBlock(t, got)); panels != observations.MaxPanelBlocks || len(cited) != 2 {
		t.Errorf("panels %d, cites %v", panels, cited)
	}
}
