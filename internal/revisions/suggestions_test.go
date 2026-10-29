package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const suggestedPanel = `{"id":"p1","type":"stat","title":"Proposed"}`

const baseSpec = `{
  "specVersion": "0",
  "title": "Base",
  "time": { "from": "now-15m", "to": "now" },
  "panels": [
    { "id": "p1", "type": "stat", "title": "Log rows",
      "gridPos": { "x": 0, "y": 0, "w": 6, "h": 4 },
      "datasource": "ch",
      "queries": [{ "query": "SELECT count() FROM otel_logs WHERE $__timeFilter(Timestamp)" }] }
  ]
}`

type fixedSpecs struct{ dashboard *spec.Dashboard }

func (f fixedSpecs) VersionSpec(context.Context, pgtype.UUID, int) (*spec.Dashboard, error) {
	return f.dashboard, nil
}

func newSuggestService(t *testing.T, reviser revisions.Reviser) (*revisions.Service, *dbtest.Database) {
	t.Helper()
	base, err := spec.Parse([]byte(baseSpec))
	if err != nil {
		t.Fatal(err)
	}
	d := dbtest.New(t)
	return newTestService(d.DB, reviser, quietLogger(), revisions.WithSpecs(fixedSpecs{dashboard: base})), d
}

func suggestFor(t *testing.T, svc *revisions.Service, dash pgtype.UUID, who auth.Principal) dbsqlc.AgentRequest {
	t.Helper()
	row, err := svc.SubmitSuggestion(context.Background(), dash, 1, "p1", "make it better", nil, who)
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	return row
}

func getSuggestion(t *testing.T, svc *revisions.Service, dash pgtype.UUID, id pgtype.UUID, who auth.Principal) dbsqlc.AgentRequest {
	t.Helper()
	row, err := svc.GetSuggestion(context.Background(), dash, id, who)
	if err != nil {
		t.Fatalf("get suggestion: %v", err)
	}
	return row
}

func waitForSuggestion(t *testing.T, svc *revisions.Service, dash, id pgtype.UUID, who auth.Principal, want revisions.Status) dbsqlc.AgentRequest {
	t.Helper()
	var row dbsqlc.AgentRequest
	waitFor(t, fmt.Sprintf("suggestion %s to be %s", id, want), func() bool {
		row = getSuggestion(t, svc, dash, id, who)
		return row.Status == string(want)
	})
	return row
}

func markRunning(t *testing.T, d *dbtest.Database, id pgtype.UUID) {
	t.Helper()
	d.Exec(t, "UPDATE agent_requests SET status = 'running', attempts = 1, started_at = clock_timestamp(), lease_until = clock_timestamp() + interval '1 hour' WHERE id = $1", id)
}

func sameJSON(t *testing.T, got []byte, want string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(a, b)
}

func principalN(i int) auth.Principal {
	return auth.Principal{Kind: auth.KindServiceAccount, ID: fmt.Sprintf("44444444-4444-4444-8444-%012d", i), Name: fmt.Sprintf("account %d", i)}
}

func proposing(panel string) reviserFunc {
	return func(_ context.Context, req revisions.Request) (revisions.Result, error) {
		if req.Source == revisions.SourceSuggest {
			return revisions.Result{Suggestion: []byte(panel)}, nil
		}
		return revisions.Result{Version: 2}, nil
	}
}

func TestSuggestionIsClaimedWhileARevisionRunsOnTheSameDashboard(t *testing.T) {
	var ran atomic.Int32
	svc, d := newSuggestService(t, reviserFunc(func(ctx context.Context, req revisions.Request) (revisions.Result, error) {
		ran.Add(1)
		return proposing(suggestedPanel)(ctx, req)
	}))
	dash := seedDashboard(t, d, 1)
	running := submit(t, svc, dash, "a revision that is mid-run")
	markRunning(t, d, mustUUID(t, running.ID))
	sug := suggestFor(t, svc, dash, user)

	startWorker(t, svc, d, revisions.WorkerOptions{})
	done := waitForSuggestion(t, svc, dash, sug.ID, user, revisions.StatusSucceeded)
	if !sameJSON(t, done.Suggestion, suggestedPanel) || done.ResultVersion.Valid || ran.Load() != 1 {
		t.Fatalf("suggestion = %+v after %d runs", done, ran.Load())
	}
	if got := get(t, svc, dash, running.ID); got.Status != revisions.StatusRunning {
		t.Fatalf("the running revision changed: %+v", got)
	}
}

func TestRevisionIsClaimedWhileASuggestionRunsOnTheSameDashboard(t *testing.T) {
	svc, d := newSuggestService(t, proposing(suggestedPanel))
	dash := seedDashboard(t, d, 1)
	sug := suggestFor(t, svc, dash, user)
	markRunning(t, d, sug.ID)
	rev := submit(t, svc, dash, "a revision next to a running suggestion")

	startWorker(t, svc, d, revisions.WorkerOptions{})
	done := waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)
	if done.ResultVersion == nil || *done.ResultVersion != 2 {
		t.Fatalf("revision = %+v", done)
	}
	if got := getSuggestion(t, svc, dash, sug.ID, user); got.Status != string(revisions.StatusRunning) {
		t.Fatalf("the running suggestion changed: %+v", got)
	}
}

func TestSuggestionsDoNotCountTowardTheDashboardRevisionCap(t *testing.T) {
	svc, d := newSuggestService(t, proposing(suggestedPanel))
	dash := seedDashboard(t, d, 1)
	suggestFor(t, svc, dash, user)
	suggestFor(t, svc, dash, user)
	for i := range revisions.MaxActive {
		submit(t, svc, dash, fmt.Sprintf("p%d", i))
	}
	_, err := svc.Submit(context.Background(), revisions.EnqueueParams{DashboardID: dash, Source: revisions.SourcePrompt, Prompt: "one too many", BaseVersion: 1, By: user})
	if !errors.Is(err, revisions.ErrTooManyRevisions) {
		t.Fatalf("sixth revision: %v", err)
	}
	items, _, err := svc.List(context.Background(), dash, 100, "")
	if err != nil || len(items) != revisions.MaxActive {
		t.Fatalf("revisions list holds %d items, err %v; suggestions must stay out of it", len(items), err)
	}
}

func TestSuggestionsAreCappedPerPrincipalAndPerInstance(t *testing.T) {
	svc, d := newSuggestService(t, proposing(suggestedPanel))
	dash := seedDashboard(t, d, 1)
	for range revisions.MaxActiveSuggestPerPrincipal {
		suggestFor(t, svc, dash, user)
	}
	_, err := svc.SubmitSuggestion(context.Background(), dash, 1, "p1", "third", nil, user)
	if !errors.Is(err, revisions.ErrTooManySuggestions) {
		t.Fatalf("third suggestion of one principal: %v", err)
	}

	for i := range (revisions.MaxActiveSuggest - revisions.MaxActiveSuggestPerPrincipal) / revisions.MaxActiveSuggestPerPrincipal {
		for range revisions.MaxActiveSuggestPerPrincipal {
			suggestFor(t, svc, dash, principalN(i))
		}
	}
	_, err = svc.SubmitSuggestion(context.Background(), dash, 1, "p1", "eleventh", nil, principalN(99))
	if !errors.Is(err, revisions.ErrAgentBusy) {
		t.Fatalf("eleventh suggestion of the instance: %v", err)
	}

	var stored int
	if err := d.SQLDB(t).QueryRow("SELECT count(*) FROM agent_requests WHERE source = 'suggest'").Scan(&stored); err != nil || stored != revisions.MaxActiveSuggest {
		t.Fatalf("stored %d suggestions, err %v", stored, err)
	}
}

func TestExhaustedLeaseFailsASuggestionWithoutRunningItAgain(t *testing.T) {
	var calls atomic.Int32
	svc, d := newSuggestService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		calls.Add(1)
		return revisions.Result{Suggestion: []byte(suggestedPanel)}, nil
	}))
	dash := seedDashboard(t, d, 1)
	sug := suggestFor(t, svc, dash, user)
	d.Exec(t, "UPDATE agent_requests SET status = 'running', attempts = 2, started_at = clock_timestamp(), lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", sug.ID)

	startWorker(t, svc, d, revisions.WorkerOptions{})
	failed := waitForSuggestion(t, svc, dash, sug.ID, user, revisions.StatusFailed)
	if failed.Error.String != revisions.ExhaustedMessage || failed.ResultVersion.Valid || failed.Suggestion != nil || calls.Load() != 0 {
		t.Fatalf("exhausted suggestion = %+v after %d calls", failed, calls.Load())
	}
}

func TestSuggestionRowsMustHoldAnObjectWhenTheyHaveSucceeded(t *testing.T) {
	svc, d := newSuggestService(t, proposing(suggestedPanel))
	dash := seedDashboard(t, d, 1)
	sqlDB := d.SQLDB(t)
	cases := []struct {
		name       string
		suggestion any
		constraint string
	}{
		{"no suggestion", nil, "agent_requests_succeeded_check"},
		{"JSON null", `null`, "agent_requests_suggestion_check"},
		{"an array", `[]`, "agent_requests_suggestion_check"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sug := suggestFor(t, svc, dash, principalN(len(tc.name)))
			_, err := sqlDB.Exec("UPDATE agent_requests SET status = 'succeeded', suggestion = $2::jsonb WHERE id = $1", sug.ID.String(), tc.suggestion)
			if err == nil || !strings.Contains(err.Error(), tc.constraint) {
				t.Fatalf("err = %v, want a %s violation", err, tc.constraint)
			}
		})
	}
	sug := suggestFor(t, svc, dash, principalN(100))
	if _, err := sqlDB.Exec("UPDATE agent_requests SET status = 'succeeded', suggestion = $2::jsonb WHERE id = $1", sug.ID.String(), `{}`); err != nil {
		t.Fatalf("a succeeded suggestion with an object: %v", err)
	}
}

func TestSuggestionIsNotVisibleToAnotherPrincipalWithTheSameIdAndADifferentKind(t *testing.T) {
	svc, d := newSuggestService(t, proposing(suggestedPanel))
	dash, other := seedDashboard(t, d, 1), seedDashboard(t, d, 1)
	sug := suggestFor(t, svc, dash, user)

	getSuggestion(t, svc, dash, sug.ID, user)
	for name, who := range map[string]auth.Principal{"same id, other kind": lookalike, "other id, same kind": {Kind: auth.KindUser, ID: strangerID}} {
		if _, err := svc.GetSuggestion(context.Background(), dash, sug.ID, who); !errors.Is(err, revisions.ErrRequestNotFound) {
			t.Errorf("%s: err = %v, want ErrRequestNotFound", name, err)
		}
	}
	if _, err := svc.GetSuggestion(context.Background(), other, sug.ID, user); !errors.Is(err, revisions.ErrRequestNotFound) {
		t.Errorf("another dashboard: err = %v, want ErrRequestNotFound", err)
	}
}

func TestSuggestionThatIsNotAnObjectFailsAsNoSuggestion(t *testing.T) {
	proposals := map[string]string{
		"null":          `null`,
		"empty":         ``,
		"blank":         ` `,
		"array":         `[{"id":"p1"}]`,
		"string":        `"p1"`,
		"cut off":       `{"id":`,
		"object, space": " \n" + suggestedPanel + "\n",
	}
	var next atomic.Value
	svc, d := newSuggestService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		return revisions.Result{Suggestion: []byte(next.Load().(string))}, nil
	}))
	dash := seedDashboard(t, d, 1)
	startWorker(t, svc, d, revisions.WorkerOptions{Workers: 1})

	for name, proposal := range proposals {
		t.Run(name, func(t *testing.T) {
			next.Store(proposal)
			sug := suggestFor(t, svc, dash, user)
			if name == "object, space" {
				done := waitForSuggestion(t, svc, dash, sug.ID, user, revisions.StatusSucceeded)
				if !sameJSON(t, done.Suggestion, suggestedPanel) {
					t.Fatalf("stored suggestion = %s", done.Suggestion)
				}
				return
			}
			failed := waitForSuggestion(t, svc, dash, sug.ID, user, revisions.StatusFailed)
			if failed.Error.String != "the agent did not propose a panel" || failed.Suggestion != nil {
				t.Fatalf("suggestion = %+v", failed)
			}
		})
	}
}

func draftWithCompactSize(t *testing.T, base *spec.Dashboard, size int) json.RawMessage {
	t.Helper()
	build := func(last int) json.RawMessage {
		queries := []map[string]string{}
		for range 3 {
			queries = append(queries, map[string]string{"query": strings.Repeat("a", 20000)})
		}
		queries = append(queries, map[string]string{"query": strings.Repeat("a", last)})
		raw, err := json.Marshal(map[string]any{
			"id": "p1", "type": "stat", "title": "Log rows", "datasource": "ch",
			"gridPos": map[string]int{"x": 0, "y": 0, "w": 6, "h": 4},
			"queries": queries,
		})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	compact := func(raw json.RawMessage) int {
		placed, err := spec.WithPanel(base, "p1", raw)
		if err != nil {
			t.Fatal(err)
		}
		entry, _ := placed.FindPanel("p1")
		out, err := json.Marshal(entry.Panel)
		if err != nil {
			t.Fatal(err)
		}
		return len(out)
	}
	const probe = 1000
	last := probe + size - compact(build(probe))
	raw := build(last)
	if got := compact(raw); got != size || len(raw) > revisions.MaxDraftBytes {
		t.Fatalf("built a draft of %d compact and %d raw bytes, want %d compact and at most %d raw", got, len(raw), size, revisions.MaxDraftBytes)
	}
	return raw
}

func TestDraftIsMeasuredAsCompactJSONAgainstTheSuggestedPanelLimit(t *testing.T) {
	svc, d := newSuggestService(t, proposing(suggestedPanel))
	base, err := spec.Parse([]byte(baseSpec))
	if err != nil {
		t.Fatal(err)
	}
	dash := seedDashboard(t, d, 1)
	submitDraft := func(draft json.RawMessage) error {
		_, err := svc.SubmitSuggestion(context.Background(), dash, 1, "p1", "make it better", draft, user)
		return err
	}

	if err := submitDraft(draftWithCompactSize(t, base, spec.MaxSuggestedPanelBytes)); err != nil {
		t.Errorf("draft exactly at the limit: %v", err)
	}
	if err := submitDraft(draftWithCompactSize(t, base, spec.MaxSuggestedPanelBytes-1)); err != nil {
		t.Errorf("draft just under the limit: %v", err)
	}
	err = submitDraft(draftWithCompactSize(t, base, spec.MaxSuggestedPanelBytes+1))
	if !errors.Is(err, revisions.ErrDraftTooLarge) || !strings.Contains(err.Error(), "compact JSON") {
		t.Errorf("draft just over the limit: err = %v, want the compact JSON ErrDraftTooLarge", err)
	}
}

func TestDraftOverTheRawBodyGuardIsRefusedWithItsOwnMessage(t *testing.T) {
	svc, d := newSuggestService(t, proposing(suggestedPanel))
	dash := seedDashboard(t, d, 1)
	draft := []byte(`{"id":"p1","pad":"` + strings.Repeat("a", revisions.MaxDraftBytes) + `"}`)
	_, err := svc.SubmitSuggestion(context.Background(), dash, 1, "p1", "make it better", draft, user)
	if !errors.Is(err, revisions.ErrDraftTooLarge) || strings.Contains(err.Error(), "compact") {
		t.Fatalf("err = %v, want ErrDraftTooLarge without the compact JSON wording", err)
	}
	if want := fmt.Sprintf("the draft panel is over %d bytes", revisions.MaxDraftBytes); err.Error() != want {
		t.Fatalf("err = %q, want %q", err, want)
	}
}
