package datasourceadmin

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/placement"
)

func refuseSharedHolders(ctx context.Context, q *dbsqlc.Queries, before datasource.Audience, def Definition) error {
	if def.Audience != datasource.AudienceAdmins || before == datasource.AudienceAdmins {
		return nil
	}
	holders, err := placement.SharedHolders(ctx, q, def.UID)
	if err != nil || len(holders) == 0 {
		return err
	}
	items, hidden := []folders.ContentItem{}, 0
	for _, h := range holders {
		if h.Personal {
			hidden++
			continue
		}
		items = append(items, h.ContentItem)
	}
	return folders.RestrictedAudience(def.UID, items, hidden)
}
