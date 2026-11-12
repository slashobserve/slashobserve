package revisions

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/origin"
)

func TestWriteOriginIsTheRunsOwnOnlyForTheBuiltInAgent(t *testing.T) {
	run := origin.Origin{Kind: origin.CLI}
	outside := origin.Origin{Kind: origin.MCP, Client: "claude-code"}
	tests := []struct {
		name     string
		assignee AssigneeKind
		want     origin.Origin
	}{
		{"built-in agent", AssignBuiltin, run},
		{"connected agent", AssignGrant, outside},
		{"service account", AssignServiceAccount, outside},
		{"unassigned", "", outside},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{Origin: run, Assignee: tt.assignee}
			if got := req.WriteOrigin(outside); got != tt.want {
				t.Errorf("WriteOrigin = %+v, want %+v", got, tt.want)
			}
		})
	}
}
