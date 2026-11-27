package tools

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/findings"
)

type schemaNode struct {
	Properties map[string]*schemaNode `json:"properties"`
	Items      *schemaNode            `json:"items"`
	Enum       []string               `json:"enum"`
	MaxItems   *int                   `json:"maxItems"`
	MaxLength  *int                   `json:"maxLength"`
	Maximum    *int                   `json:"maximum"`
}

func (n *schemaNode) at(t *testing.T, path string) *schemaNode {
	t.Helper()
	at := n
	for part := range strings.SplitSeq(path, ".") {
		switch {
		case at == nil:
		case part == "[]":
			at = at.Items
		default:
			at = at.Properties[part]
		}
	}
	if at == nil {
		t.Fatalf("the save_finding schema has no %s", path)
	}
	return at
}

func number(p *int) string {
	if p == nil {
		return "none"
	}
	return fmt.Sprint(*p)
}

func TestTheSaveFindingSchemaMatchesTheFindingsRules(t *testing.T) {
	var schema schemaNode
	if err := json.Unmarshal([]byte(schemaSaveFinding), &schema); err != nil {
		t.Fatal(err)
	}
	bands := []string{findings.BandHigh, findings.BandMedium, findings.BandLow}
	for path, want := range map[string][]string{
		"causes.[].component.kind":   findings.ComponentKinds,
		"causes.[].affected.[].kind": findings.ComponentKinds,
		"causes.[].kind":             findings.RootCauseKinds,
		"causes.[].trigger.kind":     findings.TriggerKinds,
		"causes.[].estimate":         bands,
		"confidence":                 findings.Confidences,
	} {
		if got := schema.at(t, path).Enum; !slices.Equal(got, want) {
			t.Errorf("%s: the schema's enum %q, the findings rule %q", path, got, want)
		}
	}
	for path, want := range map[string]int{
		"causes":                       findings.MaxCauses,
		"causes.[].affected":           findings.MaxAffected,
		"causes.[].evidence.facts":     findings.MaxCauseFacts,
		"causes.[].evidence.trace_ids": findings.MaxCauseTraceIDs,
	} {
		if got := schema.at(t, path).MaxItems; got == nil || *got != want {
			t.Errorf("%s: the schema's maxItems %s, the findings rule %d", path, number(got), want)
		}
	}
	for path, want := range map[string]int{
		"causes.[].component.name":      findings.MaxComponentRunes,
		"causes.[].component.namespace": findings.MaxComponentRunes,
		"causes.[].mechanism":           findings.MaxSentenceRunes,
		"causes.[].trigger.key":         findings.MaxKeyRunes,
		"causes.[].trigger.value":       findings.MaxKeyRunes,
	} {
		if got := schema.at(t, path).MaxLength; got == nil || *got != want {
			t.Errorf("%s: the schema's maxLength %s, the findings rule %d", path, number(got), want)
		}
	}
	if got := schema.at(t, "causes.[].knock_on_of").Maximum; got == nil || *got != findings.MaxCauses-1 {
		t.Errorf("knock_on_of: the schema's maximum %s, want the last cause index %d", number(got), findings.MaxCauses-1)
	}
}
