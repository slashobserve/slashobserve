package threads

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const notifyPayloadLimit = 8000

func TestCommentEventStaysUnderNotifyLimit(t *testing.T) {
	id := mustParse(t, "11111111-2222-3333-4444-555555555555")
	e := emitter{
		scope:  scope{target: Target{Kind: KindDashboard, ID: id}},
		thread: dbsqlc.Thread{ID: id, DashboardID: id, PanelID: pgtype.Text{String: strings.Repeat("\x01", revisions.MaxPanelIDRunes), Valid: true}},
		actor:  actor.Actor{Kind: auth.KindServiceAccount, ID: id.String(), Name: strings.Repeat("\x01", 100000)},
	}
	for _, eventType := range []string{events.TypeCommentCreated, events.TypeCommentUpdated} {
		payload, err := json.Marshal(e.live(eventType, id, id, StatusResolved))
		if err != nil {
			t.Fatal(err)
		}
		if len(payload) >= notifyPayloadLimit/2 {
			t.Errorf("%s payload is %d bytes, want well under %d", eventType, len(payload), notifyPayloadLimit)
		}
	}
}

func TestCommentEventOmitsAbsentPanelAndParent(t *testing.T) {
	id := mustParse(t, "11111111-2222-3333-4444-555555555555")
	parent := mustParse(t, "66666666-7777-4888-8999-aaaaaaaaaaaa")
	panel := pgtype.Text{String: "rps", Valid: true}
	tests := []struct {
		name       string
		panel      pgtype.Text
		parent     pgtype.UUID
		wantPanel  bool
		wantParent bool
	}{
		{"panel root", panel, pgtype.UUID{}, true, false},
		{"whole-dashboard root", pgtype.Text{}, pgtype.UUID{}, false, false},
		{"reply on a panel", panel, parent, true, true},
		{"reply on the whole dashboard", pgtype.Text{}, parent, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := emitter{
				scope:  scope{target: Target{Kind: KindDashboard, ID: id}},
				thread: dbsqlc.Thread{ID: id, DashboardID: id, PanelID: tt.panel},
				actor:  actor.Actor{Kind: auth.KindUser, ID: id.String(), Name: "Ada"},
			}
			payload, err := json.Marshal(e.live(events.TypeCommentCreated, id, tt.parent, StatusOpen))
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatal(err)
			}
			_, hasPanel := fields["panel_id"]
			if hasPanel != tt.wantPanel {
				t.Errorf("panel_id present = %v in %s", hasPanel, payload)
			}
			got, hasParent := fields["parent_id"]
			if hasParent != tt.wantParent || (hasParent && got != parent.String()) {
				t.Errorf("parent_id = %v (present %v) in %s", got, hasParent, payload)
			}
		})
	}
}

func TestOnlyDashboardThreadsSendLiveEvents(t *testing.T) {
	id := mustParse(t, "11111111-2222-3333-4444-555555555555")
	for _, kind := range []Kind{KindObservation, KindAnnotation} {
		e := emitter{scope: scope{target: Target{Kind: kind, ID: id}}, thread: dbsqlc.Thread{ID: id}}
		if ev := e.live(events.TypeCommentCreated, id, pgtype.UUID{}, StatusOpen); ev != nil {
			t.Errorf("a %s thread sent %+v", kind, ev)
		}
	}
}
