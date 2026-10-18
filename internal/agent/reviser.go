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
}

func NewReviser(a *Agent) *Reviser {
	return &Reviser{agent: a}
}

func (r *Reviser) Revise(ctx context.Context, req revisions.Request) (revisions.Result, error) {
	out, err := r.agent.Run(ctx, Task{
		Prompt:      req.Prompt,
		DashboardID: req.DashboardID,
		PanelID:     req.PanelID,
		RequestID:   req.ID,
		Principal:   req.RequestedBy,
	})
	if err != nil {
		return revisions.Result{}, storedError(err)
	}
	return revisions.Result{Version: out.Version}, nil
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
