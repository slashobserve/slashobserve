package revisions

import (
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type ReplyState string

const (
	ReplyNotOwed   ReplyState = "not_owed"
	ReplyOwed      ReplyState = "owed"
	ReplyReplied   ReplyState = "replied"
	ReplyAbandoned ReplyState = "abandoned"
)

var ReplyStates = lifecycle.Derived{Name: "thread reply", States: []lifecycle.DerivedState{
	{Name: string(ReplyNotOwed), When: "The run was not asked in a comment thread, so it owes no reply."},
	{Name: string(ReplyOwed), When: "The run was asked in a thread and its reply is not posted yet: the run has not finished, or the server is still trying, for 24 hours after it finished."},
	{Name: string(ReplyReplied), When: "The reply is posted in the thread."},
	{Name: string(ReplyAbandoned), When: "The reply could not be posted within 24 hours after the run finished and the server stopped trying."},
}}

type replyJSON struct {
	State ReplyState `json:"state"`
}

func replyStateOf(row dbsqlc.AgentRequest, now time.Time) ReplyState {
	switch {
	case !row.ThreadMessageID.Valid:
		return ReplyNotOwed
	case row.ThreadRepliedAt.Valid:
		return ReplyReplied
	case row.ThreadReplyAbandonedAt.Valid:
		return ReplyAbandoned
	case State(row.State).Terminal() && row.FinishedAt.Valid && !now.Before(row.FinishedAt.Time.Add(ReplyRetryFor)):
		return ReplyAbandoned
	}
	return ReplyOwed
}

type replyCursor struct {
	finishedAt pgtype.Timestamptz
	id         pgtype.UUID
}

type replyRounds struct {
	mu    sync.Mutex
	after replyCursor
}

func (r *replyRounds) start() replyCursor {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.after
}

func (r *replyRounds) visited(page []dbsqlc.AgentRequest, batch int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.after = nextReplyCursor(page, batch)
}

func nextReplyCursor(page []dbsqlc.AgentRequest, batch int) replyCursor {
	if len(page) < batch {
		return replyCursor{}
	}
	last := page[len(page)-1]
	return replyCursor{finishedAt: last.FinishedAt, id: last.ID}
}
