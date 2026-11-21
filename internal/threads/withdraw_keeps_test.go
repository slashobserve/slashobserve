package threads

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) replyHeldOpen(t *testing.T, by auth.Principal, threadID pgtype.UUID) (written <-chan struct{}, release chan<- struct{}, done <-chan error) {
	t.Helper()
	row, sc, err := r.service.writableThread(reading(t.Context()), by, threadID)
	if err != nil {
		t.Fatal(err)
	}
	inserted, letGo, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	hold := func(*dbsqlc.Queries, Thread, Message) error {
		close(inserted)
		<-letGo
		return nil
	}
	go func() {
		_, err := r.service.reply(reading(t.Context()), by, row, sc, ReplyInput{Body: "seen it here too"}, writeOptions{hook: hold})
		finished <- err
	}()
	return inserted, letGo, finished
}

func (r *rig) waitForALockWait(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the withdraw never waited for the reply's lock")
}

func TestAWithdrawWaitsForAReplyWrittenAtTheSameTimeAndThenKeepsTheShare(t *testing.T) {
	r := newShareRig(t)
	_, _, shared := r.sharedDashboardAnswer(t)
	threadID := mustParse(t, shared.Thread.ID)

	inserted, release, replied := r.replyHeldOpen(t, otherEditor, threadID)
	<-inserted
	withdrawn := make(chan error, 1)
	go func() { withdrawn <- r.service.Withdraw(reading(t.Context()), testUser, threadID) }()
	r.waitForALockWait(t)
	close(release)

	if err := <-replied; err != nil {
		t.Fatalf("the concurrent reply: %v", err)
	}
	if err := <-withdrawn; !errors.Is(err, ErrOthersWroteHere) {
		t.Fatalf("withdraw beside a concurrent reply: %v, want %v", err, ErrOthersWroteHere)
	}
	if n := r.messageCount(t); n != 3 {
		t.Fatalf("messages after the race = %d, want the share, its answer and the reply", n)
	}
}

func TestWithdrawingAnAnswerSharedIntoAnEarlierThreadKeepsTheThreadAndItsOtherMessages(t *testing.T) {
	r := newShareRig(t)
	dash := r.createDashboard(t, "rps", "errors")
	id := dash.String()
	earlier, _ := r.open(t, testUser, OpenInput{Target: Target{Kind: KindDashboard, ID: dash}, Body: "errors are up since noon"})
	run := r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "errors")
	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run, ThreadID: mustParse(t, earlier.ID)})
	if err != nil {
		t.Fatal(err)
	}
	r.reply(t, testUser, earlier.ID, "and the latency too")

	if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, earlier.ID)); err != nil {
		t.Fatal(err)
	}
	kept, err := r.service.ReadMessage(reading(t.Context()), testUser, mustParse(t, earlier.ID))
	if err != nil || kept.Body != "errors are up since noon" {
		t.Fatalf("the thread's first message after the withdraw = %+v, %v", kept, err)
	}
	if _, err := r.service.ReadMessage(reading(t.Context()), testUser, mustParse(t, shared.Message.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the share message after the withdraw: %v, want it deleted", err)
	}
	if replies := r.runReplies(t, earlier.ID); len(replies) != 0 {
		t.Fatalf("answers left in the thread = %+v, want none", replies)
	}
	if n := r.messageCount(t); n != 2 {
		t.Fatalf("messages after the withdraw = %d, want the first message and the later reply", n)
	}
	stored, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThreadMessageID.Valid || !revisions.PrivateAsk(stored) {
		t.Fatalf("the withdrawn run is %+v", stored)
	}
}
