package revisions_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type answerSaves struct {
	saved      map[pgtype.UUID]string
	unanswered pgtype.UUID
	shown      pgtype.UUID
	calls      int
}

func (a *answerSaves) save(_ context.Context, _ auth.Principal, requestID pgtype.UUID) (revisions.SavedAnswer, error) {
	a.calls++
	if requestID == a.unanswered {
		return revisions.SavedAnswer{}, revisions.ErrNoAnswer
	}
	if id, ok := a.saved[requestID]; ok {
		return revisions.SavedAnswer{ObservationID: id, Outcome: revisions.AnswerUnchanged}, nil
	}
	a.saved[requestID] = "55555555-5555-4555-8555-555555555555"
	if requestID == a.shown {
		return revisions.SavedAnswer{ObservationID: a.saved[requestID], Outcome: revisions.AnswerKept}, nil
	}
	return revisions.SavedAnswer{ObservationID: a.saved[requestID], Outcome: revisions.AnswerCreated}, nil
}

func TestSavingARequestsAnswerAsAnObservationMatchesTheDocument(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	answered, err := svc.SubmitObserve(reading(t.Context()), user, "which logs exist?")
	if err != nil {
		t.Fatal(err)
	}
	unanswered, err := svc.SubmitObserve(reading(t.Context()), user, "and the traces?")
	if err != nil {
		t.Fatal(err)
	}
	shown, err := svc.SubmitObserve(reading(t.Context()), user, "and the metrics?")
	if err != nil {
		t.Fatal(err)
	}
	saves := &answerSaves{saved: map[pgtype.UUID]string{}, unanswered: unanswered.ID, shown: shown.ID}
	opts := revisions.ObserveOptions{ObservationURL: func(id string) string { return "/o/" + id }, SaveAnswer: saves.save}
	requester := newDocumentedObserveWith(t, svc, user, opts)
	path := "/api/v1/observe/" + answered.ID.String() + "/observation"

	var first struct {
		ObservationID string `json:"observation_id"`
		URL           string `json:"url"`
		Replayed      bool   `json:"replayed"`
		Outcome       string `json:"outcome"`
	}
	if err := json.Unmarshal(requester.call(t, http.MethodPost, path, "", http.StatusCreated).Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.Outcome != "created" || first.URL != "/o/"+first.ObservationID {
		t.Fatalf("first save answered %+v", first)
	}
	again := requester.call(t, http.MethodPost, path, "", http.StatusOK).Body.String()
	if want := `{"observation_id":"` + first.ObservationID + `","url":"/o/` + first.ObservationID + `","replayed":true,"outcome":"unchanged"}`; again != want {
		t.Fatalf("repeat answered %s, want %s", again, want)
	}
	kept := requester.call(t, http.MethodPost, "/api/v1/observe/"+shown.ID.String()+"/observation", "", http.StatusOK).Body.String()
	if want := `"replayed":false,"outcome":"kept"}`; !strings.HasSuffix(kept, want) {
		t.Fatalf("keeping a shown answer answered %s, want it to end %s", kept, want)
	}

	requester.call(t, http.MethodPost, "/api/v1/observe/"+unanswered.ID.String()+"/observation", "", http.StatusConflict)
	before := saves.calls
	newDocumentedObserveWith(t, svc, stranger, opts).call(t, http.MethodPost, path, "", http.StatusNotFound)
	if saves.calls != before {
		t.Fatalf("a caller who did not ask reached the save")
	}
}

func TestAChatOnlyAnswerShowsWithoutAnObservationAsTheDocumentSays(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	row, err := svc.SubmitObserve(reading(t.Context()), user, "which logs exist?")
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `INSERT INTO findings (agent_request_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1, 'Log sources', 'two sources', now() - interval '1 hour', now(), 'user', $2::uuid, 'Ada')`, row.ID, userID)
	d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_kind = 'observation', attempts = 1, started_at = now(), finished_at = now() WHERE id = $1", row.ID)
	finding := func(_ context.Context, _ auth.Principal, requestID pgtype.UUID) (json.RawMessage, bool, error) {
		if requestID != row.ID {
			return nil, false, nil
		}
		now := time.Now()
		return mustMarshal(t, findings.JSONOf(findings.Finding{
			ID: "66666666-6666-4666-8666-666666666666", AgentRequestID: row.ID.String(), Title: "Log sources", Markdown: "two sources",
			From: now.Add(-time.Hour), To: now, CreatedAt: now,
			CreatedBy: findings.Author{Kind: "user", ID: userID, Name: "Ada"},
			Actor:     actor.Actor{Kind: auth.KindUser, ID: userID, Name: "Ada"},
		}, "")), true, nil
	}
	h := newDocumentedObserve(t, svc, user, finding)

	var got struct {
		Request struct {
			URL           *string `json:"url"`
			ResultKind    string  `json:"result_kind"`
			ObservationID *string `json:"observation_id"`
			Finding       *struct {
				ObservationID *string `json:"observation_id"`
				URL           *string `json:"url"`
			} `json:"finding"`
		} `json:"request"`
	}
	rec := h.call(t, http.MethodGet, "/api/v1/observe/"+row.ID.String(), "", http.StatusOK)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	r := got.Request
	if r.Finding == nil || r.Finding.ObservationID != nil || r.Finding.URL != nil || r.ObservationID != nil || r.URL != nil {
		t.Fatalf("GET of a chat-only answer: %s", rec.Body)
	}
	if r.ResultKind != "answer" {
		t.Fatalf("result_kind of a chat-only answer = %q, want answer", r.ResultKind)
	}
	var listed struct {
		Requests []struct {
			ResultKind string `json:"result_kind"`
		} `json:"requests"`
	}
	list := h.call(t, http.MethodGet, "/api/v1/observe", "", http.StatusOK)
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil || len(listed.Requests) != 1 || listed.Requests[0].ResultKind != "answer" {
		t.Fatalf("list of a chat-only answer: %s", list.Body)
	}
}
