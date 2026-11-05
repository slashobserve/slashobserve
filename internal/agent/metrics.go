package agent

import (
	"context"
	"errors"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	outcomeSucceeded = "succeeded"
	outcomeLeaseLost = "lease_lost"
)

type Metrics interface {
	AgentRun(kind, outcome string)
}

func WithMetrics(r revisions.Reviser, m Metrics) revisions.Reviser {
	return meteredReviser{reviser: r, metrics: m}
}

type meteredReviser struct {
	reviser revisions.Reviser
	metrics Metrics
}

func (m meteredReviser) Revise(ctx context.Context, req revisions.Request) (revisions.Result, error) {
	result, err := m.reviser.Revise(ctx, req)
	kind := modeOf(taskFor(req))
	m.metrics.AgentRun(string(kind), runOutcome(result, err))
	return result, err
}

func runOutcome(result revisions.Result, err error) string {
	switch {
	case err == nil:
		return outcomeSucceeded
	case errors.Is(err, revisions.ErrLeaseLost):
		return outcomeLeaseLost
	case result.FailureCode != "":
		return string(result.FailureCode)
	}
	return string(codeInternal)
}
