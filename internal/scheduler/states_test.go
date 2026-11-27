package scheduler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func stamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func TestStateOfSaysWhenARunDidNotFinish(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	cases := []struct {
		name string
		in   Facts
		want State
	}{
		{"never ran, due at once", Facts{NextRunAt: now, Now: now}, StateDue},
		{"finished, next run ahead", Facts{Started: true, LastStartedAt: ago(time.Minute), Finished: true, LastFinishedAt: ago(time.Second), NextRunAt: now.Add(time.Hour), Now: now}, StateWaiting},
		{"finished, next run passed", Facts{Started: true, LastStartedAt: ago(time.Hour), Finished: true, LastFinishedAt: ago(59 * time.Minute), NextRunAt: ago(time.Second), Now: now}, StateDue},
		{"under a live lease", Facts{Leased: true, LeaseUntil: now.Add(time.Minute), Started: true, LastStartedAt: ago(time.Second), Finished: true, LastFinishedAt: ago(time.Hour), NextRunAt: ago(time.Second), Now: now}, StateRunning},
		{"crashed, lease ran out", Facts{Leased: true, LeaseUntil: ago(time.Second), Started: true, LastStartedAt: ago(11 * time.Minute), Finished: true, LastFinishedAt: ago(time.Hour), NextRunAt: ago(time.Minute), Now: now}, StateInterrupted},
		{"first run crashed", Facts{Leased: true, LeaseUntil: ago(time.Second), Started: true, LastStartedAt: ago(11 * time.Minute), NextRunAt: ago(time.Minute), Now: now}, StateInterrupted},
		{"shut down mid-run, lease given back", Facts{Started: true, LastStartedAt: ago(time.Minute), Finished: true, LastFinishedAt: ago(time.Hour), NextRunAt: ago(time.Minute), Now: now}, StateInterrupted},
	}
	for _, c := range cases {
		got := StateOf(c.in)
		if got != c.want {
			t.Errorf("%s: state = %q, want %q", c.name, got, c.want)
		}
		if !States.Has(string(got)) {
			t.Errorf("%s: %q is not a declared state", c.name, got)
		}
	}
}

func TestAListedJobCarriesItsState(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	got := newJobJSON(dbsqlc.ListJobsRow{
		Name: "activity_prune", EverySeconds: 3600, NextRunAt: stamp(now.Add(-time.Minute)),
		LeasedBy: "crashed-server", LeaseUntil: stamp(now.Add(-time.Second)), LastStartedAt: stamp(now.Add(-11 * time.Minute)),
		LastFinishedAt: stamp(now.Add(-time.Hour)), LastOutcome: OutcomeSucceeded, ReadAt: stamp(now),
	})
	if got.State != StateInterrupted || got.Name != "activity_prune" || got.EverySeconds != 3600 || got.LastError != nil || got.LastOutcome == nil {
		t.Errorf("job = %+v, want an interrupted activity_prune", got)
	}
	never := newJobJSON(dbsqlc.ListJobsRow{Name: "new", EverySeconds: 60, NextRunAt: stamp(now), ReadAt: stamp(now)})
	if never.State != StateDue || never.LastStartedAt != nil || never.LastFinishedAt != nil || never.LastOutcome != nil {
		t.Errorf("a job that never ran = %+v, want due with no last run", never)
	}
}

func TestTakingOverAnExpiredLeaseRecordsTheCrashedRunAsInterrupted(t *testing.T) {
	d := dbtest.New(t)
	d.Exec(t, `INSERT INTO jobs (name, every_seconds, next_run_at, leased_by, lease_until, last_started_at, last_finished_at, last_outcome, last_error)
		VALUES ('a-crashed', 600, now() - interval '11 minutes', 'crashed-server', now() - interval '1 second', now() - interval '601 seconds', now() - interval '1 hour', 'failed', 'code=boom')`)
	d.Exec(t, `INSERT INTO jobs (name, every_seconds, next_run_at, last_started_at, last_finished_at, last_outcome)
		VALUES ('b-finished', 600, now() - interval '1 minute', now() - interval '11 minutes', now() - interval '10 minutes', 'succeeded')`)
	s, _ := newScheduler(t, d, "server-b")

	for _, name := range []string{"a-crashed", "b-finished"} {
		if _, err := s.store.claim(t.Context(), []string{name}, false); err != nil {
			t.Fatalf("claim %s: %v", name, err)
		}
	}
	if j := row(t, d, "a-crashed"); j.LastOutcome != OutcomeInterrupted || j.LastError != "" || j.LeasedBy != "server-b" {
		t.Errorf("row after the takeover %+v, want the crashed run recorded as interrupted", j)
	}
	if j := row(t, d, "b-finished"); j.LastOutcome != OutcomeSucceeded {
		t.Errorf("claiming a finished job changed its outcome to %q", j.LastOutcome)
	}
}

func listJobsAs(t *testing.T, d *dbtest.Database, by auth.Principal) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), by))
	})
	NewHandler(d.DB, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil))
	return rec
}

func TestAnAdminListsEveryJobWithItsState(t *testing.T) {
	d := dbtest.New(t)
	d.Exec(t, `INSERT INTO jobs (name, every_seconds, next_run_at, leased_by, lease_until, last_started_at)
		VALUES ('a-crashed', 600, now() - interval '11 minutes', 'crashed-server', now() - interval '1 second', now() - interval '601 seconds')`)
	d.Exec(t, `INSERT INTO jobs (name, every_seconds, next_run_at, last_started_at, last_finished_at, last_outcome)
		VALUES ('b-finished', 600, now() + interval '9 minutes', now() - interval '1 minute', now() - interval '59 seconds', 'succeeded')`)

	rec := listJobsAs(t, d, auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Role: auth.RoleAdmin})
	var body struct {
		Jobs []struct {
			Name  string `json:"name"`
			State State  `json:"state"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	if len(body.Jobs) != 2 || body.Jobs[0].Name != "a-crashed" || body.Jobs[0].State != StateInterrupted || body.Jobs[1].State != StateWaiting {
		t.Fatalf("jobs = %s, want the crashed one interrupted and the finished one waiting", rec.Body)
	}
	if rec := listJobsAs(t, d, auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Role: auth.RoleEditor}); rec.Code != http.StatusForbidden {
		t.Fatalf("an Editor's list = %d, want 403", rec.Code)
	}
}
