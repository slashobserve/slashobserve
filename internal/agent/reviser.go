package agent

import (
	"context"
	"errors"

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
	task := Task{Prompt: req.Prompt, RequestID: req.ID, DashboardID: req.DashboardID, PanelID: req.PanelID, Source: req.Source, Principal: req.RequestedBy}
	if r.steps != nil {
		task.Progress = func(s Step) {
			r.steps.RecordStep(context.WithoutCancel(ctx), req, revisions.Step{
				At: s.At, Phase: s.Phase, Tool: s.Tool, Status: s.Status, Message: s.Message, DurationMS: s.DurationMS,
			})
		}
	}
	out, err := r.agent.Run(ctx, task)
	result := revisions.Result{Version: out.Version}
	if out.Usage.Calls > 0 {
		result.Usage = &revisions.Usage{
			Provider:         out.Provider,
			Model:            out.Model,
			InputTokens:      out.Usage.InputTokens,
			OutputTokens:     out.Usage.OutputTokens,
			CacheReadTokens:  out.Usage.CacheReadTokens,
			CacheWriteTokens: out.Usage.CacheWriteTokens,
			ModelCalls:       out.Usage.Calls,
			RepairRounds:     out.RepairRounds,
		}
	}
	if err != nil {
		return result, storedError(err)
	}
	return result, nil
}

type fixedContextError struct {
	message  string
	sentinel error
}

func (e *fixedContextError) Error() string { return e.message }
func (e *fixedContextError) Unwrap() error { return e.sentinel }

// The returned error is stored on the request and shown to users, so its text is limited to the agent's own
// fixed messages; provider and tool error text never passes through.
func storedError(err error) error {
	message := storedMessage(err)
	for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, sentinel) {
			return &fixedContextError{message: message, sentinel: sentinel}
		}
	}
	return errors.New(message)
}

func storedMessage(err error) string {
	var safe *safeError
	switch {
	case errors.Is(err, ErrNoSave), errors.Is(err, ErrRepairExhausted), errors.Is(err, ErrTurnLimit), errors.Is(err, ErrRefused),
		errors.Is(err, ErrConflict), errors.Is(err, ErrInvalidTask), errors.Is(err, ErrTooLarge), errors.Is(err, llm.ErrNoFixture):
		return err.Error()
	case errors.Is(err, dashboards.ErrNotFound):
		return messageDashboardGone
	case errors.As(err, &safe):
		return safe.message
	case errors.Is(err, errModelCall):
		return messageModelCall
	case errors.Is(err, context.Canceled):
		return context.Canceled.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded.Error()
	}
	return messageRunFailed
}
