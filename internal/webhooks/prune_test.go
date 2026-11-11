package webhooks

import (
	"testing"
	"time"
)

func TestPruneDeletesAttemptsOlderThanSevenDaysOnly(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	insert := `INSERT INTO webhook_deliveries (webhook_id, delivery_id, event_type, activity_seq, attempted_at, duration_ms, status_code, outcome)
		VALUES ($1::uuid, 'd', 'finding.created', 1, now() - $2::interval, 5, 204, 'delivered')`
	for _, age := range []string{"8 days", "8 days", "8 days", "6 days", "1 hour"} {
		r.db.Exec(t, insert, created.Webhook.ID, age)
	}

	deleted, err := prune(t.Context(), r.db.Queries(), time.Now().Add(-attemptRetention), 2)

	if err != nil || deleted != 3 {
		t.Fatalf("deleted %d, err %v, want the three old attempts across two batches", deleted, err)
	}
	if left := len(r.deliveries(t, created.Webhook.ID)); left != 2 {
		t.Fatalf("%d attempts left, want 2", left)
	}
}
