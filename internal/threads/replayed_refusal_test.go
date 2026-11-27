package threads

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestAReplayedAskAnswersTheRefusalStoredWithItsMessage(t *testing.T) {
	stored := &RunRefusal{Code: RefusalTooManyRuns, Message: "the queue is full"}
	sent := (&ask{}).sent(written{replayed: true, refusal: stored})
	if sent.Refusal == nil || *sent.Refusal != *stored || sent.Run != nil {
		t.Fatalf("replayed refusal = %+v, run = %+v, want %+v and no run", sent.Refusal, sent.Run, stored)
	}
	if sent.To != ToAgent || !sent.Replayed {
		t.Fatalf("to = %q, replayed = %v, want the ask it was the first time", sent.To, sent.Replayed)
	}

	plain := (&ask{}).sent(written{replayed: true})
	if plain.Refusal != nil || plain.To != ToEveryone {
		t.Fatalf("a replayed message that asked no agent = %+v", plain)
	}
}

func TestTheStoredRefusalIsReadFromTheMessageRow(t *testing.T) {
	if got := storedRefusal(dbsqlc.ThreadMessage{}); got != nil {
		t.Fatalf("a message without a refusal = %+v, want nil", got)
	}
	row := dbsqlc.ThreadMessage{
		RunRefusalCode:    pgtype.Text{String: RefusalForbidden, Valid: true},
		RunRefusalMessage: pgtype.Text{String: "you need edit access", Valid: true},
	}
	got := storedRefusal(row)
	if got == nil || got.Code != RefusalForbidden || got.Message != "you need edit access" {
		t.Fatalf("stored refusal = %+v", got)
	}
}

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
