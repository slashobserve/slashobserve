package dashboards

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const testUserID = "11111111-1111-4111-8111-111111111111"

var testUser = auth.Principal{Kind: auth.KindUser, ID: testUserID, Name: "Ada", Email: "ada@example.com", Role: auth.RoleEditor}

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type fakeDatasource struct{}

func (fakeDatasource) Type() string                      { return "prometheus" }
func (fakeDatasource) CheckHealth(context.Context) error { return nil }
func (fakeDatasource) Lint(context.Context, string) ([]datasource.Diagnostic, error) {
	return nil, nil
}
func (fakeDatasource) Close() error { return nil }
func (fakeDatasource) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}

func (fakeDatasource) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return []string{"v"}, nil
}

func (fakeDatasource) QueryRange(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
	return fakeFrames(), nil
}

func (fakeDatasource) QueryInstant(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
	return fakeFrames(), nil
}

func fakeFrames() []datasource.Frame {
	return []datasource.Frame{{Fields: []datasource.Field{
		{Name: "time", Type: datasource.FieldTime, Values: []any{1.0}},
		{Name: "value", Type: datasource.FieldNumber, Values: []any{2.0}},
	}}}
}

type rig struct {
	db        *dbtest.Database
	service   *Service
	revisions *revisions.Service
}

func (r *rig) saveRequest(t *testing.T, id pgtype.UUID, parent int, raw []byte, summary string, request pgtype.UUID) (Dashboard, Version, bool, error) {
	t.Helper()
	res, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: id, Parent: parent, Spec: raw, Summary: summary, RequestID: request})
	return res.Dashboard, res.Version, res.Replayed, err
}

func (r *rig) saveChangedRequest(t *testing.T, id pgtype.UUID, parent int, raw []byte, summary string, request pgtype.UUID) (Dashboard, Version, bool, error) {
	t.Helper()
	res, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: id, Parent: parent, Spec: raw, Summary: summary, RequestID: request, Partial: true})
	return res.Dashboard, res.Version, res.Replayed, err
}

func newRig(t *testing.T, agent bool) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: fakeDatasource{}}, datasource.Instance{UID: "ch", Datasource: fakeDatasource{}})
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	service := New(database.DB, queryproxy.NewRunner(registry, logger), access, logger)
	revisionOpts := []revisions.Option{revisions.WithAccess(access)}
	if agent {
		revisionOpts = append(revisionOpts, revisions.WithBuiltInAgent())
	}
	queue := revisions.NewService(database.DB, logger, revisionOpts...)
	return &rig{db: database, service: service, revisions: queue}
}

func panelsSpec(title string, panelIDs ...string) []byte {
	panels := ""
	for i, id := range panelIDs {
		if i > 0 {
			panels += ","
		}
		panels += fmt.Sprintf(`{"id":%q,"type":"timeseries","title":%q,"gridPos":{"x":0,"y":%d,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]}`, id, id, i*8)
	}
	return []byte(fmt.Sprintf(`{"specVersion":"0","title":%q,"time":{"from":"now-15m","to":"now"},"panels":[%s]}`, title, panels))
}

func (r *rig) createDashboard(t *testing.T, panelIDs ...string) pgtype.UUID {
	t.Helper()
	if len(panelIDs) == 0 {
		panelIDs = []string{"rps"}
	}
	dash, _, err := r.service.Create(t.Context(), testUser, pgtype.UUID{}, panelsSpec("t", panelIDs...), "create")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) queueRevision(t *testing.T, dash pgtype.UUID) pgtype.UUID {
	t.Helper()
	rev, err := r.revisions.Submit(t.Context(), revisions.EnqueueParams{
		DashboardID: dash, Source: revisions.SourcePrompt, Prompt: "revise", BaseVersion: 1, By: testUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rev.ID
}

func TestRequestVersionIsSavedExactlyOnce(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	request := r.queueRevision(t, dash)

	_, first, replayed, err := r.saveRequest(t, dash, 1, panelsSpec("t", "rps", "p99"), "first", request)
	if err != nil || replayed || first.Number != 2 {
		t.Fatalf("first save: version %d, replayed %t, err %v", first.Number, replayed, err)
	}
	_, again, replayed, err := r.saveRequest(t, dash, 1, panelsSpec("t", "rps", "p99"), "first", request)
	if err != nil || !replayed || again.Number != 2 {
		t.Fatalf("repeat: version %d, replayed %t, err %v", again.Number, replayed, err)
	}
	_, different, replayed, err := r.saveRequest(t, dash, 2, panelsSpec("t", "rps", "other"), "second try", request)
	if err != nil || !replayed || different.Number != 2 || len(different.Spec.Panels) != 2 || different.Spec.Panels[1].ID != "p99" {
		t.Fatalf("second attempt with another spec: version %d, replayed %t, err %v", different.Number, replayed, err)
	}
	if latest, err := r.service.LatestVersion(t.Context(), dash); err != nil || latest != 2 {
		t.Fatalf("latest = %d, err %v", latest, err)
	}
}

func TestRequestVersionReplaysEvenWhenTheDashboardMovedOn(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	request := r.queueRevision(t, dash)
	if _, _, _, err := r.saveRequest(t, dash, 1, panelsSpec("t", "rps", "p99"), "first", request); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, 2, panelsSpec("t", "rps"), "someone else"); err != nil {
		t.Fatal(err)
	}
	_, ver, replayed, err := r.saveRequest(t, dash, 1, panelsSpec("t", "rps", "p99"), "first", request)
	if err != nil || !replayed || ver.Number != 2 {
		t.Fatalf("version %d, replayed %t, err %v", ver.Number, replayed, err)
	}
}

func TestConcurrentSavesForOneRequestCreateOneVersion(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	request := r.queueRevision(t, dash)

	var created, replays atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ver, replayed, err := r.saveRequest(t, dash, 1, panelsSpec("t", "rps", "p99"), "first", request)
			switch {
			case err != nil:
				t.Errorf("save: %v", err)
			case ver.Number != 2:
				t.Errorf("version = %d, want 2", ver.Number)
			case replayed:
				replays.Add(1)
			default:
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 || replays.Load() != 7 {
		t.Fatalf("%d saves created a version and %d replayed", created.Load(), replays.Load())
	}
	if latest, err := r.service.LatestVersion(t.Context(), dash); err != nil || latest != 2 {
		t.Fatalf("latest = %d, err %v", latest, err)
	}
}

func TestRequestIdOfAnotherDashboardIsAConflictNotAReplay(t *testing.T) {
	r := newRig(t, true)
	dash, other := r.createDashboard(t), r.createDashboard(t)
	request := r.queueRevision(t, dash)
	if _, _, _, err := r.saveRequest(t, dash, 1, panelsSpec("t", "rps", "p99"), "first", request); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := r.saveRequest(t, other, 1, panelsSpec("t", "rps", "p99"), "stolen", request)
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a conflict", err)
	}
	if latest, err := r.service.LatestVersion(t.Context(), other); err != nil || latest != 1 {
		t.Fatalf("the other dashboard moved to %d, err %v", latest, err)
	}
}

func TestRequestSaveNeverReplaysAnIdenticalSaveMadeWithoutItsId(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	request := r.queueRevision(t, dash)
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "p99"), "same spec, no request id"); err != nil {
		t.Fatal(err)
	}
	_, _, replayed, err := r.saveRequest(t, dash, 1, panelsSpec("t", "rps", "p99"), "same spec", request)
	var conflict *ConflictError
	if !errors.As(err, &conflict) || replayed || conflict.Latest != 2 {
		t.Fatalf("replayed %t, err %v, want a conflict at version 2", replayed, err)
	}
}

func TestSavesWithoutARequestIdAreUnchanged(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	for want := 2; want <= 3; want++ {
		_, ver, replayed, err := r.service.AddVersion(t.Context(), testUser, dash, want-1, panelsSpec("t", "rps", fmt.Sprintf("p%d", want)), "next")
		if err != nil || replayed || ver.Number != want {
			t.Fatalf("version %d, replayed %t, err %v", ver.Number, replayed, err)
		}
	}
}

func TestHasPanelLooksInTheGivenVersionAndMapsAMissingDashboard(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t, "rps", "errors")

	for panel, want := range map[string]bool{"errors": true, "rps": true, "gone": false} {
		if got, err := r.service.HasPanel(t.Context(), dash, 1, panel); err != nil || got != want {
			t.Errorf("HasPanel(%q) = %v, %v, want %v", panel, got, err, want)
		}
	}
	if _, err := r.service.HasPanel(t.Context(), dash, 2, "rps"); !errors.Is(err, revisions.ErrDashboardNotFound) {
		t.Errorf("a missing version gave %v, want revisions.ErrDashboardNotFound", err)
	}
	missing := mustParse(t, "00000000-0000-4000-8000-000000000000")
	if _, err := r.service.HasPanel(t.Context(), missing, 1, "rps"); !errors.Is(err, revisions.ErrDashboardNotFound) {
		t.Errorf("a missing dashboard gave %v, want revisions.ErrDashboardNotFound", err)
	}
}

func mustParse(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPeopleCountDashboardsSavedByTheirAgentRequests(t *testing.T) {
	r := newRig(t, true)
	user, err := r.db.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "s", Email: "ada@example.com", Name: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	person := auth.Principal{Kind: auth.KindUser, ID: user.ID.String(), Name: "Ada", Email: "ada@example.com", Role: auth.RoleEditor}
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: "33333333-3333-4333-8333-333333333333", Name: "ci", Role: auth.RoleEditor}

	if _, _, err := r.service.Create(t.Context(), person, pgtype.UUID{}, panelsSpec("by hand", "rps"), "create"); err != nil {
		t.Fatal(err)
	}
	for _, requester := range []auth.Principal{person, key} {
		request, err := r.revisions.SubmitObserve(t.Context(), requester, "show checkout")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := r.service.CreateRequestDashboard(t.Context(), requester, pgtype.UUID{}, panelsSpec("by agent", "rps"), "create", request.ID); err != nil {
			t.Fatal(err)
		}
	}

	levels, err := folders.NewAccess(r.db.DB).Levels(t.Context(), person)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := r.db.Queries().ListUsers(t.Context(), dbsqlc.ListUsersParams{RowLimit: 10, Folders: levels.Visible()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DashboardCount != 2 {
		t.Fatalf("dashboard_count = %+v, want 2: the one saved by hand and the one saved by the person's agent request, not the key's", rows)
	}
}

func TestDashboardCreatedUnderARequestRecordsTheRun(t *testing.T) {
	r := newRig(t, true)
	request, err := r.revisions.SubmitObserve(t.Context(), testUser, "show checkout")
	if err != nil {
		t.Fatal(err)
	}
	created, _, _, err := r.service.CreateRequestDashboard(t.Context(), testUser, pgtype.UUID{}, panelsSpec("by agent", "rps"), "create", request.ID)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := r.service.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Creator.RunID != request.ID.String() {
		t.Fatalf("Creator.RunID = %q, want %q", got.Creator.RunID, request.ID.String())
	}

	direct, _, err := r.service.Create(t.Context(), testUser, pgtype.UUID{}, panelsSpec("by hand", "rps"), "create")
	if err != nil {
		t.Fatal(err)
	}
	if direct.Creator.RunID != "" {
		t.Fatalf("direct Creator.RunID = %q, want empty", direct.Creator.RunID)
	}
}
