package revisions

import (
	"cmp"
	"context"
	"encoding/json"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/origin"
)

func runEventFor(row dbsqlc.AgentRequest, a Action) (string, bool) {
	if row.Source == string(SourceSuggest) {
		return "", false
	}
	typ := RunMachine.EventFor(a, State(row.State))
	return typ, typ != ""
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

func AssigneeActor(row dbsqlc.AgentRequest, assigneeName string) actor.Actor {
	switch AssigneeKind(row.AssignedKind) {
	case AssignGrant:
		person := actor.FromColumns(requesterColumns(row))
		person.Origin = origin.Origin{Kind: origin.MCP, Client: origin.CleanClient(assigneeName)}
		return person
	case AssignServiceAccount:
		return actor.Actor{Kind: auth.KindServiceAccount, ID: row.AssignedID.String(), Name: assigneeName, Origin: origin.Origin{Kind: origin.MCP}}
	}
	return RunActor(row)
}

func runActivity(row dbsqlc.AgentRequest, typ string) activity.Entry {
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
	case activity.TypeRunReleased:
		columns.RunID = row.ID
		data["attempt"] = row.Attempts + 1
	case activity.TypeRunKept:
		if row.ResultVersion.Valid {
			data["result_version"] = row.ResultVersion.Int32
		}
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
	}
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

func recordRun(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest, a Action) error {
	typ, ok := runEventFor(row, a)
	if !ok {
		return notifyRevision(ctx, q, row)
	}
	entry := runActivity(row, typ)
	var live *events.Event
	if ev, ok := revisionEvent(row); ok {
		live = &ev
	}
	_, err := activity.Append(ctx, q, entry, live)
	return err
}
