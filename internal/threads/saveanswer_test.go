package threads

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) savedAsObservation(t *testing.T, run, about pgtype.UUID) pgtype.UUID {
	t.Helper()
	saved, _, err := r.findings.SaveAsObservation(reading(t.Context()), testUser, run)
	if err != nil {
		t.Fatalf("save the answer as an observation: %v", err)
	}
	if saved.ObservationID == "" || saved.ObservationID == about.String() {
		t.Fatalf("the answer was saved into %q, want a new observation", saved.ObservationID)
	}
	stored, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ObservationID.String() != saved.ObservationID || stored.SubjectObservationID != about {
		t.Fatalf("run is in observation %v about %v, want in %s about %v", stored.ObservationID, stored.SubjectObservationID, saved.ObservationID, about)
	}
	return mustParse(t, saved.ObservationID)
}

func TestSavingAPrivateAnswerAboutAnObservationFilesANewOne(t *testing.T) {
	r := newShareRig(t)
	about := r.filedObservationIn(t, testUser, pgtype.UUID{})
	id := about.String()
	run := r.privateAnswer(t, revisions.StartRequest{ObservationID: &id}, "prom", "")
	blocks := r.blockCount(t, about)

	saved := r.savedAsObservation(t, run, about)

	if got := r.blockCount(t, about); got != blocks {
		t.Fatalf("the observation the ask was about has %d blocks, want %d", got, blocks)
	}
	if r.blockCount(t, saved) == 0 {
		t.Fatal("the new observation has no blocks")
	}
}

func TestSavingAThreadAnswerFilesANewObservationAndKeepsTheReply(t *testing.T) {
	r := newShareRig(t)
	about := r.filedObservationIn(t, testUser, pgtype.UUID{})
	thread, run := r.mentionRun(t, target(KindObservation, about))
	r.answer(t, testUser, findings.SaveInput{AgentRequestID: run.ID})
	finished := ended(run, revisions.StateSucceeded)
	finished.ResultKind = string(revisions.ResultObservation)
	if err := r.runEnded(t, finished); err != nil {
		t.Fatal(err)
	}
	blocks := r.blockCount(t, about)

	saved := r.savedAsObservation(t, run.ID, about)

	if got := r.blockCount(t, about); got != blocks {
		t.Fatalf("the observation the thread is on has %d blocks, want %d", got, blocks)
	}
	replies := r.runReplies(t, thread)
	if len(replies) != 1 || replies[0].Body != "Answered." || replies[0].RunID != run.ID.String() {
		t.Fatalf("replies = %+v, want the one answer reply", replies)
	}
	if code := r.observeStatus(t, testUser, run.ID); code != http.StatusOK {
		t.Fatalf("the asker reads the run with %d", code)
	}
	f, err := r.findingOfRun(t, testUser, run.ID)
	if err != nil || f.ObservationID != saved.String() {
		t.Fatalf("the asker reads the answer %+v, %v, want it in %s", f, err, saved)
	}
}
