package contextsources

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	actionCreate   audit.Action = "context_source.create"
	actionUpdate   audit.Action = "context_source.update"
	actionDelete   audit.Action = "context_source.delete"
	actionDiscover audit.Action = "context_source.discover"
	actionApprove  audit.Action = "context_source.approve"
	actionRevoke   audit.Action = "context_source.revoke"

	targetKind = "context_source"
)

func targetOf(row dbsqlc.ContextSource) audit.Target {
	return audit.Target{Kind: targetKind, ID: row.Uid, Name: row.Name}
}

func hostData(row dbsqlc.ContextSource, host string) map[string]any {
	data := map[string]any{"host": host}
	if row.AllowPrivate {
		data["private"] = true
	}
	return data
}

func identity(row dbsqlc.ContextSource, host string) map[string]any {
	data := hostData(row, host)
	data["name"], data["audience"] = row.Name, row.Audience
	return data
}

func recordCreated(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.ContextSource, host string) error {
	return audit.Record(ctx, q, audit.Event{Action: actionCreate, Target: targetOf(row), Data: identity(row, host)})
}

func recordDeleted(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.ContextSource) error {
	return audit.Record(ctx, q, audit.Event{Action: actionDelete, Target: targetOf(row), Data: identity(row, hostOfURL(row.Url))})
}

func recordUpdated(ctx context.Context, q *dbsqlc.Queries, before, after dbsqlc.ContextSource, next revision, beforeHost string) error {
	changes := map[string]any{}
	if c := activity.Changed(before.Name, after.Name); c != nil {
		changes["name"] = c
	}
	if c := activity.Changed(before.Description.String, after.Description.String); c != nil {
		changes["description"] = c
	}
	if c := activity.Changed(beforeHost, next.host); c != nil {
		changes["host"] = c
	}
	if c := activity.Changed(before.Audience, after.Audience); c != nil {
		changes["audience"] = c
	}
	if c := activity.Changed(before.AllowPrivate, after.AllowPrivate); c != nil {
		changes["private"] = c
	}
	urlChanged := before.Url != after.Url
	if len(changes) == 0 && !urlChanged && !next.credentials {
		return nil
	}
	data := identity(after, next.host)
	data["changes"] = changes
	if urlChanged {
		data["url_changed"] = true
	}
	if next.credentials {
		data["credential_changed"] = true
	}
	return audit.Record(ctx, q, audit.Event{Action: actionUpdate, Target: targetOf(after), Data: data})
}

func recordDiscovered(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.ContextSource, tally Tally, failure string) error {
	e := audit.Event{Action: actionDiscover, Target: targetOf(row), Data: hostData(row, hostOfURL(row.Url))}
	if failure != "" {
		e.Outcome, e.Reason = audit.OutcomeFailed, failure
	} else {
		e.Data["found"], e.Data["new"], e.Data["changed"], e.Data["gone"], e.Data["ignored"] = tally.Found, tally.New, tally.Changed, tally.Gone, tally.Ignored
	}
	return audit.Record(ctx, q, e)
}

func toolData(row dbsqlc.ContextSource, t Tool) map[string]any {
	data := hostData(row, hostOfURL(row.Url))
	data["tool"], data["hash"] = t.Name, t.Pinned.Hash
	return data
}

func recordTool(ctx context.Context, q *dbsqlc.Queries, action audit.Action, row dbsqlc.ContextSource, t Tool) error {
	return audit.Record(ctx, q, audit.Event{Action: action, Target: targetOf(row), Data: toolData(row, t)})
}

func recordFileApproval(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.ContextSource, t Tool) error {
	data := toolData(row, t)
	data["provisioned"] = true
	return audit.Record(ctx, q, audit.Event{Action: actionApprove, Target: targetOf(row), Data: data})
}
