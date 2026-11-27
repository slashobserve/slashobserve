package cli

import (
	"fmt"
	"slices"
)

const (
	runSucceeded = "succeeded"
	runCanceled  = "canceled"
)

type runStateView struct {
	word    string
	settled bool
}

var runStates = map[string]runStateView{
	"queued":     {word: "Queued"},
	"running":    {word: "Running"},
	"canceling":  {word: "Stopping"},
	runSucceeded: {word: "Done", settled: true},
	"failed":     {word: "Failed", settled: true},
	runCanceled:  {word: "Canceled", settled: true},
}

func requestStateWord(state string) string {
	if view, ok := runStates[state]; ok {
		return view.word
	}
	return text(state)
}

func runInFlight(state string) bool {
	view, known := runStates[state]
	return known && !view.settled
}

func stateOrLegacyStatus(state, status string) string {
	if state != "" {
		return state
	}
	return status
}

func (r agentRequest) state() string {
	if _, known := runStates[r.State]; known {
		return r.State
	}
	return r.Status
}

func (r agentRequest) nextStep() string {
	allowed := func(action string) bool { return slices.Contains(r.AllowedActions, action) }
	switch {
	case allowed("continue") && allowed("keep_build"):
		return fmt.Sprintf("Save it in the browser, or continue it: observe continue %s", text(r.ID))
	case allowed("continue"):
		return fmt.Sprintf("Continue it: observe continue %s", text(r.ID))
	case allowed("retry") && r.state() != runSucceeded:
		return fmt.Sprintf("Run it again: observe retry %s", text(r.ID))
	}
	return ""
}

func (r agentRequest) notes() []string {
	var notes []string
	if r.QueuePosition != nil {
		notes = append(notes, fmt.Sprintf("Queued, position %d", *r.QueuePosition))
	}
	if r.ContinuedBy != nil {
		notes = append(notes, fmt.Sprintf("Continued by %s", text(*r.ContinuedBy)))
	}
	if r.RetriedBy != nil {
		notes = append(notes, fmt.Sprintf("Retried by %s", text(*r.RetriedBy)))
	}
	if step := r.nextStep(); step != "" {
		notes = append(notes, step)
	}
	return notes
}

func (r agentRequest) failure() error {
	reason := "the agent gave no reason"
	if r.Error != nil && *r.Error != "" {
		reason = text(*r.Error)
	}
	ending := "failed"
	if r.state() == runCanceled {
		ending = runCanceled
	}
	message := fmt.Sprintf("observe %s: %s", ending, reason)
	if step := r.nextStep(); step != "" {
		message += "\n" + step
	}
	return &runEnded{id: r.ID, state: r.state(), message: message}
}

type runEnded struct {
	id      string
	state   string
	message string
}

func (e *runEnded) Error() string { return e.message }

type interruptedWait struct{ id string }

func (e interruptedWait) Error() string {
	return "still running on the server: observe status " + e.id
}

func (e interruptedWait) Unwrap() error { return errInterrupted }
