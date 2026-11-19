package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func (f *accessFixture) outputSchema(t *testing.T, tool string) *jsonschema.Schema {
	t.Helper()
	rec := f.post(rpc(1, "tools/list", map[string]any{}), map[string]string{"Authorization": bearer(adminKey)})
	var listed struct {
		Tools []struct {
			Name         string          `json:"name"`
			OutputSchema json.RawMessage `json:"outputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(decodeRPC(t, rec).Result, &listed); err != nil {
		t.Fatal(err)
	}
	for _, l := range listed.Tools {
		if l.Name != tool {
			continue
		}
		doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(l.OutputSchema)))
		if err != nil {
			t.Fatalf("%s outputSchema %s: %v", tool, l.OutputSchema, err)
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("output.json", doc); err != nil {
			t.Fatal(err)
		}
		schema, err := c.Compile("output.json")
		if err != nil {
			t.Fatal(err)
		}
		return schema
	}
	t.Fatalf("tools/list has no %s", tool)
	return nil
}

func TestDashboardToolsAnswerWhatTheirOutputSchemaPromises(t *testing.T) {
	f := newAccessFixture(t)
	id := f.seed(t, adminKey, pgtype.UUID{}, openTitle, "open-tag")
	for _, tt := range []struct {
		tool string
		args map[string]any
	}{
		{"list_dashboards", map[string]any{}},
		{"get_dashboard", map[string]any{"dashboard_id": id}},
		{"get_dashboard", map[string]any{"dashboard_id": id, "version": 1}},
		{"get_dashboard", map[string]any{"dashboard_id": "not-a-uuid"}},
		{"list_datasources", map[string]any{}},
	} {
		res := f.as(t, adminKey, tt.tool, tt.args)
		body, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		value, err := jsonschema.UnmarshalJSON(strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.outputSchema(t, tt.tool).Validate(value); err != nil {
			t.Errorf("%s: %v\nbody %s", tt.tool, err, body)
		}
	}
}
