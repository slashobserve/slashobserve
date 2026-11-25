package tools

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
)

func (r activityRig) mentionOf(t *testing.T, person auth.Principal) {
	t.Helper()
	_, err := r.db.SQLDB(t).ExecContext(reading(t.Context()), `INSERT INTO activity (type, target_kind, target_id, folder_id, actor_kind, actor_id, actor_name, data)
		VALUES ('thread.mentioned', 'thread', $1::uuid, general_folder_id(), 'user', $2::uuid, 'Ola', jsonb_build_object('mentioned', jsonb_build_object('kind', 'user', 'id', $3::text)))`,
		uuid.NewString(), uuid.NewString(), person.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func (r activityRig) inbox(t *testing.T, by auth.Principal, input string) (activity.InboxJSON, Result) {
	t.Helper()
	res, err := r.registry.Call(auth.WithPrincipal(t.Context(), by), nameListInbox, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		return activity.InboxJSON{}, res
	}
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	var out activity.InboxJSON
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("content %s: %v", raw, err)
	}
	return out, res
}

func TestListInboxGivesAPersonTheirForYouItems(t *testing.T) {
	r := newActivityRig(t)
	r.mentionOf(t, viewerCaller)
	r.mentionOf(t, editorCaller)

	out, _ := r.inbox(t, viewerCaller, `{}`)

	if len(out.Items) != 1 {
		t.Fatalf("items = %d, want the one mention of this person", len(out.Items))
	}
	item := out.Items[0]
	if item.Type != activity.TypeThreadMentioned || item.Reason == nil || *item.Reason != string(activity.ReasonMention) || !item.Unread {
		t.Errorf("item = %+v", item)
	}
	if out.UnreadCount != 1 || out.NextCursor != nil {
		t.Errorf("unread_count %d next_cursor %v", out.UnreadCount, out.NextCursor)
	}
}

func TestListInboxAnswersWhatTheRouteAnswers(t *testing.T) {
	r := newActivityRig(t)
	for range 3 {
		r.mentionOf(t, viewerCaller)
	}

	out, res := r.inbox(t, viewerCaller, `{"view":"mentions","limit":2}`)

	if len(out.Items) != 2 || out.NextCursor == nil {
		t.Fatalf("first page = %d items, next_cursor %v", len(out.Items), out.NextCursor)
	}
	want, err := r.registry.deps.Activity.Inbox(auth.WithPrincipal(t.Context(), viewerCaller), viewerCaller, activity.ViewMentions, activity.TriageOpen, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := json.Marshal(activity.InboxJSONOf(want))
	gotJSON, _ := json.Marshal(res.Content)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("tool %s\nstore %s", gotJSON, wantJSON)
	}
	cursor, _ := json.Marshal(*out.NextCursor)
	last, _ := r.inbox(t, viewerCaller, `{"view":"mentions","limit":2,"cursor":`+string(cursor)+`}`)
	if len(last.Items) != 1 || last.NextCursor != nil {
		t.Fatalf("second page = %d items, next_cursor %v", len(last.Items), last.NextCursor)
	}
}

func TestListInboxRefusesAKey(t *testing.T) {
	r := newActivityRig(t)

	_, res := r.inbox(t, keyCaller, `{}`)

	body, _ := res.Content.(errorBody)
	if !res.IsError || body.Code != codeUserSessionRequired {
		t.Fatalf("error %v, %+v", res.IsError, body)
	}
}

func TestListInboxRefusesBadInput(t *testing.T) {
	r := newActivityRig(t)
	cases := map[string]struct {
		input   string
		message string
	}{
		"limit 0":        {`{"limit":0}`, "limit must be between 1 and 100"},
		"limit 101":      {`{"limit":101}`, "limit must be between 1 and 100"},
		"unknown view":   {`{"view":"all"}`, "view must be one of for_you, everything, proposals, mentions"},
		"unknown filter": {`{"filter":"archived"}`, "filter must be one of open, unread, done"},
		"bad cursor":     {`{"cursor":"nope"}`, "cursor is not valid"},
		"cursor too big": {`{"cursor":"` + strings.Repeat("c", maxNameBytes+1) + `"}`, "cursor must be at most 256 bytes"},
	}
	for name, c := range cases {
		_, res := r.inbox(t, viewerCaller, c.input)
		body, _ := res.Content.(errorBody)
		if !res.IsError || body.Code != codeInvalidRequest || body.Message != c.message {
			t.Errorf("%s: error %v, %+v", name, res.IsError, body)
		}
	}
}

func TestListInboxIsOfferedInEveryRunAndNeedsAViewer(t *testing.T) {
	var tool Tool
	for _, candidate := range New(Deps{}).List() {
		if candidate.Name == nameListInbox {
			tool = candidate
		}
	}
	if tool.MCPOnly || !tool.ReadOnly || tool.MinRole != auth.RoleViewer {
		t.Fatalf("list_inbox = %+v", tool)
	}
	for _, mode := range []Mode{ModeCreate, ModeRevise, ModeSuggest, ModeInvestigate} {
		if !tool.OfferedIn(mode) {
			t.Errorf("list_inbox is not offered to the agent in %s", mode)
		}
	}
}

func (r activityRig) setDone(t *testing.T, by auth.Principal, input string) Result {
	t.Helper()
	res, err := r.registry.Call(auth.WithPrincipal(t.Context(), by), nameSetInboxItemDone, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSetInboxItemDoneMovesAnItemOutOfTheDefaultListAndBack(t *testing.T) {
	r := newActivityRig(t)
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $1, $1 || '@example.com', $2)`, viewerCaller.ID, viewerCaller.Name)
	r.mentionOf(t, viewerCaller)
	r.mentionOf(t, viewerCaller)
	open, _ := r.inbox(t, viewerCaller, `{}`)
	if len(open.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(open.Items))
	}
	seq := open.Items[0].Seq

	if res := r.setDone(t, viewerCaller, `{"seq":`+strconv.FormatInt(seq, 10)+`}`); res.IsError {
		t.Fatalf("set done: %+v", res.Content)
	}
	after, _ := r.inbox(t, viewerCaller, `{}`)
	done, _ := r.inbox(t, viewerCaller, `{"filter":"done"}`)
	if len(after.Items) != 1 || after.Items[0].Seq == seq || len(done.Items) != 1 || done.Items[0].Seq != seq || after.UnreadCount != 1 {
		t.Fatalf("open %+v, done %+v", after.Items, done.Items)
	}

	if res := r.setDone(t, viewerCaller, `{"seq":`+strconv.FormatInt(seq, 10)+`,"done":false}`); res.IsError {
		t.Fatalf("bring back: %+v", res.Content)
	}
	if back, _ := r.inbox(t, viewerCaller, `{}`); len(back.Items) != 2 {
		t.Fatalf("items after bringing it back = %d, want 2", len(back.Items))
	}
}

func TestSetInboxItemDoneRefusesAKeyBadInputAndAnUnknownSeq(t *testing.T) {
	r := newActivityRig(t)
	cases := map[string]struct {
		by    auth.Principal
		input string
		code  string
	}{
		"a key":        {keyCaller, `{"seq":1}`, codeUserSessionRequired},
		"no seq":       {viewerCaller, `{}`, codeInvalidRequest},
		"seq 0":        {viewerCaller, `{"seq":0}`, codeInvalidRequest},
		"unknown seq":  {viewerCaller, `{"seq":99999999}`, codeNotFound},
		"extra fields": {viewerCaller, `{"seq":1,"x":1}`, codeInvalidRequest},
	}
	for name, c := range cases {
		body, _ := r.setDone(t, c.by, c.input).Content.(errorBody)
		if body.Code != c.code {
			t.Errorf("%s: code %q, want %q", name, body.Code, c.code)
		}
	}
}
