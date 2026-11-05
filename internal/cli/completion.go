package cli

import (
	"io"

	"github.com/spf13/cobra"
)

func (a *app) completionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion <bash|zsh|fish>",
		Short: "Print a shell completion script",
		Long:  "completion prints the completion script for bash, zsh or fish.\nLoad it in the current shell with source <(observe completion bash), or write it to your shell's completion directory.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	shells := []struct {
		name     string
		generate func(root *cobra.Command, out io.Writer) error
	}{
		{"bash", func(root *cobra.Command, out io.Writer) error { return root.GenBashCompletionV2(out, true) }},
		{"zsh", func(root *cobra.Command, out io.Writer) error { return root.GenZshCompletion(out) }},
		{"fish", func(root *cobra.Command, out io.Writer) error { return root.GenFishCompletion(out, true) }},
	}
	for _, shell := range shells {
		cmd.AddCommand(&cobra.Command{
			Use:   shell.name,
			Short: "Print the " + shell.name + " completion script",
			Args:  noArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return shell.generate(cmd.Root(), a.std.Out)
			},
		})
	}
	return cmd
}
