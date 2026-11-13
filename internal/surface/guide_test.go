package surface

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts/skill"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/cli"
)

const guideFile = "internal/agent/prompts/skill/observe/SKILL.md"

var guideFields = []string{"folder_id", "parent_version", "version_conflict", "failing_panels", "next_cursor", "next_offset", "not_found", "start_runs", "run_refusal", "thread_context"}

var (
	toolLikeName = regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`)
	observeSpan  = regexp.MustCompile("`(observe [^`]*)`")
)

func mcpToolNames() map[string]bool {
	names := map[string]bool{}
	for _, tool := range tools.New(tools.Deps{}).List() {
		if !tool.AgentOnly {
			names[tool.Name] = true
		}
	}
	return names
}

func TestObserveGuideNamesOnlyMCPTools(t *testing.T) {
	offered := mcpToolNames()
	for _, name := range toolLikeName.FindAllString(skill.Body(), -1) {
		if !offered[name] && !slices.Contains(guideFields, name) {
			t.Errorf("the guide names %s, which MCP does not offer and which is not a known field; fix %s", name, guideFile)
		}
	}
}

func TestObserveGuideNamesOnlyCLICommands(t *testing.T) {
	root := cli.Commands()
	spans := observeSpan.FindAllStringSubmatch(skill.Body(), -1)
	if len(spans) == 0 {
		t.Fatal("the guide names no observe command")
	}
	for _, span := range spans {
		words := strings.Fields(span[1])[1:]
		if strings.HasPrefix(words[0], "<") {
			continue
		}
		if cmd, _, err := root.Find(words); err != nil || cmd == root {
			t.Errorf("the guide names `%s`, which the CLI does not have; fix %s", span[1], guideFile)
		}
	}
}

func TestObserveGuideReadsTheSpecGuideBeforeTheFirstSave(t *testing.T) {
	body := skill.Body()
	guide, create := strings.Index(body, "get_spec_guide"), strings.Index(body, "create_dashboard")
	if guide < 0 || create < 0 || guide > create {
		t.Errorf("get_spec_guide is at %d and create_dashboard at %d; the guide must come first in %s", guide, create, guideFile)
	}
}
