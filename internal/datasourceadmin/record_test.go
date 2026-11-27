package datasourceadmin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/egress/domains"
)

type recordedEvent struct {
	Type string
	Data map[string]any
	Raw  string
}

func (r *rig) recorded(t *testing.T) []recordedEvent {
	t.Helper()
	rows, err := r.database.SQLDB(t).QueryContext(reading(t.Context()), `SELECT type, data::text FROM activity WHERE type LIKE 'datasource.%' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []recordedEvent
	for rows.Next() {
		var event recordedEvent
		if err := rows.Scan(&event.Type, &event.Raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(event.Raw), &event.Data); err != nil {
			t.Fatal(err)
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCreateUpdateAndDeleteAreRecordedWithoutCredentials(t *testing.T) {
	r, svc := newRig(t, testKey)
	secrets := Secrets{BearerToken: "token-abc-123", Headers: map[string]string{"X-Org": "org-secret-7"}}

	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("metrics", "https://prom.example.com:9090/prefix"), secrets); err != nil {
		t.Fatal(err)
	}
	moved := prometheus("metrics", "https://prom2.example.com")
	moved.Name = "Renamed"
	if _, err := svc.Update(reading(t.Context()), r.admin, "metrics", moved, SecretsChange{Set: Secrets{BearerToken: "token-new-456"}, Clear: []string{"header:X-Org"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(reading(t.Context()), r.admin, "metrics"); err != nil {
		t.Fatal(err)
	}

	events := r.recorded(t)
	if len(events) != 3 || events[0].Type != "datasource.created" || events[1].Type != "datasource.updated" || events[2].Type != "datasource.deleted" {
		t.Fatalf("events = %+v", events)
	}
	created := events[0].Data
	if created["uid"] != "metrics" || created["type"] != TypePrometheus || created["name"] != "Metrics metrics" || created["host"] != "prom.example.com:9090" {
		t.Fatalf("created = %v", created)
	}
	updated := events[1].Data
	changes, _ := updated["changes"].(map[string]any)
	name, _ := changes["name"].(map[string]any)
	host, _ := changes["host"].(map[string]any)
	if name["from"] != "Metrics metrics" || name["to"] != "Renamed" || host["from"] != "prom.example.com:9090" || host["to"] != "prom2.example.com" {
		t.Fatalf("changes = %v", changes)
	}
	if updated["credentials_changed"] != true || updated["address_changed"] != true {
		t.Fatalf("updated = %v", updated)
	}
	if deleted := events[2].Data; deleted["uid"] != "metrics" || deleted["name"] != "Renamed" || deleted["host"] != "prom2.example.com" {
		t.Fatalf("deleted = %v", deleted)
	}
	for _, event := range events {
		for _, secret := range []string{"token-abc-123", "org-secret-7", "token-new-456", "/prefix"} {
			if strings.Contains(event.Raw, secret) {
				t.Fatalf("%s holds %q: %s", event.Type, secret, event.Raw)
			}
		}
	}
}

func TestAnUpdateThatChangesNothingRecordsNothing(t *testing.T) {
	r, svc := newRig(t, testKey)
	def := prometheus("metrics", "https://prom.example.com")
	if _, err := svc.Create(reading(t.Context()), r.admin, def, Secrets{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(reading(t.Context()), r.admin, "metrics", def, SecretsChange{}); err != nil {
		t.Fatal(err)
	}
	if events := r.recorded(t); len(events) != 1 {
		t.Fatalf("events = %+v, want only the create", events)
	}
}

func TestARefusedCreateRecordsNothing(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, testKey)
	r.insert(t, testKey, prometheus("taken", server.URL), Secrets{})

	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("taken", "https://prom.example.com"), Secrets{}); err == nil {
		t.Fatal("a taken uid was created")
	}
	if events := r.recorded(t); len(events) != 0 {
		t.Fatalf("events = %+v", events)
	}
}

func allowed(t *testing.T, r *rig) []string {
	t.Helper()
	list, err := r.domains.Allowed(reading(t.Context()))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(list))
	for i, p := range list {
		out[i] = p.String()
	}
	return out
}

func TestMovingOrDeletingADatasourceReleasesTheDomainItAdded(t *testing.T) {
	r, svc := newRig(t, testKey)
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("metrics", "https://prom.example.com"), Secrets{}); err != nil {
		t.Fatal(err)
	}
	if got := allowed(t, r); len(got) != 1 || got[0] != "prom.example.com" {
		t.Fatalf("allowlist after create = %v", got)
	}

	if _, err := svc.Update(reading(t.Context()), r.admin, "metrics", prometheus("metrics", "https://other.example.com"), SecretsChange{}); err != nil {
		t.Fatal(err)
	}
	if got := allowed(t, r); len(got) != 1 || got[0] != "other.example.com" {
		t.Fatalf("allowlist after the move = %v", got)
	}

	if err := svc.Delete(reading(t.Context()), r.admin, "metrics"); err != nil {
		t.Fatal(err)
	}
	if got := allowed(t, r); len(got) != 0 {
		t.Fatalf("allowlist after delete = %v", got)
	}
}

func TestADomainAnotherDatasourceStillUsesStaysAllowed(t *testing.T) {
	r, svc := newRig(t, testKey)
	for _, uid := range []string{"first", "second"} {
		if _, err := svc.Create(reading(t.Context()), r.admin, prometheus(uid, "https://prom.example.com"), Secrets{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Delete(reading(t.Context()), r.admin, "first"); err != nil {
		t.Fatal(err)
	}
	if got := allowed(t, r); len(got) != 1 {
		t.Fatalf("allowlist = %v, want the domain the second datasource uses", got)
	}
	if err := svc.Delete(reading(t.Context()), r.admin, "second"); err != nil {
		t.Fatal(err)
	}
	if got := allowed(t, r); len(got) != 0 {
		t.Fatalf("allowlist = %v", got)
	}
}

func TestADomainAnAdminAllowedByHandSurvivesItsDatasource(t *testing.T) {
	r, svc := newRig(t, testKey)
	pattern, _ := domains.ParsePattern("prom.example.com")
	if _, err := r.domains.Add(reading(t.Context()), r.admin, pattern); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("metrics", "https://prom.example.com"), Secrets{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(reading(t.Context()), r.admin, "metrics"); err != nil {
		t.Fatal(err)
	}
	if got := allowed(t, r); len(got) != 1 {
		t.Fatalf("allowlist = %v, want the entry added by hand", got)
	}
}

func TestAClickHouseDatasourceKeepsItsHostAllowedWhileItExists(t *testing.T) {
	r, svc := newRig(t, testKey)
	def := Definition{UID: "warehouse", Type: TypeClickHouse, Name: "Warehouse", Address: "ch.example.com:9000", Username: "reader"}
	if _, err := svc.Create(reading(t.Context()), r.admin, def, Secrets{Password: "pw-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("metrics", "https://ch.example.com"), Secrets{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(reading(t.Context()), r.admin, "metrics"); err != nil {
		t.Fatal(err)
	}
	if got := allowed(t, r); len(got) != 1 {
		t.Fatalf("allowlist = %v, want the host the ClickHouse datasource uses", got)
	}
}
