package runsession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type prior struct {
	Agent *tools.AgentState `json:"agent_state"`
}

func priorSession(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID, attempt int32) (prior, error) {
	raw, err := q.GetPriorRunSession(ctx, dbsqlc.GetPriorRunSessionParams{AgentRequestID: id, Attempt: attempt})
	if errors.Is(err, pgx.ErrNoRows) {
		return prior{}, nil
	}
	if err != nil {
		return prior{}, fmt.Errorf("get the prior run session: %w", err)
	}
	var p prior
	if err := json.Unmarshal(raw, &p); err != nil {
		return prior{}, fmt.Errorf("decode the prior run session: %w", err)
	}
	return p, nil
}

func (p prior) carried() *tools.AgentState {
	if p.Agent == nil || isNull(p.Agent.Carry) {
		return nil
	}
	return &tools.AgentState{Carry: p.Agent.Carry}
}

func isNull(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}
