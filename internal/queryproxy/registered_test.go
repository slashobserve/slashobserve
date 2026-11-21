package queryproxy

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func registered(t *testing.T, reg *datasource.Registry, uid string) (datasource.Instance, bool) {
	t.Helper()
	in, err := reg.For(datasource.WithAccess(t.Context(), datasource.Access{System: true}), uid)
	return in, err == nil
}

func reading(ctx context.Context) context.Context {
	return datasource.WithAccess(ctx, datasource.Access{System: true})
}
