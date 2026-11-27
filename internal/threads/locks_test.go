package threads

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const lockWaitTimeout = 10 * time.Second

type heldTx struct {
	ready   chan struct{}
	release chan struct{}
	done    chan error
}

func (r *rig) hold(t *testing.T, body func(q *dbsqlc.Queries) error) *heldTx {
	t.Helper()
	h := &heldTx{ready: make(chan struct{}), release: make(chan struct{}), done: make(chan error, 1)}
	go func() {
		h.done <- r.db.InTx(reading(t.Context()), func(q *dbsqlc.Queries) error {
			if err := body(q); err != nil {
				return err
			}
			close(h.ready)
			<-h.release
			return nil
		})
	}()
	select {
	case <-h.ready:
	case err := <-h.done:
		t.Fatalf("the held transaction ended early: %v", err)
	case <-time.After(lockWaitTimeout):
		t.Fatal("the held transaction did not take its locks")
	}
	t.Cleanup(func() {
		select {
		case <-h.release:
		default:
			close(h.release)
		}
	})
	return h
}

func (h *heldTx) finish(t *testing.T) {
	t.Helper()
	close(h.release)
	select {
	case err := <-h.done:
		if err != nil {
			t.Fatalf("the held transaction failed: %v", err)
		}
	case <-time.After(lockWaitTimeout):
		t.Fatal("the held transaction did not finish")
	}
}

func (r *rig) waitForLockWait(t *testing.T) {
	t.Helper()
	conn := r.db.SQLDB(t)
	deadline := time.Now().Add(lockWaitTimeout)
	for {
		var waiting int
		if err := conn.QueryRowContext(reading(t.Context()), `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no transaction is waiting on a lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func resultOf[T any](t *testing.T, c <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(lockWaitTimeout):
		t.Fatalf("%s did not finish", what)
		panic("unreachable")
	}
}

func notADeadlock(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "40P01" {
		t.Fatalf("deadlock detected: %v", err)
	}
}

func lockDashboardAndDelete(t *testing.T, id pgtype.UUID) func(q *dbsqlc.Queries) error {
	return func(q *dbsqlc.Queries) error {
		if _, err := q.LockDashboardForDelete(reading(t.Context()), id); err != nil {
			return err
		}
		_, err := q.DeleteDashboard(reading(t.Context()), id)
		return err
	}
}

func TestRunSkipsItsResolutionInsteadOfWaitingForADelete(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	comment, queued := r.commentRevision(t, dash, new("rps"), "fix it")
	run := r.runFor(t, dash, queued, 2)
	deleting := r.hold(t, func(q *dbsqlc.Queries) error {
		_, err := q.LockDashboardForDelete(reading(t.Context()), dash)
		return err
	})

	resolved := make(chan error, 1)
	go func() { resolved <- r.resolveForRun(t, run) }()
	err := resultOf(t, resolved, "the run's resolution")
	notADeadlock(t, err)
	if err != nil {
		t.Fatal(err)
	}
	deleting.finish(t)
	if got := r.threadRow(t, comment).Status; got != StatusOpen {
		t.Fatalf("status = %s, want open because the resolution was skipped", got)
	}
}

func TestDeleteWaitsForARunResolvingItsThread(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	comment, queued := r.commentRevision(t, dash, new("rps"), "fix it")
	run := r.runFor(t, dash, queued, 2)
	resolving := r.hold(t, func(q *dbsqlc.Queries) error {
		return NewRuns(r.db.DB).RunEnded(reading(t.Context()), q, run)
	})

	deleted := make(chan error, 1)
	go func() { deleted <- r.dashboards.Delete(reading(t.Context()), testUser, dash) }()
	r.waitForLockWait(t)
	select {
	case err := <-deleted:
		t.Fatalf("the delete finished while the run held the dashboard: %v", err)
	default:
	}
	resolving.finish(t)

	err := resultOf(t, deleted, "the delete")
	notADeadlock(t, err)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Queries().GetThread(reading(t.Context()), mustParse(t, comment)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("thread after the delete: err = %v, want no rows", err)
	}
}

func TestResolvingWithAVersionWhileTheDashboardIsDeletedAnswersCommentNotFound(t *testing.T) {
	r := newAgentRig(t, false)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	comment, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Version: new(1), PanelID: new("rps"), Body: "fix it"})
	deleting := r.hold(t, lockDashboardAndDelete(t, dash))

	resolved := make(chan error, 1)
	go func() {
		_, _, err := r.service.SetStatus(reading(t.Context()), testUser, mustParse(t, comment.ID), StatusResolved, new(2))
		resolved <- err
	}()
	r.waitForLockWait(t)
	deleting.finish(t)

	err := resultOf(t, resolved, "the resolve")
	notADeadlock(t, err)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCommentingWhileTheDashboardIsDeletedAnswersCommentNotFound(t *testing.T) {
	r := newAgentRig(t, false)
	dash := r.createDashboard(t)
	root, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Version: new(1), PanelID: new("rps"), Body: "root"})
	deleting := r.hold(t, lockDashboardAndDelete(t, dash))

	replied := make(chan error, 1)
	go func() {
		_, _, err := r.service.Reply(reading(t.Context()), testUser, mustParse(t, root.ID), ReplyInput{Body: "late"}, nil)
		replied <- err
	}()
	r.waitForLockWait(t)
	deleting.finish(t)

	err := resultOf(t, replied, "the reply")
	notADeadlock(t, err)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestASaveDoesNotMakeARunSkipItsResolution(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	comment, queued := r.commentRevision(t, dash, new("rps"), "fix it")
	run := r.runFor(t, dash, queued, 2)

	saving, err := r.db.SQLDB(t).BeginTx(reading(t.Context()), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = saving.Rollback() })
	if _, err := saving.ExecContext(reading(t.Context()), `UPDATE dashboards SET latest_version = latest_version + 1, title = 'renamed', updated_at = now() WHERE id = $1`, dash.String()); err != nil {
		t.Fatal(err)
	}

	resolved := make(chan error, 1)
	go func() { resolved <- r.resolveForRun(t, run) }()
	if err := resultOf(t, resolved, "the run's resolution"); err != nil {
		t.Fatal(err)
	}
	if err := saving.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := r.threadRow(t, comment).Status; got != StatusResolved {
		t.Fatalf("status = %s, want resolved even though a save was in flight", got)
	}
}
