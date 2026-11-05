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

func TestMissingSpecErrorIsTheSameOnBothSurfaces(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	for _, tool := range []string{nameCreateDashboard, nameUpdateDashboard} {
		t.Run(tool, func(t *testing.T) {
			mcp, err := call(t, r, tool, `{}`)
			if err != nil {
				t.Fatal(err)
			}
			base := auth.WithPrincipal(context.Background(), auth.Principal{Role: auth.RoleAdmin})
			ctx := WithRun(base, Run{Mode: ModeCreate})
			if tool == nameUpdateDashboard {
				ctx = WithRun(base, Run{Mode: ModeRevise, DashboardID: dashA, ParentVersion: 1})
			}
			inRun, err := r.Call(ctx, tool, []byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			for label, res := range map[string]Result{"MCP": mcp, "in-run": inRun} {
				if got := fmt.Sprint(res.Content.(errorBody).Message); got != missingSpec {
					t.Errorf("%s message = %q, want %q", label, got, missingSpec)
				}
			}
		})
	}
}

func requiredOf(t *testing.T, schema json.RawMessage) []string {
	t.Helper()
	var parsed struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed.Required
}

func TestSaveAndReadToolsRequireTheDashboardOnlyForMCP(t *testing.T) {
	byName := map[string]Tool{}
	for _, tool := range New(Deps{}).List() {
		byName[tool.Name] = tool
	}
	for _, tc := range []struct{ tool, mcp, agent string }{
		{nameUpdateDashboard, "[dashboard_id parent_version spec]", "[spec]"},
		{nameGetPanels, "[dashboard_id ids]", "[ids]"},
		{nameUpdatePanels, "[dashboard_id parent_version summary]", "[summary]"},
	} {
		tool := byName[tc.tool]
		if got := fmt.Sprint(requiredOf(t, tool.InputSchema)); got != tc.mcp {
			t.Errorf("%s MCP required = %s, want %s", tc.tool, got, tc.mcp)
		}
		if got := fmt.Sprint(requiredOf(t, tool.ForAgent().InputSchema)); got != tc.agent {
			t.Errorf("%s agent required = %s, want %s", tc.tool, got, tc.agent)
		}
	}
}

func TestUpdateToolsSteerEachOtherByName(t *testing.T) {
	byName := map[string]Tool{}
	for _, tool := range New(Deps{}).List() {
		byName[tool.Name] = tool
	}
	for _, tc := range []struct{ tool, want string }{
		{nameUpdateDashboard, "use update_panels"},
		{nameUpdatePanels, "or every query when the default time range changes"},
		{nameGetPanels, "before changing it with update_panels"},
	} {
		if got := byName[tc.tool].ForAgent().Description; !strings.Contains(got, tc.want) {
			t.Errorf("%s agent description lost %q: %s", tc.tool, tc.want, got)
		}
	}
}

func TestOnlyTheBuiltInAgentIsOfferedFinishAndPlannedPanels(t *testing.T) {
	checked := 0
	for _, tool := range New(Deps{}).List() {
		switch tool.Name {
		case nameCreateDashboard, nameUpdateDashboard, nameUpdatePanels:
		default:
			continue
		}
		checked++
		var mcp, agent struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &mcp); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(tool.ForAgent().InputSchema, &agent); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"finish", "planned_panels"} {
			if _, ok := mcp.Properties[field]; ok {
				t.Errorf("%s: the MCP schema offers %s", tool.Name, field)
			}
			if _, ok := agent.Properties[field]; !ok {
				t.Errorf("%s: the agent schema lacks %s", tool.Name, field)
			}
		}
		for _, schema := range []json.RawMessage{tool.InputSchema, tool.ForAgent().InputSchema} {
			if strings.Contains(string(schema), `"plan"`) {
				t.Errorf("%s: a schema still has a plan property", tool.Name)
			}
		}
		if strings.Contains(tool.Description, "build that people watch") {
			t.Errorf("%s: the MCP description carries the agent's build guidance", tool.Name)
		}
		if !strings.Contains(tool.ForAgent().Description, "Each save adds or changes exactly one panel.") {
			t.Errorf("%s: the agent description lacks the build guidance", tool.Name)
		}
	}
	if checked != 3 {
		t.Fatalf("checked %d save tools, want 3", checked)
	}
}

func schemaProperties(t *testing.T, schema json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed.Properties
}

func upsertMaxItems(t *testing.T, schema json.RawMessage) int {
	t.Helper()
	var upsert struct {
		MaxItems int `json:"maxItems"`
	}
	if err := json.Unmarshal(schemaProperties(t, schema)["upsert"], &upsert); err != nil {
		t.Fatal(err)
	}
	return upsert.MaxItems
}

func TestOnlyTheBuiltInSavesNameARowAndUpdatePanelsHoldsOnePanel(t *testing.T) {
	byName := map[string]Tool{}
	for _, tool := range New(Deps{}).List() {
		byName[tool.Name] = tool
	}
	updatePanels := byName[nameUpdatePanels]
	if _, ok := schemaProperties(t, updatePanels.InputSchema)["row"]; ok {
		t.Error("the MCP update_panels schema offers row")
	}
	if got := upsertMaxItems(t, updatePanels.InputSchema); got != 50 {
		t.Errorf("MCP upsert maxItems = %d, want 50", got)
	}
	if _, ok := schemaProperties(t, updatePanels.ForAgent().InputSchema)["row"]; !ok {
		t.Error("the agent update_panels schema lacks row")
	}
	if got := upsertMaxItems(t, updatePanels.ForAgent().InputSchema); got != 1 {
		t.Errorf("agent upsert maxItems = %d, want 1", got)
	}
	if _, ok := schemaProperties(t, byName[nameCreateDashboard].InputSchema)["row"]; ok {
		t.Error("the MCP create_dashboard schema offers row")
	}
	if _, ok := schemaProperties(t, byName[nameCreateDashboard].ForAgent().InputSchema)["row"]; !ok {
		t.Error("the agent create_dashboard schema lacks row")
	}
	if _, ok := schemaProperties(t, byName[nameUpdateDashboard].ForAgent().InputSchema)["row"]; ok {
		t.Error("the agent update_dashboard schema offers row")
	}
}

func TestTheAgentAndMCPDescribeGetSpecGuideDifferently(t *testing.T) {
	var guide Tool
	for _, tool := range New(Deps{}).List() {
		if tool.Name == nameGetSpecGuide {
			guide = tool
		}
	}
	if guide.MCPOnly {
		t.Fatal("the built-in agent is not offered get_spec_guide")
	}
	if !strings.Contains(guide.Description, "authoring guide, JSON Schema and an example") {
		t.Errorf("MCP description = %q", guide.Description)
	}
	agent := guide.ForAgent().Description
	for _, want := range []string{"ObserveSpec JSON Schema", "already in your instructions", "schema issue you cannot fix from the guide"} {
		if !strings.Contains(agent, want) {
			t.Errorf("agent description lacks %q: %s", want, agent)
		}
	}
	if agent == guide.Description {
		t.Error("the agent and MCP descriptions are the same")
	}
}

const agentToolTextBudget = 19609

func TestAgentToolTextStaysWithinBudget(t *testing.T) {
	total := 0
	for _, tool := range New(Deps{}).List() {
		if tool.MCPOnly {
			continue
		}
		offered := tool.ForAgent()
		total += len(offered.Description) + len(offered.InputSchema)
	}
	if total > agentToolTextBudget {
		t.Errorf("agent tool text is %d bytes, over the %d byte budget; raise the budget deliberately and say why in the commit body", total, agentToolTextBudget)
	}
}

func TestOnlyTheBuiltInAgentIsToldToReadItsBuildWithoutADashboardID(t *testing.T) {
	var getPanels Tool
	for _, tool := range New(Deps{}).List() {
		if tool.Name == nameGetPanels {
			getPanels = tool
		}
	}
	if !strings.Contains(getPanels.ForAgent().Description, "Leave out dashboard_id") {
		t.Errorf("agent description = %q", getPanels.ForAgent().Description)
	}
	if strings.Contains(getPanels.Description, "Leave out dashboard_id") {
		t.Errorf("MCP description = %q", getPanels.Description)
	}
}
