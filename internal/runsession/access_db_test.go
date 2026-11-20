package runsession

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

func TestTheScopeOfARunReadsDatasourcesAsItsRequesterAndNothingMore(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	run, err := r.svc.Scope(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if run.Access.System || run.Access.By.ID != req.RequestedBy.ID || run.Access.By.Role != req.RequestedBy.Role || run.Access.Audience != tools.RunAudience(req) {
		t.Fatalf("run access = %+v, want the requester %+v", run.Access, req.RequestedBy)
	}
}
