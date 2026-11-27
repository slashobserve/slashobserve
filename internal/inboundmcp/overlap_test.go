package inboundmcp

import (
	"context"
	"net/http"
	"testing"
)

type overtaken struct {
	Connector
	overtake func(ctx context.Context)
}

func (o *overtaken) Tools(ctx context.Context, target Target) ([]Found, error) {
	found, err := o.Connector.Tools(ctx, target)
	if o.overtake == nil || err != nil {
		return found, err
	}
	overtake := o.overtake
	o.overtake = nil
	overtake(ctx)
	return found[:1], nil
}

func TestADiscoveryOvertakenByANewerOneKeepsTheNewerTools(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"), readOnlyTool("list", "List"))
	r.call(t, http.MethodPost, base, registration("docs", "s3cret-value"))
	wrapped := &overtaken{Connector: r.service.connector}
	r.service.connector = wrapped
	wrapped.overtake = func(ctx context.Context) {
		newer, err := r.service.Discover(ctx, r.admin, "docs")
		if err != nil || newer.Superseded || newer.Tally.Found != 2 {
			t.Errorf("the newer discovery = %+v, %v", newer, err)
		}
	}

	older, err := r.service.Discover(t.Context(), r.admin, "docs")
	if err != nil {
		t.Fatalf("the overtaken discovery failed: %v", err)
	}
	if !older.Superseded || older.Tally != (Tally{}) || older.Source.State != StateReady {
		t.Fatalf("the overtaken discovery = %+v, want it superseded with the stored state", older)
	}
	tools := r.tools(t, "docs")
	if _, kept := tools["list"]; !kept || len(tools) != 2 {
		t.Fatalf("tools after the overlap = %v, want the newer discovery's two", tools)
	}

	found := r.discover(t, "docs")
	if found.Source.State != StateReady || found.Tally.Found != 2 {
		t.Fatalf("a later discovery = %+v", found)
	}
}
