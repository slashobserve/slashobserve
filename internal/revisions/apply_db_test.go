package revisions_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const declinesPanel = "Add a Declines per minute panel next to Error rate"

func suggestingAnswer(t *testing.T, svc *revisions.Service, d *dbtest.Database, dash pgtype.UUID, change *string) string {
	t.Helper()
	ask, err := svc.Start(reading(t.Context()), user, revisions.StartRequest{Prompt: text("why are declines up?"), DashboardID: text(dash.String())})
	if err != nil {
		t.Fatal(err)
	}
	var finding string
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO findings (agent_request_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name, suggested_change)
VALUES ($1::uuid, 'Declines rose', 'the card issuer', now() - interval '1 hour', now(), 'user', $2::uuid, 'Ada', $3) RETURNING id::text`, ask.ID.String(), userID, change).Scan(&finding); err != nil {
		t.Fatal(err)
	}
	d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_kind = 'observation', attempts = 1, started_at = now(), finished_at = now() WHERE id = $1", ask.ID)
	return finding
}

func applying(dash pgtype.UUID, finding string) revisions.StartRequest {
	return revisions.StartRequest{Kind: text(revisions.KindRevise), DashboardID: text(dash.String()), AppliesFindingID: text(finding)}
}

func TestApplyingAnAnswerRevisesTheDashboardWithItsSuggestedChange(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	finding := suggestingAnswer(t, svc, d, dash, text(declinesPanel))

	first, err := svc.Start(reading(t.Context()), user, applying(dash, finding))
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.Intent != string(revisions.IntentApply) || first.Prompt != declinesPanel || first.AppliesFindingID.String() != finding || first.DashboardID != dash {
		t.Fatalf("apply run = %+v", first.AgentRequest)
	}
	again, err := svc.Start(reading(t.Context()), user, applying(dash, finding))
	if err != nil || !again.Replayed || again.ID != first.ID {
		t.Fatalf("second apply = %+v, %v; want the first run replayed", again.AgentRequest, err)
	}

	d.Exec(t, "UPDATE agent_requests SET requested_by_id = $2::uuid WHERE id = $1", first.ID, strangerID)
	if _, err := svc.Start(reading(t.Context()), user, applying(dash, finding)); !errors.Is(err, revisions.ErrFindingApplied) {
		t.Fatalf("applying an answer someone else applies: %v", err)
	}

	d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed', failure_code = 'provider_error', attempts = 1, started_at = now(), finished_at = now() WHERE id = $1", first.ID)
	after, err := svc.Start(reading(t.Context()), user, applying(dash, finding))
	if err != nil || after.Replayed || after.ID == first.ID {
		t.Fatalf("apply after the first run failed = %+v, %v", after.AgentRequest, err)
	}
}

func TestApplyingAnAnswerRefusesWhatItCannotApply(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	other := seedDashboard(t, d, 1)
	finding := suggestingAnswer(t, svc, d, dash, text(declinesPanel))
	plain := suggestingAnswer(t, svc, d, dash, nil)
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	elsewhere, err := svc.Start(reading(t.Context()), user, revisions.StartRequest{Prompt: text("which logs exist?"), DashboardID: text(dash.String())})
	if err != nil {
		t.Fatal(err)
	}
	inAnotherChat := applying(dash, finding)
	inAnotherChat.ChatID = text(elsewhere.ChatID.String())

	refusals := map[string]revisions.StartRequest{
		"in another chat":          inAnotherChat,
		"with a prompt":            {Kind: text(revisions.KindRevise), DashboardID: text(dash.String()), AppliesFindingID: text(finding), Prompt: text("something else")},
		"as an ask":                {DashboardID: text(dash.String()), AppliesFindingID: text(finding)},
		"on another dashboard":     applying(other, finding),
		"with no suggested change": applying(dash, plain),
	}
	for name, req := range refusals {
		var invalid *revisions.StartError
		if _, err := svc.Start(reading(t.Context()), user, req); !errors.As(err, &invalid) {
			t.Errorf("applying %s: %v, want a refusal", name, err)
		}
	}
	if _, err := svc.Start(reading(t.Context()), eve, applying(dash, finding)); !errors.Is(err, revisions.ErrFindingNotFound) {
		t.Errorf("applying someone else's private answer: %v", err)
	}
	if _, err := svc.Start(reading(t.Context()), user, applying(dash, "77777777-7777-4777-8777-777777777777")); !errors.Is(err, revisions.ErrFindingNotFound) {
		t.Errorf("applying an unknown finding: %v", err)
	}
	var applies int
	if err := d.SQLDB(t).QueryRowContext(t.Context(), "SELECT count(*) FROM agent_requests WHERE applies_finding_id IS NOT NULL").Scan(&applies); err != nil {
		t.Fatal(err)
	}
	if applies != 0 {
		t.Fatalf("%d apply runs after refusals", applies)
	}
}

func TestApplyingAnAnswerOverHTTPMatchesTheDocument(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	finding := suggestingAnswer(t, svc, d, dash, text(declinesPanel))
	h := newDocumentedObserve(t, svc, user, nil)
	body := `{"kind":"revise","dashboard_id":"` + dash.String() + `","applies_finding_id":"` + finding + `"}`

	var first, again struct {
		Request struct {
			ID       string `json:"id"`
			Prompt   string `json:"prompt"`
			Replayed bool   `json:"replayed"`
		} `json:"request"`
	}
	if err := json.Unmarshal(h.call(t, http.MethodPost, "/api/v1/observe", body, http.StatusAccepted).Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(h.call(t, http.MethodPost, "/api/v1/observe", body, http.StatusOK).Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if first.Request.Prompt != declinesPanel || !again.Request.Replayed || again.Request.ID != first.Request.ID {
		t.Fatalf("first %+v, again %+v", first.Request, again.Request)
	}
	d.Exec(t, "UPDATE agent_requests SET requested_by_id = $2::uuid WHERE id = $1::uuid", first.Request.ID, strangerID)
	h.call(t, http.MethodPost, "/api/v1/observe", body, http.StatusConflict)
	h.call(t, http.MethodPost, "/api/v1/observe", `{"kind":"revise","dashboard_id":"`+dash.String()+`","applies_finding_id":"77777777-7777-4777-8777-777777777777"}`, http.StatusNotFound)
}
