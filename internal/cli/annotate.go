package cli

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type annotateOptions struct {
	dashboard string
	tags      string
	at        string
}

type annotateBody struct {
	Text        string   `json:"text"`
	Tags        []string `json:"tags"`
	DashboardID *string  `json:"dashboard_id,omitempty"`
	TimeFrom    *string  `json:"time_from,omitempty"`
}

type createdAnnotation struct {
	ID          string  `json:"id"`
	DashboardID *string `json:"dashboard_id"`
	TimeFrom    string  `json:"time_from"`
}

var relativeAt = regexp.MustCompile(`^now-([1-9][0-9]{0,5})([smhd])$`)

var atUnits = map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}

func (a *app) annotateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "annotate <text...>",
		Short: "Add an annotation, such as a deploy marker",
		Long:  "annotate adds an annotation at the server's current time on every dashboard, or on one dashboard with --dashboard.\nCharts draw it as a marker, and a CI job can post one per deploy with a service-account key.",
		Example: "  observe annotate Deploy checkout v1.4.2 --tags deploy,checkout\n" +
			"  observe annotate \"Database failover\" --dashboard <id> --at now-15m",
		Args: minimumArgs(1, "the annotation text"),
		RunE: a.runAnnotate,
	}
	cmd.Flags().StringVar(&a.annotating.dashboard, "dashboard", "", "the dashboard id; without it the annotation shows on every dashboard")
	cmd.Flags().StringVar(&a.annotating.tags, "tags", "", "comma-separated tags, such as deploy,checkout")
	cmd.Flags().StringVar(&a.annotating.at, "at", "", "when it happened: now, now-15m, now-2h, now-1d or an RFC 3339 time (default now)")
	return cmd
}

func (a *app) runAnnotate(cmd *cobra.Command, args []string) error {
	body := annotateBody{Text: strings.Join(args, " "), Tags: splitList(a.annotating.tags)}
	if a.annotating.dashboard != "" {
		id, err := parseID(a.annotating.dashboard)
		if err != nil {
			return err
		}
		body.DashboardID = &id
	}
	at, err := parseAt(a.annotating.at, time.Now())
	if err != nil {
		return err
	}
	body.TimeFrom = at
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "annotations", body)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	created, err := decode[createdAnnotation](data)
	if err != nil {
		return err
	}
	where := "on every dashboard"
	if created.DashboardID != nil {
		where = "on dashboard " + text(*created.DashboardID)
	}
	out.confirm("Added annotation %s at %s %s", text(created.ID), when(created.TimeFrom), where)
	return nil
}

func splitList(raw string) []string {
	items := []string{}
	for item := range strings.SplitSeq(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func parseAt(raw string, now time.Time) (*string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "now" {
		return nil, nil
	}
	if _, err := time.Parse(time.RFC3339, raw); err == nil {
		return &raw, nil
	}
	if match := relativeAt.FindStringSubmatch(raw); match != nil {
		count, _ := strconv.Atoi(match[1])
		at := now.Add(-time.Duration(count) * atUnits[match[2]]).UTC().Format(time.RFC3339)
		return &at, nil
	}
	return nil, usagef("--at %q is not a time; use now, now-<N> with s, m, h or d such as now-15m, or an RFC 3339 time such as 2026-11-01T10:00:00Z", text(raw))
}
