package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/llm"
)

const (
	maxParallelTools = 4
	maxIssueCodes    = 3

	phaseDiscover = "discover"
	phaseLint     = "lint"
	phaseQuery    = "query"
	phaseSave     = "save"
	phaseRepair   = "repair"

	statusOK    = "ok"
	statusError = "error"

	toolCreateDashboard = "create_dashboard"
	toolUpdateDashboard = "update_dashboard"

	codeInvalidRequest  = "invalid_request"
	codeVersionConflict = "version_conflict"
	codeNotFound        = "not_found"

	internalStep = "internal error"

	messageTurnLimit = "turn limit reached; only a save can run"
)

type safeError struct {
	message string
	err     error
}

func (e *safeError) Error() string { return e.message }
func (e *safeError) Unwrap() error { return e.err }

type savedVersion struct {
	dashboardID string
	version     int
	replayed    bool
}

type turnOutcome struct {
	saved      *savedVersion
	conflict   bool
	notFound   bool
	rejected   bool
	issueCodes []string
}

type runState struct {
	tools         ToolSet
	progress      func(Step)
	logger        *slog.Logger
	mode          tools.Mode
	parentVersion int
	rejections    int
	repairs       int
}

type callSlot struct {
	name     string
	phase    string
	result   tools.Result
	content  []byte
	duration time.Duration
	end      time.Time
	err      error
}

func isSave(name string) bool {
	return name == toolCreateDashboard || name == toolUpdateDashboard
}

func phaseFor(name string, repairing bool) string {
	switch name {
	case "lint_query":
		return phaseLint
	case "query_range", "query_instant":
		return phaseQuery
	case toolCreateDashboard, toolUpdateDashboard:
		if repairing {
			return phaseRepair
		}
		return phaseSave
	}
	return phaseDiscover
}

func (s *runState) executeTurn(ctx context.Context, calls []llm.ToolCall, lastTurn bool) ([]llm.ToolResult, turnOutcome, error) {
	repairing := s.rejections > 0
	slots := make([]callSlot, len(calls))
	var reads []int
	saveIndex := -1
	var extraSaves []int
	for i, c := range calls {
		switch {
		case !isSave(c.Name):
			reads = append(reads, i)
		case saveIndex < 0:
			saveIndex = i
		default:
			extraSaves = append(extraSaves, i)
		}
	}

	sem := make(chan struct{}, maxParallelTools)
	var wg sync.WaitGroup
	for _, i := range reads {
		if lastTurn {
			slots[i] = rejectCall(calls[i], repairing, messageTurnLimit)
			continue
		}
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			slots[i] = s.call(ctx, calls[i], repairing)
		})
	}
	wg.Wait()
	for _, i := range reads {
		s.report(slots[i])
		if slots[i].err != nil {
			return nil, turnOutcome{}, s.internal(ctx, slots[i])
		}
	}

	var outcome turnOutcome
	if saveIndex >= 0 {
		slot := &slots[saveIndex]
		*slot = s.call(ctx, calls[saveIndex], repairing)
		if slot.phase == phaseRepair {
			s.repairs++
		}
		s.report(*slot)
		if slot.err != nil {
			return nil, turnOutcome{}, s.internal(ctx, *slot)
		}
		var err error
		if outcome, err = s.classifySave(ctx, *slot); err != nil {
			return nil, turnOutcome{}, err
		}
	}
	for _, i := range extraSaves {
		slots[i] = rejectCall(calls[i], repairing, "only one save per turn")
		s.report(slots[i])
	}

	results := make([]llm.ToolResult, len(calls))
	for i, c := range calls {
		results[i] = llm.ToolResult{CallID: c.ID, Content: string(slots[i].content), IsError: slots[i].result.IsError}
	}
	return results, outcome, nil
}

func (s *runState) call(ctx context.Context, c llm.ToolCall, repairing bool) callSlot {
	slot := callSlot{name: c.Name, phase: phaseFor(c.Name, repairing)}
	start := time.Now()
	slot.result, slot.err = s.tools.Call(ctx, c.Name, c.Input)
	slot.end = time.Now()
	slot.duration = slot.end.Sub(start)
	if slot.err != nil {
		return slot
	}
	slot.content, slot.err = json.Marshal(slot.result.Content)
	return slot
}

func rejectCall(c llm.ToolCall, repairing bool, reason string) callSlot {
	content, _ := json.Marshal(map[string]string{"code": codeInvalidRequest, "message": reason})
	return callSlot{
		name:    c.Name,
		phase:   phaseFor(c.Name, repairing),
		content: content,
		result:  tools.Result{IsError: true, Step: "Rejected " + c.Name + " input: " + reason},
	}
}

func (s *runState) report(slot callSlot) {
	if s.progress == nil {
		return
	}
	at := slot.end
	if at.IsZero() {
		at = time.Now()
	}
	step := Step{
		At:         at.UTC(),
		Phase:      slot.phase,
		Tool:       slot.name,
		Status:     statusOK,
		Message:    slot.result.Step,
		DurationMS: slot.duration.Milliseconds(),
	}
	if slot.result.IsError {
		step.Status = statusError
	}
	if slot.err != nil {
		step.Status = statusError
		step.Message = internalStep
	} else if step.Message == "" {
		step.Message = slot.name
	}
	s.progress(step)
}

func (s *runState) internal(ctx context.Context, slot callSlot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.logger.ErrorContext(ctx, "agent tool failed", slog.String("tool", slot.name), slog.String("error", slot.err.Error()))
	return &safeError{message: "the " + slot.name + " tool failed with an internal error", err: slot.err}
}

type saveBody struct {
	Code          string `json:"code"`
	DashboardID   string `json:"dashboard_id"`
	Version       int    `json:"version"`
	LatestVersion int    `json:"latest_version"`
	Replayed      *bool  `json:"replayed"`
	Issues        []struct {
		Code string `json:"code"`
	} `json:"issues"`
}

func (s *runState) classifySave(ctx context.Context, slot callSlot) (turnOutcome, error) {
	var body saveBody
	if err := json.Unmarshal(slot.content, &body); err != nil {
		return turnOutcome{}, s.malformed(ctx, slot)
	}
	if !slot.result.IsError {
		if body.DashboardID == "" || body.Version < 1 {
			return turnOutcome{}, s.malformed(ctx, slot)
		}
		return turnOutcome{saved: &savedVersion{
			dashboardID: body.DashboardID,
			version:     body.Version,
			replayed:    body.Replayed != nil && *body.Replayed,
		}}, nil
	}
	switch {
	case body.Code == "":
		return turnOutcome{}, s.malformed(ctx, slot)
	case body.Code == codeVersionConflict && s.mode == tools.ModeRevise && body.LatestVersion != s.parentVersion:
		return turnOutcome{conflict: true}, nil
	case body.Code == codeNotFound && s.mode == tools.ModeRevise:
		return turnOutcome{notFound: true}, nil
	}
	s.rejections++
	return turnOutcome{rejected: true, issueCodes: body.issueCodes()}, nil
}

func (b saveBody) issueCodes() []string {
	var codes []string
	for _, issue := range b.Issues {
		if llm.IsPlainToken(issue.Code) && !slices.Contains(codes, issue.Code) {
			codes = append(codes, issue.Code)
		}
		if len(codes) == maxIssueCodes {
			break
		}
	}
	if len(codes) == 0 && llm.IsPlainToken(b.Code) {
		codes = []string{b.Code}
	}
	return codes
}

func (s *runState) malformed(ctx context.Context, slot callSlot) error {
	s.logger.ErrorContext(ctx, "agent save result is malformed", slog.String("tool", slot.name))
	return &safeError{
		message: "the " + slot.name + " tool returned an unreadable result",
		err:     errors.New("save result is malformed"),
	}
}
