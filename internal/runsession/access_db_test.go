package runsession

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestARunsToolCallsReadDatasourcesAsItsRequesterWithoutAnyOtherAccess(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	var seen datasource.Access
	var set bool
	r.call(req, "list_metrics", `{"q":"x"}`, nil, func(ctx context.Context) (tools.Result, error) {
		seen, set = datasource.AccessFrom(ctx)
		return readOK(ctx)
	})
	if !set || seen.System || seen.By.ID != req.RequestedBy.ID || seen.By.Role != req.RequestedBy.Role || seen.Audience != tools.RunAudience(req) {
		t.Fatalf("tool call access = %+v (set %v), want the requester %+v", seen, set, req.RequestedBy)
	}
}
