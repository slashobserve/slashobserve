package threads

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestAClientIDResendOfARefusedAskReturnsTheSameRefusal(t *testing.T) {
	r := newAgentRig(t, true)
	ctx := reading(t.Context())
	dash := r.createDashboard(t)
	note := r.createAnnotation(t, testUser, dash)
	in := OpenInput{Target: target(KindAnnotation, note), Body: "@observe was this the deploy?", ClientID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}

	first, err := r.asks.Open(ctx, testUser, in, ToAuto)
	if err != nil || first.Refusal == nil || first.Refusal.Code != RefusalUnsupportedTarget || first.Replayed {
		t.Fatalf("first = %+v, %v", first, err)
	}
	again, err := r.asks.Open(ctx, testUser, in, ToAuto)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Replayed || again.Message.ID != first.Message.ID || again.Run != nil || again.To != first.To {
		t.Fatalf("resend = %+v, want the first message %s replayed", again, first.Message.ID)
	}
	if again.Refusal == nil || *again.Refusal != *first.Refusal {
		t.Fatalf("resend refusal = %+v, want %+v", again.Refusal, first.Refusal)
	}

	reply := ReplyInput{Body: "@observe and this one?", ClientID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}
	refused, err := r.asks.Reply(ctx, testUser, mustParse(t, first.Thread.ID), reply, ToAuto)
	if err != nil || refused.Refusal == nil {
		t.Fatalf("reply = %+v, %v", refused, err)
	}
	resent, err := r.asks.Reply(ctx, testUser, mustParse(t, first.Thread.ID), reply, ToAuto)
	if err != nil || !resent.Replayed || resent.Refusal == nil || *resent.Refusal != *refused.Refusal {
		t.Fatalf("resent reply = %+v, %v, want refusal %+v", resent, err, refused.Refusal)
	}

	plain := OpenInput{Target: target(KindDashboard, dash), Body: "no agent here", ClientID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}
	if _, err := r.asks.Open(ctx, testUser, plain, ToAuto); err != nil {
		t.Fatal(err)
	}
	if replayed, err := r.asks.Open(ctx, testUser, plain, ToAuto); err != nil || !replayed.Replayed || replayed.Refusal != nil {
		t.Fatalf("a resent plain message = %+v, %v, want no refusal", replayed, err)
	}
}
