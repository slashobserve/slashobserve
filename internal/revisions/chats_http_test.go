package revisions_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type chatPage struct {
	Chats []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Turns    int    `json:"turns"`
		LastTurn *struct {
			ID     string `json:"id"`
			Prompt string `json:"prompt"`
		} `json:"last_turn"`
	} `json:"chats"`
	NextCursor *string `json:"next_cursor"`
}

func TestTheChatRoutesMatchTheDocumentAndServeOnlyTheOwner(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	first := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs exist?")})
	second := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("and the traces?"), ChatID: text(first.String())})
	startTurn(t, svc, stranger, revisions.StartRequest{Prompt: text("someone else's question")})
	owner := newDocumentedObserve(t, svc, user, nil)
	admin := newDocumentedObserve(t, svc, stranger, nil)
	chat := "/api/v1/chats/" + first.String()

	var listed chatPage
	if err := json.Unmarshal(owner.call(t, http.MethodGet, "/api/v1/chats", "", http.StatusOK).Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Chats) != 1 || listed.Chats[0].ID != first.String() || listed.Chats[0].Title != "which logs exist?" || listed.Chats[0].Turns != 2 ||
		listed.Chats[0].LastTurn == nil || listed.Chats[0].LastTurn.ID != second.String() || listed.NextCursor != nil {
		t.Fatalf("listed %+v", listed)
	}
	for query, want := range map[string]int{"?q=TRACES": 1, "?q=metrics": 0, "?pinned=true": 0, "?pinned=false": 1, "?observation_id=" + first.String(): 0} {
		var page chatPage
		if err := json.Unmarshal(owner.call(t, http.MethodGet, "/api/v1/chats"+query, "", http.StatusOK).Body.Bytes(), &page); err != nil || len(page.Chats) != want {
			t.Errorf("%s listed %d chats, want %d (%v)", query, len(page.Chats), want, err)
		}
	}
	for _, query := range []string{"?pinned=maybe", "?dashboard_id=x", "?limit=51", "?cursor=nope"} {
		owner.call(t, http.MethodGet, "/api/v1/chats"+query, "", http.StatusBadRequest)
	}

	var detail struct {
		Chat struct {
			ID string `json:"id"`
		} `json:"chat"`
		Turns []struct {
			ID     string `json:"id"`
			ChatID string `json:"chat_id"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(owner.call(t, http.MethodGet, chat, "", http.StatusOK).Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Chat.ID != first.String() || len(detail.Turns) != 2 || detail.Turns[0].ID != first.String() || detail.Turns[1].ID != second.String() || detail.Turns[1].ChatID != first.String() {
		t.Fatalf("detail %+v", detail)
	}

	var read struct {
		Chat struct {
			ReadOnly bool `json:"read_only"`
			Owner    struct {
				Name string `json:"name"`
			} `json:"owner"`
		} `json:"chat"`
	}
	if err := json.Unmarshal(admin.call(t, http.MethodGet, chat, "", http.StatusOK).Body.Bytes(), &read); err != nil || !read.Chat.ReadOnly || read.Chat.Owner.Name != user.Name {
		t.Fatalf("an Admin read %+v, %v", read, err)
	}
	for _, refused := range []*httptest.ResponseRecorder{
		admin.call(t, http.MethodDelete, chat, "", http.StatusForbidden),
		admin.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"mine now","chat_id":"`+first.String()+`"}`, http.StatusForbidden),
	} {
		var body struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(refused.Body.Bytes(), &body); err != nil || body.Code != revisions.CodeChatReadOnly {
			t.Fatalf("an Admin changing someone else's chat got %s, %v; want code %s", refused.Body, err, revisions.CodeChatReadOnly)
		}
	}
	var theirs chatPage
	if err := json.Unmarshal(admin.call(t, http.MethodGet, "/api/v1/chats", "", http.StatusOK).Body.Bytes(), &theirs); err != nil || len(theirs.Chats) != 1 || theirs.Chats[0].ID == first.String() {
		t.Fatalf("an Admin listed %+v, %v", theirs, err)
	}

	owner.call(t, http.MethodDelete, chat, "", http.StatusNoContent)
	owner.call(t, http.MethodGet, chat, "", http.StatusNotFound)
	var after chatPage
	if err := json.Unmarshal(owner.call(t, http.MethodGet, "/api/v1/chats", "", http.StatusOK).Body.Bytes(), &after); err != nil || len(after.Chats) != 0 {
		t.Fatalf("after archiving listed %+v, %v", after, err)
	}
	owner.call(t, http.MethodGet, "/api/v1/observe/"+second.String(), "", http.StatusOK)
	owner.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"one more","chat_id":"`+first.String()+`"}`, http.StatusNotFound)
	owner.call(t, http.MethodDelete, "/api/v1/chats/"+second.String(), "", http.StatusNotFound)
}

func TestAResentClientIDAnswersTheSameRequestAsTheDocumentSays(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	requester := newDocumentedObserve(t, svc, user, nil)
	const clientID = "77777777-7777-4777-8777-777777777777"
	body := `{"prompt":"which logs exist?","client_id":"` + clientID + `"}`

	var created, again struct {
		Request struct {
			ID       string `json:"id"`
			Replayed bool   `json:"replayed"`
		} `json:"request"`
	}
	if err := json.Unmarshal(requester.call(t, http.MethodPost, "/api/v1/observe", body, http.StatusAccepted).Body.Bytes(), &created); err != nil || created.Request.Replayed {
		t.Fatalf("first send %+v, %v", created, err)
	}
	if err := json.Unmarshal(requester.call(t, http.MethodPost, "/api/v1/observe", body, http.StatusOK).Body.Bytes(), &again); err != nil || again.Request.ID != created.Request.ID || !again.Request.Replayed {
		t.Fatalf("resend %+v, want %s replayed", again, created.Request.ID)
	}
	rec := requester.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"another question","client_id":"`+clientID+`"}`, http.StatusUnprocessableEntity)
	if code := errorCode(t, rec); code != "client_id_reused" {
		t.Fatalf("reuse answered %q", code)
	}
	requester.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"which logs exist?","client_id":"`+clientID+`","chat_id":"`+clientID+`"}`, http.StatusUnprocessableEntity)
	newDocumentedObserve(t, svc, stranger, nil).call(t, http.MethodPost, "/api/v1/observe", body, http.StatusAccepted)

	var rows int
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM agent_requests WHERE client_id = $1::uuid`, clientID).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("%d requests carry the client_id, want one per requester (%v)", rows, err)
	}
	var turns int
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE action = 'chat.turn'`).Scan(&turns); err != nil || turns != 2 {
		t.Fatalf("%d chat.turn events, want one per queued request (%v)", turns, err)
	}
}

func TestTwoSendsWithOneClientIDAtOnceQueueOneRequest(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	const clientID = "88888888-8888-4888-8888-888888888888"
	req := revisions.StartRequest{Prompt: text("which logs exist?"), ClientID: text(clientID)}
	results := make([]revisions.Started, 2)
	errs := make([]error, 2)
	var start, done sync.WaitGroup
	start.Add(1)
	for i := range results {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			results[i], errs[i] = svc.Start(t.Context(), user, req)
		}()
	}
	start.Done()
	done.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	if results[0].ID != results[1].ID || results[0].Replayed == results[1].Replayed {
		t.Fatalf("sends answered %v (replayed %v) and %v (replayed %v), want one request and one replay", results[0].ID, results[0].Replayed, results[1].ID, results[1].Replayed)
	}
	var rows, chats int
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT (SELECT count(*) FROM agent_requests), (SELECT count(*) FROM chats)`).Scan(&rows, &chats); err != nil || rows != 1 || chats != 1 {
		t.Fatalf("%d requests and %d chats, want one of each (%v)", rows, chats, err)
	}
}

func TestAnAnchoredTurnNeedsItsSubjectAndABlockOfThatObservation(t *testing.T) {
	r := newObservationRig(t)
	router := observeRouterAs(r.svc, user)
	first := investigate(t, r.svc, revisions.InvestigateParams{Prompt: "why are checkout errors up"})
	other := investigate(t, r.svc, revisions.InvestigateParams{Prompt: "why is search slow"})
	block := r.fetch(t, user, first.ObservationID).Blocks[0].ID
	otherBlock := r.fetch(t, user, other.ObservationID).Blocks[0].ID
	spot := `"anchor":{"kind":"position","pos":{"x":0.25,"y":0.5}}`
	observation := `"observation_id":"` + first.ObservationID.String() + `"`

	for name, body := range map[string]string{
		"anchor without a subject":      `{"prompt":"here?",` + spot + `}`,
		"block without an anchor":       `{"prompt":"here?","kind":"investigate",` + observation + `,"block_id":"` + block + `"}`,
		"block without an observation":  `{"prompt":"here?","kind":"investigate","dashboard_id":"` + seedDashboard(t, r.d, 1).String() + `",` + spot + `,"block_id":"` + block + `"}`,
		"anchor that breaks its shape":  `{"prompt":"here?","kind":"investigate",` + observation + `,"anchor":{"kind":"position","pos":{"x":2,"y":0.5}}}`,
		"block id that is not an id":    `{"prompt":"here?","kind":"investigate",` + observation + `,` + spot + `,"block_id":"nope"}`,
		"anchor kind no one knows":      `{"prompt":"here?","kind":"investigate",` + observation + `,"anchor":{"kind":"nowhere","pos":{"x":0.5,"y":0.5}}}`,
		"block without anything at all": `{"prompt":"here?","block_id":"` + block + `"}`,
		"variable anchor on a block":    `{"prompt":"here?","kind":"investigate",` + observation + `,"anchor":{"kind":"variable","pos":{"x":0.5,"y":0.5},"id":"service"},"block_id":"` + block + `"}`,
		"anchor too large to store":     `{"prompt":"here?","kind":"investigate",` + observation + `,"anchor":{"kind":"series","pos":{"x":0.5,"y":0.5},"series":` + largestLabels() + `}}`,
	} {
		if rec := call(router, http.MethodPost, "/api/v1/observe", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}

	if rec := call(router, http.MethodPost, "/api/v1/observe", `{"prompt":"here?","kind":"investigate",`+observation+`,`+spot+`,"block_id":"`+otherBlock+`"}`); rec.Code != http.StatusNotFound {
		t.Errorf("block of another observation: %d %s", rec.Code, rec.Body)
	}
	rec := call(router, http.MethodPost, "/api/v1/observe", `{"prompt":"what happened here?","kind":"investigate",`+observation+`,`+spot+`,"block_id":"`+block+`"}`)
	var created struct {
		Request struct {
			ID string `json:"id"`
		} `json:"request"`
	}
	if rec.Code != http.StatusAccepted || json.Unmarshal(rec.Body.Bytes(), &created) != nil {
		t.Fatalf("anchored turn: %d %s", rec.Code, rec.Body)
	}
	var kind, stored string
	if err := r.d.SQLDB(t).QueryRowContext(t.Context(), `SELECT anchor->>'kind', block_id::text FROM agent_requests WHERE id = $1::uuid`, created.Request.ID).Scan(&kind, &stored); err != nil || kind != "position" || stored != block {
		t.Fatalf("stored point %q on %q, want position on %s (%v)", kind, stored, block, err)
	}
	turns := r.d.AuditEvents(t, "chat.turn")
	if len(turns) != 1 || turns[0].Data["anchor_kind"] != "position" || turns[0].Data["body_length"] != float64(len("what happened here?")) {
		t.Fatalf("chat.turn events = %+v, want one with anchor_kind position and body_length 19", turns)
	}
}

func largestLabels() string {
	labels := map[string]string{}
	for i := range 16 {
		labels[fmt.Sprintf("%02d%s", i, strings.Repeat("k", 198))] = strings.Repeat("v", 200)
	}
	raw, err := json.Marshal(labels)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func TestAStrangersAnchoredAskAnswersTheSameForABlockThatIsAndIsNotThere(t *testing.T) {
	r := newObservationRig(t)
	r.d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', 'ada', 'ada@example.com', 'Ada')`, userID)
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	mine := investigate(t, r.svc, revisions.InvestigateParams{Prompt: "why are checkout errors up"})
	block := r.fetch(t, user, mine.ObservationID).Blocks[0].ID
	router := observeRouterAs(r.svc, eve)
	ask := func(blockID string) (int, string) {
		rec := call(router, http.MethodPost, "/api/v1/observe", `{"prompt":"here?","kind":"investigate","observation_id":"`+mine.ObservationID.String()+`","anchor":{"kind":"position","pos":{"x":0.25,"y":0.5}},"block_id":"`+blockID+`"}`)
		return rec.Code, rec.Body.String()
	}
	thereCode, there := ask(block)
	missingCode, missing := ask("00000000-0000-4000-8000-0000000000b1")
	if thereCode != http.StatusNotFound || thereCode != missingCode || there != missing {
		t.Fatalf("a block that is there answered %d %s, one that is not %d %s; want the same 404", thereCode, there, missingCode, missing)
	}
}
