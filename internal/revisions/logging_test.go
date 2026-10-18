package revisions_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/logtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	leakedPrompt = "show me the payroll dashboard for bob@example.com"
	leakedDetail = `password authentication failed for user "bob" at db.internal`
)

func TestFailedRunLogsOnlyCodeTypeAndIds(t *testing.T) {
	logger, logs := logtest.New()
	d := dbtest.New(t)
	svc := revisions.NewService(d.DB, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		return revisions.Result{}, errors.New("tool result: " + leakedDetail)
	}), logger)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, leakedPrompt)
	startWorkerLogged(t, svc, d, revisions.WorkerOptions{}, logger)
	failed := waitForStatus(t, svc, dash, rev.ID, revisions.StatusFailed)
	if failed.Error != "tool result: "+leakedDetail {
		t.Fatalf("stored error = %q", failed.Error)
	}

	record := findEventually(t, logs, "agent request failed")
	logs.AssertSafeFailure(t, record, "agent_run_failed", leakedDetail, leakedPrompt, "tool result")
	if record["agent_request_id"] != rev.ID || record["dashboard_id"] != dash.String() || record["attempt"] != float64(1) {
		t.Errorf("record lacks the ids: %v", record)
	}
}

func TestTimedOutRunLogsOnlyCodeAndType(t *testing.T) {
	logger, logs := logtest.New()
	d := dbtest.New(t)
	svc := revisions.NewService(d.DB, reviserFunc(func(ctx context.Context, _ revisions.Request) (revisions.Result, error) {
		<-ctx.Done()
		return revisions.Result{}, ctx.Err()
	}), logger)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, leakedPrompt)
	startWorkerLogged(t, svc, d, revisions.WorkerOptions{RunTimeout: 100 * time.Millisecond}, logger)
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusFailed)

	record := findEventually(t, logs, "agent request failed")
	logs.AssertSafeFailure(t, record, "agent_run_timed_out", leakedPrompt, "timed out")
	if record["agent_request_id"] != rev.ID {
		t.Errorf("record = %v", record)
	}
}

func TestPanicAfterTheDeadlineLogsAsPanickedAndStoresTheTimeoutMessage(t *testing.T) {
	logger, logs := logtest.New()
	d := dbtest.New(t)
	svc := revisions.NewService(d.DB, reviserFunc(func(ctx context.Context, _ revisions.Request) (revisions.Result, error) {
		<-ctx.Done()
		panic(leakedDetail)
	}), logger)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, leakedPrompt)
	startWorkerLogged(t, svc, d, revisions.WorkerOptions{RunTimeout: 100 * time.Millisecond}, logger)
	failed := waitForStatus(t, svc, dash, rev.ID, revisions.StatusFailed)
	if failed.Error != "the agent run timed out" {
		t.Fatalf("stored error = %q", failed.Error)
	}

	record := findEventually(t, logs, "agent request failed")
	logs.AssertSafeFailure(t, record, "agent_run_panicked", leakedPrompt, leakedDetail)
}

func TestStoreFailuresAfterARunLogOnlyCodeAndType(t *testing.T) {
	logger, logs := logtest.New()
	d := dbtest.New(t)
	svc := revisions.NewService(d.DB, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		d.Close()
		return revisions.Result{}, errors.New(leakedDetail)
	}), logger)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, leakedPrompt)
	startWorkerLogged(t, svc, d, revisions.WorkerOptions{}, logger)

	for msg, code := range map[string]string{
		"check agent request result": "agent_request_check_failed",
		"finish agent request":       "agent_request_finish_failed",
	} {
		record := findEventually(t, logs, msg)
		logs.AssertSafeFailure(t, record, code, "closed pool", leakedDetail, leakedPrompt)
		if record["agent_request_id"] != rev.ID {
			t.Errorf("%s: record = %v", msg, record)
		}
	}
}

func TestListenerFailureLogsOnlyCodeAndType(t *testing.T) {
	logger, logs := logtest.New()
	d := dbtest.New(t)
	svc := revisions.NewService(d.DB, succeedWith(2), logger)
	listen := func(ctx context.Context, _ string, _ func(), _ func(string)) error {
		if ctx.Err() == nil {
			return &pgconn.PgError{Code: "08006", Message: leakedDetail}
		}
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		revisions.NewWorker(svc, listen, logger, revisions.WorkerOptions{Poll: time.Hour}).Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	record := waitForLog(t, logs, "agent request listener failed")[0]
	logs.AssertSafeFailure(t, record, "agent_request_listener_failed", leakedDetail, "db.internal")
	if record["sqlstate"] != "08006" {
		t.Errorf("record = %v", record)
	}
}

func TestRecordStepFailureLogsOnlyCodeTypeAndIds(t *testing.T) {
	logger, logs := logtest.New()
	d := dbtest.New(t)
	svc := revisions.NewService(d.DB, succeedWith(1), quietLogger())
	row, err := svc.SubmitObserve(context.Background(), user, leakedPrompt)
	if err != nil {
		t.Fatal(err)
	}
	steps := revisions.NewSteps(d.DB, logger)
	d.Close()

	steps.RecordStep(context.Background(), revisions.Request{ID: row.ID.String(), Attempt: 1}, revisions.Step{
		At: time.Now(), Phase: "query", Tool: "query_range", Status: "ok", Message: leakedDetail,
	})

	record := logs.Find(t, "record agent request step")
	logs.AssertSafeFailure(t, record, "agent_step_record_failed", leakedDetail, leakedPrompt, "closed pool")
	if record["agent_request_id"] != row.ID.String() || record["tool"] != "query_range" {
		t.Errorf("record lacks the ids: %v", record)
	}
}

func TestInternalHTTPFailureLogsOnlyCodeTypeAndIds(t *testing.T) {
	logger, logs := logtest.New()
	d := dbtest.New(t)
	svc := revisions.NewService(d.DB, succeedWith(2), logger)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, leakedPrompt)
	r := newRouter(svc)
	d.Close()

	rec := call(r, http.MethodGet, revisionsPath(dash)+"/"+rev.ID, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500: %s", rec.Code, rec.Body)
	}
	record := logs.Find(t, "get revision")
	logs.AssertSafeFailure(t, record, "internal", "closed pool", leakedPrompt)
	if record["dashboard_id"] != dash.String() || record["agent_request_id"] != rev.ID ||
		record["principal_id"] != userID || record["principal_kind"] != string(auth.KindUser) {
		t.Errorf("record lacks the ids: %v", record)
	}
}

func TestInternalObserveFailureLogsTheAgentRequestId(t *testing.T) {
	logger, logs := logtest.New()
	d := dbtest.New(t)
	svc := revisions.NewService(d.DB, succeedWith(2), logger)
	row, err := svc.SubmitObserve(context.Background(), user, leakedPrompt)
	if err != nil {
		t.Fatal(err)
	}
	engine := newObserveRouter(svc)
	d.Close()

	rec := call(engine, http.MethodGet, "/api/v1/observe/"+row.ID.String(), "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500: %s", rec.Code, rec.Body)
	}
	record := logs.Find(t, "get agent request")
	logs.AssertSafeFailure(t, record, "internal", "closed pool", leakedPrompt)
	if record["agent_request_id"] != row.ID.String() || record["dashboard_id"] != nil {
		t.Errorf("record = %v", record)
	}
}

func TestUndecodableStoredStepsAndUsageLogOnlyCodeAndType(t *testing.T) {
	logger, logs := logtest.New()
	d := dbtest.New(t)
	svc := revisions.NewService(d.DB, succeedWith(2), logger)
	row, err := svc.SubmitObserve(context.Background(), user, leakedPrompt)
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `UPDATE agent_requests SET steps = '[{"phase": 5}]', run_usage = '{"model": "x", "input_tokens": "leaked-usage"}' WHERE id = $1`, row.ID)
	engine := newObserveRouter(svc)

	rec := call(engine, http.MethodGet, "/api/v1/observe/"+row.ID.String(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body)
	}
	for msg, code := range map[string]string{
		"decode agent request steps": "agent_request_steps_decode_failed",
		"decode agent request usage": "agent_request_usage_decode_failed",
	} {
		record := logs.Find(t, msg)
		logs.AssertSafeFailure(t, record, code, "leaked-usage", leakedPrompt, "cannot unmarshal")
		if record["agent_request_id"] != row.ID.String() {
			t.Errorf("%s: record = %v", msg, record)
		}
	}
}
