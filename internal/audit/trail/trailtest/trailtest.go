package trailtest

import (
	"cmp"
	"encoding/binary"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

type Event struct {
	At         time.Time
	Action     string
	Outcome    string
	Actor      string
	ActorID    string
	Origin     string
	Initiator  string
	Request    string
	Run        string
	TargetKind string
	TargetName string
	Status     int
}

func IDAt(at time.Time) uuid.UUID {
	id := uuid.New()
	var ms [8]byte
	binary.BigEndian.PutUint64(ms[:], uint64(at.UnixMilli()))
	copy(id[0:6], ms[2:8])
	id[6] = id[6]&0x0f | 0x70
	id[8] = id[8]&0x3f | 0x80
	return id
}

func Insert(t testing.TB, d *dbtest.Database, events ...Event) []string {
	t.Helper()
	ids := make([]string, 0, len(events))
	for _, e := range events {
		id := IDAt(e.At)
		ids = append(ids, id.String())
		d.Exec(t, `INSERT INTO audit_events (id, occurred_at, category, action, outcome, status, actor_kind, actor_name, initiator, origin,
request_id, run_id, target_kind, target_name, actor_id, server_id, server_version)
VALUES ($1, $2, 'content', $3, $4, $5, 'user', $6, $7, $8, nullif($9, '')::uuid, nullif($10, '')::uuid, nullif($11, ''), nullif($12, ''), nullif($13, '')::uuid, 'test', 'test')`,
			id, e.At, cmp.Or(e.Action, "dashboard.update"), cmp.Or(e.Outcome, "success"), cmp.Or(e.Status, 200),
			cmp.Or(e.Actor, "Ada"), cmp.Or(e.Initiator, "person"), cmp.Or(e.Origin, "web"), e.Request, e.Run, e.TargetKind, e.TargetName, e.ActorID)
	}
	return ids
}
