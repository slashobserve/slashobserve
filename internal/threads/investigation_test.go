package threads

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type investigationSeen struct {
	stored, run, listed, turn bool
}

func (r *rig) investigationSeen(t *testing.T, run pgtype.UUID) investigationSeen {
	t.Helper()
	desk := revisions.NewRunDesk(r.revisions, revisions.ObserveOptions{})
	row := r.runRow(t, run)
	seen := investigationSeen{stored: row.Investigation, turn: row.Investigation}
	raw, err := desk.Run(reading(t.Context()), run, testUser, 0)
	if err != nil {
		t.Fatalf("read the run: %v", err)
	}
	var shown struct {
		Investigation bool `json:"investigation"`
	}
	if err := json.Unmarshal(raw, &shown); err != nil {
		t.Fatal(err)
	}
	seen.run = shown.Investigation
	raw, err = desk.List(reading(t.Context()), testUser, revisions.ListFilter{}, 50, "")
	if err != nil {
		t.Fatalf("list the runs: %v", err)
	}
	var page struct {
		Requests []struct {
			ID            string `json:"id"`
			Investigation bool   `json:"investigation"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, item := range page.Requests {
		if item.ID == run.String() {
			seen.listed, listed = item.Investigation, true
		}
	}
	if !listed {
		t.Fatalf("run %v is not listed", run)
	}
	if row.ChatID.Valid {
		type turn struct {
			ID            string `json:"id"`
			Investigation bool   `json:"investigation"`
		}
		raw, err = desk.Chat(reading(t.Context()), testUser, row.ChatID)
		if err != nil {
			t.Fatalf("read the chat: %v", err)
		}
		var detail struct {
			Chat struct {
				LastTurn turn `json:"last_turn"`
			} `json:"chat"`
		}
		if err := json.Unmarshal(raw, &detail); err != nil {
			t.Fatal(err)
		}
		raw, err = desk.Chats(reading(t.Context()), testUser, revisions.ChatFilter{}, 50, "")
		if err != nil {
			t.Fatalf("list the chats: %v", err)
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
		var listedTurn turn
		for _, c := range chats.Chats {
			if c.ID == row.ChatID.String() {
				listedTurn = c.LastTurn
			}
		}
		if detail.Chat.LastTurn.ID != run.String() || listedTurn.ID != run.String() {
			t.Fatalf("the chat's last turn is %s, listed %s, want %v", detail.Chat.LastTurn.ID, listedTurn.ID, run)
		}
		seen.turn = listedTurn.Investigation
		if detail.Chat.LastTurn.Investigation != listedTurn.Investigation {
			t.Fatalf("the chat reads investigation %v, the chat list %v", detail.Chat.LastTurn.Investigation, listedTurn.Investigation)
		}
	}
	return seen
}

func (r *rig) wantInvestigation(t *testing.T, run pgtype.UUID, when string, want bool) {
	t.Helper()
	if got := r.investigationSeen(t, run); got != (investigationSeen{want, want, want, want}) {
		t.Fatalf("%s: investigation seen as %+v, want %v everywhere", when, got, want)
	}
}

func TestAnAskIsNeverAnInvestigationAndAnInvestigationAlwaysIs(t *testing.T) {
	t.Run("an ask about a dashboard", func(t *testing.T) {
		r := newShareRig(t)
		dash := r.createDashboard(t, "rps", "errors")
		id := dash.String()
		run := r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "errors")
		r.wantInvestigation(t, run, "answered", false)
		shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
		if err != nil {
			t.Fatal(err)
		}
		r.wantInvestigation(t, run, "shared", false)
		if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); err != nil {
			t.Fatal(err)
		}
		r.wantInvestigation(t, run, "withdrawn", false)
		if _, _, err := r.findings.SaveAsObservation(reading(t.Context()), testUser, run); err != nil {
			t.Fatal(err)
		}
		r.wantInvestigation(t, run, "saved", false)
	})

	t.Run("an ask about an observation", func(t *testing.T) {
		r := newShareRig(t)
		about := r.filedObservationIn(t, testUser, pgtype.UUID{})
		id := about.String()
		run := r.privateAnswer(t, revisions.StartRequest{ObservationID: &id}, "prom", "")
		r.wantInvestigation(t, run, "answered", false)
		r.savedAsObservation(t, run, about)
		r.wantInvestigation(t, run, "saved", false)
		shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
		if err != nil {
			t.Fatal(err)
		}
		r.wantInvestigation(t, run, "shared", false)
		if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); err != nil {
			t.Fatal(err)
		}
		r.wantInvestigation(t, run, "withdrawn", false)
	})

	t.Run("a thread ask", func(t *testing.T) {
		r := newShareRig(t)
		about := r.filedObservationIn(t, testUser, pgtype.UUID{})
		_, _, started, refusal := r.openWithRun(t, testUser, OpenInput{Target: target(KindObservation, about), Body: "@observe look at this"})
		if started == nil || refusal != nil {
			t.Fatalf("started = %+v, refusal = %+v", started, refusal)
		}
		run := mustParse(t, started.ID)
		r.answer(t, testUser, findings.SaveInput{AgentRequestID: run})
		r.finishWithAnswer(t, run)
		r.wantInvestigation(t, run, "answered in the thread", false)
		if _, _, err := r.findings.SaveAsObservation(reading(t.Context()), testUser, run); err != nil {
			t.Fatal(err)
		}
		r.wantInvestigation(t, run, "saved", false)
	})

	t.Run("an investigation", func(t *testing.T) {
		r := newShareRig(t)
		prompt, kind := "why does "+sharedWord+" fail", "investigate"
		started, err := r.revisions.Start(reading(t.Context()), testUser, revisions.StartRequest{Prompt: &prompt, Kind: &kind})
		if err != nil {
			t.Fatal(err)
		}
		run := started.ID
		r.wantInvestigation(t, run, "queued", true)
		r.answer(t, testUser, findings.SaveInput{AgentRequestID: run})
		r.finishWithAnswer(t, run)
		r.wantInvestigation(t, run, "filed", true)
		saved, _, err := r.findings.SaveAsObservation(reading(t.Context()), testUser, run)
		if err != nil || saved.ObservationID != r.runRow(t, run).ObservationID.String() {
			t.Fatalf("saving an investigation's answer gave %q, %v, want the observation it filed", saved.ObservationID, err)
		}
		if _, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run}); err == nil {
			t.Fatal("an investigation's answer was shared as an ask")
		}
		r.wantInvestigation(t, run, "after the refused save and share", true)
	})
}
