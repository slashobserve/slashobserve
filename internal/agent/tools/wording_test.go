package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

var leftoverPlaceholder = regexp.MustCompile(`\{[a-z_]+\}`)

func toolsByName(t *testing.T) map[string]Tool {
	t.Helper()
	byName := map[string]Tool{}
	for _, tool := range New(Deps{}).List() {
		byName[tool.Name] = tool
	}
	if len(byName) == 0 {
		t.Fatal("no tools registered")
	}
	return byName
}

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

func TestSurfacesDifferOnlyByNamedVariants(t *testing.T) {
	base := (&Registry{}).baseEntries()
	declaredBy := map[string][]string{}
	for _, v := range variants {
		for _, placeholder := range v.placeholders() {
			declaredBy[placeholder] = append(declaredBy[placeholder], v.name)
		}
	}
	usedInBase := map[string]bool{}
	listedBy := map[string][]string{}
	for _, e := range base {
		text := e.tool.Description + string(e.tool.InputSchema)
		for _, placeholder := range leftoverPlaceholder.FindAllString(text, -1) {
			usedInBase[placeholder] = true
			if len(declaredBy[placeholder]) != 1 {
				t.Errorf("%s uses %s, which is declared by %v, want exactly one variant", e.tool.Name, placeholder, declaredBy[placeholder])
			}
		}
		for _, v := range variants {
			uses := false
			for _, placeholder := range v.placeholders() {
				uses = uses || strings.Contains(text, placeholder)
			}
			if uses != slices.Contains(v.tools, e.tool.Name) {
				t.Errorf("variant %q lists %s = %t, but the base text of %s uses its placeholders = %t", v.name, e.tool.Name, slices.Contains(v.tools, e.tool.Name), e.tool.Name, uses)
			}
			if uses {
				listedBy[e.tool.Name] = append(listedBy[e.tool.Name], v.name)
			}
		}
	}
	for placeholder, names := range declaredBy {
		if !usedInBase[placeholder] {
			t.Errorf("variants %v declare %s, which no base text uses", names, placeholder)
		}
	}

	for _, tool := range New(Deps{}).List() {
		agent := tool.ForAgent()
		same := tool.Description == agent.Description && string(tool.InputSchema) == string(agent.InputSchema)
		switch {
		case len(listedBy[tool.Name]) == 0 && !same:
			t.Errorf("%s differs between the surfaces and no variant lists it", tool.Name)
		case len(listedBy[tool.Name]) > 0 && same:
			t.Errorf("%s is listed by variants %v and does not differ between the surfaces", tool.Name, listedBy[tool.Name])
		}
		for label, text := range map[string]string{
			"mcp description":   tool.Description,
			"mcp schema":        string(tool.InputSchema),
			"agent description": agent.Description,
			"agent schema":      string(agent.InputSchema),
		} {
			if strings.Contains(text, "—") {
				t.Errorf("%s %s contains an em dash", tool.Name, label)
			}
		}
	}
}

var agentRunWording = regexp.MustCompile(`\bbuild\b|your instructions|planned_panels|"finish"`)

func TestMCPTextNeverMentionsTheAgentsRun(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		for label, text := range map[string]string{"description": tool.Description, "schema": string(tool.InputSchema)} {
			if m := agentRunWording.FindString(text); m != "" {
				t.Errorf("%s MCP %s mentions %q", tool.Name, label, m)
			}
		}
	}
}

func TestGuideVariant(t *testing.T) {
	byName := toolsByName(t)
	for _, name := range []string{nameCreateDashboard, nameUpdateDashboard} {
		tool := byName[name]
		if !strings.Contains(tool.Description, "Read get_spec_guide before your first save.") {
			t.Errorf("%s: the MCP description lacks the guide pointer: %s", name, tool.Description)
		}
		if got := tool.ForAgent().Description; strings.Contains(got, "get_spec_guide") {
			t.Errorf("%s: the agent description names get_spec_guide: %s", name, got)
		}
	}
	guide := byName[nameGetSpecGuide]
	if guide.MCPOnly {
		t.Fatal("the built-in agent is not offered get_spec_guide")
	}
	if !strings.Contains(guide.Description, "authoring guide, JSON Schema and an example") {
		t.Errorf("MCP description = %q", guide.Description)
	}
	if got := guide.ForAgent().Description; !strings.Contains(got, "already in your instructions") {
		t.Errorf("agent description = %q", got)
	}
}

func TestFolderVariant(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		if tool.MCPOnly {
			continue
		}
		_, mcp := schemaProperties(t, tool.InputSchema)["folder_id"]
		_, agent := schemaProperties(t, tool.ForAgent().InputSchema)["folder_id"]
		if mcp != (tool.Name == nameCreateDashboard || tool.Name == nameListFindings) || agent {
			t.Errorf("%s offers folder_id: MCP %t, agent %t", tool.Name, mcp, agent)
		}
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
	byName := toolsByName(t)
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

func TestRunTargetVariant(t *testing.T) {
	byName := toolsByName(t)
	for _, name := range []string{nameUpdateDashboard, nameUpdatePanels} {
		tool := byName[name]
		for _, field := range []string{"dashboard_id", "parent_version"} {
			if _, ok := schemaProperties(t, tool.InputSchema)[field]; !ok {
				t.Errorf("%s: the MCP schema lacks %s", name, field)
			}
			if _, ok := schemaProperties(t, tool.ForAgent().InputSchema)[field]; ok {
				t.Errorf("%s: the agent schema offers %s", name, field)
			}
		}
	}
	var dashboardID struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(schemaProperties(t, byName[nameGetPanels].ForAgent().InputSchema)["dashboard_id"], &dashboardID); err != nil {
		t.Fatal(err)
	}
	if dashboardID.Description != agentPanelsDashboard {
		t.Errorf("agent get_panels dashboard_id description = %q, want %q", dashboardID.Description, agentPanelsDashboard)
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

func TestBuildVariant(t *testing.T) {
	byName := toolsByName(t)
	saves := []string{nameCreateDashboard, nameUpdateDashboard, nameUpdatePanels}
	for _, name := range saves {
		tool := byName[name]
		mcp := schemaProperties(t, tool.InputSchema)
		agent := schemaProperties(t, tool.ForAgent().InputSchema)
		for _, field := range []string{"finish", "planned_panels"} {
			if _, ok := mcp[field]; ok {
				t.Errorf("%s: the MCP schema offers %s", name, field)
			}
			if _, ok := agent[field]; !ok {
				t.Errorf("%s: the agent schema lacks %s", name, field)
			}
		}
		for _, schema := range []json.RawMessage{tool.InputSchema, tool.ForAgent().InputSchema} {
			if strings.Contains(string(schema), `"plan"`) {
				t.Errorf("%s: a schema still has a plan property", name)
			}
		}
		if !strings.HasSuffix(tool.ForAgent().Description, agentBuildNote) {
			t.Errorf("%s: the agent description does not end with the build note", name)
		}
		if strings.Contains(tool.Description, agentBuildNote) {
			t.Errorf("%s: the MCP description carries the build note", name)
		}
	}

	updatePanels := byName[nameUpdatePanels]
	if got := upsertMaxItems(t, updatePanels.InputSchema); got != 50 {
		t.Errorf("MCP upsert maxItems = %d, want 50", got)
	}
	if got := upsertMaxItems(t, updatePanels.ForAgent().InputSchema); got != 1 {
		t.Errorf("agent upsert maxItems = %d, want 1", got)
	}
	for _, tc := range []struct {
		tool      string
		mcp, want bool
	}{
		{nameUpdatePanels, false, true},
		{nameCreateDashboard, false, true},
		{nameUpdateDashboard, false, false},
	} {
		tool := byName[tc.tool]
		if _, ok := schemaProperties(t, tool.InputSchema)["row"]; ok != tc.mcp {
			t.Errorf("%s MCP row property = %t, want %t", tc.tool, ok, tc.mcp)
		}
		if _, ok := schemaProperties(t, tool.ForAgent().InputSchema)["row"]; ok != tc.want {
			t.Errorf("%s agent row property = %t, want %t", tc.tool, ok, tc.want)
		}
	}
}

func TestNeighbourToolsNameEachOther(t *testing.T) {
	byName := toolsByName(t)
	for _, tc := range []struct{ tool, want string }{
		{nameUpdateDashboard, "use update_panels"},
		{nameUpdatePanels, "prefer it to update_dashboard"},
		{nameGetPanels, "get_dashboard returns the whole document"},
		{nameGetPanels, "before changing it with update_panels"},
		{nameGetDashboard, "use get_panels"},
		{nameQueryRange, "Use query_instant instead"},
		{nameQueryInstant, "use query_range"},
		{nameQueryInstant, "It takes no format or max_data_points"},
		{nameLintQuery, "query_range and query_instant do"},
		{nameGetSchema, "sample_rows"},
	} {
		tool := byName[tc.tool]
		for label, got := range map[string]string{"MCP": tool.Description, "agent": tool.ForAgent().Description} {
			if !strings.Contains(got, tc.want) {
				t.Errorf("%s %s description lacks %q: %s", tc.tool, label, tc.want, got)
			}
		}
	}
}

var savedSearchTools = map[string]bool{nameSaveSearch: true, nameListSavedSearches: true}

func TestDescriptionsOfferOnlySearchTheToolTakes(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		for label, offered := range map[string]Tool{"MCP": tool, "agent": tool.ForAgent()} {
			if !strings.Contains(offered.Description, "search") || savedSearchTools[tool.Name] {
				continue
			}
			if _, ok := schemaProperties(t, offered.InputSchema)["search"]; !ok {
				t.Errorf("%s %s description offers search, but the schema has no search property", tool.Name, label)
			}
		}
	}
}

const agentToolTextCap = 2000

var agentToolTextAllowances = map[string]int{
	nameUpdatePanels: 3700,
}

func TestEachAgentToolTextStaysWithinItsCap(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		offered := tool.ForAgent()
		size := len(offered.Description) + len(offered.InputSchema)
		limit := cmp.Or(agentToolTextAllowances[tool.Name], agentToolTextCap)
		if size > limit {
			t.Errorf("%s: agent tool text is %d bytes, over its %d byte cap; shorten it, or add an allowance deliberately and say why in the commit body", tool.Name, size, limit)
		}
	}
}

var toolLikeName = regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`)

func TestDiscoveryTextNamesEveryDatasourceType(t *testing.T) {
	mentions := map[string][]string{
		nameListLabels:      {"Loki", "Tempo"},
		nameListLabelValues: {"Loki"},
		nameSampleRows:      {"Loki"},
		nameGetSchema:       {"Tempo"},
		nameListDatasources: {"prometheus", "clickhouse", "loki", "tempo"},
	}
	byName := toolsByName(t)
	for name, words := range mentions {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		for surface, offered := range map[string]Tool{"mcp": tool, "agent": tool.ForAgent()} {
			text := offered.Description + string(offered.InputSchema)
			for _, word := range words {
				if !strings.Contains(text, word) {
					t.Errorf("%s %s text does not mention %s", surface, name, word)
				}
			}
		}
	}
}

func TestServerInstructionsNameOnlyRealToolsAndFields(t *testing.T) {
	byName := toolsByName(t)
	fields := []string{"folder_id", "parent_version", "version_conflict", "next_offset", "next_cursor"}
	for _, name := range toolLikeName.FindAllString(ServerInstructions, -1) {
		if _, ok := byName[name]; !ok && !slices.Contains(fields, name) {
			t.Errorf("the server instructions mention %s, which is neither a tool nor a known field", name)
		}
	}
	if guide, create := strings.Index(ServerInstructions, nameGetSpecGuide), strings.Index(ServerInstructions, nameCreateDashboard); guide < 0 || guide > create {
		t.Errorf("get_spec_guide is at %d and create_dashboard at %d; the guide must come first", guide, create)
	}
}
