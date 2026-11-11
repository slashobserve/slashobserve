package revisions

import (
	"cmp"
	"context"
	"encoding/json"

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

func requesterColumns(row dbsqlc.AgentRequest) actor.Columns {
	return actor.Columns{
		Kind:   row.RequestedByKind,
		ID:     row.RequestedByID,
		Name:   cmp.Or(row.RequestedByName, row.RequestedByEmail.String),
		Origin: row.Origin,
		Client: row.OriginClient,
	}
}

func RunActor(row dbsqlc.AgentRequest) actor.Actor {
	columns := requesterColumns(row)
	columns.RunID = row.ID
	return actor.FromColumns(columns)
}

func runActivity(row dbsqlc.AgentRequest) (activity.Entry, bool) {
	typ, ok := runType(row)
	if !ok {
		return activity.Entry{}, false
	}
	columns := requesterColumns(row)
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
	switch typ {
	case activity.TypeRunSucceeded, activity.TypeRunFailed, activity.TypeRunCanceled:
		if cost, ok := storedCost(row.RunUsage); ok {
			data["cost_usd"] = cost
		}
	}
	return activity.Entry{
		Type:     typ,
		TargetID: row.ID,
		Owner:    activity.OnDashboard(row.DashboardID),
		Actor:    actor.FromColumns(columns),
		Data:     data,
	}, true
}

func storedCost(runUsage []byte) (float64, bool) {
	var usage struct {
		CostUSD *float64 `json:"cost_usd"`
	}
	if err := json.Unmarshal(runUsage, &usage); err != nil || usage.CostUSD == nil {
		return 0, false
	}
	return *usage.CostUSD, true
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
