package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func answerWith(t *testing.T, d *dbtest.Database, id pgtype.UUID, markdown string) {
	t.Helper()
	d.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_kind = 'observation', finished_at = clock_timestamp() WHERE id = $1`, id)
	d.Exec(t, `WITH o AS (INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name)
  VALUES (general_folder_id(), 'answer', 'user', $3::uuid, 'Ada') RETURNING id)
INSERT INTO findings (id, observation_id, agent_request_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
SELECT o.id, o.id, $1, 'answer', $2, now() - interval '1 hour', now(), 'user', $3::uuid, 'Ada' FROM o`, id, markdown, userID)
}

func followUp(ctx context.Context, svc *revisions.Service, by auth.Principal, prompt string, chat pgtype.UUID) (dbsqlc.AgentRequest, error) {
	chatID := chat.String()
	row, err := svc.Start(ctx, by, revisions.StartRequest{Prompt: &prompt, ChatID: &chatID})
	return row.AgentRequest, err
}

func claimedContext(t *testing.T, svc *revisions.Service, id pgtype.UUID) string {
	t.Helper()
	claimed, found := claimOne(t, svc)
	if !found || claimed.ID != id {
		t.Fatalf("claimed %v (found %v), want %v", claimed.ID, found, id)
	}
	return claimed.ThreadContext.String
}

func TestObserveFollowUpJoinsTheChatAndCarriesItsAnswers(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := reading(context.Background())
	first, err := svc.SubmitObserve(ctx, user, "which logs exist")
	if err != nil {
		t.Fatal(err)
	}
	if first.ChatID != first.ID {
		t.Fatalf("a first request starts a chat under its own id, got %v", first.ChatID)
	}
	answerWith(t, d, first.ID, "Loki and Elasticsearch")

	second, err := followUp(ctx, svc, user, "show the errors", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.ChatID != first.ID {
		t.Fatalf("chat_id = %v, want the first request %v", second.ChatID, first.ID)
	}
	if second.ThreadContext.Valid {
		t.Fatalf("a chat turn's transcript is built when it is claimed, got %q at enqueue", second.ThreadContext.String)
	}
	if got := claimedContext(t, svc, second.ID); !strings.Contains(got, "You: which logs exist") || !strings.Contains(got, "Observe Agent: Loki and Elasticsearch") {
		t.Fatalf("thread context = %q, want the first question and answer", got)
	}
	answerWith(t, d, second.ID, "Errors are flat")

	third, err := followUp(ctx, svc, user, "and latency", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if third.ChatID != first.ID {
		t.Fatalf("chat_id = %v, want the root %v", third.ChatID, first.ID)
	}
	if got := claimedContext(t, svc, third.ID); strings.Index(got, "Loki") > strings.Index(got, "Errors are flat") || !strings.Contains(got, "Errors are flat") {
		t.Fatalf("thread context = %q, want both answers oldest first", got)
	}
}

func TestObserveFollowUpRefusesAnotherPrincipalsRequest(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	first, err := svc.SubmitObserve(reading(context.Background()), user, "mine")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := followUp(reading(context.Background()), svc, account, "not mine", first.ID); !errors.Is(err, revisions.ErrChatNotFound) {
		t.Fatalf("err = %v, want chat not found", err)
	}
}

func TestDashboardFollowUpCarriesTheEarlierTurnsOnThatDashboard(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	first, err := svc.SubmitFollowUp(reading(context.Background()), dash, nil, "", "add a latency panel", user)
	if err != nil {
		t.Fatal(err)
	}
	answerWith(t, d, first.ID, "Latency is already covered")
	second, err := svc.SubmitFollowUp(reading(context.Background()), dash, nil, "", "and errors", user)
	if err != nil {
		t.Fatal(err)
	}
	if !first.ChatID.Valid || second.ChatID != first.ChatID {
		t.Fatalf("chat = %v then %v, want the second turn in the first turn's chat", first.ChatID, second.ChatID)
	}
	if got := claimedContext(t, svc, second.ID); !strings.Contains(got, "You: add a latency panel") || !strings.Contains(got, "Latency is already covered") {
		t.Fatalf("thread context = %q, want the earlier turn on the dashboard", got)
	}
}

func TestListObserveByChatReturnsEveryTurnOfOneChat(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := reading(context.Background())
	first, err := svc.SubmitObserve(ctx, user, "one")
	if err != nil {
		t.Fatal(err)
	}
	answerWith(t, d, first.ID, "answer one")
	second, err := followUp(ctx, svc, user, "two", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.SubmitObserve(ctx, user, "elsewhere")
	if err != nil {
		t.Fatal(err)
	}

	r := observeRouterFor(svc, user)
	page := listObserve(t, r, "?chat_id="+first.ID.String())
	if got, want := page.ids(), newest(first.ID.String(), second.ID.String()); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("ids = %v, want %v (not %s)", got, want, other.ID)
	}
	if rec := call(r, http.MethodGet, "/api/v1/observe?chat_id=nope", ""); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_request" {
		t.Fatalf("bad chat_id: %d %s", rec.Code, rec.Body)
	}
}

func TestCreateObserveWithChatIDReportsTheChat(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	first, err := svc.SubmitObserve(reading(context.Background()), user, "one")
	if err != nil {
		t.Fatal(err)
	}
	r := observeRouterFor(svc, user)

	rec := call(r, http.MethodPost, "/api/v1/observe", fmt.Sprintf(`{"prompt":"two","chat_id":%q}`, first.ID))
	var wrapped struct {
		Request struct {
			ID     string `json:"id"`
			ChatID string `json:"chat_id"`
		} `json:"request"`
	}
	body := &wrapped.Request
	if rec.Code != http.StatusAccepted || json.Unmarshal(rec.Body.Bytes(), &wrapped) != nil || body.ChatID != first.ID.String() || body.ID == first.ID.String() {
		t.Fatalf("follow-up: %d %s", rec.Code, rec.Body)
	}

	rec = call(r, http.MethodGet, "/api/v1/observe/"+first.ID.String(), "")
	if err := json.Unmarshal(rec.Body.Bytes(), &wrapped); err != nil || body.ChatID != first.ID.String() {
		t.Fatalf("a root request reports its own id as chat_id: %d %s", rec.Code, rec.Body)
	}
}

func TestCreateObserveRejectsAMalformedOrUnknownChatID(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	r := observeRouterFor(svc, user)
	if rec := call(r, http.MethodPost, "/api/v1/observe", `{"prompt":"x","chat_id":"nope"}`); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_request" {
		t.Fatalf("not a uuid: %d %s", rec.Code, rec.Body)
	}
	if rec := call(r, http.MethodPost, "/api/v1/observe", `{"prompt":"x","chat_id":"00000000-0000-4000-8000-000000000001"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown chat_id: %d %s", rec.Code, rec.Body)
	}
}

func TestRetryKeepsTheChat(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := reading(context.Background())
	first, err := svc.SubmitObserve(ctx, user, "one")
	if err != nil {
		t.Fatal(err)
	}
	answerWith(t, d, first.ID, "answer one")
	second, err := followUp(ctx, svc, user, "two", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `UPDATE agent_requests SET state = 'failed', status = 'failed', failure_code = 'agent_error', error = 'x', finished_at = clock_timestamp() WHERE id = $1`, second.ID)

	retried, err := svc.Retry(ctx, second.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ChatID != first.ID {
		t.Fatalf("retry chat = %v, want %v", retried.ChatID, first.ID)
	}
	if got := claimedContext(t, svc, retried.ID); !strings.Contains(got, "answer one") {
		t.Fatalf("retry context = %q, want the earlier answer", got)
	}
}

func TestRetryOfAStoppedFirstRequestStaysInItsChat(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := reading(context.Background())
	first, err := svc.SubmitObserve(ctx, user, "one")
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `UPDATE agent_requests SET state = 'canceled', status = 'failed', failure_code = 'canceled', error = 'x', finished_at = clock_timestamp() WHERE id = $1`, first.ID)

	retried, err := svc.Retry(ctx, first.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ChatID != first.ID {
		t.Fatalf("retry chat_id = %v, want the stopped request %v", retried.ChatID, first.ID)
	}
	page := listObserve(t, observeRouterFor(svc, user), "?chat_id="+first.ID.String())
	if len(page.ids()) != 2 {
		t.Fatalf("chat has %d requests, want the stopped one and its retry", len(page.ids()))
	}
}

func TestContinueOnADashboardChatKeepsItsEarlierTurns(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := reading(context.Background())
	dash := seedDashboard(t, d, 1)
	first, err := svc.SubmitFollowUp(ctx, dash, nil, "", "payments p95", user)
	if err != nil {
		t.Fatal(err)
	}
	answerWith(t, d, first.ID, "Payments p95 is 420ms")
	second, err := svc.SubmitFollowUp(ctx, dash, nil, "", "add a panel for that latency", user)
	if err != nil {
		t.Fatal(err)
	}
	claimed, found := claimOne(t, svc)
	if !found || claimed.ID != second.ID {
		t.Fatalf("claim = %+v, found %v; want the second turn", claimed, found)
	}
	if _, err := d.Queries().UpsertAgentRequestBuild(ctx, dbsqlc.UpsertAgentRequestBuildParams{
		AgentRequestID: second.ID, DashboardID: dash, BaseVersion: pgtype.Int4{Int32: 1, Valid: true}, Spec: []byte(`{}`), Panels: 2, Attempt: claimed.Attempts,
	}); err != nil {
		t.Fatal(err)
	}
	d.Exec(t, "UPDATE agent_requests SET state = 'canceled', status = 'failed', failure_code = 'canceled', error = 'stopped', lease_until = NULL, finished_at = now() WHERE id = $1", second.ID)

	next, err := svc.Continue(ctx, second.ID, user, "")
	if err != nil {
		t.Fatal(err)
	}
	if next.ChatID != second.ChatID {
		t.Fatalf("continued chat = %v, want %v", next.ChatID, second.ChatID)
	}
	if got := claimedContext(t, svc, next.ID); !strings.Contains(got, "Payments p95 is 420ms") {
		t.Fatalf("continued run context = %q, want the earlier turn on the dashboard", got)
	}
}

func TestAFollowUpThatCarriesAnAnswerThatReadAdminOnlyDataIsMarkedLikeIt(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		svc, d := newService(t, succeedWith(2))
		ctx := reading(context.Background())
		first, err := svc.SubmitObserve(ctx, user, "who changed the checkout dashboard")
		if err != nil {
			t.Fatal(err)
		}
		answerWith(t, d, first.ID, "Root changed it at 09:40")
		d.Exec(t, `UPDATE agent_requests SET read_restricted = $2 WHERE id = $1`, first.ID, restricted)
		second, err := followUp(ctx, svc, user, "build a dashboard of it", first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := claimedContext(t, svc, second.ID); !strings.Contains(got, "Root changed it") {
			t.Fatalf("thread context = %q, want the first answer", got)
		}
		stored, err := svc.GetByID(ctx, second.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.ReadRestricted != restricted {
			t.Fatalf("earlier answer restricted %v: the follow-up is marked %v", restricted, stored.ReadRestricted)
		}
	}
}
