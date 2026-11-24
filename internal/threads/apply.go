package threads

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	actionFindingApply audit.Action = "finding.apply"

	ApplyLead             = "Revise the dashboard: "
	CodeNoSuggestedChange = "no_suggested_change"
)

var (
	ErrAnswerNotHere = errors.New("no answer with this id was given in this thread")

	errApplyOnDashboard  = &InvalidInputError{Code: CodeWrongThread, Message: "a suggested change is applied from a thread on the dashboard it is about"}
	errNoSuggestedChange = &InvalidInputError{Code: CodeNoSuggestedChange, Message: "this answer suggests no change to the dashboard"}
)

type suggestion struct {
	finding  pgtype.UUID
	answered dbsqlc.AgentRequest
	change   string
}

type applying struct {
	asks *Asks
	ctx  context.Context
	by   auth.Principal
	sg   suggestion
	run  *StartedRun
}

func (a *Asks) Apply(ctx context.Context, by auth.Principal, threadID, findingID pgtype.UUID) (Sent, error) {
	row, sc, err := a.service.writableThread(ctx, by, threadID)
	if err != nil {
		return Sent{}, err
	}
	if sc.target.Kind != KindDashboard {
		return Sent{}, errApplyOnDashboard
	}
	sg, err := a.suggestionIn(ctx, row, findingID)
	if err != nil {
		return Sent{}, err
	}
	if earlier, found, err := a.queue.RunApplying(ctx, findingID); err != nil || found {
		if err != nil {
			return Sent{}, err
		}
		return a.appliedBefore(ctx, by, earlier)
	}
	if err := a.queue.CheckRun(ctx, revisions.RunTarget{Intent: revisions.IntentApply, DashboardID: sc.target.ID}, by); err != nil {
		return Sent{}, err
	}
	ap := &applying{asks: a, ctx: ctx, by: by, sg: sg}
	w, err := a.service.reply(ctx, by, row, sc, ReplyInput{Body: ApplyLead + sg.change}, writeOptions{hook: ap.hook})
	if errors.Is(err, revisions.ErrFindingApplied) {
		earlier, found, readErr := a.queue.RunApplying(ctx, findingID)
		if readErr != nil || !found {
			return Sent{}, errors.Join(err, readErr)
		}
		return a.appliedBefore(ctx, by, earlier)
	}
	if err != nil {
		return Sent{}, err
	}
	return Sent{Thread: w.thread, Message: w.message, Run: ap.run, To: ToAgent}, nil
}

func (a *Asks) suggestionIn(ctx context.Context, thread dbsqlc.Thread, findingID pgtype.UUID) (suggestion, error) {
	q := a.service.db.Queries()
	found, err := q.GetFinding(ctx, findingID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return suggestion{}, ErrAnswerNotHere
	case err != nil:
		return suggestion{}, fmt.Errorf("read finding: %w", err)
	case !found.Finding.AgentRequestID.Valid || !found.RequestThreadMessageID.Valid:
		return suggestion{}, ErrAnswerNotHere
	}
	askedIn, err := q.GetThreadOfMessage(ctx, found.RequestThreadMessageID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return suggestion{}, ErrAnswerNotHere
	case err != nil:
		return suggestion{}, fmt.Errorf("read the thread an answer was given in: %w", err)
	case askedIn.ID != thread.ID:
		return suggestion{}, ErrAnswerNotHere
	case !found.Finding.SuggestedChange.Valid:
		return suggestion{}, errNoSuggestedChange
	}
	answered, err := q.GetAgentRequestByID(ctx, found.Finding.AgentRequestID)
	if err != nil {
		return suggestion{}, fmt.Errorf("read the run that answered: %w", err)
	}
	return suggestion{finding: findingID, answered: answered, change: withoutMentionSyntax(found.Finding.SuggestedChange.String)}, nil
}

func (ap *applying) hook(q *dbsqlc.Queries, thread Thread, message Message) error {
	asked, err := ap.asks.service.askedIn(ap.ctx, q, ap.by, thread, message)
	if err != nil {
		return err
	}
	params := revisions.ApplyParams{
		FindingID:       ap.sg.finding,
		DashboardID:     thread.Target.ID,
		Answered:        ap.sg.answered,
		Change:          ap.sg.change,
		CommentID:       asked.thread,
		ThreadMessageID: asked.message,
		ThreadContext:   asked.transcript,
		Point:           asked.point,
		By:              ap.by,
	}
	if thread.PanelPresent {
		params.PanelID = thread.PanelID
	}
	row, err := ap.asks.queue.EnqueueApply(ap.ctx, q, params)
	if err != nil {
		return err
	}
	ap.run = &StartedRun{ID: row.ID.String(), Status: row.Status, AssignedKind: row.AssignedKind}
	return ap.record(q, asked.thread, row)
}

func (ap *applying) record(q *dbsqlc.Queries, threadID pgtype.UUID, run dbsqlc.AgentRequest) error {
	cols, err := actorOf(ap.ctx, ap.by).Columns()
	if err != nil {
		return err
	}
	return audit.Record(ap.ctx, q, audit.Event{
		Action: actionFindingApply,
		Target: audit.Target{Kind: "finding", ID: ap.sg.finding.String(), DashboardID: uuid.NullUUID{UUID: run.DashboardID.Bytes, Valid: true}},
		Thread: uuid.NullUUID{UUID: threadID.Bytes, Valid: true},
		Data:   map[string]any{"run_id": run.ID.String()},
		By:     cols.AuditBy(),
	})
}

func (a *Asks) appliedBefore(ctx context.Context, by auth.Principal, run dbsqlc.AgentRequest) (Sent, error) {
	if !run.ThreadMessageID.Valid {
		return Sent{}, revisions.ErrFindingApplied
	}
	message, err := a.service.ReadMessage(ctx, by, run.ThreadMessageID)
	if err != nil {
		return Sent{}, err
	}
	threadID, err := uuid.Parse(message.ThreadID)
	if err != nil {
		return Sent{}, fmt.Errorf("thread id: %w", err)
	}
	row, sc, err := a.service.threadAndScope(ctx, by, pgtype.UUID{Bytes: threadID, Valid: true})
	if err != nil {
		return Sent{}, err
	}
	thread, err := a.service.present(ctx, row, by, sc)
	if err != nil {
		return Sent{}, err
	}
	started := &StartedRun{ID: run.ID.String(), Status: run.Status, AssignedKind: run.AssignedKind}
	return Sent{Thread: thread, Message: message, Run: started, To: ToAgent, Replayed: true}, nil
}
