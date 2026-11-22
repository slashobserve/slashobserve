package threads

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const sharedWord = "wombatledger"

var otherEditor = auth.Principal{Kind: auth.KindUser, ID: "66666666-6666-4666-8666-666666666666", Name: "Eve", Role: auth.RoleEditor}

func newShareRig(t *testing.T) *rig {
	t.Helper()
	r := newAgentRig(t, true)
	r.service.ShowAnswersWith(r.findings.ShowIn, r.findings.HideFrom)
	r.findings.ReadThreadsWith(func(ctx context.Context, by auth.Principal, messageID pgtype.UUID) error {
		_, err := r.service.ReadMessage(ctx, by, messageID)
		return err
	})
	r.revisions.ReadThreadsWith(func(ctx context.Context, by auth.Principal, messageID pgtype.UUID) error {
		_, err := r.service.ReadMessage(ctx, by, messageID)
		return err
	})
	return r
}

func (r *rig) privateAnswer(t *testing.T, req revisions.StartRequest, datasourceUID, panel string) pgtype.UUID {
	t.Helper()
	prompt := "why does " + sharedWord + " fail"
	kind := "ask"
	req.Prompt, req.Kind = &prompt, &kind
	started, err := r.revisions.Start(reading(t.Context()), testUser, req)
	if err != nil {
		t.Fatal(err)
	}
	if !revisions.PrivateAsk(started.AgentRequest) {
		t.Fatalf("run %+v is not a private ask", started.AgentRequest)
	}
	r.db.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_kind = 'observation', finished_at = now(), panel_id = nullif($2, '') WHERE id = $1`, started.ID, panel)
	evidence := spec.Panel{ID: "e1", Type: "stat", Title: "errors", Datasource: datasourceUID, Queries: []spec.Query{{Query: "up"}}}
	_, _, err = r.findings.Save(reading(t.Context()), testUser, findings.SaveInput{
		AgentRequestID: started.ID, AskDashboardID: started.DashboardID,
		Title: "Checkout is slow", Markdown: "the " + sharedWord + " rate rose", Evidence: []spec.Panel{evidence},
		From: time.Now().Add(-time.Hour), To: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return started.ID
}

func (r *rig) blockCount(t *testing.T, observation pgtype.UUID) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM observation_blocks WHERE observation_id = $1`, observation).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

type auditLinks struct {
	thread, run, finding string
}

func (r *rig) auditLinksOf(t *testing.T, action string) []auditLinks {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT coalesce(thread_id::text, ''),
		CASE WHEN target_kind = 'run' THEN target_id ELSE coalesce(data->>'run_id', '') END,
		CASE WHEN target_kind = 'finding' THEN target_id ELSE coalesce(data->>'finding_id', '') END
		FROM audit_events WHERE action = $1 ORDER BY occurred_at, seq`, action)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []auditLinks
	for rows.Next() {
		var l auditLinks
		if err := rows.Scan(&l.thread, &l.run, &l.finding); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rig) findingOfRun(t *testing.T, by auth.Principal, run pgtype.UUID) (findings.Finding, error) {
	t.Helper()
	f, found, err := r.findings.ForRequest(reading(t.Context()), by, run)
	if err == nil && !found {
		err = findings.ErrNotFound
	}
	return f, err
}

func TestSharingAPrivateDashboardAnswerOpensAThreadThatTheDashboardsReadersRead(t *testing.T) {
	r := newShareRig(t)
	dash := r.createDashboard(t, "rps", "errors")
	id := dash.String()
	run := r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "errors")

	if _, err := r.findingOfRun(t, otherEditor, run); !errors.Is(err, findings.ErrNotFound) {
		t.Fatalf("another editor read the private answer before the share: %v", err)
	}
	if _, err := r.service.ShareRun(reading(t.Context()), otherEditor, ShareInput{RunID: run}); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("someone else shared the run: %v", err)
	}

	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil {
		t.Fatal(err)
	}
	if shared.Replayed || shared.Thread.PanelID != "errors" || !strings.Contains(shared.Message.Body, sharedWord) {
		t.Fatalf("shared = %+v", shared)
	}
	replies := r.runReplies(t, shared.Thread.ID)
	if len(replies) != 1 || replies[0].RunID != run.String() || replies[0].Body != "Answered." {
		t.Fatalf("replies = %+v", replies)
	}

	f, err := r.findingOfRun(t, otherEditor, run)
	if err != nil || !strings.Contains(f.Markdown, sharedWord) || f.ObservationID != "" {
		t.Fatalf("a reader of the thread reads the answer: %+v, %v", f, err)
	}
	stored, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	if !revisions.PrivateAsk(stored) || stored.ThreadMessageID.String() != shared.Message.ID {
		t.Fatalf("the shared run is %+v", stored)
	}

	again, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil || !again.Replayed || again.Thread.ID != shared.Thread.ID {
		t.Fatalf("again = %+v, %v", again, err)
	}
	if n := r.messageCount(t); n != 2 {
		t.Fatalf("messages after a repeat = %d, want 2", n)
	}
	events := r.db.AuditEvents(t, "run.share")
	if len(events) != 1 || events[0].TargetKind != "run" || events[0].TargetID != run.String() || events[0].ActorID != testUserID {
		t.Fatalf("run.share events = %+v, want one for the run by its requester", events)
	}
	if links := r.auditLinksOf(t, "run.share"); len(links) != 1 || links[0] != (auditLinks{thread: shared.Thread.ID, run: run.String(), finding: f.ID}) {
		t.Fatalf("run.share links = %+v, want the thread, the run and the finding %s", links, f.ID)
	}
}

func TestSharingAPrivateObservationAnswerWritesItOntoThePage(t *testing.T) {
	r := newShareRig(t)
	observation := r.filedObservationIn(t, testUser, pgtype.UUID{})
	id := observation.String()
	before := r.blockCount(t, observation)
	run := r.privateAnswer(t, revisions.StartRequest{ObservationID: &id}, "prom", "")
	blocks := r.blockCount(t, observation)
	if blocks != before {
		t.Fatalf("the private ask and its answer changed the page from %d to %d blocks", before, blocks)
	}

	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil {
		t.Fatal(err)
	}
	if after := r.blockCount(t, observation); after != blocks+2 {
		t.Fatalf("blocks %d -> %d, want the text and one evidence block", blocks, after)
	}
	var kinds string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT coalesce(string_agg(kind, ',' ORDER BY position), '') FROM observation_blocks WHERE observation_id = $1 AND run_id = $2`, observation, run).Scan(&kinds); err != nil {
		t.Fatal(err)
	}
	if kinds != "text,panel" {
		t.Fatalf("the shown answer wrote blocks %v, want its text and its evidence panel", kinds)
	}
	f, err := r.findingOfRun(t, testUser, run)
	if err != nil || f.ObservationID != id {
		t.Fatalf("finding = %+v, %v", f, err)
	}
	replies := r.runReplies(t, shared.Thread.ID)
	if len(replies) != 1 || replies[0].Body != "Answered." {
		t.Fatalf("replies = %+v", replies)
	}
	stored, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ObservationID.Valid || !revisions.PrivateAsk(stored) {
		t.Fatalf("the shared run became a run in the observation: %+v", stored)
	}
}

func TestAShareIsRefusedWhenTheEvidenceReadsAnAdminsOnlyDatasourceOutsideAnAdminsOnlyFolder(t *testing.T) {
	r := newShareRig(t)
	r.db.Exec(t, `INSERT INTO datasources (uid, type, name, config, audience) VALUES ('audit', 'prometheus', 'Audit', '{}', 'admins')`)
	dash := r.createDashboard(t)
	id := dash.String()
	run := r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "audit", "")

	_, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	var restricted *folders.RestrictedContentError
	if !errors.As(err, &restricted) || restricted.Datasource != "audit" {
		t.Fatalf("err = %v, want the restricted content refusal", err)
	}
	if n := r.messageCount(t); n != 0 {
		t.Fatalf("messages after a refused share = %d, want 0", n)
	}
	stored, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThreadMessageID.Valid {
		t.Fatal("a refused share marked the run shared")
	}
}

func TestAShareIntoAThreadMustBeOnTheSubjectAndOnlyAFinishedPrivateAskCanBeShared(t *testing.T) {
	r := newShareRig(t)
	dash, other := r.createDashboard(t), r.createDashboard(t)
	id := dash.String()
	run := r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "")
	elsewhere, _, err := r.service.Open(reading(t.Context()), testUser, OpenInput{Target: target(KindDashboard, other), Body: "unrelated"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var invalid *InvalidInputError
	if _, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run, ThreadID: mustParse(t, elsewhere.ID)}); !errors.As(err, &invalid) || invalid.Code != CodeWrongThread {
		t.Fatalf("share into another dashboard's thread: %v", err)
	}
	here, _, err := r.service.Open(reading(t.Context()), testUser, OpenInput{Target: target(KindDashboard, dash), Body: "the latency thread"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run, ThreadID: mustParse(t, here.ID)})
	if err != nil || shared.Thread.ID != here.ID {
		t.Fatalf("shared = %+v, %v", shared, err)
	}

	prompt, kind := "why", "ask"
	unfinished, err := r.revisions.Start(reading(t.Context()), testUser, revisions.StartRequest{Prompt: &prompt, Kind: &kind, DashboardID: &id})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: unfinished.ID}); !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("an unfinished ask: %v", err)
	}
	thread, _, started, _ := r.openWithRun(t, testUser, OpenInput{Target: target(KindDashboard, dash), Body: "@observe what changed"})
	if started == nil {
		t.Fatalf("no run from thread %s", thread.ID)
	}
	if _, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: mustParse(t, started.ID)}); !errors.Is(err, ErrNotShareable) {
		t.Fatalf("a thread run: %v", err)
	}
}
