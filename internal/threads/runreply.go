package threads

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	guard "github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	replyUnclaimed       = "No agent picked this up within 10 minutes."
	replyCanceled        = "The run was stopped before it finished."
	replyFinished        = "The run finished."
	replyAnswered        = "Answered."
	replyRestricted      = "The run ended. Its answer read data only Admins can query, such as the audit trail, and people who are not Admins read this thread, so it is not shown here; only whoever asked and Admins read it, in the run."
	replyAgentFailed     = "the agent reported that it could not finish"
	replyFailedUnstated  = "the run did not finish"
	maxReplyReasonRunes  = 500
	maxReplyHeadingRunes = 200
)

func replyForRun(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest, askedIn pgtype.UUID) error {
	thread, err := q.GetThreadOfMessage(ctx, askedIn)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read thread of the message that started the run: %w", err)
	}
	sc, folder, held, err := lockForReply(ctx, q, thread)
	if err != nil || !held {
		return err
	}
	body, err := placedReplyBody(ctx, q, run, folder)
	if err != nil {
		return err
	}
	author, err := replyAuthor(ctx, q, run)
	if err != nil {
		return err
	}
	columns, err := author.Columns()
	if err != nil {
		return err
	}
	message, err := q.InsertRunReply(ctx, dbsqlc.InsertRunReplyParams{
		ThreadID:     thread.ID,
		Body:         body,
		AuthorKind:   columns.Kind,
		AuthorID:     columns.ID,
		AuthorName:   columns.Name,
		Origin:       columns.Origin,
		OriginClient: columns.Client,
		RunID:        run.ID,

		RunAssignedKind: run.AssignedKind,
		RunFailed:       revisions.State(run.State) == revisions.StateFailed,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("insert run reply: %w", err)
	}
	if err := q.MarkRunReplied(ctx, run.ID); err != nil {
		return fmt.Errorf("mark run replied: %w", err)
	}
	return emitter{scope: sc, thread: thread, actor: author}.replied(ctx, q, message, nil)
}

func replyAuthor(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest) (actor.Actor, error) {
	if revisions.AssigneeKind(run.AssignedKind) == revisions.AssignBuiltin {
		return revisions.AssigneeActor(run, ""), nil
	}
	name, err := q.GetAssigneeName(ctx, dbsqlc.GetAssigneeNameParams{Kind: run.AssignedKind, ID: run.AssignedID})
	if err != nil {
		return actor.Actor{}, fmt.Errorf("read the agent that took the run: %w", err)
	}
	return revisions.AssigneeActor(run, name), nil
}

func lockForReply(ctx context.Context, q *dbsqlc.Queries, thread dbsqlc.Thread) (sc scope, folder pgtype.UUID, held bool, err error) {
	var lock error
	switch {
	case thread.DashboardID.Valid:
		folder, lock = q.LockDashboardKeyShareSkipLocked(ctx, thread.DashboardID)
		sc = scope{target: Target{Kind: KindDashboard, ID: thread.DashboardID}, dashboard: thread.DashboardID, activity: activity.OnDashboard(thread.DashboardID)}
	case thread.ObservationID.Valid:
		folder, lock = q.LockObservationKeyShareSkipLocked(ctx, thread.ObservationID)
		sc = scope{target: Target{Kind: KindObservation, ID: thread.ObservationID}, activity: activity.OnObservation(thread.ObservationID)}
	default:
		return scope{}, pgtype.UUID{}, false, nil
	}
	if errors.Is(lock, pgx.ErrNoRows) {
		return scope{}, pgtype.UUID{}, false, nil
	}
	if lock != nil {
		return scope{}, pgtype.UUID{}, false, fmt.Errorf("lock thread target: %w", lock)
	}
	return sc, folder, true, nil
}

func placedReplyBody(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest, folder pgtype.UUID) (string, error) {
	err := guard.Answer(ctx, q, run.ID, folder)
	if errors.Is(err, folders.ErrRestrictedAnswer) {
		return replyRestricted, nil
	}
	if err != nil {
		return "", err
	}
	return replyBody(ctx, q, run)
}

func replyBody(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest) (string, error) {
	switch revisions.State(run.State) {
	case revisions.StateSucceeded:
		return succeededBody(ctx, q, run)
	case revisions.StateCanceled:
		return replyCanceled, nil
	}
	return failedBody(run), nil
}

func succeededBody(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest) (string, error) {
	switch {
	case run.ResultVersion.Valid:
		return fmt.Sprintf("Saved version %d of this dashboard.", run.ResultVersion.Int32), nil
	case run.ResultKind == string(revisions.ResultProposal):
		title, err := q.GetRunProposalTitle(ctx, run.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "Opened a proposal.", nil
		}
		if err != nil {
			return "", fmt.Errorf("read proposal of run: %w", err)
		}
		return "Opened a proposal: " + sentenceEnd(activity.Excerpt(title, maxReplyHeadingRunes)), nil
	case run.ResultKind == string(revisions.ResultObservation):
		_, err := q.GetFindingByAgentRequest(ctx, run.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return replyFinished, nil
		}
		if err != nil {
			return "", fmt.Errorf("read finding of run: %w", err)
		}
		return replyAnswered, nil
	}
	return replyFinished, nil
}

func failedBody(run dbsqlc.AgentRequest) string {
	switch revisions.FailureCode(run.FailureCode.String) {
	case revisions.FailureAgentUnavailable:
		return replyUnclaimed
	case revisions.FailureAgentFailed:
		return "The run failed: " + sentenceEnd(replyAgentFailed)
	}
	reason := clip(strings.TrimSpace(run.Error.String), maxReplyReasonRunes)
	if reason == "" {
		reason = replyFailedUnstated
	}
	return "The run failed: " + sentenceEnd(reason)
}

func sentenceEnd(text string) string {
	if strings.HasSuffix(text, "...") {
		return text
	}
	return strings.TrimRight(text, ".") + "."
}

func clip(text string, limit int) string {
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit])
	}
	return text
}
