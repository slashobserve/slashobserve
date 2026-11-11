package observations_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type publishFreezer struct {
	mu        sync.Mutex
	failTitle string
	err       error
	data      json.RawMessage
	onFreeze  func()
	running   int
	peak      int
	titles    []string
}

func (f *publishFreezer) Freeze(ctx context.Context, _ auth.Principal, d *spec.Dashboard, _ string, _, _ time.Time) (observations.Frozen, error) {
	title := d.Panels[0].Title
	f.mu.Lock()
	f.running++
	f.peak = max(f.peak, f.running)
	f.titles = append(f.titles, title)
	failTitle, err, data, hook := f.failTitle, f.err, f.data, f.onFreeze
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.running--
		f.mu.Unlock()
	}()
	select {
	case <-time.After(20 * time.Millisecond):
	case <-ctx.Done():
		return observations.Frozen{}, ctx.Err()
	}
	if hook != nil {
		hook()
	}
	if err != nil && (failTitle == "" || failTitle == title) {
		return observations.Frozen{}, err
	}
	if data == nil {
		data = json.RawMessage(frozenData)
	}
	return observations.Frozen{Data: data}, nil
}

func newPublishRig(t *testing.T) (rig, *publishFreezer) {
	t.Helper()
	r := newRig(t)
	f := &publishFreezer{}
	r.service = observations.New(r.db.DB, folders.NewAccess(r.db.DB), f, nil, slog.New(slog.DiscardHandler))
	return r, f
}

func titledPanel(title string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"panel":{"type":"timeseries","title":%q,"datasource":"prom","queries":[{"ref":"A","query":"sum(rate(http_requests_total[5m]))"}]}}`, title))
}

func (r rig) addTitledPanel(t *testing.T, by auth.Principal, o observations.Observation, title string) observations.Block {
	t.Helper()
	b, err := r.service.AddBlock(t.Context(), by, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindPanel, Body: titledPanel(title), Width: observations.WidthHalf})
	if err != nil {
		t.Fatalf("add panel block: %v", err)
	}
	return b
}

func (r rig) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r rig) publish(t *testing.T, by auth.Principal, o observations.Observation) observations.RCA {
	t.Helper()
	rca, err := r.service.Publish(t.Context(), by, mustUUID(t, o.ID))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return rca
}

func TestPublishingTwiceMakesTwoRevisionsWithTheirOwnSnapshots(t *testing.T) {
	r, _ := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	r.addText(t, editor, o, "Started after the deploy")
	panel := r.addTitledPanel(t, editor, o, "Rate")
	kept := r.addSnapshot(t, editor, o, panel)

	first := r.publish(t, editor, o)
	second := r.publish(t, editor, o)

	if first.Revision != 1 || second.Revision != 2 || first.ID == second.ID {
		t.Fatalf("revisions %d and %d, ids %s and %s", first.Revision, second.Revision, first.ID, second.ID)
	}
	if first.Title != "Checkout 5xx" || first.Summary != "" || first.State != observations.StateOpen || first.Actor.ID != editor.ID {
		t.Errorf("first revision %+v", first)
	}
	kinds := func(rca observations.RCA) []string {
		out := make([]string, len(rca.Blocks))
		for i, b := range rca.Blocks {
			out[i] = string(b.Kind)
		}
		return out
	}
	if got := kinds(first); !slices.Equal(got, []string{"text", "snapshot", "snapshot"}) {
		t.Fatalf("kinds %v", got)
	}
	text, frozen, existing := first.Blocks[0], first.Blocks[1], first.Blocks[2]
	if text.Markdown != "Started after the deploy" || text.Width != observations.WidthFull {
		t.Errorf("text block %+v", text)
	}
	if frozen.Title != "Rate" || frozen.Width != observations.WidthHalf || strings.Contains(string(frozen.Panel), "queries") {
		t.Errorf("frozen block %+v", frozen)
	}
	if existing.SnapshotID != kept.SnapshotID {
		t.Errorf("an existing snapshot block keeps its snapshot: %s, want %s", existing.SnapshotID, kept.SnapshotID)
	}
	if second.Blocks[1].SnapshotID == frozen.SnapshotID || second.Blocks[2].SnapshotID != kept.SnapshotID {
		t.Errorf("snapshot ids: first %s, second %s", frozen.SnapshotID, second.Blocks[1].SnapshotID)
	}
	if got := r.count(t, `SELECT count(*) FROM snapshots WHERE observation_id = $1::uuid`, o.ID); got != 3 {
		t.Errorf("%d snapshot rows, want 3: the kept one and one frozen per publish", got)
	}
	if got := r.activityTypes(t, o); !slices.Contains(got, activity.TypeObservationPublished) {
		t.Errorf("activity %v", got)
	}

	again, err := r.service.GetRCA(t.Context(), viewer, mustUUID(t, o.ID), 1)
	if err != nil || again.ID != first.ID || len(again.Blocks) != 3 {
		t.Fatalf("read back %+v, %v", again, err)
	}
	listed, err := r.service.ListRCAs(t.Context(), viewer, mustUUID(t, o.ID))
	if err != nil || len(listed) != 2 || listed[0].Revision != 2 || listed[1].Revision != 1 {
		t.Fatalf("list %+v, %v", listed, err)
	}
}

func TestAnEditedPageDoesNotChangeAPublishedRevision(t *testing.T) {
	r, _ := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	text := r.addText(t, editor, o, "before")
	first := r.publish(t, editor, o)
	if _, err := r.service.UpdateBlock(t.Context(), editor, mustUUID(t, text.ID), observations.BlockChange{Version: text.Version, Body: textBody("after")}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.Resolve(t.Context(), editor, mustUUID(t, o.ID), "Bad deploy"); err != nil {
		t.Fatal(err)
	}
	got, err := r.service.GetRCA(t.Context(), editor, mustUUID(t, o.ID), 1)
	if err != nil || got.Blocks[0].Markdown != "before" || got.State != observations.StateOpen || got.Summary != "" {
		t.Fatalf("revision 1 changed: %+v, %v", got, err)
	}
	second := r.publish(t, editor, o)
	if second.Blocks[0].Markdown != "after" || second.State != observations.StateResolved || second.Summary != "Bad deploy" || first.Revision != 1 || second.Revision != 2 {
		t.Errorf("revision 2 %+v", second)
	}
}

func TestPublishingLeavesOutRequestedBlocksAndAcceptsAnEmptyPage(t *testing.T) {
	r, _ := newPublishRig(t)
	empty := r.publish(t, editor, r.create(t, editor, r.shared))
	if len(empty.Blocks) != 0 {
		t.Errorf("an empty page published %+v", empty.Blocks)
	}
	o := r.create(t, editor, r.shared)
	r.addText(t, editor, o, "live")
	if _, err := r.service.AddBlock(t.Context(), commenter, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindText, Body: textBody("asked for")}); err != nil {
		t.Fatal(err)
	}
	rca := r.publish(t, editor, o)
	if len(rca.Blocks) != 1 || rca.Blocks[0].Markdown != "live" {
		t.Errorf("blocks %+v", rca.Blocks)
	}
}

func TestPublishingNeedsEditOnAVisibleSharedFolder(t *testing.T) {
	r, _ := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	mine := r.create(t, editor, r.personalFolder(t, editor))
	hidden := r.hiddenFolder(t)
	hiddenOne := r.create(t, admin, hidden)
	cases := []struct {
		name string
		by   auth.Principal
		o    observations.Observation
		want error
	}{
		{"a commenter", commenter, o, folders.ErrForbidden},
		{"a viewer", viewer, o, folders.ErrForbidden},
		{"a folder the caller cannot see", otherEditor, hiddenOne, observations.ErrNotFound},
		{"another person's My Dashboards", otherEditor, mine, observations.ErrNotFound},
		{"the owner's own My Dashboards", editor, mine, observations.ErrPublishPersonal},
	}
	for _, c := range cases {
		if _, err := r.service.Publish(t.Context(), c.by, mustUUID(t, c.o.ID)); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	if got := r.count(t, `SELECT count(*) FROM observation_rcas`); got != 0 {
		t.Errorf("%d revisions written by refused publishes", got)
	}
}

func TestAPanelThatFailsToFreezeRefusesThePublishAndWritesNothing(t *testing.T) {
	r, f := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	r.addText(t, editor, o, "intro")
	r.addTitledPanel(t, editor, o, "Fine")
	r.addTitledPanel(t, editor, o, "Broken")
	f.failTitle, f.err = "Broken", queryproxy.ErrUpstreamBusy

	_, err := r.service.Publish(t.Context(), editor, mustUUID(t, o.ID))
	var failed *observations.PanelFailedError
	if !errors.As(err, &failed) {
		t.Fatalf("publish: %v", err)
	}
	issue := failed.Issue()
	if failed.Position != 2 || issue.Path != "/blocks/2" || string(issue.Code) != "too_many_requests" || issue.Message == "" {
		t.Errorf("failure %+v issue %+v", failed, issue)
	}
	if rcas, snaps := r.count(t, `SELECT count(*) FROM observation_rcas`), r.count(t, `SELECT count(*) FROM snapshots`); rcas != 0 || snaps != 0 {
		t.Errorf("%d revisions and %d snapshots written", rcas, snaps)
	}

	f.err = errors.New("driver exploded with secret text")
	_, err = r.service.Publish(t.Context(), editor, mustUUID(t, o.ID))
	if !errors.As(err, &failed) {
		t.Fatalf("publish: %v", err)
	}
	if issue := failed.Issue(); issue.Code != "query_failed" || strings.Contains(issue.Message, "secret") {
		t.Errorf("an unclassified failure shows %+v", issue)
	}
}

func TestFrozenDataOverTwoMebibytesRefusesThePublish(t *testing.T) {
	r, f := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	r.addTitledPanel(t, editor, o, "Big")
	f.data = json.RawMessage(`{"frames":[],"pad":"` + strings.Repeat("x", observations.MaxSnapshotBytes) + `"}`)
	_, err := r.service.Publish(t.Context(), editor, mustUUID(t, o.ID))
	if !errors.Is(err, observations.ErrSnapshotTooLarge) {
		t.Errorf("publish: %v", err)
	}
	if got := r.count(t, `SELECT count(*) FROM observation_rcas`); got != 0 {
		t.Errorf("%d revisions written", got)
	}
}

func TestPublishFreezesAtMostFourPanelsAtATime(t *testing.T) {
	r, f := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	for i := range 10 {
		r.addTitledPanel(t, editor, o, fmt.Sprintf("P%d", i))
	}
	rca := r.publish(t, editor, o)
	if len(rca.Blocks) != 10 {
		t.Fatalf("%d blocks", len(rca.Blocks))
	}
	for i, b := range rca.Blocks {
		if want := fmt.Sprintf("P%d", i); b.Title != want {
			t.Errorf("block %d is %q, want %q", i, b.Title, want)
		}
	}
	if f.peak < 2 || f.peak > 4 {
		t.Errorf("%d panels were frozen at once, want 2 to 4", f.peak)
	}
}

func TestABlockEditedWhileFreezingRefusesThePublish(t *testing.T) {
	r, f := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	text := r.addText(t, editor, o, "before")
	r.addTitledPanel(t, editor, o, "Rate")
	var once sync.Once
	f.onFreeze = func() {
		once.Do(func() {
			if _, err := r.service.UpdateBlock(context.WithoutCancel(t.Context()), editor, mustUUID(t, text.ID), observations.BlockChange{Version: text.Version, Body: textBody("after")}); err != nil {
				t.Error(err)
			}
		})
	}
	if _, err := r.service.Publish(t.Context(), editor, mustUUID(t, o.ID)); !errors.Is(err, observations.ErrObservationChanged) {
		t.Fatalf("publish: %v", err)
	}
	if rcas, snaps := r.count(t, `SELECT count(*) FROM observation_rcas`), r.count(t, `SELECT count(*) FROM snapshots`); rcas != 0 || snaps != 0 {
		t.Errorf("%d revisions and %d snapshots written", rcas, snaps)
	}
	f.onFreeze = nil
	if got := r.publish(t, editor, o); got.Blocks[0].Markdown != "after" {
		t.Errorf("publishing again %+v", got.Blocks)
	}
}

func (r rig) fillRevisions(t *testing.T, o observations.Observation, n int) {
	t.Helper()
	r.db.Exec(t, `INSERT INTO observation_rcas (observation_id, revision, title, observation_state, content, created_by_kind, created_by_id, created_by_name)
		SELECT $1::uuid, g, 'old', 'open', '{"blocks":[]}', 'user', $2::uuid, 'Eve' FROM generate_series(1, $3::int) g`, o.ID, editor.ID, n)
}

func TestAnObservationKeepsAtMostFiftyRevisions(t *testing.T) {
	r, _ := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	r.fillRevisions(t, o, 49)
	if got := r.publish(t, editor, o); got.Revision != 50 {
		t.Fatalf("revision %d", got.Revision)
	}
	if _, err := r.service.Publish(t.Context(), editor, mustUUID(t, o.ID)); !errors.Is(err, observations.ErrRCALimit) {
		t.Errorf("the 51st publish: %v", err)
	}
}

func TestARevisionRowCannotBeUpdated(t *testing.T) {
	r, _ := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	r.publish(t, editor, o)
	_, err := r.db.SQLDB(t).ExecContext(t.Context(), `UPDATE observation_rcas SET title = 'changed'`)
	if err == nil || !strings.Contains(err.Error(), "cannot be changed") {
		t.Errorf("update: %v", err)
	}
}

func TestReadingARevisionNeedsViewAndAKnownRevision(t *testing.T) {
	r, _ := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	hidden := r.create(t, admin, r.hiddenFolder(t))
	r.publish(t, admin, hidden)
	r.publish(t, editor, o)
	id := mustUUID(t, o.ID)
	for _, revision := range []int{0, 2, -1, 1 << 40} {
		if _, err := r.service.GetRCA(t.Context(), viewer, id, revision); !errors.Is(err, observations.ErrRevisionNotFound) {
			t.Errorf("revision %d: %v", revision, err)
		}
	}
	if _, err := r.service.GetRCA(t.Context(), otherEditor, mustUUID(t, hidden.ID), 1); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("a hidden folder's revision: %v", err)
	}
	if _, err := r.service.ListRCAs(t.Context(), otherEditor, mustUUID(t, hidden.ID)); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("a hidden folder's list: %v", err)
	}
}

func TestRCASnapshotFramesAnswerOnlyForASnapshotOfThatRevision(t *testing.T) {
	r, _ := newPublishRig(t)
	o := r.create(t, editor, r.shared)
	panel := r.addTitledPanel(t, editor, o, "Rate")
	first := r.publish(t, editor, o)
	if err := r.service.RemoveBlock(t.Context(), editor, mustUUID(t, panel.ID), panel.Version, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	r.addText(t, editor, o, "no panel any more")
	second := r.publish(t, editor, o)
	id := mustUUID(t, o.ID)
	inFirst := mustUUID(t, first.Blocks[0].SnapshotID)

	frames, err := r.service.RCASnapshotFrames(t.Context(), id, 1, inFirst)
	if err != nil || !jsonEqual(t, frames, json.RawMessage(frozenData)) {
		t.Errorf("frames %s, %v", frames, err)
	}
	if _, err := r.service.RCASnapshotFrames(t.Context(), id, 2, inFirst); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("a snapshot of another revision: %v", err)
	}
	if _, err := r.service.RCASnapshotFrames(t.Context(), id, 3, inFirst); !errors.Is(err, observations.ErrRevisionNotFound) {
		t.Errorf("an unknown revision: %v", err)
	}
	if len(second.Blocks) != 1 {
		t.Errorf("second %+v", second.Blocks)
	}
}

type rcaDocumented struct {
	*documented
	freezer *publishFreezer
}

func newRCADocumented(t *testing.T) rcaDocumented {
	t.Helper()
	d := newDocumented(t)
	f := &publishFreezer{}
	d.service = observations.New(d.db.DB, folders.NewAccess(d.db.DB), f, nil, slog.New(slog.DiscardHandler))
	gin.SetMode(gin.TestMode)
	d.engine = gin.New()
	api := auth.NewRoutes(d.engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), d.who))
	}))
	observations.NewHandler(d.service, slog.New(slog.DiscardHandler)).Register(api)
	observations.NewRCAHandler(d.service, slog.New(slog.DiscardHandler)).Register(api)
	return rcaDocumented{documented: d, freezer: f}
}

type rcaEnvelope struct {
	RCA observations.RCAJSON `json:"rca"`
}

type rcaList struct {
	RCAs []observations.RCASummaryJSON `json:"rcas"`
}

type panelProblem struct {
	Code   string `json:"code"`
	Issues []struct {
		Path string `json:"path"`
		Code string `json:"code"`
	} `json:"issues"`
}

func TestRCAHTTPRoutesMatchTheDocument(t *testing.T) {
	d := newRCADocumented(t)
	created := d.newObservation(t, editor, d.shared.String(), `,"first_block":{"kind":"text","body":{"markdown":"Started after the deploy"}}`)
	id := created.Observation.ID
	d.send(t, editor, http.MethodPost, "/api/v1/observations/"+id+"/blocks", `{"kind":"panel","body":`+string(titledPanel("Rate"))+`}`, http.StatusCreated)

	rec := d.call(t, editor, http.MethodPost, "/api/v1/observations/"+id+"/rcas", http.StatusCreated)
	published := decode[rcaEnvelope](t, rec).RCA
	if published.Revision != 1 || published.ObservationID != id || len(published.Blocks) != 2 || published.Summary != nil || published.From != nil {
		t.Errorf("published %+v", published)
	}
	if strings.Contains(rec.Body.String(), "sum(rate") {
		t.Errorf("the RCA carries query text: %s", rec.Body)
	}
	d.call(t, editor, http.MethodPost, "/api/v1/observations/"+id+"/rcas", http.StatusCreated)

	listed := decode[rcaList](t, d.call(t, viewer, http.MethodGet, "/api/v1/observations/"+id+"/rcas", http.StatusOK))
	if len(listed.RCAs) != 2 || listed.RCAs[0].Revision != 2 {
		t.Errorf("list %+v", listed)
	}
	got := decode[rcaEnvelope](t, d.call(t, viewer, http.MethodGet, "/api/v1/observations/"+id+"/rcas/1", http.StatusOK)).RCA
	if got.ID != published.ID {
		t.Errorf("revision 1 is %s, want %s", got.ID, published.ID)
	}
}

func TestRCAHTTPRefusalsUseTheDocumentedCodes(t *testing.T) {
	d := newRCADocumented(t)
	id := d.newObservation(t, editor, d.shared.String(), "").Observation.ID
	panelPath := "/api/v1/observations/" + id + "/blocks"
	d.send(t, editor, http.MethodPost, panelPath, `{"kind":"panel","body":`+string(titledPanel("Rate"))+`}`, http.StatusCreated)
	publishPath := "/api/v1/observations/" + id + "/rcas"
	missing := "/api/v1/observations/00000000-0000-4000-8000-000000000000/rcas"

	cases := []struct {
		name   string
		by     auth.Principal
		method string
		path   string
		status int
		code   string
	}{
		{"a commenter publishing", commenter, http.MethodPost, publishPath, http.StatusForbidden, "forbidden_role"},
		{"an unknown observation", editor, http.MethodPost, missing, http.StatusNotFound, "not_found"},
		{"an unknown revision", viewer, http.MethodGet, publishPath + "/9", http.StatusNotFound, "not_found"},
		{"a revision that is not a number", viewer, http.MethodGet, publishPath + "/abc", http.StatusBadRequest, "invalid_request"},
		{"revision zero", viewer, http.MethodGet, publishPath + "/0", http.StatusBadRequest, "invalid_request"},
		{"an id that is not a UUID", viewer, http.MethodGet, "/api/v1/observations/nope/rcas", http.StatusBadRequest, "invalid_request"},
	}
	for _, c := range cases {
		rec := d.call(t, c.by, c.method, c.path, c.status)
		if got := errorCode(t, rec); got != c.code {
			t.Errorf("%s: code %q, want %q", c.name, got, c.code)
		}
	}

	d.freezer.err = queryproxy.ErrUpstreamBusy
	rec := d.call(t, editor, http.MethodPost, publishPath, http.StatusUnprocessableEntity)
	invalid := decode[panelProblem](t, rec)
	if invalid.Code != "panel_failed" || len(invalid.Issues) != 1 || invalid.Issues[0].Path != "/blocks/0" || invalid.Issues[0].Code != "too_many_requests" {
		t.Errorf("panel failure %s", rec.Body)
	}

	d.freezer.err = nil
	d.freezer.data = json.RawMessage(`{"frames":[],"pad":"` + strings.Repeat("x", observations.MaxSnapshotBytes) + `"}`)
	if got := errorCode(t, d.call(t, editor, http.MethodPost, publishPath, http.StatusUnprocessableEntity)); got != "snapshot_too_large" {
		t.Errorf("oversize code %q", got)
	}

	d.freezer.data = nil
	d.db.Exec(t, `INSERT INTO observation_rcas (observation_id, revision, title, observation_state, content, created_by_kind, created_by_id, created_by_name)
		SELECT $1::uuid, g, 'old', 'open', '{"blocks":[]}', 'user', $2::uuid, 'Eve' FROM generate_series(1, 50) g`, id, editor.ID)
	if got := errorCode(t, d.call(t, editor, http.MethodPost, publishPath, http.StatusConflict)); got != "rca_limit" {
		t.Errorf("limit code %q", got)
	}

	mine := d.newObservation(t, editor, d.personalFolder(t, editor).String(), "").Observation.ID
	if got := errorCode(t, d.call(t, editor, http.MethodPost, "/api/v1/observations/"+mine+"/rcas", http.StatusConflict)); got != "personal_folder" {
		t.Errorf("personal folder code %q", got)
	}
}

func TestRCAHTTPAnObservationEditedWhileFreezingAnswers409(t *testing.T) {
	d := newRCADocumented(t)
	id := d.newObservation(t, editor, d.shared.String(), `,"first_block":{"kind":"text","body":{"markdown":"before"}}`).Observation.ID
	d.send(t, editor, http.MethodPost, "/api/v1/observations/"+id+"/blocks", `{"kind":"panel","body":`+string(titledPanel("Rate"))+`}`, http.StatusCreated)
	page, err := d.service.Get(t.Context(), editor, mustUUID(t, id))
	if err != nil {
		t.Fatal(err)
	}
	first := page.Blocks[0]
	d.freezer.onFreeze = func() {
		_, _ = d.service.UpdateBlock(context.WithoutCancel(t.Context()), editor, mustUUID(t, first.ID), observations.BlockChange{Version: first.Version, Body: textBody("after")})
	}
	rec := d.call(t, editor, http.MethodPost, "/api/v1/observations/"+id+"/rcas", http.StatusConflict)
	if got := errorCode(t, rec); got != "observation_changed" {
		t.Errorf("code %q", got)
	}
}
