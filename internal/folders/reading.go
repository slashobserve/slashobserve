package folders

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func ReadingIn(ctx context.Context, f Folder) context.Context {
	a, ok := datasource.AccessFrom(ctx)
	if !ok || f.AdminsOnly {
		return ctx
	}
	return datasource.WithAccess(ctx, datasource.Access{By: a.By, Audience: datasource.AudienceEveryone})
}
