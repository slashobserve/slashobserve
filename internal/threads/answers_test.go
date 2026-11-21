package threads

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type findingAnswers struct {
	findings *findings.Service
}

func (a findingAnswers) ForRuns(ctx context.Context, by auth.Principal, repliedIn map[pgtype.UUID]pgtype.UUID) (map[pgtype.UUID]Answer, error) {
	found, err := a.findings.ForThreadRuns(ctx, by, repliedIn)
	if err != nil {
		return nil, err
	}
	out := map[pgtype.UUID]Answer{}
	for run, answer := range found {
		raw, err := json.Marshal(findings.JSONOf(answer.Finding, ""))
		if err != nil {
			return nil, err
		}
		out[run] = Answer{RunID: run.String(), FindingID: answer.Finding.ID, Title: answer.Finding.Title, CanSave: answer.Savable, Finding: raw}
	}
	return out, nil
}

func (r *rig) answeredMentionRun(t *testing.T, tg Target) (dbsqlc.AgentRequest, findings.Finding) {
	t.Helper()
	_, run := r.mentionRun(t, tg)
	f := r.answer(t, testUser, findings.SaveInput{AgentRequestID: run.ID})
	finished := ended(run, revisions.StateSucceeded)
	finished.ResultKind = string(revisions.ResultObservation)
	if err := r.runEnded(t, finished); err != nil {
		t.Fatal(err)
	}
	return run, f
}

func (r *rig) answersListed(t *testing.T, by auth.Principal, tg Target) map[string]*Answer {
	t.Helper()
	page, err := r.service.List(reading(t.Context()), by, ListQuery{Target: tg, Status: StatusAll, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*Answer{}
	for _, m := range page.Messages {
		out[m.Body] = m.Answer
	}
	return out
}

func TestAThreadListCarriesTheAnswerOfEachRunReply(t *testing.T) {
	r := newAgentRig(t, true)
	r.service.ReadAnswersWith(findingAnswers{r.findings})
	tg := target(KindDashboard, r.createDashboard(t, "rps", "errors"))
	run, f := r.answeredMentionRun(t, tg)

	for _, tt := range []struct {
		reader  auth.Principal
		canSave bool
	}{{testUser, true}, {viewer, false}} {
		t.Run(tt.reader.Name, func(t *testing.T) {
			listed := r.answersListed(t, tt.reader, tg)
			if a := listed["@observe look at this"]; a != nil {
				t.Fatalf("the question carries an answer %+v", a)
			}
			a := listed[replyAnswered]
			if a == nil || a.RunID != run.ID.String() || a.FindingID != f.ID || a.Title != "Checkout is slow" || a.CanSave != tt.canSave {
				t.Fatalf("the reply's answer = %+v, want finding %s of run %s with can_save %v", a, f.ID, run.ID, tt.canSave)
			}
			var finding findings.FindingJSON
			if err := json.Unmarshal(a.Finding, &finding); err != nil || finding.ID != f.ID || finding.Markdown != "the 5xx rate rose" {
				t.Fatalf("the reply's finding = %+v, %v", finding, err)
			}
		})
	}
}

func TestAnAnswerReadThroughAnotherThreadFallsBackToTheFindingsOwnRule(t *testing.T) {
	r := newAgentRig(t, true)
	tg := target(KindDashboard, r.createDashboard(t, "rps", "errors"))
	run, _ := r.answeredMentionRun(t, tg)
	other, _ := r.open(t, testUser, OpenInput{Target: tg, Version: new(1), PanelID: new("errors"), Body: "another thread"})

	found, err := r.findings.ForThreadRuns(reading(t.Context()), viewer, map[pgtype.UUID]pgtype.UUID{run.ID: mustParse(t, other.ID)})
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := found[run.ID]; ok {
		t.Fatalf("a viewer reads an answer through a thread it was not asked in: %+v", a)
	}
	found, err = r.findings.ForThreadRuns(reading(t.Context()), testUser, map[pgtype.UUID]pgtype.UUID{run.ID: mustParse(t, other.ID)})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := found[run.ID]; !ok {
		t.Fatal("the requester no longer reads their own answer outside its thread")
	}
}

func TestAThreadListWithoutAnAnswerReaderCarriesNoAnswers(t *testing.T) {
	r := newAgentRig(t, true)
	tg := target(KindDashboard, r.createDashboard(t, "rps", "errors"))
	r.answeredMentionRun(t, tg)

	if a := r.answersListed(t, testUser, tg)[replyAnswered]; a != nil {
		t.Fatalf("answer = %+v with no answer reader set", a)
	}
}
