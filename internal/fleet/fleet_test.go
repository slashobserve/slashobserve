package fleet

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

func TestShare(t *testing.T) {
	tests := []struct {
		live, total, want int
	}{
		{1, 1, 1}, {1, 32, 32}, {1, 120, 120},
		{2, 1, 1}, {2, 32, 16}, {2, 120, 60},
		{3, 1, 1}, {3, 32, 11}, {3, 120, 40},
		{100, 32, 1},
		{2, 0, 1}, {2, -5, 1},
	}
	for _, tc := range tests {
		m := &Membership{}
		m.live.Store(int64(tc.live))
		if got := m.Share(tc.total); got != tc.want {
			t.Errorf("Share(%d) with %d live = %d, want %d", tc.total, tc.live, got, tc.want)
		}
	}
}

func TestLiveIsOneBeforeTheFirstBeat(t *testing.T) {
	if got := (&Membership{}).Live(); got != 1 {
		t.Errorf("Live() = %d, want 1", got)
	}
}

func newMembership(d *dbtest.Database, id string) *Membership {
	logger, _ := logtest.New()
	m := New(d.DB, id, "test", logger)
	m.beat = 20 * time.Millisecond
	return m
}

func start(t *testing.T, m *Membership) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()
	stopped := false
	stop = func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		<-done
	}
	t.Cleanup(stop)
	return stop
}

func waitLive(t *testing.T, m *Membership, want int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for m.Live() != want {
		if time.Now().After(deadline) {
			t.Fatalf("%s sees %d live servers, want %d", m.id, m.Live(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func instanceIDs(t *testing.T, d *dbtest.Database) map[string]bool {
	t.Helper()
	rows, err := d.SQLDB(t).QueryContext(t.Context(), "SELECT id FROM server_instances")
	if err != nil {
		t.Fatalf("read server_instances: %v", err)
	}
	defer func() { _ = rows.Close() }()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		ids[id] = true
	}
	return ids
}

func TestMembershipsCountEachOtherAndLeave(t *testing.T) {
	d := dbtest.New(t)
	a, b := newMembership(d, "server-a"), newMembership(d, "server-b")
	start(t, a)
	stopB := start(t, b)
	waitLive(t, a, 2)
	waitLive(t, b, 2)
	if got, want := a.Share(32), 16; got != want {
		t.Errorf("Share(32) with two servers = %d, want %d", got, want)
	}

	stopB()
	waitLive(t, a, 1)
	if ids := instanceIDs(t, d); len(ids) != 1 || !ids["server-a"] {
		t.Errorf("rows after server-b left: %v", ids)
	}
}

func TestStaleRowsAreDeletedAndNotCounted(t *testing.T) {
	d := dbtest.New(t)
	d.Exec(t, "INSERT INTO server_instances (id, version, seen_at) VALUES ('crashed', 'old', now() - interval '31 seconds')")
	d.Exec(t, "INSERT INTO server_instances (id, version, seen_at) VALUES ('recent', 'old', now() - interval '29 seconds')")
	m := newMembership(d, "server-a")

	if err := m.heartbeat(t.Context()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if got := m.Live(); got != 2 {
		t.Errorf("Live() = %d, want 2 (itself and the row seen 29 s ago)", got)
	}
	ids := instanceIDs(t, d)
	if ids["crashed"] || !ids["recent"] || !ids["server-a"] {
		t.Errorf("rows after the beat: %v", ids)
	}

	d.Exec(t, "UPDATE server_instances SET seen_at = now() - interval '31 seconds' WHERE id = 'recent'")
	if err := m.heartbeat(t.Context()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if got := m.Live(); got != 1 {
		t.Errorf("Live() = %d after the other row went stale, want 1", got)
	}
	if ids := instanceIDs(t, d); len(ids) != 1 {
		t.Errorf("rows after the second beat: %v", ids)
	}
}

func TestAFailingHeartbeatKeepsTheLastCount(t *testing.T) {
	d := dbtest.New(t)
	m := newMembership(d, "server-a")
	if err := m.heartbeat(t.Context()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	m.live.Store(3)
	d.Exec(t, "ALTER TABLE server_instances RENAME TO server_instances_away")
	if err := m.heartbeat(t.Context()); err == nil {
		t.Fatal("heartbeat succeeded without the table")
	}
	if got := m.Live(); got != 3 {
		t.Errorf("Live() = %d after a failed beat, want the last count 3", got)
	}
}

func TestAFailureStreakLogsOnce(t *testing.T) {
	d := dbtest.New(t)
	d.Exec(t, "ALTER TABLE server_instances RENAME TO server_instances_away")
	logger, logs := logtest.New()
	m := New(d.DB, "server-a", "test", logger)
	m.beat = 10 * time.Millisecond
	stop := start(t, m)

	deadline := time.Now().Add(30 * time.Second)
	for strings.Count(logs.String(), "fleet_heartbeat_failed") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no heartbeat failure was logged")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	stop()
	if got := strings.Count(logs.String(), "fleet_heartbeat_failed"); got != 1 {
		t.Errorf("logged the failure %d times, want once per streak", got)
	}
}
