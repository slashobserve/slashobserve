package revisions

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/origin"
)

func runEventFor(row dbsqlc.AgentRequest, a Action) (string, bool) {
	if SourceOf(row) == SourceSuggest {
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

const builtInAgentName = "Observe Agent"

func AssignedAgent(row dbsqlc.AgentRequest, assigneeName string) audit.Agent {
	id := uuid.NullUUID{UUID: row.AssignedID.Bytes, Valid: row.AssignedID.Valid}
	switch AssigneeKind(row.AssignedKind) {
	case AssignBuiltin:
		return audit.Agent{Name: builtInAgentName}
	case AssignGrant:
		return audit.Agent{ID: id, Name: origin.CleanClient(assigneeName)}
	}
	return audit.Agent{ID: id, Name: assigneeName}
}

func takenByAgent(row dbsqlc.AgentRequest, typ string) bool {
	switch typ {
	case activity.TypeRunStarted, activity.TypeRunSucceeded, activity.TypeRunFailed, activity.TypeRunCanceled, activity.TypeRunReleased:
		return row.StartedAt.Valid
	}
	return false
}

func assigneeNameOf(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest, typ string) (string, error) {
	if AssigneeKind(row.AssignedKind) == AssignBuiltin || !takenByAgent(row, typ) {
		return "", nil
	}
	name, err := q.GetAssigneeName(ctx, dbsqlc.GetAssigneeNameParams{Kind: row.AssignedKind, ID: row.AssignedID})
	if err != nil {
		return "", fmt.Errorf("read the agent that took the run: %w", err)
	}
	return name, nil
}

func runActivity(row dbsqlc.AgentRequest, typ string) activity.Entry {
	return runActivityWith(row, typ, "", nil)
}

func runActivityWith(row dbsqlc.AgentRequest, typ string, assigneeName string, extra map[string]any) activity.Entry {
	columns := requesterColumns(row)
	data := map[string]any{"intent": row.Intent}
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
	var recorded map[string]any
	switch typ {
	case activity.TypeRunSucceeded, activity.TypeRunFailed, activity.TypeRunCanceled:
		if cost, ok := storedCost(row.RunUsage); ok {
			data["cost_usd"] = cost
		}
		recorded = storedModelUse(row.RunUsage)
	}
	maps.Copy(data, extra)
	entry := activity.Entry{
		Type:      typ,
		TargetID:  row.ID,
		Owner:     runOwner(row),
		Actor:     actor.FromColumns(columns),
		Data:      data,
		AuditOnly: recorded,
	}
	if takenByAgent(row, typ) {
		entry.Agent = AssignedAgent(row, assigneeName)
	}
	return entry
}

func runOwner(row dbsqlc.AgentRequest) activity.Owner {
	if PrivateAsk(row) {
		return activity.Owner{}
	}
	return activity.OnDashboard(row.DashboardID)
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

func storedModelUse(runUsage []byte) map[string]any {
	var usage struct {
		Model        string `json:"model"`
		InputTokens  *int64 `json:"input_tokens"`
		OutputTokens *int64 `json:"output_tokens"`
	}
	if err := json.Unmarshal(runUsage, &usage); err != nil {
		return nil
	}
	use := map[string]any{}
	if usage.Model != "" {
		use["model"] = usage.Model
	}
	if usage.InputTokens != nil {
		use["input_tokens"] = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		use["output_tokens"] = *usage.OutputTokens
	}
	return use
}

func revisionEvent(row dbsqlc.AgentRequest) (events.Event, bool) {
	if !row.DashboardID.Valid || Intent(row.Intent) != IntentApply {
		return events.Event{}, false
	}
	ev := events.Event{
		Type:        events.TypeRevisionUpdated,
		DashboardID: row.DashboardID.String(),
		RevisionID:  row.ID.String(),
		Status:      string(State(row.State).Status()),
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
	if err := attachChatDashboard(ctx, q, row, a); err != nil {
		return err
	}
	typ, ok := runEventFor(row, a)
	if !ok {
		return notifyRevision(ctx, q, row)
	}
	var extra map[string]any
	if typ == activity.TypeRunSucceeded && isHealthRepair(row) {
		outcome, found, err := readRepairOutcome(ctx, q, row)
		if err != nil {
			return err
		}
		if found {
			extra = outcome.data()
		}
	}
	assigneeName, err := assigneeNameOf(ctx, q, row, typ)
	if err != nil {
		return err
	}
	entry := runActivityWith(row, typ, assigneeName, extra)
	var live *events.Event
	if ev, ok := revisionEvent(row); ok {
		live = &ev
	}
	_, err = activity.Append(ctx, q, entry, live)
	return err
}

func attachChatDashboard(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest, a Action) error {
	if !row.ChatID.Valid || !row.DashboardID.Valid || a == ActionEnqueue || a == ActionClaim {
		return nil
	}
	if err := q.SetChatSubjectDashboard(ctx, dbsqlc.SetChatSubjectDashboardParams{ID: row.ChatID, DashboardID: row.DashboardID}); err != nil {
		return fmt.Errorf("attach the saved dashboard to its chat: %w", err)
	}
	return nil
}
