package contextsources

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	maxArgumentIssues   = 5
	maxIssueBytes       = 200
	maxLoggedValueBytes = 200
	maxLoggedArguments  = 16
	maskedValue         = "[secret]"
	schemaResource      = "schema.json"
)

var errNoReferences = errors.New("a context tool's schema may not load other documents")

type noLoader struct{}

func (noLoader) Load(string) (any, error) { return nil, errNoReferences }

func compileSchema(raw json.RawMessage) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(noLoader{})
	if err := c.AddResource(schemaResource, doc); err != nil {
		return nil, err
	}
	return c.Compile(schemaResource)
}

func checkArguments(schema json.RawMessage, arguments json.RawMessage) error {
	compiled, err := compileSchema(schema)
	if err != nil {
		return &tools.ContextFailure{Code: tools.ContextCodeSchemaUnusable, Message: "the pinned input schema of this tool cannot be checked, so the tool cannot be called; an Admin has to look at the source"}
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(arguments))
	if err != nil {
		return &tools.ContextFailure{Code: tools.ContextCodeInvalidArguments, Message: "arguments must be a JSON object"}
	}
	err = compiled.Validate(instance)
	if err == nil {
		return nil
	}
	var violation *jsonschema.ValidationError
	if !errors.As(err, &violation) {
		return &tools.ContextFailure{Code: tools.ContextCodeInvalidArguments, Message: "arguments do not match the tool's input schema"}
	}
	return &tools.ContextFailure{Code: tools.ContextCodeInvalidArguments, Message: "arguments do not match the tool's input schema: " + issuesOf(violation)}
}

func issuesOf(violation *jsonschema.ValidationError) string {
	var issues []string
	for _, unit := range violation.BasicOutput().Errors {
		if unit.Error == nil {
			continue
		}
		where := unit.InstanceLocation
		if where == "" {
			where = "/"
		}
		issue := queryproxy.ClipBytes(fmt.Sprintf("%s: %s", where, unit.Error.String()), maxIssueBytes)
		if !slices.Contains(issues, issue) {
			issues = append(issues, issue)
		}
		if len(issues) == maxArgumentIssues {
			break
		}
	}
	return strings.Join(issues, "; ")
}

type propertyMarks struct {
	Properties map[string]struct {
		WriteOnly bool   `json:"writeOnly"`
		Format    string `json:"format"`
	} `json:"properties"`
}

func secretArguments(schema json.RawMessage) map[string]bool {
	var marks propertyMarks
	if json.Unmarshal(schema, &marks) != nil {
		return nil
	}
	out := map[string]bool{}
	for name, p := range marks.Properties {
		if p.WriteOnly || p.Format == "password" {
			out[name] = true
		}
	}
	return out
}

func loggedArguments(schema json.RawMessage, arguments json.RawMessage) map[string]any {
	var values map[string]json.RawMessage
	if json.Unmarshal(arguments, &values) != nil || len(values) == 0 {
		return nil
	}
	secret := secretArguments(schema)
	out := map[string]any{}
	for _, name := range slices.Sorted(maps.Keys(values)) {
		if len(out) == maxLoggedArguments {
			break
		}
		out[name] = loggedValue(values[name], secret[name] || audit.SecretName(name))
	}
	return out
}

func loggedValue(raw json.RawMessage, secret bool) any {
	if secret {
		return maskedValue
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return maskedValue
	}
	switch v := value.(type) {
	case string:
		return queryproxy.ClipBytes(audit.RecordedQuery(v), maxLoggedValueBytes)
	case bool, float64, nil:
		return v
	case []any:
		return fmt.Sprintf("[%d values]", len(v))
	}
	return "[object]"
}
