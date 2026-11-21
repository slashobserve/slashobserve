package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func compileOutput(t *testing.T, name string, raw json.RawMessage) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("output.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := c.Compile("output.json")
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return schema
}

func validateOutput(t *testing.T, r *Registry, name string, content any) {
	t.Helper()
	var raw json.RawMessage
	for _, tool := range r.List() {
		if tool.Name == name {
			raw = tool.OutputSchema
		}
	}
	if raw == nil {
		t.Fatalf("%s has no output schema", name)
	}
	body, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonschema.UnmarshalJSON(strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	if err := compileOutput(t, name, raw).Validate(value); err != nil {
		t.Fatalf("%s: %v\nbody %s", name, err, body)
	}
}

func TestEveryOutputSchemaNamesARegisteredToolAndCompiles(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	listed := map[string]Tool{}
	for _, tool := range r.List() {
		listed[tool.Name] = tool
	}
	for name, schema := range outputSchemas {
		tool, ok := listed[name]
		if !ok {
			t.Errorf("outputSchemas names %q, which is not a tool", name)
			continue
		}
		compileOutput(t, name, tool.OutputSchema)
		var root struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(schema, &root); err != nil || root.Type != "object" {
			t.Errorf("%s: the root of an output schema must be an object, got %s", name, schema)
		}
	}
}

func TestOutputSchemasAcceptTheErrorBodyEveryToolAnswersWith(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	for name := range outputSchemas {
		validateOutput(t, r, name, errorBody{Code: codeInvalidRequest, Message: "no", Note: "n"})
	}
}

func TestListDatasourcesAndQueriesAnswerWhatTheirOutputSchemaPromises(t *testing.T) {
	ds := &fakeDS{
		rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
			return []datasource.Frame{{Name: "up", Labels: map[string]string{"job": "api"}, Fields: []datasource.Field{
				{Name: "time", Type: datasource.FieldTime, Values: []any{time.Unix(1700000000, 0), time.Unix(1700000060, 0)}},
				{Name: "value", Type: datasource.FieldNumber, Values: []any{1.0, 2.0}},
			}}}, nil
		},
		instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return []datasource.Frame{{Fields: []datasource.Field{
				{Name: "job", Type: datasource.FieldString, Values: []any{"api", "web"}},
				{Name: "value", Type: datasource.FieldNumber, Values: []any{1.0, 2.0}},
			}}}, nil
		},
	}
	r := newTestRegistry(t, ds, nil)
	ctx := auth.WithPrincipal(reading(context.Background()), auth.Principal{Role: auth.RoleViewer})
	for _, tt := range []struct{ tool, input string }{
		{nameListDatasources, `{}`},
		{nameQueryRange, `{"datasource":"fake","query":"up","from":"now-1h","to":"now"}`},
		{nameQueryInstant, `{"datasource":"fake","query":"up","from":"now-1h","to":"now"}`},
		{nameQueryInstant, `{"datasource":"fake","query":"up","from":"now-1h","to":"now","page_size":10}`},
		{nameQueryRange, `{"datasource":"nope","query":"up","from":"now-1h","to":"now"}`},
	} {
		res, err := r.Call(ctx, tt.tool, json.RawMessage(tt.input))
		if err != nil {
			t.Fatalf("%s: %v", tt.tool, err)
		}
		validateOutput(t, r, tt.tool, res.Content)
	}
}
