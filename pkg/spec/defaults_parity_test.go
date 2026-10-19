package spec_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/spec/schema"
)

const (
	webDefaultsPrefix = "export const specDefaults = "
	webDefaultsSuffix = " as const"
)

func TestWebDefaultsMatchSchemaDefaults(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "spec", "defaults.gen.ts"))
	if err != nil {
		t.Fatal(err)
	}
	_, module, found := strings.Cut(string(source), "*/\n")
	if !found {
		t.Fatal("defaults.gen.ts has no banner comment")
	}
	module = strings.TrimSpace(module)
	body, ok := strings.CutPrefix(module, webDefaultsPrefix)
	if !ok {
		t.Fatalf("defaults.gen.ts does not start with %q", webDefaultsPrefix)
	}
	body, ok = strings.CutSuffix(body, webDefaultsSuffix)
	if !ok {
		t.Fatalf("defaults.gen.ts does not end with %q", webDefaultsSuffix)
	}

	var web, file any
	if err := json.Unmarshal([]byte(body), &web); err != nil {
		t.Fatalf("defaults.gen.ts is not JSON after the wrapper is removed: %v", err)
	}
	if err := json.NewDecoder(bytes.NewReader(schema.Defaults)).Decode(&file); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(web, file) {
		t.Fatal("web/src/spec/defaults.gen.ts differs from spec/schema/defaults.json; run make generate")
	}
}
