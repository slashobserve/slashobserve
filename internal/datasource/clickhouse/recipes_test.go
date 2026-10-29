package clickhouse

import (
	"regexp"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

var inlineCode = regexp.MustCompile("`([^`\n]+)`")

func clickHouseGuideRecipes(t *testing.T) []string {
	t.Helper()
	guide := prompts.SpecGuide()
	_, section, found := strings.Cut(guide, "## ClickHouse")
	if !found {
		t.Fatal("spec guide has no ClickHouse section")
	}
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	var recipes []string
	for _, m := range inlineCode.FindAllStringSubmatch(section, -1) {
		if strings.HasPrefix(m[1], "SELECT ") {
			recipes = append(recipes, m[1])
		}
	}
	return recipes
}

func TestGuideRecipes(t *testing.T) {
	recipes := clickHouseGuideRecipes(t)
	if len(recipes) < 3 {
		t.Fatalf("found %d SELECT recipes in the ClickHouse guide section, want at least 3", len(recipes))
	}
	for _, recipe := range recipes {
		t.Run(recipe[:min(40, len(recipe))], func(t *testing.T) {
			expansion, err := macros.ExpandForLint(macros.ClickHouse, recipe)
			if err != nil {
				t.Fatalf("ExpandForLint: %v", err)
			}
			if _, err := guardWith(expansion.Text, loadedTableFunctions()); err != nil {
				t.Errorf("guard rejects the expanded recipe: %v", err)
			}
			if !strings.Contains(recipe, "$__timeFilter(") {
				t.Error("recipe has no $__timeFilter( call, which the guide's own rule requires")
			}
			t.Log(expansion.Text)
		})
	}
}
