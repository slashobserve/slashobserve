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

var mcpDocsToolRow = regexp.MustCompile("^\\| `([a-z_]+)` \\| (Viewer|Commenter|Editor|Admin) \\|$")

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
	agentDocsRunTools    = regexp.MustCompile(`A run offers (\d+) of them in a new-dashboard or revision request, (\d+) in an investigation and (\d+) in a suggestion`)
	agentDocsLeftOut     = regexp.MustCompile(`The other (\d+) tools are left out of every run`)
)

func TestAgentDocsStateTheToolCountsOfTheRegistry(t *testing.T) {
	page, err := os.ReadFile(agentDocsPath)
	if err != nil {
		t.Fatal(err)
	}
	var total, mcp, create, investigate, suggest, leftOut int
	for _, tool := range New(Deps{}).List() {
		total++
		if !tool.AgentOnly {
			mcp++
		}
		if tool.OfferedIn(ModeCreate) {
			create++
		}
		if tool.OfferedIn(ModeInvestigate) {
			investigate++
		}
		if tool.OfferedIn(ModeSuggest) {
			suggest++
		}
		if tool.MCPOnly {
			leftOut++
		}
	}
	cases := []struct {
		name    string
		pattern *regexp.Regexp
		want    []string
	}{
		{"the server and MCP counts", agentDocsServerTools, []string{strconv.Itoa(total), strconv.Itoa(mcp)}},
		{"the run counts", agentDocsRunTools, []string{strconv.Itoa(create), strconv.Itoa(investigate), strconv.Itoa(suggest)}},
		{"the left-out count", agentDocsLeftOut, []string{strconv.Itoa(leftOut)}},
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
