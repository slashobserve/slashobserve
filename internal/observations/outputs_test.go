package observations_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type fakeDashboards struct {
	t     *testing.T
	mu    sync.Mutex
	db    *dbtest.Database
	specs []spec.Dashboard
	from  []placement.Copied
}

func (f *fakeDashboards) CreateCopy(ctx context.Context, _ auth.Principal, raw []byte, _ string, from placement.Copied) (dashboards.Dashboard, dashboards.Version, error) {
	var d spec.Dashboard
	if err := json.Unmarshal(raw, &d); err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, err
	}
	f.mu.Lock()
	f.specs = append(f.specs, d)
	f.from = append(f.from, from)
	f.mu.Unlock()
	var id string
	err := f.db.SQLDB(f.t).QueryRowContext(ctx, `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ($1, 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`, d.Title).Scan(&id)
	return f.shown(id, &d), f.version(&d), err
}

func (f *fakeDashboards) shown(id string, d *spec.Dashboard) dashboards.Dashboard {
	author := dashboards.Author{Kind: "user", ID: editor.ID, Name: editor.Name}
	return dashboards.Dashboard{
		ID: id, Title: d.Title, Tags: []string{}, LatestVersion: 1, CreatedAt: windowTo, UpdatedAt: windowTo, UpdatedBy: author,
		Creator: dashboards.Creator{Author: author, Via: "direct"}, Folder: folders.Folder{ID: mustUUID(f.t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"), Title: "My Dashboards"},
	}
}

func (f *fakeDashboards) version(d *spec.Dashboard) dashboards.Version {
	return dashboards.Version{VersionMeta: dashboards.VersionMeta{Number: 1, Author: dashboards.Author{Kind: "user", ID: editor.ID, Name: editor.Name}, CreatedAt: windowTo}, Spec: d}
}

func (f *fakeDashboards) Get(_ context.Context, id pgtype.UUID) (dashboards.Dashboard, dashboards.Version, error) {
	d := f.specs[0]
	return f.shown(id.String(), &d), f.version(&d), nil
}

func (f *fakeDashboards) Delete(context.Context, auth.Principal, pgtype.UUID) error { return nil }

func (r rig) dashboard(t *testing.T) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ('Checkout', 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return mustUUID(t, raw)
}

func (r rig) runOn(t *testing.T, o observations.Observation, dashboardID pgtype.UUID) {
	t.Helper()
	var run string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO agent_requests (intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, dashboard_id, context_from, context_to)
VALUES ('ask', 'investigate', 'why', 'user', $1::uuid, 'Eve', $2, $3::timestamptz - interval '1 hour', $3::timestamptz) RETURNING id::text`, editor.ID, dashboardID, windowTo).Scan(&run); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE observations SET run_id = $1::uuid, time_from = $2::timestamptz - interval '1 hour', time_to = $2::timestamptz WHERE id = $3::uuid`, run, windowTo, o.ID)
}

func TestPinningAnObservationMakesOneAnnotationAtTheRangeEnd(t *testing.T) {
	r := newRig(t)
	about := r.dashboard(t)
	o := r.create(t, editor, r.shared)
	r.runOn(t, o, about)
	id := mustUUID(t, o.ID)

	pin, created, err := r.service.Pin(reading(t.Context()), editor, id, pgtype.UUID{})
	if err != nil || !created || pin.DashboardID != about.String() {
		t.Fatalf("pin %+v created %v: %v", pin, created, err)
	}
	var text, tags string
	var at string
	if err := r.db.SQLDB(t).QueryRow(`SELECT text, tags::text, time_from::text FROM annotations WHERE id = $1::uuid`, pin.AnnotationID).Scan(&text, &tags, &at); err != nil {
		t.Fatal(err)
	}
	if text != "Checkout 5xx /o/"+o.ID || tags != "{finding}" || at != "2026-11-01 10:00:00+00" {
		t.Errorf("annotation text %q tags %s at %s", text, tags, at)
	}

	again, created, err := r.service.Pin(reading(t.Context()), viewer, id, about)
	if err != nil || created || again.AnnotationID != pin.AnnotationID {
		t.Fatalf("second pin %+v created %v: %v", again, created, err)
	}
	var count int
	if err := r.db.SQLDB(t).QueryRow(`SELECT count(*) FROM annotations`).Scan(&count); err != nil || count != 1 {
		t.Errorf("annotations %d: %v", count, err)
	}
	read, err := r.service.Get(reading(t.Context()), viewer, id)
	if err != nil || read.DashboardID != about.String() || !read.Pinned {
		t.Errorf("read %q pinned %v: %v", read.DashboardID, read.Pinned, err)
	}
}

func TestPinningNeedsADashboardAndEditOnIt(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	if _, _, err := r.service.Pin(reading(t.Context()), editor, id, pgtype.UUID{}); !errors.Is(err, observations.ErrNoDashboard) {
		t.Errorf("pin without a dashboard: %v", err)
	}
	about := r.dashboard(t)
	if _, _, err := r.service.Pin(reading(t.Context()), viewer, id, about); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("pin by a viewer of the dashboard: %v", err)
	}
	if _, _, err := r.service.Pin(reading(t.Context()), editor, id, about); err != nil {
		t.Errorf("pin to a dashboard named by the caller: %v", err)
	}
	hidden := r.create(t, admin, r.hiddenFolder(t))
	if _, _, err := r.service.Pin(reading(t.Context()), viewer, mustUUID(t, hidden.ID), about); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("pin of an observation the caller cannot read: %v", err)
	}
}

func TestMakingADashboardGivesTheFirstOneBackUnlessAskedForAnother(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	if _, _, _, err := r.service.MakeDashboard(reading(t.Context()), editor, id, pgtype.UUID{}, false); !errors.Is(err, observations.ErrNoPanels) {
		t.Fatalf("make from an observation without panels: %v", err)
	}
	r.addText(t, editor, o, "Checkout is failing")
	first := r.addPanel(t, editor, o)
	r.addPanel(t, editor, o)
	withVariable, err := r.service.AddBlock(reading(t.Context()), editor, id, observations.BlockInput{Kind: observations.KindPanel, Body: json.RawMessage(`{"panel":` + variablePanel + `,"variable_values":{"service":["checkout"]}}`)})
	if err != nil {
		t.Fatal(err)
	}

	d, _, created, err := r.service.MakeDashboard(reading(t.Context()), editor, id, pgtype.UUID{}, false)
	if err != nil || !created {
		t.Fatalf("make: created %v: %v", created, err)
	}
	made := r.made.specs[0]
	if len(r.made.specs) != 1 || len(made.Panels) != 3 || len(made.Variables) != 1 || made.Variables[0].Name != "service" || made.Title != "Checkout 5xx" {
		t.Fatalf("dashboard made: %d specs, %d panels, variables %+v", len(r.made.specs), len(made.Panels), made.Variables)
	}

	again, _, created, err := r.service.MakeDashboard(reading(t.Context()), editor, id, pgtype.UUID{}, false)
	if err != nil || created || again.ID != d.ID || len(r.made.specs) != 1 {
		t.Fatalf("second make: %q created %v, %d specs: %v", again.ID, created, len(r.made.specs), err)
	}
	other, _, created, err := r.service.MakeDashboard(reading(t.Context()), otherEditor, id, pgtype.UUID{}, false)
	if err != nil || !created || other.ID == d.ID {
		t.Fatalf("make by another person: %q created %v: %v", other.ID, created, err)
	}
	fresh, _, created, err := r.service.MakeDashboard(reading(t.Context()), editor, id, pgtype.UUID{}, true)
	if err != nil || !created || fresh.ID == d.ID {
		t.Fatalf("make with new: %q created %v: %v", fresh.ID, created, err)
	}
	latest, _, created, err := r.service.MakeDashboard(reading(t.Context()), editor, id, pgtype.UUID{}, false)
	if err != nil || created || latest.ID != fresh.ID {
		t.Fatalf("make after new: %q created %v: %v", latest.ID, created, err)
	}

	one, _, created, err := r.service.MakeDashboard(reading(t.Context()), editor, pgtype.UUID{}, mustUUID(t, withVariable.ID), false)
	if err != nil || !created || len(r.made.specs[len(r.made.specs)-1].Panels) != 1 || one.ID == fresh.ID {
		t.Fatalf("make from a block: %q created %v: %v", one.ID, created, err)
	}
	sameBlock, _, created, err := r.service.MakeDashboard(reading(t.Context()), editor, id, mustUUID(t, withVariable.ID), false)
	if err != nil || created || sameBlock.ID != one.ID {
		t.Fatalf("make from the block again: %q created %v: %v", sameBlock.ID, created, err)
	}
	other2 := r.create(t, editor, r.shared)
	if _, _, _, err := r.service.MakeDashboard(reading(t.Context()), editor, mustUUID(t, other2.ID), mustUUID(t, first.ID), false); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("make from a block of another observation: %v", err)
	}
}

func TestMakingADashboardLeavesOutBlocksTheCallerCannotSee(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	suggested, err := r.service.AddBlock(reading(t.Context()), commenter, id, observations.BlockInput{Kind: observations.KindPanel, Body: panelBody()})
	if err != nil || suggested.State != observations.BlockRequested {
		t.Fatalf("suggest: %+v %v", suggested, err)
	}
	for _, by := range []auth.Principal{editor, viewer, commenter} {
		if _, _, _, err := r.service.MakeDashboard(reading(t.Context()), by, id, pgtype.UUID{}, false); !errors.Is(err, observations.ErrNoPanels) {
			t.Errorf("%s makes a dashboard from only a suggested block: %v", by.Name, err)
		}
	}
	if _, _, _, err := r.service.MakeDashboard(reading(t.Context()), viewer, pgtype.UUID{}, mustUUID(t, suggested.ID), false); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("a viewer makes one from a suggested block: %v", err)
	}
	hidden := r.create(t, admin, r.hiddenFolder(t))
	r.addPanel(t, admin, hidden)
	if _, _, _, err := r.service.MakeDashboard(reading(t.Context()), viewer, mustUUID(t, hidden.ID), pgtype.UUID{}, false); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("make from an observation the caller cannot read: %v", err)
	}
}

func TestPinAndMakeDashboardRoutesMatchTheDocument(t *testing.T) {
	d := newDocumented(t)
	about := d.dashboard(t)
	o := d.newObservation(t, editor, d.shared.String(), "").Observation
	pin := "/api/v1/observations/" + o.ID + "/pin"
	madeAt := "/api/v1/observations/" + o.ID + "/dashboard"

	if got := errorCode(t, d.send(t, editor, http.MethodPost, pin, `{}`, http.StatusConflict)); got != "observation_has_no_dashboard" {
		t.Errorf("pin code %q", got)
	}
	if got := errorCode(t, d.send(t, editor, http.MethodPost, madeAt, `{}`, http.StatusConflict)); got != "observation_has_no_panels" {
		t.Errorf("make code %q", got)
	}
	d.send(t, editor, http.MethodPost, pin, `{"dashboard_id":"nope"}`, http.StatusBadRequest)
	d.send(t, viewer, http.MethodPost, pin, `{"dashboard_id":"`+about.String()+`"}`, http.StatusForbidden)

	created := d.send(t, editor, http.MethodPost, pin, `{"dashboard_id":"`+about.String()+`"}`, http.StatusCreated)
	if created.Header().Get("Location") == "" {
		t.Error("a new pin has no Location")
	}
	d.send(t, editor, http.MethodPost, pin, `{"dashboard_id":"`+about.String()+`"}`, http.StatusOK)

	block := decode[blockEnvelope](t, d.send(t, editor, http.MethodPost, "/api/v1/observations/"+o.ID+"/blocks", `{"kind":"panel","body":`+string(panelBody())+`}`, http.StatusCreated)).Block
	first := decode[struct {
		Dashboard struct {
			ID string `json:"id"`
		} `json:"dashboard"`
	}](t, d.send(t, editor, http.MethodPost, madeAt, `{}`, http.StatusCreated))
	again := d.send(t, editor, http.MethodPost, madeAt, `{}`, http.StatusOK)
	if !strings.Contains(again.Body.String(), first.Dashboard.ID) {
		t.Errorf("a second make answered %s, want dashboard %s", again.Body, first.Dashboard.ID)
	}
	d.send(t, editor, http.MethodPost, madeAt, `{"new":true}`, http.StatusCreated)
	d.send(t, editor, http.MethodPost, "/api/v1/blocks/"+block.ID+"/dashboard", `{}`, http.StatusCreated)
	d.send(t, editor, http.MethodPost, "/api/v1/blocks/"+block.ID+"/dashboard", `{}`, http.StatusOK)
	d.send(t, editor, http.MethodPost, "/api/v1/blocks/"+o.ID+"/dashboard", `{}`, http.StatusNotFound)

	read := decode[envelope](t, d.call(t, editor, http.MethodGet, "/api/v1/observations/"+o.ID, http.StatusOK))
	if read.Observation.DashboardID != nil || read.Observation.Pinned {
		t.Errorf("an observation with no run says dashboard %v pinned %v", read.Observation.DashboardID, read.Observation.Pinned)
	}
}

func TestMakingADashboardAgainReplacesOneThatIsNoLongerVisible(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	r.addPanel(t, editor, o)
	first, _, _, err := r.service.MakeDashboard(reading(t.Context()), editor, id, pgtype.UUID{}, false)
	if err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE dashboards SET folder_id = $2::uuid WHERE id = $1::uuid`, first.ID, r.hiddenFolder(t).String())

	second, version, created, err := r.service.MakeDashboard(reading(t.Context()), editor, id, pgtype.UUID{}, false)
	if err != nil || !created || second.ID == "" || second.ID == first.ID || version.Spec == nil {
		t.Fatalf("make after the first moved away: %q created %v: %v", second.ID, created, err)
	}
	third, _, created, err := r.service.MakeDashboard(reading(t.Context()), editor, id, pgtype.UUID{}, false)
	if err != nil || created || third.ID != second.ID {
		t.Fatalf("make once more: %q created %v: %v", third.ID, created, err)
	}
}

func TestSimultaneousMakesAndPinsGiveOneAnswerAndDeleteNothing(t *testing.T) {
	r := newRig(t)
	about := r.dashboard(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	r.addPanel(t, editor, o)
	r.runOn(t, o, about)
	const callers = 8
	var wg sync.WaitGroup
	dashboardIDs := make([]string, callers)
	annotationIDs := make([]string, callers)
	createdDashboards, createdPins := make([]bool, callers), make([]bool, callers)
	errs := make([]error, 2*callers)
	for i := range callers {
		wg.Add(2)
		go func() {
			defer wg.Done()
			d, _, created, err := r.service.MakeDashboard(reading(t.Context()), editor, id, pgtype.UUID{}, false)
			dashboardIDs[i], createdDashboards[i], errs[i] = d.ID, created, err
		}()
		go func() {
			defer wg.Done()
			p, created, err := r.service.Pin(reading(t.Context()), editor, id, pgtype.UUID{})
			annotationIDs[i], createdPins[i], errs[callers+i] = p.AnnotationID, created, err
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(r.made.specs) != 1 {
		t.Errorf("%d dashboards were made for one request", len(r.made.specs))
	}
	var made, pinned int
	for i := range callers {
		if createdDashboards[i] {
			made++
		}
		if createdPins[i] {
			pinned++
		}
		if dashboardIDs[i] != dashboardIDs[0] || annotationIDs[i] != annotationIDs[0] || dashboardIDs[i] == "" || annotationIDs[i] == "" {
			t.Errorf("caller %d got dashboard %q and annotation %q, caller 0 got %q and %q", i, dashboardIDs[i], annotationIDs[i], dashboardIDs[0], annotationIDs[0])
		}
	}
	if made != 1 || pinned != 1 {
		t.Errorf("%d callers made a dashboard and %d made a pin", made, pinned)
	}
	var left int
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT count(*) FROM annotations`).Scan(&left); err != nil || left != 1 {
		t.Errorf("%d annotations, %v", left, err)
	}
}

func TestAnObservationSaysWhetherTheCallerMayPinIt(t *testing.T) {
	r := newRig(t)
	about := r.dashboard(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	got, err := r.service.Get(reading(t.Context()), editor, id)
	if err != nil || got.CanPin {
		t.Fatalf("an observation without a dashboard: can pin %v: %v", got.CanPin, err)
	}
	r.runOn(t, o, about)
	for who, want := range map[string]bool{"editor": true, "viewer": false} {
		by := map[string]auth.Principal{"editor": editor, "viewer": viewer}[who]
		got, err := r.service.Get(reading(t.Context()), by, id)
		if err != nil || got.CanPin != want || got.DashboardID != about.String() {
			t.Errorf("%s: can pin %v, dashboard %q: %v", who, got.CanPin, got.DashboardID, err)
		}
	}
	r.db.Exec(t, `UPDATE dashboards SET folder_id = $2::uuid WHERE id = $1::uuid`, about.String(), r.hiddenFolder(t).String())
	got, err = r.service.Get(reading(t.Context()), editor, id)
	if err != nil || got.CanPin {
		t.Errorf("a dashboard the caller cannot open: can pin %v: %v", got.CanPin, err)
	}
	if _, _, err := r.service.Pin(reading(t.Context()), editor, id, about); !errors.Is(err, observations.ErrDashboardNotFound) {
		t.Errorf("pin to a dashboard the caller cannot open: %v", err)
	}
}

func TestDeletingAnObservationTakesItsPinOffTheDashboard(t *testing.T) {
	r := newRig(t)
	about := r.dashboard(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	r.runOn(t, o, about)
	r.db.Exec(t, `UPDATE agent_requests SET state = 'failed', status = 'failed', error = 'stopped'`)
	if _, _, err := r.service.Pin(reading(t.Context()), editor, id, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	count := func() (n int) {
		if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT count(*) FROM annotations WHERE dashboard_id = $1::uuid`, about.String()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count() != 1 {
		t.Fatalf("%d annotations after the pin", count())
	}
	if err := r.service.Delete(reading(t.Context()), editor, id); err != nil {
		t.Fatal(err)
	}
	if count() != 0 {
		t.Errorf("%d annotations left after the observation was deleted", count())
	}
}
