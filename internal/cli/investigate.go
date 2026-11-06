package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const sourceInvestigate = "investigate"

type investigateOptions struct {
	dashboard string
	panel     string
	from      string
	to        string
	noWait    bool
	timeout   time.Duration
}

type findingSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
	URL      string `json:"url"`
}

func (a *app) investigateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "investigate <question...>",
		Short: "Ask the agent a question about your data and print its finding",
		Long: "investigate asks the agent a question, follows it while it reads your datasources, then prints the finding: its title, its answer and its address.\n" +
			"With --dashboard the question is about that dashboard, and --panel narrows it to one of its panels.",
		Example: "  observe investigate why did checkout errors rise after 10:00\n" +
			"  observe investigate --dashboard <id> --panel <panel-id> --from now-6h --to now what changed",
		Args: minimumArgs(1, "a question"),
		RunE: a.runInvestigate,
	}
	flags := cmd.Flags()
	flags.StringVar(&a.investigating.dashboard, "dashboard", "", "the dashboard the question is about")
	flags.StringVar(&a.investigating.panel, "panel", "", "one panel of --dashboard the question is about")
	flags.StringVar(&a.investigating.from, "from", "", "start of the window to look at, such as now-6h or epoch milliseconds; needs --to")
	flags.StringVar(&a.investigating.to, "to", "", "end of the window to look at, such as now or epoch milliseconds; needs --from")
	flags.BoolVar(&a.investigating.noWait, "no-wait", false, "print the request id and exit without waiting")
	flags.DurationVar(&a.investigating.timeout, "timeout", defaultTimeout, "stop waiting after this long; the investigation keeps running on the server")
	flags.BoolVar(&a.open, "open", false, "open the finding in the browser once it is saved")
	return cmd
}

func (a *app) runInvestigate(cmd *cobra.Command, args []string) error {
	payload, err := investigatePayload(args, a.investigating)
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	return a.submit(cmd.Context(), c, payload, a.investigating.noWait, a.investigating.timeout)
}

func investigatePayload(args []string, o investigateOptions) (map[string]any, error) {
	question := strings.TrimSpace(strings.Join(args, " "))
	switch {
	case question == "":
		return nil, usagef("the question is empty")
	case o.timeout <= 0:
		return nil, usagef("--timeout must be positive")
	case o.panel != "" && o.dashboard == "":
		return nil, usagef("--panel needs --dashboard")
	case (o.from == "") != (o.to == ""):
		return nil, usagef("--from and --to go together")
	}
	payload := map[string]any{"prompt": question, "kind": sourceInvestigate}
	if o.dashboard != "" {
		id, err := parseID(o.dashboard)
		if err != nil {
			return nil, err
		}
		payload["dashboard_id"] = id
	}
	if o.panel != "" {
		payload["panel_id"] = o.panel
	}
	if o.from != "" {
		payload["from"], payload["to"] = o.from, o.to
	}
	return payload, nil
}

func printFinding(out *output, f findingSummary) {
	out.line("%s", text(f.Title))
	out.line("")
	for line := range strings.SplitSeq(strings.TrimSpace(f.Markdown), "\n") {
		out.line("%s", text(line))
	}
	out.line("")
	out.line("%s", text(f.URL))
}
