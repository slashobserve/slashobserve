package findings_test

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func reading(ctx context.Context) context.Context {
	return datasource.WithAccess(ctx, datasource.Access{System: true})
}
