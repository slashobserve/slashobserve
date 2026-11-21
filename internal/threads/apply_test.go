package threads

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const declinesPanel = "Add a Declines per minute panel next to Error rate"

func (r *rig) suggestingAnswer(t *testing.T) (thread string, ask dbsqlc.AgentRequest, finding pgtype.UUID) {
	t.Helper()
	dash := r.createDashboard(t, "rps", "errors")
	thread, ask = r.mentionRun(t, target(KindDashboard, dash))
	f := r.answer(t, testUser, findings.SaveInput{AgentRequestID: ask.ID, SuggestedChange: declinesPanel})
	return thread, ask, mustParse(t, f.ID)
}

func (r *rig) applied(t *testing.T, by auth.Principal, thread string, finding pgtype.UUID) Sent {
	t.Helper()
	sent, err := r.asks.Apply(reading(t.Context()), by, mustParse(t, thread), finding)
	if err != nil {
		t.Fatalf("%s applies the suggested change: %v", by.Name, err)
	}
	return sent
}

func (r *rig) runsApplying(t *testing.T, finding pgtype.UUID) []dbsqlc.AgentRequest {
	t.Helper()
	var out []dbsqlc.AgentRequest
	for _, run := range r.queuedRuns(t) {
		row, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), mustParse(t, run.ID))
		if err != nil {
			t.Fatal(err)
		}
		if row.AppliesFindingID == finding {
			out = append(out, row)
		}
	}
	return out
}

func TestApplyingASuggestedChangePostsTheRequestAndStartsARevisionAsTheClicker(t *testing.T) {
	r := newShareRig(t)
	thread, ask, finding := r.suggestingAnswer(t)

	sent := r.applied(t, commenter, thread, finding)
	if sent.Replayed || sent.Run == nil || sent.Message.Body != ApplyLead+declinesPanel || sent.Message.Author.ID != commenterID {
		t.Fatalf("sent = %+v", sent)
	}
	runs := r.runsApplying(t, finding)
	if len(runs) != 1 {
		t.Fatalf("%d runs apply the finding, want 1", len(runs))
	}
	run := runs[0]
	if run.ID.String() != sent.Run.ID || revisions.Intent(run.Intent) != revisions.IntentApply || run.Kind != revisions.KindBuild ||
		run.Prompt != declinesPanel || run.RequestedByID.String() != commenterID || run.DashboardID != ask.DashboardID ||
		run.BaseVersion.Int32 != 1 || run.ThreadMessageID.String() != sent.Message.ID || !run.ThreadContext.Valid {
		t.Fatalf("apply run = %+v", run)
	}
	events := r.db.AuditEvents(t, "finding.apply")
	if len(events) != 1 || events[0].TargetID != finding.String() {
		t.Fatalf("finding.apply events = %+v", events)
	}
}

func TestTwoApplyCallsForOneAnswerReturnTheSameRun(t *testing.T) {
	r := newShareRig(t)
	thread, _, finding := r.suggestingAnswer(t)

	first := r.applied(t, testUser, thread, finding)
	messages := r.messageCount(t)
	second := r.applied(t, commenter, thread, finding)
	if !second.Replayed || second.Run == nil || second.Run.ID != first.Run.ID || second.Message.ID != first.Message.ID {
		t.Fatalf("second apply = %+v, first = %+v", second, first)
	}
	if n := r.messageCount(t); n != messages {
		t.Fatalf("messages %d -> %d, want the second apply to post nothing", messages, n)
	}
	if runs := r.runsApplying(t, finding); len(runs) != 1 {
		t.Fatalf("%d runs apply the finding, want 1", len(runs))
	}

	r.db.Exec(t, `UPDATE agent_requests SET state = 'failed', status = 'failed', failure_code = 'provider_error', finished_at = clock_timestamp() WHERE id = $1`, first.Run.ID)
	again := r.applied(t, testUser, thread, finding)
	if again.Replayed || again.Run == nil || again.Run.ID == first.Run.ID {
		t.Fatalf("apply after the first run failed = %+v", again)
	}
}

func TestApplyRefusesAnAnswerFromElsewhereOrWithoutASuggestion(t *testing.T) {
	r := newShareRig(t)
	thread, _, finding := r.suggestingAnswer(t)
	other, plain := r.mentionRun(t, target(KindDashboard, r.createDashboard(t, "rps", "errors")))
	unsuggesting := r.answer(t, testUser, findings.SaveInput{AgentRequestID: plain.ID})

	if _, err := r.asks.Apply(reading(t.Context()), testUser, mustParse(t, other), finding); !errors.Is(err, ErrAnswerNotHere) {
		t.Fatalf("applying an answer from another thread: %v", err)
	}
	var invalid *InvalidInputError
	if _, err := r.asks.Apply(reading(t.Context()), testUser, mustParse(t, other), mustParse(t, unsuggesting.ID)); !errors.As(err, &invalid) || invalid.Code != CodeNoSuggestedChange {
		t.Fatalf("applying an answer without a suggestion: %v", err)
	}
	if _, err := r.asks.Apply(reading(t.Context()), viewer, mustParse(t, thread), finding); err == nil {
		t.Fatal("a Viewer applied a suggested change")
	}
	if runs := r.runsApplying(t, finding); len(runs) != 0 {
		t.Fatalf("%d runs apply the finding after refusals", len(runs))
	}
}

func TestApplyOverHTTPAnswersCreatedThenReplayed(t *testing.T) {
	r := newShareRig(t)
	thread, _, finding := r.suggestingAnswer(t)
	path := "/threads/" + thread + "/apply"

	created := r.call(t, testUser, http.MethodPost, path, map[string]string{"finding_id": finding.String()})
	if created.status != http.StatusCreated || created.at(t, "replayed") != false || created.at(t, "message", "run", "id") == nil {
		t.Fatalf("first apply = %d %v", created.status, created.body)
	}
	replayed := r.call(t, testUser, http.MethodPost, path, map[string]string{"finding_id": finding.String()})
	if replayed.status != http.StatusOK || replayed.at(t, "replayed") != true || replayed.at(t, "message", "run", "id") != created.at(t, "message", "run", "id") {
		t.Fatalf("second apply = %d %v", replayed.status, replayed.body)
	}
	if missing := r.call(t, testUser, http.MethodPost, path, map[string]string{"finding_id": "00000000-0000-4000-8000-000000000000"}); missing.status != http.StatusNotFound {
		t.Fatalf("unknown finding = %d %v", missing.status, missing.body)
	}
}
