package runsession

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	codeRunEnded    = "run_ended"
	messageRunEnded = "this run has ended"
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
}

func (a Answer) Text() string {
	text := slices.Clone(a.Content)
	for _, note := range a.Notes {
		text = append(text, "\n\n<note>"+note+"</note>"...)
	}
	return string(text)
}

type admitted struct {
	adm    guard.Admission
	seq    int
	notes  []string
	answer *Answer
}

type dispatched struct {
	res     tools.Result
	content []byte
	err     error
	start   time.Time
	took    time.Duration
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
	done := dispatched{start: time.Now()}
	done.res, done.err = dispatch(ctx)
	done.took = time.Since(done.start)
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
	if sess.state.Turn.Open && !sess.state.Turn.joinedBy(turn) {
		nudge, err := sess.closeTurn(ctx)
		if err != nil {
			return out, err
		}
		if nudge != "" {
			out.notes = append(out.notes, nudge)
		}
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
	step := revisions.Step{At: d.start, Phase: a.adm.Phase, Tool: c.Name, Status: "ok", DurationMS: d.took.Milliseconds()}
	if d.err != nil {
		state.Usage.FailedCalls++
		step.Status, step.Message = "error", internalStep
		sess.record(step)
		_, err := sess.complete(ctx, a.seq)
		return Answer{}, err
	}
	if d.res.IsError {
		state.Usage.FailedCalls++
		step.Status = "error"
	}
	switch {
	case a.adm.Verdict != guard.Dispatch:
		step.Message = a.adm.Step
	case d.res.Step != "":
		step.Message = d.res.Step
	default:
		step.Message = c.Name
	}
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
		if found := d.res.Discovered; found != nil && !d.res.IsError {
			state.addDiscovery(*found)
		}
		return false, nil
	}
	out := settled.Outcome
	state.IssueCodes = nil
	if out.Rejected {
		state.IssueCodes = out.IssueCodes
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
	case out.Conflict:
		return sess.resolveConflict(ctx)
	case out.NotFound:
		return false, sess.end(ctx, endSpec{failure: revisions.FailureNotFound})
	case out.Built:
		sess.refreshBuild(ctx)
	}
	return false, nil
}
