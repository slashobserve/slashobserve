package agent

import (
	"errors"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	maxCutOffsInRow          = 2
	defaultSessionRenewBytes = 256 << 10

	compactTailTurns   = 4
	compactTailBytes   = 32 << 10
	compactResultBytes = 2 << 10

	pruneKeepTurns = 4
	pruneStepBytes = 64 << 10
)

var (
	ErrNoProgress      = errors.New(revisions.FailureMessage(revisions.FailureNoProgress, revisions.SourceObserve))
	ErrBudgetExhausted = errors.New(revisions.FailureMessage(revisions.FailureBudgetExhausted, revisions.SourceObserve))
)

func (r *runner) overBudget() bool {
	if r.agent.maxRunCost <= 0 {
		return false
	}
	cost, ok := r.agent.cost(r.out.Usage)
	return ok && cost >= r.agent.maxRunCost
}
