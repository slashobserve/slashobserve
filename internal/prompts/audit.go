package prompts

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	actionCreate  audit.Action = "saved_prompt.create"
	actionUpdate  audit.Action = "saved_prompt.update"
	actionShare   audit.Action = "saved_prompt.share"
	actionUnshare audit.Action = "saved_prompt.unshare"
	actionDelete  audit.Action = "saved_prompt.delete"

	targetKind = "saved_prompt"
)

type promptState struct {
	id     pgtype.UUID
	owner  pgtype.UUID
	name   string
	body   string
	shared bool
}

func recordCreated(ctx context.Context, q *dbsqlc.Queries, now promptState) (bool, error) {
	if !now.shared {
		return false, nil
	}
	return true, recordEvent(ctx, q, actionCreate, now, "", nil)
}

func recordChanged(ctx context.Context, q *dbsqlc.Queries, before, now promptState) (bool, error) {
	changed := changedFields(before, now)
	named, action := now, actionUpdate
	data := map[string]any{"changed": changed}
	switch {
	case before.shared && now.shared:
		data["from_name"] = before.name
	case now.shared:
		action = actionShare
	case before.shared:
		named, action = before, actionUnshare
	default:
		return false, nil
	}
	reason := ""
	if len(changed) == 0 {
		reason = audit.ReasonNoChange
	}
	return true, recordEvent(ctx, q, action, named, reason, data)
}

func recordDeleted(ctx context.Context, q *dbsqlc.Queries, before promptState) (bool, error) {
	if !before.shared {
		return false, nil
	}
	return true, recordEvent(ctx, q, actionDelete, before, "", nil)
}

func changedFields(before, now promptState) []string {
	changed := []string{}
	if before.name != now.name {
		changed = append(changed, "name")
	}
	if before.body != now.body {
		changed = append(changed, "body")
	}
	if before.shared != now.shared {
		changed = append(changed, "shared")
	}
	return changed
}

func recordEvent(ctx context.Context, q *dbsqlc.Queries, action audit.Action, p promptState, reason string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	data["owner_id"] = p.owner.String()
	return audit.Record(ctx, q, audit.Event{
		Action: action,
		Reason: reason,
		Target: audit.Target{Kind: targetKind, ID: p.id.String(), Name: p.name},
		Data:   data,
	})
}
