package threads

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

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

	transcriptMessages = 20
	transcriptRunes    = 8000
	triggerMention     = "mention"
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

type MentionRuns struct {
	service      *Service
	queue        *revisions.Service
	builtinAgent bool
}

func NewMentionRuns(service *Service, queue *revisions.Service, builtinAgent bool) *MentionRuns {
	return &MentionRuns{service: service, queue: queue, builtinAgent: builtinAgent}
}

func (m *MentionRuns) Open(ctx context.Context, by auth.Principal, in OpenInput, startRuns bool) (Thread, Message, *StartedRun, *RunRefusal, error) {
	run, err := m.prepare(ctx, startRuns, in.Body, func() (Target, error) {
		sc, err := m.service.scopeOf(ctx, by, in.Target)
		return sc.target, err
	}, by)
	if err != nil {
		return Thread{}, Message{}, nil, nil, err
	}
	thread, message, err := m.service.Open(ctx, by, in, run.hook())
	return thread, message, run.started(), run.refusal(), err
}

func (m *MentionRuns) Reply(ctx context.Context, by auth.Principal, threadID pgtype.UUID, body string, startRuns bool) (Thread, Message, *StartedRun, *RunRefusal, error) {
	run, err := m.prepare(ctx, startRuns, body, func() (Target, error) {
		_, sc, err := m.service.writableThread(ctx, by, threadID)
		return sc.target, err
	}, by)
	if err != nil {
		return Thread{}, Message{}, nil, nil, err
	}
	thread, message, err := m.service.Reply(ctx, by, threadID, body, run.hook())
	return thread, message, run.started(), run.refusal(), err
}

type mentionRun struct {
	runs      *MentionRuns
	ctx       context.Context
	by        auth.Principal
	agent     Mention
	refused   *RunRefusal
	queuedRun *StartedRun
}

func (m *MentionRuns) prepare(ctx context.Context, startRuns bool, body string, targetOf func() (Target, error), by auth.Principal) (*mentionRun, error) {
	if !startRuns {
		return nil, nil
	}
	agent, found, err := agentOf(body)
	if err != nil || !found {
		return nil, err
	}
	target, err := targetOf()
	if err != nil {
		return nil, err
	}
	run := &mentionRun{runs: m, ctx: ctx, by: by, agent: agent}
	run.refused, err = run.decide(target)
	return run, err
}

func agentOf(body string) (Mention, bool, error) {
	cleaned, ok := revisions.CleanPrompt(body)
	if !ok {
		return Mention{}, false, nil
	}
	mentions, err := ParseMentions(cleaned)
	if err != nil {
		return Mention{}, false, nil
	}
	switch agents := mentions.Agents(); len(agents) {
	case 0:
		return Mention{}, false, nil
	case 1:
		return agents[0], true, nil
	}
	return Mention{}, false, &InvalidInputError{Code: CodeTooManyAgents, Message: "start a run for one agent at a time: mention only one of @observe, a connected agent or a service account"}
}

func (r *mentionRun) decide(target Target) (*RunRefusal, error) {
	var source revisions.Source
	var dashboardID pgtype.UUID
	switch target.Kind {
	case KindDashboard:
		source, dashboardID = revisions.SourceComment, target.ID
	case KindObservation:
		source = revisions.SourceInvestigate
	default:
		return &RunRefusal{Code: RefusalUnsupportedTarget, Message: "agents can be asked from dashboard and observation threads"}, nil
	}
	if r.agent.Kind() == mentionAgent && !r.runs.builtinAgent {
		return &RunRefusal{Code: RefusalAgentUnavailable, Message: revisions.ErrUnavailable.Error()}, nil
	}
	err := r.runs.queue.CheckRun(r.ctx, source, dashboardID, r.by)
	if errors.Is(err, revisions.ErrForbidden) || errors.Is(err, folders.ErrForbidden) {
		return &RunRefusal{Code: RefusalForbidden, Message: err.Error()}, nil
	}
	return nil, err
}

func (r *mentionRun) hook() MessageHook {
	if r == nil || r.refused != nil {
		return nil
	}
	return func(q *dbsqlc.Queries, thread Thread, message Message) error {
		row, err := r.enqueue(q, thread, message)
		if refusal := capRefusal(err); refusal != nil {
			r.refused = refusal
			return nil
		}
		if err != nil {
			return err
		}
		r.queuedRun = &StartedRun{ID: row.ID.String(), Status: row.Status, AssignedKind: row.AssignedKind}
		return nil
	}
}

func (r *mentionRun) started() *StartedRun {
	if r == nil {
		return nil
	}
	return r.queuedRun
}

func (r *mentionRun) refusal() *RunRefusal {
	if r == nil {
		return nil
	}
	return r.refused
}

func capRefusal(err error) *RunRefusal {
	if errors.Is(err, revisions.ErrTooManyRevisions) || errors.Is(err, revisions.ErrTooManyRequests) || errors.Is(err, revisions.ErrAgentBusy) {
		return &RunRefusal{Code: RefusalTooManyRuns, Message: err.Error()}
	}
	return nil
}

func (r *mentionRun) enqueue(q *dbsqlc.Queries, thread Thread, message Message) (dbsqlc.AgentRequest, error) {
	threadID, err := db.ParseUUID(thread.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("thread id: %w", err)
	}
	messageID, err := db.ParseUUID(message.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("message id: %w", err)
	}
	prompt := withoutMention(message.Body, r.agent)
	if prompt == "" {
		prompt = message.Body
	}
	earlier, err := q.ThreadTranscript(r.ctx, dbsqlc.ThreadTranscriptParams{ThreadID: threadID, BeforeID: messageID, RowLimit: transcriptMessages})
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("read the thread transcript: %w", err)
	}
	transcript := transcriptOf(earlier)
	assignee := r.assignee()
	switch thread.Target.Kind {
	case KindDashboard:
		return r.runs.queue.EnqueueAssigned(r.ctx, q, revisions.EnqueueParams{
			DashboardID:     thread.Target.ID,
			Source:          revisions.SourceComment,
			CommentID:       threadID,
			Prompt:          prompt,
			PanelID:         thread.PanelID,
			BaseVersion:     *thread.Version,
			By:              r.by,
			Assignee:        assignee,
			Trigger:         triggerMention,
			ThreadMessageID: messageID,
			ThreadContext:   transcript,
		})
	case KindObservation:
		return r.runs.queue.EnqueueAssignedInvestigate(r.ctx, q, revisions.InvestigateParams{
			Prompt:          prompt,
			By:              r.by,
			ObservationID:   thread.Target.ID,
			Assignee:        assignee,
			Trigger:         triggerMention,
			ThreadMessageID: messageID,
			ThreadContext:   transcript,
		})
	}
	return dbsqlc.AgentRequest{}, fmt.Errorf("start a run on a %s thread", thread.Target.Kind)
}

func (r *mentionRun) assignee() revisions.Assignee {
	switch r.agent.Kind() {
	case mentionGrant:
		return revisions.Assignee{Kind: revisions.AssignGrant, ID: r.agent.GrantID}
	case mentionServiceAccount:
		return revisions.Assignee{Kind: revisions.AssignServiceAccount, ID: r.agent.ServiceAccountID}
	}
	return revisions.Assignee{Kind: revisions.AssignBuiltin}
}

func transcriptOf(newestFirst []dbsqlc.ThreadMessage) string {
	if len(newestFirst) == 0 {
		return ""
	}
	lines := make([]string, 0, len(newestFirst))
	for _, row := range slices.Backward(newestFirst) {
		message := messageOf(row, nil, auth.Principal{}, false)
		lines = append(lines, message.Author.Label()+": "+message.Body)
	}
	text := strings.Join(lines, "\n\n")
	if n := utf8.RuneCountInString(text); n > transcriptRunes {
		text = strings.TrimSpace(string([]rune(text)[n-transcriptRunes:]))
	}
	return text
}
