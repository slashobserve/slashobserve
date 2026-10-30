package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

var leftoverPlaceholder = regexp.MustCompile(`\{[a-z_]+\}`)

func TestBuiltInToolsNeverMentionToolsTheyLack(t *testing.T) {
	all := New(Deps{}).List()
	var absent []string
	for _, tool := range all {
		if tool.MCPOnly {
			absent = append(absent, tool.Name)
		}
	}
	if len(absent) == 0 {
		t.Fatal("no MCP-only tools to check against")
	}
	checked := 0
	for _, tool := range all {
		if tool.MCPOnly {
			continue
		}
		checked++
		offered := tool.ForAgent()
		text := offered.Description + string(offered.InputSchema)
		for _, name := range absent {
			if strings.Contains(text, name) {
				t.Errorf("%s: the built-in agent's description or schema mentions %s, which it does not have", tool.Name, name)
			}
		}
		if !json.Valid(offered.InputSchema) {
			t.Errorf("%s: built-in schema is not valid JSON: %s", tool.Name, offered.InputSchema)
		}
	}
	if checked == 0 {
		t.Fatal("no built-in tools checked")
	}
}

func TestToolTextHasNoPlaceholderLeft(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		for label, text := range map[string]string{
			"mcp description":   tool.Description,
			"mcp schema":        string(tool.InputSchema),
			"agent description": tool.ForAgent().Description,
			"agent schema":      string(tool.ForAgent().InputSchema),
		} {
			if m := leftoverPlaceholder.FindString(text); m != "" {
				t.Errorf("%s %s: placeholder %s left unfilled", tool.Name, label, m)
			}
		}
	}
}

func TestMCPToolsKeepTheGuideAndFolderWording(t *testing.T) {
	byName := map[string]Tool{}
	for _, tool := range New(Deps{}).List() {
		byName[tool.Name] = tool
	}
	for _, tc := range []struct{ tool, want string }{
		{nameCreateDashboard, "call get_spec_guide first if you have not read it"},
		{nameCreateDashboard, "Pass folder_id (see list_folders)"},
		{nameCreateDashboard, `"folder_id"`},
		{nameLintQuery, "using the macros in get_spec_guide."},
		{nameUpdateDashboard, "see get_spec_guide."},
	} {
		tool := byName[tc.tool]
		if !strings.Contains(tool.Description+string(tool.InputSchema), tc.want) {
			t.Errorf("%s: MCP text lost %q", tc.tool, tc.want)
		}
	}
}

func TestBuiltInCreateDashboardPointsAtTheGuideInInstructions(t *testing.T) {
	var create Tool
	for _, tool := range New(Deps{}).List() {
		if tool.Name == nameCreateDashboard {
			create = tool.ForAgent()
		}
	}
	if !strings.Contains(create.Description, "follow the authoring guide in your instructions") {
		t.Errorf("description = %q", create.Description)
	}
	if strings.Contains(string(create.InputSchema), "folder_id") {
		t.Errorf("the built-in agent cannot list folders, so it is offered no folder_id: %s", create.InputSchema)
	}
}

func TestMissingSpecErrorNamesOnlyWhatTheCallerHas(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	for _, tool := range []string{nameCreateDashboard, nameUpdateDashboard} {
		t.Run(tool, func(t *testing.T) {
			mcp, err := call(t, r, tool, `{}`)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprint(mcp.Content); !strings.Contains(got, "get_spec_guide") {
				t.Errorf("MCP error = %s", got)
			}
			base := auth.WithPrincipal(context.Background(), auth.Principal{Role: auth.RoleAdmin})
			ctx := WithRun(base, Run{Mode: ModeCreate})
			if tool == nameUpdateDashboard {
				ctx = WithRun(base, Run{Mode: ModeRevise, DashboardID: dashA, ParentVersion: 1})
			}
			res, err := r.Call(ctx, tool, []byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprint(res.Content)
			if strings.Contains(got, "get_spec_guide") || !strings.Contains(got, "authoring guide in your instructions") {
				t.Errorf("built-in error = %s", got)
			}
		})
	}
}
