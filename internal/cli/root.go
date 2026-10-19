package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

type app struct {
	std     IO
	version string
	server  string
	token   string
	json    bool
	quiet   bool

	prompt         promptOptions
	status         statusOptions
	list           pageOptions
	versions       pageOptions
	show           showOptions
	commentList    commentListOptions
	commentResolve commentResolveOptions
	withToken      bool
}

type usageError struct{ message string }

func (e usageError) Error() string { return e.message }

func usagef(format string, args ...any) error {
	return usageError{message: fmt.Sprintf(format, args...)}
}

func Main(args []string, std IO, version string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := &app{std: std, version: version}
	root := a.command()
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	if message := strings.TrimSpace(err.Error()); message != "" {
		_, _ = fmt.Fprintln(std.Err, message)
	}
	var usage usageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

func (a *app) command() *cobra.Command {
	root := &cobra.Command{
		Use:           "observe [flags] <prompt words...>",
		Short:         "Turn a prompt into a dashboard",
		Long:          "observe sends a prompt to a SlashObserve server and prints the address of the dashboard the agent builds.\nA prompt whose first word is a command name (dashboards, comments, status, login, logout, version, help) must be quoted as one argument.\nFlags go before the prompt; every word after the first prompt word is part of the prompt, flag-like words included.",
		Example:       "  observe checkout p99 latency, last 6h\n  observe --dashboard <id> add a panel for errors by pod\n  observe dashboards list\n  observe comments list <dashboard-id>",
		Args:          cobra.ArbitraryArgs,
		RunE:          a.runPrompt,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(a.std.In)
	root.SetOut(a.std.Out)
	root.SetErr(a.std.Err)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{message: err.Error()} })
	root.CompletionOptions.DisableDefaultCmd = true

	flags := root.PersistentFlags()
	flags.StringVar(&a.server, "server", "", "server address (env OBSERVE_SERVER)")
	flags.StringVar(&a.token, "token", "", "service-account key; prefer env OBSERVE_TOKEN or observe login, since flags land in shell history and the process list")
	flags.BoolVar(&a.json, "json", false, "print the API response body instead of text")
	flags.BoolVarP(&a.quiet, "quiet", "q", false, "hide progress and confirmations; results and errors still print")
	a.addPromptFlags(root)
	root.Flags().SetInterspersed(false)

	root.AddCommand(a.statusCommand(), a.dashboardsCommand(), a.commentsCommand(), a.loginCommand(), a.logoutCommand(), a.versionCommand())
	return root
}

func (a *app) versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintln(a.std.Out, a.version)
			return nil
		},
	}
}

func groupArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usagef("unknown command %q for %s", text(args[0]), cmd.CommandPath())
	}
	return nil
}

func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usagef("unexpected argument %q for %s", args[0], cmd.CommandPath())
	}
	return nil
}

func exactArgs(count int, names string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != count {
			return usagef("%s takes %s", cmd.CommandPath(), names)
		}
		return nil
	}
}
