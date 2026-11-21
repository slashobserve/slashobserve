package threads

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) savedPrivateAnswer(t *testing.T) (about, run, saved pgtype.UUID) {
	t.Helper()
	about = r.filedObservationIn(t, testUser, foldertest.Open(t, r.db, "Shared"))
	id := about.String()
	run = r.privateAnswer(t, revisions.StartRequest{ObservationID: &id}, "prom", "")
	saved = r.savedAsObservation(t, run, about)
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, saved, foldertest.AdminsOnly(t, r.db, "Admins"))
	return about, run, saved
}

func (r *rig) findingObservation(t *testing.T, run pgtype.UUID) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT observation_id FROM findings WHERE agent_request_id = $1`, run).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestASavedPrivateAnswerIsNotReadByTheReadersOfTheObservationItWasAbout(t *testing.T) {
	r := newShareRig(t)
	_, run, _ := r.savedPrivateAnswer(t)

	if code := r.observeStatus(t, commenter, run); code != http.StatusNotFound {
		t.Fatalf("a reader of the observation the ask was about reads the run with %d", code)
	}
	if _, err := r.findingOfRun(t, commenter, run); !errors.Is(err, findings.ErrNotFound) {
		t.Fatalf("a reader of the observation the ask was about reads the answer: %v", err)
	}
}

func TestASavedPrivateAnswerIsSharedOntoThePageItWasAboutAndSharedAgainAfterAWithdraw(t *testing.T) {
	r := newShareRig(t)
	about, run, saved := r.savedPrivateAnswer(t)
	blocks := r.blockCount(t, about)

	for round := range 2 {
		shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
		if err != nil {
			t.Fatalf("share %d: %v", round+1, err)
		}
		if round == 0 {
			blocks += 2
		}
		if got := r.blockCount(t, about); got != blocks {
			t.Fatalf("share %d left %d blocks on the page, want %d", round+1, got, blocks)
		}
		if got := r.findingObservation(t, run); got != saved {
			t.Fatalf("share %d moved the answer to %v, want it kept in %v", round+1, got, saved)
		}
		if code := r.observeStatus(t, commenter, run); code != http.StatusOK {
			t.Fatalf("a reader of the thread reads the shared run with %d", code)
		}
		if _, err := r.findingOfRun(t, commenter, run); err != nil {
			t.Fatalf("a reader of the thread reads the shared answer: %v", err)
		}

		if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); err != nil {
			t.Fatalf("withdraw %d: %v", round+1, err)
		}
		if got := r.findingObservation(t, run); got != saved {
			t.Fatalf("withdraw %d moved the answer to %v, want it kept in %v", round+1, got, saved)
		}
		if code := r.observeStatus(t, commenter, run); code != http.StatusNotFound {
			t.Fatalf("a reader of the thread reads the withdrawn run with %d", code)
		}
	}
}

func TestARetryOfASavedPrivateAnswerIsAPrivateAskAboutTheSameObservation(t *testing.T) {
	r := newShareRig(t)
	about, run, _ := r.savedPrivateAnswer(t)

	next := r.runRow(t, retriedOverHTTP(t, r.observeEngine(testUser), run))

	if !revisions.PrivateAsk(next) || next.SubjectObservationID != about || next.ChatID != r.runRow(t, run).ChatID {
		t.Fatalf("retry %+v, want a private ask about %v in the same chat", next, about)
	}
}
