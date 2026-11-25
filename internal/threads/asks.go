package threads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	RefusalUnsupportedTarget = "mention_target_unsupported"
	RefusalAgentUnavailable  = "agent_unavailable"
	RefusalForbidden         = "forbidden_role"
	RefusalTooManyRuns       = "too_many_runs"
	RefusalPersonStarts      = "investigation_person_only"
	RefusalNotYourAgent      = "agent_not_yours"

	transcriptMessages = 20
	transcriptRunes    = 8000
	answerRunes        = 1500
	triggerMention     = "mention"

	actionThreadAgentSet audit.Action = "thread.agent_set"
)

type StartedRun struct {
	ID           string
	Status       string
	AssignedKind string
}

type RunRefusal struct {
	Code    string
	Message string
}

func ParseTo(s string) (To, error) {
	switch to := To(s); to {
	case ToAuto, ToAgent, ToEveryone:
		return to, nil
	}
	return ToAuto, &InvalidInputError{Code: CodeInvalidTo, Message: "to must be agent or everyone"}
}

type Sent struct {
	Thread   Thread
	Message  Message
	Run      *StartedRun
	Refusal  *RunRefusal
	To       To
	Replayed bool
}

type Asks struct {
	service      *Service
	queue        *revisions.Service
	builtinAgent bool
}

func NewAsks(service *Service, queue *revisions.Service, builtinAgent bool) *Asks {
	return &Asks{service: service, queue: queue, builtinAgent: builtinAgent}
}

func (a *Asks) Open(ctx context.Context, by auth.Principal, in OpenInput, to To) (Sent, error) {
	sc, err := a.service.scopeOf(ctx, by, in.Target)
	if err != nil {
		return Sent{}, err
	}
	req := a.newAsk(ctx, by, sc, to)
	if err := req.precheck(threadAgent{}, in.Body); err != nil {
		return Sent{}, err
	}
	w, err := a.service.open(ctx, by, sc, in, req.options())
	var exists *ObservationThreadExistsError
	if errors.As(err, &exists) && in.Anchor == nil {
		return a.Reply(ctx, by, exists.ThreadID, ReplyInput{Body: in.Body, BlockID: in.BlockID, ClientID: in.ClientID}, to)
	}
	if err != nil {
		return Sent{}, err
	}
	return req.sent(w), nil
}

func (a *Asks) Reply(ctx context.Context, by auth.Principal, threadID pgtype.UUID, in ReplyInput, to To) (Sent, error) {
	row, sc, err := a.service.writableThread(ctx, by, threadID)
	if err != nil {
		return Sent{}, err
	}
	req := a.newAsk(ctx, by, sc, to)
	if err := req.precheck(threadOf(row, nil, rights{}).recipientState(), in.Body); err != nil {
		return Sent{}, err
	}
	if req.checked {
		req.answers = a.service.answersBefore(ctx, by, row.ID)
	}
	w, err := a.service.reply(ctx, by, row, sc, in, req.options())
	if err != nil {
		return Sent{}, err
	}
	return req.sent(w), nil
}

type ask struct {
	asks      *Asks
	ctx       context.Context
	by        auth.Principal
	sc        scope
	to        To
	checked   bool
	checkErr  error
	denied    *RunRefusal
	refusal   *RunRefusal
	queuedRun *StartedRun
	asked     bool
	answers   map[pgtype.UUID]Answer
}

func (a *Asks) newAsk(ctx context.Context, by auth.Principal, sc scope, to To) *ask {
	return &ask{asks: a, ctx: ctx, by: by, sc: sc, to: to}
}

func (r *ask) precheck(state threadAgent, body string) error {
	recipient, err := recipientOf(state, body, r.to, r.asks.builtinAgent)
	if err != nil || recipient.Agent == nil {
		return err
	}
	_, err = r.runAllowed()
	return err
}

func (r *ask) options() writeOptions {
	return writeOptions{hook: r.hook, lockForRun: r.sc.target.Kind == KindObservation}
}

func (r *ask) sent(w written) Sent {
	out := Sent{Thread: w.thread, Message: w.message, Run: r.queuedRun, Refusal: r.refusal, To: ToEveryone, Replayed: w.replayed}
	if w.replayed {
		out.Run, out.Refusal = w.message.Run, nil
	}
	if r.asked || out.Run != nil {
		out.To = ToAgent
	}
	return out
}

func (r *ask) runAllowed() (*RunRefusal, error) {
	if r.checked {
		return r.denied, r.checkErr
	}
	r.checked = true
	var target revisions.RunTarget
	switch r.sc.target.Kind {
	case KindDashboard:
		target = revisions.RunTarget{Intent: revisions.IntentAsk, DashboardID: r.sc.target.ID}
	case KindObservation:
		target = revisions.RunTarget{Intent: revisions.IntentAsk, ObservationID: r.sc.target.ID}
	default:
		r.denied = &RunRefusal{Code: RefusalUnsupportedTarget, Message: "agents can be asked from dashboard and observation threads"}
		return r.denied, nil
	}
	err := r.asks.queue.CheckRun(r.ctx, target, r.by)
	if errors.Is(err, revisions.ErrForbidden) || errors.Is(err, folders.ErrForbidden) {
		r.denied = &RunRefusal{Code: RefusalForbidden, Message: err.Error()}
		return r.denied, nil
	}
	r.checkErr = err
	return nil, err
}

type reach int

const (
	reachable reach = iota
	reachNotYours
	reachGone
)

func (r *ask) reachOf(q *dbsqlc.Queries, agent revisions.Assignee) (reach, error) {
	switch agent.Kind {
	case revisions.AssignGrant:
		live, err := q.AgentGrantIsLive(r.ctx, agent.ID)
		if err != nil {
			return reachGone, fmt.Errorf("read the thread's agent: %w", err)
		}
		if !live {
			return reachGone, nil
		}
		personID, parseErr := db.ParseUUID(r.by.ID)
		if r.by.Kind != auth.KindUser || parseErr != nil {
			return reachNotYours, nil
		}
		_, err = q.MentionableGrant(r.ctx, dbsqlc.MentionableGrantParams{ID: agent.ID, UserID: personID})
		if errors.Is(err, pgx.ErrNoRows) {
			return reachNotYours, nil
		}
		if err != nil {
			return reachGone, fmt.Errorf("read the thread's agent: %w", err)
		}
	case revisions.AssignServiceAccount:
		_, err := q.MentionableServiceAccount(r.ctx, agent.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return reachGone, nil
		}
		if err != nil {
			return reachGone, fmt.Errorf("read the thread's agent: %w", err)
		}
	}
	return reachable, nil
}

func (r *ask) hook(q *dbsqlc.Queries, thread Thread, message Message) error {
	state := thread.recipientState()
	recipient, err := recipientOf(state, message.Body, r.to, r.asks.builtinAgent)
	if err != nil {
		return err
	}
	if recipient.Unavailable {
		r.asked = true
		denied, err := r.runAllowed()
		if err != nil {
			return err
		}
		r.refusal = denied
		if denied == nil {
			r.refusal = &RunRefusal{Code: RefusalAgentUnavailable, Message: revisions.ErrUnavailable.Error()}
		}
		return nil
	}
	if recipient.Agent == nil {
		return r.setSticky(q, thread, state, recipient)
	}
	r.asked = true
	if len(agentMentions(message.Body)) == 0 {
		switch reached, err := r.reachOf(q, *recipient.Agent); {
		case err != nil:
			return err
		case reached == reachGone:
			r.refusal = &RunRefusal{Code: RefusalAgentUnavailable, Message: revisions.ErrAssigneeUnavailable.Error()}
			return r.writeSticky(q, thread, nil, false)
		case reached == reachNotYours:
			r.refusal = &RunRefusal{Code: RefusalNotYourAgent, Message: "this thread's agent is someone else's connected agent; mention your own agent or @observe to ask, the message was saved without a run"}
			return nil
		}
	}
	if err := r.setSticky(q, thread, state, recipient); err != nil {
		return err
	}
	denied, err := r.runAllowed()
	if err != nil || denied != nil {
		r.refusal = denied
		return err
	}
	row, err := r.enqueue(q, thread, message, *recipient.Agent)
	if refusal := capRefusal(err); refusal != nil {
		r.refusal = refusal
		return nil
	}
	if err != nil {
		return err
	}
	r.queuedRun = &StartedRun{ID: row.ID.String(), Status: row.Status, AssignedKind: row.AssignedKind}
	return nil
}

func (r *ask) setSticky(q *dbsqlc.Queries, thread Thread, state threadAgent, recipient Recipient) error {
	switch recipient.Sticky {
	case stickyOff:
		if state.Agent == nil || state.Off {
			return nil
		}
		return r.writeSticky(q, thread, state.Agent, true)
	case stickyFollow:
		if state.Agent != nil && *state.Agent == *recipient.Agent && !state.Off {
			return nil
		}
		return r.writeSticky(q, thread, recipient.Agent, false)
	}
	return nil
}

func (r *ask) writeSticky(q *dbsqlc.Queries, thread Thread, agent *revisions.Assignee, off bool) error {
	threadID, err := db.ParseUUID(thread.ID)
	if err != nil {
		return fmt.Errorf("thread id: %w", err)
	}
	params := dbsqlc.SetThreadAgentParams{ID: threadID, AgentOff: off}
	if agent != nil {
		params.AgentKind = pgtype.Text{String: string(agent.Kind), Valid: true}
		params.AgentID = agent.ID
	}
	if _, err := q.SetThreadAgent(r.ctx, params); err != nil {
		return fmt.Errorf("set the thread's agent: %w", err)
	}
	cols, err := actorOf(r.ctx, r.by).Columns()
	if err != nil {
		return err
	}
	target := audit.Target{Kind: "thread", ID: thread.ID}
	switch r.sc.target.Kind {
	case KindDashboard:
		target.DashboardID = uuid.NullUUID{UUID: r.sc.target.ID.Bytes, Valid: true}
	case KindObservation:
		target.ObservationID = uuid.NullUUID{UUID: r.sc.target.ID.Bytes, Valid: true}
	}
	return audit.Record(r.ctx, q, audit.Event{Action: actionThreadAgentSet, Target: target, Thread: uuid.NullUUID{UUID: threadID.Bytes, Valid: true}, By: cols.AuditBy()})
}

func agentMentions(body string) []Mention {
	mentions, err := ParseMentions(body)
	if err != nil {
		return nil
	}
	return mentions.Agents()
}

func capRefusal(err error) *RunRefusal {
	if errors.Is(err, revisions.ErrTooManyRevisions) || errors.Is(err, revisions.ErrTooManyRequests) || errors.Is(err, revisions.ErrAgentBusy) {
		return &RunRefusal{Code: RefusalTooManyRuns, Message: err.Error()}
	}
	return nil
}

func (r *ask) enqueue(q *dbsqlc.Queries, thread Thread, message Message, agent revisions.Assignee) (dbsqlc.AgentRequest, error) {
	asked, err := askedIn(r.ctx, q, r.answers, thread, message)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	prompt := message.Body
	if mentioned := agentMentions(message.Body); len(mentioned) == 1 {
		if cut := withoutMention(message.Body, mentioned[0]); cut != "" {
			prompt = cut
		}
	}
	params := revisions.InvestigateParams{
		Prompt:          prompt,
		By:              r.by,
		Assignee:        agent,
		Trigger:         triggerMention,
		ThreadMessageID: asked.message,
		ThreadContext:   asked.transcript,
		Point:           asked.point,
		Ask:             true,
	}
	switch thread.Target.Kind {
	case KindDashboard:
		params.DashboardID, params.PanelID = thread.Target.ID, thread.PanelID
	case KindObservation:
		params.ObservationID = thread.Target.ID
	default:
		return dbsqlc.AgentRequest{}, fmt.Errorf("start a run on a %s thread", thread.Target.Kind)
	}
	return r.asks.queue.EnqueueAssignedInvestigate(r.ctx, q, params)
}

type askContext struct {
	thread, message pgtype.UUID
	transcript      string
	point           revisions.Point
}

func askedIn(ctx context.Context, q *dbsqlc.Queries, answers map[pgtype.UUID]Answer, thread Thread, message Message) (askContext, error) {
	threadID, err := db.ParseUUID(thread.ID)
	if err != nil {
		return askContext{}, fmt.Errorf("thread id: %w", err)
	}
	messageID, err := db.ParseUUID(message.ID)
	if err != nil {
		return askContext{}, fmt.Errorf("message id: %w", err)
	}
	earlier, err := q.ThreadTranscript(ctx, dbsqlc.ThreadTranscriptParams{ThreadID: threadID, BeforeID: messageID, RowLimit: transcriptMessages})
	if err != nil {
		return askContext{}, fmt.Errorf("read the thread transcript: %w", err)
	}
	point, err := pointOfMessage(thread, message)
	if err != nil {
		return askContext{}, err
	}
	return askContext{thread: threadID, message: messageID, transcript: transcriptOf(earlier, answers), point: point}, nil
}

func pointOfMessage(thread Thread, message Message) (revisions.Point, error) {
	var point revisions.Point
	blockID := thread.BlockID
	if message.BlockID != "" {
		blockID = message.BlockID
	}
	if thread.Anchor != nil && blockID == thread.BlockID {
		raw, err := json.Marshal(thread.Anchor)
		if err != nil {
			return point, fmt.Errorf("encode anchor: %w", err)
		}
		point.Anchor = raw
	}
	if blockID != "" {
		id, err := db.ParseUUID(blockID)
		if err != nil {
			return point, fmt.Errorf("block id: %w", err)
		}
		point.BlockID = id
	}
	return point, nil
}

func transcriptOf(newestFirst []dbsqlc.ThreadMessage, answers map[pgtype.UUID]Answer) string {
	if len(newestFirst) == 0 {
		return ""
	}
	lines := make([]string, 0, len(newestFirst))
	for _, row := range slices.Backward(newestFirst) {
		message := messageOf(row, nil, auth.Principal{}, false)
		lines = append(lines, message.Author.Label()+": "+transcriptBody(message.Body, answerOf(answers, row)))
	}
	text := strings.Join(lines, "\n\n")
	if n := utf8.RuneCountInString(text); n > transcriptRunes {
		text = strings.TrimSpace(string([]rune(text)[n-transcriptRunes:]))
	}
	return text
}

func transcriptBody(body string, answer *Answer) string {
	if answer == nil {
		return body
	}
	text := strings.TrimSpace(answer.Title + "\n\n" + answer.Markdown)
	if runes := []rune(text); len(runes) > answerRunes {
		return strings.TrimSpace(string(runes[:answerRunes-len("...")])) + "..."
	}
	return text
}
