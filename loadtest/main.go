package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxMessageBytes = 200

var (
	keyPattern   = regexp.MustCompile(`sok_[A-Za-z0-9_\-]+`)
	labelPattern = regexp.MustCompile(`^[a-z0-9-]+$`)
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type config struct {
	target           *url.URL
	remote           string
	replicas         []string
	viewerKeys       []string
	editorKeys       []string
	label            string
	out              string
	commit           string
	duration         time.Duration
	ramp             time.Duration
	refresh          time.Duration
	viewers          int
	panelConcurrency int
	streams          int
	events           int
	eventEvery       time.Duration
	mcpClients       int
	mcpInFlight      int
	builds           []int
	buildTimeout     time.Duration
	scenarios        []string
}

type scenario struct {
	name string
	run  func(ctx context.Context, e *env) (result, error)
}

var allScenarios = []scenario{
	{"viewers", runViewers},
	{"streams", runStreams},
	{"mcp", runMCP},
	{"agent", runAgent},
}

func printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func redact(s string) string {
	return keyPattern.ReplaceAllString(s, "sok_[redacted]")
}

func readKeys(path string) ([]string, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path is a flag the operator passes
	if err != nil {
		return nil, fmt.Errorf("read keys: %w", err)
	}
	var keys []string
	for _, line := range strings.Split(string(raw), "\n") {
		if key := strings.TrimSpace(line); key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%s holds no keys", path)
	}
	return keys, nil
}

func parseBuilds(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &n); err != nil || n < 1 {
			return nil, fmt.Errorf("--builds %q must be positive integers separated by commas", s)
		}
		out = append(out, n)
	}
	return out, nil
}

func parseScenarios(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, errors.New("name a scenario: viewers, streams, mcp, agent or all")
	}
	var out []string
	for _, arg := range args {
		if arg == "all" {
			out = out[:0]
			for _, sc := range allScenarios {
				out = append(out, sc.name)
			}
			return out, nil
		}
		known := false
		for _, sc := range allScenarios {
			known = known || sc.name == arg
		}
		if !known {
			return nil, fmt.Errorf("unknown scenario %q; use viewers, streams, mcp, agent or all", arg)
		}
		out = append(out, arg)
	}
	return out, nil
}

func parseFlags(args []string, stderr io.Writer) (*config, error) {
	fs := flag.NewFlagSet("loadtest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := &config{}
	var target, viewerKeys, editorKeys, builds string
	var replicas stringList
	fs.StringVar(&target, "target", "", "base URL of the load balancer in front of the servers (required)")
	fs.StringVar(&cfg.remote, "remote", "", "host name of a target that is not this computer, to load test it on purpose")
	fs.Var(&replicas, "replica", "base URL of one server, scraped for /metrics; repeat for each server")
	fs.StringVar(&viewerKeys, "viewer-keys", "", "file with one viewer service-account key per line")
	fs.StringVar(&editorKeys, "editor-keys", "", "file with one editor service-account key per line")
	fs.StringVar(&cfg.label, "label", "", "name of the run, lowercase letters, digits and dashes (required)")
	fs.StringVar(&cfg.out, "out", "loadtest/reports", "directory that receives one folder per run")
	fs.StringVar(&cfg.commit, "commit", "", "commit the servers run, recorded in the report")
	fs.DurationVar(&cfg.duration, "duration", 3*time.Minute, "how long each phase holds its full load")
	fs.DurationVar(&cfg.ramp, "ramp", 30*time.Second, "how long each phase takes to start its clients")
	fs.IntVar(&cfg.viewers, "viewers", 200, "simulated dashboard viewers")
	fs.IntVar(&cfg.panelConcurrency, "panel-concurrency", 6, "panel data requests one viewer keeps in flight")
	fs.DurationVar(&cfg.refresh, "refresh", 10*time.Second, "time a viewer waits between reloads")
	fs.IntVar(&cfg.streams, "streams", 500, "open activity streams")
	fs.IntVar(&cfg.events, "events", 60, "annotations written while the streams are open")
	fs.DurationVar(&cfg.eventEvery, "event-every", time.Second, "time between annotations")
	fs.IntVar(&cfg.mcpClients, "mcp-clients", 10, "MCP clients, one editor key each")
	fs.IntVar(&cfg.mcpInFlight, "mcp-in-flight", 4, "tool calls each MCP client keeps in flight")
	fs.StringVar(&builds, "builds", "20,50", "batch sizes of simultaneous agent builds")
	fs.DurationVar(&cfg.buildTimeout, "build-timeout", 10*time.Minute, "how long one batch of builds may take")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	cfg.replicas = replicas
	if err := resolve(cfg, target, viewerKeys, editorKeys, builds, fs.Args()); err != nil {
		return nil, err
	}
	return cfg, nil
}

func resolve(cfg *config, target, viewerKeys, editorKeys, builds string, scenarios []string) error {
	if target == "" || cfg.label == "" {
		return errors.New("--target and --label are required")
	}
	if !labelPattern.MatchString(cfg.label) {
		return fmt.Errorf("--label %q must be lowercase letters, digits and dashes", cfg.label)
	}
	u, err := checkTarget(target, cfg.remote)
	if err != nil {
		return err
	}
	cfg.target = u
	for _, r := range cfg.replicas {
		if _, err := checkTarget(r, cfg.remote); err != nil {
			return fmt.Errorf("--replica: %w", err)
		}
	}
	if cfg.scenarios, err = parseScenarios(scenarios); err != nil {
		return err
	}
	if cfg.builds, err = parseBuilds(builds); err != nil {
		return err
	}
	if cfg.viewerKeys, err = readKeys(viewerKeys); err != nil {
		return fmt.Errorf("--viewer-keys: %w", err)
	}
	if cfg.editorKeys, err = readKeys(editorKeys); err != nil {
		return fmt.Errorf("--editor-keys: %w", err)
	}
	return nil
}

type apiClient struct {
	base string
	http *http.Client
}

func newAPIClient(base string, timeout time.Duration) *apiClient {
	transport := &http.Transport{
		MaxIdleConns:        2000,
		MaxIdleConnsPerHost: 2000,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	return &apiClient{base: strings.TrimRight(base, "/"), http: &http.Client{Transport: transport, Timeout: timeout}}
}

type reply struct {
	status int
	body   []byte
	took   time.Duration
	err    error
}

func (r reply) ok() bool { return r.err == nil && r.status >= 200 && r.status < 300 }

func (r reply) code() string {
	var e struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(r.body, &e) != nil {
		return ""
	}
	return e.Code
}

func (r reply) failureCode() string {
	switch {
	case r.err != nil:
		return "transport"
	case r.code() != "":
		return r.code()
	}
	return fmt.Sprintf("http_%d", r.status)
}

func (r reply) message() string {
	if r.err != nil {
		return truncate(redact(r.err.Error()))
	}
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(r.body, &e) == nil && e.Message != "" {
		return truncate(redact(e.Message))
	}
	return truncate(redact(string(r.body)))
}

func truncate(text string) string {
	if len(text) > maxMessageBytes {
		return text[:maxMessageBytes]
	}
	return text
}

func (r reply) describe() string {
	if r.err != nil {
		return redact(r.err.Error())
	}
	return fmt.Sprintf("%d %s", r.status, truncate(redact(string(r.body))))
}

func (r reply) record(rec *recorder) {
	if r.ok() {
		rec.Observe(r.took)
		return
	}
	rec.Fail(r.status, r.code())
}

func (a *apiClient) call(ctx context.Context, key, method, path string, payload any) reply {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return reply{err: err}
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, body) //nolint:gosec // the base is the checked --target
	if err != nil {
		return reply{err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	start := time.Now()
	resp, err := a.http.Do(req) //nolint:gosec // the base is the checked --target
	if err != nil {
		return reply{err: err, took: time.Since(start)}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return reply{err: err, took: time.Since(start)}
	}
	return reply{status: resp.StatusCode, body: raw, took: time.Since(start)}
}

func (a *apiClient) get(ctx context.Context, key, path string) reply {
	return a.call(ctx, key, http.MethodGet, path, nil)
}

func (a *apiClient) post(ctx context.Context, key, path string, payload any) reply {
	return a.call(ctx, key, http.MethodPost, path, payload)
}

type env struct {
	cfg      *config
	api      *apiClient
	scrapers []scraper
	once     sync.Once
	dash     dashboardInfo
	dashErr  error
	slots    map[string]chan struct{}
}

func newEnv(cfg *config) *env {
	e := &env{cfg: cfg, api: newAPIClient(cfg.target.String(), 30*time.Second), slots: map[string]chan struct{}{}}
	for _, key := range cfg.viewerKeys {
		e.slots[key] = make(chan struct{}, callerQueryLimit)
	}
	bases := cfg.replicas
	if len(bases) == 0 {
		bases = []string{cfg.target.String()}
	}
	for _, b := range bases {
		e.scrapers = append(e.scrapers, scraper{client: &http.Client{Timeout: 10 * time.Second}, baseURL: strings.TrimRight(b, "/")})
	}
	return e
}

func (e *env) scrapeAll(ctx context.Context) [][]sample {
	out := make([][]sample, len(e.scrapers))
	for i, sc := range e.scrapers {
		samples, err := sc.scrape(ctx)
		if err == nil {
			out[i] = samples
		}
	}
	return out
}

func (e *env) sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (e *env) editorKey(i int) string { return e.cfg.editorKeys[i%len(e.cfg.editorKeys)] }

func (e *env) viewerKey(i int) string { return e.cfg.viewerKeys[i%len(e.cfg.viewerKeys)] }

func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		printf(stderr, "loadtest: %s\n", redact(err.Error()))
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return execute(ctx, cfg, stdout, stderr)
}

func execute(ctx context.Context, cfg *config, stdout, stderr io.Writer) int {
	e := newEnv(cfg)
	rep := report{
		Label:   cfg.label,
		Commit:  cfg.commit,
		Target:  cfg.target.String(),
		Started: time.Now().UTC(),
		Flags:   cfg.describe(),
	}
	for _, name := range cfg.scenarios {
		printf(stdout, "running %s\n", name)
		res, err := runScenario(ctx, e, name)
		if err != nil {
			rep.Gates = append(rep.Gates, gate{Name: name + " ran to the end", Pass: false, Detail: redact(err.Error())})
		}
		rep.Sections = append(rep.Sections, res.sections...)
		rep.Gates = append(rep.Gates, res.gates...)
	}
	rep.Ended = time.Now().UTC()
	dir, err := writeReport(cfg.out, rep)
	if err != nil {
		printf(stderr, "loadtest: %s\n", err)
		return 2
	}
	printf(stdout, "report directory: %s\n", dir)
	printf(stdout, "%s", rep.gateSummary())
	if rep.failed() {
		return 1
	}
	return 0
}

func runScenario(ctx context.Context, e *env, name string) (result, error) {
	for _, sc := range allScenarios {
		if sc.name == name {
			return sc.run(ctx, e)
		}
	}
	return result{}, fmt.Errorf("unknown scenario %q", name)
}

func (cfg *config) describe() map[string]string {
	return map[string]string{
		"remote":            cfg.remote,
		"replicas":          strings.Join(cfg.replicas, ","),
		"viewer_keys":       fmt.Sprint(len(cfg.viewerKeys)),
		"editor_keys":       fmt.Sprint(len(cfg.editorKeys)),
		"duration":          cfg.duration.String(),
		"ramp":              cfg.ramp.String(),
		"viewers":           fmt.Sprint(cfg.viewers),
		"panel_concurrency": fmt.Sprint(cfg.panelConcurrency),
		"refresh":           cfg.refresh.String(),
		"streams":           fmt.Sprint(cfg.streams),
		"events":            fmt.Sprint(cfg.events),
		"event_every":       cfg.eventEvery.String(),
		"mcp_clients":       fmt.Sprint(cfg.mcpClients),
		"mcp_in_flight":     fmt.Sprint(cfg.mcpInFlight),
		"builds":            fmt.Sprint(cfg.builds),
		"build_timeout":     cfg.buildTimeout.String(),
		"scenarios":         strings.Join(cfg.scenarios, ","),
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
