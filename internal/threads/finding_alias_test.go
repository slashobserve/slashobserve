package threads

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestAFindingTargetListsTheObservationsDiscussion(t *testing.T) {
	r := newRig(t)
	finding := r.createFinding(t, testUser, pgtype.UUID{})
	observation := target(KindObservation, r.observationOf(t, finding))
	thread, _ := r.open(t, testUser, OpenInput{Target: observation, Body: "what changed?"})

	page := r.list(t, viewer, target(KindFinding, finding))
	if len(page.Threads) != 1 || page.Threads[0].ID != thread.ID || page.Threads[0].Target != observation || len(page.Messages) != 1 {
		t.Fatalf("page %+v", page)
	}
	abilities, err := r.service.Abilities(t.Context(), commenter, target(KindFinding, finding))
	if err != nil || !abilities.Comment {
		t.Errorf("abilities %+v, %v", abilities, err)
	}
}

func TestOpeningOnAFindingTargetOpensTheObservationsDiscussionThenReplies(t *testing.T) {
	r := newRig(t)
	finding := r.createFinding(t, testUser, pgtype.UUID{})
	observation := target(KindObservation, r.observationOf(t, finding))

	first, firstMessage := r.open(t, testUser, OpenInput{Target: target(KindFinding, finding), Body: "first"})
	if first.Target != observation {
		t.Fatalf("thread target %s, want %s", first.Target, observation)
	}
	second, secondMessage := r.open(t, commenter, OpenInput{Target: target(KindFinding, finding), Body: "second"})
	if second.ID != first.ID || secondMessage.ThreadID != first.ID || secondMessage.ID == firstMessage.ID {
		t.Fatalf("second open: thread %s message %+v, want a reply in %s", second.ID, secondMessage, first.ID)
	}
	if page := r.list(t, testUser, observation); len(page.Threads) != 1 || len(page.Messages) != 2 {
		t.Errorf("observation page %+v", page)
	}
}

func TestAFindingTargetRepliesToADiscussionOpenedOnTheObservation(t *testing.T) {
	r := newRig(t)
	finding := r.createFinding(t, testUser, pgtype.UUID{})
	thread, _ := r.open(t, testUser, OpenInput{Target: target(KindObservation, r.observationOf(t, finding)), Body: "first"})

	again, message := r.open(t, testUser, OpenInput{Target: target(KindFinding, finding), Body: "second"})
	if again.ID != thread.ID || message.ThreadID != thread.ID {
		t.Fatalf("thread %s message %+v, want a reply in %s", again.ID, message, thread.ID)
	}
}

func TestOpeningOnTheObservationItselfStillAnswersTheExistingDiscussion(t *testing.T) {
	r := newRig(t)
	finding := r.createFinding(t, testUser, pgtype.UUID{})
	tg := "observation:" + r.observationOf(t, finding).String()
	first := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": "finding:" + finding.String(), "body": "first"})
	first.expect(t, http.StatusCreated, "")

	second := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": tg, "body": "second"})
	second.expect(t, http.StatusConflict, CodeObservationThreadExists)
	if second.at(t, "thread_id") != first.at(t, "thread", "id") {
		t.Errorf("409 body %v, want the id of %v", second.body, first.at(t, "thread", "id"))
	}
}

func TestAnUnknownFindingTargetIsNotFound(t *testing.T) {
	r := newRig(t)
	missing := target(KindFinding, mustParse(t, "00000000-0000-4000-8000-000000000000"))
	if _, _, err := r.service.Open(t.Context(), testUser, OpenInput{Target: missing, Body: "hi"}, nil); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("open: %v", err)
	}
	if _, err := r.service.Abilities(t.Context(), testUser, missing); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("abilities: %v", err)
	}
	r.call(t, testUser, http.MethodGet, "/threads?target="+missing.String(), nil).expect(t, http.StatusNotFound, "not_found")
	r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": missing.String(), "body": "hi"}).expect(t, http.StatusNotFound, "not_found")
}

func TestAFindingTargetWhoseObservationFolderIsHiddenIsNotFound(t *testing.T) {
	r := newRig(t)
	finding := r.createFindingIn(t, admin, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))
	tg := "finding:" + finding.String()
	r.call(t, admin, http.MethodPost, "/threads", map[string]any{"target": tg, "body": "private"}).expect(t, http.StatusCreated, "")

	r.call(t, commenter, http.MethodGet, "/threads?target="+tg, nil).expect(t, http.StatusNotFound, "not_found")
	r.call(t, commenter, http.MethodPost, "/threads", map[string]any{"target": tg, "body": "hi"}).expect(t, http.StatusNotFound, "not_found")
}
