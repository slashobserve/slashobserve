package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/egress"
)

type deliveryEvent struct {
	outcome, reason, actorKind, initiator, origin, targetKind, targetID, targetName, data string
	status                                                                                int
}

func (r *rig) deliveryEvents(t *testing.T, recorder *audit.Recorder) []deliveryEvent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recorder.Run(ctx)
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT outcome, coalesce(reason, ''), actor_kind, initiator, origin, coalesce(target_kind, ''), coalesce(target_id, ''), coalesce(target_name, ''), data::text, coalesce(status, 0)
FROM audit_events WHERE action = 'webhook.deliver' ORDER BY occurred_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events []deliveryEvent
	for rows.Next() {
		var e deliveryEvent
		if err := rows.Scan(&e.outcome, &e.reason, &e.actorKind, &e.initiator, &e.origin, &e.targetKind, &e.targetID, &e.targetName, &e.data, &e.status); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func TestEachDeliveryAttemptIsRecordedWithItsOutcomeAndNoPayloadOrSecret(t *testing.T) {
	r := newRig(t)
	recorder := audit.New(audit.Config{DB: r.db.DB})
	audit.Install(recorder)
	t.Cleanup(func() { audit.Install(nil) })
	created := r.create(t, activity.TypeFindingCreated)
	r.append(t, activity.TypeFindingCreated)
	r.sender.script(func(n int, _ call) (egress.SendResult, error) {
		if n == 0 {
			return egress.SendResult{Status: http.StatusNoContent}, nil
		}
		return egress.SendResult{Status: http.StatusInternalServerError}, nil
	})
	r.pass(t)
	r.append(t, activity.TypeFindingCreated)
	r.makeDue(t, created.Webhook.ID)
	r.pass(t)

	data := fmt.Sprintf(`{"event_type": %q}`, activity.TypeFindingCreated)
	got := r.deliveryEvents(t, recorder)
	if len(got) != 2 {
		t.Fatalf("events = %+v, want a success then a failure", got)
	}
	delivered := deliveryEvent{outcome: "success", actorKind: "system", initiator: "system", origin: "job", targetKind: "webhook", targetID: created.Webhook.ID, targetName: "Pipeline", data: data, status: http.StatusNoContent}
	if got[0] != delivered {
		t.Fatalf("delivered = %+v, want %+v", got[0], delivered)
	}
	if got[1].outcome != "failed" || got[1].reason == "" || got[1].status != http.StatusInternalServerError || got[1].targetID != created.Webhook.ID || got[1].data != data {
		t.Fatalf("failed = %+v", got[1])
	}
	for _, secret := range []string{created.Secret, receiverURL, "X-SlashObserve-Signature"} {
		var n int
		err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE position($1 in row_to_json(audit_events)::text) > 0`, secret).Scan(&n)
		if err != nil || n != 0 {
			t.Fatalf("%d audit rows carry %q, %v", n, secret, err)
		}
	}
}
