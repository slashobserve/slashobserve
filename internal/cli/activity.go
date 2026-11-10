package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultActivityLimit = 20
	maxActivityLimit     = 100
	defaultStreamRetry   = 3 * time.Second
	maxFrameBytes        = 1 << 20
	activityTimeLayout   = "2006-01-02T15:04:05Z"
)

type activityOptions struct {
	dashboard string
	folder    string
	types     []string
	since     string
	limit     int
	follow    bool
}

type activityActor struct {
	Label string `json:"label"`
}

type activityData struct {
	Version       int      `json:"version"`
	RestoredFrom  int      `json:"restored_from"`
	Title         string   `json:"title"`
	Tags          []string `json:"tags"`
	ResultVersion int      `json:"result_version"`
	FailureCode   string   `json:"failure_code"`
}

type activityEvent struct {
	Seq       int64         `json:"seq"`
	Type      string        `json:"type"`
	Actor     activityActor `json:"actor"`
	Data      activityData  `json:"data"`
	CreatedAt string        `json:"created_at"`
}

type activityEntry struct {
	raw   []byte
	event activityEvent
}

type activityPage struct {
	Activity []json.RawMessage `json:"activity"`
}

type sseFrame struct {
	id    string
	event string
	data  string
	retry time.Duration
}

func (a *app) activityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activity",
		Short: "List and follow what changed in the workspace",
		Long:  "activity prints the newest events of the activity log, oldest first, one line each: when, who and what.\nIt covers dashboard saves, moves and deletes, annotations, findings and agent runs in the folders the key can see.\nWith --follow it then keeps printing new events until Ctrl-C, reconnecting from the last event it printed.",
		Example: "  observe activity\n" +
			"  observe activity --dashboard <id> --since 24h\n" +
			"  observe activity --follow --type run.failed --type run.canceled",
		Args: noArgs,
		RunE: a.runActivity,
	}
	flags := cmd.Flags()
	flags.StringVar(&a.activity.dashboard, "dashboard", "", "only events of this dashboard id")
	flags.StringVar(&a.activity.folder, "folder", "", "only events of this folder id")
	flags.StringSliceVar(&a.activity.types, "type", nil, "only events of this type, such as dashboard.moved; repeat for several")
	flags.StringVar(&a.activity.since, "since", "", "only events since a duration ago such as 24h, or since an RFC 3339 time")
	flags.IntVar(&a.activity.limit, "limit", defaultActivityLimit, "the newest events to print, 1 to 100")
	flags.BoolVar(&a.activity.follow, "follow", false, "keep printing new events until Ctrl-C")
	return cmd
}

func (o activityOptions) filters(now time.Time) (url.Values, error) {
	if o.limit < 1 || o.limit > maxActivityLimit {
		return nil, usagef("--limit must be 1 to %d", maxActivityLimit)
	}
	query := url.Values{}
	if o.dashboard != "" {
		id, err := parseID(o.dashboard)
		if err != nil {
			return nil, err
		}
		query.Set("dashboard_id", id)
	}
	if o.folder != "" {
		id, err := parseID(o.folder)
		if err != nil {
			return nil, err
		}
		query.Set("folder_id", id)
	}
	for _, kind := range o.types {
		query.Add("type", kind)
	}
	since, err := parseSince(o.since, now)
	if err != nil {
		return nil, err
	}
	if since != "" {
		query.Set("since", since)
	}
	return query, nil
}

func parseSince(raw string, now time.Time) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if _, err := time.Parse(time.RFC3339, raw); err == nil {
		return raw, nil
	}
	if span, err := time.ParseDuration(raw); err == nil && span > 0 {
		return now.Add(-span).UTC().Format(time.RFC3339), nil
	}
	return "", usagef("--since %q is not a duration such as 24h or an RFC 3339 time such as 2026-11-01T10:00:00Z", text(raw))
}

func (a *app) runActivity(cmd *cobra.Command, _ []string) error {
	opts := a.activity
	filters, err := opts.filters(time.Now())
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	page := maps.Clone(filters)
	page.Set("limit", strconv.Itoa(opts.limit))
	data, err := c.get(ctx, "activity", page)
	if err != nil {
		return err
	}
	out := a.output()
	entries, err := readActivityPage(data)
	if err != nil {
		return err
	}
	switch {
	case a.json && !opts.follow:
		out.body(data)
		return nil
	case len(entries) == 0 && !opts.follow:
		out.notice("No activity yet")
		return nil
	}
	for _, entry := range entries {
		printActivity(out, a.json, entry)
	}
	if !opts.follow {
		return nil
	}
	follower := &activityFollower{c: c, query: filters, out: out, json: a.json, retry: defaultStreamRetry}
	if len(entries) > 0 {
		follower.last = strconv.FormatInt(entries[len(entries)-1].event.Seq, 10)
	}
	return follower.run(ctx)
}

func readActivityPage(data []byte) ([]activityEntry, error) {
	page, err := decode[activityPage](data)
	if err != nil {
		return nil, err
	}
	entries := make([]activityEntry, 0, len(page.Activity))
	for _, raw := range slices.Backward(page.Activity) {
		event, err := decode[activityEvent](raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, activityEntry{raw: raw, event: event})
	}
	return entries, nil
}

func printActivity(out *output, asJSON bool, entry activityEntry) {
	if asJSON {
		out.body(entry.raw)
		return
	}
	out.line("%s", activityLine(entry.event))
}

func activityLine(e activityEvent) string {
	return strings.Join([]string{activityTime(e.CreatedAt), text(e.Actor.Label), describe(e)}, "  ")
}

func activityTime(raw string) string {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return text(raw)
	}
	return t.UTC().Format(activityTimeLayout)
}

func quotedTitle(title string) string {
	if title == "" {
		return "a dashboard"
	}
	return `"` + text(title) + `"`
}

func describe(e activityEvent) string {
	d := e.Data
	switch e.Type {
	case "annotation.created":
		if len(d.Tags) > 0 {
			return "added an annotation (" + text(strings.Join(d.Tags, ", ")) + ")"
		}
		return "added an annotation"
	case "annotation.deleted":
		return "deleted an annotation"
	case "finding.created":
		return "saved the finding " + quotedTitle(d.Title)
	case "finding.pinned":
		return "pinned a finding"
	}
	if phrase, ok := describeDashboard(e); ok {
		return phrase
	}
	if phrase, ok := describeRun(e); ok {
		return phrase
	}
	return text(e.Type)
}

func describeDashboard(e activityEvent) (string, bool) {
	d := e.Data
	switch e.Type {
	case "dashboard.created":
		return "created " + quotedTitle(d.Title), true
	case "dashboard.version_created":
		return fmt.Sprintf("saved version %d of %s", d.Version, quotedTitle(d.Title)), true
	case "dashboard.restored":
		return fmt.Sprintf("restored version %d of %s as version %d", d.RestoredFrom, quotedTitle(d.Title), d.Version), true
	case "dashboard.moved":
		return "moved " + quotedTitle(d.Title), true
	case "dashboard.deleted":
		return "deleted " + quotedTitle(d.Title), true
	}
	return "", false
}

func describeRun(e activityEvent) (string, bool) {
	d := e.Data
	switch e.Type {
	case "run.queued":
		return "run queued", true
	case "run.started":
		return "run started", true
	case "run.succeeded":
		if d.ResultVersion > 0 {
			return fmt.Sprintf("run succeeded (saved version %d)", d.ResultVersion), true
		}
		return "run succeeded", true
	case "run.failed":
		if d.FailureCode != "" {
			return "run failed (" + text(d.FailureCode) + ")", true
		}
		return "run failed", true
	case "run.canceled":
		return "run canceled", true
	}
	return "", false
}

type activityFollower struct {
	c     *client
	query url.Values
	out   *output
	json  bool
	last  string
	retry time.Duration
}

func (f *activityFollower) run(ctx context.Context) error {
	failures := 0
	for {
		received, err := f.once(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if received {
			failures = 0
		}
		if err == nil && !received {
			err = transportFailure{errors.New("the server closed the activity stream without sending anything")}
		}
		if err != nil {
			failures++
			if (!isTransient(err) && !isTooManyStreams(err)) || failures >= maxPollFailures {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(f.retry):
		}
	}
}

func isTooManyStreams(err error) bool {
	var api *apiError
	return errors.As(err, &api) && api.status == http.StatusTooManyRequests
}

func (f *activityFollower) once(ctx context.Context) (bool, error) {
	body, err := f.c.stream(ctx, "activity/stream", f.query, f.last)
	if err != nil {
		return false, err
	}
	defer func() { _ = body.Close() }()
	received := false
	var handled error
	err = readFrames(body, func(fr sseFrame) error {
		received = true
		handled = f.handle(fr)
		return handled
	})
	switch {
	case handled != nil:
		return received, handled
	case err != nil:
		return received, f.c.transportError(ctx, err)
	}
	return received, nil
}

func (f *activityFollower) handle(fr sseFrame) error {
	if fr.retry > 0 {
		f.retry = fr.retry
	}
	switch fr.event {
	case "activity":
		event, err := decode[activityEvent]([]byte(fr.data))
		if err != nil {
			return err
		}
		printActivity(f.out, f.json, activityEntry{raw: []byte(fr.data), event: event})
		f.last = strconv.FormatInt(event.Seq, 10)
	case "reset":
		f.out.warn("Some activity may have been missed.")
	}
	return nil
}

func readFrames(r io.Reader, emit func(sseFrame) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), maxFrameBytes)
	var current sseFrame
	var data []string
	seen := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if !seen {
				continue
			}
			current.data = strings.Join(data, "\n")
			err := emit(current)
			current, data, seen = sseFrame{}, nil, false
			if err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			current.id = value
		case "event":
			current.event = value
		case "data":
			data = append(data, value)
		case "retry":
			if millis, err := strconv.Atoi(value); err == nil && millis > 0 {
				current.retry = time.Duration(millis) * time.Millisecond
			}
		default:
			continue
		}
		seen = true
	}
	return scanner.Err()
}
