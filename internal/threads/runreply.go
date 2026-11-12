package threads

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	replyUnclaimed       = "No agent picked this up within 10 minutes."
	replyCanceled        = "The run was canceled."
	replyFinished        = "The run finished."
	replyAgentFailed     = "the agent reported that it could not finish"
	replyFailedUnstated  = "the run did not finish"
	maxReplyReasonRunes  = 500
	maxReplyHeadingRunes = 200
)

func replyForRun(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest) error {
	thread, err := q.GetThreadOfMessage(ctx, run.ThreadMessageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read thread of the message that started the run: %w", err)
	}
	sc, held, err := lockForReply(ctx, q, thread)
	if err != nil || !held {
		return err
	}
	body, err := replyBody(ctx, q, run, sc.target.Kind)
	if err != nil {
		return err
	}
	author := revisions.RunActor(run)
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
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("insert run reply: %w", err)
	}
	return emitter{scope: sc, thread: thread, actor: author}.replied(ctx, q, message, nil)
}

func lockForReply(ctx context.Context, q *dbsqlc.Queries, thread dbsqlc.Thread) (sc scope, held bool, err error) {
	var lock error
	switch {
	case thread.DashboardID.Valid:
		_, lock = q.LockDashboardKeyShareSkipLocked(ctx, thread.DashboardID)
		sc = scope{target: Target{Kind: KindDashboard, ID: thread.DashboardID}, dashboard: thread.DashboardID, activity: activity.OnDashboard(thread.DashboardID)}
	case thread.ObservationID.Valid:
		_, lock = q.LockObservationKeyShareSkipLocked(ctx, thread.ObservationID)
		sc = scope{target: Target{Kind: KindObservation, ID: thread.ObservationID}, activity: activity.OnObservation(thread.ObservationID)}
	default:
		return scope{}, false, nil
	}
	if errors.Is(lock, pgx.ErrNoRows) {
		return scope{}, false, nil
	}
	if lock != nil {
		return scope{}, false, fmt.Errorf("lock thread target: %w", lock)
	}
	return sc, true, nil
}

func replyBody(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest, target Kind) (string, error) {
	switch revisions.State(run.State) {
	case revisions.StateSucceeded:
		return succeededBody(ctx, q, run, target)
	case revisions.StateCanceled:
		return replyCanceled, nil
	}
	return failedBody(run), nil
}

func succeededBody(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest, target Kind) (string, error) {
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
		return "Opened a proposal: " + sentenceEnd(clip(title, maxReplyHeadingRunes)), nil
	case run.ResultKind == string(revisions.ResultObservation):
		found, err := q.GetFindingByAgentRequest(ctx, run.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return replyFinished, nil
		}
		if err != nil {
			return "", fmt.Errorf("read finding of run: %w", err)
		}
		lead := "Saved a finding: "
		if target == KindObservation {
			lead = "Wrote the answer into this observation: "
		}
		return lead + sentenceEnd(clip(found.Finding.Title, maxReplyHeadingRunes)), nil
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
	return strings.TrimRight(text, ".") + "."
}

func clip(text string, limit int) string {
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit])
	}
	return text
}
