package main

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/grafana"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	corpusIDPrefix = "corpus"
	tagCorpus      = "corpus"
	tagK8s         = "k8s"
	profileK8s     = "k8s"

	taskFix     = "fix"
	taskAdd     = "add"
	taskPaste   = "paste"
	taskRebuild = "rebuild"
	taskChange  = "change"
	taskRetime  = "retime"
	taskSuggest = "suggest"
	taskRowedit = "rowedit"

	failurePromptTooLong = "prompt_too_long"

	replayBytesPerToken = 3.2
	corpusTokenCap      = 16000
	fixSaveTokenBudget  = 2000

	rebuildMetricShare  = 0.25
	rebuildMetricCap    = 20
	rebuildFloor        = 0.8
	rebuildMaxPanels    = 200
	rebuildSlackDivisor = 4
	minRebuildPanels    = 2

	changeSeconds      = 300
	retimeSeconds      = 90
	retimeModelCalls   = 5
	rowEditSeconds     = 600
	minRowChildren     = 3
	maxRowChildren     = 8
	retimeMaxStep      = 2 * time.Minute
	retimeDataPoints   = 200
	suggestPrompt      = "Suggest a clearer title for this panel."
	rowEditDescription = "Checked by the eval"
	maxIDLength        = 64
	idHashLength       = 6

	addedPanelID    = "eval-added"
	addedPanelTitle = "Added by eval"
	addedPanelWidth = 12
	addedPanelRows  = 8

	healthyPromQuery = "up"
	healthyCHQuery   = "SELECT now() AS time, toFloat64(1) AS value"

	pasteIntro = "Import this Grafana dashboard:"
)

var (
	fixPrompt = `Fix the unverified panel "` + placeholderPanelTitle + `" (` + placeholderPanelID + `).`

	nonSlug = regexp.MustCompile(`[^a-z0-9]+`)
)

var largeDashboards = []string{
	"rfmoz/node-exporter-full.json",
	"rfmoz/haproxy.json",
	"cilium/cilium-dashboard.json",
	"victoriametrics/victoriametrics.json",
	"argo-cd/dashboard.json",
	"victoriametrics/vmalert.json",
	"clickhouse-datasource/otel-service-dashboard.json",
	"slashobserve/demo-shop.json",
}

func largeDashboardSet() map[string]bool {
	set := make(map[string]bool, len(largeDashboards))
	for _, key := range largeDashboards {
		set[key] = true
	}
	return set
}

var k8sDashboards = []string{
	"dotdc/k8s-addons-prometheus.json",
	"dotdc/k8s-system-api-server.json",
	"dotdc/k8s-system-coredns.json",
	"dotdc/k8s-views-global.json",
	"dotdc/k8s-views-namespaces.json",
	"dotdc/k8s-views-nodes.json",
	"dotdc/k8s-views-pods.json",
	"kube-prometheus/apiserver.json",
	"kube-prometheus/cluster-total.json",
	"kube-prometheus/controller-manager.json",
	"kube-prometheus/k8s-resources-cluster.json",
	"kube-prometheus/k8s-resources-multicluster.json",
	"kube-prometheus/k8s-resources-namespace.json",
	"kube-prometheus/k8s-resources-node.json",
	"kube-prometheus/k8s-resources-nodes-overview.json",
	"kube-prometheus/k8s-resources-pod.json",
	"kube-prometheus/k8s-resources-workload.json",
	"kube-prometheus/k8s-resources-workloads-namespace.json",
	"kube-prometheus/kubelet.json",
	"kube-prometheus/namespace-by-pod.json",
	"kube-prometheus/namespace-by-workload.json",
	"kube-prometheus/node-cluster-rsrc-use.json",
	"kube-prometheus/node-rsrc-use.json",
	"kube-prometheus/nodes.json",
	"kube-prometheus/persistentvolumesusage.json",
	"kube-prometheus/pod-total.json",
	"kube-prometheus/proxy.json",
	"kube-prometheus/scheduler.json",
	"kube-prometheus/workload-total.json",
}

func k8sDashboardSet() map[string]bool {
	set := make(map[string]bool, len(k8sDashboards))
	for _, key := range k8sDashboards {
		set[key] = true
	}
	return set
}

type corpusDashboard struct {
	key    string
	source string
	name   string
	raw    []byte
}

type subject struct {
	kind spec.DatasourceType
	name string
}

type queryRef struct {
	kind     spec.DatasourceType
	text     string
	variable bool
}

type analyzed struct {
	corpusDashboard
	mapping map[string]string
	dash    *spec.Dashboard
	kinds   []spec.DatasourceType
	metrics []string
	tables  []string
	subject *subject
	titles  []string
}

type task struct {
	scenario Scenario
	fixture  *replayFixture
}

type skipTask struct{ reason string }

func (s skipTask) Error() string { return s.reason }

type planner struct {
	suite   string
	k8s     bool
	seen    map[string]bool
	tasks   []task
	skipped []string
}

type sampleFile struct {
	Dashboards []string `yaml:"dashboards"`
	K8s        []string `yaml:"k8s"`
}

func stackOptions() grafana.Options { return importOptions(stackDatasources, nil) }

var stackDatasources = map[string]string{"prom": string(spec.Prometheus), "ch": string(spec.ClickHouse)}

func corpusTasks(root string, sample map[string]bool, suite string) ([]Scenario, error) {
	p, err := planCorpus(root, sample, suite)
	if err != nil {
		return nil, err
	}
	out := make([]Scenario, len(p.tasks))
	for i, t := range p.tasks {
		out[i] = t.scenario
	}
	return out, nil
}

func loadSample(path string) (map[string]bool, error) {
	file, err := readSampleFile(path)
	if err != nil {
		return nil, err
	}
	return sampleSet(path, "dashboards", file.Dashboards)
}

func loadK8sSample(path string) (map[string]bool, error) {
	file, err := readSampleFile(path)
	if err != nil {
		return nil, err
	}
	sample, err := sampleSet(path, "k8s", file.K8s)
	if err != nil {
		return nil, err
	}
	for key := range sample {
		if !slices.Contains(k8sDashboards, key) {
			return nil, fmt.Errorf("%s: k8s entry %q is not one of the Kubernetes dashboards", path, key)
		}
	}
	return sample, nil
}

func readSampleFile(path string) (sampleFile, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path comes from the operator's flags
	if err != nil {
		return sampleFile{}, err
	}
	var file sampleFile
	if err := decodeStrict(content, &file); err != nil {
		return sampleFile{}, fmt.Errorf("%s: %w", path, err)
	}
	return file, nil
}

func sampleSet(path, field string, keys []string) (map[string]bool, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("%s: %s is empty", path, field)
	}
	sample := make(map[string]bool, len(keys))
	for _, key := range keys {
		if !strings.HasSuffix(key, ".json") || !filepath.IsLocal(key) || strings.Count(key, "/") != 1 {
			return nil, fmt.Errorf("%s: %q must be <source>/<name>.json", path, key)
		}
		if sample[key] {
			return nil, fmt.Errorf("%s: %q is listed twice", path, key)
		}
		sample[key] = true
	}
	return sample, nil
}

func loadCorpus(root string, sample map[string]bool) ([]corpusDashboard, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*", "*.json"))
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	var out []corpusDashboard
	found := map[string]bool{}
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		key := filepath.ToSlash(rel)
		if len(sample) > 0 && !sample[key] {
			continue
		}
		raw, err := os.ReadFile(path) //nolint:gosec // the path comes from the operator's flags
		if err != nil {
			return nil, err
		}
		source, file := filepath.Split(key)
		out = append(out, corpusDashboard{key: key, source: strings.TrimSuffix(source, "/"), name: strings.TrimSuffix(file, ".json"), raw: raw})
		found[key] = true
	}
	for key := range sample {
		if !found[key] {
			return nil, fmt.Errorf("sample entry %q is not in the corpus %s", key, root)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no dashboards in the corpus %s", root)
	}
	return out, nil
}

func datasourceKind(dash *spec.Dashboard, ref string) (spec.DatasourceType, bool) {
	if name, ok := spec.DatasourceVariableName(ref); ok {
		for _, v := range dash.Variables {
			if v.Name == name && v.Type == spec.DatasourceVariable {
				return v.DatasourceType, v.DatasourceType != ""
			}
		}
		return "", false
	}
	uidType, ok := stackDatasources[ref]
	return spec.DatasourceType(uidType), ok
}

func queryRefs(dash *spec.Dashboard) []queryRef {
	var refs []queryRef
	for _, entry := range dash.AllPanels() {
		for _, q := range entry.Panel.Queries {
			if kind, ok := datasourceKind(dash, cmp.Or(q.Datasource, entry.Panel.Datasource)); ok && !blank(q.Query) {
				refs = append(refs, queryRef{kind: kind, text: q.Query})
			}
		}
	}
	for _, v := range dash.Variables {
		if v.Type != spec.QueryVariable || blank(v.Query) {
			continue
		}
		if kind, ok := datasourceKind(dash, v.Datasource); ok {
			refs = append(refs, queryRef{kind: kind, text: v.Query, variable: true})
		}
	}
	return refs
}

func analyze(d corpusDashboard) (analyzed, error) {
	dash, err := convertSource(d.raw, stackOptions())
	if err != nil {
		return analyzed{}, fmt.Errorf("%s: %w", d.key, err)
	}
	a := analyzed{corpusDashboard: d, dash: dash}
	kinds, metrics, tables := map[spec.DatasourceType]bool{}, map[string]bool{}, map[string]bool{}
	for _, ref := range queryRefs(dash) {
		kinds[ref.kind] = true
		switch ref.kind {
		case spec.Prometheus:
			a.collectMetrics(ref, metrics)
		case spec.ClickHouse:
			a.collectTables(ref, tables)
		}
	}
	a.kinds = slices.Sorted(maps.Keys(kinds))
	a.metrics, a.tables = slices.Sorted(maps.Keys(metrics)), slices.Sorted(maps.Keys(tables))
	a.titles = dataTitles(dash)
	return a, nil
}

func (a *analyzed) collectMetrics(ref queryRef, into map[string]bool) {
	sels, err := promSelectors(ref)
	if err != nil {
		return
	}
	for _, s := range sels {
		into[s.metric] = true
		if !ref.variable {
			a.offerSubject(subject{kind: spec.Prometheus, name: s.metric})
		}
	}
}

func (a *analyzed) offerSubject(s subject) {
	if a.subject == nil || (a.subject.hostDependent() && s.servedByEveryHost()) {
		a.subject = &s
	}
}

func (s subject) servedByEveryHost() bool {
	return s.kind == spec.Prometheus && hostIndependentMetrics[s.name]
}

func (s subject) hostDependent() bool {
	return s.kind == spec.Prometheus && !hostIndependentMetrics[s.name]
}

func (a *analyzed) collectTables(ref queryRef, into map[string]bool) {
	for _, table := range chTables(ref.text) {
		into[table] = true
		if !ref.variable {
			a.offerSubject(subject{kind: spec.ClickHouse, name: table})
		}
	}
}

func dataTitles(dash *spec.Dashboard) []string {
	var titles []string
	for _, entry := range dash.DataPanels() {
		title := strings.TrimSpace(entry.Panel.Title)
		if title != "" && !slices.Contains(titles, title) {
			titles = append(titles, title)
		}
	}
	return titles
}

func estimatedTokens(inputBytes int) int {
	return int(math.Ceil(float64(inputBytes) / replayBytesPerToken))
}

func planCorpus(root string, sample map[string]bool, suite string) (*planner, error) {
	return planProfile(root, sample, suite, false)
}

func k8sCorpusTasks(root string, sample map[string]bool, suite string) ([]Scenario, error) {
	if suite != suiteLive {
		return nil, errors.New("the k8s profile runs the live suite only")
	}
	if sample == nil {
		sample = k8sDashboardSet()
	}
	for key := range sample {
		if !slices.Contains(k8sDashboards, key) {
			return nil, fmt.Errorf("sample entry %q is not one of the Kubernetes dashboards", key)
		}
	}
	p, err := planProfile(root, sample, suite, true)
	if err != nil {
		return nil, err
	}
	out := make([]Scenario, len(p.tasks))
	for i, t := range p.tasks {
		out[i] = t.scenario
	}
	return out, nil
}

func planProfile(root string, sample map[string]bool, suite string, k8s bool) (*planner, error) {
	if suite != suiteReplay && suite != suiteLive {
		return nil, fmt.Errorf("suite %q must be replay or live", suite)
	}
	dashboards, err := loadCorpus(root, sample)
	if err != nil {
		return nil, err
	}
	p := &planner{suite: suite, k8s: k8s, seen: map[string]bool{}}
	for _, d := range dashboards {
		a, err := analyze(d)
		if err != nil {
			return nil, err
		}
		if k8s {
			if a.mapping, err = k8sMapping(d.raw); err != nil {
				return nil, fmt.Errorf("%s: %w", d.key, err)
			}
		}
		if err := p.dashboard(a); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *planner) dashboard(a analyzed) error {
	builders := []struct {
		name  string
		build func(analyzed) (*task, error)
	}{
		{taskFix, p.fix}, {taskAdd, p.add}, {taskPaste, p.paste}, {taskRebuild, p.rebuild},
		{taskChange, p.change}, {taskRetime, p.retime}, {taskSuggest, p.suggest}, {taskRowedit, p.rowedit},
	}
	for _, b := range builders {
		t, err := b.build(a)
		var skip skipTask
		switch {
		case errors.As(err, &skip):
			p.skipped = append(p.skipped, fmt.Sprintf("%s %s: %s", b.name, a.key, skip.reason))
		case err != nil:
			return fmt.Errorf("%s %s: %w", b.name, a.key, err)
		case t != nil:
			p.tasks = append(p.tasks, *t)
		}
	}
	return nil
}

func (p *planner) id(taskName string, a analyzed) string {
	id := strings.Join([]string{corpusIDPrefix, taskName, slug(a.source), slug(a.name)}, "-")
	if len(id) <= maxIDLength {
		return id
	}
	sum := sha256.Sum256([]byte(a.key))
	suffix := "-" + hex.EncodeToString(sum[:])[:idHashLength]
	return strings.TrimRight(id[:maxIDLength-len(suffix)], "-") + suffix
}

func slug(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func (p *planner) scenario(taskName string, a analyzed, kind, prompt string, marked bool) Scenario {
	id := p.id(taskName, a)
	if marked && p.suite == suiteReplay {
		prompt += " [" + id + "]"
	}
	sc := Scenario{
		ID:     id,
		Prompt: prompt,
		Kind:   kind,
		Suites: []string{suiteReplay, suiteLive},
		Tags:   []string{tagCorpus, taskName},
		Seed:   &Seed{Corpus: a.key},
	}
	if p.k8s {
		sc.Tags = append(sc.Tags, tagK8s)
		sc.Seed.Mapping = a.mapping
	}
	return sc
}

func (p *planner) finish(sc Scenario, fixture *replayFixture) (*task, error) {
	applyDefaults(&sc)
	if err := validateScenario(sc); err != nil {
		return nil, err
	}
	if p.seen[sc.ID] {
		return nil, fmt.Errorf("duplicate task id %q", sc.ID)
	}
	p.seen[sc.ID] = true
	return &task{scenario: sc, fixture: fixture}, nil
}

func (p *planner) fix(a analyzed) (*task, error) {
	broken, err := breakPanel(a.raw)
	if err != nil {
		return nil, skipTask{err.Error()}
	}
	stored, err := convertSource(broken, stackOptions())
	if err != nil {
		return nil, skipTask{err.Error()}
	}
	id, err := changedPanel(spec.Diff(a.dash, stored))
	if err != nil {
		return nil, skipTask{err.Error()}
	}
	entry, err := pickTarget(stored, id)
	if err != nil {
		return nil, skipTask{err.Error()}
	}
	sc := p.scenario(taskFix, a, kindFixUnverified, fixPrompt, true)
	sc.Seed.BreakPanel = true
	var fixture *replayFixture
	if p.suite == suiteReplay {
		prompt, err := fillPrompt(sc, seeded{targetID: entry.Panel.ID, targetTitle: entry.Panel.Title})
		if err != nil {
			return nil, skipTask{err.Error()}
		}
		if err := healTarget(stored, entry.Panel); err != nil {
			return nil, skipTask{err.Error()}
		}
		fixture, err = newFixture(prompt, toolUpdatePanels, panelsInput{Summary: "Point " + entry.Panel.Title + " at a query that returns data", Upsert: []spec.Panel{*entry.Panel}})
		if err != nil {
			return nil, err
		}
	}
	sc.Expect.OnlyTargetChanged, sc.Expect.TargetReturnsData = true, true
	sc.Expect.UnverifiedCleared, sc.Expect.KeepsPanels = true, true
	sc.Expect.MaxSaveOutputTokens = fixSaveTokenBudget
	return p.finish(sc, fixture)
}

func (p *planner) add(a analyzed) (*task, error) {
	if a.subject == nil {
		return nil, skipTask{"no metric or table to name in the prompt"}
	}
	sc := p.scenario(taskAdd, a, kindRevise, "Add a panel showing "+a.subject.name+".", true)
	var fixture *replayFixture
	if p.suite == suiteReplay {
		var err error
		fixture, err = newFixture(sc.Prompt, toolUpdatePanels, panelsInput{Summary: "Add a panel for " + a.subject.name, Upsert: []spec.Panel{addedPanel(a.dash, a.subject.kind)}})
		if err != nil {
			return nil, err
		}
	}
	sc.Expect.KeepsPanels = true
	return p.finish(sc, fixture)
}

func (p *planner) paste(a analyzed) (*task, error) {
	var minified bytes.Buffer
	if err := json.Compact(&minified, a.raw); err != nil {
		return nil, fmt.Errorf("minify the dashboard: %w", err)
	}
	prompt := pasteIntro + " " + minified.String()
	if utf8.RuneCountInString(prompt) > maxPromptRunes {
		sc := p.scenario(taskPaste, a, kindSizeGuard, pasteIntro, false)
		sc.Expect.FailureCode = failurePromptTooLong
		return p.finish(sc, nil)
	}
	sc := p.scenario(taskPaste, a, kindChatImport, pasteIntro, false)
	fixture, err := newFixture(prompt, toolImportGrafana, importInput{JSON: minified.String()})
	if err != nil {
		return nil, err
	}
	return p.finish(sc, fixture)
}

func (p *planner) rebuild(a analyzed) (*task, error) {
	if p.suite != suiteLive && !largeDashboardSet()[a.key] {
		return nil, nil
	}
	if len(a.titles) == 0 || len(a.kinds) == 0 {
		return nil, skipTask{"no titled data panel with a datasource to rebuild from"}
	}
	groups := titleGroups(a.dash)
	titled, _ := groupTotals(groups)
	sc := p.scenario(taskRebuild, a, kindCreate, rebuildPrompt(a.dash.Title, groups), false)
	sc.Seed = nil
	if p.suite == suiteReplay {
		return p.replayRebuild(sc, a)
	}
	sc.Suites = []string{suiteLive}
	sc.Category, sc.Expect.Datasources = categoryOf(a.kinds)
	sc.Expect.MinPanels = max(1, int(math.Ceil(rebuildFloor*float64(titled))))
	sc.Expect.MaxPanels = min(rebuildMaxPanels, titled+(titled+rebuildSlackDivisor-1)/rebuildSlackDivisor)
	sc.Expect.MaxSeconds = int(serverRunLimit.Seconds())
	if slices.Contains(a.kinds, spec.Prometheus) {
		sc.Expect.Metrics = a.metrics
	}
	if slices.Contains(a.kinds, spec.ClickHouse) {
		sc.Expect.Tables = a.tables
	}
	if len(sc.Expect.Metrics) > 0 || len(sc.Expect.Tables) > 0 {
		sc.Expect.RebuildMetricShare = rebuildMetricShare
		sc.Expect.RebuildMetricCap = rebuildMetricCap
	}
	return p.finish(sc, nil)
}

func (p *planner) replayRebuild(sc Scenario, a analyzed) (*task, error) {
	sc.Suites = []string{suiteReplay}
	fixture, shape, err := rebuildFixture(a)
	if err != nil {
		return nil, err
	}
	if shape.panels < minRebuildPanels {
		return nil, skipTask{fmt.Sprintf("only %d panels survive a one-panel save", shape.panels)}
	}
	sc.Category, sc.Expect.Datasources = categoryOf(shape.kinds)
	sc.Expect.MinPanels, sc.Expect.MaxPanels = shape.panels, shape.panels
	fixture.Prompt = sc.Prompt
	return p.finish(sc, fixture)
}

type titleGroup struct {
	Row    string
	Titles []string
}

func titleGroups(dash *spec.Dashboard) []titleGroup {
	groups := []titleGroup{{}}
	index := map[string]int{}
	seen := map[string]bool{}
	for _, entry := range dash.DataPanels() {
		title := strings.TrimSpace(entry.Panel.Title)
		if title == "" || seen[title] {
			continue
		}
		seen[title] = true
		at := 0
		if entry.Row != nil {
			var ok bool
			if at, ok = index[entry.Row.ID]; !ok {
				groups = append(groups, titleGroup{Row: rowTitle(entry.Row)})
				at = len(groups) - 1
				index[entry.Row.ID] = at
			}
		}
		groups[at].Titles = append(groups[at].Titles, title)
	}
	return slices.DeleteFunc(groups, func(g titleGroup) bool { return len(g.Titles) == 0 })
}

func rowTitle(row *spec.Panel) string {
	if title := strings.TrimSpace(row.Title); title != "" {
		return title
	}
	return "Row " + row.ID
}

func groupTotals(groups []titleGroup) (panels, rows int) {
	for _, g := range groups {
		panels += len(g.Titles)
		if g.Row != "" {
			rows++
		}
	}
	return panels, rows
}

func rebuildPrompt(title string, groups []titleGroup) string {
	panels, rows := groupTotals(groups)
	for keep := panels; ; keep-- {
		prompt := listRebuild(title, panels, rows, groups, keep)
		if keep <= 1 || utf8.RuneCountInString(prompt) <= maxPromptRunes {
			return prompt
		}
	}
}

func listRebuild(title string, panels, rows int, groups []titleGroup, keep int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Build a dashboard titled %q with %d panels in %d rows.", title, panels, rows)
	for _, g := range groups {
		if keep <= 0 {
			break
		}
		titles := g.Titles[:min(len(g.Titles), keep)]
		keep -= len(titles)
		if g.Row == "" {
			fmt.Fprintf(&b, " Above the rows: %s.", strings.Join(titles, "; "))
			continue
		}
		fmt.Fprintf(&b, " Row %q: %s.", g.Row, strings.Join(titles, "; "))
	}
	return b.String()
}

func categoryOf(kinds []spec.DatasourceType) (string, []string) {
	datasources := make([]string, len(kinds))
	for i, k := range kinds {
		datasources[i] = string(k)
	}
	switch {
	case len(kinds) > 1:
		return categoryMixed, datasources
	case kinds[0] == spec.ClickHouse:
		return categoryClickHouse, datasources
	}
	return categoryPrometheus, datasources
}

func healTarget(dash *spec.Dashboard, panel *spec.Panel) error {
	healed := 0
	for i := range panel.Queries {
		q := &panel.Queries[i]
		kind, ok := datasourceKind(dash, cmp.Or(q.Datasource, panel.Datasource))
		if blank(q.Query) || !ok {
			continue
		}
		*q = healthyQuery(kind, *q)
		healed++
	}
	if healed == 0 {
		return skipTask{"the target panel has no query on a datasource the stack knows"}
	}
	panel.Interval = ""
	return nil
}

func healthyQuery(kind spec.DatasourceType, replaced spec.Query) spec.Query {
	healthy := spec.Query{Ref: replaced.Ref, Datasource: replaced.Datasource, Hide: replaced.Hide}
	if kind == spec.ClickHouse {
		healthy.Query = healthyCHQuery
		return healthy
	}
	healthy.Query, healthy.Instant = healthyPromQuery, true
	return healthy
}

func addedPanel(dash *spec.Dashboard, kind spec.DatasourceType) spec.Panel {
	id, bottom := addedPanelID, 0
	taken := map[string]bool{}
	for _, entry := range dash.AllPanels() {
		taken[entry.Panel.ID] = true
		bottom = max(bottom, entry.Panel.GridPos.Y+entry.Panel.GridPos.H)
	}
	for n := 2; taken[id]; n++ {
		id = fmt.Sprintf("%s-%d", addedPanelID, n)
	}
	panel := spec.Panel{
		ID:      id,
		Type:    spec.Timeseries,
		Title:   addedPanelTitle,
		GridPos: spec.GridPos{X: 0, Y: bottom, W: addedPanelWidth, H: addedPanelRows},
		Queries: []spec.Query{healthyQuery(kind, spec.Query{})},
	}
	panel.Datasource = "prom"
	if kind == spec.ClickHouse {
		panel.Type, panel.Datasource = spec.Table, "ch"
	}
	return panel
}

func (p *planner) change(a analyzed) (*task, error) {
	if !largeDashboardSet()[a.key] {
		return nil, nil
	}
	entry, ok := pickWorkTarget(a)
	if !ok {
		return nil, skipTask{"no titled data panel with a query that is verified at import"}
	}
	newType, phrase := spec.Stat, "a stat panel"
	if entry.Panel.Type == spec.Stat {
		newType, phrase = spec.Timeseries, "a time series"
	}
	sc := p.scenario(taskChange, a, kindRevise, `Show the panel "`+placeholderPanelTitle+`" (`+placeholderPanelID+`) as `+phrase+`.`, true)
	sc.Seed.Target = entry.Panel.ID
	sc.Expect.OnlyTargetChanged, sc.Expect.TargetReturnsData = true, true
	sc.Expect.TargetType, sc.Expect.MaxSeconds = string(newType), changeSeconds
	var fixture *replayFixture
	if p.suite == suiteReplay {
		prompt, err := fillPrompt(sc, seeded{targetID: entry.Panel.ID, targetTitle: entry.Panel.Title})
		if err != nil {
			return nil, skipTask{err.Error()}
		}
		panel := *entry.Panel
		panel.Queries = slices.Clone(panel.Queries)
		if err := healTarget(a.dash, &panel); err != nil {
			return nil, skipTask{err.Error()}
		}
		panel.Type, panel.Display = newType, nil
		if fixture, err = newFixture(prompt, toolUpdatePanels, panelsInput{Summary: "Show " + panel.Title + " as " + phrase, Upsert: []spec.Panel{panel}, Finish: true}); err != nil {
			return nil, err
		}
		if err := appliesTo(a.dash, fixture); err != nil {
			return nil, skipTask{err.Error()}
		}
	}
	return p.finish(sc, fixture)
}

type retimeRange struct {
	from, phrase string
	span         time.Duration
}

var retimeRanges = []retimeRange{
	{"now-6h", "6 hours", 6 * time.Hour},
	{"now-3h", "3 hours", 3 * time.Hour},
	{"now-1h", "hour", time.Hour},
	{"now-15m", "15 minutes", 15 * time.Minute},
}

// The server rejects a save whose panel is empty at the new range, and the stack holds only minutes of history, so a query step above what it holds has no point with data.
func pickRetimeRange(dash *spec.Dashboard) (retimeRange, bool) {
	factor := 1
	for _, entry := range dash.DataPanels() {
		for _, q := range entry.Panel.Queries {
			factor = max(factor, q.IntervalFactor)
		}
	}
	i := slices.IndexFunc(retimeRanges, func(r retimeRange) bool {
		return r.from != dash.Time.From && r.span/retimeDataPoints*time.Duration(factor) <= retimeMaxStep
	})
	if i < 0 {
		return retimeRange{}, false
	}
	return retimeRanges[i], true
}

func (p *planner) retime(a analyzed) (*task, error) {
	if !largeDashboardSet()[a.key] {
		return nil, nil
	}
	target, ok := pickRetimeRange(a.dash)
	if !ok {
		return nil, skipTask{"no default range other than " + a.dash.Time.From + " whose query steps the stack's history covers"}
	}
	sc := p.scenario(taskRetime, a, kindRevise, "Default this dashboard to the last "+target.phrase+".", true)
	sc.Expect.TimeRangeFrom, sc.Expect.ChangedPaths = []string{target.from}, []string{"/time"}
	sc.Expect.MaxSaveOutputTokens, sc.Expect.MaxSeconds, sc.Expect.MaxModelCalls = fixSaveTokenBudget, retimeSeconds, retimeModelCalls
	var fixture *replayFixture
	if p.suite == suiteReplay {
		var err error
		patch := map[string]any{"summary": "Default to the last " + target.phrase, "dashboard": map[string]any{"time": map[string]string{"from": target.from, "to": "now"}}}
		if fixture, err = newFixture(sc.Prompt, toolUpdatePanels, patch); err != nil {
			return nil, err
		}
		if err := appliesTo(a.dash, fixture); err != nil {
			return nil, skipTask{err.Error()}
		}
	}
	return p.finish(sc, fixture)
}

func (p *planner) suggest(a analyzed) (*task, error) {
	if !largeDashboardSet()[a.key] {
		return nil, nil
	}
	entry, ok := pickWorkTarget(a)
	if !ok {
		return nil, skipTask{"no titled data panel with a query that is verified at import"}
	}
	sc := p.scenario(taskSuggest, a, kindSuggest, suggestPrompt, true)
	sc.Seed.Target = entry.Panel.ID
	var fixture *replayFixture
	if p.suite == suiteReplay {
		panel := *entry.Panel
		panel.Title += " (renamed)"
		var err error
		if fixture, err = newFixture(sc.Prompt, toolUpdatePanels, panelsInput{Summary: "Name the panel for what it measures", Upsert: []spec.Panel{panel}}); err != nil {
			return nil, err
		}
		if err := appliesTo(a.dash, fixture); err != nil {
			return nil, skipTask{err.Error()}
		}
	}
	return p.finish(sc, fixture)
}

func (p *planner) rowedit(a analyzed) (*task, error) {
	if !largeDashboardSet()[a.key] {
		return nil, nil
	}
	row, ok := pickWorkRow(a)
	if !ok {
		return nil, skipTask{fmt.Sprintf("no row with %d to %d editable panels that are all verified at import", minRowChildren, maxRowChildren)}
	}
	sc := p.scenario(taskRowedit, a, kindRevise, `Add the description "`+rowEditDescription+`" to every panel in the row "`+placeholderRowTitle+`".`, true)
	sc.Seed.Row = row.ID
	sc.Expect.ChangedPanels, sc.Expect.MaxSeconds = len(row.Panels), rowEditSeconds
	var fixture *replayFixture
	if p.suite == suiteReplay {
		prompt, err := fillPrompt(sc, seeded{rowTitle: rowTitle(row)})
		if err != nil {
			return nil, skipTask{err.Error()}
		}
		fixture = &replayFixture{Prompt: prompt}
		for i, child := range row.Panels {
			child.Description = rowEditDescription
			one, err := newFixture(prompt, toolUpdatePanels, panelsInput{Summary: "Describe " + child.Title, Upsert: []spec.Panel{child}, Finish: i == len(row.Panels)-1})
			if err != nil {
				return nil, err
			}
			fixture.Turns = append(fixture.Turns, one.Turns...)
		}
		if err := appliesTo(a.dash, fixture); err != nil {
			return nil, skipTask{err.Error()}
		}
	}
	for _, child := range row.Panels {
		path := "/panels/" + pointerToken(row.ID) + "/panels/" + pointerToken(child.ID)
		if p.suite == suiteLive {
			path += "/description"
		}
		sc.Expect.ChangedPaths = append(sc.Expect.ChangedPaths, path)
	}
	return p.finish(sc, fixture)
}

func pointerToken(id string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(id)
}

func appliesTo(dash *spec.Dashboard, f *replayFixture) error {
	current := dash
	for i, turn := range f.Turns {
		var patch spec.PanelPatch
		if err := json.Unmarshal(turn.ToolCalls[0].Input, &patch); err != nil {
			return fmt.Errorf("turn %d is not a panel patch: %w", i, err)
		}
		next, err := spec.ApplyPanelPatch(current, patch)
		if err != nil {
			return fmt.Errorf("turn %d does not apply to the stored dashboard: %w", i, err)
		}
		current = next
	}
	return nil
}

func pickWorkTarget(a analyzed) (spec.PanelEntry, bool) {
	entries := slices.DeleteFunc(workPanels(a), func(e spec.PanelEntry) bool { return !verifiedAtImport(a.key, e.Panel.ID) })
	if i := slices.IndexFunc(entries, func(e spec.PanelEntry) bool { return hostIndependent(a.dash, e.Panel) }); i >= 0 {
		return entries[i], true
	}
	if len(entries) > 0 {
		return entries[0], true
	}
	return spec.PanelEntry{}, false
}

func pickWorkRow(a analyzed) (*spec.Panel, bool) {
	var rows []*spec.Panel
	for i := range a.dash.Panels {
		row := &a.dash.Panels[i]
		if row.Type == spec.Row && len(row.Panels) >= minRowChildren && len(row.Panels) <= maxRowChildren && editableChildren(row) && childrenVerified(a.key, row) {
			rows = append(rows, row)
		}
	}
	if i := slices.IndexFunc(rows, func(row *spec.Panel) bool { return childrenHostIndependent(a.dash, row) }); i >= 0 {
		return rows[i], true
	}
	if len(rows) > 0 {
		return rows[0], true
	}
	return nil, false
}

func childrenVerified(key string, row *spec.Panel) bool {
	return !slices.ContainsFunc(row.Panels, func(child spec.Panel) bool { return !verifiedAtImport(key, child.ID) })
}

func editableChildren(row *spec.Panel) bool {
	return !slices.ContainsFunc(row.Panels, func(child spec.Panel) bool {
		return child.Type == spec.Placeholder || child.Type == spec.Row || child.Type == spec.Canvas || hasActions(&child)
	})
}

func childrenHostIndependent(dash *spec.Dashboard, row *spec.Panel) bool {
	for i := range row.Panels {
		child := &row.Panels[i]
		if child.Type.IsData() && len(child.Queries) > 0 && !hostIndependent(dash, child) {
			return false
		}
	}
	return true
}

func workPanels(a analyzed) []spec.PanelEntry {
	return slices.DeleteFunc(a.dash.DataPanels(), func(e spec.PanelEntry) bool {
		return blank(e.Panel.Title) || e.Panel.Type == spec.Canvas || hasActions(e.Panel) ||
			!slices.ContainsFunc(e.Panel.Queries, func(q spec.Query) bool { return !blank(q.Query) })
	})
}

func hostIndependent(dash *spec.Dashboard, panel *spec.Panel) bool {
	found := false
	for _, q := range panel.Queries {
		kind, ok := datasourceKind(dash, cmp.Or(q.Datasource, panel.Datasource))
		if !ok || kind != spec.Prometheus || blank(q.Query) {
			continue
		}
		sels, err := exprSelectors(q.Query)
		if err != nil {
			return false
		}
		for _, s := range sels {
			if !hostIndependentMetrics[s.metric] {
				return false
			}
			found = true
		}
	}
	return found
}
