package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const frozenFrames = `{"frames":[],"meta":{"kind":"range"}}`

type snapshotFreezer struct{}

func (snapshotFreezer) Freeze(context.Context, auth.Principal, *spec.Dashboard, string, time.Time, time.Time) (observations.Frozen, error) {
	return observations.Frozen{Data: json.RawMessage(frozenFrames)}, nil
}

type observationRig struct {
	registry *Registry
	service  *observations.Service
	db       *dbtest.Database
	folder   string
}

func newObservationRig(t *testing.T) *observationRig {
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
	for _, p := range []auth.Principal{adminCaller, editorCaller, commenterCaller, viewerCaller} {
		database.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $2 || '@example.com', $3)`, p.ID, p.Name, p.Name)
	}
	access := folders.NewAccess(database.DB)
	service := observations.New(database.DB, access, snapshotFreezer{}, nil, logger)
	registry := New(Deps{Datasources: datasources, Runner: queryproxy.NewRunner(datasources, logger), Folders: access, Observations: service, Logger: logger})
	r := &observationRig{registry: registry, service: service, db: database}
	if err := database.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title) VALUES ('Incidents') RETURNING id::text`).Scan(&r.folder); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *observationRig) call(t *testing.T, by auth.Principal, tool, input string) Result {
	t.Helper()
	res, err := r.registry.Call(auth.WithPrincipal(t.Context(), by), tool, []byte(input))
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return res
}

func (r *observationRig) written(t *testing.T, by auth.Principal, tool, input string) observationWrittenOut {
	t.Helper()
	res := r.call(t, by, tool, input)
	out, ok := res.Content.(observationWrittenOut)
	if res.IsError || !ok || out.Code != "ok" {
		t.Fatalf("%s %s: %#v", tool, input, res.Content)
	}
	return out
}

func (r *observationRig) view(t *testing.T, by auth.Principal, id string) observationViewOut {
	t.Helper()
	res := r.call(t, by, nameGetObservation, fmt.Sprintf(`{"observation_id":%q}`, id))
	out, ok := res.Content.(observationViewOut)
	if res.IsError || !ok {
		t.Fatalf("get_observation: %#v", res.Content)
	}
	return out
}

func (r *observationRig) start(t *testing.T, by auth.Principal) string {
	t.Helper()
	return r.written(t, by, nameCreateObservation, fmt.Sprintf(`{"title":"Checkout 5xx","folder_id":%q}`, r.folder)).ObservationID
}

func textBlockInput(id, markdown string) string {
	encoded, _ := json.Marshal(markdown)
	return fmt.Sprintf(`{"observation_id":%q,"kind":"text","markdown":%s}`, id, encoded)
}

func panelBlockInput(id, panel string) string {
	return fmt.Sprintf(`{"observation_id":%q,"kind":"panel","panel":%s}`, id, panel)
}

func TestObservationToolsBuildAPageFromCreateToResolve(t *testing.T) {
	r := newObservationRig(t)
	created := r.written(t, editorCaller, nameCreateObservation, fmt.Sprintf(`{"title":"Checkout 5xx","folder_id":%q,"blocks":[{"kind":"text","markdown":"Errors began at 09:12"},{"kind":"panel","panel":%s,"width":"half"}]}`, r.folder, evidencePanel("Errors", "sum(rate(errors[5m]))")))
	if created.State != "open" || !strings.HasSuffix(created.URL, "/o/"+created.ObservationID) {
		t.Fatalf("created = %#v", created)
	}

	page := r.view(t, viewerCaller, created.ObservationID)
	if page.Title != "Checkout 5xx" || len(page.Blocks) != 2 || page.BlocksTruncated {
		t.Fatalf("page = %#v", page)
	}
	text, panel := page.Blocks[0], page.Blocks[1]
	if text.Kind != "text" || text.Markdown != "Errors began at 09:12" || text.State != "live" || text.Position != 0 || text.Version != 1 {
		t.Errorf("text block = %#v", text)
	}
	if panel.Kind != "panel" || panel.Title != "Errors" || panel.Type != "timeseries" || strings.Join(panel.Datasources, ",") != fakeUID || panel.Width != "half" {
		t.Errorf("panel block = %#v", panel)
	}

	added := r.written(t, editorCaller, nameAddBlock, textBlockInput(created.ObservationID, "  Checkout is the only service  "))
	if added.State != "live" || added.Version != 1 || added.BlockID == "" || added.Note != "" {
		t.Fatalf("added = %#v", added)
	}
	updated := r.written(t, editorCaller, nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"version":1,"markdown":"Only checkout fails","position":0}`, added.BlockID))
	if updated.Version != 2 {
		t.Fatalf("updated = %#v", updated)
	}
	if first := r.view(t, editorCaller, created.ObservationID).Blocks[0]; first.ID != added.BlockID || first.Markdown != "Only checkout fails" {
		t.Errorf("first block after the update = %#v", first)
	}

	list := r.call(t, viewerCaller, nameListObservations, `{"state":"open"}`).Content.(observationListOut)
	if len(list.Observations) != 1 || list.Observations[0].ID != created.ObservationID || list.Observations[0].Blocks != nil {
		t.Fatalf("open list = %#v", list)
	}

	resolved := r.written(t, editorCaller, nameResolveObservation, fmt.Sprintf(`{"observation_id":%q,"summary":"A bad deploy of checkout"}`, created.ObservationID))
	if resolved.State != "resolved" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if list := r.call(t, viewerCaller, nameListObservations, `{"state":"open"}`).Content.(observationListOut); len(list.Observations) != 0 {
		t.Errorf("open list after resolve = %#v", list)
	}
	again := r.call(t, editorCaller, nameResolveObservation, fmt.Sprintf(`{"observation_id":%q,"summary":"twice"}`, created.ObservationID))
	body, _ := again.Content.(errorBody)
	if errorCode(t, again) != codeInvalidTransition || !strings.Contains(body.Message, "resolve is not allowed while the observation is resolved") {
		t.Errorf("a second resolve: %#v", again.Content)
	}
}

func TestObservationToolsRefuseHTMLInText(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	block := r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "plain"))
	for name, call := range map[string][2]string{
		"add_block":    {nameAddBlock, textBlockInput(id, "see <script>alert(1)</script>")},
		"update_block": {nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"version":1,"markdown":"<img src=x onerror=y>"}`, block.BlockID)},
		"create":       {nameCreateObservation, `{"title":"x","blocks":[{"kind":"text","markdown":"<b>bold</b>"}]}`},
	} {
		res := r.call(t, editorCaller, call[0], call[1])
		if !res.IsError {
			t.Fatalf("%s accepted HTML: %#v", name, res.Content)
		}
		got := res.Content
		if refused, ok := got.(blockRefusedOut); ok {
			got = refused.Refused
		}
		if body, ok := got.(errorBody); !ok || body.Code != "html_not_allowed" || body.Message != "the agent cannot write HTML; write Markdown without tags" {
			t.Errorf("%s: %#v", name, res.Content)
		}
	}
	if page := r.view(t, editorCaller, id); len(page.Blocks) != 1 || page.Blocks[0].Markdown != "plain" {
		t.Errorf("blocks after the refusals = %#v", page.Blocks)
	}
	if res := r.call(t, editorCaller, nameAddBlock, textBlockInput(id, "a < b and 3 > 2")); res.IsError {
		t.Errorf("a comparison is not HTML: %#v", res.Content)
	}
}

func TestObservationPanelBlocksAreCheckedLikeFindingEvidence(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)

	res := r.call(t, editorCaller, nameAddBlock, panelBlockInput(id, evidencePanel("Nothing", "empty")))
	rejected, ok := res.Content.(invalidSpecOut)
	if !res.IsError || !ok || rejected.Code != codeInvalidSpec || len(rejected.Issues) == 0 || !strings.HasPrefix(rejected.Issues[0].Path, "/panels/0") {
		t.Fatalf("a panel with no data: %#v", res.Content)
	}

	for name, panel := range map[string]string{
		"a text panel":      `{"type":"text","title":"Notes","content":"hi"}`,
		"not an object":     `"timeseries"`,
		"an unknown type":   `{"type":"nope","title":"x"}`,
		"a missing queries": `{"type":"timeseries","title":"x","datasource":"fake"}`,
	} {
		if res := r.call(t, editorCaller, nameAddBlock, panelBlockInput(id, panel)); !res.IsError {
			t.Errorf("%s was accepted: %#v", name, res.Content)
		}
	}

	good := r.written(t, editorCaller, nameAddBlock, panelBlockInput(id, evidencePanel("Errors", "up")))
	change := r.call(t, editorCaller, nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"version":1,"panel":%s}`, good.BlockID, evidencePanel("Nothing", "empty")))
	if _, ok := change.Content.(invalidSpecOut); !change.IsError || !ok {
		t.Errorf("updating to a panel with no data: %#v", change.Content)
	}
	if page := r.view(t, editorCaller, id); len(page.Blocks) != 1 || page.Blocks[0].Title != "Errors" || page.Blocks[0].Version != 1 {
		t.Errorf("blocks after the refusals = %#v", page.Blocks)
	}
	changed := r.written(t, editorCaller, nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"version":1,"panel":%s,"from":"2026-11-01T08:00:00Z","to":"2026-11-01T09:00:00Z"}`, good.BlockID, evidencePanel("Latency", "histogram_quantile(0.99, x)")))
	if page := r.view(t, editorCaller, id); changed.Version != 2 || page.Blocks[0].Title != "Latency" || page.Blocks[0].From != "2026-11-01T08:00:00Z" {
		t.Errorf("blocks after a good update = %#v", page.Blocks)
	}
}

func TestObservationBlocksFromSomeoneWhoCanOnlyCommentAreRequested(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)

	if res := r.call(t, viewerCaller, nameAddBlock, textBlockInput(id, "a viewer's note")); errorCode(t, res) != codeForbiddenRole {
		t.Fatalf("a viewer: %#v", res.Content)
	}
	asked := r.written(t, commenterCaller, nameAddBlock, textBlockInput(id, "Did the deploy touch this?"))
	if asked.State != "requested" || asked.Note != requestedBlockNote {
		t.Fatalf("a commenter's block = %#v", asked)
	}
	if page := r.view(t, editorCaller, id); page.Blocks[0].State != "requested" || strings.Join(page.Blocks[0].AllowedActions, ",") != "update,keep,discard" {
		t.Errorf("an editor's view = %#v", page.Blocks[0])
	}
	if page := r.view(t, commenterCaller, id); strings.Join(page.Blocks[0].AllowedActions, ",") != "discard" {
		t.Errorf("the requester's view = %#v", page.Blocks[0])
	}
	denied := r.call(t, commenterCaller, nameKeepBlock, fmt.Sprintf(`{"block_id":%q,"version":1}`, asked.BlockID))
	if body, _ := denied.Content.(errorBody); errorCode(t, denied) != codeInvalidTransition || !strings.HasSuffix(body.Message, "allowed now: discard") {
		t.Errorf("a commenter keeping: %#v", denied.Content)
	}

	stale := r.call(t, editorCaller, nameKeepBlock, fmt.Sprintf(`{"block_id":%q,"version":7}`, asked.BlockID))
	if body, _ := stale.Content.(errorBody); errorCode(t, stale) != codeVersionConflict || !strings.Contains(body.Message, "apply your change to version 1") {
		t.Errorf("a stale keep: %#v", stale.Content)
	}
	kept := r.written(t, editorCaller, nameKeepBlock, fmt.Sprintf(`{"block_id":%q,"version":1}`, asked.BlockID))
	if kept.State != "live" {
		t.Fatalf("kept = %#v", kept)
	}
	again := r.call(t, editorCaller, nameKeepBlock, fmt.Sprintf(`{"block_id":%q,"version":1}`, asked.BlockID))
	if body, _ := again.Content.(errorBody); errorCode(t, again) != codeInvalidTransition || !strings.Contains(body.Message, "keep is not allowed while the observation_block is live") {
		t.Errorf("keeping a live block: %#v", again.Content)
	}

	second := r.written(t, commenterCaller, nameAddBlock, textBlockInput(id, "And the cache?"))
	discarded := r.written(t, commenterCaller, nameDiscardBlock, fmt.Sprintf(`{"block_id":%q}`, second.BlockID))
	if discarded.State != "discarded" || discarded.BlockID != second.BlockID {
		t.Fatalf("discarded = %#v", discarded)
	}
	if page := r.view(t, editorCaller, id); len(page.Blocks) != 1 {
		t.Errorf("blocks after the discard = %#v", page.Blocks)
	}
	if res := r.call(t, editorCaller, nameDiscardBlock, fmt.Sprintf(`{"block_id":%q}`, second.BlockID)); errorCode(t, res) != codeNotFound {
		t.Errorf("discarding a gone block: %#v", res.Content)
	}
}

func TestObservationUpdatesNeedTheVersionTheCallerRead(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	block := r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "first"))
	r.written(t, editorCaller, nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"version":1,"markdown":"second"}`, block.BlockID))

	stale := r.call(t, editorCaller, nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"version":1,"markdown":"third"}`, block.BlockID))
	if body, _ := stale.Content.(errorBody); errorCode(t, stale) != codeVersionConflict || !strings.Contains(body.Message, "apply your change to version 2") {
		t.Fatalf("a stale update: %#v", stale.Content)
	}
	if page := r.view(t, editorCaller, id); page.Blocks[0].Markdown != "second" {
		t.Errorf("the stale update changed the block: %#v", page.Blocks[0])
	}
}

func TestCreateObservationReportsTheBlockThatStoppedIt(t *testing.T) {
	r := newObservationRig(t)
	res := r.call(t, editorCaller, nameCreateObservation, fmt.Sprintf(`{"title":"Checkout 5xx","folder_id":%q,"blocks":[{"kind":"text","markdown":"fine"},{"kind":"panel","panel":%s},{"kind":"text","markdown":"never added"}]}`, r.folder, evidencePanel("Nothing", "empty")))
	refused, ok := res.Content.(blockRefusedOut)
	if !res.IsError || !ok || refused.Code != codeBlockRefused || refused.BlockIndex != 1 || refused.ObservationID == "" {
		t.Fatalf("a bad second block: %#v", res.Content)
	}
	if inner, ok := refused.Refused.(invalidSpecOut); !ok || len(inner.Issues) == 0 {
		t.Errorf("refused = %#v", refused.Refused)
	}
	page := r.view(t, editorCaller, refused.ObservationID)
	if len(page.Blocks) != 1 || page.Blocks[0].Markdown != "fine" {
		t.Errorf("the created observation = %#v", page)
	}
}

func TestObservationToolsKeepHiddenFoldersHidden(t *testing.T) {
	r := newObservationRig(t)
	hidden := foldertest.Restricted(t, r.db, hiddenFolder, privateGroup).String()
	if res := r.call(t, editorCaller, nameCreateObservation, fmt.Sprintf(`{"title":"Payroll","folder_id":%q}`, hidden)); errorCode(t, res) != codeNotFound {
		t.Errorf("creating in a hidden folder: %#v", res.Content)
	}
	secret := r.written(t, adminCaller, nameCreateObservation, fmt.Sprintf(`{"title":"Payroll is late","folder_id":%q}`, hidden)).ObservationID
	if res := r.call(t, editorCaller, nameGetObservation, fmt.Sprintf(`{"observation_id":%q}`, secret)); errorCode(t, res) != codeNotFound {
		t.Errorf("reading a hidden observation: %#v", res.Content)
	}
	if res := r.call(t, editorCaller, nameAddBlock, textBlockInput(secret, "x")); errorCode(t, res) != codeNotFound {
		t.Errorf("adding to a hidden observation: %#v", res.Content)
	}
	if res := r.call(t, editorCaller, nameListObservations, fmt.Sprintf(`{"folder_id":%q}`, hidden)); errorCode(t, res) != codeNotFound {
		t.Errorf("listing a hidden folder: %#v", res.Content)
	}
	if list := r.call(t, editorCaller, nameListObservations, `{}`).Content.(observationListOut); len(list.Observations) != 0 {
		t.Errorf("an editor's list shows %#v", list)
	}
	if list := r.call(t, adminCaller, nameListObservations, `{}`).Content.(observationListOut); len(list.Observations) != 1 {
		t.Errorf("an admin's list shows %#v", list)
	}
}

func TestObservationSnapshotsFreezeAPanelBlock(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	panel := r.written(t, editorCaller, nameAddBlock, panelBlockInput(id, evidencePanel("Errors", "up")))
	note := r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "a note"))

	snapshot := r.written(t, editorCaller, nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"snapshot","from_block_id":%q,"position":1}`, id, panel.BlockID))
	if snapshot.State != "live" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	page := r.view(t, editorCaller, id)
	if len(page.Blocks) != 3 || page.Blocks[1].Kind != "snapshot" || page.Blocks[1].SnapshotID == "" {
		t.Errorf("blocks = %#v", page.Blocks)
	}
	res := r.call(t, editorCaller, nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"snapshot","from_block_id":%q}`, id, note.BlockID))
	if body, _ := res.Content.(errorBody); errorCode(t, res) != observations.CodeInvalidBlock || !strings.Contains(body.Message, "panel block") {
		t.Errorf("a snapshot of a text block: %#v", res.Content)
	}
}

func TestObservationToolsRefuseMalformedInput(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	block := r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "x")).BlockID
	panel := evidencePanel("Errors", "up")
	cases := []struct {
		name, tool, input, code string
	}{
		{"an observation id that is not a uuid", nameGetObservation, `{"observation_id":"nope"}`, codeInvalidRequest},
		{"an unknown field", nameGetObservation, fmt.Sprintf(`{"observation_id":%q,"limit":1}`, id), codeInvalidRequest},
		{"an unknown kind", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"run"}`, id), codeInvalidRequest},
		{"text without markdown", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"text"}`, id), codeInvalidRequest},
		{"text with a panel", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"text","markdown":"x","panel":%s}`, id, panel), codeInvalidRequest},
		{"a panel with markdown", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"panel","panel":%s,"markdown":"x"}`, id, panel), codeInvalidRequest},
		{"a panel with a snapshot source", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"panel","panel":%s,"from_block_id":%q}`, id, panel, block), codeInvalidRequest},
		{"a snapshot without a source", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"snapshot"}`, id), codeInvalidRequest},
		{"a snapshot with a range", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"snapshot","from_block_id":%q,"from":"2026-11-01T08:00:00Z"}`, id, block), codeInvalidRequest},
		{"a panel without a panel", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"panel"}`, id), codeInvalidRequest},
		{"a panel range with one end", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"panel","panel":%s,"from":"2026-11-01T08:00:00Z"}`, id, panel), observations.CodeInvalidRange},
		{"a panel range that is not a time", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"panel","panel":%s,"from":"now-1h","to":"now"}`, id, panel), codeInvalidRequest},
		{"a reversed panel range", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"panel","panel":%s,"from":"2026-11-01T09:00:00Z","to":"2026-11-01T08:00:00Z"}`, id, panel), observations.CodeInvalidRange},
		{"a panel range over 400 days", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"panel","panel":%s,"from":"2025-01-01T00:00:00Z","to":"2026-11-01T00:00:00Z"}`, id, panel), observations.CodeInvalidRange},
		{"a width that is neither", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"text","markdown":"x","width":"wide"}`, id), observations.CodeInvalidBlock},
		{"markdown with a control character", nameAddBlock, fmt.Sprintf(`{"observation_id":%q,"kind":"text","markdown":"a\u202eb"}`, id), codeInvalidRequest},
		{"markdown over 4000 characters", nameAddBlock, textBlockInput(id, strings.Repeat("x", 4001)), codeInvalidRequest},
		{"a title that is empty", nameCreateObservation, `{"title":"  "}`, observations.CodeInvalidTitle},
		{"a range with one end", nameCreateObservation, `{"title":"x","from":"2026-11-01T08:00:00Z"}`, observations.CodeInvalidRange},
		{"too many blocks", nameCreateObservation, `{"title":"x","blocks":[` + strings.TrimSuffix(strings.Repeat(`{"kind":"text","markdown":"x"},`, 11), ",") + `]}`, codeInvalidRequest},
		{"a folder id that is not a uuid", nameCreateObservation, `{"title":"x","folder_id":"nope"}`, codeInvalidRequest},
		{"an unknown folder", nameCreateObservation, `{"title":"x","folder_id":"99999999-9999-4999-8999-999999999999"}`, codeNotFound},
		{"an update without a version", nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"markdown":"y"}`, block), codeInvalidRequest},
		{"an update with nothing to change", nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"version":1}`, block), codeInvalidRequest},
		{"an update with markdown and a panel", nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"version":1,"markdown":"y","panel":%s}`, block, panel), codeInvalidRequest},
		{"an update of a range with no panel", nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"version":1,"markdown":"y","from":"2026-11-01T08:00:00Z","to":"2026-11-01T09:00:00Z"}`, block), codeInvalidRequest},
		{"a panel on a text block", nameUpdateBlock, fmt.Sprintf(`{"block_id":%q,"version":1,"panel":%s}`, block, panel), codeNotFound},
		{"markdown on an unknown block", nameUpdateBlock, `{"block_id":"99999999-9999-4999-8999-999999999999","version":1,"markdown":"y"}`, codeNotFound},
		{"a keep without a version", nameKeepBlock, fmt.Sprintf(`{"block_id":%q}`, block), codeInvalidRequest},
		{"a discard of a live block", nameDiscardBlock, fmt.Sprintf(`{"block_id":%q}`, block), codeInvalidTransition},
		{"a block id that is not a uuid", nameDiscardBlock, `{"block_id":"nope"}`, codeInvalidRequest},
		{"a resolve with a two line summary", nameResolveObservation, fmt.Sprintf(`{"observation_id":%q,"summary":"a\nb"}`, id), observations.CodeInvalidSummary},
		{"a list state that is neither", nameListObservations, `{"state":"closed"}`, codeInvalidRequest},
		{"a list limit of zero", nameListObservations, `{"limit":0}`, codeInvalidRequest},
		{"a list cursor that is not one", nameListObservations, `{"cursor":"nope"}`, codeInvalidRequest},
	}
	for _, c := range cases {
		if got := errorCodeOf(t, r.call(t, editorCaller, c.tool, c.input)); got != c.code {
			t.Errorf("%s: code %q, want %q", c.name, got, c.code)
		}
	}
}

func TestListObservationsPagesMostRecentlyChangedFirst(t *testing.T) {
	r := newObservationRig(t)
	var ids []string
	for i := range 3 {
		ids = append(ids, r.written(t, editorCaller, nameCreateObservation, fmt.Sprintf(`{"title":"Observation %d","folder_id":%q}`, i, r.folder)).ObservationID)
	}
	first := r.call(t, viewerCaller, nameListObservations, `{"limit":2}`).Content.(observationListOut)
	if len(first.Observations) != 2 || first.NextCursor == nil || first.Observations[0].ID != ids[2] || first.Observations[1].ID != ids[1] {
		t.Fatalf("first page = %#v", first)
	}
	second := r.call(t, viewerCaller, nameListObservations, fmt.Sprintf(`{"limit":2,"cursor":%q}`, *first.NextCursor)).Content.(observationListOut)
	if len(second.Observations) != 1 || second.NextCursor != nil || second.Observations[0].ID != ids[0] {
		t.Fatalf("second page = %#v", second)
	}
}

func TestGetObservationClipsMarkdownAndFitsTheResult(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	long := strings.Repeat("é", 3900)
	for range 12 {
		r.written(t, editorCaller, nameAddBlock, textBlockInput(id, long))
	}
	res := r.call(t, viewerCaller, nameGetObservation, fmt.Sprintf(`{"observation_id":%q}`, id))
	page, ok := res.Content.(observationViewOut)
	if res.IsError || !ok {
		t.Fatalf("get_observation: %#v", res.Content)
	}
	if !page.BlocksTruncated || len(page.Blocks) == 0 || len(page.Blocks) >= 12 {
		t.Errorf("blocks kept = %d, truncated = %v", len(page.Blocks), page.BlocksTruncated)
	}
	for _, b := range page.Blocks {
		if !b.MarkdownTruncated || len([]rune(b.Markdown)) != maxBlockMarkdownOut {
			t.Fatalf("block markdown: %d characters, truncated = %v", len([]rune(b.Markdown)), b.MarkdownTruncated)
		}
	}
	if size := jsonSize(page); size > maxContentBytes {
		t.Errorf("result is %d bytes, over the %d byte limit", size, maxContentBytes)
	}
}

func TestObservationToolsNeedOnlyTheViewerRole(t *testing.T) {
	names := []string{nameCreateObservation, nameGetObservation, nameListObservations, nameAddBlock, nameUpdateBlock, nameResolveObservation, nameKeepBlock, nameDiscardBlock}
	readOnly := map[string]bool{nameGetObservation: true, nameListObservations: true}
	builtIn := map[string]bool{nameListObservations: true}
	byName := toolsByName(t)
	for _, name := range names {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		if tool.MCPOnly == builtIn[name] || tool.MinRole != auth.RoleViewer || tool.ReadOnly != readOnly[name] {
			t.Errorf("%s = MCPOnly %v, MinRole %s, ReadOnly %v", name, tool.MCPOnly, tool.MinRole, tool.ReadOnly)
		}
		if tool.OfferedIn(ModeCreate) != builtIn[name] || tool.OfferedIn(ModeInvestigate) != builtIn[name] {
			t.Errorf("%s offered to the built-in agent = %v, want %v", name, tool.OfferedIn(ModeCreate), builtIn[name])
		}
	}
}
