package folders

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

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
	return ok && a.Audience == datasource.AudienceEveryone && a.By.Kind == p.Kind && a.By.ID == p.ID
}

func (a *Access) BuiltIn(ctx context.Context) (map[pgtype.UUID]bool, error) {
	refs, err := a.db.Queries().ListStarterContentRefs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the content the server seeded: %w", err)
	}
	builtIn := make(map[pgtype.UUID]bool, len(refs))
	for _, ref := range refs {
		builtIn[ref] = true
	}
	return builtIn, nil
}
