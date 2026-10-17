package tools

import (
	"context"
	"encoding/json"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/examples"
)

func (r *Registry) guideEntries() []entry {
	return []entry{
		{Tool{Name: nameGetSpecGuide, Description: descGetSpecGuide, InputSchema: json.RawMessage(schemaEmpty), ReadOnly: true}, r.getSpecGuide},
	}
}

type guideOut struct {
	Guide   string          `json:"guide"`
	Schema  json.RawMessage `json:"schema"`
	Example json.RawMessage `json:"example"`
}

func (r *Registry) getSpecGuide(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct{}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	return success("Read the Spec guide", guideOut{
		Guide:   prompts.SpecGuide(),
		Schema:  json.RawMessage(spec.Schema()),
		Example: json.RawMessage(examples.CheckoutServiceHealth),
	}), nil
}
