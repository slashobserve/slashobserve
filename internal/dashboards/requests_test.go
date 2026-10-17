package dashboards

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const testUserID = "11111111-1111-4111-8111-111111111111"

var testUser = auth.Principal{Kind: auth.KindUser, ID: testUserID, Name: "Ada", Email: "ada@example.com"}

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type fakeDatasource struct{}

func (fakeDatasource) Type() string                                         { return "prometheus" }
func (fakeDatasource) CheckHealth(context.Context) error                    { return nil }
func (fakeDatasource) Lint(context.Context, string) []datasource.Diagnostic { return nil }
func (fakeDatasource) Close() error                                         { return nil }
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

type noopReviser struct{}

func (noopReviser) Revise(context.Context, revisions.Request) (revisions.Result, error) {
	return revisions.Result{}, nil
}

type rig struct {
	db        *dbtest.Database
	service   *Service
	revisions *revisions.Service
}

func newRig(t *testing.T, agent bool) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: fakeDatasource{}})
	if err != nil {
		t.Fatal(err)
	}
	service := New(database.DB, queryproxy.NewRunner(registry, logger), logger)
	var reviser revisions.Reviser
	if agent {
		reviser = noopReviser{}
	}
	queue := revisions.NewService(database.DB, reviser, logger)
	return &rig{db: database, service: service, revisions: queue}
}

func panelsSpec(title string, panelIDs ...string) []byte {
	panels := ""
	for i, id := range panelIDs {
		if i > 0 {
			panels += ","
		}
		panels += fmt.Sprintf(`{"id":%q,"type":"timeseries","title":%q,"gridPos":{"x":0,"y":%d,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"},"query":"up"}`, id, id, i*8)
	}
	return []byte(fmt.Sprintf(`{"specVersion":"0","title":%q,"timeRange":{"from":"now-15m","to":"now"},"panels":[%s]}`, title, panels))
}

func (r *rig) createDashboard(t *testing.T, panelIDs ...string) pgtype.UUID {
	t.Helper()
	if len(panelIDs) == 0 {
		panelIDs = []string{"rps"}
	}
	dash, _, err := r.service.Create(t.Context(), testUser, panelsSpec("t", panelIDs...), "create")
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
	id, err := db.ParseUUID(rev.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRequestVersionIsSavedExactlyOnce(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	request := r.queueRevision(t, dash)

	_, first, replayed, err := r.service.AddRequestVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "p99"), "first", request)
	if err != nil || replayed || first.Number != 2 {
		t.Fatalf("first save: version %d, replayed %t, err %v", first.Number, replayed, err)
	}
	_, again, replayed, err := r.service.AddRequestVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "p99"), "first", request)
	if err != nil || !replayed || again.Number != 2 {
		t.Fatalf("repeat: version %d, replayed %t, err %v", again.Number, replayed, err)
	}
	_, different, replayed, err := r.service.AddRequestVersion(t.Context(), testUser, dash, 2, panelsSpec("t", "rps", "other"), "second try", request)
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
	if _, _, _, err := r.service.AddRequestVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "p99"), "first", request); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, 2, panelsSpec("t", "rps"), "someone else"); err != nil {
		t.Fatal(err)
	}
	_, ver, replayed, err := r.service.AddRequestVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "p99"), "first", request)
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
			_, ver, replayed, err := r.service.AddRequestVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "p99"), "first", request)
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
	if _, _, _, err := r.service.AddRequestVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "p99"), "first", request); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := r.service.AddRequestVersion(t.Context(), testUser, other, 1, panelsSpec("t", "rps", "p99"), "stolen", request)
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a conflict", err)
	}
	if latest, err := r.service.LatestVersion(t.Context(), other); err != nil || latest != 1 {
		t.Fatalf("the other dashboard moved to %d, err %v", latest, err)
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
