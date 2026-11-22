package revisions

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/audit"
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
	return shapedAs(dbsqlc.AgentRequest{
		ID:              runRequestID,
		DashboardID:     runDashboardID,
		Status:          string(status),
		State:           string(status),
		RequestedByKind: string(auth.KindUser),
		RequestedByID:   runRequesterID,
		RequestedByName: "Ada",
		Origin:          pgtype.Text{String: string(origin.Web), Valid: true},
	}, source)
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

func inState(r dbsqlc.AgentRequest, state State) dbsqlc.AgentRequest {
	r.State = string(state)
	switch state {
	case StateCanceled:
		r.Status = string(StatusFailed)
		r.FailureCode = pgtype.Text{String: string(FailureCanceled), Valid: true}
	case StateCanceling:
		r.Status = string(StatusRunning)
		r.CancelRequestedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	return r
}

func TestRunEventNamesTheMoveOfEveryStateAndSource(t *testing.T) {
	cases := []struct {
		name   string
		row    dbsqlc.AgentRequest
		action Action
		want   string
		wanted bool
	}{
		{"queued prompt", runRow(StatusQueued, SourcePrompt), ActionEnqueue, activity.TypeRunQueued, true},
		{"queued comment", runRow(StatusQueued, SourceComment), ActionEnqueue, activity.TypeRunQueued, true},
		{"queued observe", runRow(StatusQueued, SourceObserve), ActionEnqueue, activity.TypeRunQueued, true},
		{"queued investigate", runRow(StatusQueued, SourceInvestigate), ActionEnqueue, activity.TypeRunQueued, true},
		{"running prompt", runRow(StatusRunning, SourcePrompt), ActionClaim, activity.TypeRunStarted, true},
		{"running investigate", runRow(StatusRunning, SourceInvestigate), ActionClaim, activity.TypeRunStarted, true},
		{"succeeded comment", runRow(StatusSucceeded, SourceComment), ActionSucceed, activity.TypeRunSucceeded, true},
		{"succeeded observe by exhaustion", runRow(StatusSucceeded, SourceObserve), ActionExhaust, activity.TypeRunSucceeded, true},
		{"failed prompt", runRow(StatusFailed, SourcePrompt), ActionFail, activity.TypeRunFailed, true},
		{"failed investigate by exhaustion", runRow(StatusFailed, SourceInvestigate), ActionExhaust, activity.TypeRunFailed, true},
		{"canceled prompt settled", inState(runRow(StatusFailed, SourcePrompt), StateCanceled), ActionSettle, activity.TypeRunCanceled, true},
		{"canceled investigate by cancel", inState(runRow(StatusFailed, SourceInvestigate), StateCanceled), ActionCancel, activity.TypeRunCanceled, true},
		{"succeeded by cancel with a saved version", runRow(StatusSucceeded, SourcePrompt), ActionCancel, activity.TypeRunSucceeded, true},
		{"canceling prompt", inState(runRow(StatusRunning, SourcePrompt), StateCanceling), ActionCancel, activity.TypeRunCanceling, true},
		{"released prompt", runRow(StatusQueued, SourcePrompt), ActionRelease, activity.TypeRunReleased, true},
		{"kept build of a failed run", runRow(StatusFailed, SourcePrompt), ActionKeepBuild, activity.TypeRunKept, true},
		{"kept build of a canceled run", inState(runRow(StatusFailed, SourceObserve), StateCanceled), ActionKeepBuild, activity.TypeRunKept, true},
		{"continue records nothing on the stopped run", runRow(StatusFailed, SourcePrompt), ActionContinue, "", false},
		{"retry records nothing on the finished run", runRow(StatusFailed, SourcePrompt), ActionRetry, "", false},
		{"pin records nothing", runRow(StatusSucceeded, SourcePrompt), ActionPin, "", false},
		{"queued suggest", runRow(StatusQueued, SourceSuggest), ActionEnqueue, "", false},
		{"running suggest", runRow(StatusRunning, SourceSuggest), ActionClaim, "", false},
		{"succeeded suggest", runRow(StatusSucceeded, SourceSuggest), ActionSucceed, "", false},
		{"failed suggest", runRow(StatusFailed, SourceSuggest), ActionFail, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			typ, ok := runEventFor(tc.row, tc.action)
			if ok != tc.wanted || typ != tc.want {
				t.Fatalf("runEventFor = %q, %v; want %q, %v", typ, ok, tc.want, tc.wanted)
			}
			if !ok {
				return
			}
			entry := runActivity(tc.row, typ)
			if entry.Type != typ || entry.TargetID != runRequestID || entry.Owner != activity.OnDashboard(tc.row.DashboardID) {
				t.Errorf("entry %q targets %v owned by %+v, want the request in its dashboard", entry.Type, entry.TargetID, entry.Owner)
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
		typ       string
		status    Status
		wantRunID string
	}{
		{activity.TypeRunQueued, StatusQueued, ""},
		{activity.TypeRunStarted, StatusRunning, runRequestID.String()},
		{activity.TypeRunSucceeded, StatusSucceeded, runRequestID.String()},
		{activity.TypeRunFailed, StatusFailed, runRequestID.String()},
		{activity.TypeRunCanceled, StatusFailed, runRequestID.String()},
		{activity.TypeRunCanceling, StatusRunning, ""},
		{activity.TypeRunReleased, StatusQueued, runRequestID.String()},
		{activity.TypeRunKept, StatusFailed, ""},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			row := runRow(tc.status, SourcePrompt)
			row.RequestedByKind = string(auth.KindServiceAccount)
			row.Origin = pgtype.Text{String: string(origin.MCP), Valid: true}
			row.OriginClient = pgtype.Text{String: "claude-code", Valid: true}
			entry := runActivity(row, tc.typ)
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

func TestRunActivityNamesTheAgentOnlyOnEventsOfARunThatWasClaimed(t *testing.T) {
	accountID := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	grantID := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}
	assignees := []struct {
		name     string
		kind     AssigneeKind
		id       pgtype.UUID
		assignee string
		want     audit.Agent
	}{
		{"the built-in agent", AssignBuiltin, pgtype.UUID{}, "", audit.Agent{Name: "Observe Agent"}},
		{"a connected agent", AssignGrant, grantID, " Claude Code ", audit.Agent{ID: uuid.NullUUID{UUID: grantID.Bytes, Valid: true}, Name: "Claude Code"}},
		{"a service account", AssignServiceAccount, accountID, "ci-bot", audit.Agent{ID: uuid.NullUUID{UUID: accountID.Bytes, Valid: true}, Name: "ci-bot"}},
	}
	events := []struct {
		typ    string
		status Status
		agent  bool
	}{
		{activity.TypeRunQueued, StatusQueued, false},
		{activity.TypeRunStarted, StatusRunning, true},
		{activity.TypeRunSucceeded, StatusSucceeded, true},
		{activity.TypeRunFailed, StatusFailed, true},
		{activity.TypeRunCanceled, StatusFailed, true},
		{activity.TypeRunCanceling, StatusRunning, false},
		{activity.TypeRunReleased, StatusQueued, true},
		{activity.TypeRunKept, StatusFailed, false},
	}
	for _, a := range assignees {
		for _, ev := range events {
			t.Run(a.name+" "+ev.typ, func(t *testing.T) {
				row := runRow(ev.status, SourcePrompt)
				row.AssignedKind, row.AssignedID = string(a.kind), a.id
				row.StartedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
				got := runActivityWith(row, ev.typ, a.assignee, nil).Agent
				want := audit.Agent{}
				if ev.agent {
					want = a.want
				}
				if got != want {
					t.Fatalf("agent = %+v, want %+v", got, want)
				}
				row.StartedAt = pgtype.Timestamptz{}
				if got := runActivityWith(row, ev.typ, a.assignee, nil).Agent; got != (audit.Agent{}) {
					t.Fatalf("agent of a run nobody claimed = %+v, want none", got)
				}
			})
		}
	}
}

func TestRunActivityNamesTheRequesterByEmailWhenTheNameIsEmpty(t *testing.T) {
	row := runRow(StatusQueued, SourcePrompt)
	row.RequestedByName = ""
	row.RequestedByEmail = pgtype.Text{String: "ada@example.com", Valid: true}
	entry := runActivity(row, activity.TypeRunQueued)
	if entry.Actor.Name != "ada@example.com" {
		t.Fatalf("actor name = %q, want the email", entry.Actor.Name)
	}
	row.RequestedByName = "Ada"
	entry = runActivity(row, activity.TypeRunQueued)
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

	canceled := inState(runRow(StatusFailed, SourcePrompt), StateCanceled)

	released := runRow(StatusQueued, SourcePrompt)
	released.Attempts = 1

	kept := runRow(StatusFailed, SourcePrompt)
	kept.ResultVersion = pgtype.Int4{Int32: 4, Valid: true}

	withUsage := func(row dbsqlc.AgentRequest, usage string) dbsqlc.AgentRequest {
		row.RunUsage = []byte(usage)
		return row
	}
	const priced = `{"model":"m","model_calls":3,"cost_usd":0.125}`

	cases := []struct {
		name string
		row  dbsqlc.AgentRequest
		typ  string
		want map[string]any
	}{
		{"succeeded with a cost", withUsage(succeeded, priced), activity.TypeRunSucceeded, map[string]any{"intent": "apply", "result_version": float64(7), "cost_usd": 0.125}},
		{"failed with a cost", withUsage(failed, priced), activity.TypeRunFailed, map[string]any{"intent": "apply", "failure_code": "no_progress", "cost_usd": 0.125}},
		{"canceled with a cost", withUsage(canceled, priced), activity.TypeRunCanceled, map[string]any{"intent": "apply", "cost_usd": 0.125}},
		{"free run", withUsage(runRow(StatusSucceeded, SourcePrompt), `{"cost_usd":0}`), activity.TypeRunSucceeded, map[string]any{"intent": "apply", "cost_usd": float64(0)}},
		{"succeeded with an unpriced model", withUsage(runRow(StatusSucceeded, SourcePrompt), `{"model":"m","cost_usd":null}`), activity.TypeRunSucceeded, map[string]any{"intent": "apply"}},
		{"succeeded with usage from before costs", withUsage(runRow(StatusSucceeded, SourcePrompt), `{"model":"m","model_calls":3}`), activity.TypeRunSucceeded, map[string]any{"intent": "apply"}},
		{"failed with no usage", failed, activity.TypeRunFailed, map[string]any{"intent": "apply", "failure_code": "no_progress"}},
		{"succeeded with unreadable usage", withUsage(runRow(StatusSucceeded, SourcePrompt), `[1`), activity.TypeRunSucceeded, map[string]any{"intent": "apply"}},
		{"started keeps no cost", withUsage(running, priced), activity.TypeRunStarted, map[string]any{"intent": "apply", "attempt": float64(2)}},
		{"queued", runRow(StatusQueued, SourceComment), activity.TypeRunQueued, map[string]any{"intent": "apply"}},
		{"queued continuation", queuedContinuation, activity.TypeRunQueued, map[string]any{"intent": "apply", "continues_id": runContinuesID.String()}},
		{"started", running, activity.TypeRunStarted, map[string]any{"intent": "apply", "attempt": float64(2)}},
		{"succeeded without a version", runRow(StatusSucceeded, SourceInvestigate), activity.TypeRunSucceeded, map[string]any{"intent": "ask"}},
		{"succeeded", succeeded, activity.TypeRunSucceeded, map[string]any{"intent": "apply", "result_version": float64(7)}},
		{"failed without a code", runRow(StatusFailed, SourcePrompt), activity.TypeRunFailed, map[string]any{"intent": "apply"}},
		{"failed", failed, activity.TypeRunFailed, map[string]any{"intent": "apply", "failure_code": "no_progress"}},
		{"canceled", canceled, activity.TypeRunCanceled, map[string]any{"intent": "apply"}},
		{"canceling", inState(running, StateCanceling), activity.TypeRunCanceling, map[string]any{"intent": "apply"}},
		{"released names the attempt handed back", released, activity.TypeRunReleased, map[string]any{"intent": "apply", "attempt": float64(2)}},
		{"kept names the saved version", kept, activity.TypeRunKept, map[string]any{"intent": "apply", "result_version": float64(4)}},
		{"kept without a version", runRow(StatusFailed, SourcePrompt), activity.TypeRunKept, map[string]any{"intent": "apply"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dataOf(t, runActivity(tc.row, tc.typ))
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

func TestAFinishedRunKeepsItsModelAndTokensForTheAuditEventOnly(t *testing.T) {
	finished := runRow(StatusSucceeded, SourcePrompt)
	finished.RunUsage = []byte(`{"model":"claude-haiku-5-5","input_tokens":1200,"output_tokens":340,"cost_usd":0.5}`)
	cases := []struct {
		name  string
		row   dbsqlc.AgentRequest
		typ   string
		audit map[string]any
	}{
		{"succeeded", finished, activity.TypeRunSucceeded, map[string]any{"model": "claude-haiku-5-5", "input_tokens": int64(1200), "output_tokens": int64(340)}},
		{"failed", finished, activity.TypeRunFailed, map[string]any{"model": "claude-haiku-5-5", "input_tokens": int64(1200), "output_tokens": int64(340)}},
		{"canceled", finished, activity.TypeRunCanceled, map[string]any{"model": "claude-haiku-5-5", "input_tokens": int64(1200), "output_tokens": int64(340)}},
		{"no usage", runRow(StatusSucceeded, SourcePrompt), activity.TypeRunSucceeded, map[string]any{}},
		{"unreadable usage", func() dbsqlc.AgentRequest { r := finished; r.RunUsage = []byte(`[1`); return r }(), activity.TypeRunSucceeded, nil},
		{"started", finished, activity.TypeRunStarted, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := runActivity(tc.row, tc.typ)
			if len(entry.AuditOnly) != len(tc.audit) {
				t.Fatalf("audit data = %v, want %v", entry.AuditOnly, tc.audit)
			}
			for k, v := range tc.audit {
				if entry.AuditOnly[k] != v {
					t.Errorf("audit data[%q] = %v, want %v", k, entry.AuditOnly[k], v)
				}
			}
			if data := dataOf(t, entry); data["model"] != nil || data["input_tokens"] != nil {
				t.Errorf("activity data = %v, want no model or tokens", data)
			}
		})
	}
}

func TestRevisionEventIsSentOnlyForRunsThatChangeADashboard(t *testing.T) {
	withComment := runRow(StatusSucceeded, SourceComment)
	withComment.CommentID = pgtype.UUID{Bytes: [16]byte{4}, Valid: true}
	withComment.ResultVersion = pgtype.Int4{Int32: 3, Valid: true}
	noDashboard := runRow(StatusQueued, SourceObserve)
	noDashboard.DashboardID = pgtype.UUID{}
	privateAsk := runRow(StatusRunning, SourceObserve)
	privateAsk.Intent, privateAsk.ChatID = string(IntentAsk), pgtype.UUID{Bytes: [16]byte{5}, Valid: true}
	threadAsk := runRow(StatusRunning, SourceObserve)
	threadAsk.Intent, threadAsk.ThreadMessageID = string(IntentAsk), pgtype.UUID{Bytes: [16]byte{6}, Valid: true}

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
		"a private ask on the dashboard":  privateAsk,
		"an ask in a dashboard thread":    threadAsk,
	} {
		if ev, ok := revisionEvent(row); ok {
			t.Errorf("%s sent %+v", name, ev)
		}
	}
}

func TestAssigneeActorReadsAsTheAgentThatTookTheRun(t *testing.T) {
	accountID := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	tests := []struct {
		name     string
		assigned AssigneeKind
		assignee pgtype.UUID
		agent    string
		want     string
		wantRun  bool
		wantKind auth.PrincipalKind
		wantID   string
	}{
		{"the built-in agent works for the requester", AssignBuiltin, pgtype.UUID{}, "", "Observe Agent for Ada", true, auth.KindUser, runRequesterID.String()},
		{"a connected agent is the person over its client", AssignGrant, runDashboardID, "claude-code", "Ada (via Claude Code)", false, auth.KindUser, runRequesterID.String()},
		{"a connected agent whose client is gone is the person over MCP", AssignGrant, runDashboardID, "", "Ada (via MCP)", false, auth.KindUser, runRequesterID.String()},
		{"a service account is its own actor", AssignServiceAccount, accountID, "ci-bot", "ci-bot (via MCP)", false, auth.KindServiceAccount, accountID.String()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := runRow(StatusSucceeded, SourceComment)
			row.AssignedKind, row.AssignedID = string(tt.assigned), tt.assignee
			got := AssigneeActor(row, tt.agent)
			if got.Label() != tt.want || (got.RunID != "") != tt.wantRun || got.Kind != tt.wantKind || got.ID != tt.wantID {
				t.Fatalf("actor = %+v label %q, want label %q, run %v, kind %s, id %s", got, got.Label(), tt.want, tt.wantRun, tt.wantKind, tt.wantID)
			}
		})
	}
}

func TestTheAgentOfARunIsTheAssigneeThatTookIt(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{0x33, 1}, Valid: true}
	for _, tc := range []struct {
		kind AssigneeKind
		name string
		want audit.Agent
	}{
		{AssignBuiltin, "", audit.Agent{Name: "Observe Agent"}},
		{AssignGrant, " Claude\u0007 Code ", audit.Agent{ID: uuid.NullUUID{UUID: id.Bytes, Valid: true}, Name: "Claude Code"}},
		{AssignServiceAccount, "ci-agent", audit.Agent{ID: uuid.NullUUID{UUID: id.Bytes, Valid: true}, Name: "ci-agent"}},
	} {
		run := dbsqlc.AgentRequest{AssignedKind: string(tc.kind), AssignedID: id}
		if tc.kind == AssignBuiltin {
			run.AssignedID = pgtype.UUID{}
		}
		if got := AssignedAgent(run, tc.name); got != tc.want {
			t.Errorf("%s: agent = %+v, want %+v", tc.kind, got, tc.want)
		}
	}
}
