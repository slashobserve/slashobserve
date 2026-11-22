package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
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
	open    bool

	prompt            promptOptions
	status            statusOptions
	continuing        continueOptions
	retrying          retryOptions
	list              pageOptions
	listOrder         string
	excludeAudit      bool
	versions          pageOptions
	show              showOptions
	remove            deleteOptions
	commentList       commentListOptions
	commentAdd        commentAddOptions
	commentReply      commentReplyOptions
	commentResolve    commentResolveOptions
	importing         importOptions
	run               runOptions
	withToken         bool
	annotating        annotateOptions
	logs              logsOptions
	metricsQuery      metricsQueryOptions
	traces            tracesOptions
	grafanaConnect    grafanaConnectOptions
	grafanaImport     grafanaImportOptions
	health            healthOptions
	investigating     investigateOptions
	activity          activityOptions
	audit             auditOptions
	webhooks          webhooksOptions
	people            peopleOptions
	savedSearchList   savedSearchListOptions
	savedSearchSave   savedSearchSaveOptions
	savedSearchUpdate savedSearchUpdateOptions
	savedSearchMove   savedSearchMoveOptions
	searching         searchOptions
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
	message := strings.TrimSpace(err.Error())
	switch {
	case message != "" && a.json:
		_, _ = fmt.Fprintln(std.Err, errorJSON(err, message))
	case message != "":
		_, _ = fmt.Fprintln(std.Err, message)
	}
	var usage usageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

func errorJSON(err error, message string) string {
	var start, end *int
	var api *apiError
	if errors.As(err, &api) {
		start, end = api.start, api.end
	}
	data, marshalErr := json.Marshal(struct {
		Error string `json:"error"`
		Code  string `json:"code"`
		Start *int   `json:"start,omitempty"`
		End   *int   `json:"end,omitempty"`
	}{Error: message, Code: errorCode(err), Start: start, End: end})
	if marshalErr != nil {
		return message
	}
	return string(data)
}

func errorCode(err error) string {
	var usage usageError
	var api *apiError
	var transport transportFailure
	switch {
	case errors.As(err, &usage):
		return "usage"
	case errors.Is(err, errUnauthorized):
		return "unauthenticated"
	case errors.Is(err, errInterrupted):
		return "interrupted"
	case errors.As(err, &api) && api.code != "":
		return api.code
	case errors.As(err, &api):
		return "http_" + strconv.Itoa(api.status)
	case errors.As(err, &transport):
		return "unreachable"
	}
	return "failed"
}

func Commands() *cobra.Command {
	quiet := IO{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard}
	return (&app{std: quiet}).command()
}

func (a *app) command() *cobra.Command {
	root := &cobra.Command{
		Use:           "observe [flags] <prompt words...>",
		Short:         "Turn a prompt into a dashboard",
		Long:          "observe sends a prompt to a SlashObserve server and prints the address of the dashboard the agent builds.",
		Example:       "  observe checkout p99 latency, last 6h\n  observe --revise --dashboard <id> add a panel for errors by pod\n  observe dashboards list\n  observe comments list <dashboard-id>\n  observe import dashboard.json\n  observe run Checkout health\n  observe investigate why did checkout errors rise after 10:00\n  observe annotate Deploy checkout v1.4.2 --tags deploy\n  observe health\n  observe open <dashboard-id>\n  observe doctor\n  observe logs '{app=\"shop\"}' --datasource loki --tail\n  observe traces <trace-id> --datasource tempo\n  observe grafana import <connection-id> --all\n  observe activity --follow\n  observe skill install\n  observe webhooks list\n  observe proposals list <dashboard-id>\n  observe saved-searches list --kind logs\n  observe search checkout latency --kind dashboard\n  observe completion zsh",
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
	root.Version = a.version
	root.SetVersionTemplate("{{.Version}}\n")
	root.AddCommand(a.foldersCommand(), a.annotationsCommand())

	flags := root.PersistentFlags()
	flags.StringVar(&a.server, "server", "", "server address (env OBSERVE_SERVER)")
	flags.StringVar(&a.token, "token", "", "service-account key; prefer env OBSERVE_TOKEN or observe login, since flags land in shell history and the process list")
	flags.BoolVar(&a.json, "json", false, "print the API response body instead of text")
	flags.BoolVarP(&a.quiet, "quiet", "q", false, "hide progress and confirmations; results and errors still print")
	a.addPromptFlags(root)
	a.addOpenFlag(root)

	root.AddCommand(a.completionCommand(), a.statusCommand(), a.cancelCommand(), a.continueCommand(), a.retryCommand(), a.saveAnswerCommand(), a.shareAnswerCommand(), a.dashboardsCommand(), a.commentsCommand(), a.importCommand(), a.runCommand(), a.investigateCommand(), a.loginCommand(), a.logoutCommand(), a.versionCommand(), a.annotateCommand(), a.healthCommand(), a.openCommand(), a.doctorCommand(), a.logsCommand(), a.tracesCommand(), a.metricsGroupCommand(), a.grafanaCommand(), a.activityCommand(), a.auditCommand(), a.skillCommand(), a.webhooksCommand(), a.peopleCommand(), a.teamsCommand(), a.savedSearchesCommand(), a.searchCommand(), a.observationsCommand(), a.proposalsCommand(), a.requestsCommand(), a.chatsCommand(), a.datasourcesCommand(), a.accessCommand())
	root.Long += quotedCommandsNote(root)
	return root
}

func commandNames(root *cobra.Command) []string {
	names := []string{"help"}
	for _, c := range root.Commands() {
		names = append(names, c.Name())
		names = append(names, c.Aliases...)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func quotedCommandsNote(root *cobra.Command) string {
	return "\nA prompt whose first word is a command name (" + strings.Join(commandNames(root), ", ") + ") must be quoted as one argument.\nA lone word, or words whose first word nearly spells a command name, is refused as a probable typo; quote the words as one argument to send them as a prompt; a prompt with --dashboard is never refused."
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

func rangeArgs(least, most int, names string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < least || len(args) > most {
			return usagef("%s takes %s", cmd.CommandPath(), names)
		}
		return nil
	}
}

func minimumArgs(count int, names string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < count {
			return usagef("%s takes %s", cmd.CommandPath(), names)
		}
		return nil
	}
}
