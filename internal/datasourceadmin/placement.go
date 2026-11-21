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
	c, err := placement.SharedCensus(ctx, q, def.UID)
	if err != nil || !c.Held() {
		return err
	}
	items := make([]folders.ContentItem, len(c.Listed))
	for i, h := range c.Listed {
		items[i] = h.ContentItem
	}
	return folders.RestrictedAudience(def.UID, items, c.Unlisted(), c.Personal)
}
