package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/grafana"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var corpusDir = filepath.Join("pkg", "grafana", "testdata", "corpus")

var panelBreaker = func([]byte) ([]byte, error) {
	return nil, errors.New("no panel breaker is wired in")
}

type seeded struct {
	dashboardID    string
	version        int
	targetID       string
	targetTitle    string
	targetVerified bool
	rowTitle       string

	variables []string
}

type brokenSource struct {
	json     []byte
	targetID string
}

func readCorpus(corpus string) ([]byte, error) {
	if !filepath.IsLocal(corpus) {
		return nil, fmt.Errorf("corpus path %q must stay inside the corpus", corpus)
	}
	return os.ReadFile(filepath.Join(corpusDir, corpus)) //nolint:gosec // the path is confined to the corpus by filepath.IsLocal
}

func importOptions(types map[string]string, mapping map[string]string) grafana.Options {
	uids := make([]string, 0, len(types))
	for uid := range types {
		uids = append(uids, uid)
	}
	slices.Sort(uids)
	opts := grafana.Options{Mapping: mapping}
	for _, uid := range uids {
		opts.Datasources = append(opts.Datasources, grafana.Datasource{UID: uid, Type: types[uid]})
	}
	return opts
}

func convertSource(source []byte, opts grafana.Options) (*spec.Dashboard, error) {
	result, err := grafana.Convert(source, opts)
	if err != nil {
		return nil, err
	}
	if !result.Ready() || result.Spec == nil {
		return nil, errors.New("the dashboard needs a datasource mapping the stack cannot satisfy")
	}
	return result.Spec, nil
}

func breakSourceFor(ctx context.Context, c *client, seed Seed) (brokenSource, error) {
	source, err := readCorpus(seed.Corpus)
	if err != nil {
		return brokenSource{}, err
	}
	broken, err := panelBreaker(source)
	if err != nil {
		return brokenSource{}, fmt.Errorf("break a panel of %s: %w", seed.Corpus, err)
	}
	types, err := c.datasourceTypes(ctx)
	if err != nil {
		return brokenSource{}, fmt.Errorf("read the datasources: %w", err)
	}
	opts := importOptions(types, seed.Mapping)
	pristine, err := convertSource(source, opts)
	if err != nil {
		return brokenSource{}, fmt.Errorf("convert %s: %w", seed.Corpus, err)
	}
	changed, err := convertSource(broken, opts)
	if err != nil {
		return brokenSource{}, fmt.Errorf("convert the broken %s: %w", seed.Corpus, err)
	}
	id, err := changedPanel(spec.Diff(pristine, changed))
	if err != nil {
		return brokenSource{}, fmt.Errorf("break a panel of %s: %w", seed.Corpus, err)
	}
	return brokenSource{json: broken, targetID: id}, nil
}

func changedPanel(changes []spec.Change) (string, error) {
	if len(changes) == 0 {
		return "", errors.New("the break changed nothing")
	}
	var id string
	for _, change := range changes {
		got, ok := panelOfPath(change.Path)
		switch {
		case !ok:
			return "", fmt.Errorf("the break changed %s, which is not inside one panel", change.Path)
		case id != "" && got != id:
			return "", fmt.Errorf("the break changed panels %s and %s, want one", id, got)
		}
		id = got
	}
	return id, nil
}

func panelOfPath(path string) (string, bool) {
	tokens := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(tokens) < 2 || tokens[0] != "panels" {
		return "", false
	}
	id := tokens[1]
	if len(tokens) >= 4 && tokens[2] == "panels" {
		id = tokens[3]
	}
	return strings.NewReplacer("~1", "/", "~0", "~").Replace(id), true
}

func seedScenario(ctx context.Context, c *client, sc Scenario) (seeded, error) {
	if sc.Seed == nil {
		return seeded{}, errors.New("the scenario has no seed")
	}
	source, err := readCorpus(sc.Seed.Corpus)
	if err != nil {
		return seeded{}, err
	}
	targetID := sc.Seed.Target
	if sc.Seed.BreakPanel {
		broken, err := breakSourceFor(ctx, c, *sc.Seed)
		if err != nil {
			return seeded{}, err
		}
		source, targetID = broken.json, broken.targetID
	}
	imported, err := c.createImport(ctx, source, sc.Seed.Mapping)
	if err != nil {
		return seeded{}, fmt.Errorf("import %s: %w", sc.Seed.Corpus, err)
	}
	dash, err := spec.Parse(imported.Spec)
	if err != nil {
		return seeded{}, fmt.Errorf("the imported %s does not parse: %w", sc.Seed.Corpus, err)
	}
	entry, err := pickTarget(dash, targetID)
	if err != nil {
		return seeded{}, fmt.Errorf("%s: %w", sc.Seed.Corpus, err)
	}
	s := seeded{
		dashboardID:    imported.DashboardID,
		version:        imported.Version,
		targetID:       entry.Panel.ID,
		targetTitle:    entry.Panel.Title,
		targetVerified: !imported.Unverified.hasPanel(entry.Panel.ID),

		variables: imported.Unverified.variableNames(),
	}
	if sc.Seed.Target != "" && !s.targetVerified {
		return seeded{}, fmt.Errorf("the target panel %s was unverified at import", s.targetID)
	}
	if sc.Seed.Row != "" {
		if s.rowTitle, err = rowOfImport(dash, imported.Unverified, sc.Seed.Row); err != nil {
			return seeded{}, err
		}
	}
	return s, nil
}

func rowOfImport(dash *spec.Dashboard, unverified Unverified, id string) (string, error) {
	entry, ok := dash.FindPanel(id)
	if !ok || entry.Row != nil || entry.Panel.Type != spec.Row {
		return "", fmt.Errorf("the row %s is not a row of the import", id)
	}
	for i := range entry.Panel.Panels {
		if child := entry.Panel.Panels[i]; unverified.hasPanel(child.ID) {
			return "", fmt.Errorf("the panel %s of the row %s was unverified at import", child.ID, id)
		}
	}
	return rowTitle(entry.Panel), nil
}

func unlistableVariable(ctx context.Context, c *client, s seeded) string {
	for _, name := range s.variables {
		values, err := c.getVariableValues(ctx, s.dashboardID, s.version, name)
		var api *apiError
		switch {
		case errors.As(err, &api) && api.Status == http.StatusUnprocessableEntity:
			return fmt.Sprintf("variable %s: %s", name, err)
		case err == nil && len(values) == 0:
			return fmt.Sprintf("variable %s has no values", name)
		}
	}
	return ""
}

func pickTarget(dash *spec.Dashboard, id string) (spec.PanelEntry, error) {
	if id != "" {
		entry, ok := dash.FindDataPanel(id)
		if !ok {
			return spec.PanelEntry{}, fmt.Errorf("the target panel %s is not a data panel of the import", id)
		}
		return entry, nil
	}
	entries := dash.DataPanels()
	if len(entries) == 0 {
		return spec.PanelEntry{}, errors.New("the import has no data panel to target")
	}
	return entries[0], nil
}

func fillPrompt(sc Scenario, s seeded) (string, error) {
	if sc.Kind == kindChatImport || sc.Kind == kindSizeGuard {
		return chatImportPrompt(sc)
	}
	prompt := strings.NewReplacer(placeholderPanelID, s.targetID, placeholderPanelTitle, s.targetTitle, placeholderRowTitle, s.rowTitle).Replace(sc.Prompt)
	if n := utf8.RuneCountInString(prompt); n > maxPromptRunes {
		return "", fmt.Errorf("the prompt is %d characters after filling the panel placeholders, over the %d limit", n, maxPromptRunes)
	}
	return prompt, nil
}

func chatImportPrompt(sc Scenario) (string, error) {
	if sc.Seed == nil {
		return sc.Prompt, nil
	}
	source, err := readCorpus(sc.Seed.Corpus)
	if err != nil {
		return "", err
	}
	var minified bytes.Buffer
	if err := json.Compact(&minified, source); err != nil {
		return "", fmt.Errorf("minify %s: %w", sc.Seed.Corpus, err)
	}
	return strings.TrimSpace(sc.Prompt) + " " + minified.String(), nil
}

const (
	k8sDatasourceUID   = "prom_k8s"
	defaultK8sGapsFile = "evals/k8s-gaps.yaml"
	maxK8sGapShare     = 0.15

	defaultK8sShopPort  = "38081"
	defaultK8sShop1Port = "38082"
)

var k8sDatasources = map[string]string{
	"prom":           string(spec.Prometheus),
	"ch":             string(spec.ClickHouse),
	k8sDatasourceUID: string(spec.Prometheus),
}

type k8sGap struct {
	Dashboard  string `yaml:"dashboard"`
	PanelTitle string `yaml:"panel_title"`
	Reason     string `yaml:"reason"`
}

type k8sGapFile struct {
	Gaps []k8sGap `yaml:"gaps"`
}

type k8sResult struct {
	dashboard  string
	panels     int
	unverified []string
}

func k8sMapping(source []byte) (map[string]string, error) {
	result, err := grafana.Convert(source, importOptions(k8sDatasources, nil))
	if err != nil {
		return nil, err
	}
	mapping := map[string]string{}
	for _, ds := range result.Datasources {
		if ds.Kind != nil && *ds.Kind == string(spec.Prometheus) {
			mapping[ds.Key] = k8sDatasourceUID
		}
	}
	return mapping, nil
}

func k8sShopURLs(getenv func(string) string) map[string]string {
	port := func(name, fallback string) string {
		if v := getenv(name); v != "" {
			return v
		}
		return fallback
	}
	return map[string]string{
		"checkout-0": "http://127.0.0.1:" + port("SLASHOBSERVE_DEMO_K8S_SHOP_PORT", defaultK8sShopPort),
		"checkout-1": "http://127.0.0.1:" + port("SLASHOBSERVE_DEMO_K8S_SHOP1_PORT", defaultK8sShop1Port),
	}
}

func shopURLsFor(profile string, flagged shopURLs, getenv func(string) string) map[string]string {
	if profile != profileK8s {
		return resolveShopURLs(flagged, getenv)
	}
	urls := k8sShopURLs(getenv)
	for pod, base := range flagged {
		urls[pod] = base
	}
	return urls
}

func loadK8sGaps(path string) ([]k8sGap, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path comes from the operator's flags
	if err != nil {
		return nil, err
	}
	var file k8sGapFile
	if err := decodeStrict(content, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	seen := map[[2]string]bool{}
	for _, gap := range file.Gaps {
		key := [2]string{gap.Dashboard, gap.PanelTitle}
		switch {
		case !slices.Contains(k8sDashboards, gap.Dashboard):
			return nil, fmt.Errorf("%s: gap dashboard %q is not one of the Kubernetes dashboards", path, gap.Dashboard)
		case blank(gap.PanelTitle) || blank(gap.Reason):
			return nil, fmt.Errorf("%s: the gap for %s needs a panel_title and a reason", path, gap.Dashboard)
		case seen[key]:
			return nil, fmt.Errorf("%s: %s panel %q is listed twice", path, gap.Dashboard, gap.PanelTitle)
		}
		seen[key] = true
	}
	return file.Gaps, nil
}

func compareK8s(results []k8sResult, gaps []k8sGap) (problems, warnings []string) {
	panels := 0
	for _, r := range results {
		panels += r.panels
		for _, title := range r.unverified {
			if !slices.ContainsFunc(gaps, func(g k8sGap) bool { return g.Dashboard == r.dashboard && g.PanelTitle == title }) {
				problems = append(problems, fmt.Sprintf("%s: panel %q is unverified and not listed in the gaps", r.dashboard, title))
			}
		}
	}
	for _, g := range gaps {
		i := slices.IndexFunc(results, func(r k8sResult) bool { return r.dashboard == g.Dashboard })
		if i < 0 || !slices.Contains(results[i].unverified, g.PanelTitle) {
			warnings = append(warnings, fmt.Sprintf("%s: panel %q is listed as a gap but verifies", g.Dashboard, g.PanelTitle))
		}
	}
	if float64(len(gaps)) > maxK8sGapShare*float64(panels) {
		problems = append(problems, fmt.Sprintf("%d gaps are more than %.0f%% of the %d data panels", len(gaps), maxK8sGapShare*100, panels))
	}
	return problems, warnings
}

func verifyK8s(ctx context.Context, c *client, gaps []k8sGap) (problems, warnings []string, err error) {
	results := make([]k8sResult, 0, len(k8sDashboards))
	for _, key := range k8sDashboards {
		result, err := importK8s(ctx, c, key)
		if err != nil {
			return nil, nil, err
		}
		results = append(results, result)
	}
	problems, warnings = compareK8s(results, gaps)
	return problems, warnings, nil
}

func importK8s(ctx context.Context, c *client, key string) (k8sResult, error) {
	source, err := readCorpus(key)
	if err != nil {
		return k8sResult{}, err
	}
	mapping, err := k8sMapping(source)
	if err != nil {
		return k8sResult{}, fmt.Errorf("map %s: %w", key, err)
	}
	imported, err := c.createImport(ctx, source, mapping)
	if err != nil {
		return k8sResult{}, fmt.Errorf("import %s: %w", key, err)
	}
	dash, err := spec.Parse(imported.Spec)
	if err != nil {
		return k8sResult{}, fmt.Errorf("the imported %s does not parse: %w", key, err)
	}
	result := k8sResult{dashboard: key, panels: len(dash.DataPanels())}
	for _, entry := range dash.AllPanels() {
		title := strings.TrimSpace(entry.Panel.Title)
		if imported.Unverified.hasPanel(entry.Panel.ID) && !slices.Contains(result.unverified, title) {
			result.unverified = append(result.unverified, title)
		}
	}
	return result, nil
}
