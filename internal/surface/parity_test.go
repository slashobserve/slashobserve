package surface

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/cli"
)

const surfacesExtension = "x-surfaces"

var surfaceNames = []string{"mcp", "cli"}

func (e exception) reason(surface string) string {
	if surface == "mcp" {
		return e.MCP
	}
	return e.CLI
}

func surfaceLabel(surface string) string {
	if surface == "mcp" {
		return "MCP tool"
	}
	return "CLI command"
}

func specFile(item *openapi3.PathItem) string {
	if file, _, ok := strings.Cut(strings.TrimPrefix(item.Ref, "./"), "#"); ok && strings.HasPrefix(file, "paths/") {
		return "api/" + file
	}
	return "api/paths/<file>.yaml"
}

func readSurfaces(op *openapi3.Operation) (map[string][]string, error) {
	raw, ok := op.Extensions[surfacesExtension]
	if !ok {
		return nil, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var surfaces map[string][]string
	if err := json.Unmarshal(encoded, &surfaces); err != nil {
		return nil, fmt.Errorf("it must map mcp and cli to lists of strings: %w", err)
	}
	return surfaces, nil
}

type tally struct {
	operations, named, excepted int
}

func check(doc *openapi3.T, mcpTools, cliCommands map[string]bool, exceptions map[string]exception) []string {
	problems, _ := checkCounting(doc, mcpTools, cliCommands, exceptions)
	return problems
}

func checkCounting(doc *openapi3.T, mcpTools, cliCommands map[string]bool, exceptions map[string]exception) ([]string, tally) {
	var problems []string
	var counts tally
	known := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			counts.operations++
			where := fmt.Sprintf("%s %s", strings.ToLower(method), path)
			file := specFile(item)
			id := op.OperationID
			if id == "" {
				problems = append(problems, fmt.Sprintf("%s has no operationId: add one in %s", where, file))
				continue
			}
			known[id] = true
			label := fmt.Sprintf("%s (%s)", id, where)

			surfaces, err := readSurfaces(op)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s has a malformed %s: %v; fix %s", label, surfacesExtension, err, file))
			}
			for _, key := range slices.Sorted(maps.Keys(surfaces)) {
				if !slices.Contains(surfaceNames, key) {
					problems = append(problems, fmt.Sprintf("%s has %s.%s, but the only keys are mcp and cli; fix %s", label, surfacesExtension, key, file))
				} else if len(surfaces[key]) == 0 {
					problems = append(problems, fmt.Sprintf("%s has an empty %s.%s list; name an entry or remove the key in %s", label, surfacesExtension, key, file))
				}
			}
			for _, tool := range surfaces["mcp"] {
				if !mcpTools[tool] {
					problems = append(problems, fmt.Sprintf("%s names the MCP tool %q, which MCP does not offer; fix %s", label, tool, file))
				}
			}
			for _, command := range surfaces["cli"] {
				if !cliCommands[command] {
					problems = append(problems, fmt.Sprintf("%s names the CLI command %q, which is not a runnable command of the CLI; fix %s", label, command, file))
				}
			}

			named, excepted := 0, 0
			for _, surface := range surfaceNames {
				listed := len(surfaces[surface]) > 0
				reason := exceptions[id].reason(surface)
				switch {
				case listed && reason != "":
					problems = append(problems, fmt.Sprintf("%s names a %s and also has a %s exception; remove one", label, surfaceLabel(surface), surfaceLabel(surface)))
				case !listed && reason == "":
					problems = append(problems, fmt.Sprintf("%s has no %s: add %s: {%s: [...]} to %s or a reason to internal/surface/exceptions_test.go", label, surfaceLabel(surface), surfacesExtension, surface, file))
				}
				if listed {
					named++
				}
				if reason != "" {
					excepted++
				}
			}
			if named > 0 {
				counts.named++
			} else if excepted == len(surfaceNames) {
				counts.excepted++
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(exceptions)) {
		if !known[id] {
			problems = append(problems, fmt.Sprintf("the exception for %q names no operationId of the API; remove it from internal/surface/exceptions_test.go", id))
		} else if exceptions[id].MCP == "" && exceptions[id].CLI == "" {
			problems = append(problems, fmt.Sprintf("the exception for %q has no reason; fill a field or remove it from internal/surface/exceptions_test.go", id))
		}
	}
	slices.Sort(problems)
	return problems, counts
}

func runnableCommands(root *cobra.Command) map[string]bool {
	paths := map[string]bool{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd != root && (cmd.Name() == "help" || cmd.Name() == "completion") {
			return
		}
		if cmd.Runnable() {
			paths[cmd.CommandPath()] = true
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	return paths
}

func TestEveryOperationReachesMCPAndTheCLI(t *testing.T) {
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	problems, counts := checkCounting(doc, mcpToolNames(), runnableCommands(cli.Commands()), exceptions)
	for _, problem := range problems {
		t.Error(problem)
	}
	var noTool, noCommand int
	for _, e := range exceptions {
		if e.MCP == notBuilt {
			noTool++
		}
		if e.CLI == notBuilt {
			noCommand++
		}
	}
	t.Logf("%d operations: %d name a surface, %d are excepted on both; %d have no MCP tool yet and %d no CLI command yet", counts.operations, counts.named, counts.excepted, noTool, noCommand)
}

func parityDoc(t *testing.T, operations string) *openapi3.T {
	t.Helper()
	spec := "openapi: 3.0.3\ninfo: {title: t, version: '1'}\npaths:\n" + operations
	doc, err := openapi3.NewLoader().LoadFromData([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

const namedBoth = `  /things:
    get:
      operationId: listThings
      x-surfaces:
        mcp: [list_things]
        cli: [observe things]
      responses: {"200": {description: ok}}
`

var (
	fakeTools    = map[string]bool{"list_things": true}
	fakeCommands = map[string]bool{"observe things": true}
)

func requireProblems(t *testing.T, got []string, wants ...string) {
	t.Helper()
	if len(got) != len(wants) {
		t.Fatalf("problems = %q, want %d", got, len(wants))
	}
	for i, want := range wants {
		if !strings.Contains(got[i], want) {
			t.Errorf("problem %d = %q, want it to contain %q", i, got[i], want)
		}
	}
}

func TestCheckAcceptsNamedSurfacesAndExceptions(t *testing.T) {
	doc := parityDoc(t, namedBoth+`    delete:
      operationId: deleteThings
      responses: {"204": {description: ok}}
`)
	got, counts := checkCounting(doc, fakeTools, fakeCommands, map[string]exception{"deleteThings": {MCP: "a", CLI: "b"}})
	requireProblems(t, got)
	if counts != (tally{operations: 2, named: 1, excepted: 1}) {
		t.Errorf("counts = %+v, want 2 operations, 1 named and 1 excepted", counts)
	}
}

func TestCheckReportsAMissingSurface(t *testing.T) {
	doc := parityDoc(t, `  /things:
    get:
      operationId: listThings
      x-surfaces:
        mcp: [list_things]
      responses: {"200": {description: ok}}
`)
	requireProblems(t, check(doc, fakeTools, fakeCommands, nil), "listThings (get /things) has no CLI command: add x-surfaces: {cli: [...]} to api/paths/<file>.yaml or a reason to internal/surface/exceptions_test.go")
}

func TestCheckReportsASurfaceThatIsAlsoExcepted(t *testing.T) {
	doc := parityDoc(t, namedBoth)
	got := check(doc, fakeTools, fakeCommands, map[string]exception{"listThings": {MCP: "because"}})
	requireProblems(t, got, "listThings (get /things) names a MCP tool and also has a MCP tool exception; remove one")
}

func TestCheckReportsAnUnknownTool(t *testing.T) {
	doc := parityDoc(t, `  /things:
    get:
      operationId: listThings
      x-surfaces:
        mcp: [list_other]
        cli: [observe things]
      responses: {"200": {description: ok}}
`)
	requireProblems(t, check(doc, fakeTools, fakeCommands, nil), `names the MCP tool "list_other", which MCP does not offer`)
}

func TestCheckReportsAnUnknownCommand(t *testing.T) {
	doc := parityDoc(t, `  /things:
    get:
      operationId: listThings
      x-surfaces:
        mcp: [list_things]
        cli: [observe other]
      responses: {"200": {description: ok}}
`)
	requireProblems(t, check(doc, fakeTools, fakeCommands, nil), `names the CLI command "observe other"`)
}

func TestCheckReportsAnUnknownKey(t *testing.T) {
	doc := parityDoc(t, `  /things:
    get:
      operationId: listThings
      x-surfaces:
        mcp: [list_things]
        cli: [observe things]
        http: [GET]
      responses: {"200": {description: ok}}
`)
	requireProblems(t, check(doc, fakeTools, fakeCommands, nil), "has x-surfaces.http, but the only keys are mcp and cli")
}

func TestCheckReportsAnEmptyList(t *testing.T) {
	doc := parityDoc(t, `  /things:
    get:
      operationId: listThings
      x-surfaces:
        mcp: [list_things]
        cli: []
      responses: {"200": {description: ok}}
`)
	got := check(doc, fakeTools, fakeCommands, map[string]exception{"listThings": {CLI: "later"}})
	requireProblems(t, got, "has an empty x-surfaces.cli list")
}

func TestCheckReportsAMalformedExtension(t *testing.T) {
	doc := parityDoc(t, `  /things:
    get:
      operationId: listThings
      x-surfaces:
        mcp: list_things
        cli: [observe things]
      responses: {"200": {description: ok}}
`)
	requireProblems(t, check(doc, fakeTools, fakeCommands, nil), "has a malformed x-surfaces", "has no CLI command", "has no MCP tool")
}

func TestCheckReportsAStaleException(t *testing.T) {
	doc := parityDoc(t, namedBoth)
	got := check(doc, fakeTools, fakeCommands, map[string]exception{"gone": {MCP: "old"}})
	requireProblems(t, got, `the exception for "gone" names no operationId of the API`)
}

func TestCheckReportsAnEmptyException(t *testing.T) {
	doc := parityDoc(t, namedBoth)
	got := check(doc, fakeTools, fakeCommands, map[string]exception{"listThings": {}})
	requireProblems(t, got, `the exception for "listThings" has no reason`)
}

func TestCheckReportsAMissingOperationID(t *testing.T) {
	doc := parityDoc(t, `  /things:
    get:
      responses: {"200": {description: ok}}
`)
	requireProblems(t, check(doc, fakeTools, fakeCommands, nil), "get /things has no operationId")
}
