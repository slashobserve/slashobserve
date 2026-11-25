package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	kindInvestigate = "investigate"
	kindAsk         = "ask"
	kindRevise      = "revise"
)

type investigateOptions struct {
	dashboard   string
	panel       string
	observation string
	block       string
	at          string
	from        string
	to          string
	asOf        string
	private     bool
	noWait      bool
	timeout     time.Duration
}

type findingSummary struct {
	ID       string        `json:"id"`
	RunID    string        `json:"run_id"`
	Title    string        `json:"title"`
	Markdown string        `json:"markdown"`
	URL      string        `json:"url"`
	AskOn    *findingAskOn `json:"ask_on"`

	SuggestedChange string `json:"suggested_change"`
	DashboardID     string `json:"dashboard_id"`

	RootCause  *findingRootCause `json:"root_cause"`
	Onset      string            `json:"onset"`
	Impact     string            `json:"impact"`
	Confidence string            `json:"confidence"`
	RuledOut   []string          `json:"ruled_out"`
	NextSteps  []string          `json:"next_steps"`
	NoIncident bool              `json:"no_incident"`
}

type findingRootCause struct {
	Component string `json:"component"`
	Kind      string `json:"kind"`
	Summary   string `json:"summary"`
}

func (f findingSummary) header() []string {
	var lines []string
	if c := f.RootCause; c != nil {
		lines = append(lines, fmt.Sprintf("Root cause: %s (%s) - %s", text(c.Component), text(c.Kind), text(c.Summary)))
	}
	if f.NoIncident {
		lines = append(lines, "No incident found.")
	}
	for _, field := range [][2]string{{"Onset", f.Onset}, {"Impact", f.Impact}, {"Confidence", f.Confidence}} {
		if field[1] != "" {
			lines = append(lines, field[0]+": "+text(field[1]))
		}
	}
	for _, list := range []struct {
		label string
		items []string
	}{{"Ruled out", f.RuledOut}, {"Next steps", f.NextSteps}} {
		if len(list.items) == 0 {
			continue
		}
		cleaned := make([]string, len(list.items))
		for i, item := range list.items {
			cleaned[i] = text(item)
		}
		lines = append(lines, list.label+": "+strings.Join(cleaned, "; "))
	}
	return lines
}

type findingAskOn struct {
	DashboardID string `json:"dashboard_id"`
	Title       string `json:"title"`
}

func printAskOn(out *output, askOn *findingAskOn, prompt string) {
	if askOn == nil {
		return
	}
	out.line("")
	out.line("This change is made on the dashboard %s. Ask there:", text(askOn.Title))
	out.line("  observe --revise --dashboard %s %s", text(askOn.DashboardID), shellWord(text(prompt)))
}

func shellWord(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (f findingSummary) saved() bool {
	return f.URL != ""
}

func (a *app) investigateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "investigate <question...>",
		Short: "Ask the agent a question about your data and print its finding",
		Long: "investigate asks the agent a question, follows it while it reads your datasources, then prints the finding: its title, its answer and its address.\n" +
			"With --dashboard the question is about that dashboard, and --panel narrows it to one of its panels.",
		Example: "  observe investigate why did checkout errors rise after 10:00\n" +
			"  observe investigate --dashboard <id> --panel <panel-id> --from now-6h --to now what changed\n" +
			"  observe investigate --observation <id> why did checkout errors rise\n" +
			"  observe investigate --as-of 2026-11-20T10:30:00Z --from now-1h --to now why did checkout errors rise\n" +
			"  observe investigate --private --dashboard <id> why is this panel flat",
		Args: minimumArgs(1, "a question"),
		RunE: a.runInvestigate,
	}
	flags := cmd.Flags()
	flags.StringVar(&a.investigating.dashboard, "dashboard", "", "the dashboard the question is about")
	flags.StringVar(&a.investigating.panel, "panel", "", "one panel of --dashboard the question is about")
	flags.StringVar(&a.investigating.observation, "observation", "", "the observation to write into, by id; without it a new observation is made")
	flags.StringVar(&a.investigating.at, "at", "", "the spot the question is about, as the anchor JSON object or a short form such as pos:0.4,0.3; needs --dashboard or --observation")
	flags.StringVar(&a.investigating.block, "block", "", "a block of --observation the question is about, from observe o show; with --at, the block --at points into")
	flags.StringVar(&a.investigating.from, "from", "", "start of the window to look at, such as now-6h or epoch milliseconds; needs --to")
	flags.StringVar(&a.investigating.to, "to", "", "end of the window to look at, such as now or epoch milliseconds; needs --from")
	flags.StringVar(&a.investigating.asOf, "as-of", "", "Admin only: the time to investigate as of, as epoch milliseconds or an RFC 3339 time in the past; now-1h then means the hour before it, and --to must not be later")
	flags.BoolVar(&a.investigating.private, "private", false, "answer privately about --dashboard or --observation, writing nothing shared until you share the answer")
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

func (o investigateOptions) kind() string {
	if o.private {
		return kindAsk
	}
	return kindInvestigate
}

func (o investigateOptions) check(question string) error {
	switch {
	case question == "":
		return usagef("the question is empty")
	case o.timeout <= 0:
		return usagef("--timeout must be positive")
	case o.panel != "" && o.dashboard == "":
		return usagef("--panel needs --dashboard")
	case o.block != "" && o.observation == "":
		return usagef("--block needs --observation")
	case (o.from == "") != (o.to == ""):
		return usagef("--from and --to go together")
	case o.private && (o.dashboard == "") == (o.observation == ""):
		return usagef("--private needs one of --dashboard or --observation")
	}
	return nil
}

func investigatePayload(args []string, o investigateOptions) (map[string]any, error) {
	question := strings.TrimSpace(strings.Join(args, " "))
	if err := o.check(question); err != nil {
		return nil, err
	}
	payload := map[string]any{"prompt": question, "kind": o.kind()}
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
	if o.observation != "" {
		id, err := parseID(o.observation)
		if err != nil {
			return nil, err
		}
		payload["observation_id"] = id
	}
	if o.block != "" {
		id, err := parseID(o.block)
		if err != nil {
			return nil, err
		}
		payload["block_id"] = id
	}
	if err := addAnchor(payload, o.at); err != nil {
		return nil, err
	}
	if o.from != "" {
		payload["from"], payload["to"] = o.from, o.to
	}
	if o.asOf != "" {
		payload["as_of"] = o.asOf
	}
	return payload, nil
}

func printFinding(out *output, f findingSummary, requestID string) {
	out.line("%s", text(f.Title))
	out.line("")
	if header := f.header(); len(header) > 0 {
		for _, line := range header {
			out.line("%s", line)
		}
		out.line("")
	}
	for line := range strings.SplitSeq(strings.TrimSpace(f.Markdown), "\n") {
		out.line("%s", text(line))
	}
	out.line("")
	if f.SuggestedChange != "" {
		out.line("Suggested change: %s", text(f.SuggestedChange))
		out.line("  observe --revise --dashboard %s %s", text(f.DashboardID), shellWord(text(f.SuggestedChange)))
		out.line("")
	}
	if f.saved() {
		out.line("%s", text(f.URL))
		return
	}
	out.line("This answer is kept in the chat only. Save it as an observation: observe save-answer %s", text(requestID))
}
