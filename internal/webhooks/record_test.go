package webhooks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/activity"
)

type recordedRow struct {
	Type string
	Data map[string]any
}

func (r *rig) adminEvents(t *testing.T) []recordedRow {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT type, data FROM activity WHERE type = ANY($1) ORDER BY seq`, activity.AdminTypes())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []recordedRow
	for rows.Next() {
		var typ string
		var raw []byte
		if err := rows.Scan(&typ, &raw); err != nil {
			t.Fatal(err)
		}
		row := recordedRow{Type: typ, Data: map[string]any{}}
		if err := json.Unmarshal(raw, &row.Data); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEachWebhookChangeWritesOneAdminEventWithoutTheSecretOrPath(t *testing.T) {
	r := newRig(t)
	created := r.create(t, "dashboard.created")
	id := r.uuid(t, created.Webhook.ID)

	enabled := false
	renamed := "Renamed"
	types := []string{"dashboard.created", "run.failed"}
	if _, err := r.svc.Update(t.Context(), adminTwo, id, Patch{Name: &renamed, Enabled: &enabled, EventTypes: &types}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Update(t.Context(), adminTwo, id, Patch{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	rotated, err := r.svc.RotateSecret(t.Context(), adminOne, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Delete(t.Context(), adminOne, id); err != nil {
		t.Fatal(err)
	}

	events := r.adminEvents(t)
	var got []string
	for _, e := range events {
		got = append(got, e.Type)
	}
	want := []string{"webhook.created", "webhook.updated", "webhook.secret_rotated", "webhook.deleted"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events %v, want %v (a no-op update must not record)", got, want)
	}
	if events[0].Data["name"] != "Pipeline" || events[0].Data["host"] != receiverHost {
		t.Fatalf("created data %v", events[0].Data)
	}
	changes, _ := events[1].Data["changes"].(map[string]any)
	for _, key := range []string{"name", "enabled", "event_types"} {
		if changes[key] == nil {
			t.Fatalf("updated data lacks a %s change: %v", key, events[1].Data)
		}
	}
	if changes["host"] != nil || events[1].Data["url_changed"] != nil {
		t.Fatalf("updated data reports an address change that did not happen: %v", events[1].Data)
	}
	body, _ := json.Marshal(events)
	for _, secret := range []string{created.Secret, rotated.Secret, "/in"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("an admin event holds %q: %s", secret, body)
		}
	}
}

func TestAFailedWebhookWriteRecordsNothing(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Delete(t.Context(), adminOne, r.uuid(t, "88888888-8888-4888-8888-888888888888")); err == nil {
		t.Fatal("deleting a missing webhook succeeded")
	}
	if events := r.adminEvents(t); len(events) != 0 {
		t.Fatalf("events %v", events)
	}
}
