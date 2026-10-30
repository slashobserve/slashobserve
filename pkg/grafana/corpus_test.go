package grafana_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/grafana"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var update = flag.Bool("update", false, "rewrite goldens")

const (
	corpusDir               = "corpus"
	goldenDir               = "golden"
	minimumRatio            = 90
	minimumFull             = 72.6
	fullSlack               = 0.5
	maximumOpenPlaceholders = 8
	worklistSize            = 10
	maxLinePaths            = 100
	tooManyPanels           = "too_many_panels"
)

var panelIndex = regexp.MustCompile(`\[\d+\]`)

var corpusOptions = grafana.Options{Datasources: []grafana.Datasource{
	{UID: "prom", Type: "prometheus"},
	{UID: "ch", Type: "clickhouse"},
}}

type corpusFile struct {
	source string
	name   string
	raw    []byte
}

func (f corpusFile) id() string { return f.source + "/" + f.name }

func testdata(t *testing.T) *os.Root {
	t.Helper()
	root, err := os.OpenRoot("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func corpusFiles(t *testing.T) []corpusFile {
	t.Helper()
	root := testdata(t)
	paths, err := fs.Glob(root.FS(), corpusDir+"/*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	files := make([]corpusFile, 0, len(paths))
	for _, p := range paths {
		raw, err := root.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, corpusFile{source: filepath.Base(filepath.Dir(p)), name: filepath.Base(p), raw: raw})
	}
	if len(files) == 0 {
		t.Fatal("the corpus is empty")
	}
	return files
}

type outcome struct {
	res       *grafana.Result
	inputCode string
}

func convertFile(t *testing.T, f corpusFile) outcome {
	t.Helper()
	res, err := grafana.Convert(f.raw, corpusOptions)
	var input *grafana.InputError
	switch {
	case errors.As(err, &input):
		return outcome{inputCode: input.Code}
	case err != nil:
		t.Fatalf("%s: %v", f.id(), err)
	}
	return outcome{res: res}
}

type goldenFailure struct {
	InputError string `json:"input_error"`
}

type goldenResult struct {
	InputError  *string                    `json:"input_error"`
	Summary     grafana.Summary            `json:"summary"`
	Datasources []grafana.SourceDatasource `json:"datasources"`
	Report      grafana.Report             `json:"report"`
	Spec        *spec.Dashboard            `json:"spec"`
}

var importerVersion = regexp.MustCompile(`"importer_version": \d+`)

func goldenBytes(t *testing.T, o outcome) []byte {
	t.Helper()
	var doc any = goldenFailure{InputError: o.inputCode}
	if o.res != nil {
		doc = goldenResult{Summary: o.res.Summary, Datasources: o.res.Datasources, Report: o.res.Report, Spec: o.res.Spec}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		t.Fatal(err)
	}
	return importerVersion.ReplaceAll(buf.Bytes(), []byte(`"importer_version": "<version>"`))
}

func TestCorpusGoldens(t *testing.T) {
	for _, f := range corpusFiles(t) {
		t.Run(f.id(), func(t *testing.T) {
			got := goldenBytes(t, convertFile(t, f))
			root := testdata(t)
			path := filepath.Join(goldenDir, f.source, f.name)
			if *update {
				if err := root.MkdirAll(filepath.Dir(path), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := root.WriteFile(path, got, 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := root.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run make golden): %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("golden %s differs from the converter output; run make golden and review the diff", path)
			}
		})
	}
}

func TestCorpusGoldensHaveSources(t *testing.T) {
	root := testdata(t)
	goldens, err := fs.Glob(root.FS(), goldenDir+"/*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	inCorpus := map[string]bool{}
	for _, f := range corpusFiles(t) {
		inCorpus[f.source+"/"+f.name] = true
	}
	for _, g := range goldens {
		if id := filepath.Base(filepath.Dir(g)) + "/" + filepath.Base(g); !inCorpus[id] {
			t.Errorf("golden %s has no corpus file %s", g, id)
		}
	}
}

type sourcePanel struct {
	path string
	raw  map[string]any
}

var staticSourceTypes = map[string]bool{"text": true, "dashlist": true, "news": true}

func (p sourcePanel) targets() []map[string]any {
	if kind, _ := p.raw["type"].(string); staticSourceTypes[kind] {
		return nil
	}
	items, _ := p.raw["targets"].([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		target, _ := item.(map[string]any)
		out = append(out, target)
	}
	return out
}

func decodeDashboard(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")), &doc); err != nil {
		t.Fatal(err)
	}
	_, panels := doc["panels"].([]any)
	_, rows := doc["rows"].([]any)
	if inner, ok := doc["dashboard"].(map[string]any); ok && !panels && !rows {
		return inner
	}
	return doc
}

func isRow(p map[string]any) bool { return p["type"] == "row" }

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	}
	return false
}

func present(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

func isClone(p map[string]any) bool {
	return p["repeatPanelId"] != nil || p["repeatRowId"] != nil || truthy(p["repeatedByRow"])
}

func isLegacyCloneRow(row map[string]any) bool {
	return isClone(row) || (row["repeatIteration"] != nil && !present(row["repeat"]))
}

func gridCoordinate(p map[string]any, key string) float64 {
	pos, _ := p["gridPos"].(map[string]any)
	f, _ := pos[key].(float64)
	return math.Floor(f)
}

func expectedPanels(t *testing.T, raw []byte) []sourcePanel {
	t.Helper()
	dash := decodeDashboard(t, raw)
	items, _ := dash["panels"].([]any)
	rows, _ := dash["rows"].([]any)
	if len(items) == 0 && len(rows) > 0 {
		return expectedLegacyPanels(rows)
	}
	var tops []sourcePanel
	for i, item := range items {
		if p, ok := item.(map[string]any); ok {
			tops = append(tops, sourcePanel{fmt.Sprintf("panels[%d]", i), p})
		}
	}
	sort.SliceStable(tops, func(a, b int) bool {
		ya, yb := gridCoordinate(tops[a].raw, "y"), gridCoordinate(tops[b].raw, "y")
		if ya != yb {
			return ya < yb
		}
		return gridCoordinate(tops[a].raw, "x") < gridCoordinate(tops[b].raw, "x")
	})
	var out []sourcePanel
	inCloneRow := false
	for _, n := range tops {
		collapsed := truthy(n.raw["collapsed"])
		if isRow(n.raw) {
			inCloneRow = isClone(n.raw) && !collapsed
			if isClone(n.raw) || !collapsed {
				continue
			}
			children, _ := n.raw["panels"].([]any)
			for j, child := range children {
				if c, ok := child.(map[string]any); ok && !isRow(c) && !isClone(c) {
					out = append(out, sourcePanel{fmt.Sprintf("%s.panels[%d]", n.path, j), c})
				}
			}
			continue
		}
		if !inCloneRow && !isClone(n.raw) {
			out = append(out, n)
		}
	}
	return out
}

func expectedLegacyPanels(rows []any) []sourcePanel {
	var out []sourcePanel
	for i, r := range rows {
		row, ok := r.(map[string]any)
		if !ok || isLegacyCloneRow(row) {
			continue
		}
		items, _ := row["panels"].([]any)
		for j, item := range items {
			if p, ok := item.(map[string]any); ok && !isRow(p) && !isClone(p) {
				out = append(out, sourcePanel{fmt.Sprintf("rows[%d].panels[%d]", i, j), p})
			}
		}
	}
	return out
}

func TestCorpusInvariants(t *testing.T) {
	for _, f := range corpusFiles(t) {
		t.Run(f.id(), func(t *testing.T) {
			first := convertFile(t, f)
			if first.res == nil {
				if first.inputCode != tooManyPanels {
					t.Fatalf("input error %s", first.inputCode)
				}
				return
			}
			res := first.res
			t.Run("spec parses", func(t *testing.T) { checkSpecParses(t, res) })
			t.Run("one line per panel and target", func(t *testing.T) { checkLines(t, f, res) })
			t.Run("statuses carry reasons and paths", func(t *testing.T) { checkStatuses(t, res) })
			t.Run("spec panels and lines agree", func(t *testing.T) { checkPanelIDs(t, res) })
			t.Run("query text round trips", func(t *testing.T) { checkQueryText(t, f, res) })
			t.Run("panel order follows the grid", func(t *testing.T) { checkOrder(t, f, res) })
			t.Run("units are known", func(t *testing.T) { checkUnits(t, res) })
			t.Run("conversion is deterministic", func(t *testing.T) {
				if !bytes.Equal(goldenBytes(t, first), goldenBytes(t, convertFile(t, f))) {
					t.Error("two conversions of the same bytes differ")
				}
			})
		})
	}
}

func checkSpecParses(t *testing.T, res *grafana.Result) {
	t.Helper()
	if res.Spec == nil {
		t.Fatal("Result.Spec is nil although both datasource kinds are configured")
	}
	data, err := json.Marshal(res.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.Parse(data); err != nil {
		t.Errorf("spec.Parse: %v", err)
	}
}

func checkLines(t *testing.T, f corpusFile, res *grafana.Result) {
	t.Helper()
	lines := res.Report.Panels
	counts := res.Report.Counts
	if counts.Total != len(lines) {
		t.Errorf("Counts.Total = %d, want the %d panel lines", counts.Total, len(lines))
	}
	if sum := counts.Imported + counts.Partial + counts.Placeholder; sum != counts.Total {
		t.Errorf("imported + partial + placeholder = %d, want Total %d", sum, counts.Total)
	}
	byPath := map[string]grafana.PanelLine{}
	for _, line := range lines {
		if _, dup := byPath[line.GrafanaPath]; dup {
			t.Errorf("two panel lines for %s", line.GrafanaPath)
		}
		byPath[line.GrafanaPath] = line
	}
	expected := expectedPanels(t, f.raw)
	if len(lines) != len(expected) {
		t.Errorf("%d panel lines, want %d Grafana panels that are neither rows nor repeat clones", len(lines), len(expected))
	}
	for _, p := range expected {
		line, ok := byPath[p.path]
		if !ok {
			t.Errorf("no panel line for %s", p.path)
			continue
		}
		targets := p.targets()
		if want := min(len(targets), maxLinePaths); len(line.Queries) != want {
			t.Errorf("%s: %d query lines, want %d", p.path, len(line.Queries), want)
			continue
		}
		for i, q := range line.Queries {
			if want, _ := targets[i]["refId"].(string); q.RefID != want {
				t.Errorf("%s: query line %d has refId %q, want %q", p.path, i, q.RefID, want)
			}
		}
	}
}

func checkStatuses(t *testing.T, res *grafana.Result) {
	t.Helper()
	for _, line := range res.Report.Panels {
		switch line.Status {
		case "partial":
			if len(line.Paths) == 0 || len(line.Reasons) == 0 {
				t.Errorf("%s is partial with %d paths and %d reasons", line.PanelID, len(line.Paths), len(line.Reasons))
			}
		case "placeholder":
			if len(line.Reasons) == 0 {
				t.Errorf("%s is a placeholder without a reason", line.PanelID)
			}
		case "imported":
			if len(line.Paths) != 0 || len(line.Reasons) != 0 {
				t.Errorf("%s is imported with %d paths and %d reasons", line.PanelID, len(line.Paths), len(line.Reasons))
			}
		default:
			t.Errorf("%s has status %q", line.PanelID, line.Status)
		}
	}
}

func checkPanelIDs(t *testing.T, res *grafana.Result) {
	t.Helper()
	inSpec := map[string]int{}
	for _, entry := range res.Spec.AllPanels() {
		if entry.Panel.Type != spec.Row {
			inSpec[entry.Panel.ID]++
		}
	}
	inLines := map[string]int{}
	for _, line := range res.Report.Panels {
		inLines[line.PanelID]++
	}
	for id, n := range inSpec {
		if inLines[id] != 1 {
			t.Errorf("Spec panel %s appears in %d lines, want 1", id, inLines[id])
		}
		if n != 1 {
			t.Errorf("Spec panel id %s is used %d times", id, n)
		}
	}
	for id := range inLines {
		if inSpec[id] == 0 {
			t.Errorf("line %s has no Spec panel", id)
		}
	}
}

var legacyReference = regexp.MustCompile(`\[\[([A-Za-z0-9_]+)(?::([A-Za-z0-9_]+))?\]\]`)

func normalizeLegacy(s string) string {
	return legacyReference.ReplaceAllStringFunc(s, func(match string) string {
		m := legacyReference.FindStringSubmatch(match)
		if m[2] != "" {
			return "${" + m[1] + ":" + m[2] + "}"
		}
		return "${" + m[1] + "}"
	})
}

func undoRewrites(text string, rewrites []string) string {
	for i := len(rewrites) - 1; i >= 0; i-- {
		from, to, _ := strings.Cut(rewrites[i], " -> ")
		if strings.HasPrefix(from, "refId ") {
			continue
		}
		text = strings.ReplaceAll(text, to, from)
	}
	return text
}

func sourceQueryTexts(target map[string]any) []string {
	var texts []string
	for _, key := range []string{"expr", "rawSql", "query"} {
		if s, _ := target[key].(string); s != "" {
			texts = append(texts, normalizeLegacy(s))
		}
	}
	return texts
}

func checkQueryText(t *testing.T, f corpusFile, res *grafana.Result) {
	t.Helper()
	byPath := map[string]grafana.PanelLine{}
	for _, line := range res.Report.Panels {
		byPath[line.GrafanaPath] = line
	}
	for _, p := range expectedPanels(t, f.raw) {
		line, ok := byPath[p.path]
		if !ok {
			continue
		}
		entry, ok := res.Spec.FindPanel(line.PanelID)
		if !ok {
			continue
		}
		targets := p.targets()
		if p.raw["type"] == "alertlist" && len(targets) == 0 && line.Status != "placeholder" {
			checkImporterQuery(t, p.path, entry.Panel, line)
			continue
		}
		kept := 0
		for i, ql := range line.Queries {
			if (ql.Ref == nil) == (ql.Dropped == nil) {
				t.Errorf("%s: query line %d (refId %q) must have exactly one of ref and dropped", p.path, i, ql.RefID)
				continue
			}
			if ql.Dropped != nil {
				continue
			}
			kept++
			for _, rewrite := range ql.Rewrites {
				if strings.Contains(rewrite, " (instant) -> ") {
					kept++
				}
			}
			if i >= len(targets) {
				t.Errorf("%s: query line %d has no Grafana target", p.path, i)
				continue
			}
			text, found := specQueryText(entry.Panel, *ql.Ref)
			if !found {
				t.Errorf("%s: no Spec query with ref %s", p.path, *ql.Ref)
				continue
			}
			got := normalizeLegacy(undoRewrites(text, ql.Rewrites))
			if !slices.Contains(sourceQueryTexts(targets[i]), got) {
				t.Errorf("%s targets[%d]: Spec query %q with rewrites %q undone does not equal the Grafana text", p.path, i, text, ql.Rewrites)
			}
		}
		if kept != len(entry.Panel.Queries) {
			t.Errorf("%s: %d kept query lines, want the %d Spec queries", p.path, kept, len(entry.Panel.Queries))
		}
	}
}

func checkImporterQuery(t *testing.T, path string, panel *spec.Panel, line grafana.PanelLine) {
	t.Helper()
	if len(panel.Queries) != 1 || len(line.Queries) != 0 {
		t.Errorf("%s: want one Spec query and no report query lines, got %d and %d", path, len(panel.Queries), len(line.Queries))
		return
	}
	if text := panel.Queries[0].Query; !strings.HasPrefix(text, "ALERTS{") || !strings.HasSuffix(text, "}") {
		t.Errorf("%s: Spec query %q is not an ALERTS selector", path, text)
	}
}

func specQueryText(p *spec.Panel, ref string) (string, bool) {
	for k, q := range p.Queries {
		effective := q.Ref
		if effective == "" {
			effective = spec.DefaultQueryRef(k)
		}
		if effective == ref {
			return q.Query, true
		}
	}
	return "", false
}

func checkOrder(t *testing.T, f corpusFile, res *grafana.Result) {
	t.Helper()
	pathOf := map[string]string{}
	for _, line := range res.Report.Panels {
		pathOf[line.PanelID] = line.GrafanaPath
	}
	var got []string
	for _, entry := range res.Spec.AllPanels() {
		if entry.Panel.Type != spec.Row {
			got = append(got, pathOf[entry.Panel.ID])
		}
	}
	var want []string
	for _, p := range expectedPanels(t, f.raw) {
		want = append(want, p.path)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Spec panel order %v, want the Grafana grid order %v", got, want)
	}
}

func checkUnits(t *testing.T, res *grafana.Result) {
	t.Helper()
	known := map[string]bool{}
	for _, u := range spec.Units() {
		known[u] = true
	}
	check := func(id, unit string) {
		if unit == "" || known[unit] {
			return
		}
		for _, form := range []string{"suffix:", "prefix:", "time:", "si:", "count:", "currency:"} {
			if strings.HasPrefix(unit, form) {
				return
			}
		}
		t.Errorf("%s has unknown unit %q", id, unit)
	}
	for _, entry := range res.Spec.AllPanels() {
		check(entry.Panel.ID, entry.Panel.Unit)
		for _, o := range entry.Panel.Overrides {
			check(entry.Panel.ID, o.Unit)
		}
	}
}

func TestCorpusRatio(t *testing.T) {
	var total, imported, partial, placeholder, failedPanels int
	var failed []string
	byType := map[string]int{}
	openByType := map[string]int{}
	soleBlocker := map[string]int{}
	for _, f := range corpusFiles(t) {
		o := convertFile(t, f)
		if o.res == nil {
			if o.inputCode != tooManyPanels {
				t.Errorf("%s: input error %s", f.id(), o.inputCode)
			}
			n := len(expectedPanels(t, f.raw))
			total += n
			failedPanels += n
			failed = append(failed, f.id())
			continue
		}
		counts := o.res.Report.Counts
		total += counts.Total
		imported += counts.Imported
		partial += counts.Partial
		placeholder += counts.Placeholder
		sources := map[string]map[string]any{}
		for _, source := range expectedPanels(t, f.raw) {
			sources[source.path] = source.raw
		}
		for _, line := range o.res.Report.Panels {
			if line.Status == "placeholder" {
				byType[line.SourceType]++
				if !excludedPlaceholder(line.SourceType, sources[line.GrafanaPath]) {
					openByType[line.SourceType]++
				}
			}
			if path, ok := soleOptionPath(line); ok {
				soleBlocker[path]++
			}
		}
	}
	open := 0
	for _, n := range openByType {
		open += n
	}
	t.Logf("panels %d: imported %d, partial %d, placeholder %d, in failed files %d", total, imported, partial, placeholder, failedPanels)
	t.Logf("fully imported %.1f%%, imported or partial %.1f%%", percent(imported, total), percent(imported+partial, total))
	t.Logf("placeholders by source type: %s", countsByType(byType))
	t.Logf("open placeholders by source type: %s", countsByType(openByType))
	t.Logf("failed files: %s", strings.Join(failed, " "))
	t.Logf("partial by one path only: %s", strings.Join(topCounts(soleBlocker, worklistSize), " "))
	if (imported+partial)*100 < total*minimumRatio {
		t.Errorf("imported or partial is %.1f%% of %d panels, the gate is %d%%", percent(imported+partial, total), total, minimumRatio)
	}
	if percent(imported, total) < minimumFull {
		t.Errorf("fully imported is %.1f%% of %d panels, the floor is %.1f%%", percent(imported, total), total, minimumFull)
	}
	if full := percent(imported, total); full >= minimumFull+fullSlack {
		t.Errorf("fully imported is %.1f%% of %d panels; raise minimumFull from %.1f to %.1f", full, total, minimumFull, math.Floor(full*10)/10)
	}
	if open > maximumOpenPlaceholders {
		t.Errorf("%d placeholders are not alert, annotation, live or library panels; the ceiling is %d", open, maximumOpenPlaceholders)
	}
	if open < maximumOpenPlaceholders {
		t.Errorf("%d placeholders are not alert, annotation, live or library panels; lower maximumOpenPlaceholders from %d to %d", open, maximumOpenPlaceholders, open)
	}
}

var excludedPlaceholderTypes = map[string]bool{"annolist": true, "alertGroups": true, "live": true, "alertlist": true}

func excludedPlaceholder(sourceType string, source map[string]any) bool {
	if excludedPlaceholderTypes[sourceType] {
		return true
	}
	_, library := source["libraryPanel"].(map[string]any)
	if !library {
		return false
	}
	for _, key := range []string{"targets", "options", "fieldConfig", "content"} {
		if present(source[key]) {
			return false
		}
	}
	return true
}

func countsByType(counts map[string]int) string {
	types := make([]string, 0, len(counts))
	for typ := range counts {
		types = append(types, typ)
	}
	sort.Strings(types)
	entries := make([]string, len(types))
	for i, typ := range types {
		entries[i] = typ + "=" + strconv.Itoa(counts[typ])
	}
	return strings.Join(entries, " ")
}

func TestExcludedPlaceholders(t *testing.T) {
	library := map[string]any{"uid": "u", "name": "n"}
	query := []any{map[string]any{"refId": "A"}}
	cases := []struct {
		name       string
		sourceType string
		source     map[string]any
		want       bool
	}{
		{"annotation list", "annolist", map[string]any{}, true},
		{"alert group list", "alertGroups", map[string]any{}, true},
		{"live panel", "live", map[string]any{}, true},
		{"alert list", "alertlist", map[string]any{}, true},
		{"welcome panel", "welcome", map[string]any{}, false},
		{"query panel", "timeseries", map[string]any{"targets": query}, false},
		{"library panel without a model", "", map[string]any{"libraryPanel": library}, true},
		{"library panel with empty model keys", "", map[string]any{"libraryPanel": library, "targets": []any{}, "options": map[string]any{}}, true},
		{"library panel with targets", "timeseries", map[string]any{"libraryPanel": library, "targets": query}, false},
		{"library panel with options", "timeseries", map[string]any{"libraryPanel": library, "options": map[string]any{"legend": true}}, false},
		{"no source panel", "timeseries", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := excludedPlaceholder(c.sourceType, c.source); got != c.want {
				t.Errorf("excludedPlaceholder(%q) = %v, want %v", c.sourceType, got, c.want)
			}
		})
	}
}

func soleOptionPath(line grafana.PanelLine) (string, bool) {
	if line.Status != "partial" {
		return "", false
	}
	distinct := map[string]bool{}
	for _, path := range line.Paths {
		path = strings.TrimPrefix(path, line.GrafanaPath+".")
		distinct[panelIndex.ReplaceAllString(path, "[]")] = true
	}
	if len(distinct) != 1 {
		return "", false
	}
	for path := range distinct {
		return path, true
	}
	return "", false
}

func topCounts(counts map[string]int, limit int) []string {
	paths := make([]string, 0, len(counts))
	for path := range counts {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		if counts[paths[i]] != counts[paths[j]] {
			return counts[paths[i]] > counts[paths[j]]
		}
		return paths[i] < paths[j]
	})
	if len(paths) > limit {
		paths = paths[:limit]
	}
	entries := make([]string, len(paths))
	for i, path := range paths {
		entries[i] = path + "=" + strconv.Itoa(counts[path])
	}
	return entries
}

func percent(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return 100 * float64(part) / float64(whole)
}

func listedFiles(t *testing.T) map[string][]string {
	t.Helper()
	sources, err := testdata(t).ReadFile(filepath.Join(corpusDir, "SOURCES.md"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string][]string{}
	section := ""
	for _, line := range strings.Split(string(sources), "\n") {
		if name, ok := strings.CutPrefix(line, "## "); ok {
			section = strings.TrimSpace(name)
			continue
		}
		if entry, ok := strings.CutPrefix(strings.TrimSpace(line), "- `"); ok && strings.HasSuffix(entry, ".json`") {
			listed[section] = append(listed[section], strings.TrimSuffix(entry, "`"))
		}
	}
	return listed
}

func TestCorpusLicences(t *testing.T) {
	root := testdata(t)
	dirs, err := fs.ReadDir(root.FS(), corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	listed := listedFiles(t)
	onDisk := map[string][]string{}
	for _, f := range corpusFiles(t) {
		onDisk[f.source] = append(onDisk[f.source], f.name)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		source := d.Name()
		if info, err := root.Stat(filepath.Join(corpusDir, source, "LICENSE")); err != nil || info.IsDir() {
			t.Errorf("%s has no LICENSE file", source)
		}
		for _, name := range onDisk[source] {
			if !slices.Contains(listed[source], name) {
				t.Errorf("%s/%s is not listed in SOURCES.md", source, name)
			}
		}
		for _, name := range listed[source] {
			if !slices.Contains(onDisk[source], name) {
				t.Errorf("SOURCES.md lists %s/%s, which does not exist", source, name)
			}
		}
	}
}

var typeCountRow = regexp.MustCompile(`(?m)^\| ([A-Za-z-]+) \| ([0-9]+) \|$`)

func TestCorpusTotals(t *testing.T) {
	sources, err := testdata(t).ReadFile(filepath.Join(corpusDir, "SOURCES.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, asWritten, _ := strings.Cut(string(sources), "### As written")
	asWritten, _, _ = strings.Cut(asWritten, "###")
	var wantRows, wantPanels int
	for _, m := range typeCountRow.FindAllStringSubmatch(asWritten, -1) {
		n, _ := strconv.Atoi(m[2])
		if m[1] == "row" {
			wantRows += n
		} else {
			wantPanels += n
		}
	}
	var rows, panels int
	for _, f := range corpusFiles(t) {
		r, p := countEntries(t, f.raw)
		rows += r
		panels += p
	}
	if rows != wantRows || panels != wantPanels {
		t.Errorf("the corpus holds %d panels and %d rows; SOURCES.md says %d and %d", panels, rows, wantPanels, wantRows)
	}
}

func countEntries(t *testing.T, raw []byte) (rows, panels int) {
	t.Helper()
	dash := decodeDashboard(t, raw)
	count := func(items []any) {
		for _, item := range items {
			if p, ok := item.(map[string]any); ok {
				if isRow(p) {
					rows++
				} else {
					panels++
				}
			}
		}
	}
	items, _ := dash["panels"].([]any)
	count(items)
	for _, item := range items {
		if p, ok := item.(map[string]any); ok && isRow(p) {
			children, _ := p["panels"].([]any)
			count(children)
		}
	}
	legacy, _ := dash["rows"].([]any)
	for _, r := range legacy {
		rows++
		row, _ := r.(map[string]any)
		children, _ := row["panels"].([]any)
		count(children)
	}
	return rows, panels
}
