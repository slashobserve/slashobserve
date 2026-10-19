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
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const testUserID = "11111111-1111-4111-8111-111111111111"

var testUser = auth.Principal{Kind: auth.KindUser, ID: testUserID, Name: "Ada", Email: "ada@example.com"}

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

type noopReviser struct{}

func (noopReviser) Revise(context.Context, revisions.Request) (revisions.Result, error) {
	return revisions.Result{}, nil
}

type rig struct {
	db        *dbtest.Database
	service   *Service
	comments  *Comments
	revisions *revisions.Service
}

func newRig(t *testing.T, agent bool) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
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
	return &rig{db: database, service: service, comments: NewComments(database.DB, service, queue), revisions: queue}
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
	return rev.ID
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

func TestRequestSaveNeverReplaysAnIdenticalSaveMadeWithoutItsId(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	request := r.queueRevision(t, dash)
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "p99"), "same spec, no request id"); err != nil {
		t.Fatal(err)
	}
	_, _, replayed, err := r.service.AddRequestVersion(t.Context(), testUser, dash, 1, panelsSpec("t", "rps", "p99"), "same spec", request)
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

func TestCommentQueuesARevisionInTheSameTransaction(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t, "rps", "errors")

	comment, err := r.comments.Create(t.Context(), testUser, dash, 1, "errors", "show this per route", true)
	if err != nil {
		t.Fatal(err)
	}
	if comment.Revision == nil || comment.Revision.Status != "queued" {
		t.Fatalf("revision = %+v", comment.Revision)
	}
	items, _, err := r.revisions.List(t.Context(), dash, 10, "")
	if err != nil || len(items) != 1 {
		t.Fatalf("queued = %v, err %v", items, err)
	}
	rev := items[0]
	if rev.ID != comment.Revision.ID || rev.Source != revisions.SourceComment || rev.CommentID != comment.ID || rev.PanelID != "errors" ||
		rev.Prompt != "show this per route" || rev.BaseVersion == nil || *rev.BaseVersion != 1 || rev.RequestedBy != testUser {
		t.Fatalf("revision = %+v", rev)
	}

	listed, _, err := r.comments.List(t.Context(), dash, CommentAll, "", 10, "")
	if err != nil || len(listed) != 1 || listed[0].Revision == nil || listed[0].Revision.ID != rev.ID {
		t.Fatalf("listed = %+v, err %v", listed, err)
	}
	resolved, err := r.comments.SetStatus(t.Context(), testUser, dash, mustParse(t, comment.ID), CommentResolved)
	if err != nil || resolved.Revision == nil || resolved.Revision.ID != rev.ID {
		t.Fatalf("resolved = %+v, err %v", resolved, err)
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

func TestCommentCanOptOutOfARevision(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	comment, err := r.comments.Create(t.Context(), testUser, dash, 1, "rps", "just a note", false)
	if err != nil || comment.Revision != nil {
		t.Fatalf("comment = %+v, err %v", comment, err)
	}
	if items, _, err := r.revisions.List(t.Context(), dash, 10, ""); err != nil || len(items) != 0 {
		t.Fatalf("queued = %v, err %v", items, err)
	}
}

func TestCommentWithoutAnAgentQueuesNothing(t *testing.T) {
	r := newRig(t, false)
	dash := r.createDashboard(t)
	comment, err := r.comments.Create(t.Context(), testUser, dash, 1, "rps", "anything", true)
	if err != nil || comment.Revision != nil {
		t.Fatalf("comment = %+v, err %v", comment, err)
	}
	if items, _, err := r.revisions.List(t.Context(), dash, 10, ""); err != nil || len(items) != 0 {
		t.Fatalf("queued = %v, err %v", items, err)
	}
}

func TestCommentIsNotCreatedWhenTheDashboardIsFull(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	for i := range revisions.MaxActive {
		if _, err := r.comments.Create(t.Context(), testUser, dash, 1, "rps", fmt.Sprintf("note %d", i), true); err != nil {
			t.Fatal(err)
		}
	}
	_, err := r.comments.Create(t.Context(), testUser, dash, 1, "rps", "one too many", true)
	if !errors.Is(err, revisions.ErrTooManyRevisions) {
		t.Fatalf("err = %v", err)
	}
	listed, _, err := r.comments.List(t.Context(), dash, CommentAll, "", 100, "")
	if err != nil || len(listed) != revisions.MaxActive {
		t.Fatalf("%d comments stored, err %v", len(listed), err)
	}
	if _, err := r.comments.Create(t.Context(), testUser, dash, 1, "rps", "plain note", false); err != nil {
		t.Fatalf("a comment that asks for no revision is not capped: %v", err)
	}
}

func TestPeopleCountDashboardsSavedByTheirAgentRequests(t *testing.T) {
	r := newRig(t, true)
	user, err := r.db.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "s", Email: "ada@example.com", Name: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	person := auth.Principal{Kind: auth.KindUser, ID: user.ID.String(), Name: "Ada", Email: "ada@example.com"}
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: "33333333-3333-4333-8333-333333333333", Name: "ci"}

	if _, _, err := r.service.Create(t.Context(), person, panelsSpec("by hand", "rps"), "create"); err != nil {
		t.Fatal(err)
	}
	for _, requester := range []auth.Principal{person, key} {
		request, err := r.revisions.SubmitObserve(t.Context(), requester, "show checkout")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := r.service.CreateRequestDashboard(t.Context(), requester, panelsSpec("by agent", "rps"), "create", request.ID); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := r.db.Queries().ListUsers(t.Context(), dbsqlc.ListUsersParams{RowLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DashboardCount != 2 {
		t.Fatalf("dashboard_count = %+v, want 2: the one saved by hand and the one saved by the person's agent request, not the key's", rows)
	}
}
