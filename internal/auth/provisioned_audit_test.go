package auth

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/config"
)

type provisionedEvent struct {
	action, outcome, actorKind, initiator, origin, targetKind, targetName, data string
}

func (r *provisionRig) provisionedEvents() []provisionedEvent {
	r.t.Helper()
	rows, err := r.db.SQLDB(r.t).QueryContext(r.t.Context(), `SELECT action, outcome, actor_kind, initiator, origin, coalesce(target_kind, ''), coalesce(target_name, ''), data::text
FROM audit_events WHERE action LIKE 'service_account.%' ORDER BY occurred_at, id`)
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events []provisionedEvent
	for rows.Next() {
		var e provisionedEvent
		if err := rows.Scan(&e.action, &e.outcome, &e.actorKind, &e.initiator, &e.origin, &e.targetKind, &e.targetName, &e.data); err != nil {
			r.t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		r.t.Fatal(err)
	}
	return events
}

func TestProvisioningFromTheFileRecordsEachAccountAndKeyChange(t *testing.T) {
	r := newProvisionRig(t)
	audit.Install(audit.New(audit.Config{DB: r.db.DB}))
	t.Cleanup(func() { audit.Install(nil) })

	account, key := fileAccount(t, "editor")
	if err := r.service.EnsureProvisioned(t.Context(), []config.ServiceAccount{account}); err != nil {
		t.Fatal(err)
	}
	if err := r.service.EnsureProvisioned(t.Context(), []config.ServiceAccount{account}); err != nil {
		t.Fatal(err)
	}
	rotated, newKey := fileAccount(t, "viewer")
	if err := r.service.EnsureProvisioned(t.Context(), []config.ServiceAccount{rotated}); err != nil {
		t.Fatal(err)
	}

	system := provisionedEvent{outcome: "success", actorKind: "system", initiator: "system", origin: "job", targetKind: "service_account", targetName: "demo-loadgen"}
	with := func(action, data string) provisionedEvent {
		e := system
		e.action, e.data = action, data
		return e
	}
	want := []provisionedEvent{
		with("service_account.create", `{"name": "demo-loadgen", "role": "editor", "takes_runs": false}`),
		with("service_account.key_create", `{"name": "demo-loadgen", "key_name": "From file"}`),
		with("service_account.update", `{"name": "demo-loadgen", "role": "viewer", "takes_runs": false}`),
		with("service_account.key_revoke", `{"name": "demo-loadgen", "key_name": "From file"}`),
		with("service_account.key_create", `{"name": "demo-loadgen", "key_name": "From file"}`),
	}
	got := r.provisionedEvents()
	if len(got) != len(want) {
		t.Fatalf("events = %+v, want %d", got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	for _, secret := range []string{key[4:], newKey[4:]} {
		var n int
		err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE position($1 in row_to_json(audit_events)::text) > 0`, secret).Scan(&n)
		if err != nil || n != 0 {
			t.Fatalf("%d audit rows carry a file key, %v", n, err)
		}
	}
	if strings.Contains(r.logs.String(), key) {
		t.Fatal("the logs hold a file key")
	}
}
