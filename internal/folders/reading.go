package folders

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func ReadingIn(ctx context.Context, f Folder) context.Context {
	a, ok := datasource.AccessFrom(ctx)
	if !ok || f.AdminsOnly {
		return ctx
	}
	return datasource.WithAccess(ctx, datasource.Access{By: a.By, Audience: datasource.AudienceEveryone})
}

func Read(ctx context.Context, f Folder) {
	if f.AdminsOnly {
		datasource.NoteRead(ctx, datasource.AudienceAdmins)
	}
}

func withholdsAdminsOnly(ctx context.Context, p auth.Principal) bool {
	a, ok := datasource.AccessFrom(ctx)
	return ok && a.By.Kind == p.Kind && a.By.ID == p.ID && !a.Reads(datasource.AudienceAdmins)
}
