package public

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db"
)

type linkEvent struct {
	Type    string
	ActorID string
	Owner   string
	Data    struct {
		Object struct{ Kind, ID string }
		Title  string
		Prefix string
	}
}

func linkEvents(t *testing.T, r rig) []linkEvent {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(reading(t.Context()), `SELECT type, actor_id::text, COALESCE(dashboard_id::text, observation_id::text, ''), data FROM activity WHERE type LIKE 'public_link.%' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []linkEvent
	for rows.Next() {
		var e linkEvent
		var raw []byte
		if err := rows.Scan(&e.Type, &e.ActorID, &e.Owner, &raw); err != nil {
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

func TestDashboardLinkCreateAndRevokeRecordEventsOnTheDashboardWithoutTheToken(t *testing.T) {
	r := newRig(t)
	created := r.create(t, CreateInput{Mode: ModeFree})
	linkID, _ := db.ParseUUID(created.Link.ID)
	if err := r.service.Delete(reading(t.Context()), admin, r.shared, linkID); err != nil {
		t.Fatal(err)
	}
	if err := r.service.Delete(reading(t.Context()), admin, r.shared, linkID); err == nil {
		t.Fatal("a second revoke succeeded")
	}

	events := linkEvents(t, r)
	if len(events) != 2 || events[0].Type != "public_link.created" || events[1].Type != "public_link.revoked" {
		t.Fatalf("events %+v", events)
	}
	for _, e := range events {
		if e.Owner != r.shared.String() || e.ActorID != admin.ID || e.Data.Object.Kind != "dashboard" || e.Data.Object.ID != r.shared.String() || e.Data.Title != "T" || e.Data.Prefix != created.Link.Prefix {
			t.Fatalf("event %+v", e)
		}
	}
	body, _ := json.Marshal(events)
	if strings.Contains(string(body), created.Token) {
		t.Fatalf("an event holds the token: %s", body)
	}
}

func TestRCALinkCreateAndRevokeRecordEventsOnTheObservation(t *testing.T) {
	r := newRCARig(t)
	id := r.published(t)
	created := r.createLink(t, id, 1, CreateRCAInput{})
	linkID, _ := db.ParseUUID(created.Link.ID)
	if err := r.service.DeleteRCALink(reading(t.Context()), admin, id, linkID); err != nil {
		t.Fatal(err)
	}

	events := linkEvents(t, r.rig)
	if len(events) != 2 || events[0].Type != "public_link.created" || events[1].Type != "public_link.revoked" {
		t.Fatalf("events %+v", events)
	}
	for _, e := range events {
		if e.Owner != id.String() || e.Data.Object.Kind != "observation" || e.Data.Object.ID != id.String() || e.Data.Title == "" {
			t.Fatalf("event %+v", e)
		}
	}
}
