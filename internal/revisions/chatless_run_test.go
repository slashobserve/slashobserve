package revisions_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

func chatIDOfListedRun(t *testing.T, body, run string) *string {
	t.Helper()
	type listed struct {
		ID     string  `json:"id"`
		ChatID *string `json:"chat_id"`
	}
	var page struct {
		Requests []listed `json:"requests"`
		Runs     []listed `json:"runs"`
	}
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatal(err)
	}
	for _, r := range append(page.Requests, page.Runs...) {
		if r.ID == run {
			return r.ChatID
		}
	}
	t.Fatalf("run %s is not listed: %s", run, body)
	return nil
}

func TestARunThatBelongsToNoChatIsNeverToldAChatThatDoesNotExist(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	run := investigate(t, svc, revisions.InvestigateParams{DashboardID: dash, Prompt: "why is checkout failing", Ask: true}).ID.String()
	if chat := chatOfTurn(t, d, investigate(t, svc, revisions.InvestigateParams{DashboardID: dash, Prompt: "again", Ask: true}).ID); chat != "" {
		t.Fatalf("a run started without a chat was stored in chat %s", chat)
	}

	w := newPrivateWalk(t, svc, d)
	if chat := chatIDIn(t, w.http(user, "/api/v1/observe/"+run, http.StatusOK)); chat != nil {
		t.Errorf("the requester reads the chat %s on the run", *chat)
	}
	if chat := chatIDOfListedRun(t, w.http(user, "/api/v1/observe", http.StatusOK), run); chat != nil {
		t.Errorf("the requester lists the chat %s on the run", *chat)
	}
	if chat := chatIDIn(t, w.mcp(user, "get_run", `{"id":"`+run+`"}`, false)); chat != nil {
		t.Errorf("get_run tells the chat %s", *chat)
	}
	if chat := chatIDOfListedRun(t, w.mcp(user, "list_runs", `{}`, false), run); chat != nil {
		t.Errorf("list_runs tells the chat %s", *chat)
	}
	w.http(user, "/api/v1/chats/"+run, http.StatusNotFound)
}
