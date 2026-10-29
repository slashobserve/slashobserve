package cli

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type runOptions struct {
	noWait  bool
	timeout time.Duration
}

type savedPrompt struct {
	Name  string `json:"name"`
	Body  string `json:"body"`
	Owner struct {
		Name string `json:"name"`
	} `json:"owner"`
}

type savedPromptList struct {
	Prompts []savedPrompt `json:"prompts"`
}

func (a *app) runCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [name...]",
		Short: "Run a shared saved prompt, or list them",
		Long:  "run sends the text of a saved prompt to the agent, like observe <prompt>, and prints the dashboard address.\nWithout a name it lists the prompts this key can run. A key runs prompts shared with everyone on the server; private prompts stay in the web app.\nThe name is matched ignoring case, and its words may be given unquoted.",
		Args:  cobra.ArbitraryArgs,
		RunE:  a.runSavedPrompt,
	}
	flags := cmd.Flags()
	flags.BoolVar(&a.run.noWait, "no-wait", false, "print the request id and exit without waiting")
	flags.DurationVar(&a.run.timeout, "timeout", defaultTimeout, "stop waiting after this long; the request keeps running on the server")
	return cmd
}

func promptName(args []string) string {
	return strings.Join(strings.Fields(strings.Join(args, " ")), " ")
}

func (a *app) runSavedPrompt(cmd *cobra.Command, args []string) error {
	name := promptName(args)
	if name != "" && a.run.timeout <= 0 {
		return usagef("--timeout must be positive")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	var query url.Values
	if name != "" {
		query = url.Values{"name": {name}}
	}
	data, err := c.get(ctx, "prompts", query)
	if err != nil {
		return err
	}
	list, err := decode[savedPromptList](data)
	if err != nil {
		return err
	}
	out := a.output()
	if name == "" {
		return printSavedPrompts(out, data, list, a.json)
	}
	if len(list.Prompts) == 0 {
		return fmt.Errorf("no shared prompt named %q; run observe run to list them", name)
	}
	prompt := list.Prompts[0]
	out.notice("Running %q", text(prompt.Name))
	return a.submit(ctx, c, map[string]any{"prompt": prompt.Body}, a.run.noWait, a.run.timeout)
}

func printSavedPrompts(out *output, data []byte, list savedPromptList, asJSON bool) error {
	if asJSON {
		out.body(data)
		return nil
	}
	if len(list.Prompts) == 0 {
		out.line("no shared prompts yet; share one from the chat home in the web app")
		return nil
	}
	rows := make([][]string, 0, len(list.Prompts))
	for _, p := range list.Prompts {
		rows = append(rows, []string{text(p.Name), text(p.Owner.Name)})
	}
	out.table([]string{"NAME", "SHARED BY"}, rows)
	return nil
}
