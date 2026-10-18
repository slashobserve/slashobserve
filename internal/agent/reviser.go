package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	messageModelCall     = "model call failed"
	messageRunFailed     = "the agent run failed"
	messageDashboardGone = "the dashboard no longer exists"
)

type Reviser struct {
	agent *Agent
	steps revisions.StepRecorder
}

func NewReviser(a *Agent, steps revisions.StepRecorder) *Reviser {
	return &Reviser{agent: a, steps: steps}
}

func (r *Reviser) Revise(ctx context.Context, req revisions.Request) (revisions.Result, error) {
	task := Task{Prompt: req.Prompt, RequestID: req.ID, Principal: req.RequestedBy}
	if req.Source != revisions.SourceObserve {
		task.DashboardID = req.DashboardID
		task.PanelID = req.PanelID
	}
	if r.steps != nil {
		task.Progress = func(s Step) {
			r.steps.RecordStep(ctx, req, revisions.Step{
				At: s.At, Attempt: req.Attempt, Phase: s.Phase, Tool: s.Tool, Status: s.Status, Message: s.Message, DurationMS: s.DurationMS,
			})
		}
	}
	out, err := r.agent.Run(ctx, task)
	result := revisions.Result{
		Version:     out.Version,
		DashboardID: out.DashboardID,
		Usage: &revisions.Usage{
			Provider:         out.Provider,
			Model:            out.Model,
			InputTokens:      out.Usage.InputTokens,
			OutputTokens:     out.Usage.OutputTokens,
			CacheReadTokens:  out.Usage.CacheReadTokens,
			CacheWriteTokens: out.Usage.CacheWriteTokens,
			ModelCalls:       out.Usage.Calls,
			RepairRounds:     out.RepairRounds,
		},
	}
	if err != nil {
		return result, storedError(err)
	}
	return result, nil
}

// The returned error is stored on the request and shown to users, so its text is limited to the agent's own
// fixed messages; provider and tool error text never passes through.
func storedError(err error) error {
	var safe *safeError
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, ErrNoSave), errors.Is(err, ErrRepairExhausted), errors.Is(err, ErrTurnLimit), errors.Is(err, ErrRefused),
		errors.Is(err, ErrConflict), errors.Is(err, ErrInvalidTask), errors.Is(err, ErrTooLarge), errors.Is(err, llm.ErrNoFixture):
		return errors.New(err.Error())
	case errors.Is(err, dashboards.ErrNotFound):
		return errors.New(messageDashboardGone)
	case errors.As(err, &safe):
		return errors.New(safe.message)
	case strings.HasPrefix(err.Error(), messageModelCall):
		return errors.New(messageModelCall)
	}
	return errors.New(messageRunFailed)
}
