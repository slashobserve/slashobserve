package activity

import (
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

var (
	mia      = auth.Principal{Kind: auth.KindUser, ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Name: "Mia", Role: auth.RoleEditor}
	ola      = actor.Actor{Kind: auth.KindUser, ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Name: "Ola"}
	miaActor = actor.Actor{Kind: auth.KindUser, ID: mia.ID, Name: mia.Name}
)

func (r rig) inboxUser(t *testing.T, p auth.Principal) {
	t.Helper()
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $1, $1 || '@example.com', $2) ON CONFLICT DO NOTHING`, p.ID, p.Name)
}

func (r rig) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var out string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r rig) uuid(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r rig) openProposal(t *testing.T, by actor.Actor) (proposal, dashboard pgtype.UUID) {
	t.Helper()
	dashboard = r.dashboardIn(t, pgtype.UUID{})
	proposal = r.uuid(t, r.scalar(t, `INSERT INTO proposals (dashboard_id, title, author_kind, author_id, author_name) VALUES ($1::uuid, 'Add latency', 'user', $2::uuid, $3) RETURNING id::text`, dashboard.String(), by.ID, by.Name))
	r.append(t, Entry{Type: TypeProposalOpened, TargetID: proposal, Owner: OnDashboard(dashboard), Actor: by, Data: map[string]any{"title": "Add latency"}})
	return proposal, dashboard
}

func (r rig) closeProposal(t *testing.T, id pgtype.UUID) {
	t.Helper()
	r.db.Exec(t, `UPDATE proposals SET state = 'merged', closed_at = now(), closed_by_kind = 'user', closed_by_id = gen_random_uuid(), closed_by_name = 'x' WHERE id = $1::uuid`, id.String())
}

func (r rig) observation(t *testing.T, createdBy actor.Actor) pgtype.UUID {
	t.Helper()
	return r.uuid(t, r.scalar(t, `INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name) VALUES (general_folder_id(), 'Checkout latency', 'user', $1::uuid, $2) RETURNING id::text`, createdBy.ID, createdBy.Name))
}

var blockPositions = map[pgtype.UUID]int{}

func (r rig) addBlock(t *testing.T, observation pgtype.UUID, state string, by actor.Actor) pgtype.UUID {
	t.Helper()
	position := blockPositions[observation]
	blockPositions[observation]++
	block := r.uuid(t, r.scalar(t, `INSERT INTO observation_blocks (observation_id, kind, state, position, body, created_by_kind, created_by_id, created_by_name) VALUES ($1::uuid, 'text', $2, $3, '{}', 'user', $4::uuid, $5) RETURNING id::text`, observation.String(), state, position, by.ID, by.Name))
	r.append(t, Entry{Type: TypeObservationBlockAdded, TargetID: observation, Owner: OnObservation(observation), Actor: by, Data: map[string]any{"title": "Checkout latency", "block_id": block.String(), "block_kind": "text", "state": state}})
	return block
}

func (r rig) thread(t *testing.T, observation pgtype.UUID) pgtype.UUID {
	t.Helper()
	return r.uuid(t, r.scalar(t, `INSERT INTO threads (observation_id) VALUES ($1::uuid) RETURNING id::text`, observation.String()))
}

func (r rig) message(t *testing.T, thread pgtype.UUID, by actor.Actor) pgtype.UUID {
	t.Helper()
	return r.uuid(t, r.scalar(t, `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name) VALUES ($1::uuid, 'text', 'user', $2::uuid, $3) RETURNING id::text`, thread.String(), by.ID, by.Name))
}

func (r rig) reply(t *testing.T, observation, thread pgtype.UUID, by actor.Actor) int64 {
	t.Helper()
	message := r.message(t, thread, by)
	return r.append(t, Entry{Type: TypeThreadReplied, TargetID: thread, Owner: OnObservation(observation), Actor: by, Data: map[string]any{"object": map[string]any{"kind": "observation", "id": observation.String()}, "message_id": message.String()}})
}

func (r rig) mention(t *testing.T, observation, thread pgtype.UUID, by actor.Actor, mentioned string) int64 {
	t.Helper()
	message := r.message(t, thread, by)
	return r.append(t, Entry{Type: TypeThreadMentioned, TargetID: thread, Owner: OnObservation(observation), Actor: by, Data: map[string]any{
		"object": map[string]any{"kind": "observation", "id": observation.String()}, "message_id": message.String(),
		"mentioned": map[string]any{"kind": "user", "id": mentioned, "name": "x"},
	}})
}

func (r rig) run(t *testing.T, requester actor.Actor, state string) pgtype.UUID {
	t.Helper()
	failure := any(nil)
	if state == "failed" {
		failure = "internal"
	}
	return r.uuid(t, r.scalar(t, `INSERT INTO agent_requests (source, prompt, requested_by_kind, requested_by_id, requested_by_name, status, state, failure_code)
		VALUES ('observe', 'p', 'user', $1::uuid, $2, $3, $3, $4) RETURNING id::text`, requester.ID, requester.Name, state, failure))
}

func (r rig) failedRun(t *testing.T, requester actor.Actor) (run pgtype.UUID, seq int64) {
	t.Helper()
	run = r.run(t, requester, "failed")
	runActor := requester
	runActor.RunID = run.String()
	return run, r.append(t, Entry{Type: TypeRunFailed, TargetID: run, Owner: General(), Actor: runActor, Data: map[string]any{"source": "observe"}})
}

func (r rig) inbox(t *testing.T, by auth.Principal, view View) InboxPage {
	t.Helper()
	page, err := r.store.Inbox(t.Context(), by, view, "", 100)
	if err != nil {
		t.Fatalf("inbox %s: %v", view, err)
	}
	return page
}

func inboxSeqs(page InboxPage) []int64 {
	out := make([]int64, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, item.Seq)
	}
	return out
}

func inboxReasons(page InboxPage) []Reason {
	out := make([]Reason, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, item.Reason)
	}
	return out
}

func wantReasons(t *testing.T, what string, page InboxPage, want ...Reason) {
	t.Helper()
	if got := inboxReasons(page); !slices.Equal(got, want) {
		t.Fatalf("%s: reasons = %v, want %v", what, got, want)
	}
}

func TestInboxListsAProposalOthersOpenedOnAFolderTheCallerEditsUntilItIsClosed(t *testing.T) {
	r := newRig(t)
	proposal, _ := r.openProposal(t, ola)
	r.openProposal(t, miaActor)

	page := r.inbox(t, mia, ViewForYou)
	wantReasons(t, "an open proposal by Ola", page, ReasonProposal)
	if !page.Items[0].Unread || page.UnreadCount != 1 {
		t.Fatalf("item unread = %v, count = %d, want unread and 1", page.Items[0].Unread, page.UnreadCount)
	}
	wantReasons(t, "a Viewer who cannot edit the folder", r.inbox(t, reader, ViewForYou))
	wantReasons(t, "an Admin", r.inbox(t, admin, ViewForYou), ReasonProposal, ReasonProposal)

	r.closeProposal(t, proposal)
	wantReasons(t, "after the proposal merged", r.inbox(t, mia, ViewForYou))
}

func TestInboxListsARequestedBlockOnTheCallersObservationUntilItIsKept(t *testing.T) {
	r := newRig(t)
	mine, theirs := r.observation(t, miaActor), r.observation(t, ola)
	block := r.addBlock(t, mine, "requested", ola)
	r.addBlock(t, theirs, "requested", ola)
	r.addBlock(t, mine, "live", ola)
	r.addBlock(t, mine, "requested", miaActor)

	wantReasons(t, "a block Ola requested on Mia's observation", r.inbox(t, mia, ViewForYou), ReasonRequestedBlock)

	r.db.Exec(t, `UPDATE observation_blocks SET state = 'live' WHERE id = $1::uuid`, block.String())
	wantReasons(t, "after the block was kept", r.inbox(t, mia, ViewForYou))
}

func TestInboxListsAMentionOfTheCallerAndNoOtherMention(t *testing.T) {
	r := newRig(t)
	o := r.observation(t, ola)
	thread := r.thread(t, o)
	r.mention(t, o, thread, ola, mia.ID)
	r.mention(t, o, thread, ola, ola.ID)
	r.mention(t, o, thread, miaActor, mia.ID)

	wantReasons(t, "mentions", r.inbox(t, mia, ViewForYou), ReasonMention)
}

func TestInboxListsRepliesInThreadsTheCallerWroteInAndRunAnswersForTheirRuns(t *testing.T) {
	r := newRig(t)
	o := r.observation(t, ola)
	joined, strangers := r.thread(t, o), r.thread(t, r.observation(t, ola))
	r.message(t, joined, miaActor)
	theirs := r.reply(t, o, joined, ola)
	r.reply(t, o, joined, miaActor)
	r.reply(t, o, strangers, ola)

	page := r.inbox(t, mia, ViewForYou)
	wantReasons(t, "replies", page, ReasonReply)
	if page.Items[0].Seq != theirs {
		t.Fatalf("listed seq %d, want Ola's reply %d", page.Items[0].Seq, theirs)
	}

	run := r.run(t, miaActor, "succeeded")
	answer := miaActor
	answer.RunID = run.String()
	message := r.message(t, strangers, answer)
	answered := r.append(t, Entry{Type: TypeThreadReplied, TargetID: strangers, Owner: OnObservation(o), Actor: answer, Data: map[string]any{"message_id": message.String()}})
	page = r.inbox(t, mia, ViewForYou)
	wantReasons(t, "with a run answer for Mia's run", page, ReasonReply, ReasonReply)
	if page.Items[0].Seq != answered {
		t.Fatalf("newest seq %d, want the run answer %d", page.Items[0].Seq, answered)
	}

	elsewhere := r.run(t, ola, "succeeded")
	foreign := ola
	foreign.RunID = elsewhere.String()
	unrelated := r.thread(t, r.observation(t, ola))
	message = r.message(t, unrelated, foreign)
	r.append(t, Entry{Type: TypeThreadReplied, TargetID: unrelated, Owner: OnObservation(o), Actor: foreign, Data: map[string]any{"message_id": message.String()}})
	wantReasons(t, "with a run answer for Ola's run", r.inbox(t, mia, ViewForYou), ReasonReply, ReasonReply)
}

func TestInboxListsAFailedRunUntilItIsContinued(t *testing.T) {
	r := newRig(t)
	run, _ := r.failedRun(t, miaActor)
	r.failedRun(t, ola)

	wantReasons(t, "a failed run", r.inbox(t, mia, ViewForYou), ReasonRunFailed)

	r.db.Exec(t, `INSERT INTO agent_requests (source, prompt, requested_by_kind, requested_by_id, requested_by_name, continues_id) VALUES ('observe', 'again', 'user', $1::uuid, 'Mia', $2::uuid)`, mia.ID, run.String())
	wantReasons(t, "after a continuation", r.inbox(t, mia, ViewForYou))
}

func TestInboxLeavesOutItemsOlderThanThirtyDays(t *testing.T) {
	r := newRig(t)
	o := r.observation(t, ola)
	thread := r.thread(t, o)
	old := r.mention(t, o, thread, ola, mia.ID)
	recent := r.mention(t, o, thread, ola, mia.ID)
	r.db.Exec(t, `UPDATE activity SET created_at = now() - interval '31 days' WHERE seq = $1`, old)

	if got := inboxSeqs(r.inbox(t, mia, ViewForYou)); !slices.Equal(got, []int64{recent}) {
		t.Fatalf("seqs = %v, want only %d", got, recent)
	}
	if got := inboxSeqs(r.inbox(t, mia, ViewEverything)); slices.Contains(got, old) {
		t.Fatalf("everything lists the 31 day old item %d: %v", old, got)
	}
}

func TestInboxViewsFilterByReason(t *testing.T) {
	r := newRig(t)
	r.openProposal(t, ola)
	o := r.observation(t, ola)
	thread := r.thread(t, o)
	r.message(t, thread, miaActor)
	mention := r.mention(t, o, thread, ola, mia.ID)
	reply := r.reply(t, o, thread, ola)
	r.failedRun(t, miaActor)

	wantReasons(t, "for_you", r.inbox(t, mia, ViewForYou), ReasonRunFailed, ReasonReply, ReasonMention, ReasonProposal)
	wantReasons(t, "proposals", r.inbox(t, mia, ViewProposals), ReasonProposal)
	mentions := r.inbox(t, mia, ViewMentions)
	wantReasons(t, "mentions", mentions, ReasonReply, ReasonMention)
	if got := inboxSeqs(mentions); !slices.Equal(got, []int64{reply, mention}) {
		t.Fatalf("mention seqs = %v, want [%d %d]", got, reply, mention)
	}
}

func TestInboxEverythingHoldsOthersActivityWithAReasonWhereOneApplies(t *testing.T) {
	r := newRig(t)
	o := r.observation(t, ola)
	thread := r.thread(t, o)
	mention := r.mention(t, o, thread, ola, mia.ID)
	plainEvent := r.append(t, Entry{Type: TypeObservationUpdated, TargetID: o, Owner: OnObservation(o), Actor: ola, Data: map[string]any{"title": "x"}})
	own := r.append(t, Entry{Type: TypeObservationUpdated, TargetID: o, Owner: OnObservation(o), Actor: miaActor, Data: map[string]any{"title": "y"}})
	_, failed := r.failedRun(t, miaActor)

	page := r.inbox(t, mia, ViewEverything)
	if got := inboxSeqs(page); !slices.Equal(got, []int64{failed, plainEvent, mention}) {
		t.Fatalf("everything seqs = %v, want [%d %d %d] and not Mia's own %d", got, failed, plainEvent, mention, own)
	}
	wantReasons(t, "everything", page, ReasonRunFailed, "", ReasonMention)
}

func TestInboxPagesShowEachItemOnceWhileAnotherTransactionHoldsAnAppend(t *testing.T) {
	r := newRig(t)
	o := r.observation(t, ola)
	thread := r.thread(t, o)
	var want []int64
	for range 5 {
		want = append([]int64{r.mention(t, o, thread, ola, mia.ID)}, want...)
	}

	held, err := r.db.SQLDB(t).BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Rollback() }()
	if _, err := held.ExecContext(t.Context(), `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, data)
		VALUES ('thread.mentioned', 'thread', $1::uuid, 'user', $2::uuid, 'Ola', jsonb_build_object('mentioned', jsonb_build_object('kind', 'user', 'id', $3::text)))`, thread.String(), ola.ID, mia.ID); err != nil {
		t.Fatal(err)
	}
	r.mention(t, o, thread, ola, mia.ID)

	var got []int64
	cursor := ""
	for range 10 {
		page, err := r.store.Inbox(t.Context(), mia, ViewForYou, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, inboxSeqs(page)...)
		if page.Next == "" {
			break
		}
		cursor = page.Next
	}
	if !slices.Equal(got, want) {
		t.Fatalf("paged seqs = %v, want the five committed before the held append, once each: %v", got, want)
	}
}

func TestInboxUnreadCountStopsAtOneHundred(t *testing.T) {
	r := newRig(t)
	o := r.observation(t, ola)
	thread := r.thread(t, o)
	for range 101 {
		r.append(t, Entry{Type: TypeThreadMentioned, TargetID: thread, Owner: OnObservation(o), Actor: ola, Data: map[string]any{"mentioned": map[string]any{"kind": "user", "id": mia.ID, "name": "Mia"}}})
	}

	page, err := r.store.Inbox(t.Context(), mia, ViewForYou, "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if page.UnreadCount != UnreadCap || len(page.Items) != 3 {
		t.Fatalf("unread_count = %d with %d items, want %d and 3", page.UnreadCount, len(page.Items), UnreadCap)
	}
}

func TestInboxMarkReadMovesForwardOnlyAndRefusesUnknownSeqs(t *testing.T) {
	r := newRig(t)
	r.inboxUser(t, mia)
	o := r.observation(t, ola)
	thread := r.thread(t, o)
	first := r.mention(t, o, thread, ola, mia.ID)
	second := r.mention(t, o, thread, ola, mia.ID)
	third := r.mention(t, o, thread, ola, mia.ID)

	unread := func() []bool {
		var out []bool
		for _, item := range r.inbox(t, mia, ViewForYou).Items {
			out = append(out, item.Unread)
		}
		return out
	}
	if got := unread(); !slices.Equal(got, []bool{true, true, true}) {
		t.Fatalf("unread before reading = %v, want all", got)
	}
	if err := r.store.MarkRead(t.Context(), mia, second); err != nil {
		t.Fatal(err)
	}
	if got := unread(); !slices.Equal(got, []bool{true, false, false}) {
		t.Fatalf("unread after reading the second = %v, want only the third", got)
	}
	if page := r.inbox(t, mia, ViewForYou); page.UnreadCount != 1 {
		t.Fatalf("unread_count = %d, want 1", page.UnreadCount)
	}

	if err := r.store.MarkRead(t.Context(), mia, first); err != nil {
		t.Fatal(err)
	}
	if got := unread(); !slices.Equal(got, []bool{true, false, false}) {
		t.Fatalf("unread after an older seq = %v, want the cursor unmoved", got)
	}

	if err := r.store.MarkRead(t.Context(), mia, third); err != nil {
		t.Fatal(err)
	}
	if page := r.inbox(t, mia, ViewForYou); page.UnreadCount != 0 || page.Items[0].Unread {
		t.Fatalf("after reading the third: count %d, newest unread %v, want none", page.UnreadCount, page.Items[0].Unread)
	}

	if err := r.store.MarkRead(t.Context(), mia, third+1000); !errors.Is(err, ErrUnknownSeq) {
		t.Fatalf("mark read of an unknown seq = %v, want ErrUnknownSeq", err)
	}
}

func TestInboxBelongsToPeopleOnly(t *testing.T) {
	r := newRig(t)
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: "99999999-9999-4999-8999-999999999999", Name: "ci", Role: auth.RoleAdmin}
	if _, err := r.store.Inbox(t.Context(), key, ViewForYou, "", 10); !errors.Is(err, ErrPersonOnly) {
		t.Fatalf("inbox of a key = %v, want ErrPersonOnly", err)
	}
	if err := r.store.MarkRead(t.Context(), key, 1); !errors.Is(err, ErrPersonOnly) {
		t.Fatalf("mark read by a key = %v, want ErrPersonOnly", err)
	}
}

func TestInboxHidesWhatTheCallerCannotSee(t *testing.T) {
	r := newRig(t)
	secret := foldertest.Restricted(t, r.db, "Secret", "secret-team")
	dashboard := r.dashboardIn(t, secret)
	proposal := r.uuid(t, r.scalar(t, `INSERT INTO proposals (dashboard_id, title, author_kind, author_id, author_name) VALUES ($1::uuid, 'x', 'user', $2::uuid, 'Ola') RETURNING id::text`, dashboard.String(), ola.ID))
	r.append(t, Entry{Type: TypeProposalOpened, TargetID: proposal, Owner: OnDashboard(dashboard), Actor: ola})

	wantReasons(t, "a restricted folder the Editor holds no grant on", r.inbox(t, mia, ViewForYou))
	wantReasons(t, "everything", r.inbox(t, mia, ViewEverything))
}
