package threads

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/observations"
)

func (r *rig) addBlock(t *testing.T, observation pgtype.UUID) string {
	t.Helper()
	block, err := r.observations.AddBlock(reading(t.Context()), testUser, observation, observations.BlockInput{Kind: observations.KindText, Body: []byte(`{"markdown":"p99 doubled"}`)})
	if err != nil {
		t.Fatal(err)
	}
	return block.ID
}

func wantInvalid(t *testing.T, err error, code string) {
	t.Helper()
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func TestAMessageAnchorsToABlockOfItsObservationAndKeepsTheMessageWhenTheBlockIsDeleted(t *testing.T) {
	r := newRig(t)
	observation := r.createObservation(t, testUser, pgtype.UUID{})
	tg := target(KindObservation, observation)
	first, second := r.addBlock(t, observation), r.addBlock(t, observation)

	thread, opened := r.open(t, testUser, OpenInput{Target: tg, Body: "on the first", BlockID: &first})
	if opened.BlockID != first {
		t.Fatalf("opened on block %q, want %q", opened.BlockID, first)
	}
	_, reply, err := r.service.Reply(reading(t.Context()), commenter, mustParse(t, thread.ID), ReplyInput{Body: "on the second", BlockID: &second}, nil)
	if err != nil || reply.BlockID != second {
		t.Fatalf("reply on block %q, %v", reply.BlockID, err)
	}
	_, whole := r.reply(t, testUser, thread.ID, "about the whole page")
	if whole.BlockID != "" {
		t.Errorf("a reply with no block_id is about the whole observation, got %q", whole.BlockID)
	}

	if _, edited, _, err := r.service.Edit(reading(t.Context()), testUser, mustParse(t, opened.ID), "still on the first"); err != nil || edited.BlockID != first {
		t.Errorf("an edit keeps the anchor: %q, %v", edited.BlockID, err)
	}

	r.db.Exec(t, `DELETE FROM observation_blocks WHERE id = $1::uuid`, first)
	byID := map[string]string{}
	for _, m := range r.list(t, testUser, tg).Messages {
		byID[m.Body] = m.BlockID
	}
	if len(byID) != 3 || byID["still on the first"] != "" || byID["on the second"] != second {
		t.Errorf("after deleting the first block the messages anchor to %v", byID)
	}
}

func TestABlockOfAnotherObservationOrOnAnotherTargetIsRefused(t *testing.T) {
	r := newRig(t)
	observation := r.createObservation(t, testUser, pgtype.UUID{})
	other := r.createObservation(t, testUser, pgtype.UUID{})
	foreign := r.addBlock(t, other)
	own := r.addBlock(t, observation)
	missing := "00000000-0000-4000-8000-000000000001"
	notAnID := "nope"
	dash := r.createDashboard(t, "rps")
	annotation := r.createAnnotation(t, testUser, dash)

	tests := []struct {
		name   string
		target Target
		block  *string
	}{
		{"a block of another observation", target(KindObservation, observation), &foreign},
		{"a block that does not exist", target(KindObservation, observation), &missing},
		{"an id that is not a UUID", target(KindObservation, observation), &notAnID},
		{"a dashboard", target(KindDashboard, dash), &own},
		{"an annotation", target(KindAnnotation, annotation), &own},
	}
	for _, tt := range tests {
		t.Run("open on "+tt.name, func(t *testing.T) {
			_, _, err := r.service.Open(reading(t.Context()), testUser, OpenInput{Target: tt.target, Body: "hi", BlockID: tt.block}, nil)
			wantInvalid(t, err, CodeInvalidBlock)
		})
	}

	thread, _ := r.open(t, testUser, OpenInput{Target: target(KindObservation, observation), Body: "first"})
	annotationThread, _ := r.open(t, testUser, OpenInput{Target: target(KindAnnotation, annotation), Body: "first"})
	replies := []struct {
		name   string
		thread string
		block  *string
	}{
		{"a block of another observation", thread.ID, &foreign},
		{"a block that does not exist", thread.ID, &missing},
		{"an annotation thread", annotationThread.ID, &own},
	}
	for _, tt := range replies {
		t.Run("reply with "+tt.name, func(t *testing.T) {
			_, _, err := r.service.Reply(reading(t.Context()), testUser, mustParse(t, tt.thread), ReplyInput{Body: "hi", BlockID: tt.block}, nil)
			wantInvalid(t, err, CodeInvalidBlock)
		})
	}
	if page := r.list(t, testUser, target(KindObservation, observation)); len(page.Messages) != 1 {
		t.Errorf("a refused message is not stored: %d messages", len(page.Messages))
	}
}

func TestAReplyQuotesAMessageOfItsOwnThreadWithAnExcerptOfAtMost200Characters(t *testing.T) {
	r := newRig(t)
	tg := target(KindObservation, r.createObservation(t, testUser, pgtype.UUID{}))
	long := strings.Repeat("é", 300)
	thread, first := r.open(t, testUser, OpenInput{Target: tg, Body: long})
	_, short := r.reply(t, commenter, thread.ID, "short one")

	quoteFirst := first.ID
	_, reply, err := r.service.Reply(reading(t.Context()), commenter, mustParse(t, thread.ID), ReplyInput{Body: "agreed", ReplyToID: &quoteFirst}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reply.ReplyTo == nil || reply.ReplyTo.ID != first.ID || reply.ReplyTo.Author.Name != first.Author.Name || reply.ReplyTo.Excerpt != strings.Repeat("é", 200) {
		t.Fatalf("reply_to = %+v", reply.ReplyTo)
	}
	quoteShort := short.ID
	_, second, err := r.service.Reply(reading(t.Context()), testUser, mustParse(t, thread.ID), ReplyInput{Body: "thanks", ReplyToID: &quoteShort}, nil)
	if err != nil || second.ReplyTo == nil || second.ReplyTo.Excerpt != "short one" || second.ReplyTo.Author.Name != short.Author.Name {
		t.Fatalf("second = %+v, %v", second.ReplyTo, err)
	}

	listed := map[string]*ReplyTo{}
	for _, m := range r.list(t, testUser, tg).Messages {
		listed[m.Body] = m.ReplyTo
	}
	if listed["agreed"] == nil || listed["agreed"].ID != first.ID || listed["thanks"] == nil || listed["short one"] != nil || listed[long] != nil {
		t.Errorf("listed quotes = %+v", listed)
	}

	_, edited, _, err := r.service.Edit(reading(t.Context()), commenter, mustParse(t, reply.ID), "agreed, fully")
	if err != nil || edited.ReplyTo == nil || edited.ReplyTo.ID != first.ID {
		t.Errorf("an edit keeps the quote: %+v, %v", edited.ReplyTo, err)
	}

	r.db.Exec(t, `DELETE FROM thread_messages WHERE id = $1::uuid`, short.ID)
	for _, m := range r.list(t, testUser, tg).Messages {
		if m.Body == "thanks" && m.ReplyTo != nil {
			t.Errorf("a deleted quoted message leaves no quote: %+v", m.ReplyTo)
		}
	}
}

func TestAReplyCannotQuoteAMessageOfAnotherThreadOrOneThatDoesNotExist(t *testing.T) {
	r := newRig(t)
	thread, _ := r.open(t, testUser, OpenInput{Target: target(KindObservation, r.createObservation(t, testUser, pgtype.UUID{})), Body: "here"})
	_, elsewhere := r.open(t, testUser, OpenInput{Target: target(KindObservation, r.createObservation(t, testUser, pgtype.UUID{})), Body: "there"})
	missing := "00000000-0000-4000-8000-000000000001"
	notAnID := "nope"

	for name, quoted := range map[string]*string{"another thread": &elsewhere.ID, "no message": &missing, "not a UUID": &notAnID} {
		t.Run(name, func(t *testing.T) {
			_, _, err := r.service.Reply(reading(t.Context()), testUser, mustParse(t, thread.ID), ReplyInput{Body: "hi", ReplyToID: quoted}, nil)
			wantInvalid(t, err, CodeInvalidReplyTo)
		})
	}
}

func TestTheAnchorAndTheQuoteAreOnEveryMessageOverHTTP(t *testing.T) {
	r := newRig(t)
	observation := r.createObservation(t, testUser, pgtype.UUID{})
	block := r.addBlock(t, observation)
	targetValue := "observation:" + observation.String()

	opened := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": targetValue, "body": "on the block", "block_id": block})
	opened.expect(t, http.StatusCreated, "")
	if opened.at(t, "message", "block_id") != block || opened.at(t, "message", "reply_to") != nil {
		t.Fatalf("opened message = %v", opened.at(t, "message"))
	}
	threadID := opened.at(t, "thread", "id").(string)

	replied := r.call(t, commenter, http.MethodPost, "/threads/"+threadID+"/messages", map[string]any{"body": "me too", "reply_to_id": threadID})
	replied.expect(t, http.StatusCreated, "")
	if replied.at(t, "message", "block_id") != nil || replied.at(t, "message", "reply_to", "id") != threadID || replied.at(t, "message", "reply_to", "excerpt") != "on the block" {
		t.Fatalf("replied message = %v", replied.at(t, "message"))
	}
	if replied.at(t, "message", "reply_to", "actor", "id") != testUser.ID {
		t.Errorf("the quote names its author: %v", replied.at(t, "message", "reply_to"))
	}

	page := r.call(t, testUser, http.MethodGet, "/threads?target="+targetValue, nil)
	page.expect(t, http.StatusOK, "")
	messages := page.at(t, "messages").([]any)
	if len(messages) != 2 || messages[0].(map[string]any)["block_id"] != block || messages[1].(map[string]any)["reply_to"] == nil {
		t.Errorf("listed messages = %v", messages)
	}

	r.call(t, testUser, http.MethodPost, "/threads/"+threadID+"/messages", map[string]any{"body": "x", "block_id": "00000000-0000-4000-8000-000000000001"}).expect(t, http.StatusUnprocessableEntity, CodeInvalidBlock)
	r.call(t, testUser, http.MethodPost, "/threads/"+threadID+"/messages", map[string]any{"body": "x", "reply_to_id": "00000000-0000-4000-8000-000000000001"}).expect(t, http.StatusUnprocessableEntity, CodeInvalidReplyTo)
}
