package tools

import (
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const mcpDocsPath = "../../../docs/use/mcp.md"

var mcpDocsToolRow = regexp.MustCompile("^\\| `([a-z_]+)` \\| (Viewer|Commenter|Editor) \\|$")

func TestMCPDocsListEveryToolWithItsRole(t *testing.T) {
	page, err := os.ReadFile(mcpDocsPath)
	if err != nil {
		t.Fatal(err)
	}
	var rowNames []string
	rowRoles := map[string]string{}
	for _, line := range strings.Split(string(page), "\n") {
		m := mcpDocsToolRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rowNames = append(rowNames, m[1])
		rowRoles[m[1]] = strings.ToLower(m[2])
	}
	if !slices.IsSorted(rowNames) {
		t.Errorf("the tool rows in %s are not sorted by name: %v", mcpDocsPath, rowNames)
	}

	offered := map[string]bool{}
	for _, tool := range New(Deps{}).List() {
		if tool.AgentOnly {
			continue
		}
		offered[tool.Name] = true
		role, ok := rowRoles[tool.Name]
		if !ok {
			t.Errorf("%s has no row in %s; add `| `%s` | %s |`", tool.Name, mcpDocsPath, tool.Name, capitalised(string(tool.MinRole)))
			continue
		}
		if role != string(tool.MinRole) {
			t.Errorf("%s row in %s says %s, the tool's lowest role is %s; fix its row", tool.Name, mcpDocsPath, role, tool.MinRole)
		}
	}
	for _, name := range rowNames {
		if !offered[name] {
			t.Errorf("%s has a row in %s but is not an MCP tool; remove its row", name, mcpDocsPath)
		}
	}
}

func capitalised(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

const agentDocsPath = "../../../docs/concepts/agent.md"

var (
	agentDocsServerTools = regexp.MustCompile(`The server has (\d+) tools: MCP clients get (\d+),`)
	agentDocsAgentTools  = regexp.MustCompile(`The built-in agent gets (\d+) in a new-dashboard or revision request and (\d+) in a suggestion`)
	agentDocsMCPOnly     = regexp.MustCompile(`The other (\d+) tools are for MCP clients only`)
)

func TestAgentDocsStateTheToolCountsOfTheRegistry(t *testing.T) {
	page, err := os.ReadFile(agentDocsPath)
	if err != nil {
		t.Fatal(err)
	}
	var total, mcp, create, suggest, mcpOnly int
	for _, tool := range New(Deps{}).List() {
		total++
		if !tool.AgentOnly {
			mcp++
		}
		if tool.OfferedIn(ModeCreate) {
			create++
		}
		if tool.OfferedIn(ModeSuggest) {
			suggest++
		}
		if tool.MCPOnly {
			mcpOnly++
		}
	}
	cases := []struct {
		name    string
		pattern *regexp.Regexp
		want    []string
	}{
		{"the server and MCP counts", agentDocsServerTools, []string{strconv.Itoa(total), strconv.Itoa(mcp)}},
		{"the built-in agent counts", agentDocsAgentTools, []string{strconv.Itoa(create), strconv.Itoa(suggest)}},
		{"the MCP-only count", agentDocsMCPOnly, []string{strconv.Itoa(mcpOnly)}},
	}
	for _, c := range cases {
		m := c.pattern.FindStringSubmatch(string(page))
		if m == nil {
			t.Errorf("%s: %s has no sentence matching %s", c.name, agentDocsPath, c.pattern)
			continue
		}
		if !slices.Equal(m[1:], c.want) {
			t.Errorf("%s: %s says %v, the registry has %v", c.name, agentDocsPath, m[1:], c.want)
		}
	}
}
