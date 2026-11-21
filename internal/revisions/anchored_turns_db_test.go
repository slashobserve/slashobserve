package revisions_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type pointedTurn struct {
	ID      string          `json:"id"`
	Anchor  json.RawMessage `json:"anchor"`
	BlockID *string         `json:"block_id"`
}

func decodeInto[T any](t *testing.T, body []byte) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return out
}

func wantPointed(t *testing.T, where string, got pointedTurn, shown bool) {
	t.Helper()
	if shown {
		if string(got.Anchor) == "null" || len(got.Anchor) == 0 {
			t.Errorf("%s: the anchor is missing", where)
		}
		return
	}
	if string(got.Anchor) != "null" || got.BlockID != nil {
		t.Errorf("%s: shows anchor %s and block %v to someone who did not ask", where, got.Anchor, got.BlockID)
	}
}

func TestATurnsAnchorIsShownOnlyToTheOneWhoAskedIt(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	spot := &anchor.Anchor{Kind: anchor.Row, Pos: anchor.Pos{X: 0.4, Y: 0.3}, Row: map[string]string{"service": "payments"}, Shown: "440"}
	asked := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("why here"), Kind: text("ask"), DashboardID: text(dash.String()), Anchor: spot})
	revised := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("change here"), Kind: text("revise"), DashboardID: text(dash.String()), Anchor: spot})
	plain := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("no spot"), Kind: text("ask"), DashboardID: text(dash.String())})
	chat := chatOfTurn(t, d, asked)

	owner := newDocumentedObserve(t, svc, user, nil)
	admin := newDocumentedObserve(t, svc, bea, nil)
	second := observeRouterAs(svc, stranger)

	for _, id := range []string{asked.String(), revised.String()} {
		wantPointed(t, "owner get "+id, decodeInto[struct {
			Request pointedTurn `json:"request"`
		}](t, owner.call(t, http.MethodGet, "/api/v1/observe/"+id, "", http.StatusOK).Body.Bytes()).Request, true)
	}
	wantPointed(t, "owner get of a turn with no anchor", decodeInto[struct {
		Request pointedTurn `json:"request"`
	}](t, owner.call(t, http.MethodGet, "/api/v1/observe/"+plain.String(), "", http.StatusOK).Body.Bytes()).Request, false)

	rec := call(second, http.MethodGet, "/api/v1/observe/"+revised.String(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("a second person reading the revision: %d %s", rec.Code, rec.Body)
	}
	wantPointed(t, "second person get", decodeInto[struct {
		Request pointedTurn `json:"request"`
	}](t, rec.Body.Bytes()).Request, false)
	wantPointed(t, "admin get", decodeInto[struct {
		Request pointedTurn `json:"request"`
	}](t, admin.call(t, http.MethodGet, "/api/v1/observe/"+asked.String(), "", http.StatusOK).Body.Bytes()).Request, false)

	list := decodeInto[struct {
		Requests []pointedTurn `json:"requests"`
	}](t, owner.call(t, http.MethodGet, "/api/v1/observe", "", http.StatusOK).Body.Bytes())
	shownIn := map[string]bool{}
	for _, r := range list.Requests {
		shownIn[r.ID] = string(r.Anchor) != "null"
	}
	if !shownIn[asked.String()] || !shownIn[revised.String()] || shownIn[plain.String()] {
		t.Errorf("owner list shows anchors %v", shownIn)
	}
	for _, r := range decodeInto[struct {
		Requests []pointedTurn `json:"requests"`
	}](t, admin.call(t, http.MethodGet, "/api/v1/observe?scope=all", "", http.StatusOK).Body.Bytes()).Requests {
		wantPointed(t, "admin list of all "+r.ID, r, false)
	}

	detail := decodeInto[struct {
		Chat struct {
			LastTurn pointedTurn `json:"last_turn"`
		} `json:"chat"`
		Turns []pointedTurn `json:"turns"`
	}](t, owner.call(t, http.MethodGet, "/api/v1/chats/"+chat, "", http.StatusOK).Body.Bytes())
	if len(detail.Turns) == 0 {
		t.Fatal("the chat has no turns")
	}
	wantPointed(t, "owner chat turn", detail.Turns[0], true)
	read := decodeInto[struct {
		Chat struct {
			LastTurn pointedTurn `json:"last_turn"`
		} `json:"chat"`
		Turns []pointedTurn `json:"turns"`
	}](t, admin.call(t, http.MethodGet, "/api/v1/chats/"+chat, "", http.StatusOK).Body.Bytes())
	for _, turn := range read.Turns {
		wantPointed(t, "admin read-only chat turn", turn, false)
	}
	wantPointed(t, "admin read-only chat last turn", read.Chat.LastTurn, false)

	chats := decodeInto[struct {
		Chats []struct {
			ID       string      `json:"id"`
			LastTurn pointedTurn `json:"last_turn"`
		} `json:"chats"`
	}](t, owner.call(t, http.MethodGet, "/api/v1/chats", "", http.StatusOK).Body.Bytes())
	found := false
	for _, c := range chats.Chats {
		if c.ID == chat {
			found = true
			wantPointed(t, "owner chat list last turn", c.LastTurn, true)
		}
	}
	if !found {
		t.Fatalf("the owner's chat list lacks chat %s", chat)
	}
}

func TestAnchoredKeepsOnlyTheCallersOwnAnchoredTurns(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	const observation = "eeeeeeee-0000-4000-8000-0000000000a1"
	d.Exec(t, `INSERT INTO observations (id, folder_id, title, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, general_folder_id(), 'Checkout errors', 'user', $2::uuid, 'Ada')`, observation, userID)
	spot := &anchor.Anchor{Kind: anchor.Position, Pos: anchor.Pos{X: 0.5, Y: 0.5}}
	onDashboard := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("here?"), Kind: text("ask"), DashboardID: text(dash.String()), Anchor: spot})
	plainOnDashboard := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("anywhere"), Kind: text("ask"), DashboardID: text(dash.String())})
	onObservation := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("this part?"), Kind: text("ask"), ObservationID: text(observation), Anchor: spot})
	plainOnObservation := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("whole page"), Kind: text("ask"), ObservationID: text(observation)})
	theirs := startTurn(t, svc, stranger, revisions.StartRequest{Prompt: text("their spot"), Kind: text("ask"), DashboardID: text(dash.String()), Anchor: spot})

	owner := newDocumentedObserve(t, svc, user, nil)
	admin := newDocumentedObserve(t, svc, bea, nil)
	ids := func(query string) []string {
		page := decodeInto[listedPage](t, owner.call(t, http.MethodGet, "/api/v1/observe"+query, "", http.StatusOK).Body.Bytes())
		got := page.ids()
		slices.Sort(got)
		return got
	}
	sorted := func(in ...string) []string {
		slices.Sort(in)
		return in
	}
	for query, want := range map[string][]string{
		"?anchored=true":                                            sorted(onDashboard.String(), onObservation.String()),
		"?anchored=true&scope=mine":                                 sorted(onDashboard.String(), onObservation.String()),
		"?anchored=true&dashboard_id=" + dash.String():              sorted(onDashboard.String()),
		"?anchored=true&observation_id=" + observation:              sorted(onObservation.String()),
		"?observation_id=" + observation:                            sorted(onObservation.String(), plainOnObservation.String()),
		"?anchored=false&dashboard_id=" + dash.String():             sorted(onDashboard.String(), plainOnDashboard.String()),
		"?anchored=true&state=queued&dashboard_id=" + dash.String(): sorted(onDashboard.String()),
	} {
		if got := ids(query); !slices.Equal(got, want) {
			t.Errorf("%s listed %v, want %v", query, got, want)
		}
	}
	if got := decodeInto[struct {
		Summary struct {
			Total int64 `json:"total"`
		} `json:"summary"`
	}](t, owner.call(t, http.MethodGet, "/api/v1/observe?anchored=true", "", http.StatusOK).Body.Bytes()).Summary.Total; got != 2 {
		t.Errorf("the summary counts %d anchored turns, want 2", got)
	}

	for _, who := range []*documentedObserve{owner, admin} {
		for _, query := range []string{"?anchored=true&scope=all", "?anchored=maybe", "?observation_id=nope"} {
			who.call(t, http.MethodGet, "/api/v1/observe"+query, "", http.StatusBadRequest)
		}
	}
	if rec := call(observeRouterAs(svc, stranger), http.MethodGet, "/api/v1/observe?anchored=true&scope=all", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("a non-Admin asking for everyone's anchored turns: %d %s", rec.Code, rec.Body)
	}
	for _, r := range decodeInto[struct {
		Requests []pointedTurn `json:"requests"`
	}](t, call(observeRouterAs(svc, stranger), http.MethodGet, "/api/v1/observe?anchored=true", "").Body.Bytes()).Requests {
		if r.ID != theirs.String() {
			t.Errorf("a second person listed the turn %s that is not theirs", r.ID)
		}
	}
}
