package activity

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestEveryActivityTypeRecordsACatalogChangeAction(t *testing.T) {
	for _, typ := range Types() {
		action, ok := AuditAction(typ)
		if !ok {
			t.Errorf("%s has no audit action", typ)
			continue
		}
		spec, ok := audit.Lookup(action)
		if !ok {
			t.Errorf("%s maps to %s, which is not in the audit catalog", typ, action)
			continue
		}
		if spec.Class != audit.ClassChange {
			t.Errorf("%s maps to %s of class %s, want change", typ, action, spec.Class)
		}
	}
	for typ := range auditActions {
		if !Known(typ) {
			t.Errorf("the audit mapping names %s, which is not an activity type", typ)
		}
	}
}

func TestARunFinishIsASuccessOnlyWhenTheRunSucceeded(t *testing.T) {
	for typ, want := range map[string]audit.Outcome{
		TypeRunSucceeded: "",
		TypeRunFailed:    audit.OutcomeFailed,
		TypeRunCanceled:  audit.OutcomeFailed,
	} {
		if got := auditActions[typ].outcome; got != want {
			t.Errorf("%s outcome = %q, want %q", typ, got, want)
		}
	}
}

func TestAnAppendAndItsAuditEventCommitAndRollBackTogether(t *testing.T) {
	r := newRig(t)
	dashboard := r.dashboardIn(t, pgtype.UUID{})
	errRollback := errors.New("roll back")
	entry := Entry{Type: TypeDashboardCreated, TargetID: dashboard, Owner: OnDashboard(dashboard), Actor: editorActor, Data: map[string]any{"title": "Checkout", "version": 1}}

	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		if _, err := Append(t.Context(), q, entry, nil); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("rolled back append: %v", err)
	}
	if n := countRows(t, r, "activity"); n != 0 {
		t.Fatalf("activity rows after rollback = %d, want 0", n)
	}
	if n := countRows(t, r, "audit_events"); n != 0 {
		t.Fatalf("audit events after rollback = %d, want 0", n)
	}

	r.append(t, entry)
	var action, targetID, targetName, actorID, dashboardID, initiator, data string
	err = r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT action, target_id, target_name, actor_id::text, dashboard_id::text, initiator, data::text FROM audit_events`).
		Scan(&action, &targetID, &targetName, &actorID, &dashboardID, &initiator, &data)
	if err != nil {
		t.Fatal(err)
	}
	if action != "dashboard.create" || targetID != dashboard.String() || targetName != "Checkout" || actorID != editorActor.ID || dashboardID != dashboard.String() || initiator != "person" {
		t.Fatalf("audit event = %s %s %q by %s on %s (%s)", action, targetID, targetName, actorID, dashboardID, initiator)
	}
	if data != `{"title": "Checkout", "version": 1}` {
		t.Fatalf("data = %s", data)
	}
}

func TestATerminalRunRecordsRunFinishWithItsResult(t *testing.T) {
	r := newRig(t)
	r.append(t, Entry{Type: TypeRunCanceled, TargetID: newTarget(), Actor: editorActor})
	var action, result string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT action, data->>'result' FROM audit_events`).Scan(&action, &result); err != nil {
		t.Fatal(err)
	}
	if action != "run.finish" || result != "canceled" {
		t.Fatalf("event = %s with result %q, want run.finish with canceled", action, result)
	}
}

func TestAuditOnlyDataReachesTheAuditEventAndNotTheActivityRow(t *testing.T) {
	r := newRig(t)
	r.append(t, Entry{Type: TypeRunSucceeded, TargetID: newTarget(), Actor: editorActor, Data: map[string]any{"intent": "apply"}, AuditOnly: map[string]any{"model": "m", "input_tokens": 120, "output_tokens": 30}})
	var model, in, out, intent, activityModel string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT data->>'model', data->>'input_tokens', data->>'output_tokens', data->>'intent', (SELECT coalesce(data->>'model', 'none') FROM activity) FROM audit_events`).
		Scan(&model, &in, &out, &intent, &activityModel); err != nil {
		t.Fatal(err)
	}
	if model != "m" || in != "120" || out != "30" || intent != "apply" {
		t.Fatalf("audit data = model %q, input %q, output %q, intent %q, want m, 120, 30, apply", model, in, out, intent)
	}
	if activityModel != "none" {
		t.Fatalf("the activity row holds model %q, want none", activityModel)
	}
}

func TestARunEventRecordsTheAgentThatTookTheRun(t *testing.T) {
	r := newRig(t)
	grant := newTarget()
	r.append(t, Entry{Type: TypeRunStarted, TargetID: newTarget(), Actor: editorActor, Agent: audit.Agent{ID: uuid.NullUUID{UUID: grant.Bytes, Valid: true}, Name: "Claude Code"}})
	var agentID, agentName string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT agent_id::text, agent_name FROM audit_events`).Scan(&agentID, &agentName); err != nil {
		t.Fatal(err)
	}
	if agentID != grant.String() || agentName != "Claude Code" {
		t.Fatalf("agent = %s %q, want the grant named Claude Code", agentID, agentName)
	}
}

func countRows(t *testing.T, r rig, table string) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
