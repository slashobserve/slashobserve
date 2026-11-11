package claims

import (
	"context"
	"errors"
	"math"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	outcomeSucceeded = "succeeded"
	outcomeFailed    = "failed"
	silentFailure    = "the agent did not say why"
)

var ErrInvalidOutcome = errors.New("outcome must be succeeded or failed")

type FinishUsage struct {
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd"`
}

type FinishInput struct {
	RunID   string       `json:"run_id"`
	Attempt int          `json:"attempt"`
	Outcome string       `json:"outcome"`
	Message string       `json:"message"`
	Usage   *FinishUsage `json:"usage"`
}

type FinishAnswer struct {
	State         string  `json:"state"`
	ResultKind    string  `json:"result_kind"`
	ResultVersion *int    `json:"result_version"`
	FailureCode   *string `json:"failure_code"`
}

func (s *Service) Finish(ctx context.Context, c Claimer, in FinishInput) (FinishAnswer, error) {
	if in.Outcome != outcomeSucceeded && in.Outcome != outcomeFailed {
		return FinishAnswer{}, ErrInvalidOutcome
	}
	row, err := s.claimedRun(ctx, c, in.RunID, in.Attempt)
	if err != nil {
		return FinishAnswer{}, err
	}
	if revisions.State(row.State).Terminal() {
		return answerOf(row), nil
	}
	ended, err := s.runs.End(ctx, row, endingOf(in))
	if errors.Is(err, revisions.ErrLeaseLost) {
		return FinishAnswer{}, ErrNotYourRun
	}
	if err != nil {
		return FinishAnswer{}, err
	}
	s.dropTokens(ctx, row)
	return answerOf(ended), nil
}

func endingOf(in FinishInput) revisions.Ending {
	ending := revisions.Ending{Usage: cleanUsage(in.Usage)}
	if in.Outcome == outcomeFailed {
		ending.Failure = revisions.FailureAgentFailed
		ending.Message = cleanText(in.Message, maxMessageRunes)
		if ending.Message == "" {
			ending.Message = silentFailure
		}
	}
	return ending
}

func answerOf(row dbsqlc.AgentRequest) FinishAnswer {
	answer := FinishAnswer{State: row.State, ResultKind: row.ResultKind}
	if row.ResultVersion.Valid {
		version := int(row.ResultVersion.Int32)
		answer.ResultVersion = &version
	}
	if row.FailureCode.Valid {
		answer.FailureCode = &row.FailureCode.String
	}
	return answer
}

func cleanUsage(u *FinishUsage) *revisions.Usage {
	if u == nil {
		return nil
	}
	usage := &revisions.Usage{
		Provider:     cleanText(u.Provider, maxUsageTextRunes),
		Model:        cleanText(u.Model, maxUsageTextRunes),
		InputTokens:  max(u.InputTokens, 0),
		OutputTokens: max(u.OutputTokens, 0),
	}
	if cost := u.CostUSD; cost != nil && !math.IsNaN(*cost) && *cost >= 0 && *cost <= maxCostUSD {
		kept := *cost
		usage.CostUSD = &kept
	}
	return usage
}
