package threads

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
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

func TestASavedPrivateAnswerAboutADashboardStaysAPrivateAsk(t *testing.T) {
	r := newShareRig(t)
	dash := r.createDashboard(t, "rps", "errors")
	id := dash.String()
	run := r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "")
	saved, _, err := r.findings.SaveAsObservation(reading(t.Context()), testUser, run)
	if err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, mustParse(t, saved.ObservationID), foldertest.AdminsOnly(t, r.db, "Admins"))

	if code := r.observeStatus(t, otherEditor, run); code != http.StatusNotFound {
		t.Fatalf("a reader of the dashboard reads the saved private answer with %d", code)
	}
	next := r.runRow(t, retriedOverHTTP(t, r.observeEngine(testUser), run))
	if next.Investigation || !revisions.PrivateAsk(next) || next.DashboardID != dash || next.ChatID != r.runRow(t, run).ChatID {
		t.Fatalf("retry %+v, want a private ask about dashboard %v in the same chat", next, dash)
	}
}

func TestASharedSavedPrivateAnswerSaysItWasWrittenOntoThePage(t *testing.T) {
	r := newShareRig(t)
	_, run, _ := r.savedPrivateAnswer(t)

	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil {
		t.Fatal(err)
	}

	replies := r.runReplies(t, shared.Thread.ID)
	if len(replies) != 1 || replies[0].Body != "Answered." {
		t.Fatalf("replies after the share = %+v", replies)
	}
}

func (r *rig) runSeenBy(t *testing.T, who auth.Principal, run pgtype.UUID) (redacted bool, prompt, title, asker string) {
	t.Helper()
	raw, err := revisions.NewRunDesk(r.revisions, revisions.ObserveOptions{}).Run(reading(t.Context()), run, who, 0)
	if err != nil {
		t.Fatalf("%s reads run %v: %v", who.Name, run, err)
	}
	var shown struct {
		Redacted    bool   `json:"redacted"`
		Prompt      string `json:"prompt"`
		Title       string `json:"title"`
		RequestedBy struct {
			Name string `json:"name"`
		} `json:"requested_by"`
	}
	if err := json.Unmarshal(raw, &shown); err != nil {
		t.Fatal(err)
	}
	return shown.Redacted, shown.Prompt, shown.Title, shown.RequestedBy.Name
}

func TestAReaderOfTheObservationAPrivateAnswerWasSavedIntoReadsTheRunWithoutItsQuestion(t *testing.T) {
	r := newShareRig(t)
	about := r.filedObservationIn(t, testUser, foldertest.Open(t, r.db, "Shared"))
	id := about.String()
	run := r.privateAnswer(t, revisions.StartRequest{ObservationID: &id}, "prom", "")
	r.savedAsObservation(t, run, about)

	if redacted, prompt, title, asker := r.runSeenBy(t, otherEditor, run); !redacted || prompt != "" || title != "" || asker != testUser.Name {
		t.Fatalf("a reader of the saved answer sees redacted %v, prompt %q, title %q, asker %q", redacted, prompt, title, asker)
	}
	if redacted, prompt, _, _ := r.runSeenBy(t, testUser, run); redacted || !strings.Contains(prompt, sharedWord) {
		t.Fatalf("the requester sees redacted %v and prompt %q", redacted, prompt)
	}
	if _, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run}); err != nil {
		t.Fatal(err)
	}
	if redacted, prompt, _, _ := r.runSeenBy(t, otherEditor, run); redacted || !strings.Contains(prompt, sharedWord) {
		t.Fatalf("after the share, a reader sees redacted %v and prompt %q, want the question it was asked in the thread", redacted, prompt)
	}
}

func TestASharedPrivateAnswerStaysRedactedForAReaderOfItsObservationWhoCannotReadTheThread(t *testing.T) {
	r := newShareRig(t)
	about := r.filedObservationIn(t, testUser, foldertest.Open(t, r.db, "Shared"))
	id := about.String()
	run := r.privateAnswer(t, revisions.StartRequest{ObservationID: &id}, "prom", "")
	saved := r.savedAsObservation(t, run, about)
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, saved, foldertest.Open(t, r.db, "Team"))
	if _, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run}); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, about, foldertest.Restricted(t, r.db, "Payroll", "payroll-team"))

	raw, err := revisions.NewRunDesk(r.revisions, revisions.ObserveOptions{}).Run(reading(t.Context()), run, otherEditor, 0)
	if err != nil {
		t.Fatalf("a reader of the saved answer reads the run: %v", err)
	}
	var shown struct {
		Redacted       bool     `json:"redacted"`
		ReadOnly       bool     `json:"read_only"`
		Prompt         string   `json:"prompt"`
		AllowedActions []string `json:"allowed_actions"`
	}
	if err := json.Unmarshal(raw, &shown); err != nil {
		t.Fatal(err)
	}
	if !shown.Redacted || !shown.ReadOnly || shown.Prompt != "" || len(shown.AllowedActions) != 0 {
		t.Fatalf("a reader of the saved answer who cannot read the thread sees %+v, want it redacted and read only", shown)
	}
	rec := httptest.NewRecorder()
	r.observeEngine(otherEditor).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/observe/"+run.String()+"/retry", nil))
	if rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), sharedWord) {
		t.Fatalf("their retry answered %d: %s", rec.Code, rec.Body.String())
	}
}

type subjectSeen struct {
	run, turn, listedTurn *string
}

func (r *rig) subjectSeenBy(t *testing.T, who auth.Principal, run pgtype.UUID) subjectSeen {
	t.Helper()
	desk := revisions.NewRunDesk(r.revisions, revisions.ObserveOptions{})
	raw, err := desk.Run(reading(t.Context()), run, who, 0)
	if err != nil {
		t.Fatalf("%s reads run %v: %v", who.Name, run, err)
	}
	type turn struct {
		ID      string  `json:"id"`
		Subject *string `json:"subject_observation_id"`
	}
	var shown struct {
		Subject *string `json:"subject_observation_id"`
		ChatID  *string `json:"chat_id"`
	}
	if err := json.Unmarshal(raw, &shown); err != nil {
		t.Fatal(err)
	}
	seen := subjectSeen{run: shown.Subject}
	if shown.ChatID == nil {
		return seen
	}
	raw, err = desk.Chat(reading(t.Context()), who, mustParse(t, *shown.ChatID))
	if err != nil {
		t.Fatal(err)
	}
	var detail struct {
		Chat struct {
			LastTurn turn `json:"last_turn"`
		} `json:"chat"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	seen.turn = detail.Chat.LastTurn.Subject
	raw, err = desk.Chats(reading(t.Context()), who, revisions.ChatFilter{}, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	var chats struct {
		Chats []struct {
			ID       string `json:"id"`
			LastTurn turn   `json:"last_turn"`
		} `json:"chats"`
	}
	if err := json.Unmarshal(raw, &chats); err != nil {
		t.Fatal(err)
	}
	for _, c := range chats.Chats {
		if c.ID == *shown.ChatID && c.LastTurn.ID == run.String() {
			seen.listedTurn = c.LastTurn.Subject
		}
	}
	return seen
}

func TestTheRequesterReadsTheObservationAPrivateAskIsAboutAfterItsAnswerIsSaved(t *testing.T) {
	r := newShareRig(t)
	about := r.filedObservationIn(t, testUser, foldertest.Open(t, r.db, "Shared"))
	id := about.String()
	run := r.privateAnswer(t, revisions.StartRequest{ObservationID: &id}, "prom", "")
	same := func(got *string) bool { return got != nil && *got == id }

	for _, when := range []string{"answered", "saved"} {
		if when == "saved" {
			r.savedAsObservation(t, run, about)
		}
		if seen := r.subjectSeenBy(t, testUser, run); !same(seen.run) || !same(seen.turn) || !same(seen.listedTurn) {
			t.Fatalf("%s: the requester reads the subject as run %v, chat turn %v, listed turn %v, want %s", when, seen.run, seen.turn, seen.listedTurn, id)
		}
	}
	if seen := r.subjectSeenBy(t, otherEditor, run); seen.run != nil {
		t.Fatalf("a reader who gets the run redacted reads its subject %s", *seen.run)
	}
}
