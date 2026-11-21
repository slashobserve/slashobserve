package threads

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
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
