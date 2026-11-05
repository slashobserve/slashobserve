package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
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
	toolUpdatePanels    = "update_panels"
	toolImportGrafana   = "import_grafana_dashboard"

	codeInvalidRequest  = "invalid_request"
	codeVersionConflict = "version_conflict"
	codeNotFound        = "not_found"

	internalStep     = "internal error"
	stopTool         = "agent"
	unknownToolLabel = "unknown_tool"

	stepReusedRead = "Reused the result of an identical call"
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

type savedVersion struct {
	dashboardID string
	version     int
	replayed    bool
}

type turnOutcome struct {
	saved      *savedVersion
	built      bool
	suggestion json.RawMessage
	conflict   bool
	notFound   bool
	rejected   bool
	issueCodes []string
	answered   int
	note       string
}

type runState struct {
	tools         ToolSet
	known         map[string]bool
	progress      func(Step)
	logger        *slog.Logger
	logInputs     bool
	mode          tools.Mode
	parentVersion int
	rejections    int
	repairs       int
	toolCalls     int
	toolTime      time.Duration
	maxSaveOutput int64
	buildSaves    int
	cutOffs       int
	modelCalls    int
	pendingNudged bool

	nudges           int
	saveNowRefusals  int
	stalledTurns     int
	onePanelRefusals int

	window       saveWindow
	lastGuard    guard
	churn        churn
	sessionBytes int
	guards       guards
	digest       digest
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
	switch name {
	case toolCreateDashboard, toolUpdateDashboard, toolUpdatePanels, toolImportGrafana:
		return true
	}
	return false
}

func phaseFor(name string, repairing bool) string {
	switch {
	case name == "lint_query":
		return phaseLint
	case name == "query_range" || name == "query_instant":
		return phaseQuery
	case isSave(name):
		if repairing {
			return phaseRepair
		}
		return phaseSave
	}
	return phaseDiscover
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
	s.rejections, s.cutOffs, s.pendingNudged = 0, 0, false
	s.window = saveWindow{}
	s.churn = churn{}
	s.guards.resetAll()
}

func (s *runState) nudge(results []llm.ToolResult, outcome turnOutcome) {
	if len(results) == 0 || outcome.built || outcome.saved != nil || outcome.suggestion != nil {
		return
	}
	if note := nudgeFor(s.mode, s.window); note != "" {
		last := &results[len(results)-1]
		last.Content = string(withNote([]byte(last.Content), note))
		s.nudges++
	}
}

func (s *runState) executeTurn(ctx context.Context, calls []llm.ToolCall, readGuard guard) ([]llm.ToolResult, turnOutcome, error) {
	start := time.Now()
	defer func() { s.toolTime += time.Since(start) }()
	s.toolCalls += len(calls)
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

	if err := s.runReads(ctx, calls, reads, slots, repairing, readGuard); err != nil {
		return nil, turnOutcome{}, err
	}
	var outcome turnOutcome
	if saveIndex >= 0 {
		var err error
		if outcome, err = s.runSave(ctx, calls[saveIndex], &slots[saveIndex], repairing); err != nil {
			return nil, turnOutcome{}, err
		}
	}
	for _, i := range extraSaves {
		slots[i] = s.rejectCall(calls[i], repairing, "only one save per turn")
		s.report(slots[i])
	}

	results := make([]llm.ToolResult, len(calls))
	for i, c := range calls {
		if !slots[i].refused {
			outcome.answered++
		}
		results[i] = llm.ToolResult{
			CallID:   c.ID,
			Content:  string(slots[i].content),
			IsError:  slots[i].result.IsError,
			Prunable: !isSave(c.Name) && !slots[i].refused && !slots[i].result.IsError,
		}
	}
	return results, outcome, nil
}

func (s *runState) runReads(ctx context.Context, calls []llm.ToolCall, reads []int, slots []callSlot, repairing bool, readGuard guard) error {
	keys := make(map[int]string, len(reads))
	firstOf := map[string]int{}
	duplicates := map[int]int{}
	var dispatch []int
	for _, i := range reads {
		if readGuard.message != "" {
			slots[i] = s.refuseCall(calls[i], repairing, readGuard)
			s.saveNowRefusals++
			continue
		}
		key := callKey(calls[i].Name, calls[i].Input)
		if first, ok := firstOf[key]; ok {
			duplicates[i] = first
			continue
		}
		firstOf[key] = i
		keys[i] = key
		decision, content := s.guards.noteRead(key)
		switch decision {
		case readRefuse:
			slots[i] = s.refuseCall(calls[i], repairing, guardRepeatedRead)
		case readReplay:
			slots[i] = callSlot{
				name:    s.label(calls[i].Name),
				phase:   phaseFor(calls[i].Name, repairing),
				content: withNote(content, messageReplayed),
				result:  tools.Result{Step: stepReusedRead},
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
			slots[i] = s.call(ctx, calls[i], repairing)
		})
	}
	wg.Wait()
	for _, i := range dispatch {
		failed := slots[i].result.IsError || slots[i].err != nil
		s.guards.rememberRead(keys[i], calls[i].Name, slots[i].content, failed)
		if found := slots[i].result.Discovered; found != nil && !failed {
			s.digest.add(*found)
		}
	}
	for _, i := range reads {
		if first, ok := duplicates[i]; ok {
			slots[i] = reusedSlot(slots[first])
		}
		s.report(slots[i])
		if slots[i].err != nil {
			return s.internal(ctx, slots[i])
		}
	}
	return nil
}

func reusedSlot(first callSlot) callSlot {
	if first.refused {
		return first
	}
	first.duration = 0
	first.result.Step = stepReusedRead
	return first
}

func (s *runState) runSave(ctx context.Context, c llm.ToolCall, slot *callSlot, repairing bool) (turnOutcome, error) {
	if s.guards.noteSave(callKey(c.Name, c.Input)) {
		*slot = s.refuseCall(c, repairing, guardRepeatedSave)
		s.report(*slot)
		return turnOutcome{}, nil
	}
	*slot = s.call(ctx, c, repairing)
	if slot.phase == phaseRepair {
		s.repairs++
	}
	if slot.result.Refusal == tools.RefusalOnePanel {
		s.onePanelRefusals++
	}
	s.report(*slot)
	if slot.err != nil {
		return turnOutcome{}, s.internal(ctx, *slot)
	}
	outcome, err := s.classifySave(ctx, *slot)
	if outcome.note != "" {
		slot.content = withNote(slot.content, outcome.note)
	}
	return outcome, err
}

func (s *runState) call(ctx context.Context, c llm.ToolCall, repairing bool) callSlot {
	slot := callSlot{name: s.label(c.Name), phase: phaseFor(c.Name, repairing)}
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

func (s *runState) rejectCall(c llm.ToolCall, repairing bool, reason string) callSlot {
	content, _ := json.Marshal(map[string]string{"code": codeInvalidRequest, "message": reason})
	return callSlot{
		name:    s.label(c.Name),
		phase:   phaseFor(c.Name, repairing),
		content: content,
		result:  tools.Result{IsError: true, Step: "Rejected " + s.label(c.Name) + " input: " + reason},
		refused: true,
	}
}

func (s *runState) refuseCall(c llm.ToolCall, repairing bool, g guard) callSlot {
	s.lastGuard = g
	slot := s.rejectCall(c, repairing, g.message)
	slot.result.Step = "Refused " + s.label(c.Name) + ": " + g.refused
	return slot
}

func (s *runState) endStall(outcome turnOutcome) error {
	s.window.diagnose = outcome.rejected
	if outcome.answered > 0 {
		s.window.stalled = 0
		return nil
	}
	s.stalledTurns++
	if s.window.stalled++; s.window.stalled < maxStalledTurns {
		return nil
	}
	reason := s.lastGuard.stopped
	if reason == "" {
		reason = fmt.Sprintf("Stopped: %d turns in a row had every call refused", maxStalledTurns)
	}
	s.stop(reason)
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

type saveBody struct {
	Code          string          `json:"code"`
	DashboardID   string          `json:"dashboard_id"`
	Version       int             `json:"version"`
	LatestVersion int             `json:"latest_version"`
	Replayed      *bool           `json:"replayed"`
	Suggested     bool            `json:"suggested"`
	Build         *struct{}       `json:"build"`
	Panel         json.RawMessage `json:"panel"`
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
		return s.classifyAccepted(ctx, slot, body)
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

func (s *runState) classifyAccepted(ctx context.Context, slot callSlot, body saveBody) (turnOutcome, error) {
	switch {
	case s.mode == tools.ModeSuggest:
		if !body.Suggested || len(body.Panel) == 0 {
			return turnOutcome{}, s.malformed(ctx, slot)
		}
		return turnOutcome{suggestion: body.Panel}, nil
	case body.Build != nil && body.Version < 1:
		s.rejections, s.cutOffs = 0, 0
		s.window = saveWindow{}
		s.buildSaves++
		s.guards.resetReads()
		note, stop := s.churn.note(slot.result.Edits)
		if stop {
			s.stop(fmt.Sprintf("Stopped: %d saves in a row changed nothing new", maxChurnedSavesInRow+1))
			return turnOutcome{}, ErrNoProgress
		}
		return turnOutcome{built: true, note: note}, nil
	case body.DashboardID == "" || body.Version < 1:
		return turnOutcome{}, s.malformed(ctx, slot)
	}
	if slot.name != toolImportGrafana {
		s.buildSaves++
	}
	s.window = saveWindow{}
	return turnOutcome{saved: &savedVersion{
		dashboardID: body.DashboardID,
		version:     body.Version,
		replayed:    body.Replayed != nil && *body.Replayed,
	}}, nil
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
	s.logger.ErrorContext(ctx, "agent save result is malformed",
		slog.String("tool", slot.name), slog.String("error_code", string(codeMalformedResult)), agentRequestAttr(ctx))
	return &safeError{
		message: "the " + slot.name + " tool returned an unreadable result",
		code:    codeMalformedResult,
		err:     errors.New("save result is malformed"),
	}
}
