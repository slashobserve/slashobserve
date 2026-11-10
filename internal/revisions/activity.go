package revisions

import (
	"cmp"
	"context"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
)

func runType(row dbsqlc.AgentRequest) (string, bool) {
	if row.Source == string(SourceSuggest) {
		return "", false
	}
	switch Status(row.Status) {
	case StatusQueued:
		return activity.TypeRunQueued, true
	case StatusRunning:
		return activity.TypeRunStarted, true
	case StatusSucceeded:
		return activity.TypeRunSucceeded, true
	case StatusFailed:
		if row.FailureCode.String == string(FailureCanceled) {
			return activity.TypeRunCanceled, true
		}
		return activity.TypeRunFailed, true
	}
	return "", false
}

func runActivity(row dbsqlc.AgentRequest) (activity.Entry, bool) {
	typ, ok := runType(row)
	if !ok {
		return activity.Entry{}, false
	}
	columns := actor.Columns{
		Kind:   row.RequestedByKind,
		ID:     row.RequestedByID,
		Name:   cmp.Or(row.RequestedByName, row.RequestedByEmail.String),
		Origin: row.Origin,
		Client: row.OriginClient,
	}
	data := map[string]any{"source": row.Source}
	switch typ {
	case activity.TypeRunQueued:
		if row.ContinuesID.Valid {
			data["continues_id"] = row.ContinuesID.String()
		}
	case activity.TypeRunStarted:
		columns.RunID = row.ID
		data["attempt"] = row.Attempts
	case activity.TypeRunSucceeded:
		columns.RunID = row.ID
		if row.ResultVersion.Valid {
			data["result_version"] = row.ResultVersion.Int32
		}
	case activity.TypeRunFailed:
		columns.RunID = row.ID
		if row.FailureCode.Valid {
			data["failure_code"] = row.FailureCode.String
		}
	case activity.TypeRunCanceled:
		columns.RunID = row.ID
	}
	return activity.Entry{
		Type:        typ,
		TargetID:    row.ID,
		DashboardID: row.DashboardID,
		Actor:       actor.FromColumns(columns),
		Data:        data,
	}, true
}

func revisionEvent(row dbsqlc.AgentRequest) (events.Event, bool) {
	if !row.DashboardID.Valid || row.Source == string(SourceSuggest) || row.Source == string(SourceInvestigate) {
		return events.Event{}, false
	}
	ev := events.Event{
		Type:        events.TypeRevisionUpdated,
		DashboardID: row.DashboardID.String(),
		RevisionID:  row.ID.String(),
		Status:      row.Status,
	}
	if row.ResultVersion.Valid {
		ev.Version = int(row.ResultVersion.Int32)
	}
	if row.CommentID.Valid {
		ev.CommentID = row.CommentID.String()
	}
	return ev, true
}

func notifyRevision(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) error {
	ev, ok := revisionEvent(row)
	if !ok {
		return nil
	}
	return events.Notify(ctx, q, 0, &ev)
}

func recordRun(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) error {
	entry, ok := runActivity(row)
	if !ok {
		return notifyRevision(ctx, q, row)
	}
	var live *events.Event
	if ev, ok := revisionEvent(row); ok {
		live = &ev
	}
	_, err := activity.Append(ctx, q, entry, live)
	return err
}
