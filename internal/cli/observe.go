package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/links"
)

const (
	pollInterval    = time.Second
	defaultTimeout  = 2 * time.Hour
	maxPollFailures = 3
)

type agentRequest struct {
	ID            string        `json:"id"`
	Kind          string        `json:"kind"`
	Intent        string        `json:"intent"`
	Investigation bool          `json:"investigation"`
	Trigger       string        `json:"trigger"`
	Title         string        `json:"title"`
	Prompt        string        `json:"prompt"`
	Redacted      bool          `json:"redacted"`
	DashboardID   *string       `json:"dashboard_id"`
	SubjectID     *string       `json:"subject_observation_id"`
	Status        string        `json:"status"`
	State         string        `json:"state"`
	ResultVer     *int          `json:"result_version"`
	URL           *string       `json:"url"`
	Error         *string       `json:"error"`
	Steps         []agentStep   `json:"steps"`
	LiveText      *string       `json:"live_text"`
	Usage         *requestUsage `json:"usage"`

	QueuePosition *int            `json:"queue_position"`
	FailureCode   *string         `json:"failure_code"`
	Build         *buildSummary   `json:"build"`
	BuildSavedAt  *string         `json:"build_saved_at"`
	ContinuedBy   *string         `json:"continued_by"`
	RetriedBy     *string         `json:"retried_by"`
	Finding       *findingSummary `json:"finding"`
	ObservationID *string         `json:"observation_id"`
	SharedThread  *string         `json:"shared_thread_id"`
	Anchor        *anchor.Anchor  `json:"anchor"`
	BlockID       *string         `json:"block_id"`

	AllowedActions []string `json:"allowed_actions"`
}

type requestUsage struct {
	CostUSD *float64 `json:"cost_usd"`
}

type buildSummary struct {
	Saves         int `json:"saves"`
	Panels        int `json:"panels"`
	PlannedPanels int `json:"planned_panels"`
}

type agentStep struct {
	At      string `json:"at"`
	Attempt int    `json:"attempt"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

type requestEnvelope struct {
	Request agentRequest `json:"request"`
}

type promptOptions struct {
	dashboard string
	revise    bool
	newChat   bool
	apply     string
	chat      string
	at        string
	noWait    bool
	timeout   time.Duration
}

type statusOptions struct {
	wait    bool
	timeout time.Duration
}

func (a *app) addPromptFlags(root *cobra.Command) {
	flags := root.Flags()
	flags.StringVar(&a.prompt.dashboard, "dashboard", "", "ask about this dashboard; with --revise, change it")
	flags.BoolVar(&a.prompt.revise, "revise", false, "change --dashboard with this prompt instead of only answering about it")
	flags.StringVar(&a.prompt.apply, "apply", "", "with --revise and no prompt: change --dashboard as this answer's suggested change says, by its finding id")
	flags.StringVar(&a.prompt.chat, "chat", "", "ask the next turn of this chat of yours, from observe chats list")
	flags.BoolVar(&a.prompt.newChat, "new-chat", false, "start a new chat instead of joining your latest one on --dashboard")
	flags.StringVar(&a.prompt.at, "at", "", "the spot on --dashboard the prompt is about, as the anchor JSON object or a short form such as pos:0.4,0.3")
	flags.BoolVar(&a.prompt.noWait, "no-wait", false, "print the request id and exit without waiting")
	flags.DurationVar(&a.prompt.timeout, "timeout", defaultTimeout, "stop waiting after this long; the request keeps running on the server")
}

func (a *app) runPrompt(cmd *cobra.Command, args []string) error {
	if a.prompt.apply != "" {
		return a.runApplyAnswer(cmd, args)
	}
	if len(args) == 0 {
		_, _ = fmt.Fprintf(a.std.Err, "a prompt is required\n\n%s", cmd.UsageString())
		return usageError{}
	}
	if a.prompt.dashboard == "" {
		if err := probableTypo(cmd, args); err != nil {
			return err
		}
	}
	prompt := strings.TrimSpace(strings.Join(args, " "))
	if prompt == "" {
		return usagef("the prompt is empty")
	}
	if a.prompt.timeout <= 0 {
		return usagef("--timeout must be positive")
	}
	payload, err := a.prompt.payload(prompt)
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	return a.submit(cmd.Context(), c, payload, a.prompt.noWait, a.prompt.timeout)
}

func (a *app) runApplyAnswer(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usagef("--apply takes no prompt: the answer's suggested change is the prompt")
	}
	if !a.prompt.revise {
		return usagef("--apply needs --revise")
	}
	if a.prompt.timeout <= 0 {
		return usagef("--timeout must be positive")
	}
	finding, err := parseID(a.prompt.apply)
	if err != nil {
		return err
	}
	payload, err := a.prompt.payload("")
	if err != nil {
		return err
	}
	delete(payload, "prompt")
	payload["applies_finding_id"] = finding
	c, err := a.newClient()
	if err != nil {
		return err
	}
	return a.submit(cmd.Context(), c, payload, a.prompt.noWait, a.prompt.timeout)
}

func (o promptOptions) conflict() error {
	switch {
	case o.revise && o.dashboard == "":
		return usagef("--revise needs --dashboard")
	case o.newChat && o.chat != "":
		return usagef("--new-chat excludes --chat")
	}
	return nil
}

func (o promptOptions) payload(prompt string) (map[string]any, error) {
	if err := o.conflict(); err != nil {
		return nil, err
	}
	payload := map[string]any{"prompt": prompt}
	if o.revise {
		payload["kind"] = kindRevise
	}
	if o.newChat {
		payload["new_chat"] = true
	}
	for field, raw := range map[string]string{"dashboard_id": o.dashboard, "chat_id": o.chat} {
		if raw == "" {
			continue
		}
		id, err := parseID(raw)
		if err != nil {
			return nil, err
		}
		payload[field] = id
	}
	if err := addAnchor(payload, o.at); err != nil {
		return nil, err
	}
	return payload, nil
}

func addAnchor(payload map[string]any, at string) error {
	if at == "" {
		return nil
	}
	spot, err := anchor.Parse(at)
	if err != nil {
		return usagef("--at: %v", err)
	}
	payload["anchor"] = spot
	return nil
}

func (a *app) submit(ctx context.Context, c *client, payload map[string]any, noWait bool, timeout time.Duration) error {
	data, err := c.post(ctx, "observe", payload)
	if err != nil {
		return err
	}
	return a.reportQueued(ctx, c, data, noWait, timeout)
}

func (a *app) reportQueued(ctx context.Context, c *client, data []byte, noWait bool, timeout time.Duration) error {
	queued, err := decode[requestEnvelope](data)
	if err != nil {
		return err
	}
	out := a.output()
	if noWait {
		if a.json {
			out.body(data)
		} else {
			out.line("%s", text(queued.Request.ID))
		}
		return nil
	}
	return a.waitAndReport(ctx, c, queued.Request.ID, timeout)
}

func (a *app) statusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <request-id>",
		Short: "Show one agent request",
		Args:  exactArgs(1, "one request id"),
		RunE:  a.runStatus,
	}
	cmd.Flags().BoolVar(&a.status.wait, "wait", false, "wait until the request finishes, like the prompt command")
	cmd.Flags().DurationVar(&a.status.timeout, "timeout", defaultTimeout, "with --wait, stop waiting after this long")
	a.addOpenFlag(cmd)
	return cmd
}

func (a *app) runStatus(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	if a.status.timeout <= 0 {
		return usagef("--timeout must be positive")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if a.status.wait {
		return a.waitAndReport(ctx, c, id, a.status.timeout)
	}
	data, err := c.get(ctx, "observe/"+id, nil)
	if err != nil {
		return err
	}
	envelope, err := decode[requestEnvelope](data)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	printRequest(out, envelope.Request)
	return nil
}

func costText(usage *requestUsage) string {
	if usage == nil || usage.CostUSD == nil {
		return "-"
	}
	switch cost := *usage.CostUSD; {
	case cost == 0:
		return "$0.00"
	case cost < 0.01:
		return "under $0.01"
	default:
		return fmt.Sprintf("about $%.2f", cost)
	}
}

const privateChatWords = "hidden, asked in a private chat"

func wordsOf(req agentRequest, words string) string {
	if req.Redacted {
		return privateChatWords
	}
	return dash(words)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func printRequest(out *output, req agentRequest) {
	version := "-"
	if req.ResultVer != nil {
		version = strconv.Itoa(*req.ResultVer)
	}
	fields := [][2]string{
		{"ID", text(req.ID)},
		{"Status", text(req.Status)},
		{"State", text(req.State)},
		{"Intent", text(req.Intent)},
		{"Investigation", yesNo(req.Investigation)},
		{"Trigger", text(req.Trigger)},
		{"Cost", costText(req.Usage)},
		{"Dashboard", orDash(req.DashboardID)},
		{"Version", version},
		{"URL", orDash(req.URL)},
		{"Error", orDash(req.Error)},
		{"Title", wordsOf(req, req.Title)},
		{"Prompt", wordsOf(req, req.Prompt)},
	}
	if req.SubjectID != nil {
		fields = append(fields, [2]string{"About observation", text(*req.SubjectID)})
	}
	if req.Finding != nil {
		fields = append(fields, [2]string{"Finding", text(req.Finding.Title)}, [2]string{"Finding URL", orDash(&req.Finding.URL)})
	}
	fields = append(fields, pointedFields(req)...)
	if req.SharedThread != nil {
		fields = append(fields, [2]string{"Shared in thread", text(*req.SharedThread)})
	}
	out.fields(fields)
	switch {
	case req.Status == "queued" && req.QueuePosition != nil:
		out.line("Queued, position %d", *req.QueuePosition)
	case req.Status == "failed" && req.ContinuedBy != nil:
		out.line("Continued by %s", text(*req.ContinuedBy))
	case req.RetriedBy != nil:
		out.line("Retried by %s", text(*req.RetriedBy))
	case slices.Contains(req.AllowedActions, "continue"):
		out.line("Save it in the browser, or continue it: observe continue %s", text(req.ID))
	case slices.Contains(req.AllowedActions, "retry") && req.State != "succeeded":
		out.line("Run it again: observe retry %s", text(req.ID))
	}
	if len(req.Steps) > 0 {
		out.line("")
		for _, step := range req.Steps {
			out.line("  %s  %s", text(step.Phase), text(step.Message))
		}
	}
	printLiveText(out, req.LiveText)
}

func pointedFields(req agentRequest) [][2]string {
	if req.Anchor == nil {
		return nil
	}
	fields := [][2]string{{"Points at", anchorText(req.Anchor)}}
	if req.BlockID != nil {
		fields = append(fields, [2]string{"On block", text(*req.BlockID)})
	}
	return fields
}

func printLiveText(out *output, live *string) {
	if live == nil || *live == "" {
		return
	}
	out.line("")
	out.line("Live text")
	for _, line := range strings.Split(*live, "\n") {
		out.line("  %s", text(line))
	}
}

func (a *app) waitAndReport(ctx context.Context, c *client, id string, timeout time.Duration) error {
	out := a.output()
	progress := newRequestProgress(c, out, id)
	data, req, err := poll(ctx, c, progress, id, timeout, pollInterval)
	if err != nil {
		return err
	}
	if a.json {
		out.body(data)
	}
	if req.Status != "succeeded" {
		reason := "the agent gave no reason"
		if req.Error != nil && *req.Error != "" {
			reason = text(*req.Error)
		} else if req.Status != "failed" {
			reason = fmt.Sprintf("the request ended with status %q", text(req.Status))
		}
		return fmt.Errorf("observe failed: %s", reason)
	}
	if req.Kind == kindInvestigate || req.Finding != nil {
		return a.reportFinding(out, req)
	}
	address, err := dashboardAddress(c, req)
	if a.json {
		if err == nil {
			a.openLink(out, address)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if req.DashboardID != nil {
		if title, ok := progress.finish(ctx, *req.DashboardID, req.ResultVer); ok {
			address = titled(address, *req.DashboardID, title)
		}
	}
	reportSaved(out, req.ResultVer, address)
	a.openLink(out, address)
	return nil
}

func (a *app) reportFinding(out *output, req agentRequest) error {
	if req.Finding == nil {
		return fmt.Errorf("the investigation finished without a finding: observe status %s", text(req.ID))
	}
	if !a.json {
		printFinding(out, *req.Finding, req.ID)
		printAskOn(out, req.Finding.AskOn, req.Prompt)
	}
	if req.Finding.saved() {
		a.openLink(out, text(req.Finding.URL))
	}
	return nil
}

func dashboardAddress(c *client, req agentRequest) (string, error) {
	if req.URL != nil && *req.URL != "" {
		return text(*req.URL), nil
	}
	if req.DashboardID != nil && *req.DashboardID != "" {
		return links.Dashboard(c.base, *req.DashboardID, ""), nil
	}
	return "", errors.New("the agent finished without a dashboard address")
}

func stillRunning(id string) error {
	return fmt.Errorf("still running on the server: observe status %s", id)
}

func poll(ctx context.Context, c *client, progress *requestProgress, id string, timeout, interval time.Duration) ([]byte, agentRequest, error) {
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer progress.stop()
	failures := 0
	for {
		data, err := c.get(pollCtx, "observe/"+id, nil)
		if pollCtx.Err() != nil {
			return nil, agentRequest{}, stopped(ctx, id, timeout)
		}
		if err != nil {
			failures++
			if !isTransient(err) || failures >= maxPollFailures {
				return nil, agentRequest{}, fmt.Errorf("%w\nthe request may still be running: observe status %s", err, id)
			}
		} else {
			failures = 0
			envelope, err := decode[requestEnvelope](data)
			if err != nil {
				return nil, agentRequest{}, err
			}
			progress.update(pollCtx, envelope.Request)
			if envelope.Request.Status != "queued" && envelope.Request.Status != "running" {
				return data, envelope.Request, nil
			}
		}
		select {
		case <-pollCtx.Done():
			return nil, agentRequest{}, stopped(ctx, id, timeout)
		case <-ticker.C:
		}
	}
}

func stopped(parent context.Context, id string, timeout time.Duration) error {
	if parent.Err() != nil {
		return stillRunning(id)
	}
	return fmt.Errorf("gave up after %s; the request is still running on the server: observe status %s", timeout, id)
}

type stepProgress struct {
	printed int
	attempt int
	last    string
}

func (p *stepProgress) print(out *output, steps []agentStep) {
	if len(steps) == 0 {
		return
	}
	latest := steps[len(steps)-1].Attempt
	if p.printed > 0 && (latest != p.attempt || len(steps) < p.printed || steps[p.printed-1].At != p.last) {
		p.printed = 0
		for p.printed < len(steps) && steps[p.printed].Attempt != latest {
			p.printed++
		}
	}
	p.attempt = latest
	for _, step := range steps[p.printed:] {
		out.notice("  %s  %s", text(step.Phase), text(step.Message))
	}
	p.printed = len(steps)
	p.last = steps[len(steps)-1].At
}
