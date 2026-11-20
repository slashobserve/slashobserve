package db_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	oldServerTarget = "77777777-7777-4777-8777-777777777777"
	newServerTarget = "88888888-8888-4888-8888-888888888888"
)

func TestTheBackfillKeepsOnlyTheDataKeysTheCatalogAllows(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, auditEventsVersion-1)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, origin, data)
VALUES ('dashboard.version_created', 'dashboard', $1, 'user', gen_random_uuid(), 'Ada', 'web', '{"title": "Checkout", "version": 2, "prompt": "secret words"}')`, oldServerTarget)
	if err := d.MigrateUpTo(ctx, auditEventsVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	var raw []byte
	if err := d.SQLDB(t).QueryRowContext(ctx, `SELECT data FROM audit_events WHERE target_id = $1`, oldServerTarget).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 2 || data["title"] != "Checkout" || data["version"] != float64(2) {
		t.Fatalf("backfilled data = %v, want only title and version", data)
	}
}

func TestTheBackfillAllowlistMatchesTheCatalog(t *testing.T) {
	d := newMigratedTo(t, auditEventsVersion)
	rows, err := d.SQLDB(t).QueryContext(t.Context(), `SELECT type, action, category, keys FROM audit_activity_actions`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[string]bool{}
	types := pgtype.NewMap()
	for rows.Next() {
		var typ, action, category string
		var keys []string
		if err := rows.Scan(&typ, &action, &category, types.SQLScanner(&keys)); err != nil {
			t.Fatal(err)
		}
		seen[typ] = true
		want, ok := activity.AuditAction(typ)
		spec, _ := audit.Lookup(want)
		if !ok || audit.Action(action) != want || audit.Category(category) != spec.Category {
			t.Errorf("%s maps to %s (%s) in the migration, want %s (%s)", typ, action, category, want, spec.Category)
		}
		if got := slices.Sorted(slices.Values(keys)); !slices.Equal(got, slices.Sorted(slices.Values(spec.Data))) {
			t.Errorf("%s backfills keys %v, the catalog allows %v", typ, got, spec.Data)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, typ := range activity.Types() {
		if !seen[typ] {
			t.Errorf("activity type %s has no row in audit_activity_actions", typ)
		}
	}
}

func TestCatchUpRecordsWhatAnOldServerWroteAfterTheBackfill(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, auditEventsVersion)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, origin, data)
VALUES ('webhook.created', 'webhook', $1, 'user', gen_random_uuid(), 'Ada', 'web', '{"name": "hook", "secret": "s3cret"}')`, oldServerTarget)
	by := actor.Of(auth.Principal{Kind: auth.KindUser, ID: uuid.NewString(), Name: "Bo"}, origin.Origin{Kind: origin.Web}, "")
	err := d.DB.InTx(ctx, func(q *dbsqlc.Queries) error {
		return activity.AppendAdmin(ctx, q, by, activity.TypeWebhookCreated, pgtype.UUID{Bytes: uuid.MustParse(newServerTarget), Valid: true}, map[string]any{"name": "mirrored"})
	})
	if err != nil {
		t.Fatal(err)
	}
	job := audit.New(audit.Config{DB: d.DB}).PartitionJob()
	for range 2 {
		if err := job(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var oldEvents, newEvents int
	var data string
	err = d.SQLDB(t).QueryRowContext(ctx, `SELECT
  (SELECT count(*) FROM audit_events WHERE target_id = $1),
  (SELECT count(*) FROM audit_events WHERE target_id = $2),
  (SELECT coalesce(max(data::text), '') FROM audit_events WHERE target_id = $1)`, oldServerTarget, newServerTarget).Scan(&oldEvents, &newEvents, &data)
	if err != nil {
		t.Fatal(err)
	}
	if oldEvents != 1 || newEvents != 1 || data != `{"name": "hook"}` {
		t.Fatalf("events for the old server's row = %d (data %s), for the new server's row = %d; want one each, without the undeclared key", oldEvents, data, newEvents)
	}
}

func TestCatchUpStopsAfterItsWindow(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, auditEventsVersion)
	d.Exec(t, `UPDATE audit_backfill SET migrated_at = now() - interval '30 days'`)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, origin, data)
VALUES ('webhook.created', 'webhook', $1, 'user', gen_random_uuid(), 'Ada', 'web', '{"name": "hook"}')`, oldServerTarget)
	if err := audit.New(audit.Config{DB: d.DB}).PartitionJob()(ctx); err != nil {
		t.Fatal(err)
	}
	var events int
	if err := d.SQLDB(t).QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE target_id = $1`, oldServerTarget).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Fatal("catch-up ran 30 days after the migration")
	}
}
