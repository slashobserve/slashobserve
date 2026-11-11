package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type probe struct {
	fakeDatasource
	mu         sync.Mutex
	ran        []string
	fail       map[string]bool
	emptyBelow map[string]time.Duration
}

func (p *probe) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ran = nil
}

func (p *probe) texts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.ran)
}

func (p *probe) ranContaining(part string) int {
	n := 0
	for _, text := range p.texts() {
		if strings.Contains(text, part) {
			n++
		}
	}
	return n
}

func (p *probe) setFail(text string, failing bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fail[text] = failing
}

func (p *probe) setEmptyBelow(text string, d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.emptyBelow[text] = d
}

func (p *probe) answer(text string, rng datasource.TimeRange) ([]datasource.Frame, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ran = append(p.ran, text)
	for part, failing := range p.fail {
		if failing && strings.Contains(text, part) {
			return nil, errors.New("boom")
		}
	}
	for part, below := range p.emptyBelow {
		if strings.Contains(text, part) && rng.To.Sub(rng.From) < below {
			return nil, nil
		}
	}
	return fakeFrames(), nil
}

func (p *probe) QueryRange(_ context.Context, q datasource.Query, rng datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
	return p.answer(q.Text, rng)
}

func (p *probe) QueryInstant(_ context.Context, q datasource.Query, rng datasource.TimeRange) ([]datasource.Frame, error) {
	return p.answer(q.Text, rng)
}

func (p *probe) VariableValues(_ context.Context, q datasource.Query, rng datasource.TimeRange) ([]string, error) {
	if _, err := p.answer(q.Text, rng); err != nil {
		return nil, err
	}
	return []string{"v"}, nil
}

func newProbeRig(t *testing.T) (*rig, *probe) {
	t.Helper()
	p := &probe{fail: map[string]bool{}, emptyBelow: map[string]time.Duration{}}
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: p})
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	service := New(database.DB, queryproxy.NewRunner(registry, logger), access, logger)
	queue := revisions.NewService(database.DB, noopReviser{}, logger, revisions.WithAccess(access))
	return &rig{db: database, service: service, revisions: queue}, p
}

type panelDef struct {
	id, query, extra string
}

func panelJSON(i int, p panelDef) string {
	return fmt.Sprintf(`{"id":%q,"type":"timeseries","title":%q,"gridPos":{"x":0,"y":%d,"w":12,"h":4},"datasource":"prom","queries":[{"query":%q}]%s}`, p.id, p.id, i*4, p.query, p.extra)
}

type specDef struct {
	title     string
	from      string
	variables string
	panels    []panelDef
}

func (s specDef) json() []byte {
	if s.title == "" {
		s.title = "t"
	}
	if s.from == "" {
		s.from = "now-15m"
	}
	items := make([]string, len(s.panels))
	for i, p := range s.panels {
		items[i] = panelJSON(i, p)
	}
	return fmt.Appendf(nil, `{"specVersion":"0","title":%q,"time":{"from":%q,"to":"now"},"variables":[%s],"panels":[%s]}`, s.title, s.from, s.variables, strings.Join(items, ","))
}

func numberedPanels(n int) []panelDef {
	panels := make([]panelDef, n)
	for i := range panels {
		panels[i] = panelDef{id: fmt.Sprintf("p%d", i), query: fmt.Sprintf("m%d", i)}
	}
	return panels
}

func (r *rig) createSpec(t *testing.T, s specDef) pgtype.UUID {
	t.Helper()
	dash, _, err := r.service.Create(t.Context(), testUser, pgtype.UUID{}, s.json(), "create")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) importSpec(t *testing.T, s specDef) (pgtype.UUID, Version) {
	t.Helper()
	dash, ver, _, _, err := r.service.CreateImported(t.Context(), testUser, pgtype.UUID{}, s.json(), "import", pgtype.UUID{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id, ver
}

func (r *rig) mergedPatch(t *testing.T, id pgtype.UUID, parent int, patch spec.PanelPatch) []byte {
	t.Helper()
	base, err := r.service.VersionSpec(t.Context(), id, parent)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := spec.ApplyPanelPatch(base, patch)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (r *rig) saveChanged(t *testing.T, id pgtype.UUID, parent int, patch spec.PanelPatch) (Version, error) {
	t.Helper()
	_, ver, _, err := r.service.AddChangedVersion(t.Context(), testUser, id, parent, r.mergedPatch(t, id, parent, patch), "patch", pgtype.UUID{})
	return ver, err
}

func upsert(i int, p panelDef) spec.PanelPatch {
	return spec.PanelPatch{Upsert: []json.RawMessage{json.RawMessage(panelJSON(i, p))}}
}

func dashboardFields(raw string) spec.PanelPatch {
	var f spec.DashboardFields
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		panic(err)
	}
	return spec.PanelPatch{Dashboard: &f}
}

func wantInvalid(t *testing.T, err error) *InvalidSpecError {
	t.Helper()
	var invalid *InvalidSpecError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidSpecError", err)
	}
	return invalid
}

func TestChangedSaveRunsOnlyTheChangedPanelOfALargeDashboard(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(125)})
	p.reset()

	ver, err := r.saveChanged(t, id, 1, upsert(7, panelDef{id: "p7", query: "changed7"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.texts(); len(got) != 1 || got[0] != "changed7" {
		t.Fatalf("queries run %v, want only changed7", got)
	}
	if ver.Number != 2 || !ver.Unverified.Empty() {
		t.Fatalf("version %d unverified %+v", ver.Number, ver.Unverified)
	}
}

func TestChangedSaveIgnoresAnUnchangedPanelThatNowFails(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(3)})
	p.setFail("m1", true)
	p.reset()

	ver, err := r.saveChanged(t, id, 1, upsert(2, panelDef{id: "p2", query: "other2"}))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !ver.Unverified.Empty() || p.ranContaining("m1") != 0 {
		t.Fatalf("unverified %+v, m1 ran %d times", ver.Unverified, p.ranContaining("m1"))
	}
}

func TestChangedSaveCarriesAnUnchangedUnverifiedPanelWithoutRunningIt(t *testing.T) {
	r, p := newProbeRig(t)
	p.setFail("m1", true)
	id, first := r.importSpec(t, specDef{panels: numberedPanels(3)})
	if _, ok := first.Unverified.Panel("p1"); !ok {
		t.Fatalf("import unverified %+v, want p1", first.Unverified)
	}
	p.reset()

	ver, err := r.saveChanged(t, id, 1, upsert(2, panelDef{id: "p2", query: "other2"}))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := ver.Unverified.Panel("p1")
	if !ok || len(ver.Unverified.Panels) != 1 {
		t.Fatalf("unverified %+v, want only p1", ver.Unverified)
	}
	want, _ := first.Unverified.Panel("p1")
	if fmt.Sprint(got.Issues) != fmt.Sprint(want.Issues) {
		t.Fatalf("issues %v, want %v", got.Issues, want.Issues)
	}
	if p.ranContaining("m1") != 0 {
		t.Fatalf("queries run %v, want none for p1", p.texts())
	}
}

func TestChangedSaveClearsAnUnverifiedPanelThatWasEditedAndNowPasses(t *testing.T) {
	r, p := newProbeRig(t)
	p.setFail("m1", true)
	id, _ := r.importSpec(t, specDef{panels: numberedPanels(3)})

	ver, err := r.saveChanged(t, id, 1, upsert(1, panelDef{id: "p1", query: "fixed1"}))
	if err != nil {
		t.Fatal(err)
	}
	if !ver.Unverified.Empty() {
		t.Fatalf("unverified %+v, want none", ver.Unverified)
	}
}

func TestChangedSaveBlocksAChangedPanelThatFails(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(3)})
	p.setFail("bad", true)

	_, err := r.saveChanged(t, id, 1, upsert(2, panelDef{id: "p2", query: "bad"}))
	invalid := wantInvalid(t, err)
	if len(invalid.Issues) != 1 || !strings.HasPrefix(invalid.Issues[0].Path, "/panels/2/") {
		t.Fatalf("issues %v, want one under /panels/2", invalid.Issues)
	}
	if latest, err := r.service.LatestVersion(t.Context(), id); err != nil || latest != 1 {
		t.Fatalf("latest %d, err %v", latest, err)
	}
}

func TestChangedSaveKeepsAMovedUnverifiedPanelPointingAtItsNewPlace(t *testing.T) {
	r, p := newProbeRig(t)
	p.setFail("m1", true)
	id, first := r.importSpec(t, specDef{panels: numberedPanels(3)})
	before, _ := first.Unverified.Panel("p1")
	suffix := strings.TrimPrefix(before.Issues[0].Path, "/panels/1")

	ver, err := r.saveChanged(t, id, 1, spec.PanelPatch{Delete: []string{"p0"}})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := ver.Unverified.Panel("p1")
	if got := after.Issues[0].Path; got != "/panels/0"+suffix {
		t.Fatalf("path after delete %q, want /panels/0%s", got, suffix)
	}

	row := json.RawMessage(`{"id":"r","type":"row","title":"R","gridPos":{"x":0,"y":0,"w":24,"h":1}}`)
	moved, err := r.saveChanged(t, id, 2, spec.PanelPatch{Upsert: []json.RawMessage{row}, Layout: []spec.PanelPlacement{{ID: "p1", Row: new("r")}}})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := moved.Spec.FindPanel("p1")
	if !ok || entry.Row == nil {
		t.Fatalf("p1 not inside a row: %+v", entry)
	}
	in, _ := moved.Unverified.Panel("p1")
	if got := in.Issues[0].Path; got != entry.Pointer+suffix {
		t.Fatalf("path after move %q, want %s%s", got, entry.Pointer, suffix)
	}
}

func TestChangedSaveDropsADeletedUnverifiedPanel(t *testing.T) {
	r, p := newProbeRig(t)
	p.setFail("m1", true)
	id, _ := r.importSpec(t, specDef{panels: numberedPanels(3)})

	ver, err := r.saveChanged(t, id, 1, spec.PanelPatch{Delete: []string{"p1"}})
	if err != nil {
		t.Fatal(err)
	}
	if !ver.Unverified.Empty() {
		t.Fatalf("unverified %+v, want none", ver.Unverified)
	}
}

func TestChangedSaveRerunsOnlyThePanelsAChangedVariableReaches(t *testing.T) {
	const variable = `{"name":"inst","type":"query","datasource":"prom","query":"label_values(%s, instance)"}`
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{
		variables: fmt.Sprintf(variable, "up"),
		panels: []panelDef{
			{id: "uses", query: `uses{instance=~"$inst"}`},
			{id: "plain", query: "plain"},
		},
	})
	p.reset()

	patch := spec.PanelPatch{Variables: json.RawMessage("[" + fmt.Sprintf(variable, "down") + "]")}
	if _, err := r.saveChanged(t, id, 1, patch); err != nil {
		t.Fatal(err)
	}
	if p.ranContaining("uses") != 1 || p.ranContaining("plain") != 0 {
		t.Fatalf("queries run %v, want the panel that reads the variable and not the other", p.texts())
	}
}

func TestChangedSaveAnswersConflictsAndReplaysRequests(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(2)})
	request := r.queueRevision(t, id)
	raw := r.mergedPatch(t, id, 1, upsert(1, panelDef{id: "p1", query: "next1"}))

	_, first, replayed, err := r.service.AddChangedVersion(t.Context(), testUser, id, 1, raw, "patch", request)
	if err != nil || replayed || first.Number != 2 {
		t.Fatalf("first: version %d replayed %t err %v", first.Number, replayed, err)
	}
	_, again, replayed, err := r.service.AddChangedVersion(t.Context(), testUser, id, 1, raw, "patch", request)
	if err != nil || !replayed || again.Number != 2 {
		t.Fatalf("repeat: version %d replayed %t err %v", again.Number, replayed, err)
	}

	other := r.mergedPatch(t, id, 1, upsert(0, panelDef{id: "p0", query: "next0"}))
	_, _, _, err = r.service.AddChangedVersion(t.Context(), testUser, id, 1, other, "patch", pgtype.UUID{})
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.Latest != 2 {
		t.Fatalf("stale parent: err %v, want a conflict at version 2", err)
	}
}

func TestNeedsFullCheck(t *testing.T) {
	parse := func(s specDef) *spec.Dashboard {
		t.Helper()
		d, err := spec.Parse(s.json())
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	const regexVariable = `{"name":"v","type":"query","datasource":"prom","query":"label_values(up, i)","regex":"/${__dashboard}/"}`
	const queryVariable = `{"name":"v","type":"query","datasource":"prom","query":"label_values(up{d=\"$__dashboard\"}, i)"}`
	plain := []panelDef{{id: "a", query: "up"}}
	macro := []panelDef{{id: "a", query: `up{d="$__dashboard"}`}}

	cases := []struct {
		name       string
		prev, next specDef
		want       bool
	}{
		{"time to another range", specDef{panels: plain}, specDef{from: "now-6h", panels: plain}, true},
		{"time to the same values", specDef{panels: plain}, specDef{panels: plain}, false},
		{"title with no macro", specDef{panels: plain}, specDef{title: "renamed", panels: plain}, false},
		{"title with the macro in a panel", specDef{panels: macro}, specDef{title: "renamed", panels: macro}, true},
		{"title with the macro in a variable query", specDef{variables: queryVariable, panels: plain}, specDef{title: "renamed", variables: queryVariable, panels: plain}, true},
		{"title with the macro in a variable regex", specDef{variables: regexVariable, panels: plain}, specDef{title: "renamed", variables: regexVariable, panels: plain}, true},
		{"macro present and title kept", specDef{panels: macro}, specDef{panels: macro}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsFullCheck(parse(tc.prev), parse(tc.next)); got != tc.want {
				t.Fatalf("needsFullCheck = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestTimeChangeRunsEveryQueryAndTheSameTimeRunsNone(t *testing.T) {
	const variable = `{"name":"inst","type":"query","datasource":"prom","query":"label_values(vq, instance)"}`
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{variables: variable, panels: numberedPanels(5)})
	p.reset()

	same, err := r.saveChanged(t, id, 1, dashboardFields(`{"time":{"from":"now-15m","to":"now"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.texts(); len(got) != 0 {
		t.Fatalf("same time ran %v, want nothing", got)
	}

	if _, err := r.saveChanged(t, id, same.Number, dashboardFields(`{"time":{"from":"now-1h","to":"now"}}`)); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if n := p.ranContaining(fmt.Sprintf("m%d", i)); n != 1 {
			t.Fatalf("panel query m%d ran %d times, want 1", i, n)
		}
	}
	if p.ranContaining("vq") == 0 {
		t.Fatalf("variable query did not run: %v", p.texts())
	}
}

func TestTimeChangeBlocksOnAVerifiedPanelThatIsEmptyOverTheNewRange(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(3)})
	p.setEmptyBelow("m1", 30*time.Minute)
	p.reset()

	_, err := r.saveChanged(t, id, 1, dashboardFields(`{"time":{"from":"now-5m","to":"now"}}`))
	invalid := wantInvalid(t, err)
	if len(invalid.Issues) != 1 || invalid.Issues[0].Path != "/panels/1/queries" || invalid.Issues[0].Code != queryproxy.CodeQueryEmpty {
		t.Fatalf("issues %v, want the empty panel p1", invalid.Issues)
	}
}

func TestTimeChangeCarriesUnchangedUnverifiedPanelsAndClearsOnesThatNowPass(t *testing.T) {
	r, p := newProbeRig(t)
	p.setFail("m1", true)
	p.setEmptyBelow("m2", 30*time.Minute)
	id, first := r.importSpec(t, specDef{panels: numberedPanels(4)})
	if ids := panelIDs(first.Unverified); len(ids) != 2 {
		t.Fatalf("import unverified %v, want p1 and p2", ids)
	}

	ver, err := r.saveChanged(t, id, 1, dashboardFields(`{"time":{"from":"now-1h","to":"now"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if ids := panelIDs(ver.Unverified); len(ids) != 1 || ids[0] != "p1" {
		t.Fatalf("unverified %v, want only p1", ids)
	}
}

func TestTimeChangeWithAnUpsertChecksBothAndStoresTheRange(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(3)})
	p.reset()

	patch := upsert(2, panelDef{id: "p2", query: "edited2"})
	patch.Dashboard = dashboardFields(`{"time":{"from":"now-6h","to":"now"}}`).Dashboard
	ver, err := r.saveChanged(t, id, 1, patch)
	if err != nil {
		t.Fatal(err)
	}
	if p.ranContaining("m0") != 1 || p.ranContaining("m1") != 1 || p.ranContaining("edited2") != 1 || p.ranContaining("m2") != 0 {
		t.Fatalf("queries run %v, want every current panel once", p.texts())
	}
	if ver.Spec.Time.From != "now-6h" {
		t.Fatalf("stored time %+v", ver.Spec.Time)
	}
}

func TestTagsAndLinksChangesRunNoQuery(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(3)})
	p.reset()

	first, err := r.saveChanged(t, id, 1, dashboardFields(`{"tags":["linux","prod"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.saveChanged(t, id, first.Number, dashboardFields(`{"links":[{"title":"Docs","url":"https://example.com"}]}`)); err != nil {
		t.Fatal(err)
	}
	if got := p.texts(); len(got) != 0 {
		t.Fatalf("queries run %v, want none", got)
	}
}

func TestTitleChangeRunsEveryQueryOnlyWhenTheDashboardMacroIsUsed(t *testing.T) {
	r, p := newProbeRig(t)
	plain := r.createSpec(t, specDef{panels: numberedPanels(3)})
	macro := r.createSpec(t, specDef{panels: []panelDef{{id: "a", query: `a{d="$__dashboard"}`}, {id: "b", query: "b"}}})
	p.reset()

	if _, err := r.saveChanged(t, plain, 1, dashboardFields(`{"title":"renamed"}`)); err != nil {
		t.Fatal(err)
	}
	if got := p.texts(); len(got) != 0 {
		t.Fatalf("title without the macro ran %v, want none", got)
	}

	if _, err := r.saveChanged(t, macro, 1, dashboardFields(`{"title":"renamed"}`)); err != nil {
		t.Fatal(err)
	}
	if got := p.texts(); len(got) != 2 || !slices.Contains(got, "b") || p.ranContaining("a{") != 1 {
		t.Fatalf("title with the macro ran %v, want both panels", p.texts())
	}
}

func TestTitleAndTagsSavesShowInGetAndList(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(2)})

	patch := dashboardFields(`{"title":"Renamed board","tags":["linux"]}`)
	if _, err := r.saveChanged(t, id, 1, patch); err != nil {
		t.Fatal(err)
	}
	dash, ver, err := r.service.Get(t.Context(), id)
	if err != nil || dash.Title != "Renamed board" || ver.Spec.Title != "Renamed board" || len(dash.Tags) != 1 || dash.Tags[0] != "linux" {
		t.Fatalf("get: %+v, err %v", dash, err)
	}
	page, err := r.service.List(t.Context(), testUser, ListParams{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].Title != "Renamed board" || len(page.Items[0].Tags) != 1 || page.Items[0].Tags[0] != "linux" {
		t.Fatalf("list: %+v, err %v", page.Items, err)
	}
}
