package threads

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/anchor"
)

func TestASentMessageRecordsItsLengthAndTheKindOfPointItsThreadHolds(t *testing.T) {
	r := newRig(t)
	observation := r.createObservation(t, testUser, pgtype.UUID{})
	block := r.addBlock(t, observation)
	spot := &anchor.Anchor{Kind: anchor.Position, Pos: anchor.Pos{X: 0.4, Y: 0.3}}
	thread, _ := r.open(t, testUser, OpenInput{Target: target(KindObservation, observation), Body: "this spot", BlockID: &block, Anchor: spot})
	r.reply(t, testUser, thread.ID, "héllo there")
	plain, _ := r.open(t, testUser, OpenInput{Target: target(KindObservation, observation), Body: "about the page"})
	r.reply(t, testUser, plain.ID, "ok")

	opened := r.db.AuditEvents(t, "thread.open")
	replied := r.db.AuditEvents(t, "thread.message")
	if len(opened) != 2 || len(replied) != 2 {
		t.Fatalf("opened %d and replied %d events, want 2 and 2", len(opened), len(replied))
	}
	for i, want := range []struct {
		event  map[string]any
		length float64
		kind   any
	}{
		{opened[0].Data, 9, "position"},
		{replied[0].Data, 11, "position"},
		{opened[1].Data, 14, nil},
		{replied[1].Data, 2, nil},
	} {
		if want.event["body_length"] != want.length || want.event["anchor_kind"] != want.kind {
			t.Errorf("event %d data = %v, want body_length %v and anchor_kind %v", i, want.event, want.length, want.kind)
		}
	}
}
