package folders

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

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

func recordedGrants(rows []dbsqlc.ListFolderPermissionsRow) []recordedGrant {
	grants := make([]recordedGrant, 0, len(rows))
	for _, r := range rows {
		if r.GroupName.Valid {
			grants = append(grants, recordedGrant{Kind: "group", Name: r.GroupName.String, Level: r.Level})
			continue
		}
		grants = append(grants, recordedGrant{Kind: "user", ID: r.UserID.String(), Name: r.UserName, Level: r.Level})
	}
	return grants
}
