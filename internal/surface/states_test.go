package surface

import (
	"flag"
	"os"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var updateStates = flag.Bool("update", false, "rewrite docs/reference/states.md")

const statesDocPath = "../../docs/reference/states.md"

func statesReference() string {
	return lifecycle.Render(
		revisions.RunMachine.Doc("Run", "A run is one agent request, started by `POST /observe`, a comment, a prompt, an investigation or a suggestion."),
	)
}

func TestStatesReferenceIsCurrent(t *testing.T) {
	want := statesReference()
	if *updateStates {
		if err := os.WriteFile(statesDocPath, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(statesDocPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Error("docs/reference/states.md is stale; run make generate")
	}
}

func enumOf(t *testing.T, doc *openapi3.T, schema, property string) []string {
	t.Helper()
	ref, ok := doc.Components.Schemas[schema]
	if !ok {
		t.Fatalf("schema %s is not in the document", schema)
	}
	prop, ok := ref.Value.Properties[property]
	if !ok {
		t.Fatalf("%s has no property %s", schema, property)
	}
	values := prop.Value.Enum
	if prop.Value.Items != nil {
		values = prop.Value.Items.Value.Enum
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i], _ = v.(string)
	}
	return out
}

func TestRunStateEnumsMatchTheMachine(t *testing.T) {
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var states, kinds, offered []string
	for _, s := range revisions.RunMachine.States {
		states = append(states, string(s))
	}
	for _, k := range revisions.ResultKinds {
		kinds = append(kinds, string(k))
	}
	for _, a := range revisions.RunMachine.Offered() {
		offered = append(offered, string(a))
	}
	for _, schema := range []string{"AgentRequest", "RequestSummary", "Revision"} {
		if got := enumOf(t, doc, schema, "state"); !slices.Equal(got, states) {
			t.Errorf("%s.state enum = %v, want %v", schema, got, states)
		}
		if got := enumOf(t, doc, schema, "result_kind"); !slices.Equal(got, kinds) {
			t.Errorf("%s.result_kind enum = %v, want %v", schema, got, kinds)
		}
	}
	for _, schema := range []string{"AgentRequest", "RequestSummary"} {
		if got := enumOf(t, doc, schema, "allowed_actions"); !slices.Equal(got, offered) {
			t.Errorf("%s.allowed_actions items enum = %v, want %v", schema, got, offered)
		}
	}
	if got := enumOf(t, doc, "InvalidTransition", "code"); !slices.Equal(got, []string{"invalid_transition"}) {
		t.Errorf("InvalidTransition.code enum = %v", got)
	}
}
