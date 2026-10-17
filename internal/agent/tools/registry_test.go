package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestDecodeInput(t *testing.T) {
	type sample struct {
		Datasource string `json:"datasource"`
		Limit      *int   `json:"limit"`
		Count      int    `json:"count"`
	}
	tests := []struct {
		name    string
		input   string
		wantErr string
		check   func(t *testing.T, s sample)
	}{
		{"empty input", ``, "", nil},
		{"null input", `null`, "", nil},
		{"exact keys", `{"datasource":"a","limit":5}`, "", func(t *testing.T, s sample) {
			if s.Datasource != "a" || *s.Limit != 5 {
				t.Fatalf("decoded %+v", s)
			}
		}},
		{"upper case key", `{"DATASOURCE":"a"}`, `unknown field "DATASOURCE"`, nil},
		{"mixed case key", `{"Datasource":"a"}`, `unknown field "Datasource"`, nil},
		{"unknown key", `{"nope":1}`, `unknown field "nope"`, nil},
		{"unknown next to valid", `{"datasource":"a","extra":true}`, `unknown field "extra"`, nil},
		{"integral float", `{"limit":200.0}`, "", func(t *testing.T, s sample) {
			if *s.Limit != 200 {
				t.Fatalf("limit = %d", *s.Limit)
			}
		}},
		{"integral exponent", `{"limit":2e2}`, "", func(t *testing.T, s sample) {
			if *s.Limit != 200 {
				t.Fatalf("limit = %d", *s.Limit)
			}
		}},
		{"integral float on plain int", `{"count":3.0}`, "", func(t *testing.T, s sample) {
			if s.Count != 3 {
				t.Fatalf("count = %d", s.Count)
			}
		}},
		{"negative integral float", `{"count":-3.0}`, "", func(t *testing.T, s sample) {
			if s.Count != -3 {
				t.Fatalf("count = %d", s.Count)
			}
		}},
		{"fractional", `{"limit":10.5}`, `field "limit" must be an integer`, nil},
		{"fractional exponent", `{"limit":1e-1}`, `field "limit" must be an integer`, nil},
		{"huge float", `{"limit":1e30}`, `field "limit" must be an integer`, nil},
		{"string for integer", `{"limit":"5"}`, `field "limit" has the wrong type`, nil},
		{"string for string field", `{"datasource":5}`, `field "datasource" has the wrong type`, nil},
		{"float for string field", `{"datasource":1.5}`, `field "datasource" has the wrong type`, nil},
		{"array", `[]`, "input must be a JSON object", nil},
		{"scalar", `7`, "input must be a JSON object", nil},
		{"trailing value", `{} {}`, "input must be a single JSON object", nil},
		{"truncated", `{"datasource"`, "input is not valid JSON", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s sample
			res, ok := decodeInput(json.RawMessage(tt.input), &s)
			if tt.wantErr == "" {
				if !ok {
					t.Fatalf("rejected: %#v", res.Content)
				}
				if tt.check != nil {
					tt.check(t, s)
				}
				return
			}
			if ok {
				t.Fatalf("accepted %+v", s)
			}
			if errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("content = %#v", res.Content)
			}
			if got := res.Content.(errorBody).Message; got != tt.wantErr {
				t.Fatalf("message = %q, want %q", got, tt.wantErr)
			}
		})
	}
}

func TestDecodeInputEmbeddedAndRawFields(t *testing.T) {
	var in rangeInput
	if res, ok := decodeInput(json.RawMessage(`{"datasource":"a","query":"q","from":"now-1h","max_data_points":100.0}`), &in); !ok {
		t.Fatalf("rejected: %#v", res.Content)
	}
	if in.Datasource != "a" || in.From != "now-1h" || *in.MaxDataPoints != 100 {
		t.Fatalf("decoded %+v", in)
	}
	if _, ok := decodeInput(json.RawMessage(`{"Query":"q"}`), &in); ok {
		t.Fatal("case-mismatched embedded key accepted")
	}

	rawSpec := `{ "title": "a <b> & c",   "panels": [] }`
	var save createInput
	if res, ok := decodeInput(json.RawMessage(`{"summary":"s","spec":`+rawSpec+`}`), &save); !ok {
		t.Fatalf("rejected: %#v", res.Content)
	}
	if string(save.Spec) != rawSpec {
		t.Fatalf("spec bytes changed: %q", save.Spec)
	}
}

func TestCallRejectsLooseInputWithStep(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	tests := []struct{ name, tool, input string }{
		{"case mismatch", nameLintQuery, `{"DATASOURCE":"fake","query":"up"}`},
		{"unknown field", nameLintQuery, `{"datasource":"fake","query":"up","x":1}`},
		{"fractional integer", nameQueryRange, `{"datasource":"fake","query":"up","max_data_points":20.5}`},
		{"empty-object tool with a key", nameListDatasources, `{"x":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := call(t, r, tt.tool, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("content = %#v", res.Content)
			}
			if want := "Rejected " + tt.tool + " input: invalid_request"; res.Step != want {
				t.Fatalf("step = %q, want %q", res.Step, want)
			}
		})
	}
}

func TestUnknownToolStepNeverEchoesTheName(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, "secret-looking-name", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != codeUnknownTool || strings.Contains(res.Step, "secret") || res.Step == "" {
		t.Fatalf("res = %#v", res)
	}
}

func TestNonInputErrorsUseFailedStep(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, nameLintQuery, `{"datasource":"nope","query":"up"}`)
	if err != nil || res.Step != "Could not run lint_query: unknown_datasource" {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	for _, code := range []string{codeNotFound, codeTooLarge, "query_timeout"} {
		r.entries[nameGetDashboard] = entry{handle: func(context.Context, json.RawMessage) (Result, error) {
			return toolError(code, "model-visible detail"), nil
		}}
		res, err := call(t, r, nameGetDashboard, `{}`)
		if err != nil || res.Step != "Could not run get_dashboard: "+code {
			t.Fatalf("%s: res = %#v err = %v", code, res, err)
		}
	}
}

func TestToolErrorsKeepSpecificSteps(t *testing.T) {
	ds := &fakeDS{instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "bad")
	}}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameQueryInstant, `{"datasource":"fake","query":"up"}`)
	if err != nil || res.Step != "Ran a query on fake: query_invalid" {
		t.Fatalf("res = %#v err = %v", res, err)
	}
}

func TestCallRecoversPanics(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	r := newTestRegistry(t, &fakeDS{}, logger)
	r.entries[nameLintQuery] = entry{handle: func(context.Context, json.RawMessage) (Result, error) {
		panic("boom " + "hunter2")
	}}
	res, err := call(t, r, nameLintQuery, `{"query":"super-secret-query"}`)
	if err == nil || res.Content != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	logged := logs.String()
	if !strings.Contains(logged, nameLintQuery) {
		t.Fatalf("log lacks the tool name: %s", logged)
	}
	for _, leaked := range []string{"super-secret-query", "hunter2"} {
		if strings.Contains(logged, leaked) || strings.Contains(err.Error(), leaked) {
			t.Fatalf("%q leaked: %s / %v", leaked, logged, err)
		}
	}
}

func TestCallRecoversDatasourcePanics(t *testing.T) {
	ds := &fakeDS{instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
		panic("driver bug")
	}}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameQueryInstant, `{"datasource":"fake","query":"up"}`)
	if err == nil || res.Content != nil {
		t.Fatalf("res = %#v err = %v", res, err)
	}
}

func TestCallResultCaps(t *testing.T) {
	big := func(n int) handler {
		return func(context.Context, json.RawMessage) (Result, error) {
			return success("ok", map[string]string{"data": strings.Repeat("a", n)}), nil
		}
	}
	r := newTestRegistry(t, &fakeDS{}, nil)

	r.entries[nameListMetrics] = entry{handle: big(maxContentBytes - 20)}
	if _, err := call(t, r, nameListMetrics, `{}`); err != nil {
		t.Fatalf("result under the cap rejected: %v", err)
	}
	r.entries[nameListMetrics] = entry{handle: big(maxContentBytes)}
	if res, err := call(t, r, nameListMetrics, `{}`); err == nil || res.Content != nil {
		t.Fatalf("oversized result accepted: err = %v", err)
	}

	if getDashboardLimit != 128<<10 {
		t.Fatalf("get_dashboard cap = %d, want 128 KiB", getDashboardLimit)
	}
	r.entries[nameGetDashboard] = entry{handle: big(maxContentBytes * 4)}
	if _, err := call(t, r, nameGetDashboard, `{}`); err != nil {
		t.Fatalf("get_dashboard result above 16 KiB rejected: %v", err)
	}
	r.entries[nameGetDashboard] = entry{handle: big(getDashboardLimit - 20)}
	if _, err := call(t, r, nameGetDashboard, `{}`); err != nil {
		t.Fatalf("get_dashboard result at its cap rejected: %v", err)
	}
	r.entries[nameGetDashboard] = entry{handle: big(getDashboardLimit)}
	if res, err := call(t, r, nameGetDashboard, `{}`); err == nil || res.Content != nil {
		t.Fatalf("get_dashboard result over its cap accepted: err = %v", err)
	}
}

func TestListSchemas(t *testing.T) {
	wantNames := []string{
		"create_dashboard", "get_dashboard", "get_schema", "get_spec_guide", "lint_query",
		"list_dashboards", "list_datasources", "list_label_values", "list_labels", "list_metrics",
		"query_instant", "query_range", "sample_rows", "update_dashboard",
	}
	first := New(Deps{}).List()
	var names []string
	for _, tool := range first {
		names = append(names, tool.Name)
	}
	if !slices.Equal(names, wantNames) || !slices.IsSorted(names) {
		t.Fatalf("names = %v", names)
	}

	second := New(Deps{}).List()
	for i, tool := range first {
		if tool.Description == "" || strings.ContainsRune(tool.Description, 0x2014) {
			t.Errorf("%s: bad description %q", tool.Name, tool.Description)
		}
		if !bytes.Equal(tool.InputSchema, second[i].InputSchema) || tool.Description != second[i].Description {
			t.Errorf("%s: not byte-stable across registries", tool.Name)
		}
		checkSchema(t, tool)
	}

	list := New(Deps{}).List()
	list[0].InputSchema[0] = 'X'
	if again := New(Deps{}).List(); again[0].InputSchema[0] != '{' {
		t.Fatal("List exposes the registry's schema bytes")
	}
}

var numericKeywords = []string{"minimum", "maximum", "minLength", "maxLength", "minItems", "maxItems", "maxProperties"}

func checkSchema(t *testing.T, tool Tool) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(tool.InputSchema))
	dec.UseNumber()
	var schema map[string]any
	if err := dec.Decode(&schema); err != nil {
		t.Errorf("%s: schema is not valid JSON: %v", tool.Name, err)
		return
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Errorf("%s: root must be a closed object: %v", tool.Name, schema)
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		t.Errorf("%s: no properties", tool.Name)
	}
	if required, ok := schema["required"].([]any); ok {
		for _, name := range required {
			if _, ok := props[name.(string)]; !ok {
				t.Errorf("%s: required %q is not a property", tool.Name, name)
			}
		}
	}
	walkSchema(t, tool.Name, schema)
}

func walkSchema(t *testing.T, tool string, node any) {
	t.Helper()
	switch v := node.(type) {
	case map[string]any:
		for _, kw := range numericKeywords {
			if value, ok := v[kw]; ok {
				if _, isNumber := value.(json.Number); !isNumber {
					t.Errorf("%s: %s must be a JSON number, got %T", tool, kw, value)
				}
			}
		}
		for _, child := range v {
			walkSchema(t, tool, child)
		}
	case []any:
		for _, child := range v {
			walkSchema(t, tool, child)
		}
	}
}

func TestSpecGuideFitsWithMargin(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, nameGetSpecGuide, `{}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	if limit := maxContentBytes * 3 / 4; len(body) > limit {
		t.Fatalf("guide result is %d bytes, over 75%% of the cap (%d)", len(body), limit)
	}
	var out struct {
		Guide   string          `json:"guide"`
		Schema  json.RawMessage `json:"schema"`
		Example json.RawMessage `json:"example"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Guide == "" || len(out.Schema) == 0 || len(out.Example) == 0 {
		t.Fatalf("incomplete guide: %v", err)
	}
}
