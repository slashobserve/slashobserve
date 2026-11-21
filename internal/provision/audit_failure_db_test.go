package provision_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit"
)

type countedDrops struct {
	mu      sync.Mutex
	reasons map[string]int
}

func (c *countedDrops) EventsDropped(reason string, n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reasons == nil {
		c.reasons = map[string]int{}
	}
	c.reasons[reason] += n
}

func (c *countedDrops) countOf(reason string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reasons[reason]
}

func (*countedDrops) BatchWritten(time.Duration) {}
func (*countedDrops) BufferedEvents(int)         {}
func (*countedDrops) PartitionsAhead(int)        {}

func (r *rig) refuseProvisionEvents(t *testing.T) *countedDrops {
	t.Helper()
	drops := &countedDrops{}
	audit.Install(audit.New(audit.Config{DB: r.db.DB, Metrics: drops}))
	t.Cleanup(func() { audit.Install(nil) })
	exec := func(statement string) {
		if _, err := r.db.SQLDB(t).ExecContext(context.Background(), statement); err != nil {
			t.Fatal(err)
		}
	}
	exec(`ALTER TABLE audit_events ADD CONSTRAINT refuse_provision_apply CHECK (action <> 'provision.apply') NOT VALID`)
	t.Cleanup(func() { exec(`ALTER TABLE audit_events DROP CONSTRAINT refuse_provision_apply`) })
	return drops
}

func (r *rig) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAFileWhoseEventCannotBeRecordedIsStillBoundSoTheNextRunDoesNotCreateItAgain(t *testing.T) {
	r := newRig(t)
	drops := r.refuseProvisionEvents(t)
	r.write(t, "a.json", specJSON("Twice", "up"))

	if err := r.provisioner.Run(t.Context()); err != nil {
		t.Errorf("Run with the event refused: %v", err)
	}
	if err := r.provisioner.Run(t.Context()); err != nil {
		t.Errorf("second Run with the event refused: %v", err)
	}

	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE title = 'Twice'`); n != 1 {
		t.Errorf("%d dashboards titled Twice, want 1", n)
	}
	if n := len(r.report(t).Dashboards); n != 1 {
		t.Errorf("%d bound dashboards, want 1", n)
	}
	if got := drops.countOf(audit.DroppedRecordFailed); got != 2 {
		t.Errorf("dropped %s = %d, want 2, the save and the unchanged run", audit.DroppedRecordFailed, got)
	}
}

func TestAChangedFileWhoseEventCannotBeRecordedAddsOneVersionNotOnePerRun(t *testing.T) {
	r := newRig(t)
	r.write(t, "a.json", specJSON("Edited", "up"))
	report := r.run(t)
	id := entry(t, report, "a.json").DashboardID
	r.refuseProvisionEvents(t)
	r.write(t, "a.json", specJSON("Edited", "rate(up[5m])"))

	for range 3 {
		if err := r.provisioner.Run(t.Context()); err != nil {
			t.Errorf("Run with the event refused: %v", err)
		}
	}

	if n := r.count(t, `SELECT count(*) FROM dashboard_versions WHERE dashboard_id = $1`, id); n != 2 {
		t.Errorf("%d versions, want the first and one for the change", n)
	}
}

func TestAnApplyWhoseEventCannotBeRecordedIsStillBoundSoAnotherApplyDoesNotCreateItAgain(t *testing.T) {
	r := newApplyRig(t)
	drops := r.refuseProvisionEvents(t)

	first := r.apply(t, r.editor, "Shop/again.json", specJSON("Again", "up"))
	second := r.apply(t, r.editor, "Shop/again.json", specJSON("Again", "up"))

	if n := r.count(t, `SELECT count(*) FROM dashboards WHERE title = 'Again'`); n != 1 {
		t.Errorf("%d dashboards titled Again, want 1", n)
	}
	if first.DashboardID != second.DashboardID {
		t.Errorf("the second apply went to %s, the first created %s", second.DashboardID, first.DashboardID)
	}
	if got := drops.countOf(audit.DroppedRecordFailed); got != 2 {
		t.Errorf("dropped %s = %d, want 2", audit.DroppedRecordFailed, got)
	}
}
