package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/auth"
)

func contentFields(t *testing.T, res Result) map[string]json.RawMessage {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %#v", res.Content)
	}
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func TestGetSpecGuideInARunReturnsOnlyTheSchema(t *testing.T) {
	r := New(Deps{})
	ctx := WithRun(auth.WithPrincipal(context.Background(), auth.Principal{Role: auth.RoleViewer}), Run{Mode: ModeCreate})
	res, err := r.Call(ctx, nameGetSpecGuide, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	fields := contentFields(t, res)
	if len(fields) != 1 || fields["schema"] == nil {
		t.Fatalf("fields = %v, want only schema", fields)
	}
	var got, want any
	if err := json.Unmarshal(fields["schema"], &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(prompts.Schema(), &want); err != nil {
		t.Fatal(err)
	}
	if string(mustMarshal(t, got)) != string(mustMarshal(t, want)) {
		t.Fatal("the schema in the result is not the prompt schema")
	}
	if res.Step != "Read the Spec schema" {
		t.Fatalf("step = %q", res.Step)
	}
}

func TestGetSpecGuideOverMCPReturnsTheGuideTheSchemaAndTheExample(t *testing.T) {
	r := New(Deps{})
	res, err := call(t, r, nameGetSpecGuide, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	fields := contentFields(t, res)
	for _, key := range []string{"guide", "schema", "example"} {
		if len(fields[key]) == 0 {
			t.Errorf("result lacks %s", key)
		}
	}
	if len(fields) != 3 {
		t.Errorf("fields = %v", fields)
	}
	if res.Step != "Read the Spec guide" {
		t.Fatalf("step = %q", res.Step)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
