package datasourceadmin

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func registered(t *testing.T, reg *datasource.Registry, uid string) (datasource.Instance, bool) {
	t.Helper()
	in, err := reg.For(datasource.WithAccess(t.Context(), datasource.Access{System: true}), uid)
	return in, err == nil
}
