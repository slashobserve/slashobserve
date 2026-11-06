package tools

import (
	"os"
	"regexp"
	"slices"
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
