package audit

import (
	"encoding/json"
	"net/netip"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func loginFailure(actor byte, providerError string) dbsqlc.InsertAuditEventParams {
	ip := netip.MustParseAddr("203.0.113.7")
	data := []byte(`{}`)
	if providerError != "" {
		data = []byte(`{"provider_error":"` + providerError + `"}`)
	}
	return dbsqlc.InsertAuditEventParams{
		Action: "auth.login_failed", Outcome: "denied", Reason: text("access_denied"), ActorKind: "user",
		ActorID:      pgtype.UUID{Bytes: [16]byte{actor}, Valid: true},
		ActorName:    text("Ada"),
		AuthMethod:   text("session"),
		CredentialID: text("session-1"),
		ClientIp:     &ip,
		UserAgent:    text("curl/8"),
		RequestID:    pgtype.UUID{Bytes: [16]byte{actor, 1}, Valid: true},
		TraceID:      text("trace"),
		TargetKind:   text("user"),
		TargetID:     text("someone"),
		TargetName:   text("Someone"),
		Origin:       "web",
		Data:         data,
	}
}

func dataOf(t *testing.T, row dbsqlc.InsertAuditEventParams) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal(row.Data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTheOverflowBucketBelongsToNoOneActorOrTarget(t *testing.T) {
	f := newFolds(time.Minute, 1)
	now := time.Now()
	var rows []dbsqlc.InsertAuditEventParams
	for actor := byte(1); actor <= 4; actor++ {
		if first, enqueue := f.absorb(loginFailure(actor, ""), now); enqueue {
			rows = append(rows, first)
		}
	}
	rows = append(rows, f.due(now.Add(time.Minute))...)
	var overflow *dbsqlc.InsertAuditEventParams
	var counted float64
	for _, row := range rows {
		if dataOf(t, row)["overflow"] == true {
			overflow = &row
			counted += dataOf(t, row)["count"].(float64)
		}
	}
	if overflow == nil {
		t.Fatal("no row is marked as the overflow bucket")
	}
	if counted != 3 {
		t.Fatalf("overflow count over its rows = %v, want 3", counted)
	}
	if overflow.ActorID.Valid || overflow.ActorName.Valid || overflow.TargetKind.Valid || overflow.TargetID.Valid || overflow.TargetName.Valid ||
		overflow.ClientIp != nil || overflow.UserAgent.Valid || overflow.RequestID.Valid || overflow.CredentialID.Valid || overflow.AuthMethod.Valid || overflow.TraceID.Valid {
		t.Fatalf("the overflow row keeps the first overflow event's identity: %+v", *overflow)
	}
	if overflow.ActorKind != "anonymous" || overflow.Action != "auth.login_failed" || !overflow.Reason.Valid || overflow.Reason.String != "access_denied" {
		t.Fatalf("overflow row = %+v", *overflow)
	}
}

func TestFoldingKeepsProviderErrorsApart(t *testing.T) {
	f := newFolds(time.Minute, 10)
	now := time.Now()
	var rows []dbsqlc.InsertAuditEventParams
	for _, providerError := range []string{"access_denied", "temporarily_unavailable", "access_denied"} {
		if first, enqueue := f.absorb(loginFailure(1, providerError), now); enqueue {
			rows = append(rows, first)
		}
	}
	rows = append(rows, f.due(now.Add(time.Minute))...)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want a first row per provider error and a later row for the repeated one", len(rows))
	}
	seen := map[string]float64{}
	for _, row := range rows {
		data := dataOf(t, row)
		seen[data["provider_error"].(string)] += data["count"].(float64)
	}
	if seen["access_denied"] != 2 || seen["temporarily_unavailable"] != 1 {
		t.Fatalf("counts = %v", seen)
	}
}

func TestAFailedWriteCountsEveryFoldedEventAsLost(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour, timeout: 200 * time.Millisecond, settle: 200 * time.Millisecond}, 0)
	for range 5 {
		if err := s.recorder.RecordNow(t.Context(), credentialRejected("unknown_credential", "sok_abcdefgh")); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := s.db.SQLDB(t).BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(t.Context(), "LOCK TABLE audit_events IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	s.run(t)()
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 5 {
		t.Fatalf("dropped %s = %d, want the 5 refusals the folded row stood for", DroppedWriteFailed, got)
	}
}
