package mcp

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

func TestARefusedToolCallRecordsTheRoleAndAccessItNeeded(t *testing.T) {
	cases := []struct {
		name    string
		content map[string]string
		want    map[string]any
	}{
		{"a role", map[string]string{"code": "forbidden_role", "needed_role": "editor"}, map[string]any{"needed_role": "editor"}},
		{"folder access", map[string]string{"code": "forbidden_role", "needed_access": "edit", "message": "x"}, map[string]any{"needed_access": "edit"}},
		{"nothing named", map[string]string{"code": "forbidden_role"}, map[string]any{}},
		{"a failure that is not a refusal", map[string]string{"code": "not_found", "needed_role": "editor"}, map[string]any{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.handler.(*handler).call = answering(tools.Result{IsError: true, Content: c.content}, nil)
			a := mountAudited(t, f.handler)
			a.call(t, goodKey, "get_spec_guide", nil)
			var data map[string]any
			if err := json.Unmarshal([]byte(a.only(t, "agent.tool_call").data), &data); err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(data, c.want) {
				t.Fatalf("data = %v, want %v", data, c.want)
			}
		})
	}
}
