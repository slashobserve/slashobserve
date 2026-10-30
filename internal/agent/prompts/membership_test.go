package prompts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const undeclaredListPath = "testdata/undeclared-prompt-keys.txt"

var membershipValueKeywords = map[string]bool{
	"items":                 true,
	"additionalProperties":  true,
	"unevaluatedProperties": true,
	"then":                  true,
	"else":                  true,
}

type membershipCheck struct {
	undeclared map[string]bool
	seen       map[string]bool
	issues     []string
}

func membershipIssues(raw json.RawMessage, undeclared map[string]bool) ([]string, error) {
	c := &membershipCheck{undeclared: undeclared, seen: map[string]bool{}}
	if err := c.walk(raw, "", false); err != nil {
		return nil, err
	}
	for p := range undeclared {
		if !c.seen[p] {
			c.issues = append(c.issues, fmt.Sprintf("stale %s: no such undeclared property; delete it from testdata/undeclared-prompt-keys.txt", p))
		}
	}
	sort.Strings(c.issues)
	return c.issues, nil
}

func (c *membershipCheck) walk(raw json.RawMessage, ptr string, declared bool) error {
	members, isObject, err := objectMembers(raw)
	if err != nil || !isObject {
		return err
	}
	declared = declared || hasMembershipMarker(members)
	var required []string
	for _, m := range members {
		if m.key == "required" {
			if err := json.Unmarshal(m.value, &required); err != nil {
				return err
			}
		}
	}
	for _, m := range members {
		switch {
		case m.key == "properties":
			err = c.walkProperties(m.value, ptr+"/properties", declared, required)
		case m.key == "$defs":
			err = c.walkDefs(m.value, ptr+"/$defs", declared)
		case schemaArrayKeywords[m.key]:
			err = c.walkItems(m.value, ptr+"/"+m.key, declared)
		case membershipValueKeywords[m.key]:
			err = c.walk(m.value, ptr+"/"+m.key, declared)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *membershipCheck) walkProperties(raw json.RawMessage, base string, declared bool, required []string) error {
	members, isObject, err := objectMembers(raw)
	if err != nil || !isObject {
		return err
	}
	for _, m := range members {
		p := base + "/" + escapePointer(m.key)
		c.seen[p] = true
		own := declaresMembership(m.value)
		switch {
		case !declared && !own && !c.undeclared[p]:
			stripped, err := stripSchema(m.value)
			if err != nil {
				return err
			}
			c.issues = append(c.issues, fmt.Sprintf("undeclared %s: declare prompt membership with \"x-omitFromPrompt\": true (rare, import-only or purely presentational, such as axisGrid or stat justify) or false (intent the agent writes from a request, such as unit, thresholds, legend, axis label, log base, reduce, stacking or tooltip mode); keeping it costs %d bytes of the agent prompt", p, len(stripped)+len(m.key)+4))
		case (declared || own) && c.undeclared[p]:
			c.issues = append(c.issues, fmt.Sprintf("declared %s: delete it from testdata/undeclared-prompt-keys.txt", p))
		}
		if omittedFromPrompt(m.value) && slices.Contains(required, m.key) {
			c.issues = append(c.issues, fmt.Sprintf("required %s: an omitted property cannot be required", p))
		}
		if err := c.walk(m.value, p, declared); err != nil {
			return err
		}
	}
	return nil
}

func (c *membershipCheck) walkDefs(raw json.RawMessage, base string, declared bool) error {
	members, isObject, err := objectMembers(raw)
	if err != nil || !isObject {
		return err
	}
	for _, m := range members {
		if err := c.walk(m.value, base+"/"+escapePointer(m.key), declared); err != nil {
			return err
		}
	}
	return nil
}

func (c *membershipCheck) walkItems(raw json.RawMessage, base string, declared bool) error {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}
	for i, item := range items {
		if err := c.walk(item, fmt.Sprintf("%s/%d", base, i), declared); err != nil {
			return err
		}
	}
	return nil
}

func hasMembershipMarker(members []member) bool {
	return slices.ContainsFunc(members, func(m member) bool { return m.key == omitFromPromptKeyword })
}

func declaresMembership(raw json.RawMessage) bool {
	members, isObject, err := objectMembers(raw)
	return err == nil && isObject && hasMembershipMarker(members)
}

func escapePointer(name string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(name)
}

func readUndeclaredList(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(undeclaredListPath)
	if err != nil {
		t.Fatal(err)
	}
	list := map[string]bool{}
	for line := range bytes.Lines(body) {
		if p := strings.TrimSpace(string(line)); p != "" {
			list[p] = true
		}
	}
	return list
}

func TestEverySchemaPropertyDeclaresPromptMembership(t *testing.T) {
	issues, err := membershipIssues(spec.Schema(), readUndeclaredList(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		t.Error(issue)
	}
}

func TestMembershipIssues(t *testing.T) {
	tests := []struct {
		name       string
		schema     string
		undeclared []string
		want       []string
	}{
		{
			name:   "an undeclared new property is reported with its byte cost",
			schema: `{"properties": {"a": {"type": "string"}}}`,
			want:   []string{"undeclared /properties/a: declare prompt membership"},
		},
		{
			name:   "false on a parent covers its children",
			schema: `{"properties": {"a": {"x-omitFromPrompt": false, "properties": {"b": {"type": "string"}}}}}`,
		},
		{
			name:   "true on a property is accepted",
			schema: `{"properties": {"a": {"x-omitFromPrompt": true, "type": "string"}}}`,
		},
		{
			name:   "false on a oneOf item covers that item's properties",
			schema: `{"oneOf": [{"x-omitFromPrompt": false, "properties": {"a": {"type": "string"}}}]}`,
		},
		{
			name:       "a listed undeclared property is accepted",
			schema:     `{"properties": {"a": {"type": "string"}}}`,
			undeclared: []string{"/properties/a"},
		},
		{
			name:       "a listed property that is now declared is reported",
			schema:     `{"properties": {"a": {"x-omitFromPrompt": false, "type": "string"}}}`,
			undeclared: []string{"/properties/a"},
			want:       []string{"declared /properties/a: delete it from testdata/undeclared-prompt-keys.txt"},
		},
		{
			name:       "a stale listed pointer is reported",
			schema:     `{"properties": {"a": {"x-omitFromPrompt": true}}}`,
			undeclared: []string{"/properties/gone"},
			want:       []string{"stale /properties/gone: no such undeclared property; delete it from testdata/undeclared-prompt-keys.txt"},
		},
		{
			name:   "an omitted required property is reported",
			schema: `{"required": ["a"], "properties": {"a": {"x-omitFromPrompt": true}}}`,
			want:   []string{"required /properties/a: an omitted property cannot be required"},
		},
		{
			name:   "a kept required property is accepted",
			schema: `{"required": ["a"], "properties": {"a": {"x-omitFromPrompt": false}}}`,
		},
		{
			name:   "a property inside if is ignored",
			schema: `{"if": {"properties": {"a": {"const": 1}}}, "then": {"x-omitFromPrompt": false}}`,
		},
		{
			name:   "a property inside then is checked",
			schema: `{"if": {"properties": {"a": {"const": 1}}}, "then": {"properties": {"b": {"type": "string"}}}}`,
			want:   []string{"undeclared /then/properties/b: declare prompt membership"},
		},
		{
			name:   "a property name containing a slash is escaped",
			schema: `{"properties": {"a/b": {"type": "string"}}}`,
			want:   []string{"undeclared /properties/a~1b: declare prompt membership"},
		},
		{
			name:   "a def marked omitted covers its properties",
			schema: `{"$defs": {"d": {"x-omitFromPrompt": true, "properties": {"a": {"type": "string"}}}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			undeclared := map[string]bool{}
			for _, p := range tt.undeclared {
				undeclared[p] = true
			}
			got, err := membershipIssues(json.RawMessage(tt.schema), undeclared)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d issues %q, want %d starting %q", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if !strings.HasPrefix(got[i], tt.want[i]) {
					t.Errorf("issue %d = %q, want prefix %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMembershipIssueNamesWhatKeepingTheKeyCosts(t *testing.T) {
	got, err := membershipIssues(json.RawMessage(`{"properties": {"abc": {"type": "string"}}}`), map[string]bool{})
	if err != nil || len(got) != 1 {
		t.Fatalf("issues = %q, err = %v", got, err)
	}
	if want := "keeping it costs 24 bytes of the agent prompt"; !strings.Contains(got[0], want) {
		t.Fatalf("issue %q lacks %q", got[0], want)
	}
}
