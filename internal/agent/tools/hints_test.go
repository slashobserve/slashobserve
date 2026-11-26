package tools

import (
	"slices"
	"strings"
	"testing"
)

var (
	readVerbs      = []string{"get", "list", "search", "aggregate", "query", "lint", "draft", "log", "logs", "compare", "sample", "trace", "recheck", "diff", "call", "find"}
	overwriteVerbs = []string{"update", "arrange", "restore", "edit", "rename", "discard", "merge", "clear", "set", "cancel"}
	additiveVerbs  = []string{"create", "add", "duplicate", "make", "keep", "move", "pin", "unpin", "publish", "reopen", "resolve", "reply", "report", "retry", "run", "save", "start", "claim", "finish", "import", "request", "archive", "share", "apply"}
)

func TestEveryToolHintFollowsWhatItDoes(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		verb, _, _ := strings.Cut(tool.Name, "_")
		switch {
		case slices.Contains(readVerbs, verb):
			if !tool.ReadOnly || tool.Destructive {
				t.Errorf("%s only reads, so it must be read only and not destructive (read only %v, destructive %v)", tool.Name, tool.ReadOnly, tool.Destructive)
			}
		case slices.Contains(overwriteVerbs, verb):
			if tool.ReadOnly || !tool.Destructive {
				t.Errorf("%s overwrites or deletes, so it must be destructive and not read only (read only %v, destructive %v)", tool.Name, tool.ReadOnly, tool.Destructive)
			}
		case slices.Contains(additiveVerbs, verb):
			if tool.ReadOnly || tool.Destructive {
				t.Errorf("%s only adds, so it must be neither read only nor destructive (read only %v, destructive %v)", tool.Name, tool.ReadOnly, tool.Destructive)
			}
		default:
			t.Errorf("%s starts with %q, which no hint rule covers; add the verb to one of the lists in this test", tool.Name, verb)
		}
	}
}

func TestDestructiveTableNamesRegisteredTools(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range New(Deps{}).List() {
		names[tool.Name] = true
	}
	for name := range destructiveTools {
		if !names[name] {
			t.Errorf("destructiveTools names %q, which is not a registered tool", name)
		}
	}
}
