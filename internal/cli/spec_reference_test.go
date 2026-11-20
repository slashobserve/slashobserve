package cli

import (
	"encoding/json"
	"strings"
	"testing"

	observespec "github.com/slashobserve/slashobserve/pkg/spec"
)

func TestDashboardsSchemaPrintsTheSpecSchemaWithoutAServer(t *testing.T) {
	stdout, stderr, code := runCLI("dashboards", "schema")
	var doc struct {
		ID       string         `json:"$id"`
		Required []string       `json:"required"`
		Props    map[string]any `json:"properties"`
	}
	if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &doc) != nil || !strings.HasSuffix(doc.ID, "observespec.v0.json") || doc.Props["panels"] == nil {
		t.Fatalf("exit %d, stderr %q, stdout starts %.80q", code, stderr, stdout)
	}
}

func TestDashboardsSamplePrintsASpecThatParses(t *testing.T) {
	stdout, stderr, code := runCLI("dashboards", "sample")
	if code != 0 || stderr != "" || !strings.HasSuffix(stdout, "}\n") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if _, err := observespec.Parse([]byte(stdout)); err != nil {
		t.Fatalf("the sample is not a valid Spec: %v", err)
	}
}

func TestSchemaAndSampleTakeNoArguments(t *testing.T) {
	for _, name := range []string{"schema", "sample"} {
		if _, _, code := runCLI("dashboards", name, "extra"); code != 2 {
			t.Errorf("%s with an argument exits %d, want 2", name, code)
		}
	}
}
