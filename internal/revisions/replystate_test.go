package revisions

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestReplyStateOf(t *testing.T) {
	now := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	at := func(ago time.Duration) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: now.Add(-ago), Valid: true}
	}
	asked := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	cases := []struct {
		name string
		row  dbsqlc.AgentRequest
		want ReplyState
	}{
		{"not asked in a thread", dbsqlc.AgentRequest{State: string(StateSucceeded), FinishedAt: at(48 * time.Hour)}, ReplyNotOwed},
		{"still running", dbsqlc.AgentRequest{State: string(StateRunning), ThreadMessageID: asked}, ReplyOwed},
		{"finished a minute ago, reply not posted", dbsqlc.AgentRequest{State: string(StateFailed), ThreadMessageID: asked, FinishedAt: at(time.Minute)}, ReplyOwed},
		{"finished just inside the window", dbsqlc.AgentRequest{State: string(StateSucceeded), ThreadMessageID: asked, FinishedAt: at(ReplyRetryFor - time.Second)}, ReplyOwed},
		{"replied", dbsqlc.AgentRequest{State: string(StateSucceeded), ThreadMessageID: asked, FinishedAt: at(time.Hour), ThreadRepliedAt: at(time.Hour)}, ReplyReplied},
		{"replied after it was given up", dbsqlc.AgentRequest{State: string(StateSucceeded), ThreadMessageID: asked, FinishedAt: at(72 * time.Hour), ThreadRepliedAt: at(time.Hour), ThreadReplyAbandonedAt: at(48 * time.Hour)}, ReplyReplied},
		{"the sweep gave up", dbsqlc.AgentRequest{State: string(StateCanceled), ThreadMessageID: asked, FinishedAt: at(30 * time.Hour), ThreadReplyAbandonedAt: at(6 * time.Hour)}, ReplyAbandoned},
		{"past the window before the sweep marked it", dbsqlc.AgentRequest{State: string(StateFailed), ThreadMessageID: asked, FinishedAt: at(ReplyRetryFor)}, ReplyAbandoned},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := replyStateOf(c.row, now)
			if got != c.want {
				t.Errorf("replyStateOf = %s, want %s", got, c.want)
			}
			if !ReplyStates.Has(string(got)) {
				t.Errorf("%s is not a declared reply state", got)
			}
		})
	}
}

func owedReplies(n int) []dbsqlc.AgentRequest {
	newest := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	rows := make([]dbsqlc.AgentRequest, n)
	for i := range rows {
		rows[i] = dbsqlc.AgentRequest{
			ID:         pgtype.UUID{Bytes: [16]byte{byte(i >> 8), byte(i)}, Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: newest.Add(-time.Duration(i) * time.Minute), Valid: true},
		}
	}
	return rows
}

func pageAfter(newestFirst []dbsqlc.AgentRequest, after replyCursor, batch int) []dbsqlc.AgentRequest {
	start := 0
	if after.finishedAt.Valid {
		for i, row := range newestFirst {
			if row.ID == after.id {
				start = i + 1
			}
		}
	}
	return newestFirst[start:min(start+batch, len(newestFirst))]
}

func TestReplySweepsReachEveryOwedReplyWhenTheNewestKeepFailing(t *testing.T) {
	const batch = 50
	owed := owedReplies(120)
	var rounds replyRounds
	visited := map[pgtype.UUID]int{}
	for sweep := range 4 {
		page := pageAfter(owed, rounds.start(), batch)
		for _, row := range page {
			visited[row.ID]++
		}
		rounds.visited(page, batch)
		if sweep == 2 && len(visited) != len(owed) {
			t.Fatalf("three sweeps of %d visited %d of %d owed replies", batch, len(visited), len(owed))
		}
	}
	if visited[owed[0].ID] != 2 {
		t.Errorf("the newest owed reply was visited %d times in four sweeps, want 2: the fourth sweep starts over", visited[owed[0].ID])
	}
}

func TestReplySweepStartsOverAfterAShortOrEmptyPage(t *testing.T) {
	full := owedReplies(3)
	if got := nextReplyCursor(full, 3); got.id != full[2].ID || got.finishedAt != full[2].FinishedAt {
		t.Errorf("after a full page the next sweep starts at %+v, want the last row", got)
	}
	if got := nextReplyCursor(full[:2], 3); got != (replyCursor{}) {
		t.Errorf("after a short page the next sweep starts at %+v, want the newest", got)
	}
	if got := nextReplyCursor(nil, 3); got != (replyCursor{}) {
		t.Errorf("after an empty page the next sweep starts at %+v, want the newest", got)
	}
}
