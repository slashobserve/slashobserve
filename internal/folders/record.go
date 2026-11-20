package folders

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const maxRecordedGrants = 200

type changedGrant struct {
	recordedGrant
	From string `json:"from_level"`
}

type recordedGrant struct {
	Kind  string `json:"kind"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Level string `json:"level"`
}

func folderTarget(id pgtype.UUID, title string) audit.Target {
	return audit.Target{Kind: "folder", ID: id.String(), Name: title, FolderID: uuid.NullUUID{UUID: id.Bytes, Valid: true}}
}

func recordFolder(ctx context.Context, q *dbsqlc.Queries, action audit.Action, id pgtype.UUID, title string, data map[string]any) error {
	return audit.Record(ctx, q, audit.Event{Action: action, Target: folderTarget(id, title), Data: data})
}

type grantRow = dbsqlc.ListFolderGrantsForAuditRow

func grantKey(r grantRow) string {
	if r.GroupName.Valid {
		return "group:" + r.GroupName.String
	}
	return "user:" + r.UserID.String()
}

func recordedGrantOf(r grantRow) recordedGrant {
	if r.GroupName.Valid {
		return recordedGrant{Kind: "group", Name: r.GroupName.String, Level: r.Level}
	}
	return recordedGrant{Kind: "user", ID: r.UserID.String(), Name: r.UserName, Level: r.Level}
}

func capped[T any](list []T, room *int) []T {
	n := min(len(list), *room)
	*room -= n
	return list[:n]
}

func permissionChange(before, after []grantRow) map[string]any {
	was := make(map[string]grantRow, len(before))
	for _, r := range before {
		was[grantKey(r)] = r
	}
	added, removed, changed := []recordedGrant{}, []recordedGrant{}, []changedGrant{}
	now := make(map[string]bool, len(after))
	for _, r := range after {
		key := grantKey(r)
		now[key] = true
		prev, existed := was[key]
		switch {
		case !existed:
			added = append(added, recordedGrantOf(r))
		case prev.Level != r.Level:
			changed = append(changed, changedGrant{recordedGrant: recordedGrantOf(r), From: prev.Level})
		}
	}
	for _, r := range before {
		if !now[grantKey(r)] {
			removed = append(removed, recordedGrantOf(r))
		}
	}
	truncated := len(added)+len(removed)+len(changed) > maxRecordedGrants
	room := maxRecordedGrants
	return map[string]any{
		"added":        capped(added, &room),
		"removed":      capped(removed, &room),
		"changed":      capped(changed, &room),
		"count_before": len(before),
		"count_after":  len(after),
		"truncated":    truncated,
	}
}
