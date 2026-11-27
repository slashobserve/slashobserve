package runsession

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
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
}

type RunMeta struct {
	Settled *string              `json:"settled"`
	Build   *tools.BuildProgress `json:"build"`
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
	adm       guard.Admission
	seq       int
	notes     []string
	answer    *Answer
	grounding groundingCheck
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
	gc := guard.Call{Name: call.Name, Input: call.Input}
	var first admitted
	err := s.with(ctx, req, func(ctx context.Context, sess *session) error {
		var err error
		first, err = sess.admit(ctx, gc, call.Turn.valid())
		return err
	})
	if errors.Is(err, errRunNotRunning) {
		return endedAnswer(), nil
	}
	if err != nil {
		return Answer{}, err
	}
	if first.answer != nil {
		return *first.answer, nil
	}
	var scope tools.WriteScope
	if tools.Phase(call.Name) == tools.PhaseWrite {
		err = s.with(ctx, req, func(ctx context.Context, sess *session) error {
			scope = sess.state.writeScope()
			return nil
		})
		if errors.Is(err, errRunNotRunning) {
			return endedAnswer(), nil
		}
		if err != nil {
			return Answer{}, err
		}
	}
	done := dispatched{start: time.Now()}
	noted, readRestricted := datasource.NotingRestrictedReads(ctx)
	noted, ranges := tools.NotingRanges(noted)
	noted = tools.WithFacts(tools.WithUnverified(noted, first.grounding.unverified), first.grounding.ledger)
	if first.grounding.scored {
		noted = tools.WithCauseSignals(noted, first.grounding.signals)
	}
	done.res, done.err = dispatch(tools.WithWriteScope(tools.WithUnchecked(tools.WithReadsRefused(noted, first.adm.ReadsRefused), first.adm.Unchecked), scope))
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
		answer, err = sess.settle(ctx, gc, first, done)
		return err
	})
	switch {
	case errors.Is(err, errRunNotRunning):
		return Answer{Content: done.content, IsError: done.res.IsError, Meta: RunMeta{Settled: ptr(settledEnded)}}, done.err
	case err != nil:
		return Answer{}, err
	}
	return answer, done.err
}

func endedAnswer(notes ...string) Answer {
	content, _ := json.Marshal(map[string]string{"code": codeRunEnded, "message": messageRunEnded})
	return Answer{Content: content, IsError: true, Notes: notes, Meta: RunMeta{Settled: ptr(settledEnded)}}
}

func (sess *session) endedAnswer(notes []string) Answer {
	if sess.endedBy != nil && sess.endedBy.step != "" {
		notes = append(notes, sess.endedBy.step)
	}
	return endedAnswer(notes...)
}

func (sess *session) admit(ctx context.Context, c guard.Call, turn *Turn) (admitted, error) {
	var out admitted
	if turn != nil && turn.Usage != nil {
		sess.state.noteModel(*turn.Usage)
	}
	if sess.state.Turn.Open && !sess.state.Turn.joinedBy(turn) {
		notes, err := sess.closeTurn(ctx)
		if err != nil {
			return out, err
		}
		out.notes = append(out.notes, notes...)
		if sess.ended {
			answer := sess.endedAnswer(out.notes)
			out.answer = &answer
			return out, nil
		}
	}
	if !sess.state.Turn.Open {
		sess.beginTurn(c, turn)
	}
	open := &sess.state.Turn
	position := open.Admitted
	open.Admitted++
	out.seq = open.Seq
	out.grounding = sess.state.checkGrounding(&c, sess.req.Prompt)
	out.adm = sess.state.Guard.Admit(position, c)
	if out.adm.Verdict != guard.Refuse {
		return out, nil
	}
	sess.record(revisions.Step{At: time.Now(), Phase: out.adm.Phase, Tool: c.Name, Status: "error", Message: out.adm.Step})
	answer := Answer{Content: out.adm.Content, IsError: true, Notes: out.notes}
	notes, err := sess.complete(ctx, out.seq)
	if sess.ended {
		answer = sess.endedAnswerWith(answer, notes)
	} else {
		answer.Notes = append(answer.Notes, notes...)
		answer.Meta = sess.state.meta(false)
	}
	out.answer = &answer
	return out, err
}

func (sess *session) endedAnswerWith(answer Answer, notes []string) Answer {
	answer.Notes = append(answer.Notes, notes...)
	if sess.endedBy != nil && sess.endedBy.step != "" {
		answer.Notes = append(answer.Notes, sess.endedBy.step)
	}
	answer.Meta = sess.state.meta(false)
	return answer
}

func (sess *session) settle(ctx context.Context, c guard.Call, a admitted, d dispatched) (Answer, error) {
	state := &sess.state
	state.Usage.ToolMS += d.took.Milliseconds()
	sess.endRead(c, a)
	step := revisions.Step{At: d.start, Phase: a.adm.Phase, Tool: c.Name, Status: "ok", DurationMS: d.took.Milliseconds()}
	if d.err != nil {
		return Answer{}, sess.settleFailed(ctx, c, a, step)
	}
	d.content = state.issueFacts(c, d)
	state.noteGrounding(c, d)
	if d.res.IsError {
		step.Status = "error"
	}
	step.Message = state.stepMessage(c, a, d)
	sess.record(step)
	answer := Answer{Content: d.content, IsError: d.res.IsError, Notes: slices.Clone(a.notes)}
	if a.adm.Note != "" {
		answer.Notes = append(answer.Notes, a.adm.Note)
	}
	restarted := false
	if a.adm.Verdict == guard.Dispatch {
		var err error
		if restarted, err = sess.settleOutcome(ctx, c, d, &answer); err != nil {
			return Answer{}, err
		}
	}
	if !sess.ended {
		notes, err := sess.complete(ctx, a.seq)
		answer.Notes = append(answer.Notes, notes...)
		if err != nil {
			return Answer{}, err
		}
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

func (s *State) stepMessage(c guard.Call, a admitted, d dispatched) string {
	switch {
	case a.adm.Verdict != guard.Dispatch:
		return a.adm.Step
	case d.res.Ledger != nil && !d.res.IsError && !s.noteLedger(d.res.Ledger):
		return stepLedgerUnchanged
	case d.res.Step != "":
		return d.res.Step
	}
	return c.Name
}

func (sess *session) endRead(c guard.Call, a admitted) {
	if a.adm.Verdict != guard.Refuse && sess.state.Turn.Open && sess.state.Turn.Seq == a.seq {
		sess.state.Guard.EndRead(c)
	}
}

func (sess *session) settleFailed(ctx context.Context, c guard.Call, a admitted, step revisions.Step) error {
	step.Status, step.Message = "error", internalStep
	if tools.Phase(c.Name) == tools.PhaseWrite && a.adm.Verdict == guard.Dispatch {
		sess.state.Guard.ForgetWrite(c)
	}
	sess.record(step)
	_, err := sess.complete(ctx, a.seq)
	return err
}

func (sess *session) settleOutcome(ctx context.Context, c guard.Call, d dispatched, answer *Answer) (restarted bool, err error) {
	state := &sess.state
	settled, err := state.Guard.Settle(c, d.res, d.content)
	if errors.Is(err, guard.ErrMalformed) {
		sess.s.failure(ctx, "run session save result is malformed", string(revisions.FailureMalformedResult), err, sess.req)
		return false, sess.end(ctx, endSpec{failure: revisions.FailureMalformedResult, message: "the " + c.Name + " tool returned an unreadable result"})
	}
	if settled.Note != "" {
		answer.Notes = append(answer.Notes, settled.Note)
	}
	if !isSave(c.Name) {
		if d.res.AllTools && !d.res.IsError {
			state.AllTools = true
		}
		if tools.Phase(c.Name) == tools.PhaseWrite && !d.res.IsError {
			state.Wrote = true
			state.noteCreated(d.res.Created)
		}
		if found := d.res.Discovered; found != nil && !d.res.IsError {
			state.addDiscovery(*found)
		}
		return false, nil
	}
	out := settled.Outcome
	state.IssueCodes, state.Failing, state.FailingOther = nil, nil, 0
	state.RejectedFinding, state.RejectedTransient = false, false
	if out.Rejected {
		state.IssueCodes, state.Failing, state.FailingOther = out.IssueCodes, out.FailingPanels, out.FailingOther
		state.RejectedFinding, state.RejectedTransient = out.RejectedFinding, out.Transient
	}
	if settled.Stop != nil {
		return false, sess.end(ctx, endSpec{failure: settled.Stop.Failure, step: settled.Stop.Step})
	}
	return sess.apply(ctx, out)
}

func (sess *session) apply(ctx context.Context, out guard.Outcome) (restarted bool, err error) {
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
