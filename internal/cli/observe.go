package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	pollInterval    = time.Second
	defaultTimeout  = 5 * time.Minute
	maxPollFailures = 3
)

type agentRequest struct {
	ID          string      `json:"id"`
	Source      string      `json:"source"`
	DashboardID *string     `json:"dashboard_id"`
	Status      string      `json:"status"`
	ResultVer   *int        `json:"result_version"`
	URL         *string     `json:"url"`
	Error       *string     `json:"error"`
	Steps       []agentStep `json:"steps"`
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
	noWait    bool
	timeout   time.Duration
}

type statusOptions struct {
	wait    bool
	timeout time.Duration
}

func (a *app) addPromptFlags(root *cobra.Command) {
	flags := root.Flags()
	flags.StringVar(&a.prompt.dashboard, "dashboard", "", "revise this dashboard with a follow-up prompt")
	flags.BoolVar(&a.prompt.noWait, "no-wait", false, "print the request id and exit without waiting")
	flags.DurationVar(&a.prompt.timeout, "timeout", defaultTimeout, "stop waiting after this long; the request keeps running on the server")
}

func (a *app) runPrompt(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(a.std.Err, "a prompt is required\n\n%s", cmd.UsageString())
		return usageError{}
	}
	prompt := strings.TrimSpace(strings.Join(args, " "))
	if prompt == "" {
		return usagef("the prompt is empty")
	}
	if a.prompt.timeout <= 0 {
		return usagef("--timeout must be positive")
	}
	payload := map[string]any{"prompt": prompt}
	if a.prompt.dashboard != "" {
		id, err := parseID(a.prompt.dashboard)
		if err != nil {
			return err
		}
		payload["dashboard_id"] = id
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	return a.submit(cmd.Context(), c, payload, a.prompt.noWait, a.prompt.timeout)
}

func (a *app) submit(ctx context.Context, c *client, payload map[string]any, noWait bool, timeout time.Duration) error {
	data, err := c.post(ctx, "observe", payload)
	if err != nil {
		return err
	}
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

func printRequest(out *output, req agentRequest) {
	version := "-"
	if req.ResultVer != nil {
		version = strconv.Itoa(*req.ResultVer)
	}
	out.fields([][2]string{
		{"ID", text(req.ID)},
		{"Status", text(req.Status)},
		{"Source", text(req.Source)},
		{"Dashboard", orDash(req.DashboardID)},
		{"Version", version},
		{"URL", orDash(req.URL)},
		{"Error", orDash(req.Error)},
	})
	if len(req.Steps) > 0 {
		out.line("")
		for _, step := range req.Steps {
			out.line("  %s  %s", text(step.Phase), text(step.Message))
		}
	}
}

func (a *app) waitAndReport(ctx context.Context, c *client, id string, timeout time.Duration) error {
	out := a.output()
	data, req, err := poll(ctx, c, out, id, timeout, pollInterval)
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
	if a.json {
		return nil
	}
	address, err := dashboardAddress(c, req)
	if err != nil {
		return err
	}
	out.line("%s", address)
	return nil
}

func dashboardAddress(c *client, req agentRequest) (string, error) {
	if req.URL != nil && *req.URL != "" {
		return text(*req.URL), nil
	}
	if req.DashboardID != nil && *req.DashboardID != "" {
		return c.base.JoinPath("dashboards", *req.DashboardID).String(), nil
	}
	return "", errors.New("the agent finished without a dashboard address")
}

func stillRunning(id string) error {
	return fmt.Errorf("still running on the server: observe status %s", id)
}

func poll(ctx context.Context, c *client, out *output, id string, timeout, interval time.Duration) ([]byte, agentRequest, error) {
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var progress stepProgress
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
			progress.print(out, envelope.Request.Steps)
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
