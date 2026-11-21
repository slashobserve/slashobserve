package revisions_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func startTurn(t *testing.T, svc *revisions.Service, by auth.Principal, req revisions.StartRequest) pgtype.UUID {
	t.Helper()
	row, err := svc.Start(context.Background(), by, req)
	if err != nil {
		t.Fatal(err)
	}
	if !row.ChatID.Valid {
		t.Fatalf("turn %v has no chat", row.ID)
	}
	return row.ID
}

func chatOfTurn(t *testing.T, d *dbtest.Database, id pgtype.UUID) string {
	t.Helper()
	var chat string
	if err := d.SQLDB(t).QueryRowContext(context.Background(), `SELECT coalesce(chat_id::text, '') FROM agent_requests WHERE id = $1`, id).Scan(&chat); err != nil {
		t.Fatal(err)
	}
	return chat
}

func text(s string) *string { return &s }

type chatEvent struct {
	action, targetKind, targetID, chatID, actorID, whole string
}

func chatEvents(t *testing.T, d *dbtest.Database) []chatEvent {
	t.Helper()
	rows, err := d.SQLDB(t).QueryContext(context.Background(), `SELECT action, coalesce(target_kind, ''), coalesce(target_id, ''), coalesce(chat_id::text, ''), coalesce(actor_id::text, ''), row_to_json(a)::text
		FROM audit_events a WHERE action LIKE 'chat.%' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []chatEvent
	for rows.Next() {
		var e chatEvent
		if err := rows.Scan(&e.action, &e.targetKind, &e.targetID, &e.chatID, &e.actorID, &e.whole); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAChatTurnJoinsTheNamedChatAndIsAuditedWithoutText(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	first := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("secret-word-one")})
	if got := chatOfTurn(t, d, first); got != first.String() {
		t.Fatalf("first turn chat = %s, want its own id %s", got, first)
	}
	second := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("secret-word-two"), ChatID: text(first.String())})
	if got := chatOfTurn(t, d, second); got != first.String() {
		t.Fatalf("second turn chat = %s, want %s", got, first)
	}

	events := chatEvents(t, d)
	var actions []string
	for _, e := range events {
		actions = append(actions, e.action)
		if e.targetKind != "chat" || e.targetID != first.String() || e.chatID != first.String() || e.actorID != userID {
			t.Errorf("%s = %s %s in chat %s by %s", e.action, e.targetKind, e.targetID, e.chatID, e.actorID)
		}
		if strings.Contains(e.whole, "secret-word") {
			t.Errorf("%s carries the prompt: %s", e.action, e.whole)
		}
	}
	if got := strings.Join(actions, ","); got != "chat.create,chat.turn,chat.turn" {
		t.Fatalf("chat events = %s, want one create and a turn per message", got)
	}
}

func TestAChatIDMustNameTheCallersLiveChatOnTheSameSubject(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := context.Background()
	dash := seedDashboard(t, d, 1)
	home := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("one")})
	onDash := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("two"), Kind: text(revisions.KindRevise), DashboardID: text(dash.String())})
	archived := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("three")})
	if err := svc.ArchiveChat(ctx, user, archived); err != nil {
		t.Fatal(err)
	}

	for name, c := range map[string]struct {
		by   auth.Principal
		req  revisions.StartRequest
		want error
	}{
		"someone else's chat":    {account, revisions.StartRequest{Prompt: text("x"), ChatID: text(home.String())}, revisions.ErrChatNotFound},
		"an Admin":               {bea, revisions.StartRequest{Prompt: text("x"), ChatID: text(home.String())}, revisions.ErrChatReadOnly},
		"an Admin on archived":   {bea, revisions.StartRequest{Prompt: text("x"), ChatID: text(archived.String())}, revisions.ErrChatNotFound},
		"an archived chat":       {user, revisions.StartRequest{Prompt: text("x"), ChatID: text(archived.String())}, revisions.ErrChatNotFound},
		"a chat that never was":  {user, revisions.StartRequest{Prompt: text("x"), ChatID: text("00000000-0000-4000-8000-000000000009")}, revisions.ErrChatNotFound},
		"a Home chat on a board": {user, revisions.StartRequest{Prompt: text("x"), ChatID: text(home.String()), Kind: text(revisions.KindRevise), DashboardID: text(dash.String())}, nil},
		"a board chat from Home": {user, revisions.StartRequest{Prompt: text("x"), ChatID: text(chatOfTurn(t, d, onDash))}, nil},
	} {
		_, err := svc.Start(ctx, c.by, c.req)
		var refusal *revisions.StartError
		switch {
		case c.want != nil && !errors.Is(err, c.want):
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		case c.want == nil && !errors.As(err, &refusal):
			t.Errorf("%s: err = %v, want a refused start", name, err)
		}
	}
}

func TestADashboardTurnWithoutAChatIDJoinsTheLatestLiveChatOnThatDashboard(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := context.Background()
	dash := seedDashboard(t, d, 1)
	on := func(by auth.Principal) string {
		return chatOfTurn(t, d, startTurn(t, svc, by, revisions.StartRequest{Prompt: text("p"), Kind: text(revisions.KindRevise), DashboardID: text(dash.String())}))
	}
	first := on(user)
	if again := on(user); again != first {
		t.Fatalf("second turn chat = %s, want %s", again, first)
	}
	if theirs := on(account); theirs == first {
		t.Fatalf("another person's turn joined %s", first)
	}
	if err := svc.ArchiveChat(ctx, user, mustUUID(t, first)); err != nil {
		t.Fatal(err)
	}
	if fresh := on(user); fresh == first {
		t.Fatalf("a turn after the chat was archived joined it")
	}
}

func TestANewChatOnADashboardNeverJoinsTheLatestOne(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	turn := func(newChat *bool) string {
		return chatOfTurn(t, d, startTurn(t, svc, user, revisions.StartRequest{Prompt: text("p"), Kind: text(revisions.KindRevise), DashboardID: text(dash.String()), NewChat: newChat}))
	}
	first := turn(nil)
	fresh := turn(ptr(true))
	if fresh == first {
		t.Fatalf("new_chat joined the latest chat %s", first)
	}
	if latest := turn(nil); latest != fresh {
		t.Fatalf("the next turn chat = %s, want the new chat %s", latest, fresh)
	}
	if again := turn(ptr(false)); again != fresh {
		t.Fatalf("new_chat false chat = %s, want the latest chat %s", again, fresh)
	}
}

func TestAResendWithNewChatReturnsTheOriginalRunInItsChat(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	const clientID = "99999999-9999-4999-8999-999999999999"
	req := func(newChat *bool) revisions.StartRequest {
		return revisions.StartRequest{Prompt: text("p"), Kind: text(revisions.KindRevise), DashboardID: text(dash.String()), ClientID: text(clientID), NewChat: newChat}
	}
	first, err := svc.Start(context.Background(), user, req(nil))
	if err != nil || first.Replayed {
		t.Fatalf("first send = %+v, %v", first, err)
	}
	again, err := svc.Start(context.Background(), user, req(ptr(true)))
	if err != nil || !again.Replayed || again.ID != first.ID || again.ChatID != first.ChatID {
		t.Fatalf("resend with new_chat = %+v, %v; want the first run %v replayed in its chat", again.AgentRequest, err, first.ID)
	}
}

func TestTurnsOfOneChatAreClaimedOneAtATimeInOrder(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	first := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("one")})
	second := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("two"), ChatID: text(first.String())})

	var wg sync.WaitGroup
	claimed := make([]pgtype.UUID, 2)
	found := make([]bool, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, ok, err := svc.ClaimRow(context.Background())
			if err != nil {
				t.Error(err)
			}
			claimed[i], found[i] = row.ID, ok
		}()
	}
	wg.Wait()
	if found[0] == found[1] {
		t.Fatalf("two racing claims found %v and %v, want exactly one turn", found[0], found[1])
	}
	winner := claimed[0]
	if found[1] {
		winner = claimed[1]
	}
	if winner != first {
		t.Fatalf("claimed %v, want the first turn %v", winner, first)
	}
	if _, ok := claimOne(t, svc); ok {
		t.Fatal("the second turn was claimed while the first was running")
	}
	answerWith(t, d, first, "done")
	if row, ok := claimOne(t, svc); !ok || row.ID != second {
		t.Fatalf("claimed %v (found %v) after the first turn ended, want %v", row.ID, ok, second)
	}
}

func TestAQueuedTurnWithACancelRequestDoesNotHoldBackTheNextTurn(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	first := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("one")})
	second := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("two"), ChatID: text(first.String())})
	d.Exec(t, `UPDATE agent_requests SET cancel_requested_at = clock_timestamp(), canceled_by_kind = 'user', canceled_by_id = $2::uuid, canceled_by_name = 'Ada' WHERE id = $1`, first, userID)
	if row, ok := claimOne(t, svc); !ok || row.ID != second {
		t.Fatalf("claimed %v (found %v), want the second turn %v", row.ID, ok, second)
	}
}

func TestAHomeBuildThatSavesADashboardMakesItTheChatsSubject(t *testing.T) {
	var d *dbtest.Database
	var dash pgtype.UUID
	svc, d := newService(t, reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
		d.Exec(t, `UPDATE dashboard_versions SET agent_request_id = $1::uuid WHERE dashboard_id = $2 AND version = 1`, req.ID, dash)
		return revisions.Result{Version: 1}, nil
	}))
	dash = seedDashboard(t, d, 1)
	turn := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("make one")})
	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitFor(t, "the chat to take the saved dashboard", func() bool {
		var subject string
		err := d.SQLDB(t).QueryRowContext(context.Background(), `SELECT coalesce(dashboard_id::text, '') FROM chats WHERE id = $1`, turn).Scan(&subject)
		return err == nil && subject == dash.String()
	})
	if _, err := svc.Start(context.Background(), user, revisions.StartRequest{Prompt: text("again"), ChatID: text(turn.String()), Kind: text(revisions.KindRevise), DashboardID: text(dash.String())}); err != nil {
		t.Fatalf("a turn on the saved dashboard in that chat: %v", err)
	}
}

func TestChatsAreListedAndArchivedByTheirOwnerAndReadByAdmins(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := context.Background()
	older := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs exist")})
	startTurn(t, svc, user, revisions.StartRequest{Prompt: text("and errors"), ChatID: text(older.String())})
	newer := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("cpu by pod")})
	startTurn(t, svc, account, revisions.StartRequest{Prompt: text("theirs")})

	page, next, err := svc.Chats(ctx, user, revisions.ChatFilter{}, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].ID != newer || next == "" {
		t.Fatalf("first page = %+v next %q, want the newer chat and a cursor", page, next)
	}
	rest, next, err := svc.Chats(ctx, user, revisions.ChatFilter{}, 5, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 1 || rest[0].ID != older || rest[0].Turns != 2 || rest[0].Title != "which logs exist" || rest[0].LastTurn == nil || rest[0].LastTurn.Prompt != "and errors" || next != "" {
		t.Fatalf("second page = %+v next %q", rest, next)
	}
	if found, _, err := svc.Chats(ctx, user, revisions.ChatFilter{Query: "errors"}, 5, ""); err != nil || len(found) != 1 || found[0].ID != older {
		t.Fatalf("search = %+v, %v", found, err)
	}
	if theirs, _, err := svc.Chats(ctx, bea, revisions.ChatFilter{}, 5, ""); err != nil || len(theirs) != 0 {
		t.Fatalf("an Admin lists %+v, %v; want none of other people's chats", theirs, err)
	}

	chat, turns, err := svc.Chat(ctx, user, older)
	if err != nil || chat.Turns != 2 || len(turns) != 2 || turns[0].ID != older {
		t.Fatalf("chat = %+v turns %d, %v", chat, len(turns), err)
	}
	if read, _, err := svc.Chat(ctx, bea, older); err != nil || !read.ReadOnly || read.Owner.Name != user.Name {
		t.Fatalf("an Admin reads %+v, %v; want it read only with its owner", read, err)
	}
	if everyone, _, err := svc.Chats(ctx, bea, revisions.ChatFilter{Everyone: true}, 5, ""); err != nil || len(everyone) != 3 || !everyone[0].ReadOnly {
		t.Fatalf("an Admin lists everyone's chats as %+v, %v", everyone, err)
	}
	if _, _, err := svc.Chats(ctx, user, revisions.ChatFilter{Everyone: true}, 5, ""); !errors.Is(err, revisions.ErrForbidden) {
		t.Fatalf("an Editor lists everyone's chats: %v", err)
	}
	if _, _, err := svc.Chat(ctx, account, older); !errors.Is(err, revisions.ErrChatNotFound) {
		t.Fatalf("%s reads the chat: %v", account.Name, err)
	}
	if err := svc.ArchiveChat(ctx, bea, older); !errors.Is(err, revisions.ErrChatReadOnly) {
		t.Fatalf("an Admin archives the chat: %v, want it refused as read only", err)
	}
	if err := svc.ArchiveChat(ctx, account, older); !errors.Is(err, revisions.ErrChatNotFound) {
		t.Fatalf("%s archives the chat: %v", account.Name, err)
	}
	if err := svc.ArchiveChat(ctx, bea, mustUUID(t, "00000000-0000-4000-8000-000000000009")); !errors.Is(err, revisions.ErrChatNotFound) {
		t.Fatalf("an Admin archives a chat that never was: %v", err)
	}

	renamed := "Log sources"
	pinned := true
	if _, err := svc.UpdateListFields(ctx, older, user, &renamed, false, &pinned); err != nil {
		t.Fatal(err)
	}
	if chat, _, err := svc.Chat(ctx, user, older); err != nil || chat.Title != renamed || chat.PinnedAt == nil {
		t.Fatalf("after a rename and pin through the run, chat = %+v, %v", chat, err)
	}

	if err := svc.ArchiveChat(ctx, user, older); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Chat(ctx, user, older); !errors.Is(err, revisions.ErrChatNotFound) {
		t.Fatalf("archived chat read: %v", err)
	}
	if left, _, err := svc.Chats(ctx, user, revisions.ChatFilter{}, 5, ""); err != nil || len(left) != 1 || left[0].ID != newer {
		t.Fatalf("after archive = %+v, %v", left, err)
	}
	var live int
	if err := d.SQLDB(t).QueryRowContext(ctx, `SELECT count(*) FROM agent_requests WHERE chat_id = $1 AND archived_at IS NULL`, older).Scan(&live); err != nil || live != 0 {
		t.Fatalf("%d turns of the archived chat are still listed, %v", live, err)
	}
	var archives []chatEvent
	for _, e := range chatEvents(t, d) {
		if e.action == "chat.archive" {
			archives = append(archives, e)
		}
	}
	if len(archives) != 1 || archives[0].targetID != older.String() || archives[0].actorID != userID || strings.Contains(archives[0].whole, "which logs") || strings.Contains(archives[0].whole, renamed) {
		t.Fatalf("chat.archive events = %+v", archives)
	}
}

func TestAKindTellsAnAskFromARevise(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := context.Background()
	dash := seedDashboard(t, d, 1)
	start := func(req revisions.StartRequest) revisions.Started {
		t.Helper()
		started, err := svc.Start(ctx, user, req)
		if err != nil {
			t.Fatal(err)
		}
		return started
	}

	asked := start(revisions.StartRequest{Prompt: text("why is checkout slow"), DashboardID: text(dash.String())})
	if asked.Intent != string(revisions.IntentAsk) || !revisions.PrivateAsk(asked.AgentRequest) || asked.BaseVersion.Valid || asked.DashboardID != dash {
		t.Errorf("no kind on a dashboard = %+v, want a private ask about it", asked.AgentRequest)
	}
	revised := start(revisions.StartRequest{Prompt: text("add a latency panel"), Kind: text(revisions.KindRevise), DashboardID: text(dash.String())})
	if revised.Intent != string(revisions.IntentApply) || !revised.BaseVersion.Valid || revised.DashboardID != dash {
		t.Errorf("kind revise = %+v, want an apply from the latest version", revised.AgentRequest)
	}
	for _, kind := range []*string{nil, text(revisions.KindAsk)} {
		home := start(revisions.StartRequest{Prompt: text("which logs exist"), Kind: kind})
		if home.Intent != string(revisions.IntentAsk) || home.DashboardID.Valid || home.ObservationID.Valid {
			t.Errorf("an ask about nothing = %+v, want a Home turn that may build", home.AgentRequest)
		}
	}
}

func chatIDs(chats []revisions.Chat) []pgtype.UUID {
	ids := make([]pgtype.UUID, len(chats))
	for i, c := range chats {
		ids[i] = c.ID
	}
	return ids
}

func pageThrough(t *testing.T, svc *revisions.Service, by auth.Principal, f revisions.ChatFilter) []pgtype.UUID {
	t.Helper()
	var ids []pgtype.UUID
	cursor := ""
	for range 10 {
		page, next, err := svc.Chats(context.Background(), by, f, 1, cursor)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, chatIDs(page)...)
		if next == "" {
			return ids
		}
		cursor = next
	}
	t.Fatalf("paging did not end: %v", ids)
	return nil
}

func TestYourPinnedChatsComeFirstOnEveryPageAndEveryonesAreByLastTurn(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	ctx := context.Background()
	oldest := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("oldest")})
	middle := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("middle")})
	newest := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("newest")})
	pinned := true
	if _, err := svc.UpdateListFields(ctx, oldest, user, nil, false, &pinned); err != nil {
		t.Fatal(err)
	}

	mine := pageThrough(t, svc, user, revisions.ChatFilter{})
	if want := []pgtype.UUID{oldest, newest, middle}; !slices.Equal(mine, want) {
		t.Fatalf("your chats page by page = %v, want the pinned one first, then by last turn %v", mine, want)
	}
	everyone := pageThrough(t, svc, bea, revisions.ChatFilter{Everyone: true})
	if want := []pgtype.UUID{newest, middle, oldest}; !slices.Equal(everyone, want) {
		t.Fatalf("everyone's chats page by page = %v, want them by last turn only %v", everyone, want)
	}
}

func TestAChatCarriesItsDashboardTitleOnlyForAReaderWhoSeesTheDashboard(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := context.Background()
	shared := seedDashboard(t, d, 1)
	d.Exec(t, `UPDATE dashboards SET title = 'Checkout health' WHERE id = $1`, shared)
	private := personalDashboardOf(t, d, user)
	d.Exec(t, `UPDATE dashboards SET title = 'My scratch board' WHERE id = $1`, private)
	onShared := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("on shared"), DashboardID: text(shared.String())})
	onPrivate := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("on private"), DashboardID: text(private.String())})
	home := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("about nothing")})

	titles := func(by auth.Principal, f revisions.ChatFilter) map[pgtype.UUID]string {
		t.Helper()
		chats, _, err := svc.Chats(ctx, by, f, 10, "")
		if err != nil {
			t.Fatal(err)
		}
		out := map[pgtype.UUID]string{}
		for _, c := range chats {
			out[c.ID] = "<none>"
			if c.DashboardTitle != nil {
				out[c.ID] = *c.DashboardTitle
			}
		}
		return out
	}
	ownerSees := titles(user, revisions.ChatFilter{})
	if ownerSees[chatOfTurnID(t, d, onShared)] != "Checkout health" || ownerSees[chatOfTurnID(t, d, onPrivate)] != "My scratch board" || ownerSees[home] != "<none>" {
		t.Fatalf("the owner reads titles %v", ownerSees)
	}
	adminSees := titles(bea, revisions.ChatFilter{Everyone: true})
	if adminSees[chatOfTurnID(t, d, onShared)] != "Checkout health" || adminSees[chatOfTurnID(t, d, onPrivate)] != "<none>" {
		t.Fatalf("an Admin reads titles %v, want none for a dashboard in someone's personal folder", adminSees)
	}
	if chat, _, err := svc.Chat(ctx, bea, chatOfTurnID(t, d, onPrivate)); err != nil || chat.DashboardTitle != nil {
		t.Fatalf("an Admin reads the private chat as %+v, %v; want no dashboard title", chat, err)
	}
	if chat, _, err := svc.Chat(ctx, user, chatOfTurnID(t, d, onShared)); err != nil || chat.DashboardTitle == nil || *chat.DashboardTitle != "Checkout health" {
		t.Fatalf("the owner reads the shared chat as %+v, %v", chat, err)
	}
}

func chatOfTurnID(t *testing.T, d *dbtest.Database, turn pgtype.UUID) pgtype.UUID {
	t.Helper()
	return mustUUID(t, chatOfTurn(t, d, turn))
}
