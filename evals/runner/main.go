package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type corpusToolkit struct {
	tasks  func(root string, sample map[string]bool, suite string) ([]Scenario, error)
	sample func(path string) (map[string]bool, error)
	replay func(out string, tokenCap int) error
	synth  func(out string) error
}

var corpusTools = corpusToolkit{
	tasks:  corpusTasks,
	sample: loadSample,
	replay: generateReplay,
	synth:  generateSynth,
}

type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

const (
	corpusAll    = "all"
	corpusSample = "sample"
	corpusNone   = "none"

	profileDefault = "default"

	defaultMinDoneWhen = 8
	defaultSampleFile  = "evals/corpus-sample.yaml"
	defaultStackScript = "scripts/evals-stack.sh"

	replayTokenCap = 16000

	maxRepeat = 20
)

type options struct {
	server, token      string
	scenarios, pricing string
	prometheus         string
	ids, tags          listFlag
	list               bool
	parallel           int
	repeat             int
	timeout            time.Duration
	out, markdown      string
	compare            string
	compareNew         string
	minPass            float64
	minQuality         float64
	minDoneWhen        int
	minDoneWhenSet     bool

	suite, corpus string
	sampleFile    string
	matrix        string
	bootstrapDex  bool
	profile       string
	k8sGapsFile   string
	verifyK8s     bool
	shops         shopURLs
	stackScript   string
	readyTimeout  time.Duration

	faults *shopFaults
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	o := options{sampleFile: defaultSampleFile, k8sGapsFile: defaultK8sGapsFile, stackScript: defaultStackScript, readyTimeout: defaultReadyTimeout}
	fs := flag.NewFlagSet("runner", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.server, "server", os.Getenv("OBSERVE_SERVER"), "base URL of the SlashObserve server (env OBSERVE_SERVER)")
	fs.StringVar(&o.token, "token", os.Getenv("OBSERVE_TOKEN"), "service-account key (env OBSERVE_TOKEN)")
	fs.StringVar(&o.scenarios, "scenarios", "evals/scenarios", "directory of scenario files")
	fs.StringVar(&o.pricing, "pricing", "", "model pricing file (default: the server's built-in price table)")
	fs.StringVar(&o.prometheus, "prometheus", defaultPrometheus, "Prometheus that the expect.scope selectors of the coverage check read their series from")
	fs.Var(&o.ids, "id", "run only this scenario id (repeatable)")
	fs.Var(&o.tags, "tag", "run only scenarios with this tag (repeatable)")
	fs.BoolVar(&o.list, "list", false, "print the selected scenarios and exit")
	fs.IntVar(&o.parallel, "parallel", 2, "scenarios to run at a time, at most 3 (the server's per-key limit of active observe requests)")
	fs.IntVar(&o.repeat, "repeat", 1, fmt.Sprintf("run each selected scenario this many times, 1 to %d; runs of one scenario never overlap and each is its own result", maxRepeat))
	fs.DurationVar(&o.timeout, "timeout", defaultTimeout, "how long to wait for one agent request; at least the server's run limit plus a margin, so the server reports its own failure code")
	fs.StringVar(&o.out, "out", "", "write the JSON report to this file, or with --matrix to this directory")
	fs.StringVar(&o.markdown, "markdown", "", "write the Markdown report to this file")
	fs.StringVar(&o.compare, "compare", "", "compare with this base JSON report: the report named after the flags, or this run's --out, and write "+compareFile+" next to it")
	fs.Float64Var(&o.minPass, "min-pass", 0.8, "minimum share of scenarios that must pass")
	fs.Float64Var(&o.minQuality, "min-quality", 0, "minimum median quality score, 0 to 1, of every kind that saved a dashboard; 0 turns the gate off")
	fs.IntVar(&o.minDoneWhen, "min-done-when", defaultMinDoneWhen, "minimum done-when scenarios that must pass when all of them ran")
	fs.StringVar(&o.suite, "suite", "", "run every scenario and corpus task that declares this suite, such as replay, live or smoke; without it only the create scenarios of the live suite and those tagged "+tagReplaySmoke+" run")
	fs.StringVar(&o.corpus, "corpus", "", "corpus tasks to add: all, sample or none (default all for replay, sample for live, none without --suite)")
	fs.StringVar(&o.sampleFile, "corpus-sample", o.sampleFile, "file listing the corpus dashboards of the sample")
	fs.StringVar(&o.matrix, "matrix", "", "run the suite once per model of this file, recreating the server between entries")
	fs.BoolVar(&o.bootstrapDex, "bootstrap-dex", false, "sign in to the demo stack as its development user and mint a service-account key for this run")
	fs.StringVar(&o.profile, "profile", profileDefault, "corpus profile: default, or k8s for the Kubernetes dashboards on the demo cluster (live suite only)")
	fs.BoolVar(&o.verifyK8s, "verify-k8s", false, "import the Kubernetes dashboards and compare their unverified panels with "+defaultK8sGapsFile+", then exit; needs no scenarios and no LLM")
	fs.Var(&o.shops, "shop-url", "pod=url of a demo shop fault API (repeatable; defaults to the evals stack's ports)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "min-done-when" {
			o.minDoneWhenSet = true
		}
	})
	if o.compare != "" {
		if fs.NArg() > 1 {
			return o, errors.New("--compare takes one report to compare against the base, or none to compare this run's --out")
		}
		o.compareNew = fs.Arg(0)
	}
	if err := validateOptions(o); err != nil {
		return o, err
	}
	o.corpus = defaultCorpus(o.suite, o.corpus)
	return o, nil
}

func validateOptions(o options) error {
	for _, check := range []func(options) error{validateLimits, validateSelection, validateProfile} {
		if err := check(o); err != nil {
			return err
		}
	}
	return nil
}

func validateLimits(o options) error {
	switch {
	case o.parallel < 1 || o.parallel > maxPerPrincipal:
		return fmt.Errorf("--parallel must be between 1 and %d, the server's limit of queued or running observe requests per key", maxPerPrincipal)
	case o.repeat < 1 || o.repeat > maxRepeat:
		return fmt.Errorf("--repeat must be between 1 and %d", maxRepeat)
	case o.minPass < 0 || o.minPass > 1:
		return errors.New("--min-pass must be between 0 and 1")
	case o.minQuality < 0 || o.minQuality > 1:
		return errors.New("--min-quality must be between 0 and 1")
	case o.timeout < defaultTimeout:
		return fmt.Errorf("--timeout must be at least %s: the evals stack's run limit of %s plus %s to report how a run ended", defaultTimeout, serverRunLimit, serverSettleMargin)
	}
	return nil
}

func validateSelection(o options) error {
	switch {
	case o.suite != "" && !suitePattern.MatchString(o.suite):
		return fmt.Errorf("--suite %q must match %s", o.suite, suitePattern)
	case o.corpus != "" && !slices.Contains([]string{corpusAll, corpusSample, corpusNone}, o.corpus):
		return errors.New("--corpus must be all, sample or none")
	case o.corpus != "" && o.corpus != corpusNone && o.suite == "":
		return errors.New("--corpus needs --suite")
	case o.matrix != "" && o.out == "":
		return errors.New("--matrix needs --out, the directory that receives one report per model")
	case o.compare != "" && o.matrix != "":
		return errors.New("--compare cannot run a matrix; compare each <name>.json of its --out directory instead")
	case o.compare != "" && o.compareNew == "" && o.out == "":
		return errors.New("--compare needs --out, the report it compares and the directory that receives " + compareFile)
	}
	return nil
}

func validateProfile(o options) error {
	switch {
	case o.profile != profileDefault && o.profile != profileK8s:
		return errors.New("--profile must be default or k8s")
	case o.profile == profileK8s && o.suite != "" && o.suite != suiteLive:
		return fmt.Errorf("--profile k8s runs the live suite only, not --suite %s", o.suite)
	}
	return nil
}

func defaultCorpus(suite, chosen string) string {
	switch {
	case chosen != "":
		return chosen
	case suite == "":
		return corpusNone
	case suite == suiteReplay:
		return corpusAll
	}
	return corpusSample
}

func corpusSuite(suite string) string {
	if suite == suiteReplay {
		return suiteReplay
	}
	return suiteLive
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "prepare":
			return prepare(args[1:], stderr)
		case "list":
			args = append([]string{"--list"}, args[1:]...)
		}
	}
	o, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return fail(stderr, err)
	}
	switch {
	case o.compareNew != "":
		return executeCompare(o.compare, o.compareNew, stdout, stderr)
	case o.verifyK8s:
		return executeVerifyK8s(o, stdout, stderr)
	}
	all, err := loadScenarios(o.scenarios)
	if err != nil {
		return fail(stderr, err)
	}
	pool, err := withCorpusTasks(all, o)
	if err != nil {
		return fail(stderr, err)
	}
	selected, err := selectScenarios(pool, o)
	if err != nil {
		return fail(stderr, err)
	}
	if o.list {
		listScenarios(selected, stdout)
		return 0
	}
	doneWhen := countDoneWhen(all)
	if o.minDoneWhen, err = minDoneWhen(o, doneWhen); err != nil {
		return fail(stderr, err)
	}
	return execute(o, selected, doneWhen, stdout, stderr)
}

func minDoneWhen(o options, doneWhen int) (int, error) {
	switch {
	case doneWhen == 0:
	case !o.minDoneWhenSet:
		return min(defaultMinDoneWhen, doneWhen), nil
	case o.minDoneWhen < 0 || o.minDoneWhen > doneWhen:
		return 0, fmt.Errorf("--min-done-when must be between 0 and %d, the number of done_when scenarios", doneWhen)
	}
	return o.minDoneWhen, nil
}

func listScenarios(selected []Scenario, stdout io.Writer) {
	width := 0
	for _, sc := range selected {
		width = max(width, len(sc.ID))
	}
	for _, sc := range selected {
		_, _ = fmt.Fprintf(stdout, "%-*s %-10s done_when=%t tags=%s kind=%s\n", width, sc.ID, sc.Category, sc.DoneWhen, strings.Join(sc.Tags, ","), kindOf(sc))
	}
}

func withCorpusTasks(files []Scenario, o options) ([]Scenario, error) {
	if o.corpus == corpusNone {
		return files, nil
	}
	tasks, err := buildCorpusTasks(o)
	if err != nil {
		return nil, fmt.Errorf("build the corpus tasks: %w", err)
	}
	pool := slices.Clone(files)
	seen := map[string]bool{}
	for _, sc := range files {
		seen[sc.ID] = true
	}
	for _, task := range tasks {
		if seen[task.ID] {
			return nil, fmt.Errorf("corpus task id %q is also a scenario file id", task.ID)
		}
		seen[task.ID] = true
		pool = append(pool, task)
	}
	return pool, nil
}

func buildCorpusTasks(o options) ([]Scenario, error) {
	load, build := corpusTools.sample, corpusTools.tasks
	if o.profile == profileK8s {
		load, build = loadK8sSample, k8sCorpusTasks
	}
	var sample map[string]bool
	if o.corpus == corpusSample {
		var err error
		if sample, err = load(o.sampleFile); err != nil {
			return nil, fmt.Errorf("read the corpus sample: %w", err)
		}
	}
	return build(corpusDir, sample, corpusSuite(o.suite))
}

func inSelection(sc Scenario, suite string) bool {
	if suite != "" {
		return slices.Contains(sc.Suites, suite)
	}
	live := slices.Contains(sc.Suites, suiteLive)
	return ((kindOf(sc) == kindCreate || kindOf(sc) == kindQuestion) && live) || slices.Contains(sc.Tags, tagReplaySmoke)
}

func selectScenarios(pool []Scenario, o options) ([]Scenario, error) {
	if o.suite != "" && !slices.ContainsFunc(pool, func(sc Scenario) bool { return slices.Contains(sc.Suites, o.suite) }) {
		return nil, fmt.Errorf("--suite %s: no scenario or corpus task declares this suite", o.suite)
	}
	for _, id := range o.ids {
		if i := slices.IndexFunc(pool, func(sc Scenario) bool { return sc.ID == id }); i >= 0 && !inSelection(pool[i], o.suite) {
			return nil, fmt.Errorf("scenario %q is not in this selection; pass --suite %s", id, strings.Join(pool[i].Suites, " or --suite "))
		}
	}
	inSuite := slices.DeleteFunc(slices.Clone(pool), func(sc Scenario) bool { return !inSelection(sc, o.suite) })
	return filterScenarios(inSuite, o.ids, o.tags)
}

func countDoneWhen(scenarios []Scenario) int {
	n := 0
	for _, sc := range scenarios {
		if sc.DoneWhen {
			n++
		}
	}
	return n
}

func execute(o options, selected []Scenario, doneWhen int, stdout, stderr io.Writer) int {
	pricing, err := loadPricing(o.pricing)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	token := o.token
	if o.bootstrapDex {
		if token, err = bootstrapKey(ctx, o.server); err != nil {
			return fail(stderr, err)
		}
	}
	c, err := newClient(o.server, token)
	if err != nil {
		return fail(stderr, err)
	}
	o.faults = newShopFaults(shopURLsFor(o.profile, o.shops, os.Getenv))

	if o.matrix != "" {
		return executeMatrix(ctx, c, pricing, selected, o, doneWhen, stderr)
	}
	report, gates, err := runSuite(ctx, c, pricing, selected, o, doneWhen, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	markdown := renderMarkdown(report, gates)
	_, _ = io.WriteString(stdout, markdown)
	if err := writeReports(o, report, markdown); err != nil {
		return fail(stderr, err)
	}
	if o.compare != "" {
		if err := compareWith(o.compare, report, o.out, stdout); err != nil {
			return fail(stderr, err)
		}
	}
	if ctx.Err() != nil || !gates.ok() {
		return 1
	}
	return 0
}

func executeVerifyK8s(o options, stdout, stderr io.Writer) int {
	gaps, err := loadK8sGaps(o.k8sGapsFile)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	token := o.token
	if o.bootstrapDex {
		if token, err = bootstrapKey(ctx, o.server); err != nil {
			return fail(stderr, err)
		}
	}
	c, err := newClient(o.server, token)
	if err != nil {
		return fail(stderr, err)
	}
	problems, warnings, err := verifyK8s(ctx, c, gaps)
	if err != nil {
		return fail(stderr, err)
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintln(stdout, "warning:", warning)
	}
	for _, problem := range problems {
		_, _ = fmt.Fprintln(stdout, problem)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

func runSuite(ctx context.Context, c *client, pricing Pricing, selected []Scenario, o options, doneWhen int, stderr io.Writer) (Report, gateOutcome, error) {
	info, err := c.agent(ctx)
	if err != nil {
		return Report{}, gateOutcome{}, fmt.Errorf("read the agent configuration: %w", err)
	}
	if !info.Available || info.Provider == nil || info.Model == nil {
		return Report{}, gateOutcome{}, errors.New("the server has no LLM provider configured")
	}
	report := Report{StartedAt: time.Now().UTC(), Provider: *info.Provider, Model: *info.Model}
	report.Results = runAll(ctx, c, pricing, selected, o, stderr)
	report.Aggregates = aggregate(report.Results)
	for _, model := range report.Aggregates.Cost.UnpricedModels {
		_, _ = fmt.Fprintf(stderr, "pricing: cost unknown for model %s, left blank\n", model)
	}
	return report, evaluateGates(report.Aggregates, gateLimits{minPass: o.minPass, minQuality: o.minQuality, minDoneWhen: o.minDoneWhen, doneWhen: doneWhen, repeat: o.repeat}), nil
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "runner:", err)
	return 2
}

func writeReports(o options, report Report, markdown string) error {
	if o.out != "" {
		if err := writeJSON(o.out, report); err != nil {
			return fmt.Errorf("write %s: %w", o.out, err)
		}
	}
	if o.markdown != "" {
		if err := os.WriteFile(o.markdown, []byte(markdown), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", o.markdown, err)
		}
	}
	return nil
}

func prepare(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("prepare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "directory that receives llm/ (replay fixtures) and synth/ (synthetic series)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *out == "" {
		return fail(stderr, errors.New("prepare needs --out"))
	}
	steps := []struct {
		dir      string
		generate func(string) error
	}{
		{"llm", func(dir string) error { return corpusTools.replay(dir, replayTokenCap) }},
		{"synth", corpusTools.synth},
	}
	for _, step := range steps {
		target := filepath.Join(*out, step.dir)
		if err := os.MkdirAll(target, exposedDirPerm); err != nil {
			return fail(stderr, err)
		}
		if err := step.generate(target); err != nil {
			return fail(stderr, fmt.Errorf("prepare %s: %w", step.dir, err))
		}
	}
	return 0
}

func runAll(ctx context.Context, c *client, pricing Pricing, scenarios []Scenario, o options, stderr io.Writer) []Result {
	repeat := max(1, o.repeat)
	results := make([]Result, len(scenarios)*repeat)
	previous := make([]chan struct{}, len(scenarios))
	slots := make(chan struct{}, o.parallel)
	var wg sync.WaitGroup
	var logMu sync.Mutex
	for round := range repeat {
		for i, sc := range scenarios {
			if previous[i] != nil {
				<-previous[i]
			}
			done := make(chan struct{})
			previous[i] = done
			wg.Add(1)
			slots <- struct{}{}
			go func() {
				defer close(done)
				defer wg.Done()
				defer func() { <-slots }()
				res, abandoned := runScenario(ctx, c, pricing, sc, o)
				label := sc.ID
				if repeat > 1 {
					res.Run = round + 1
					label = fmt.Sprintf("%s (run %d of %d)", sc.ID, res.Run, repeat)
				}
				results[i*repeat+round] = res
				logMu.Lock()
				_, _ = fmt.Fprintf(stderr, "%s  %s\n", verdictWord(res), label)
				logMu.Unlock()
				if abandoned != nil {
					logMu.Lock()
					_, _ = fmt.Fprintf(stderr, "draining %s (up to %s)\n", abandoned.ID, graceLabel(c.grace))
					logMu.Unlock()
					drained := c.drain(ctx, *abandoned)
					logMu.Lock()
					_, _ = fmt.Fprintf(stderr, "drained %s: %s\n", drained.ID, drainedOutcome(drained))
					logMu.Unlock()
				}
			}()
		}
	}
	wg.Wait()
	return results
}

func verdictWord(r Result) string {
	switch {
	case r.Skipped:
		return "SKIP"
	case !r.Passed:
		return "FAIL"
	}
	return "PASS"
}

func drainedOutcome(req AgentRequest) string {
	if code := req.FailureCode; code != nil {
		return req.Status + " with failure code " + *code
	}
	return req.Status
}

func graceLabel(d time.Duration) string {
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return d.String()
}

var slotWaitNotice = 10 * time.Second

type execution struct {
	c       *client
	pricing Pricing
	sc      Scenario
	o       options
	res     Result
	obs     observation
}

func runScenario(ctx context.Context, c *client, pricing Pricing, sc Scenario, o options) (Result, *AgentRequest) {
	started := time.Now().UTC()
	res, abandoned := runFaulted(ctx, c, pricing, sc, o)
	res.StartedAt, res.FinishedAt = started, time.Now().UTC()
	return res, abandoned
}

func runFaulted(ctx context.Context, c *client, pricing Pricing, sc Scenario, o options) (Result, *AgentRequest) {
	e := &execution{
		c: c, pricing: pricing, sc: sc, o: o,
		res: Result{ID: sc.ID, Prompt: sc.Prompt, Category: sc.Category, DoneWhen: sc.DoneWhen, Tags: sc.Tags, Kind: kindOf(sc), Suite: o.suite, Status: "not_started"},
		obs: observation{panels: map[string]panelOutcome{}},
	}
	if sc.Fault == nil {
		return e.dispatch(ctx)
	}
	var res Result
	var abandoned *AgentRequest
	ran := false
	err := o.faults.withFault(ctx, *sc.Fault, func(ctx context.Context) error {
		ran = true
		res, abandoned = e.dispatch(ctx)
		return nil
	})
	switch {
	case !ran:
		e.res.Error = err.Error()
		e.obs.request = AgentRequest{Status: e.res.Status}
		return finish(sc, pricing, e.res, e.obs), nil
	case err != nil:
		res.Error = strings.TrimSpace(res.Error + " " + err.Error())
		res.Passed, res.LatencyOnly = false, false
	}
	return res, abandoned
}

func (e *execution) dispatch(ctx context.Context) (Result, *AgentRequest) {
	switch kindOf(e.sc) {
	case kindRevise, kindFixUnverified:
		return e.runRevision(ctx)
	case kindSuggest:
		return e.runSuggestion(ctx)
	case kindChatImport, kindSizeGuard:
		return e.runChatImport(ctx)
	case kindInvestigate:
		return e.runAnswer(ctx, kindInvestigate)
	case kindQuestion:
		return e.runAnswer(ctx, "")
	}
	return e.runCreate(ctx)
}

func (e *execution) done(abandoned *AgentRequest) (Result, *AgentRequest) {
	if e.res.Skipped {
		return e.res, nil
	}
	return finish(e.sc, e.pricing, e.res, e.obs), abandoned
}

func unfinished(req AgentRequest) *AgentRequest {
	if req.finished() {
		return nil
	}
	return &req
}

func (e *execution) runCreate(ctx context.Context) (Result, *AgentRequest) {
	waitCtx, cancel := context.WithTimeout(ctx, e.o.timeout)
	defer cancel()
	createStart := time.Now()
	req, err := e.c.createObserveWhenFree(waitCtx, e.sc.Prompt, "")
	slotWait := time.Since(createStart)
	if err != nil {
		e.createFailed(waitCtx, err)
		return e.done(nil)
	}
	req, ok := e.await(ctx, waitCtx, req, slotWait)
	if ok && req.Status == "succeeded" {
		e.inspectResult(ctx, req)
		if e.sc.Expect.Scope != "" && e.obs.dashboard != nil {
			e.measureCoverage(ctx)
		}
	}
	return e.done(unfinished(req))
}

func (e *execution) runAnswer(ctx context.Context, kind string) (Result, *AgentRequest) {
	waitCtx, cancel := context.WithTimeout(ctx, e.o.timeout)
	defer cancel()
	createStart := time.Now()
	req, err := e.c.createObserveWhenFree(waitCtx, e.sc.Prompt, kind)
	slotWait := time.Since(createStart)
	if err != nil {
		e.createFailed(waitCtx, err)
		return e.done(nil)
	}
	req, _ = e.await(ctx, waitCtx, req, slotWait)
	return e.done(unfinished(req))
}

func (e *execution) runRevision(ctx context.Context) (Result, *AgentRequest) {
	seed, ok := e.seed(ctx)
	if !ok {
		return e.done(nil)
	}
	prompt, ok := e.prompt(seed)
	if !ok {
		return e.done(nil)
	}
	panelID := ""
	if kindOf(e.sc) == kindFixUnverified {
		panelID = seed.targetID
	}
	waitCtx, cancel := context.WithTimeout(ctx, e.o.timeout)
	defer cancel()
	createStart := time.Now()
	req, err := e.c.createRevision(waitCtx, seed.dashboardID, seed.version, panelID, prompt)
	slotWait := time.Since(createStart)
	if err != nil {
		e.createFailed(waitCtx, err)
		return e.done(nil)
	}
	req, ok = e.await(ctx, waitCtx, req, slotWait)
	if ok && req.Status == "succeeded" {
		e.inspectResult(ctx, req)
		e.skipWhenVariablesBlockData(ctx, seed)
	}
	return e.done(unfinished(req))
}

func (e *execution) skipWhenVariablesBlockData(ctx context.Context, seed seeded) {
	if !needsPanelData(e.sc) || !e.obs.panelDataRejected() {
		return
	}
	if reason := unlistableVariable(ctx, e.c, seed); reason != "" {
		e.res.Skipped, e.res.Reason, e.res.Status = true, "no_data: "+reason, "skipped"
	}
}

func (e *execution) runSuggestion(ctx context.Context) (Result, *AgentRequest) {
	seed, ok := e.seed(ctx)
	if !ok {
		return e.done(nil)
	}
	prompt, ok := e.prompt(seed)
	if !ok {
		return e.done(nil)
	}
	waitCtx, cancel := context.WithTimeout(ctx, e.o.timeout)
	defer cancel()
	start := time.Now()
	sug, err := e.c.createSuggestion(waitCtx, seed.dashboardID, seed.version, seed.targetID, prompt)
	slotWait := time.Since(start)
	if err != nil {
		e.createFailed(waitCtx, err)
		return e.done(nil)
	}
	sug, err = e.c.waitSuggestion(waitCtx, seed.dashboardID, sug)
	e.obs.request = suggestionRequest(sug, start)
	e.res.RequestID, e.res.Status = sug.ID, sug.Status
	if err != nil {
		e.explainWait(ctx, waitCtx, err, slotWait)
		return e.done(nil)
	}
	if sug.Error != nil {
		e.res.Error = *sug.Error
	}
	if sug.Status == "succeeded" {
		e.obs.suggestion = &sug
		e.checkVersionAdded(ctx, seed)
	}
	return e.done(nil)
}

func suggestionRequest(s Suggestion, created time.Time) AgentRequest {
	req := AgentRequest{ID: s.ID, Status: s.Status, Error: s.Error, FailureCode: s.FailureCode, Usage: s.Usage, CreatedAt: created}
	if s.finished() {
		finished := time.Now()
		req.FinishedAt = &finished
	}
	return req
}

func (e *execution) checkVersionAdded(ctx context.Context, seed seeded) {
	_, err := e.c.getVersion(ctx, seed.dashboardID, seed.version+1)
	var api *apiError
	switch {
	case err == nil:
		e.obs.versionAdded = true
	case errors.As(err, &api) && api.Status == http.StatusNotFound:
	default:
		e.obs.specErr = "check for a new version: " + err.Error()
	}
}

func (c *client) waitSuggestion(ctx context.Context, dashboardID string, last Suggestion) (Suggestion, error) {
	ticker := time.NewTicker(c.poll)
	defer ticker.Stop()
	for {
		s, err := c.getSuggestion(ctx, dashboardID, last.ID)
		switch {
		case err == nil:
			last = s
			if last.finished() {
				return last, nil
			}
		case ctx.Err() != nil || !transient(err):
			return last, err
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (e *execution) runChatImport(ctx context.Context) (Result, *AgentRequest) {
	prompt, err := fillPrompt(e.sc, seeded{})
	if err != nil {
		e.res.Error = "build the prompt: " + err.Error()
		e.obs.request = AgentRequest{Status: e.res.Status}
		return e.done(nil)
	}
	defer e.c.holdImportSlot()()
	waitCtx, cancel := context.WithTimeout(ctx, e.o.timeout)
	defer cancel()
	createStart := time.Now()
	req, err := e.c.createObserveWhenFree(waitCtx, prompt, "")
	slotWait := time.Since(createStart)
	if err != nil {
		if promptRejected(err, prompt) {
			code := failurePromptTooLong
			e.res.Error = "create the request: " + err.Error()
			e.obs.request = AgentRequest{Status: e.res.Status, FailureCode: &code}
			return e.done(nil)
		}
		e.createFailed(waitCtx, err)
		return e.done(nil)
	}
	req, ok := e.await(ctx, waitCtx, req, slotWait)
	if ok && req.Status == "succeeded" {
		e.inspectImport(ctx, req)
	}
	return e.done(unfinished(req))
}

func promptRejected(err error, prompt string) bool {
	var api *apiError
	if !errors.As(err, &api) || utf8.RuneCountInString(prompt) <= maxPromptRunes {
		return false
	}
	return api.Status == http.StatusBadRequest && api.Code == "invalid_request" ||
		api.Status == http.StatusRequestEntityTooLarge && api.Code == "too_large"
}

func (e *execution) inspectImport(ctx context.Context, req AgentRequest) {
	if req.DashboardID == nil || req.ResultVersion == nil {
		return
	}
	e.res.DashboardID = *req.DashboardID
	record, err := e.c.dashboardImport(ctx, *req.DashboardID)
	if err != nil {
		e.obs.importErr = "read the import record: " + err.Error()
		return
	}
	e.obs.importRecord = &record
}

func (e *execution) createFailed(waitCtx context.Context, err error) {
	e.res.Error = "create the request: " + err.Error()
	if slotsFull(err) && waitCtx.Err() != nil {
		e.res.Error = fmt.Sprintf("no request slot within %s: %s", e.o.timeout, err)
	}
	e.obs.request = AgentRequest{Status: e.res.Status}
}

func (e *execution) await(ctx, waitCtx context.Context, req AgentRequest, slotWait time.Duration) (AgentRequest, bool) {
	req, err := e.c.waitObserve(waitCtx, req)
	e.obs.request = req
	e.res.RequestID, e.res.Status = req.ID, req.Status
	switch {
	case err != nil:
		e.explainWait(ctx, waitCtx, err, slotWait)
	case req.Error != nil:
		e.res.Error = *req.Error
	}
	return req, err == nil
}

func (e *execution) explainWait(ctx, waitCtx context.Context, err error, slotWait time.Duration) {
	if waitCtx.Err() == nil {
		e.res.Error = "poll the request: " + err.Error()
		return
	}
	if ctx.Err() != nil {
		e.res.Error = "interrupted while waiting for the agent"
		return
	}
	e.res.RunnerCode = runnerTimeout
	e.res.Error = fmt.Sprintf("timed out after %s waiting for the agent, whose request is still %s on the server", e.o.timeout, e.res.Status)
	if slotWait > slotWaitNotice {
		e.res.Error += fmt.Sprintf(" (%.0fs waiting for a request slot)", slotWait.Seconds())
	}
}

func (e *execution) seed(ctx context.Context) (seeded, bool) {
	if e.needsDataCheck() {
		lacks, err := targetLacksData(ctx, e.c, *e.sc.Seed)
		if err != nil {
			return e.seedFailed(err)
		}
		if lacks {
			e.res.Skipped, e.res.Reason, e.res.Status = true, "no_data", "skipped"
			return seeded{}, false
		}
	}
	s, err := seedScenario(ctx, e.c, e.sc)
	if err != nil {
		return e.seedFailed(err)
	}
	raw, err := e.c.getVersionSpec(ctx, s.dashboardID, s.version)
	if err != nil {
		return e.seedFailed(fmt.Errorf("read the seeded version: %w", err))
	}
	parent, err := spec.Parse(raw)
	if err != nil {
		return e.seedFailed(fmt.Errorf("the seeded version does not parse: %w", err))
	}
	e.res.DashboardID = s.dashboardID
	e.obs.parent, e.obs.target = parent, s.targetID
	return s, true
}

func needsPanelData(sc Scenario) bool {
	return sc.Expect.FailureCode == "" && (kindOf(sc) == kindFixUnverified || kindOf(sc) == kindRevise)
}

func (e *execution) needsDataCheck() bool {
	return kindOf(e.sc) == kindFixUnverified && e.o.suite != suiteReplay && e.sc.Seed != nil && e.sc.Seed.BreakPanel
}

func targetLacksData(ctx context.Context, c *client, seed Seed) (bool, error) {
	broken, err := breakSourceFor(ctx, c, seed)
	if err != nil {
		return false, err
	}
	source, err := readCorpus(seed.Corpus)
	if err != nil {
		return false, err
	}
	imported, err := c.createImport(ctx, source, seed.Mapping)
	if err != nil {
		return false, fmt.Errorf("import the pristine %s: %w", seed.Corpus, err)
	}
	return imported.Unverified.hasPanel(broken.targetID), nil
}

func (e *execution) seedFailed(err error) (seeded, bool) {
	e.res.Error = "seed the dashboard: " + err.Error()
	e.obs.request = AgentRequest{Status: e.res.Status}
	return seeded{}, false
}

func (e *execution) prompt(seed seeded) (string, bool) {
	prompt, err := fillPrompt(e.sc, seed)
	if err != nil {
		e.res.Error = "build the prompt: " + err.Error()
		e.obs.request = AgentRequest{Status: e.res.Status}
		return "", false
	}
	e.res.Prompt = prompt
	return prompt, true
}

func (e *execution) inspectResult(ctx context.Context, req AgentRequest) {
	types, err := e.c.datasourceTypes(ctx)
	if err != nil {
		e.res.Error = "read the datasources: " + err.Error()
		return
	}
	e.obs.datasourceTypes = types
	inspect(ctx, e.c, e.sc, req, &e.obs, &e.res)
}

func inspect(ctx context.Context, c *client, sc Scenario, req AgentRequest, obs *observation, res *Result) {
	if req.DashboardID == nil || req.ResultVersion == nil {
		obs.specErr = "the request succeeded without a saved version"
		return
	}
	res.DashboardID = *req.DashboardID
	version, err := c.getVersion(ctx, *req.DashboardID, *req.ResultVersion)
	if err != nil {
		obs.specErr = "read the saved version: " + err.Error()
		return
	}
	dash, err := spec.Parse(version.Spec)
	if err != nil {
		obs.specErr = "the saved spec does not parse: " + err.Error()
		return
	}
	obs.dashboard, obs.unverified = dash, version.Unverified
	fetch := panelsToFetch(sc, *obs)
	for _, panel := range dataPanels(dash) {
		if !fetch(panel) {
			continue
		}
		data, err := c.getPanelData(ctx, *req.DashboardID, *req.ResultVersion, panel.ID)
		outcome := panelOutcome{data: data, err: err}
		if err == nil && wantsSeries(sc.Expect, *obs, panel) {
			outcome.labels, outcome.err = c.getPanelLabels(ctx, *req.DashboardID, *req.ResultVersion, panel.ID, len(data.Frames))
		}
		obs.panels[panel.ID] = outcome
	}
}

func (c *client) getPanelLabels(ctx context.Context, dashboardID string, version int, panelID string, frames int) ([]map[string]string, error) {
	var body struct {
		Frames []struct {
			Labels map[string]string `json:"labels"`
		} `json:"frames"`
	}
	path := fmt.Sprintf("/dashboards/%s/versions/%d/panels/%s/data", url.PathEscape(dashboardID), version, url.PathEscape(panelID))
	if err := c.do(ctx, http.MethodGet, path, nil, &body); err != nil {
		return nil, err
	}
	if len(body.Frames) != frames {
		return nil, fmt.Errorf("the panel returned %d series, then %d", frames, len(body.Frames))
	}
	labels := make([]map[string]string, len(body.Frames))
	for i, f := range body.Frames {
		labels[i] = f.Labels
	}
	return labels, nil
}

func finish(sc Scenario, pricing Pricing, res Result, obs observation) Result {
	res.Checks = runChecks(sc, obs)
	res.Passed = allPassed(res.Checks)
	res.LatencyOnly = latencyOnly(res.Checks)
	res.Usage = obs.request.Usage
	if res.Usage != nil && res.Usage.ModelCalls > 0 {
		calls := res.Usage.ModelCalls
		res.ModelCalls = &calls
	}
	switch code := obs.failureCode(); {
	case code != "":
		res.FailureCode = code
	case obs.request.Status == "failed":
		res.Uncoded = true
	}
	res.CostUSD = pricing.cost(obs.request.Usage)
	if obs.dashboard != nil {
		res.Panels = len(dataPanels(obs.dashboard))
		quality, findings := scoreQuality(obs.dashboard)
		res.Quality, res.QualityFindings = &quality, findings
	}
	if obs.coverage != nil {
		res.Coverage = obs.coverage.score()
	}
	if f := obs.request.Finding; f != nil {
		res.FindingWords, res.Evidence = len(strings.Fields(f.Markdown)), len(f.Evidence)
	}
	if elapsed, ok := latency(obs.request); ok {
		seconds := elapsed.Seconds()
		res.LatencySeconds = &seconds
	}
	return withMetrics(res, cmp.Or(sc.Kind, kindCreate), obs.request)
}
