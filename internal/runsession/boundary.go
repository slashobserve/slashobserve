package runsession

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	codeRunEnded    = "run_ended"
	messageRunEnded = "this run has ended"

	stepLedgerUnchanged = "Hypotheses unchanged"
)

type Call struct {
	Name  string
	Input json.RawMessage
	Turn  *Turn
	Hints *Hints
}

type Hints struct {
	Sweep bool   `json:"sweep"`
	Step  string `json:"step"`
}

type RunMeta struct {
	Settled *SettledKind         `json:"settled"`
	Build   *tools.BuildProgress `json:"build"`
	Call    *CallMeta            `json:"call,omitempty"`
}

type CallMeta struct {
	Edits      *CallEdits     `json:"edits"`
	Refusal    *string        `json:"refusal"`
	Unchecked  *string        `json:"unchecked"`
	Discovered *CallDiscovery `json:"discovered"`
	Ranges     []CallRange    `json:"ranges"`
}

type CallEdits struct {
	Panels   []string `json:"panels"`
	Added    []string `json:"added"`
	Deleted  []string `json:"deleted"`
	Settings bool     `json:"settings"`
}

type CallDiscovery struct {
	Datasource string   `json:"datasource"`
	Of         string   `json:"of"`
	Names      []string `json:"names"`
}

type CallRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

func callMeta(res tools.Result, ranges []datasource.TimeRange) *CallMeta {
	meta := &CallMeta{Ranges: make([]CallRange, len(ranges))}
	for i, r := range ranges {
		meta.Ranges[i] = CallRange{From: r.From.UTC(), To: r.To.UTC()}
	}
	if e := res.Edits; len(e.Panels) > 0 || len(e.Added) > 0 || len(e.Deleted) > 0 || e.Settings {
		meta.Edits = &CallEdits{Panels: nonNil(e.Panels), Added: nonNil(e.Added), Deleted: nonNil(e.Deleted), Settings: e.Settings}
	}
	if res.Refusal != "" {
		meta.Refusal = ptr(string(res.Refusal))
	}
	if res.Unchecked != "" {
		meta.Unchecked = ptr(res.Unchecked)
	}
	if found := res.Discovered; found != nil {
		meta.Discovered = &CallDiscovery{Datasource: found.Datasource, Of: found.Of, Names: nonNil(found.Names)}
	}
	return meta
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

type Answer struct {
	Content []byte
	Notes   []string
	IsError bool
	Meta    RunMeta
	App     []byte
}

func (a Answer) Text() string {
	text := slices.Clone(a.Content)
	for _, note := range a.Notes {
		text = append(text, "\n\n<note>"+note+"</note>"...)
	}
	return string(text)
}

type admitted struct {
	phase string
	hints *Hints
}

func (a admitted) sweep() bool {
	return a.hints != nil && a.hints.Sweep
}

type dispatched struct {
	res     tools.Result
	content []byte
	err     error
	start   time.Time
	took    time.Duration
	ranges  []datasource.TimeRange
}

func (s *Service) Call(ctx context.Context, req revisions.Request, call Call, dispatch func(context.Context) (tools.Result, error)) (Answer, error) {
	var first admitted
	var scope tools.WriteScope
	err := s.with(ctx, req, func(ctx context.Context, sess *session) error {
		first = sess.admit(call)
		scope = sess.state.writeScope()
		return nil
	})
	if errors.Is(err, errRunNotRunning) {
		return endedAnswer(), nil
	}
	if err != nil {
		return Answer{}, err
	}
	done := dispatched{start: time.Now()}
	noted, readRestricted := datasource.NotingRestrictedReads(ctx)
	noted, ranges := tools.NotingRanges(noted)
	done.res, done.err = dispatch(tools.WithWriteScope(noted, scope))
	done.took = time.Since(done.start)
	done.ranges = ranges()
	if readRestricted() {
		if err := s.markReadRestricted(ctx, req); err != nil {
			return Answer{}, err
		}
	}
	if done.err == nil {
		done.content, done.err = json.Marshal(done.res.Content)
	}
	var answer Answer
	err = s.with(ctx, req, func(ctx context.Context, sess *session) error {
		var err error
		answer, err = sess.settle(ctx, call.Name, first, done)
		return err
	})
	switch {
	case errors.Is(err, errRunNotRunning):
		return Answer{Content: done.content, IsError: done.res.IsError, Meta: RunMeta{Settled: ptr(settledEnded), Call: callMeta(done.res, done.ranges)}}, done.err
	case err != nil:
		return Answer{}, err
	}
	if done.err == nil {
		answer.Meta.Call = callMeta(done.res, done.ranges)
	}
	return answer, done.err
}

func endedAnswer(notes ...string) Answer {
	content, _ := json.Marshal(map[string]string{"code": codeRunEnded, "message": messageRunEnded})
	return Answer{Content: content, IsError: true, Notes: notes, Meta: RunMeta{Settled: ptr(settledEnded)}}
}

func (sess *session) admit(call Call) admitted {
	if call.Turn != nil && call.Turn.Usage != nil {
		sess.state.noteModel(*call.Turn.Usage)
	}
	out := admitted{hints: call.Hints, phase: sess.state.phase(call.Name)}
	if !out.sweep() {
		sess.state.Counts.ToolCalls++
	}
	return out
}

func (sess *session) settle(ctx context.Context, name string, a admitted, d dispatched) (Answer, error) {
	state := &sess.state
	if a.sweep() {
		if d.err != nil {
			return Answer{}, d.err
		}
		return Answer{Content: d.content, IsError: d.res.IsError, Meta: state.meta(false)}, nil
	}
	state.Usage.ToolMS += d.took.Milliseconds()
	step := revisions.Step{At: d.start, Phase: a.phase, Tool: name, Status: "ok", DurationMS: d.took.Milliseconds()}
	if d.err != nil {
		step.Status, step.Message = "error", internalStep
		sess.record(step)
		return Answer{}, nil
	}
	if d.res.IsError {
		step.Status = "error"
	}
	step.Message = state.stepMessage(name, a, d)
	sess.record(step)
	answer := Answer{Content: d.content, IsError: d.res.IsError}
	restarted, err := sess.settleOutcome(ctx, name, d)
	if err != nil {
		return Answer{}, err
	}
	if restarted {
		answer.Notes = append(answer.Notes, conflictNote)
	}
	if sess.ended && sess.endedBy != nil && sess.endedBy.step != "" {
		answer.Notes = append(answer.Notes, sess.endedBy.step)
	}
	answer.Meta = state.meta(restarted && !sess.ended)
	return answer, nil
}

func (s *State) stepMessage(name string, a admitted, d dispatched) string {
	message := name
	switch {
	case d.res.Ledger != nil && !d.res.IsError && !s.noteLedger(d.res.Ledger):
		message = stepLedgerUnchanged
	case d.res.Step != "":
		message = d.res.Step
	}
	if a.hints != nil {
		if hinted := claims.CleanText(a.hints.Step, maxMessageRunes); hinted != "" {
			return hinted
		}
	}
	return message
}

func (sess *session) settleOutcome(ctx context.Context, name string, d dispatched) (restarted bool, err error) {
	state := &sess.state
	if !isSave(name) {
		if d.res.AllTools && !d.res.IsError {
			state.AllTools = true
		}
		if tools.Phase(name) == tools.PhaseWrite && !d.res.IsError {
			state.Wrote = true
			state.noteCreated(d.res.Created)
		}
		return false, nil
	}
	out, err := state.classifySave(name, d.res, d.content)
	if errors.Is(err, errMalformed) {
		sess.s.failure(ctx, "run session save result is malformed", string(revisions.FailureMalformedResult), err, sess.req)
		return false, sess.end(ctx, endSpec{failure: revisions.FailureMalformedResult, message: "the " + name + " tool returned an unreadable result"})
	}
	state.IssueCodes, state.Failing, state.FailingOther = nil, nil, 0
	state.RejectedFinding, state.RejectedTransient = false, false
	if out.Rejected {
		state.IssueCodes, state.Failing, state.FailingOther = out.IssueCodes, out.FailingPanels, out.FailingOther
		state.RejectedFinding, state.RejectedTransient = out.RejectedFinding, out.Transient
	}
	return sess.apply(ctx, out)
}

func (sess *session) apply(ctx context.Context, out outcome) (restarted bool, err error) {
	state := &sess.state
	switch {
	case out.Finding != "":
		state.Settled = Settled{Kind: settledFinding}
	case out.Suggestion != nil:
		state.Suggestion = out.Suggestion
		state.Settled = Settled{Kind: settledSuggested}
	case out.Proposal != "":
		state.Settled = Settled{Kind: settledProposed}
	case out.Saved != nil:
		state.Settled = Settled{Kind: settledSaved, Version: out.Saved.Version}
		state.Committed = out.Saved.DashboardID
	case out.Conflict:
		return sess.resolveConflict(ctx)
	case out.NotFound:
		return false, sess.end(ctx, endSpec{failure: revisions.FailureNotFound})
	case out.Built:
		sess.refreshBuild(ctx)
	}
	return false, nil
}

func isSave(name string) bool {
	return tools.Phase(name) == tools.PhaseSave
}
