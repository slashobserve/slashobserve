package tools

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/examples"
)

func (r *Registry) guideEntries() []entry {
	return []entry{
		{Tool{Name: nameGetSpecGuide, Description: descGetSpecGuide, InputSchema: json.RawMessage(schemaEmpty), ReadOnly: true, MinRole: auth.RoleViewer}, r.getSpecGuide},
	}
}

type guideOut struct {
	Guide   string          `json:"guide"`
	Schema  json.RawMessage `json:"schema"`
	Example json.RawMessage `json:"example"`
}

type schemaOut struct {
	Schema json.RawMessage `json:"schema"`
}

func (r *Registry) getSpecGuide(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct{}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if _, inRun := RunFrom(ctx); inRun {
		return success("Read the Spec schema", schemaOut{Schema: json.RawMessage(prompts.Schema())}), nil
	}
	return success("Read the Spec guide", guideOut{
		Guide:   prompts.GuideFor(configuredTypes(r.deps.Datasources)),
		Schema:  json.RawMessage(prompts.Schema()),
		Example: json.RawMessage(examples.CheckoutServiceHealth),
	}), nil
}

func configuredTypes(reg *datasource.Registry) []spec.DatasourceType {
	if reg == nil {
		return nil
	}
	var types []spec.DatasourceType
	for _, inst := range reg.List() {
		if t := typeOf(inst); !slices.Contains(types, t) {
			types = append(types, t)
		}
	}
	return types
}
