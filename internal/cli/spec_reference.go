package cli

import (
	"bytes"
	"encoding/json"

	"github.com/spf13/cobra"

	observespec "github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/examples"
)

func (a *app) schemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "schema",
		Short:   "Print the JSON Schema of an ObserveSpec file",
		Long:    "schema prints the JSON Schema that every dashboard file must satisfy, the same document the get_spec_guide MCP tool returns.\nIt is part of this program, so it matches the Spec this version of observe writes and needs no server.",
		Example: "  observe dashboards schema > observespec.schema.json",
		Args:    noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			a.output().body(observespec.Schema())
			return nil
		},
	}
}

func (a *app) sampleCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "sample",
		Short:   "Print a complete example ObserveSpec file",
		Long:    "sample prints a working dashboard Spec with variables, several panel types and a layout, the same example the get_spec_guide MCP tool returns.\nSave it as a .json file, edit it, and send it with observe dashboards apply.",
		Example: "  observe dashboards sample > dashboards/checkout.json",
		Args:    noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, examples.CheckoutServiceHealth, "", "  "); err != nil {
				return err
			}
			a.output().body(pretty.Bytes())
			return nil
		},
	}
}
