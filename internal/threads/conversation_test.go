package threads

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/auth"
)

const (
	caseGrantID   = "66666666-6666-4666-8666-666666666666"
	otherGrantID  = "88888888-8888-4888-8888-888888888888"
	caseAccountID = "77777777-7777-4777-8777-777777777777"
)

func (r *rig) seedRecipientAgents(t *testing.T) {
	t.Helper()
	for _, person := range []auth.Principal{testUser, commenter} {
		r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'issuer', $2, $2, $3) ON CONFLICT (id) DO NOTHING`, person.ID, person.ID+"@example.com", person.Name)
	}
	r.db.Exec(t, `WITH c AS (INSERT INTO oauth_clients (name, redirect_uris) VALUES ('Codex', ARRAY['http://127.0.0.1:1/cb']) RETURNING id)
		INSERT INTO oauth_grants (id, client_id, user_id, resource, expires_at)
		SELECT $1::uuid, id, $2::uuid, 'http://localhost:8080/mcp', now() + interval '1 day' FROM c
		UNION ALL SELECT $3::uuid, id, $4::uuid, 'http://localhost:8080/mcp', now() + interval '1 day' FROM c`, caseGrantID, testUserID, otherGrantID, commenterID)
	r.db.Exec(t, `INSERT INTO service_accounts (id, name, takes_runs) VALUES ($1::uuid, 'pager', true)`, caseAccountID)
}

func (r *rig) setThreadAgent(t *testing.T, threadID string, agent *recipientAgent, off bool) {
	t.Helper()
	var kind, id any
	if agent != nil {
		kind = agent.Kind
		if agent.ID != nil {
			id = *agent.ID
		}
	}
	r.db.Exec(t, `UPDATE threads SET agent_kind = $2, agent_id = $3::uuid, agent_off = $4 WHERE id = $1::uuid`, threadID, kind, id, off)
}

type storedAgent struct {
	label string
	off   bool
}

func (r *rig) threadAgentOf(t *testing.T, threadID string) storedAgent {
	t.Helper()
	var label string
	var off bool
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT CASE WHEN agent_kind IS NULL THEN 'none' WHEN agent_id IS NULL THEN agent_kind ELSE agent_kind || ':' || agent_id::text END, agent_off FROM threads WHERE id = $1::uuid`, threadID).Scan(&label, &off); err != nil {
		t.Fatal(err)
	}
	return storedAgent{label: label, off: off}
}

func caseLabel(a *recipientAgent) string {
	if a == nil {
		return "none"
	}
	if a.ID == nil {
		return a.Kind
	}
	return a.Kind + ":" + *a.ID
}

func expectedThreadAgent(c recipientCase) storedAgent {
	before := storedAgent{label: caseLabel(c.Thread.Agent), off: c.Thread.Off}
	switch c.Want.Sticky {
	case "off":
		if before.label == "none" {
			return before
		}
		return storedAgent{label: before.label, off: true}
	case "follow":
		return storedAgent{label: caseLabel(c.Want.Agent)}
	}
	return before
}

func checkSentAgainstCase(t *testing.T, r *rig, c recipientCase, sent Sent, err error) {
	t.Helper()
	if c.Want.Error != nil {
		var invalid *InvalidInputError
		if !errors.As(err, &invalid) || invalid.Code != *c.Want.Error {
			t.Fatalf("error = %v, want code %s", err, *c.Want.Error)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	runs := map[string]string{}
	for _, q := range r.queuedRuns(t) {
		if q.ThreadMessageID == sent.Message.ID {
			label := q.AssignedKind
			if q.AssignedID != "" {
				label += ":" + q.AssignedID
			}
			runs[q.ID] = label
		}
	}
	switch {
	case c.Want.Agent != nil:
		if sent.Run == nil || sent.Refusal != nil || runs[sent.Run.ID] != caseLabel(c.Want.Agent) || sent.To != ToAgent {
			t.Fatalf("run = %+v, refusal = %+v, queued = %v, to = %q; want a run for %s", sent.Run, sent.Refusal, runs, sent.To, caseLabel(c.Want.Agent))
		}
	case c.Want.Unavailable:
		if sent.Run != nil || sent.Refusal == nil || sent.Refusal.Code != RefusalAgentUnavailable || len(runs) != 0 {
			t.Fatalf("run = %+v, refusal = %+v, queued = %v; want %s", sent.Run, sent.Refusal, runs, RefusalAgentUnavailable)
		}
	default:
		if sent.Run != nil || sent.Refusal != nil || len(runs) != 0 || sent.To != ToEveryone {
			t.Fatalf("run = %+v, refusal = %+v, queued = %v, to = %q; want a plain comment", sent.Run, sent.Refusal, runs, sent.To)
		}
	}
	if got, want := r.threadAgentOf(t, sent.Thread.ID), expectedThreadAgent(c); got != want {
		t.Errorf("thread agent = %+v, want %+v", got, want)
	}
}

func TestTheRecipientCasesHoldThroughOpenAndReply(t *testing.T) {
	rigs := map[bool]*rig{true: newAgentRig(t, true), false: newAgentRig(t, false)}
	for _, r := range rigs {
		r.seedRecipientAgents(t)
	}
	cases := readRecipientCases(t)
	ran := 0
	for _, c := range cases {
		if strings.TrimSpace(c.Body) == "" {
			continue
		}
		ran++
		r := rigs[c.BuiltinAgent]
		t.Run("reply/"+c.Name, func(t *testing.T) {
			dash := r.createDashboard(t)
			thread, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "first"})
			r.setThreadAgent(t, thread.ID, c.Thread.Agent, c.Thread.Off)
			sent, err := r.asks.Reply(t.Context(), testUser, mustParse(t, thread.ID), ReplyInput{Body: c.Body}, c.To)
			if err != nil && c.Want.Error != nil {
				if got := r.threadAgentOf(t, thread.ID); got != (storedAgent{label: caseLabel(c.Thread.Agent), off: c.Thread.Off}) {
					t.Errorf("a refused message changed the thread agent to %+v", got)
				}
			}
			checkSentAgainstCase(t, r, c, sent, err)
		})
		if c.Thread.Agent != nil || c.Thread.Off {
			continue
		}
		t.Run("open/"+c.Name, func(t *testing.T) {
			dash := r.createDashboard(t)
			before := r.messageCount(t)
			sent, err := r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindDashboard, dash), Body: c.Body}, c.To)
			if c.Want.Error != nil && r.messageCount(t) != before {
				t.Errorf("a refused open stored a message")
			}
			checkSentAgainstCase(t, r, c, sent, err)
		})
	}
	if ran < 40 {
		t.Fatalf("ran %d of %d cases through the service, want at least 40", ran, len(cases))
	}
}

func TestAClientIDResendReturnsTheStoredMessageAndStartsNothingNew(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	clientID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	in := OpenInput{Target: target(KindDashboard, dash), Body: "@observe why?", ClientID: clientID}
	first, err := r.asks.Open(t.Context(), testUser, in, ToAuto)
	if err != nil || first.Run == nil || first.Replayed {
		t.Fatalf("first = %+v, %v", first, err)
	}
	again, err := r.asks.Open(t.Context(), testUser, in, ToAuto)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Replayed || again.Message.ID != first.Message.ID || again.Thread.ID != first.Thread.ID || again.Run == nil || again.Run.ID != first.Run.ID || again.To != ToAgent {
		t.Fatalf("resend = %+v, want the first message %s and run %s replayed", again, first.Message.ID, first.Run.ID)
	}
	if n, runs := r.messageCount(t), len(r.queuedRuns(t)); n != 1 || runs != 1 {
		t.Fatalf("%d messages and %d runs after a resend, want 1 and 1", n, runs)
	}

	reply, err := r.asks.Reply(t.Context(), testUser, mustParse(t, first.Thread.ID), ReplyInput{Body: "thanks", ClientID: clientID}, ToEveryone)
	if err != nil || !reply.Replayed || reply.Message.ID != first.Message.ID {
		t.Fatalf("a reply that reuses the client_id on the same thread = %+v, %v", reply, err)
	}

	other := r.createDashboard(t)
	_, err = r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindDashboard, other), Body: "elsewhere", ClientID: clientID}, ToEveryone)
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeClientIDReused {
		t.Fatalf("a client_id reused on another target = %v, want %s", err, CodeClientIDReused)
	}

	theirs, err := r.asks.Open(t.Context(), commenter, OpenInput{Target: target(KindDashboard, dash), Body: "mine", ClientID: clientID}, ToEveryone)
	if err != nil || theirs.Replayed {
		t.Fatalf("another author's message with the same client_id = %+v, %v", theirs, err)
	}
}

func TestTwoRepliesWithOneClientIDStoreOneMessageAndOneRun(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	thread, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "first"})
	clientID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	results := make([]Sent, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			results[i], errs[i] = r.asks.Reply(t.Context(), testUser, mustParse(t, thread.ID), ReplyInput{Body: "@observe look", ClientID: clientID}, ToAuto)
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0].Message.ID != results[1].Message.ID || results[0].Replayed == results[1].Replayed {
		t.Fatalf("results = %+v and %+v, want one stored and one replayed", results[0], results[1])
	}
	if n, runs := r.messageCount(t), len(r.queuedRuns(t)); n != 2 || runs != 1 {
		t.Fatalf("%d messages and %d runs, want 2 and 1", n, runs)
	}
	for _, sent := range results {
		if sent.Run == nil {
			t.Errorf("%+v has no run, want the one run on both answers", sent)
		}
	}
}

func TestTwoOpensOnOneObservationPageShareOneThread(t *testing.T) {
	r := newRig(t)
	observation := r.createObservation(t, testUser, pgtype.UUID{})
	results := make([]Sent, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			results[i], errs[i] = r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindObservation, observation), Body: "a note"}, ToEveryone)
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0].Thread.ID != results[1].Thread.ID || results[0].Message.ID == results[1].Message.ID {
		t.Fatalf("threads %s and %s, messages %s and %s; want one thread with two messages", results[0].Thread.ID, results[1].Thread.ID, results[0].Message.ID, results[1].Message.ID)
	}
}

func TestAnAnchoredObservationThreadIsItsOwnThreadAndItsRunCarriesThePoint(t *testing.T) {
	r := newAgentRig(t, true)
	observation := r.createObservation(t, testUser, pgtype.UUID{})
	block := r.addBlock(t, observation)
	page, err := r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindObservation, observation), Body: "the page"}, ToEveryone)
	if err != nil {
		t.Fatal(err)
	}
	spot := &anchor.Anchor{Kind: anchor.Position, Pos: anchor.Pos{X: 0.25, Y: 0.5}}
	pointed, err := r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindObservation, observation), Body: "@observe why here?", Anchor: spot, BlockID: &block}, ToAuto)
	if err != nil {
		t.Fatal(err)
	}
	if pointed.Thread.ID == page.Thread.ID || pointed.Thread.Anchor == nil || pointed.Thread.BlockID != block || pointed.Run == nil {
		t.Fatalf("anchored open = %+v, want a new thread on block %s with a run", pointed, block)
	}
	var kind, runBlock string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT anchor->>'kind', block_id::text FROM agent_requests WHERE id = $1::uuid`, pointed.Run.ID).Scan(&kind, &runBlock); err != nil {
		t.Fatal(err)
	}
	if kind != string(anchor.Position) || runBlock != block {
		t.Fatalf("run point = %q on %q, want %q on %q", kind, runBlock, anchor.Position, block)
	}

	_, err = r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindAnnotation, r.createAnnotation(t, testUser, r.createDashboard(t))), Body: "x", Anchor: spot}, ToEveryone)
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeInvalidTarget {
		t.Fatalf("an anchor on an annotation = %v, want %s", err, CodeInvalidTarget)
	}
	_, err = r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindObservation, observation), Body: "x", Anchor: &anchor.Anchor{Kind: anchor.Range, Pos: anchor.Pos{X: 0.5, Y: 0.5}}}, ToEveryone)
	if !errors.As(err, &invalid) || invalid.Code != CodeInvalidAnchor {
		t.Fatalf("an anchor missing its fields = %v, want %s", err, CodeInvalidAnchor)
	}
}

func TestObservationAsksAreNotCappedPerPerson(t *testing.T) {
	r := newAgentRig(t, true)
	for i := range 5 {
		observation := r.createObservation(t, testUser, pgtype.UUID{})
		sent, err := r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindObservation, observation), Body: "@observe why?"}, ToAuto)
		if err != nil || sent.Run == nil || sent.Refusal != nil {
			t.Fatalf("ask %d = %+v, %v; want a run", i+1, sent, err)
		}
	}
}

func TestARevokedStickyGrantIsClearedAndTheMessageIsKept(t *testing.T) {
	r := newAgentRig(t, true)
	grant := r.connectedAgent(t, testUser)
	dash := r.createDashboard(t)
	first, err := r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindDashboard, dash), Body: grantToken(grant) + " look"}, ToAuto)
	if err != nil || first.Run == nil {
		t.Fatalf("first = %+v, %v", first, err)
	}
	r.db.Exec(t, `UPDATE oauth_grants SET revoked_at = now(), revoked_reason = 'person' WHERE id = $1::uuid`, grant.String())
	sent, err := r.asks.Reply(t.Context(), testUser, mustParse(t, first.Thread.ID), ReplyInput{Body: "and now?"}, ToAuto)
	if err != nil {
		t.Fatal(err)
	}
	if sent.Run != nil || sent.Refusal == nil || sent.Refusal.Code != RefusalAgentUnavailable || sent.Message.ID == "" {
		t.Fatalf("reply = %+v, want the message kept and %s", sent, RefusalAgentUnavailable)
	}
	if got := r.threadAgentOf(t, first.Thread.ID); got.label != "none" || got.off {
		t.Fatalf("thread agent = %+v, want it cleared", got)
	}
	if sent.Thread.Agent != nil {
		t.Errorf("the answer shows agent %+v, want none", sent.Thread.Agent)
	}
}

func TestSomeoneElsesStickyGrantStaysAndTheirMessageStartsNothing(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	grant := r.connectedAgent(t, testUser)
	first, err := r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindDashboard, dash), Body: grantToken(grant) + " look"}, ToAuto)
	if err != nil || first.Run == nil {
		t.Fatalf("first = %+v, %v", first, err)
	}
	sent, err := r.asks.Reply(t.Context(), commenter, mustParse(t, first.Thread.ID), ReplyInput{Body: "and this?"}, ToAuto)
	if err != nil {
		t.Fatal(err)
	}
	if sent.Run != nil || sent.Refusal == nil || sent.Refusal.Code != RefusalNotYourAgent {
		t.Fatalf("reply = %+v, want %s", sent, RefusalNotYourAgent)
	}
	if got := r.threadAgentOf(t, first.Thread.ID); got.label != "grant:"+grant.String() || got.off {
		t.Fatalf("thread agent = %+v, want the grant kept", got)
	}
	if len(r.queuedRuns(t)) != 1 {
		t.Fatalf("queued %d runs, want only the first", len(r.queuedRuns(t)))
	}
}

func TestToEveryoneTurnsTheThreadAgentOffAndAMentionTurnsItBackOn(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	first, err := r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindDashboard, dash), Body: "@observe why?"}, ToAuto)
	if err != nil || first.Run == nil || first.Thread.Agent == nil || first.Thread.AgentOff {
		t.Fatalf("first = %+v, %v", first, err)
	}
	quiet, err := r.asks.Reply(t.Context(), testUser, mustParse(t, first.Thread.ID), ReplyInput{Body: "noting it"}, ToEveryone)
	if err != nil || quiet.Run != nil || quiet.To != ToEveryone || !quiet.Thread.AgentOff {
		t.Fatalf("to everyone = %+v, %v; want a plain comment and the agent off", quiet, err)
	}
	still, err := r.asks.Reply(t.Context(), testUser, mustParse(t, first.Thread.ID), ReplyInput{Body: "another note"}, ToAuto)
	if err != nil || still.Run != nil || !still.Thread.AgentOff {
		t.Fatalf("a plain reply after off = %+v, %v; want no run", still, err)
	}
	back, err := r.asks.Reply(t.Context(), testUser, mustParse(t, first.Thread.ID), ReplyInput{Body: "why again?"}, ToAgent)
	if err != nil || back.Run == nil || back.Thread.AgentOff {
		t.Fatalf("to agent = %+v, %v; want a run and the agent on", back, err)
	}
}

func TestSettingTheThreadAgentIsAuditedWithoutText(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	first, err := r.asks.Open(t.Context(), testUser, OpenInput{Target: target(KindDashboard, dash), Body: "@observe secret-word-one"}, ToAuto)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.asks.Reply(t.Context(), testUser, mustParse(t, first.Thread.ID), ReplyInput{Body: "secret-word-two"}, ToEveryone); err != nil {
		t.Fatal(err)
	}
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT target_kind, target_id, coalesce(dashboard_id::text, ''), coalesce(thread_id::text, ''), actor_id::text, data::text, row_to_json(a)::text
		FROM audit_events a WHERE action = 'thread.agent_set' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		n++
		var kind, id, dashboard, thread, actorID, data, whole string
		if err := rows.Scan(&kind, &id, &dashboard, &thread, &actorID, &data, &whole); err != nil {
			t.Fatal(err)
		}
		if kind != "thread" || id != first.Thread.ID || thread != first.Thread.ID || dashboard != dash.String() || actorID != testUserID {
			t.Errorf("event %d = %s %s on dashboard %s, thread %s, by %s", n, kind, id, dashboard, thread, actorID)
		}
		if strings.Contains(whole, "secret-word") {
			t.Errorf("event %d carries message text: %s", n, whole)
		}
		if data != "{}" {
			t.Errorf("event %d data = %s, want none", n, data)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("%d thread.agent_set events, want 2 (set, then off)", n)
	}
}
