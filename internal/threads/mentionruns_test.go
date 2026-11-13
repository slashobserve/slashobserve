package threads

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type queuedRun struct {
	ID              string
	Source          string
	Trigger         string
	AssignedKind    string
	AssignedID      string
	CommentID       string
	ObservationID   string
	DashboardID     string
	BaseVersion     string
	ThreadMessageID string
	ThreadContext   sql.NullString
	Prompt          string
	RequestedByID   string
}

func (r *rig) queuedRuns(t *testing.T) []queuedRun {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT id::text, source, trigger, assigned_kind, coalesce(assigned_id::text, ''), coalesce(comment_id::text, ''),
		coalesce(observation_id::text, ''), coalesce(dashboard_id::text, ''), coalesce(base_version::text, ''), coalesce(thread_message_id::text, ''),
		thread_context, prompt, requested_by_id::text FROM agent_requests ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []queuedRun
	for rows.Next() {
		var q queuedRun
		if err := rows.Scan(&q.ID, &q.Source, &q.Trigger, &q.AssignedKind, &q.AssignedID, &q.CommentID, &q.ObservationID, &q.DashboardID, &q.BaseVersion, &q.ThreadMessageID, &q.ThreadContext, &q.Prompt, &q.RequestedByID); err != nil {
			t.Fatal(err)
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rig) messageCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM thread_messages`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) connectedAgent(t *testing.T, person auth.Principal) pgtype.UUID {
	t.Helper()
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'issuer', $2, $2, $3) ON CONFLICT (id) DO NOTHING`, person.ID, person.ID+"@example.com", person.Name)
	var id string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `WITH c AS (INSERT INTO oauth_clients (name, redirect_uris) VALUES ('Codex', ARRAY['http://127.0.0.1:1/cb']) RETURNING id)
		INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) SELECT id, $1::uuid, 'http://localhost:8080/mcp', now() + interval '1 day' FROM c RETURNING id::text`, person.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return mustParse(t, id)
}

func (r *rig) serviceAccount(t *testing.T, name string, takesRuns bool) pgtype.UUID {
	t.Helper()
	var id string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO service_accounts (name, takes_runs) VALUES ($1, $2) RETURNING id::text`, name, takesRuns).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return mustParse(t, id)
}

func grantToken(id pgtype.UUID) string { return "@[Codex](grant:" + id.String() + ")" }

func accountToken(id pgtype.UUID) string { return "@[ci-bot](service_account:" + id.String() + ")" }

func (r *rig) openWithRun(t *testing.T, by auth.Principal, in OpenInput) (Thread, Message, *StartedRun, *RunRefusal) {
	t.Helper()
	thread, message, run, refusal, err := r.mentions.Open(t.Context(), by, in, true)
	if err != nil {
		t.Fatalf("open %+v: %v", in, err)
	}
	return thread, message, run, refusal
}

func (r *rig) replyWithRun(t *testing.T, by auth.Principal, thread, body string) (Message, *StartedRun, *RunRefusal) {
	t.Helper()
	_, message, run, refusal, err := r.mentions.Reply(t.Context(), by, mustParse(t, thread), body, true)
	if err != nil {
		t.Fatalf("reply to %s: %v", thread, err)
	}
	return message, run, refusal
}

func TestMentioningObserveOnADashboardThreadQueuesACommentRunWithItsMessage(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t, "rps", "errors")
	tg := target(KindDashboard, dash)

	thread, first, run, refusal := r.openWithRun(t, testUser, OpenInput{Target: tg, Version: new(1), PanelID: new("errors"), Body: "@observe why did errors rise?"})
	if run == nil || refusal != nil || run.Status != "queued" || run.AssignedKind != "builtin" {
		t.Fatalf("run = %+v, refusal = %+v", run, refusal)
	}
	queued := r.queuedRuns(t)
	if len(queued) != 1 {
		t.Fatalf("%d runs queued, want 1", len(queued))
	}
	got := queued[0]
	if got.ID != run.ID || got.Source != "comment" || got.Trigger != "mention" || got.AssignedKind != "builtin" || got.AssignedID != "" ||
		got.CommentID != thread.ID || got.DashboardID != dash.String() || got.BaseVersion != "1" || got.ThreadMessageID != first.ID ||
		got.Prompt != "why did errors rise?" || got.RequestedByID != testUserID || got.ThreadContext.Valid {
		t.Fatalf("run row = %+v", got)
	}

	r.reply(t, commenter, thread.ID, "the deploy was at 09:10")
	second, run, refusal := r.replyWithRun(t, testUser, thread.ID, "@observe and what about latency?")
	if run == nil || refusal != nil {
		t.Fatalf("reply run = %+v, refusal = %+v", run, refusal)
	}
	queued = r.queuedRuns(t)
	if len(queued) != 2 {
		t.Fatalf("%d runs queued, want 2", len(queued))
	}
	reply := queued[1]
	wantContext := "Ada: @observe why did errors rise?\n\nBo: the deploy was at 09:10"
	if reply.ThreadMessageID != second.ID || reply.CommentID != thread.ID || reply.Prompt != "and what about latency?" || !reply.ThreadContext.Valid || reply.ThreadContext.String != wantContext {
		t.Fatalf("reply run = %+v, want context %q", reply, wantContext)
	}
}

func TestAMentionStartsNothingWithoutStartRuns(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	tg := target(KindDashboard, dash)

	thread, _, run, refusal, err := r.mentions.Open(t.Context(), testUser, OpenInput{Target: tg, Body: "@observe look"}, false)
	if err != nil || run != nil || refusal != nil {
		t.Fatalf("open = %v, %+v, %+v", err, run, refusal)
	}
	if _, _, run, refusal, err := r.mentions.Reply(t.Context(), testUser, mustParse(t, thread.ID), "@observe again", false); err != nil || run != nil || refusal != nil {
		t.Fatalf("reply = %v, %+v, %+v", err, run, refusal)
	}
	if runs := r.queuedRuns(t); len(runs) != 0 {
		t.Fatalf("%d runs queued without start_runs", len(runs))
	}
}

func TestStartRunsWithoutAnAgentMentionStartsNothingAndKeepsTheMessage(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)

	thread, _, run, refusal := r.openWithRun(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "plain note for @[Bo](user:" + boID + ")"})
	if run != nil || refusal != nil {
		t.Fatalf("run = %+v, refusal = %+v", run, refusal)
	}
	if _, run, refusal := r.replyWithRun(t, testUser, thread.ID, "still plain"); run != nil || refusal != nil {
		t.Fatalf("reply run = %+v, refusal = %+v", run, refusal)
	}
	if runs := r.queuedRuns(t); len(runs) != 0 || r.messageCount(t) != 2 {
		t.Fatalf("%d runs, %d messages", len(runs), r.messageCount(t))
	}
}

func TestMentioningObserveOnAnObservationThreadQueuesAnInvestigationIntoIt(t *testing.T) {
	r := newAgentRig(t, true)
	id := r.createObservation(t, testUser, pgtype.UUID{})

	thread, first, run, refusal := r.openWithRun(t, testUser, OpenInput{Target: target(KindObservation, id), Body: "@observe what changed at 09:10?"})
	if run == nil || refusal != nil || run.AssignedKind != "builtin" {
		t.Fatalf("run = %+v, refusal = %+v", run, refusal)
	}
	queued := r.queuedRuns(t)
	if len(queued) != 1 {
		t.Fatalf("%d runs queued, want 1", len(queued))
	}
	got := queued[0]
	if got.Source != "investigate" || got.Trigger != "mention" || got.ObservationID != id.String() || got.DashboardID != "" || got.CommentID != "" ||
		got.ThreadMessageID != first.ID || got.Prompt != "what changed at 09:10?" || got.ThreadContext.Valid {
		t.Fatalf("run row = %+v", got)
	}

	_, run, refusal = r.replyWithRun(t, testUser, thread.ID, "@observe and the week before?")
	if run == nil || refusal != nil {
		t.Fatalf("reply run = %+v, refusal = %+v", run, refusal)
	}
	if queued = r.queuedRuns(t); len(queued) != 2 || queued[1].ThreadContext.String != "Ada: @observe what changed at 09:10?" {
		t.Fatalf("runs = %+v", queued)
	}
}

func TestAnnotationAndProposalThreadsStoreTheMessageAndRefuseTheRun(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	note := r.createAnnotation(t, testUser, dash)

	thread, message, run, refusal := r.openWithRun(t, testUser, OpenInput{Target: target(KindAnnotation, note), Body: "@observe was this the deploy?"})
	if run != nil || refusal == nil || refusal.Code != RefusalUnsupportedTarget || refusal.Message != "agents can be asked from dashboard and observation threads" {
		t.Fatalf("run = %+v, refusal = %+v", run, refusal)
	}
	if message.Body != "@observe was this the deploy?" || thread.Target.Kind != KindAnnotation {
		t.Fatalf("message = %+v", message)
	}
	if _, run, refusal := r.replyWithRun(t, testUser, thread.ID, "@observe hello?"); run != nil || refusal == nil || refusal.Code != RefusalUnsupportedTarget {
		t.Fatalf("reply run = %+v, refusal = %+v", run, refusal)
	}

	proposal := target(KindProposal, r.createProposal(t, dash))
	if _, _, run, refusal := r.openWithRun(t, commenter, OpenInput{Target: proposal, Body: "@observe is this safe?"}); run != nil || refusal == nil || refusal.Code != RefusalUnsupportedTarget {
		t.Fatalf("proposal run = %+v, refusal = %+v", run, refusal)
	}
	if runs := r.queuedRuns(t); len(runs) != 0 || r.messageCount(t) != 3 {
		t.Fatalf("%d runs, %d messages", len(runs), r.messageCount(t))
	}
}

func TestTwoAgentMentionsWithStartRunsAreRefusedAndWriteNothing(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	grant := r.connectedAgent(t, testUser)
	tg := target(KindDashboard, dash)

	bodies := []string{
		"@observe and " + grantToken(grant),
		accountToken(r.serviceAccount(t, "ci-bot", true)) + " and " + grantToken(grant),
	}
	for _, body := range bodies {
		_, _, _, _, err := r.mentions.Open(t.Context(), testUser, OpenInput{Target: tg, Body: body}, true)
		var invalid *InvalidInputError
		if !errors.As(err, &invalid) || invalid.Code != CodeTooManyAgents {
			t.Fatalf("open %q: err = %v, want too_many_agents", body, err)
		}
	}
	thread, _ := r.open(t, testUser, OpenInput{Target: tg, Body: "first"})
	_, _, _, _, err := r.mentions.Reply(t.Context(), testUser, mustParse(t, thread.ID), bodies[0], true)
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeTooManyAgents {
		t.Fatalf("reply: err = %v, want too_many_agents", err)
	}
	if runs := r.queuedRuns(t); len(runs) != 0 || r.messageCount(t) != 1 {
		t.Fatalf("%d runs, %d messages: a refused message writes nothing", len(runs), r.messageCount(t))
	}
	if _, _, _, _, err := r.mentions.Open(t.Context(), testUser, OpenInput{Target: tg, Body: bodies[0]}, false); err != nil {
		t.Fatalf("two agents without start_runs are only mentions: %v", err)
	}
}

func TestAnAuthorWhoMayNotStartTheRunStillPostsTheMessage(t *testing.T) {
	r := newAgentRig(t, true)
	id := r.createObservation(t, testUser, pgtype.UUID{})
	tg := target(KindObservation, id)

	thread, message, run, refusal := r.openWithRun(t, commenter, OpenInput{Target: tg, Body: "@observe look at this"})
	if run != nil || refusal == nil || refusal.Code != RefusalForbidden {
		t.Fatalf("run = %+v, refusal = %+v", run, refusal)
	}
	if message.ID == "" || thread.ID == "" || r.messageCount(t) != 1 || len(r.queuedRuns(t)) != 0 {
		t.Fatalf("message = %+v, %d messages, %d runs", message, r.messageCount(t), len(r.queuedRuns(t)))
	}
}

func TestADashboardThreadRunNeedsOnlyTheCommentRole(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)

	_, _, run, refusal := r.openWithRun(t, commenter, OpenInput{Target: target(KindDashboard, dash), Body: "@observe could this be a proposal?"})
	if run == nil || refusal != nil {
		t.Fatalf("a commenter's run = %+v, refusal = %+v", run, refusal)
	}
	if queued := r.queuedRuns(t); len(queued) != 1 || queued[0].RequestedByID != commenterID {
		t.Fatalf("runs = %+v", queued)
	}
}

func TestWithoutAProviderObserveIsRefusedButAConnectedAgentStillGetsTheRun(t *testing.T) {
	r := newAgentRig(t, false)
	dash := r.createDashboard(t)
	tg := target(KindDashboard, dash)
	grant := r.connectedAgent(t, testUser)
	account := r.serviceAccount(t, "ci-bot", true)

	thread, _, run, refusal := r.openWithRun(t, testUser, OpenInput{Target: tg, Body: "@observe hello"})
	if run != nil || refusal == nil || refusal.Code != RefusalAgentUnavailable {
		t.Fatalf("run = %+v, refusal = %+v", run, refusal)
	}
	if runs := r.queuedRuns(t); len(runs) != 0 || r.messageCount(t) != 1 {
		t.Fatalf("%d runs, %d messages", len(runs), r.messageCount(t))
	}

	_, run, refusal = r.replyWithRun(t, testUser, thread.ID, grantToken(grant)+" please look")
	if run == nil || refusal != nil || run.AssignedKind != "grant" {
		t.Fatalf("grant run = %+v, refusal = %+v", run, refusal)
	}
	_, run, refusal = r.replyWithRun(t, testUser, thread.ID, accountToken(account)+" please look")
	if run == nil || refusal != nil || run.AssignedKind != "service_account" {
		t.Fatalf("service account run = %+v, refusal = %+v", run, refusal)
	}
	queued := r.queuedRuns(t)
	if len(queued) != 2 || queued[0].AssignedKind != "grant" || queued[0].AssignedID != grant.String() || queued[0].Prompt != "please look" ||
		queued[1].AssignedKind != "service_account" || queued[1].AssignedID != account.String() {
		t.Fatalf("runs = %+v", queued)
	}
}

func TestAConnectedAgentMentionOnAnObservationAssignsTheInvestigationToIt(t *testing.T) {
	r := newAgentRig(t, true)
	id := r.createObservation(t, testUser, pgtype.UUID{})
	grant := r.connectedAgent(t, testUser)

	_, _, run, refusal := r.openWithRun(t, testUser, OpenInput{Target: target(KindObservation, id), Body: "can " + grantToken(grant) + " check the deploy log?"})
	if run == nil || refusal != nil || run.AssignedKind != "grant" {
		t.Fatalf("run = %+v, refusal = %+v", run, refusal)
	}
	queued := r.queuedRuns(t)
	if len(queued) != 1 || queued[0].AssignedID != grant.String() || queued[0].ObservationID != id.String() || queued[0].Prompt != "can  check the deploy log?" {
		t.Fatalf("runs = %+v", queued)
	}
}

func TestAFullDashboardQueueRefusesTheRunButKeepsTheMessage(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	tg := target(KindDashboard, dash)
	for i := range revisions.MaxActive {
		if _, _, run, refusal := r.openWithRun(t, testUser, OpenInput{Target: tg, Body: fmt.Sprintf("@observe note %d", i)}); run == nil || refusal != nil {
			t.Fatalf("run %d = %+v, refusal = %+v", i, run, refusal)
		}
	}
	_, message, run, refusal := r.openWithRun(t, testUser, OpenInput{Target: tg, Body: "@observe one too many"})
	if run != nil || refusal == nil || refusal.Code != RefusalTooManyRuns || refusal.Message == "" {
		t.Fatalf("run = %+v, refusal = %+v", run, refusal)
	}
	if message.ID == "" || r.messageCount(t) != revisions.MaxActive+1 || len(r.queuedRuns(t)) != revisions.MaxActive {
		t.Fatalf("%d messages, %d runs", r.messageCount(t), len(r.queuedRuns(t)))
	}
}

func TestTheTranscriptKeepsTheLastTwentyMessagesOldestFirst(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	thread, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "message 0"})
	for i := 1; i <= 24; i++ {
		r.reply(t, commenter, thread.ID, fmt.Sprintf("message %d", i))
	}
	r.replyWithRun(t, testUser, thread.ID, "@observe summarize")

	context := r.queuedRuns(t)[0].ThreadContext.String
	lines := strings.Split(context, "\n\n")
	if len(lines) != transcriptMessages || lines[0] != "Bo: message 5" || lines[transcriptMessages-1] != "Bo: message 24" {
		t.Fatalf("%d messages, first %q, last %q", len(lines), lines[0], lines[len(lines)-1])
	}
}

func TestTheTranscriptIsCutFromTheOldestSideToEightThousandRunes(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	thread, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "oldest " + strings.Repeat("a", 3900)})
	r.reply(t, commenter, thread.ID, "middle "+strings.Repeat("é", 3900))
	r.reply(t, commenter, thread.ID, "newest "+strings.Repeat("b", 3900))
	r.replyWithRun(t, testUser, thread.ID, "@observe go")

	context := r.queuedRuns(t)[0].ThreadContext.String
	if n := len([]rune(context)); n > transcriptRunes || n < transcriptRunes-1 {
		t.Fatalf("transcript is %d runes, want 8000", n)
	}
	if strings.Contains(context, "oldest") || !strings.HasSuffix(context, strings.Repeat("b", 3900)) || !strings.Contains(context, "Bo: newest ") {
		t.Fatalf("the cut did not keep the newest side: %.60q ... %.40q", context, context[len(context)-40:])
	}
}

func TestAnEnqueueFailureOtherThanARefusalRollsTheMessageBack(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	r.db.Exec(t, `ALTER TABLE agent_requests ADD CONSTRAINT test_refuses_boom CHECK (prompt <> 'boom')`)

	_, _, _, _, err := r.mentions.Open(t.Context(), testUser, OpenInput{Target: target(KindDashboard, dash), Body: "@observe boom"}, true)
	if err == nil {
		t.Fatal("open succeeded, want the enqueue failure")
	}
	var invalid *InvalidInputError
	if errors.As(err, &invalid) {
		t.Fatalf("err = %v, want an internal failure", err)
	}
	if r.messageCount(t) != 0 || len(r.queuedRuns(t)) != 0 {
		t.Fatalf("%d messages, %d runs: the message must not outlive its failed run", r.messageCount(t), len(r.queuedRuns(t)))
	}
}

func TestOnlyOwnConnectedAgentsAndServiceAccountsThatTakeRunsCanBeMentioned(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	tg := target(KindDashboard, dash)
	mine := r.connectedAgent(t, testUser)
	theirs := r.connectedAgent(t, commenter)
	revoked := r.connectedAgent(t, testUser)
	r.db.Exec(t, `UPDATE oauth_grants SET revoked_at = now(), revoked_reason = 'person' WHERE id = $1::uuid`, revoked.String())
	expired := r.connectedAgent(t, testUser)
	r.db.Exec(t, `UPDATE oauth_grants SET expires_at = now() - interval '1 minute' WHERE id = $1::uuid`, expired.String())
	takesRuns := r.serviceAccount(t, "runner", true)
	idle := r.serviceAccount(t, "idle", false)
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: takesRuns.String(), Name: "runner", Role: auth.RoleEditor}

	tests := []struct {
		name string
		by   auth.Principal
		body string
		kind string
		ok   bool
	}{
		{"my own grant", testUser, grantToken(mine), "grant", true},
		{"another person's grant", testUser, grantToken(theirs), "", false},
		{"a revoked grant", testUser, grantToken(revoked), "", false},
		{"an expired grant", testUser, grantToken(expired), "", false},
		{"an unknown grant", testUser, grantToken(mustParse(t, grantID)), "", false},
		{"a service account that takes runs", testUser, accountToken(takesRuns), "service_account", true},
		{"a service account that takes none", testUser, accountToken(idle), "", false},
		{"an unknown service account", testUser, accountToken(mustParse(t, accountID)), "", false},
		{"a key mentioning a service account", key, accountToken(takesRuns), "service_account", true},
		{"a key mentioning a grant", key, grantToken(mine), "", false},
		{"a key mentioning the built-in agent", key, "@observe look", "agent", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, message, err := r.service.Open(t.Context(), tt.by, OpenInput{Target: tg, Body: tt.body}, nil)
			if !tt.ok {
				var invalid *InvalidInputError
				if !errors.As(err, &invalid) || invalid.Code != CodeInvalidMention || invalid.Message != "you can mention only your own connected agents and service accounts that take runs" {
					t.Fatalf("err = %v, want 422 invalid_mention", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(message.Mentions) != 1 || message.Mentions[0].Kind() != tt.kind {
				t.Fatalf("mentions = %+v, want one %s", message.Mentions, tt.kind)
			}
		})
	}
}

func TestMentionsOfAgentsAreStoredNamedAndReadBack(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	grant := r.connectedAgent(t, testUser)
	account := r.serviceAccount(t, "runner", true)

	thread, message := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "@[whatever](grant:" + grant.String() + ") and @[x](service_account:" + account.String() + ")"})
	if len(message.Mentions) != 2 {
		t.Fatalf("mentions = %+v", message.Mentions)
	}
	listed := r.list(t, testUser, target(KindDashboard, dash))
	got := map[string]string{}
	for _, m := range listed.Messages[0].Mentions {
		got[m.Kind()+":"+m.RefID()] = m.Name
	}
	if got["grant:"+grant.String()] != "Codex" || got["service_account:"+account.String()] != "runner" {
		t.Fatalf("names = %v", got)
	}
	var mentioned []string
	for _, row := range r.activity(t) {
		if row.Type == "thread.mentioned" {
			mentioned = append(mentioned, fmt.Sprint(row.Data["mentioned"].(map[string]any)["kind"]))
		}
	}
	if len(mentioned) != 2 || r.messageCount(t) != 1 || thread.ID == "" {
		t.Fatalf("mentioned events = %v", mentioned)
	}
}

func TestAnEditCanNotAddAnAgentMentionThatCouldNotBePosted(t *testing.T) {
	r := newRig(t)
	dash := r.createDashboard(t)
	theirs := r.connectedAgent(t, commenter)
	_, first := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "draft"})

	_, _, _, err := r.service.Edit(t.Context(), testUser, mustParse(t, first.ID), "now with "+grantToken(theirs))
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeInvalidMention {
		t.Fatalf("err = %v, want invalid_mention", err)
	}
	if got := r.list(t, testUser, target(KindDashboard, dash)).Messages[0]; got.Body != "draft" || len(got.Mentions) != 0 {
		t.Fatalf("the refused edit changed the message: %+v", got)
	}
}

func TestAnEditNeverStartsARun(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	_, first := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "draft"})
	if _, _, _, err := r.service.Edit(t.Context(), testUser, mustParse(t, first.ID), "@observe now"); err != nil {
		t.Fatal(err)
	}
	if runs := r.queuedRuns(t); len(runs) != 0 {
		t.Fatalf("%d runs queued by an edit", len(runs))
	}
}

func TestHTTPStartRunsIsOptInAndAnswersTheRunOrTheRefusal(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	note := r.createAnnotation(t, testUser, dash)
	dashTarget := "dashboard:" + dash.String()

	plain := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": dashTarget, "body": "@observe without the flag"})
	plain.expect(t, http.StatusCreated, "")
	if plain.at(t, "message", "run") != nil || plain.at(t, "message", "run_refusal") != nil {
		t.Fatalf("a write without start_runs answered %v", plain.body)
	}

	started := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": dashTarget, "body": "@observe with the flag", "start_runs": true})
	started.expect(t, http.StatusCreated, "")
	queued := r.queuedRuns(t)
	if len(queued) != 1 || started.at(t, "message", "run", "id") != queued[0].ID || started.at(t, "message", "run", "status") != "queued" ||
		started.at(t, "message", "run", "assigned_kind") != "builtin" || started.at(t, "message", "run_refusal") != nil {
		t.Fatalf("answer %v, runs %+v", started.body, queued)
	}
	threadID := started.at(t, "thread", "id").(string)

	replied := r.call(t, testUser, http.MethodPost, "/threads/"+threadID+"/messages", map[string]any{"body": "@observe again", "start_runs": true})
	replied.expect(t, http.StatusCreated, "")
	if replied.at(t, "message", "run", "id") == nil || len(r.queuedRuns(t)) != 2 {
		t.Fatalf("reply answered %v", replied.body)
	}

	refused := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": "annotation:" + note.String(), "body": "@observe hi", "start_runs": true})
	refused.expect(t, http.StatusCreated, "")
	if refused.at(t, "message", "run") != nil || refused.at(t, "message", "run_refusal", "code") != RefusalUnsupportedTarget {
		t.Fatalf("refusal answered %v", refused.body)
	}

	two := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": dashTarget, "body": "@observe @[Codex](grant:" + grantID + ")", "start_runs": true})
	two.expect(t, http.StatusUnprocessableEntity, CodeTooManyAgents)

	foreign := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": dashTarget, "body": "@[Codex](grant:" + grantID + ")"})
	foreign.expect(t, http.StatusUnprocessableEntity, CodeInvalidMention)

	listed := r.call(t, testUser, http.MethodGet, "/threads?target="+dashTarget+"&status=all", nil)
	listed.expect(t, http.StatusOK, "")
	for _, m := range listed.at(t, "messages").([]any) {
		msg := m.(map[string]any)
		if msg["run"] != nil || msg["run_refusal"] != nil {
			t.Fatalf("a list read carried a run: %v", msg)
		}
	}
}

func TestOverMCPAnObservationThreadStoresTheMessageAndRefusesTheInvestigation(t *testing.T) {
	r := newAgentRig(t, true)
	id := r.createObservation(t, testUser, pgtype.UUID{})

	thread, _, run, refusal, err := r.mentions.Open(t.Context(), testUser, OpenInput{Target: target(KindObservation, id), Body: "@observe what changed at 09:10?"}, true, WithoutInvestigations())
	if err != nil {
		t.Fatal(err)
	}
	if run != nil || refusal == nil || refusal.Code != RefusalPersonStarts || !strings.Contains(refusal.Message, "web app or the CLI") {
		t.Fatalf("run = %+v, refusal = %+v", run, refusal)
	}

	_, _, run, refusal, err = r.mentions.Reply(t.Context(), testUser, mustParse(t, thread.ID), "@observe and the week before?", true, WithoutInvestigations())
	if err != nil {
		t.Fatal(err)
	}
	if run != nil || refusal == nil || refusal.Code != RefusalPersonStarts {
		t.Fatalf("reply run = %+v, refusal = %+v", run, refusal)
	}
	if n := r.messageCount(t); n != 2 {
		t.Fatalf("%d messages stored, want 2", n)
	}
	if queued := r.queuedRuns(t); len(queued) != 0 {
		t.Fatalf("%d runs queued over MCP on an observation, want 0", len(queued))
	}
}

func TestOverMCPAConnectedAgentMentionOnAnObservationIsRefusedToo(t *testing.T) {
	r := newAgentRig(t, true)
	id := r.createObservation(t, testUser, pgtype.UUID{})
	account := r.serviceAccount(t, "ci-bot", true)

	_, _, run, refusal, err := r.mentions.Open(t.Context(), testUser, OpenInput{Target: target(KindObservation, id), Body: accountToken(account) + " look"}, true, WithoutInvestigations())
	if err != nil {
		t.Fatal(err)
	}
	if run != nil || refusal == nil || refusal.Code != RefusalPersonStarts || len(r.queuedRuns(t)) != 0 {
		t.Fatalf("run = %+v, refusal = %+v, runs = %+v", run, refusal, r.queuedRuns(t))
	}
}

func TestOverMCPADashboardThreadStillStartsACommentRun(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)

	thread, first, run, refusal, err := r.mentions.Open(t.Context(), testUser, OpenInput{Target: target(KindDashboard, dash), Body: "@observe fix the title"}, true, WithoutInvestigations())
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || refusal != nil || run.AssignedKind != "builtin" {
		t.Fatalf("run = %+v, refusal = %+v", run, refusal)
	}
	_, _, run, refusal, err = r.mentions.Reply(t.Context(), testUser, mustParse(t, thread.ID), "@observe and the legend", true, WithoutInvestigations())
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || refusal != nil {
		t.Fatalf("reply run = %+v, refusal = %+v", run, refusal)
	}
	queued := r.queuedRuns(t)
	if len(queued) != 2 || queued[0].Source != "comment" || queued[0].DashboardID != dash.String() || queued[0].ThreadMessageID != first.ID || queued[1].Source != "comment" {
		t.Fatalf("runs = %+v", queued)
	}
}

func TestHTTPStartsAnInvestigationOnAnObservationThread(t *testing.T) {
	r := newAgentRig(t, true)
	id := r.createObservation(t, testUser, pgtype.UUID{})

	started := r.call(t, testUser, http.MethodPost, "/threads", map[string]any{"target": "observation:" + id.String(), "body": "@observe what changed?", "start_runs": true})
	started.expect(t, http.StatusCreated, "")
	queued := r.queuedRuns(t)
	if len(queued) != 1 || queued[0].Source != "investigate" || queued[0].ObservationID != id.String() ||
		started.at(t, "message", "run", "id") != queued[0].ID || started.at(t, "message", "run_refusal") != nil {
		t.Fatalf("answer %v, runs %+v", started.body, queued)
	}
}

func TestAMentionRunCarriesTheOriginAndAssigneeTheWritesOfItsRunRead(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	obs := r.createObservation(t, testUser, pgtype.UUID{})
	grant := r.connectedAgent(t, testUser)
	mcp := origin.Origin{Kind: origin.MCP, Client: "Codex"}
	outside := origin.Origin{Kind: origin.MCP, Client: "other"}
	tests := []struct {
		name         string
		target       Target
		body         string
		ctxOrigin    origin.Origin
		wantAssignee revisions.AssigneeKind
		wantSource   revisions.Source
		wantWrites   origin.Origin
	}{
		{"built-in agent on a dashboard", target(KindDashboard, dash), "@observe look", origin.Origin{Kind: origin.CLI}, revisions.AssignBuiltin, revisions.SourceComment, origin.Origin{Kind: origin.CLI}},
		{"built-in agent on an observation", target(KindObservation, obs), "@observe look", origin.Origin{Kind: origin.Web}, revisions.AssignBuiltin, revisions.SourceInvestigate, origin.Origin{Kind: origin.Web}},
		{"connected agent", target(KindDashboard, dash), grantToken(grant) + " look", mcp, revisions.AssignGrant, revisions.SourceComment, outside},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := OpenInput{Target: tt.target, Body: tt.body}
			if tt.target.Kind == KindDashboard {
				in.Version = new(1)
			}
			_, _, started, refusal, err := r.mentions.Open(origin.With(t.Context(), tt.ctxOrigin), testUser, in, true)
			if err != nil || started == nil || refusal != nil {
				t.Fatalf("started = %+v, refusal = %+v, err = %v", started, refusal, err)
			}
			row, err := r.db.Queries().GetAgentRequestByID(t.Context(), mustParse(t, started.ID))
			if err != nil {
				t.Fatal(err)
			}
			req := revisions.RequestOf(row)
			if req.Assignee != tt.wantAssignee || req.Source != tt.wantSource || req.Origin != tt.ctxOrigin || req.RequestedBy.ID != testUserID {
				t.Fatalf("request = %+v", req)
			}
			if got := req.WriteOrigin(outside); got != tt.wantWrites {
				t.Fatalf("WriteOrigin = %+v, want %+v", got, tt.wantWrites)
			}
		})
	}
}
