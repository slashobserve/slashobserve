package runsession

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	agentProtocolPath = "../../observe-agent/src/protocol.ts"
	runMetaDocPath    = "../../docs/use/mcp.md"
)

func goSettledKinds(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "state.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if !strings.HasPrefix(name.Name, "settled") || i >= len(spec.Values) {
				continue
			}
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			kinds = append(kinds, value)
		}
		return true
	})
	slices.Sort(kinds)
	return kinds
}

func agentSettledKinds(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(agentProtocolPath)
	if err != nil {
		t.Fatal(err)
	}
	list := regexp.MustCompile(`(?s)export const settledKinds = \[(.*?)\] as const`).FindSubmatch(source)
	if list == nil {
		t.Fatalf("%s has no settledKinds list", agentProtocolPath)
	}
	var kinds []string
	for _, quoted := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(list[1], -1) {
		kinds = append(kinds, string(quoted[1]))
	}
	slices.Sort(kinds)
	return kinds
}

func settledSentence(t *testing.T) string {
	t.Helper()
	doc, err := os.ReadFile(runMetaDocPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(doc), "\n") {
		if strings.HasPrefix(line, "`settled` is ") {
			return line
		}
	}
	t.Fatalf("%s has no line starting with the settled kinds", runMetaDocPath)
	return ""
}

func TestEverySettledKindIsKnownToTheAgentAndTheDocs(t *testing.T) {
	kinds := goSettledKinds(t)
	if len(kinds) == 0 {
		t.Fatal("state.go declares no settled kinds")
	}
	if agent := agentSettledKinds(t); !slices.Equal(kinds, agent) {
		t.Errorf("settled kinds differ: Go %v, observe-agent settledKinds %v", kinds, agent)
	}
	sentence := settledSentence(t)
	for _, kind := range kinds {
		if !strings.Contains(sentence, "`"+kind+"`") {
			t.Errorf("%s does not describe the settled kind %q", runMetaDocPath, kind)
		}
	}
}
