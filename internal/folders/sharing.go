package folders

import (
	"context"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func recordSharing(ctx context.Context, q *dbsqlc.Queries, folder dbsqlc.Folder, adminsOnly bool) error {
	return audit.Record(ctx, q, audit.Event{
		Action: "folder.permissions_change",
		Target: audit.Target{Kind: "folder", ID: folder.ID.String(), Name: folder.Title, FolderID: uuid.NullUUID{UUID: folder.ID.Bytes, Valid: true}},
		Data:   map[string]any{"admins_only": adminsOnly, "admins_only_before": AdminsOnly(folder)},
	})
}
