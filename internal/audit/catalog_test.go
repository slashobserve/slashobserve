package audit

import (
	"regexp"
	"slices"
	"testing"
)

var actionName = regexp.MustCompile(`^[a-z_]+\.[a-z_]+$`)

var categories = []Category{CategoryAuth, CategoryAccess, CategoryContent, CategoryConversation, CategoryAgent, CategoryQuery, CategoryAdmin, CategorySystem}

var keysThatHoldContent = []string{"body", "text", "message", "comment", "prompt", "answer", "secret", "token", "password", "rows", "series"}

func TestEveryCatalogActionHasANameACategoryAndAClass(t *testing.T) {
	for _, action := range Actions() {
		spec, _ := Lookup(action)
		if !actionName.MatchString(string(action)) {
			t.Errorf("%q is not <object>.<verb> in lower snake case", action)
		}
		if !slices.Contains(categories, spec.Category) {
			t.Errorf("%s has category %q", action, spec.Category)
		}
		if spec.Class != ClassChange && spec.Class != ClassAccess {
			t.Errorf("%s has class %q", action, spec.Class)
		}
		for _, key := range spec.Data {
			if slices.Contains(keysThatHoldContent, key) {
				t.Errorf("%s declares data key %q, which would hold content or a secret", action, key)
			}
		}
	}
}

func TestReadsAndQueriesAreAccessEvents(t *testing.T) {
	for _, action := range []Action{"dashboard.view", "observation.view", "public_link.view", "dashboard.export", "logs.export", "audit.search", "audit.export", "query.run", "agent.tool_call"} {
		spec, ok := Lookup(action)
		if !ok || spec.Class != ClassAccess {
			t.Errorf("%s: class %q (in catalog %v), want access", action, spec.Class, ok)
		}
	}
	for _, action := range []Action{"dashboard.update", "user.role_change", ActionPrune} {
		if spec, _ := Lookup(action); spec.Class != ClassChange {
			t.Errorf("%s: class %q, want change", action, spec.Class)
		}
	}
}

func TestLookupRefusesAnUnknownAction(t *testing.T) {
	if _, ok := Lookup("dashboard.explode"); ok {
		t.Fatal("an action outside the catalog was found")
	}
}
