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

func TestSchemaJudgesFilesLikeTheFullSchema(t *testing.T) {
	full := compile(t, spec.Schema())
	trimmed := compile(t, prompts.Schema())
	root, err := os.OpenRoot(filepath.Join("..", "..", "..", "spec", "examples"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	valid, err := fs.Glob(root.FS(), "*.json")
	if err != nil || len(valid) == 0 {
		t.Fatalf("valid examples: %v, %v", valid, err)
	}
	invalid, err := fs.Glob(root.FS(), "invalid/schema/*.json")
	if err != nil || len(invalid) == 0 {
		t.Fatalf("invalid examples: %v, %v", invalid, err)
	}
	for _, file := range valid {
		if !accepts(t, root, trimmed, file) {
			t.Errorf("prompt schema rejects valid %s", file)
		}
	}
	for _, file := range invalid {
		if accepts(t, root, full, file) {
			t.Fatalf("full schema accepts %s, so it is not a schema fixture", file)
		}
		if accepts(t, root, trimmed, file) {
			t.Errorf("prompt schema accepts invalid %s", file)
		}
	}
}
