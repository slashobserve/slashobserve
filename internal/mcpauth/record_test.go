package mcpauth

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/activity"
)

type recordedEvent struct {
	Type      string
	ActorID   string
	ActorName string
	Data      map[string]string
}

func (r *rig) grantEvents(t *testing.T) []recordedEvent {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT type, actor_id::text, actor_name, data FROM activity WHERE type = ANY($1) ORDER BY seq`, []string{activity.TypeMCPClientConnected, activity.TypeMCPClientDisconnected})
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []recordedEvent
	for rows.Next() {
		var e recordedEvent
		var raw []byte
		if err := rows.Scan(&e.Type, &e.ActorID, &e.ActorName, &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &e.Data); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestConnectingAndDisconnectingAnAgentRecordsOneEventEachWithTheReason(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	got := r.connect(t, ada, clientID)
	id := r.grantOf(t, ada, clientID)

	assertStatus(t, r.api(root, http.MethodDelete, "/grants/"+id), http.StatusNoContent)
	assertStatus(t, r.api(root, http.MethodDelete, "/grants/"+id), http.StatusNotFound)

	events := r.grantEvents(t)
	if len(events) != 2 {
		t.Fatalf("events %+v, want connected then disconnected once", events)
	}
	connected, disconnected := events[0], events[1]
	if connected.Type != activity.TypeMCPClientConnected || connected.ActorID != ada.ID || connected.Data["client"] != "claude-code" || connected.Data["person_name"] != "Ada" || connected.Data["reason"] != "" {
		t.Fatalf("connected %+v", connected)
	}
	if disconnected.Type != activity.TypeMCPClientDisconnected || disconnected.ActorID != root.ID || disconnected.Data["reason"] != "admin" || disconnected.Data["person_id"] != ada.ID {
		t.Fatalf("disconnected %+v", disconnected)
	}
	body, _ := json.Marshal(events)
	for _, secret := range []string{got.Access, got.Refresh} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("an event holds a token: %s", body)
		}
	}
}

func TestAPersonDisconnectingTheirOwnAgentIsTheActor(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	r.connect(t, ada, clientID)
	assertStatus(t, r.api(ada, http.MethodDelete, "/me/grants/"+r.grantOf(t, ada, clientID)), http.StatusNoContent)

	events := r.grantEvents(t)
	last := events[len(events)-1]
	if last.Type != activity.TypeMCPClientDisconnected || last.ActorID != ada.ID || last.Data["reason"] != "person" {
		t.Fatalf("last event %+v", last)
	}
}
