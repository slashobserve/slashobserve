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
	f.absorb(loginFailure(1, ""), now)
	f.absorb(loginFailure(2, ""), now)
	f.absorb(loginFailure(3, ""), now)
	var overflow *dbsqlc.InsertAuditEventParams
	for _, row := range f.due(now.Add(time.Minute)) {
		if dataOf(t, row)["overflow"] == true {
			overflow = &row
		}
	}
	if overflow == nil {
		t.Fatal("no row is marked as the overflow bucket")
	}
	if got := dataOf(t, *overflow)["count"]; got != float64(2) {
		t.Fatalf("overflow count = %v, want 2", got)
	}
	if overflow.ActorID.Valid || overflow.ActorName.Valid || overflow.TargetKind.Valid || overflow.TargetID.Valid || overflow.TargetName.Valid ||
		overflow.ClientIp != nil || overflow.UserAgent.Valid || overflow.RequestID.Valid || overflow.CredentialID.Valid || overflow.AuthMethod.Valid || overflow.TraceID.Valid {
		t.Fatalf("the overflow row keeps the first overflow event's identity: %+v", *overflow)
	}
	if overflow.ActorKind != "anonymous" || overflow.Action != "auth.login_failed" || !overflow.Reason.Valid || overflow.Reason.String != "access_denied" {
		t.Fatalf("overflow row = %+v", *overflow)
	}
}
