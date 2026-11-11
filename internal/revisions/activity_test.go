package revisions

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/origin"
)

var (
	runRequestID   = pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	runRequesterID = pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	runDashboardID = pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	runContinuesID = pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
)

func runRow(status Status, source Source) dbsqlc.AgentRequest {
	return dbsqlc.AgentRequest{
		ID:              runRequestID,
		DashboardID:     runDashboardID,
		Source:          string(source),
		Status:          string(status),
		RequestedByKind: string(auth.KindUser),
		RequestedByID:   runRequesterID,
		RequestedByName: "Ada",
		Origin:          pgtype.Text{String: string(origin.Web), Valid: true},
	}
}

func dataOf(t *testing.T, e activity.Entry) map[string]any {
	t.Helper()
	raw, err := json.Marshal(e.Data)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRunActivityNamesTheTransitionOfEveryStatusAndSource(t *testing.T) {
	canceled := func(r dbsqlc.AgentRequest) dbsqlc.AgentRequest {
		r.FailureCode = pgtype.Text{String: string(FailureCanceled), Valid: true}
		return r
	}
	cases := []struct {
		name   string
		row    dbsqlc.AgentRequest
		want   string
		wanted bool
	}{
		{"queued prompt", runRow(StatusQueued, SourcePrompt), activity.TypeRunQueued, true},
		{"queued comment", runRow(StatusQueued, SourceComment), activity.TypeRunQueued, true},
		{"queued observe", runRow(StatusQueued, SourceObserve), activity.TypeRunQueued, true},
		{"queued investigate", runRow(StatusQueued, SourceInvestigate), activity.TypeRunQueued, true},
		{"running prompt", runRow(StatusRunning, SourcePrompt), activity.TypeRunStarted, true},
		{"running investigate", runRow(StatusRunning, SourceInvestigate), activity.TypeRunStarted, true},
		{"succeeded comment", runRow(StatusSucceeded, SourceComment), activity.TypeRunSucceeded, true},
		{"succeeded observe", runRow(StatusSucceeded, SourceObserve), activity.TypeRunSucceeded, true},
		{"failed prompt", runRow(StatusFailed, SourcePrompt), activity.TypeRunFailed, true},
		{"failed with a code", func() dbsqlc.AgentRequest {
			r := runRow(StatusFailed, SourcePrompt)
			r.FailureCode = pgtype.Text{String: string(FailureNoProgress), Valid: true}
			return r
		}(), activity.TypeRunFailed, true},
		{"failed canceled", canceled(runRow(StatusFailed, SourcePrompt)), activity.TypeRunCanceled, true},
		{"failed canceled investigate", canceled(runRow(StatusFailed, SourceInvestigate)), activity.TypeRunCanceled, true},
		{"queued suggest", runRow(StatusQueued, SourceSuggest), "", false},
		{"running suggest", runRow(StatusRunning, SourceSuggest), "", false},
		{"succeeded suggest", runRow(StatusSucceeded, SourceSuggest), "", false},
		{"failed suggest", runRow(StatusFailed, SourceSuggest), "", false},
		{"unknown status", runRow(Status("archived"), SourcePrompt), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, ok := runActivity(tc.row)
			if ok != tc.wanted || entry.Type != tc.want {
				t.Fatalf("runActivity = %q, %v; want %q, %v", entry.Type, ok, tc.want, tc.wanted)
			}
			if !ok {
				return
			}
			if entry.TargetID != runRequestID || entry.Owner != activity.OnDashboard(tc.row.DashboardID) {
				t.Errorf("entry targets %v owned by %+v, want the request in its dashboard", entry.TargetID, entry.Owner)
			}
			if !activity.Known(entry.Type) {
				t.Errorf("%q is not a known activity type", entry.Type)
			}
			if _, err := entry.Actor.Columns(); err != nil {
				t.Errorf("actor columns: %v", err)
			}
		})
	}
}

func TestRunActivityActsAsTheRequesterAndOnlyCarriesTheRunIdOnceItRuns(t *testing.T) {
	cases := []struct {
		status    Status
		wantRunID string
	}{
		{StatusQueued, ""},
		{StatusRunning, runRequestID.String()},
		{StatusSucceeded, runRequestID.String()},
		{StatusFailed, runRequestID.String()},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			row := runRow(tc.status, SourcePrompt)
			row.RequestedByKind = string(auth.KindServiceAccount)
			row.Origin = pgtype.Text{String: string(origin.MCP), Valid: true}
			row.OriginClient = pgtype.Text{String: "claude-code", Valid: true}
			entry, _ := runActivity(row)
			got := entry.Actor
			if got.Kind != auth.KindServiceAccount || got.ID != runRequesterID.String() || got.Name != "Ada" {
				t.Errorf("actor = %+v, want the requesting service account", got)
			}
			if got.Origin.Kind != origin.MCP || got.Origin.Client != "claude-code" {
				t.Errorf("origin = %+v, want mcp via claude-code", got.Origin)
			}
			if got.RunID != tc.wantRunID {
				t.Errorf("run id = %q, want %q", got.RunID, tc.wantRunID)
			}
		})
	}
}

func TestRunActivityNamesTheRequesterByEmailWhenTheNameIsEmpty(t *testing.T) {
	row := runRow(StatusQueued, SourcePrompt)
	row.RequestedByName = ""
	row.RequestedByEmail = pgtype.Text{String: "ada@example.com", Valid: true}
	entry, _ := runActivity(row)
	if entry.Actor.Name != "ada@example.com" {
		t.Fatalf("actor name = %q, want the email", entry.Actor.Name)
	}
	row.RequestedByName = "Ada"
	entry, _ = runActivity(row)
	if entry.Actor.Name != "Ada" {
		t.Fatalf("actor name = %q, want the name when there is one", entry.Actor.Name)
	}
}

func TestRunActivityDataCarriesOnlyIdsNumbersAndEnums(t *testing.T) {
	queuedContinuation := runRow(StatusQueued, SourceObserve)
	queuedContinuation.ContinuesID = runContinuesID
	queuedContinuation.Prompt = "secret prompt text"
	queuedContinuation.ContinueNote = pgtype.Text{String: "secret note", Valid: true}

	running := runRow(StatusRunning, SourcePrompt)
	running.Attempts = 2

	succeeded := runRow(StatusSucceeded, SourcePrompt)
	succeeded.ResultVersion = pgtype.Int4{Int32: 7, Valid: true}

	failed := runRow(StatusFailed, SourcePrompt)
	failed.FailureCode = pgtype.Text{String: string(FailureNoProgress), Valid: true}
	failed.Error = pgtype.Text{String: "secret error text", Valid: true}

	canceled := runRow(StatusFailed, SourcePrompt)
	canceled.FailureCode = pgtype.Text{String: string(FailureCanceled), Valid: true}

	withUsage := func(row dbsqlc.AgentRequest, usage string) dbsqlc.AgentRequest {
		row.RunUsage = []byte(usage)
		return row
	}
	const priced = `{"model":"m","model_calls":3,"cost_usd":0.125}`

	cases := []struct {
		name string
		row  dbsqlc.AgentRequest
		want map[string]any
	}{
		{"succeeded with a cost", withUsage(succeeded, priced), map[string]any{"source": "prompt", "result_version": float64(7), "cost_usd": 0.125}},
		{"failed with a cost", withUsage(failed, priced), map[string]any{"source": "prompt", "failure_code": "no_progress", "cost_usd": 0.125}},
		{"canceled with a cost", withUsage(canceled, priced), map[string]any{"source": "prompt", "cost_usd": 0.125}},
		{"free run", withUsage(runRow(StatusSucceeded, SourcePrompt), `{"cost_usd":0}`), map[string]any{"source": "prompt", "cost_usd": float64(0)}},
		{"succeeded with an unpriced model", withUsage(runRow(StatusSucceeded, SourcePrompt), `{"model":"m","cost_usd":null}`), map[string]any{"source": "prompt"}},
		{"succeeded with usage from before costs", withUsage(runRow(StatusSucceeded, SourcePrompt), `{"model":"m","model_calls":3}`), map[string]any{"source": "prompt"}},
		{"failed with no usage", failed, map[string]any{"source": "prompt", "failure_code": "no_progress"}},
		{"succeeded with unreadable usage", withUsage(runRow(StatusSucceeded, SourcePrompt), `[1`), map[string]any{"source": "prompt"}},
		{"started keeps no cost", withUsage(running, priced), map[string]any{"source": "prompt", "attempt": float64(2)}},
		{"queued", runRow(StatusQueued, SourceComment), map[string]any{"source": "comment"}},
		{"queued continuation", queuedContinuation, map[string]any{"source": "observe", "continues_id": runContinuesID.String()}},
		{"started", running, map[string]any{"source": "prompt", "attempt": float64(2)}},
		{"succeeded without a version", runRow(StatusSucceeded, SourceInvestigate), map[string]any{"source": "investigate"}},
		{"succeeded", succeeded, map[string]any{"source": "prompt", "result_version": float64(7)}},
		{"failed without a code", runRow(StatusFailed, SourcePrompt), map[string]any{"source": "prompt"}},
		{"failed", failed, map[string]any{"source": "prompt", "failure_code": "no_progress"}},
		{"canceled", canceled, map[string]any{"source": "prompt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, ok := runActivity(tc.row)
			if !ok {
				t.Fatal("no activity")
			}
			got := dataOf(t, entry)
			if len(got) != len(tc.want) {
				t.Fatalf("data = %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("data[%q] = %v, want %v (data %v)", k, got[k], v, got)
				}
			}
		})
	}
}

func TestRevisionEventIsSentOnlyForDashboardRequestsThatAreNotSuggestionsOrInvestigations(t *testing.T) {
	withComment := runRow(StatusSucceeded, SourceComment)
	withComment.CommentID = pgtype.UUID{Bytes: [16]byte{4}, Valid: true}
	withComment.ResultVersion = pgtype.Int4{Int32: 3, Valid: true}
	noDashboard := runRow(StatusQueued, SourceObserve)
	noDashboard.DashboardID = pgtype.UUID{}

	ev, ok := revisionEvent(withComment)
	want := events.Event{
		Type:        events.TypeRevisionUpdated,
		DashboardID: runDashboardID.String(),
		RevisionID:  runRequestID.String(),
		Status:      "succeeded",
		Version:     3,
		CommentID:   withComment.CommentID.String(),
	}
	if !ok || ev != want {
		t.Fatalf("revisionEvent = %+v, %v; want %+v", ev, ok, want)
	}
	for name, row := range map[string]dbsqlc.AgentRequest{
		"a request with no dashboard yet": noDashboard,
		"a suggestion":                    runRow(StatusQueued, SourceSuggest),
		"an investigation":                runRow(StatusQueued, SourceInvestigate),
	} {
		if ev, ok := revisionEvent(row); ok {
			t.Errorf("%s sent %+v", name, ev)
		}
	}
}
