package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	maxParallelTools = 4

	phaseSave = tools.PhaseSave

	statusOK    = "ok"
	statusError = "error"

	toolUpdateDashboard = "update_dashboard"
	toolUpdatePanels    = "update_panels"
	toolImportGrafana   = "import_grafana_dashboard"

	internalStep     = "internal error"
	stopTool         = "agent"
	unknownToolLabel = "unknown_tool"
)

const (
	codeToolFailed      = revisions.FailureToolFailed
	codeMalformedResult = revisions.FailureMalformedResult
	codeLoadFailed      = revisions.FailureLoadFailed
	codeContextFailed   = revisions.FailureContextFailed
	codeInternal        = revisions.FailureInternal
)

type safeError struct {
	message string
	code    revisions.FailureCode
	err     error
}

func (e *safeError) Error() string { return e.message }
func (e *safeError) Unwrap() error { return e.err }

type runState struct {
	tools         ToolSet
	known         map[string]bool
	progress      func(Step)
	logger        *slog.Logger
	logInputs     bool
	guard         guard.State
	replays       map[string][]byte
	toolTime      time.Duration
	maxSaveOutput int64
	cutOffs       int
	modelCalls    int
	pendingNudged bool
	findingNudged bool
	answerNudged  bool
	sessionBytes  int
	digest        digest
}

type callSlot struct {
	name     string
	phase    string
	result   tools.Result
	content  []byte
	duration time.Duration
	end      time.Time
	err      error
	refused  bool
}

func toolNames(list []tools.Tool) map[string]bool {
	names := make(map[string]bool, len(list))
	for _, t := range list {
		names[t.Name] = true
	}
	return names
}

func (s *runState) label(name string) string {
	if s.known[name] {
		return name
	}
	return unknownToolLabel
}

func isSave(name string) bool {
	return tools.Phase(name) == tools.PhaseSave
}

func containsSave(calls []llm.ToolCall) bool {
	return slices.ContainsFunc(calls, func(c llm.ToolCall) bool { return isSave(c.Name) })
}

func (s *runState) noteSavingTurn(turn llm.Turn) {
	if containsSave(turn.ToolCalls) {
		s.maxSaveOutput = max(s.maxSaveOutput, turn.Usage.OutputTokens)
	}
}

func withNote(content []byte, note string) []byte {
	return append(slices.Clone(content), "\n\n<note>"+note+"</note>"...)
}

func (s *runState) restart() {
	s.cutOffs, s.pendingNudged = 0, false
	s.replays = nil
	s.guard.Restart()
}

func appendNudge(results []llm.ToolResult, note string) {
	if note == "" || len(results) == 0 {
		return
	}
	last := &results[len(results)-1]
	last.Content = string(withNote([]byte(last.Content), note))
}

func (s *runState) callOf(c llm.ToolCall) guard.Call {
	return guard.Call{Name: c.Name, Label: s.label(c.Name), Input: c.Input}
}

func (s *runState) executeTurn(ctx context.Context, calls []llm.ToolCall) ([]llm.ToolResult, guard.Outcome, error) {
	start := time.Now()
	defer func() { s.toolTime += time.Since(start) }()
	s.guard.BeginTurn(containsSave(calls))
	var reads, saves []int
	for i, c := range calls {
		if isSave(c.Name) {
			saves = append(saves, i)
		} else {
			reads = append(reads, i)
		}
	}
	admissions := make([]guard.Admission, len(calls))
	for _, i := range slices.Concat(reads, saves) {
		admissions[i] = s.guard.Admit(i, s.callOf(calls[i]))
	}

	slots := make([]callSlot, len(calls))
	if err := s.runReads(ctx, calls, reads, slots, admissions); err != nil {
		return nil, guard.Outcome{}, err
	}
	var outcome guard.Outcome
	for n, i := range saves {
		if n > 0 {
			slots[i] = s.refusedSlot(calls[i], admissions[i])
			s.report(slots[i])
			continue
		}
		var err error
		if outcome, err = s.runSave(ctx, calls[i], &slots[i], admissions[i]); err != nil {
			return nil, guard.Outcome{}, err
		}
	}

	results := make([]llm.ToolResult, len(calls))
	for i, c := range calls {
		results[i] = llm.ToolResult{
			CallID:   c.ID,
			Content:  string(slots[i].content),
			IsError:  slots[i].result.IsError,
			Prunable: !isSave(c.Name) && !slots[i].refused && !slots[i].result.IsError,
		}
	}
	return results, outcome, nil
}

func (s *runState) runReads(ctx context.Context, calls []llm.ToolCall, reads []int, slots []callSlot, admissions []guard.Admission) error {
	duplicates := map[int]int{}
	var dispatch []int
	for _, i := range reads {
		adm := admissions[i]
		switch adm.Verdict {
		case guard.Refuse:
			slots[i] = s.refusedSlot(calls[i], adm)
		case guard.Reuse:
			duplicates[i] = adm.ReuseOf
		case guard.Replay:
			slots[i] = callSlot{
				name:    s.label(calls[i].Name),
				phase:   adm.Phase,
				content: withNote(s.replays[adm.Key], adm.Note),
				result:  tools.Result{Step: adm.Step},
			}
		default:
			dispatch = append(dispatch, i)
		}
	}

	sem := make(chan struct{}, maxParallelTools)
	var wg sync.WaitGroup
	for _, i := range dispatch {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			slots[i] = s.call(ctx, calls[i], admissions[i].Phase)
		})
	}
	wg.Wait()
	for _, i := range dispatch {
		if slots[i].err != nil {
			continue
		}
		settled, _ := s.guard.Settle(s.callOf(calls[i]), slots[i].result, slots[i].content)
		if settled.Replayable {
			s.remember(admissions[i].Key, slots[i].content)
		}
		if found := slots[i].result.Discovered; found != nil && !slots[i].result.IsError {
			s.digest.add(*found)
		}
	}
	for _, i := range reads {
		if first, ok := duplicates[i]; ok {
			slots[i] = reusedSlot(slots[first], admissions[i])
		}
		s.report(slots[i])
		if slots[i].err != nil {
			return s.internal(ctx, slots[i])
		}
	}
	return nil
}

func (s *runState) remember(key string, content []byte) {
	if s.replays == nil {
		s.replays = map[string][]byte{}
	}
	s.replays[key] = content
}

func (s *runState) refusedSlot(c llm.ToolCall, adm guard.Admission) callSlot {
	return callSlot{
		name:    s.label(c.Name),
		phase:   adm.Phase,
		content: adm.Content,
		result:  tools.Result{IsError: true, Step: adm.Step},
		refused: true,
	}
}

func reusedSlot(first callSlot, adm guard.Admission) callSlot {
	if first.refused {
		return first
	}
	first.duration = 0
	first.result.Step = adm.Step
	return first
}

func (s *runState) runSave(ctx context.Context, c llm.ToolCall, slot *callSlot, adm guard.Admission) (guard.Outcome, error) {
	if adm.Verdict == guard.Refuse {
		*slot = s.refusedSlot(c, adm)
		s.report(*slot)
		return guard.Outcome{}, nil
	}
	*slot = s.call(ctx, c, adm.Phase)
	s.report(*slot)
	if slot.err != nil {
		return guard.Outcome{}, s.internal(ctx, *slot)
	}
	settled, err := s.guard.Settle(s.callOf(c), slot.result, slot.content)
	if errors.Is(err, guard.ErrMalformed) {
		return guard.Outcome{}, s.malformed(ctx, *slot)
	}
	if settled.Stop != nil {
		return guard.Outcome{}, s.stopped(*settled.Stop, settled.Outcome)
	}
	if settled.Note != "" {
		slot.content = withNote(slot.content, settled.Note)
	}
	if settled.Outcome.Built {
		s.cutOffs = 0
		s.replays = nil
	}
	return settled.Outcome, nil
}

func (s *runState) call(ctx context.Context, c llm.ToolCall, phase string) callSlot {
	slot := callSlot{name: s.label(c.Name), phase: phase}
	start := time.Now()
	slot.result, slot.err = s.tools.Call(ctx, c.Name, c.Input)
	slot.end = time.Now()
	slot.duration = slot.end.Sub(start)
	if slot.err == nil {
		slot.content, slot.err = json.Marshal(slot.result.Content)
	}
	if s.logInputs {
		logToolInput(ctx, s.logger, slot.name, c.Input, slot.err == nil && slot.result.IsError, slot.content)
	}
	s.logger.DebugContext(ctx, "tool call finished",
		agentRequestAttr(ctx),
		slog.String("tool", slot.name),
		slog.Duration("duration", slot.duration),
		slog.Bool("is_error", slot.err != nil || slot.result.IsError),
	)
	return slot
}

func (s *runState) stopped(stop guard.Stop, outcome guard.Outcome) error {
	s.stop(stop.Step)
	if stop.Failure == revisions.FailureValidationExhausted {
		return repairExhausted(s.guard.Mode, outcome.IssueCodes)
	}
	return ErrNoProgress
}

func (s *runState) stop(message string) {
	if s.progress == nil {
		return
	}
	s.progress(Step{At: time.Now().UTC(), Phase: phaseSave, Tool: stopTool, Status: statusError, Message: message})
}

func (s *runState) note(message string) {
	if s.progress == nil {
		return
	}
	s.progress(Step{At: time.Now().UTC(), Phase: phaseSave, Tool: stopTool, Status: statusOK, Message: message})
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

func agentRequestAttr(ctx context.Context) slog.Attr {
	run, _ := tools.RunFrom(ctx)
	return slog.String("agent_request_id", run.RequestID)
}

func (s *runState) internal(ctx context.Context, slot callSlot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if errors.Is(slot.err, dashboards.ErrBuildLost) {
		return dashboards.ErrBuildLost
	}
	code := revisions.FailureCode(tools.FailureCode(slot.err))
	s.logger.ErrorContext(ctx, "agent tool failed", append([]any{
		slog.String("tool", slot.name),
		slog.String("error_code", string(code)),
		agentRequestAttr(ctx),
	}, logattrs.Safe(slot.err)...)...)
	return &safeError{message: "the " + slot.name + " tool failed with an internal error", code: code, err: slot.err}
}

func (s *runState) malformed(ctx context.Context, slot callSlot) error {
	s.logger.ErrorContext(ctx, "agent save result is malformed",
		slog.String("tool", slot.name), slog.String("error_code", string(codeMalformedResult)), agentRequestAttr(ctx))
	return &safeError{
		message: "the " + slot.name + " tool returned an unreadable result",
		code:    codeMalformedResult,
		err:     guard.ErrMalformed,
	}
}
