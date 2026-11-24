package cli

import (
	"bufio"
	"cmp"
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
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultActivityLimit = 20
	maxActivityLimit     = 100
	defaultStreamRetry   = 3 * time.Second
	streamIdleTimeout    = 45 * time.Second
	maxFrameBytes        = 1 << 20
	activityTimeLayout   = "2006-01-02T15:04:05Z"
)

type activityOptions struct {
	dashboard   string
	folder      string
	observation string
	types       []string
	since       string
	until       string
	actor       string
	limit       int
	cursor      string
	all         bool
	follow      bool
}

type activityActor struct {
	Label string `json:"label"`
}

type activityData struct {
	raw           json.RawMessage
	Version       int      `json:"version"`
	RestoredFrom  int      `json:"restored_from"`
	Title         string   `json:"title"`
	Kind          string   `json:"kind"`
	BlockKind     string   `json:"block_kind"`
	State         string   `json:"state"`
	Tags          []string `json:"tags"`
	Changed       []string `json:"changed"`
	ResultVersion int      `json:"result_version"`
	Revision      int      `json:"revision"`
	FailureCode   string   `json:"failure_code"`
	Object        struct {
		Kind string `json:"kind"`
	} `json:"object"`
	ResolvedByVersion int `json:"resolved_by_version"`
	Mentioned         struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"mentioned"`
}

func (d *activityData) UnmarshalJSON(b []byte) error {
	type plain activityData
	if err := json.Unmarshal(b, (*plain)(d)); err != nil {
		return err
	}
	d.raw = slices.Clone(b)
	return nil
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
		Long:  "activity prints the newest events of the activity log, oldest first, one line each: when, who and what.\nIt covers dashboard saves, moves and deletes, annotations, findings, public links, access requests, comment threads and agent runs in the folders the key can see, and for an Admin key the admin changes too: roles, people invites, deactivation and removal, keys, webhooks, connected agents, outbound domains, Grafana connections and datasources.\nWith --follow it then keeps printing new events until Ctrl-C, reconnecting from the last event it printed.",
		Example: "  observe activity\n" +
			"  observe activity --dashboard <id> --since 24h\n" +
			"  observe activity --follow --type run.failed --type run.canceled",
		Args: noArgs,
		RunE: a.runActivity,
	}
	flags := cmd.Flags()
	flags.StringVar(&a.activity.dashboard, "dashboard", "", "only events of this dashboard id")
	flags.StringVar(&a.activity.folder, "folder", "", "only events of this folder id")
	flags.StringVar(&a.activity.observation, "observation", "", "only events of this observation id")
	flags.StringSliceVar(&a.activity.types, "type", nil, "only events of this type, such as dashboard.moved; repeat for several")
	flags.StringVar(&a.activity.since, "since", "", "only events since a duration ago such as 24h, or since an RFC 3339 time")
	flags.StringVar(&a.activity.until, "until", "", "only events until a duration ago such as 1h, or until an RFC 3339 time")
	flags.StringVar(&a.activity.actor, "actor", "", "only events made by this person or service account id")
	flags.IntVar(&a.activity.limit, "limit", defaultActivityLimit, "the newest events to print, 1 to 100")
	flags.StringVar(&a.activity.cursor, "cursor", "", "continue from the next_cursor a page ended with")
	flags.BoolVar(&a.activity.all, "all", false, "read every page and print them as one list")
	flags.BoolVar(&a.activity.follow, "follow", false, "keep printing new events until Ctrl-C")
	return cmd
}

func (o activityOptions) filters(now time.Time) (url.Values, error) {
	if err := (pageOptions{limit: o.limit, maxLimit: maxActivityLimit, cursor: o.cursor, all: o.all}).check(); err != nil {
		return nil, err
	}
	if o.follow && (o.all || o.cursor != "") {
		return nil, usagef("--follow prints the newest events and then new ones, so it cannot be combined with --all or --cursor")
	}
	query := url.Values{}
	for _, flag := range []struct{ key, value string }{{"dashboard_id", o.dashboard}, {"folder_id", o.folder}, {"observation_id", o.observation}, {"actor_id", o.actor}} {
		if flag.value == "" {
			continue
		}
		id, err := parseID(flag.value)
		if err != nil {
			return nil, err
		}
		query.Set(flag.key, id)
	}
	for _, kind := range o.types {
		query.Add("type", kind)
	}
	for _, flag := range []struct{ name, key, value string }{{"--since", "since", o.since}, {"--until", "until", o.until}} {
		at, err := parseTimeFlag(flag.name, flag.value, now)
		if err != nil {
			return nil, err
		}
		if at != "" {
			query.Set(flag.key, at)
		}
	}
	return query, nil
}

func parseTimeFlag(flag, raw string, now time.Time) (string, error) {
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
	return "", usagef("%s %q is not a duration such as 24h or an RFC 3339 time such as 2026-11-01T10:00:00Z", flag, text(raw))
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
	for key, values := range (pageOptions{limit: opts.limit, cursor: opts.cursor}).query() {
		page[key] = values
	}
	data, err := c.getPages(ctx, "activity", page, opts.all)
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
		next, err := nextCursor(data)
		if err != nil {
			return err
		}
		pageFooter(out, len(entries), nil, &next)
		return nil
	}
	follower := newActivityFollower(c, filters, out, a.json)
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

func annotationPhrase(d activityData) string {
	phrase := "an annotation"
	if d.Title != "" {
		phrase += ` "` + text(d.Title) + `"`
	}
	if len(d.Tags) > 0 {
		phrase += " (" + text(strings.Join(d.Tags, ", ")) + ")"
	}
	return phrase
}

func describeAnnotation(e activityEvent) (string, bool) {
	d := e.Data
	switch e.Type {
	case "annotation.created":
		return "added " + annotationPhrase(d), true
	case "annotation.updated":
		if len(d.Tags) > 0 {
			return "edited an annotation (" + text(strings.Join(d.Tags, ", ")) + ")", true
		}
		return "edited an annotation", true
	case "annotation.deleted":
		return "deleted " + annotationPhrase(d), true
	case "finding.created":
		return "saved the finding " + quotedTitle(d.Title), true
	case "finding.pinned":
		return "pinned a finding", true
	case "finding.moved":
		return "moved the finding " + quotedTitle(d.Title), true
	}
	return "", false
}

func describe(e activityEvent) string {
	if phrase, ok := describeAnnotation(e); ok {
		return phrase
	}
	if phrase, ok := describeSavedSearch(e); ok {
		return phrase
	}
	if phrase, ok := describeDashboard(e); ok {
		return phrase
	}
	if phrase, ok := describeObservation(e); ok {
		return phrase
	}
	if phrase, ok := describeRun(e); ok {
		return phrase
	}
	if phrase, ok := describeThread(e); ok {
		return phrase
	}
	if phrase, ok := describeProposal(e); ok {
		return phrase
	}
	if phrase, ok := describeAdmin(e); ok {
		return phrase
	}
	return text(e.Type)
}

func quotedObservation(title string) string {
	if title == "" {
		return "an observation"
	}
	return "the observation " + `"` + text(title) + `"`
}

func describeObservation(e activityEvent) (string, bool) {
	d := e.Data
	target := quotedObservation(d.Title)
	block := text(d.BlockKind) + " block"
	switch e.Type {
	case "observation.created":
		return "started " + target, true
	case "observation.updated":
		return "edited " + target, true
	case "observation.moved":
		return "moved " + target, true
	case "observation.resolved":
		return "resolved " + target, true
	case "observation.reopened":
		return "reopened " + target, true
	case "observation.published":
		return fmt.Sprintf("published revision %d of %s", d.Revision, target), true
	case "observation.block_added":
		if d.State == "requested" {
			return "requested a " + block + " on " + target, true
		}
		return "added a " + block + " to " + target, true
	case "observation.block_updated":
		return "changed a " + block + " on " + target, true
	case "observation.block_kept":
		return "kept a " + block + " on " + target, true
	case "observation.block_discarded":
		return "discarded a " + block + " on " + target, true
	case "observation.block_removed":
		return "removed a " + block + " from " + target, true
	}
	return "", false
}

func quotedProposal(title string) string {
	if title == "" {
		return "a proposal"
	}
	return "the proposal " + `"` + text(title) + `"`
}

func describeProposal(e activityEvent) (string, bool) {
	d := e.Data
	switch e.Type {
	case "proposal.opened":
		return "opened " + quotedProposal(d.Title), true
	case "proposal.updated":
		return fmt.Sprintf("updated %s (revision %d)", quotedProposal(d.Title), d.Revision), true
	case "proposal.merged":
		if d.Version > 0 {
			return fmt.Sprintf("merged %s as version %d", quotedProposal(d.Title), d.Version), true
		}
		return "merged " + quotedProposal(d.Title) + ", already in the dashboard", true
	case "proposal.discarded":
		return "discarded " + quotedProposal(d.Title), true
	}
	return "", false
}

func describeSavedSearch(e activityEvent) (string, bool) {
	d := e.Data
	title := `"` + text(d.Title) + `"`
	switch e.Type {
	case "saved_search.created":
		switch d.Kind {
		case "traces":
			return "saved the trace search " + title, true
		case "metrics":
			return "saved the metrics search " + title, true
		}
		return "saved the logs search " + title, true
	case "saved_search.updated":
		if what := searchParts(d.Changed); what != "" {
			return "changed the " + what + " of the saved search " + title, true
		}
		return "renamed the saved search to " + title, true
	case "saved_search.moved":
		return "moved the saved search " + title, true
	case "saved_search.deleted":
		return "deleted the saved search " + title, true
	}
	return "", false
}

func searchParts(changed []string) string {
	var parts []string
	for _, part := range changed {
		if part != "title" {
			parts = append(parts, text(part))
		}
	}
	return strings.Join(parts, ", ")
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
	case "run.canceling":
		return "run canceling", true
	case "run.released":
		return "run released back to the queue", true
	case "run.kept":
		if d.ResultVersion > 0 {
			return fmt.Sprintf("kept the build of a stopped run as version %d", d.ResultVersion), true
		}
		return "kept the build of a stopped run", true
	}
	return "", false
}

func describeThread(e activityEvent) (string, bool) {
	d := e.Data
	switch e.Type {
	case "thread.opened":
		return "commented on " + objectPhrase(d.Object.Kind), true
	case "thread.replied":
		return "replied to a thread", true
	case "thread.edited":
		return "edited a comment", true
	case "thread.deleted":
		return "deleted a comment", true
	case "thread.resolved":
		if d.ResolvedByVersion > 0 {
			return fmt.Sprintf("resolved a thread (fixed by version %d)", d.ResolvedByVersion), true
		}
		return "resolved a thread", true
	case "thread.reopened":
		return "reopened a thread", true
	case "thread.mentioned":
		return "mentioned " + mentionedPhrase(d.Mentioned.Kind, d.Mentioned.Name), true
	}
	return "", false
}

func objectPhrase(kind string) string {
	switch kind {
	case "dashboard", "finding", "proposal":
		return "a " + kind
	case "annotation":
		return "an annotation"
	case "observation":
		return "an observation"
	}
	return "an object"
}

func mentionedPhrase(kind, name string) string {
	switch {
	case kind == "agent":
		return "@observe"
	case name == "":
		return "someone"
	}
	return text(name)
}

type sseFollower struct {
	c      *client
	name   string
	path   string
	query  url.Values
	last   string
	retry  time.Duration
	idle   time.Duration
	handle func(sseFrame) (string, error)
}

func newActivityFollower(c *client, query url.Values, out *output, asJSON bool) *sseFollower {
	return &sseFollower{c: c, name: "activity", path: "activity/stream", query: query, last: "0", retry: defaultStreamRetry, idle: streamIdleTimeout, handle: activityFrames(out, asJSON)}
}

type idleReader struct {
	body  io.Reader
	idle  time.Duration
	timer *time.Timer
	fired atomic.Bool
}

func newIdleReader(body io.ReadCloser, idle time.Duration) *idleReader {
	r := &idleReader{body: body, idle: idle}
	r.timer = time.AfterFunc(idle, func() {
		r.fired.Store(true)
		_ = body.Close()
	})
	r.timer.Stop()
	return r
}

func (r *idleReader) Read(p []byte) (int, error) {
	r.timer.Reset(r.idle)
	defer r.timer.Stop()
	return r.body.Read(p)
}

func (f *sseFollower) run(ctx context.Context) error {
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
			err = transportFailure{fmt.Errorf("the server closed the %s stream without sending anything", f.name)}
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

func (f *sseFollower) once(ctx context.Context) (bool, error) {
	body, err := f.c.stream(ctx, f.path, f.query, f.last)
	if err != nil {
		return false, err
	}
	defer func() { _ = body.Close() }()
	watched := newIdleReader(body, f.idle)
	received := false
	var handled error
	err = readFrames(watched, func(fr sseFrame) error {
		received = true
		if fr.retry > 0 {
			f.retry = fr.retry
		}
		var last string
		last, handled = f.handle(fr)
		f.last = cmp.Or(last, f.last)
		return handled
	})
	switch {
	case handled != nil:
		return received, handled
	case watched.fired.Load():
		return received, transportFailure{fmt.Errorf("the %s stream sent nothing for %s", f.name, f.idle)}
	case err != nil:
		return received, f.c.transportError(ctx, err)
	}
	return received, nil
}

func activityFrames(out *output, asJSON bool) func(sseFrame) (string, error) {
	return func(fr sseFrame) (string, error) {
		switch fr.event {
		case "activity":
			event, err := decode[activityEvent]([]byte(fr.data))
			if err != nil {
				return "", err
			}
			printActivity(out, asJSON, activityEntry{raw: []byte(fr.data), event: event})
			return strconv.FormatInt(event.Seq, 10), nil
		case "reset":
			out.warn("Some activity may have been missed.")
		}
		return "", nil
	}
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
