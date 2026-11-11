package runsession

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

const (
	maxTurnCalls   = 64
	maxTurnIDBytes = 128
)

type Turn struct {
	ID    string `json:"id"`
	Calls int    `json:"calls"`
	Saves int    `json:"saves"`
}

func (t *Turn) valid() *Turn {
	if t == nil || t.ID == "" || len(t.ID) > maxTurnIDBytes || t.Calls < 1 || t.Calls > maxTurnCalls || t.Saves < 0 || t.Saves > t.Calls {
		return nil
	}
	return t
}

func isSave(name string) bool {
	return tools.Phase(name) == tools.PhaseSave
}

func (t OpenTurn) joinedBy(turn *Turn) bool {
	return t.Open && turn != nil && t.ID != "" && t.ID == turn.ID
}

func (sess *session) beginTurn(c guard.Call, turn *Turn) {
	next := OpenTurn{Seq: sess.state.Turn.Seq + 1, Open: true, Calls: 1}
	saves := 0
	if isSave(c.Name) {
		saves = 1
	}
	if turn != nil {
		next.ID, next.Calls, saves = turn.ID, turn.Calls, turn.Saves
	}
	sess.state.Guard.BeginTurn(saves > 0)
	sess.state.Turn = next
}

func (sess *session) closeTurn(ctx context.Context) (nudge string, err error) {
	nudge, stop := sess.state.Guard.EndTurn()
	sess.state.Turn.Open = false
	if stop != nil {
		return nudge, sess.end(ctx, endSpec{failure: stop.Failure, step: stop.Step})
	}
	return nudge, nil
}

func (sess *session) complete(ctx context.Context, seq int) (notes []string, err error) {
	turn := &sess.state.Turn
	if !turn.Open || turn.Seq != seq {
		return nil, nil
	}
	if turn.Settled++; turn.Settled < turn.Calls {
		return nil, nil
	}
	nudge, err := sess.closeTurn(ctx)
	if nudge != "" {
		notes = append(notes, nudge)
	}
	return notes, err
}
