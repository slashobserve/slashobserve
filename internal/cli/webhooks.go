package cli

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultDeliveriesLimit = 20
	maxDeliveriesLimit     = 100
)

type webhooksOptions struct {
	name   string
	url    string
	events []string
	limit  int
}

type webhookView struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	URL            string   `json:"url"`
	EventTypes     []string `json:"event_types"`
	Enabled        bool     `json:"enabled"`
	DisabledReason *string  `json:"disabled_reason"`
	Owner          struct {
		Label string `json:"label"`
	} `json:"owner"`
}

type webhookList struct {
	Webhooks []webhookView `json:"webhooks"`
}

type webhookWithSecret struct {
	Webhook webhookView `json:"webhook"`
	Secret  string      `json:"secret"`
}

type webhookDelivery struct {
	DeliveryID  string  `json:"delivery_id"`
	EventType   string  `json:"event_type"`
	ActivitySeq *int64  `json:"activity_seq"`
	AttemptedAt string  `json:"attempted_at"`
	DurationMs  int     `json:"duration_ms"`
	StatusCode  *int    `json:"status_code"`
	Outcome     string  `json:"outcome"`
	ErrorCode   *string `json:"error_code"`
}

type webhookTest struct {
	Delivery webhookDelivery `json:"delivery"`
}

type webhookDeliveries struct {
	Deliveries []webhookDelivery `json:"deliveries"`
}

func (a *app) webhooksCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhooks",
		Short: "Send workspace activity to signed webhooks",
		Long:  "webhooks manages the addresses that receive workspace activity as signed requests.\nEvery verb needs an Admin key.",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.webhooksListCommand(), a.webhooksAddCommand(), a.webhooksUpdateCommand(), a.webhooksSwitchCommand(true), a.webhooksSwitchCommand(false), a.webhooksTestCommand(), a.webhooksRotateCommand(), a.webhooksDeleteCommand(), a.webhooksDeliveriesCommand())
	return cmd
}

func (a *app) webhooksListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the webhooks",
		Args:  noArgs,
		RunE:  a.runWebhooksList,
	}
}

func (a *app) webhooksAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add --name <name> --url <https address> --event <type> [--event <type> ...]",
		Short:   "Create a webhook and print its signing secret once",
		Long:    "add creates a webhook that posts the chosen activity event types to an https address.\nThe address's domain must already be on the outbound allowlist.\nThe signing secret is printed once and cannot be shown again; rotate it to get a new one.",
		Example: "  observe webhooks add --name \"Failed runs\" --url https://hooks.example.com/slashobserve --event run.failed",
		Args:    noArgs,
		RunE:    a.runWebhooksAdd,
	}
	a.webhookFields(cmd)
	return cmd
}

func (a *app) webhooksUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <webhook id> [--name <name>] [--url <https address>] [--event <type> ...]",
		Short: "Change a webhook's name, address or event types",
		Long:  "update changes only the fields you pass; --event replaces the whole list of event types.\nYou become the webhook's owner, so it sends what you can read.",
		Args:  exactArgs(1, "a webhook id"),
		RunE:  a.runWebhooksUpdate,
	}
	a.webhookFields(cmd)
	return cmd
}

func (a *app) webhookFields(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&a.webhooks.name, "name", "", "the webhook's name")
	flags.StringVar(&a.webhooks.url, "url", "", "the https address on port 443 that receives the events")
	flags.StringSliceVar(&a.webhooks.events, "event", nil, "an activity event type to send, such as run.failed; repeat for several")
}

func (a *app) webhooksSwitchCommand(enable bool) *cobra.Command {
	use, short := "disable <webhook id>", "Stop a webhook sending"
	if enable {
		use, short = "enable <webhook id>", "Turn a webhook on and clear its failures"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  exactArgs(1, "a webhook id"),
		RunE:  func(cmd *cobra.Command, args []string) error { return a.runWebhooksSwitch(cmd, args, enable) },
	}
}

func (a *app) webhooksTestCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "test <webhook id>",
		Short: "Send one test event now",
		Long:  "test sends a signed webhook.test event and prints the outcome, the receiver's status and the duration.\nIt exits 1 when the event was not delivered.",
		Args:  exactArgs(1, "a webhook id"),
		RunE:  a.runWebhooksTest,
	}
}

func (a *app) webhooksRotateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rotate-secret <webhook id>",
		Short: "Make a new signing secret and print it once",
		Long:  "rotate-secret makes a new signing secret at once; the old one stops working with the next request.\nThe new secret is printed once and cannot be shown again.\nYou become the webhook's owner.",
		Args:  exactArgs(1, "a webhook id"),
		RunE:  a.runWebhooksRotate,
	}
}

func (a *app) webhooksDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <webhook id>",
		Short: "Delete a webhook and its recorded attempts",
		Args:  exactArgs(1, "a webhook id"),
		RunE:  a.runWebhooksDelete,
	}
}

func (a *app) webhooksDeliveriesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deliveries <webhook id> [--limit N]",
		Short: "List a webhook's recent delivery attempts",
		Long:  "deliveries prints the newest attempts first: when, event, outcome, the receiver's status, the duration and the error code.\nThe server keeps attempts for 7 days.",
		Args:  exactArgs(1, "a webhook id"),
		RunE:  a.runWebhooksDeliveries,
	}
	cmd.Flags().IntVar(&a.webhooks.limit, "limit", defaultDeliveriesLimit, "the newest attempts to print, 1 to 100")
	return cmd
}

func (a *app) runWebhooksList(cmd *cobra.Command, _ []string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "webhooks", nil)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	list, err := decode[webhookList](data)
	if err != nil {
		return err
	}
	if len(list.Webhooks) == 0 {
		out.notice("No webhooks yet")
		return nil
	}
	rows := make([][]string, 0, len(list.Webhooks))
	for _, w := range list.Webhooks {
		rows = append(rows, []string{text(w.ID), text(w.Name), text(w.URL), strconv.Itoa(len(w.EventTypes)), webhookStatus(w)})
	}
	out.table([]string{"ID", "NAME", "URL", "EVENTS", "STATUS"}, rows)
	return nil
}

func (a *app) runWebhooksAdd(cmd *cobra.Command, _ []string) error {
	opts := a.webhooks
	if strings.TrimSpace(opts.name) == "" || strings.TrimSpace(opts.url) == "" || len(opts.events) == 0 {
		return usagef("pass --name, --url and at least one --event")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "webhooks", map[string]any{"name": opts.name, "url": opts.url, "event_types": opts.events})
	if err != nil {
		return err
	}
	return a.printWebhookWithSecret(data, "Created")
}

func (a *app) runWebhooksUpdate(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	body := map[string]any{}
	flags := cmd.Flags()
	if flags.Changed("name") {
		body["name"] = a.webhooks.name
	}
	if flags.Changed("url") {
		body["url"] = a.webhooks.url
	}
	if flags.Changed("event") {
		body["event_types"] = a.webhooks.events
	}
	if len(body) == 0 {
		return usagef("pass at least one of --name, --url or --event")
	}
	return a.patchWebhook(cmd, id, body, "Updated")
}

func (a *app) runWebhooksSwitch(cmd *cobra.Command, args []string, enable bool) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	verb := "Disabled"
	if enable {
		verb = "Enabled"
	}
	return a.patchWebhook(cmd, id, map[string]any{"enabled": enable}, verb)
}

func (a *app) patchWebhook(cmd *cobra.Command, id string, body map[string]any, verb string) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.patch(cmd.Context(), "webhooks/"+id, body)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	updated, err := decode[webhookView](data)
	if err != nil {
		return err
	}
	out.confirm("%s webhook %s", verb, text(updated.ID))
	printWebhook(out, updated)
	return nil
}

func (a *app) runWebhooksTest(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "webhooks/"+id+"/test", struct{}{})
	if err != nil {
		return err
	}
	tested, err := decode[webhookTest](data)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
	} else {
		d := tested.Delivery
		out.fields([][2]string{
			{"Outcome", text(d.Outcome)},
			{"Status", statusCell(d.StatusCode)},
			{"Duration", fmt.Sprintf("%d ms", d.DurationMs)},
			{"Error", orDash(d.ErrorCode)},
		})
	}
	if tested.Delivery.Outcome != "delivered" {
		return errors.New("the test event was not delivered")
	}
	return nil
}

func (a *app) runWebhooksRotate(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "webhooks/"+id+"/secret", struct{}{})
	if err != nil {
		return err
	}
	return a.printWebhookWithSecret(data, "Rotated the secret of")
}

func (a *app) runWebhooksDelete(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	if _, err := c.delete(cmd.Context(), "webhooks/"+id); err != nil {
		return err
	}
	if !a.json {
		a.output().confirm("Deleted webhook %s", text(id))
	}
	return nil
}

func (a *app) runWebhooksDeliveries(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	if a.webhooks.limit < 1 || a.webhooks.limit > maxDeliveriesLimit {
		return usagef("--limit must be 1 to %d", maxDeliveriesLimit)
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.get(cmd.Context(), "webhooks/"+id+"/deliveries", url.Values{"limit": {strconv.Itoa(a.webhooks.limit)}})
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	page, err := decode[webhookDeliveries](data)
	if err != nil {
		return err
	}
	if len(page.Deliveries) == 0 {
		out.notice("No deliveries yet")
		return nil
	}
	rows := make([][]string, 0, len(page.Deliveries))
	for _, d := range page.Deliveries {
		rows = append(rows, []string{when(d.AttemptedAt), text(d.EventType), text(d.Outcome), statusCell(d.StatusCode), fmt.Sprintf("%d ms", d.DurationMs), orDash(d.ErrorCode)})
	}
	out.table([]string{"WHEN", "EVENT", "OUTCOME", "STATUS", "DURATION", "ERROR"}, rows)
	return nil
}

func (a *app) printWebhookWithSecret(data []byte, verb string) error {
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	created, err := decode[webhookWithSecret](data)
	if err != nil {
		return err
	}
	out.confirm("%s webhook %s", verb, text(created.Webhook.ID))
	printWebhook(out, created.Webhook)
	out.fields([][2]string{{"Secret", text(created.Secret)}})
	out.notice("The secret is shown only now; copy it. Rotate it with `observe webhooks rotate-secret` to get a new one.")
	return nil
}

func printWebhook(out *output, w webhookView) {
	out.fields([][2]string{
		{"ID", text(w.ID)},
		{"Name", text(w.Name)},
		{"URL", text(w.URL)},
		{"Events", text(strings.Join(w.EventTypes, ", "))},
		{"Owner", text(w.Owner.Label)},
		{"Status", webhookStatus(w)},
	})
}

func webhookStatus(w webhookView) string {
	if w.Enabled {
		return "enabled"
	}
	return "disabled (" + orDash(w.DisabledReason) + ")"
}

func statusCell(status *int) string {
	if status == nil {
		return "-"
	}
	return strconv.Itoa(*status)
}
