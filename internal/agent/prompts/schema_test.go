package prompts_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/examples"
)

func compile(t *testing.T, raw []byte) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource("urn:test:schema", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("urn:test:schema")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func accepts(t *testing.T, root *os.Root, s *jsonschema.Schema, file string) bool {
	t.Helper()
	body, err := root.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return acceptsBytes(t, s, body)
}

func acceptsBytes(t *testing.T, s *jsonschema.Schema, body []byte) bool {
	t.Helper()
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return s.Validate(instance) == nil
}

func TestSchemaIsSmallerAndKeepsValidationKeywords(t *testing.T) {
	full := spec.Schema()
	trimmed := prompts.Schema()
	if len(trimmed) >= len(full)*9/10 {
		t.Fatalf("prompt schema is %d bytes, full schema %d bytes", len(trimmed), len(full))
	}
	var fullDoc, trimmedDoc map[string]any
	if err := json.Unmarshal(full, &fullDoc); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(trimmed, &trimmedDoc); err != nil {
		t.Fatalf("prompt schema is not JSON: %v", err)
	}
	for _, key := range []string{"$schema", "$id", "title", "description"} {
		if _, ok := trimmedDoc[key]; ok {
			t.Fatalf("prompt schema keeps root %q", key)
		}
	}
	for _, key := range []string{"required", "properties", "$defs", "additionalProperties"} {
		if _, ok := trimmedDoc[key]; !ok {
			t.Fatalf("prompt schema lost root %q", key)
		}
	}
	for _, sentence := range []string{"Stable across versions; comments anchor to it.", "At most 200 panels in total", "Position on the 24-column grid"} {
		if !strings.Contains(string(full), sentence) {
			t.Fatalf("full schema lacks %q, update this test", sentence)
		}
		if strings.Contains(string(trimmed), sentence) {
			t.Fatalf("prompt schema keeps the description %q", sentence)
		}
	}
}

func TestSchemaKeepsFieldsNamedLikeAnnotations(t *testing.T) {
	var doc struct {
		Defs map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(prompts.Schema(), &doc); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dataPanel", "textPanel", "link"} {
		if _, ok := doc.Defs[name].Properties["title"]; !ok {
			t.Fatalf("%s lost its title property", name)
		}
	}
	if _, ok := doc.Defs["rowPanel"].Properties["title"]; !ok {
		t.Fatal("rowPanel lost its title property")
	}
}

func TestSchemaKeepsPropertyOrder(t *testing.T) {
	trimmed := string(prompts.Schema())
	if !strings.HasPrefix(trimmed, `{"type":"object"`) {
		t.Fatalf("prompt schema starts %q", trimmed[:40])
	}
	if strings.Index(trimmed, `"specVersion"`) > strings.Index(trimmed, `"panels"`) {
		t.Fatal("property order changed")
	}
}

func openExamples(t *testing.T) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(filepath.Join("..", "..", "..", "spec", "examples"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func globExamples(t *testing.T, root *os.Root, pattern string) []string {
	t.Helper()
	files, err := fs.Glob(root.FS(), pattern)
	if err != nil || len(files) == 0 {
		t.Fatalf("%s: %v, %v", pattern, files, err)
	}
	return files
}

func TestAnnotationStripJudgesFilesLikeTheFullSchema(t *testing.T) {
	stripped, err := prompts.StripAnnotations(spec.Schema())
	if err != nil {
		t.Fatal(err)
	}
	full := compile(t, spec.Schema())
	trimmed := compile(t, stripped)
	root := openExamples(t)
	for _, file := range globExamples(t, root, "*.json") {
		if !accepts(t, root, trimmed, file) {
			t.Errorf("annotation-stripped schema rejects valid %s", file)
		}
	}
	for _, file := range globExamples(t, root, "invalid/schema/*.json") {
		if accepts(t, root, full, file) {
			t.Fatalf("full schema accepts %s, so it is not a schema fixture", file)
		}
		if accepts(t, root, trimmed, file) {
			t.Errorf("annotation-stripped schema accepts invalid %s", file)
		}
	}
}

func TestSchemaAcceptsTheGuideExampleAndRejectsInvalidFixtures(t *testing.T) {
	trimmed := compile(t, prompts.Schema())
	if !acceptsBytes(t, trimmed, examples.CheckoutServiceHealth) {
		t.Error("prompt schema rejects the guide example")
	}
	root := openExamples(t)
	for _, file := range globExamples(t, root, "invalid/schema/*.json") {
		if accepts(t, root, trimmed, file) {
			t.Errorf("prompt schema accepts invalid %s", file)
		}
	}
}

func TestSchemaLeavesOutValuesOnlyImportsNeed(t *testing.T) {
	schema := string(prompts.Schema())
	if strings.Contains(schema, "x-omitFromPrompt") {
		t.Fatal("prompt schema keeps the x-omitFromPrompt marker")
	}
	for _, kept := range []string{`"bytes"`, `suffix|prefix`} {
		if !strings.Contains(schema, kept) {
			t.Errorf("prompt schema lost %s", kept)
		}
	}
	for _, left := range []string{`"radbq"`, `currency):`} {
		if strings.Contains(schema, left) {
			t.Errorf("prompt schema keeps %s", left)
		}
	}
}

func TestStripSchemaDropsMarkedMembers(t *testing.T) {
	input := []byte(`{
		"type": "object",
		"properties": {
			"kept": {"type": "string"},
			"rare": {"x-omitFromPrompt": true, "type": "string"}
		},
		"$defs": {
			"unit": {"anyOf": [
				{"enum": ["a"]},
				{"x-omitFromPrompt": true, "enum": ["b"]},
				{"x-omitFromPrompt": false, "enum": ["c"]}
			]},
			"rareDef": {"x-omitFromPrompt": true, "type": "string"}
		}
	}`)
	got, err := prompts.StripSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"object","properties":{"kept":{"type":"string"}},"$defs":{"unit":{"anyOf":[{"enum":["a"]},{"enum":["c"]}]}}}`
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	kept, err := prompts.StripAnnotations(input)
	if err != nil {
		t.Fatal(err)
	}
	wantKept := `{"type":"object","properties":{"kept":{"type":"string"},"rare":{"type":"string"}},"$defs":{"unit":{"anyOf":[{"enum":["a"]},{"enum":["b"]},{"enum":["c"]}]},"rareDef":{"type":"string"}}}`
	if string(kept) != wantKept {
		t.Fatalf("got  %s\nwant %s", kept, wantKept)
	}
}

func TestStripSchemaRejectsMisplacedMarkers(t *testing.T) {
	for name, schema := range map[string]string{
		"allOf":       `{"allOf": [{"x-omitFromPrompt": true, "type": "string"}]}`,
		"prefixItems": `{"prefixItems": [{"x-omitFromPrompt": true, "type": "string"}]}`,
		"emptied":     `{"anyOf": [{"x-omitFromPrompt": true, "enum": ["a"]}]}`,
		"emptied one": `{"oneOf": [{"x-omitFromPrompt": true, "enum": ["a"]}]}`,
	} {
		if _, err := prompts.StripSchema([]byte(schema)); err == nil {
			t.Errorf("%s: StripSchema accepted %s", name, schema)
		}
	}
}

func TestSchemaSize(t *testing.T) {
	t.Logf("prompt schema bytes: %d", len(prompts.Schema()))
}
