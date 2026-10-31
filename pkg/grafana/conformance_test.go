package grafana

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	conformanceDir     = "conformance"
	conformanceVersion = "v12.3.0"
	expectConvert      = "convert"
	expectConvertNoted = "convert_noted"
	expectCosmetic     = "cosmetic"
	expectBookkeeping  = "bookkeeping"
	expectExcluded     = "excluded"
)

var (
	conformanceAreaName = regexp.MustCompile(`^[a-z0-9-]+$`)
	conformanceID       = regexp.MustCompile(`^[A-Za-z0-9_.\[\]=:+-]{1,200}$`)
	conformancePanel    = regexp.MustCompile(`^panels\[[0-9]+\]$`)
	conformanceStep     = regexp.MustCompile(`^([A-Za-z0-9_-]+)((?:\[[0-9]+\])*)$`)
	conformanceIndex    = regexp.MustCompile(`\[([0-9]+)\]`)
)

var conformanceOptions = Options{Datasources: []Datasource{
	{UID: "prom", Type: "prometheus"},
	{UID: "ch", Type: "clickhouse"},
}}

type conformanceEntry struct {
	ID     string         `json:"id"`
	Path   string         `json:"path"`
	Value  any            `json:"value"`
	With   map[string]any `json:"with"`
	Expect string         `json:"expect"`
}

func (e conformanceEntry) expectation() string {
	if e.Expect == "" {
		return expectConvert
	}
	return e.Expect
}

func (e conformanceEntry) converts() bool {
	return e.expectation() == expectConvert || e.expectation() == expectConvertNoted
}

type conformanceCatalog struct {
	Grafana string             `json:"grafana"`
	Sources []string           `json:"sources"`
	Root    string             `json:"root"`
	Entries []conformanceEntry `json:"entries"`
}

type conformanceArea struct {
	name    string
	base    []byte
	catalog conformanceCatalog
	gaps    []string
}

type pathStep struct {
	key   string
	index int
	isKey bool
}

func parsePath(p string) ([]pathStep, error) {
	if p == "" {
		return nil, nil
	}
	var steps []pathStep
	for _, part := range strings.Split(p, ".") {
		m := conformanceStep.FindStringSubmatch(part)
		if m == nil {
			return nil, fmt.Errorf("path %q: %q is not a key with optional [n] indexes (keys with '.' or '[' are not supported)", p, part)
		}
		steps = append(steps, pathStep{key: m[1], isKey: true})
		for _, idx := range conformanceIndex.FindAllStringSubmatch(m[2], -1) {
			n, err := strconv.Atoi(idx[1])
			if err != nil {
				return nil, fmt.Errorf("path %q: bad index %q", p, idx[1])
			}
			steps = append(steps, pathStep{index: n})
		}
	}
	return steps, nil
}

func setAt(node any, steps []pathStep, value any) (any, error) {
	if len(steps) == 0 {
		return value, nil
	}
	step := steps[0]
	if step.isKey {
		var obj map[string]any
		switch n := node.(type) {
		case nil:
			obj = map[string]any{}
		case map[string]any:
			obj = n
		default:
			return nil, fmt.Errorf("%q is addressed as an object but holds %T", step.key, node)
		}
		child, err := setAt(obj[step.key], steps[1:], value)
		if err != nil {
			return nil, err
		}
		obj[step.key] = child
		return obj, nil
	}
	var list []any
	switch n := node.(type) {
	case nil:
	case []any:
		list = n
	default:
		return nil, fmt.Errorf("index [%d] is addressed on a %T", step.index, node)
	}
	switch {
	case step.index < len(list):
		child, err := setAt(list[step.index], steps[1:], value)
		if err != nil {
			return nil, err
		}
		list[step.index] = child
	case step.index == len(list):
		child, err := setAt(nil, steps[1:], value)
		if err != nil {
			return nil, err
		}
		list = append(list, child)
	default:
		return nil, fmt.Errorf("index [%d] leaves a gap after %d elements", step.index, len(list))
	}
	return list, nil
}

func getAt(node any, steps []pathStep) (any, bool) {
	for _, step := range steps {
		if step.isKey {
			obj, ok := node.(map[string]any)
			if !ok {
				return nil, false
			}
			if node, ok = obj[step.key]; !ok {
				return nil, false
			}
			continue
		}
		list, ok := node.([]any)
		if !ok || step.index >= len(list) {
			return nil, false
		}
		node = list[step.index]
	}
	return node, true
}

func joinPath(root, p string) string {
	if root == "" {
		return p
	}
	return root + "." + p
}

func decodeConformanceDoc(raw []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func applyAt(doc map[string]any, fullPath string, value any) error {
	steps, err := parsePath(fullPath)
	if err != nil {
		return err
	}
	if len(steps) == 0 {
		return errors.New("empty path")
	}
	_, err = setAt(doc, steps, value)
	return err
}

func (a *conformanceArea) documents(e conformanceEntry) (baseline, variant []byte, err error) {
	if _, ok := e.With[e.Path]; ok {
		return nil, nil, fmt.Errorf("%s: with sets the entry's own path", e.ID)
	}
	doc, err := decodeConformanceDoc(a.base)
	if err != nil {
		return nil, nil, err
	}
	for _, key := range sortedKeys(e.With) {
		if err := applyAt(doc, joinPath(a.catalog.Root, key), e.With[key]); err != nil {
			return nil, nil, fmt.Errorf("%s: with %s: %w", e.ID, key, err)
		}
	}
	if baseline, err = json.Marshal(doc); err != nil {
		return nil, nil, err
	}
	if err := applyAt(doc, joinPath(a.catalog.Root, e.Path), e.Value); err != nil {
		return nil, nil, fmt.Errorf("%s: path %s: %w", e.ID, e.Path, err)
	}
	if variant, err = json.Marshal(doc); err != nil {
		return nil, nil, err
	}
	return baseline, variant, nil
}

type conformanceIssue struct {
	paths   []string
	reasons []string
}

type conformanceObservation struct {
	clean  bool
	detail string
	spec   []byte
	issues []conformanceIssue
}

func observeConversion(raw []byte) conformanceObservation {
	res, err := Convert(raw, conformanceOptions)
	if err != nil {
		var input *InputError
		if errors.As(err, &input) {
			return conformanceObservation{detail: fmt.Sprintf("input error %s: %s", input.Code, input.Message)}
		}
		return conformanceObservation{detail: "error: " + err.Error()}
	}
	if !res.Ready() {
		return conformanceObservation{detail: "datasource not matched"}
	}
	specJSON, err := json.Marshal(res.Spec)
	if err != nil {
		return conformanceObservation{detail: "error: " + err.Error()}
	}
	var parts []string
	var issues []conformanceIssue
	for _, p := range res.Report.Panels {
		if p.Status == statusImported && len(p.Paths) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("panel %s %s %s %s", p.PanelID, p.Status, strings.Join(p.Reasons, " "), strings.Join(p.Paths, " ")))
		issues = append(issues, conformanceIssue{paths: p.Paths, reasons: p.Reasons})
	}
	for _, n := range res.Report.Notes {
		parts = append(parts, fmt.Sprintf("note %s: %s %s", n.Subject, n.Reason, strings.Join(n.Paths, " ")))
		issues = append(issues, conformanceIssue{paths: n.Paths, reasons: []string{n.Reason}})
	}
	if len(parts) > 0 {
		return conformanceObservation{detail: "reported: " + strings.Join(parts, "; "), spec: specJSON, issues: issues}
	}
	return conformanceObservation{clean: true, spec: specJSON}
}

func classifyConformance(e conformanceEntry, fullPath string, variant conformanceObservation, changed, cosmetic bool) string {
	expect := e.expectation()
	if expect == expectExcluded {
		if variant.clean {
			return "imported cleanly but is excluded"
		}
		return ""
	}
	if expect == expectConvertNoted {
		return classifyNoted(fullPath, variant, changed)
	}
	if !variant.clean {
		return variant.detail
	}
	switch expect {
	case expectConvert:
		if !changed {
			return "silently dropped: the Spec did not change and the option is not cosmetic"
		}
	case expectCosmetic:
		if changed {
			return "changed the Spec but is marked cosmetic"
		}
		if !cosmetic {
			return fmt.Sprintf("not cosmetic: isCosmetic is false for %s=%s", e.Path, valueJSON(e.Value))
		}
	case expectBookkeeping:
		if changed {
			return "changed the Spec but is marked bookkeeping"
		}
	}
	return ""
}

func classifyNoted(fullPath string, variant conformanceObservation, changed bool) string {
	if variant.spec == nil {
		return variant.detail
	}
	if variant.clean {
		return "converted without a note: the report has no issue at " + fullPath
	}
	if !changed {
		return "silently dropped: the Spec did not change and the option is only noted"
	}
	noted := false
	for _, issue := range variant.issues {
		if slices.ContainsFunc(issue.reasons, isDecisionReason) {
			return "noted as an exclusion decision, which is never a convert_noted option: " + variant.detail
		}
		if !slices.ContainsFunc(issue.paths, func(p string) bool { return related(fullPath, p) }) {
			return "reports an issue away from " + fullPath + ": " + variant.detail
		}
		noted = true
	}
	if !noted {
		return "converted without a note: the report has no issue at " + fullPath
	}
	return ""
}

func valueJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func (a *conformanceArea) run(e conformanceEntry) string {
	baselineDoc, variantDoc, err := a.documents(e)
	if err != nil {
		return "catalog error: " + err.Error()
	}
	baseline := observeConversion(baselineDoc)
	if baseline.spec == nil {
		return "baseline: " + baseline.detail
	}
	variant := observeConversion(variantDoc)
	changed := !bytes.Equal(baseline.spec, variant.spec)
	cosmetic := e.expectation() == expectCosmetic && isCosmetic(e.Path, e.Value)
	return classifyConformance(e, joinPath(a.catalog.Root, e.Path), variant, changed, cosmetic)
}

func (a *conformanceArea) rootIsPanel() bool {
	return conformancePanel.MatchString(a.catalog.Root)
}

func (a *conformanceArea) panelType() string {
	doc, err := decodeConformanceDoc(a.base)
	if err != nil {
		return ""
	}
	steps, err := parsePath(joinPath(a.catalog.Root, "type"))
	if err != nil {
		return ""
	}
	typ, _ := getAt(doc, steps)
	s, _ := typ.(string)
	return s
}

func (a *conformanceArea) problems() []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	c := a.catalog
	if !conformanceAreaName.MatchString(a.name) {
		add("area name %q is not lowercase [a-z0-9-]+", a.name)
	}
	if c.Grafana != conformanceVersion {
		add("grafana is %q, want %q", c.Grafana, conformanceVersion)
	}
	if len(c.Sources) == 0 {
		add("sources is empty")
	}
	if _, err := parsePath(c.Root); err != nil {
		add("root: %v", err)
	}
	doc, err := decodeConformanceDoc(a.base)
	if err != nil {
		add("base.json: %v", err)
	} else if steps, _ := parsePath(c.Root); len(steps) > 0 {
		if _, ok := getAt(doc, steps); !ok {
			add("root %q does not exist in base.json", c.Root)
		}
	}
	ids := map[string]conformanceEntry{}
	for _, e := range c.Entries {
		out = append(out, a.entryProblems(e, ids)...)
		ids[e.ID] = e
	}
	out = append(out, a.gapProblems()...)
	return out
}

func (a *conformanceArea) entryProblems(e conformanceEntry, seen map[string]conformanceEntry) []string {
	var out []string
	add := func(format string, args ...any) {
		out = append(out, fmt.Sprintf("%s: "+format, append([]any{e.ID}, args...)...))
	}
	if !conformanceID.MatchString(e.ID) {
		add("id does not match %s", conformanceID)
	}
	if _, dup := seen[e.ID]; dup {
		add("duplicate id")
	}
	switch e.expectation() {
	case expectConvert, expectConvertNoted, expectBookkeeping, expectExcluded:
	case expectCosmetic:
		if !a.rootIsPanel() {
			add("cosmetic is only for areas whose root is a panel")
		}
	default:
		add("expect %q is not convert, convert_noted, cosmetic, bookkeeping or excluded", e.Expect)
	}
	if _, _, err := a.documents(e); err != nil {
		add("%v", err)
	}
	if e.converts() && a.rootIsPanel() && isGrafanaDefault(spec.PanelType(a.panelType()), e.Path, e.Value) {
		add("value is the importer's Grafana default")
	}
	return out
}

func (a *conformanceArea) gapProblems() []string {
	var out []string
	for _, id := range a.gaps {
		out = append(out, fmt.Sprintf("gaps/%s: every option needs a conformance outcome now that M2 is closed; give the entry convert, convert_noted, cosmetic, bookkeeping or excluded and delete testdata/conformance/%s/gaps/%s", id, a.name, id))
	}
	return out
}

func loadConformanceAreas(t *testing.T, root *os.Root) []*conformanceArea {
	t.Helper()
	catalogs, err := fs.Glob(root.FS(), conformanceDir+"/*/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(catalogs)
	if len(catalogs) == 0 {
		t.Fatal("no conformance area has a catalog.json")
	}
	areas := make([]*conformanceArea, 0, len(catalogs))
	for _, p := range catalogs {
		areas = append(areas, loadConformanceArea(t, root, path.Base(path.Dir(p))))
	}
	return areas
}

func loadConformanceArea(t *testing.T, root *os.Root, name string) *conformanceArea {
	t.Helper()
	dir := path.Join(conformanceDir, name)
	a := &conformanceArea{name: name}
	var err error
	if a.base, err = root.ReadFile(path.Join(dir, "base.json")); err != nil {
		t.Fatal(err)
	}
	raw, err := root.ReadFile(path.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&a.catalog); err != nil {
		t.Fatalf("%s/catalog.json: %v", name, err)
	}
	gaps, err := fs.ReadDir(root.FS(), path.Join(dir, "gaps"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	for _, g := range gaps {
		a.gaps = append(a.gaps, g.Name())
	}
	return a
}

func TestOptionConformance(t *testing.T) {
	root := conformanceRoot(t)
	totalEntries, totalPass := 0, 0
	for _, a := range loadConformanceAreas(t, root) {
		for _, problem := range a.gapProblems() {
			t.Error(problem)
		}
		pass := 0
		t.Run(a.name, func(t *testing.T) {
			for _, e := range a.catalog.Entries {
				t.Run(e.ID, func(t *testing.T) {
					if outcome := a.run(e); outcome != "" {
						t.Error(outcome)
						return
					}
					pass++
				})
			}
		})
		t.Logf("%s: %d entries, %d pass", a.name, len(a.catalog.Entries), pass)
		totalEntries += len(a.catalog.Entries)
		totalPass += pass
	}
	t.Logf("total: %d entries, %d pass", totalEntries, totalPass)
}

func TestConformanceCatalogs(t *testing.T) {
	root := conformanceRoot(t)
	for _, a := range loadConformanceAreas(t, root) {
		t.Run(a.name, func(t *testing.T) {
			for _, problem := range a.problems() {
				t.Error(problem)
			}
			if base := observeConversion(a.base); !base.clean {
				t.Errorf("base.json does not import cleanly: %s", base.detail)
			}
		})
	}
	for _, key := range uncataloguedCosmeticOptions(cosmeticOptions, loadConformanceAreas(t, root)) {
		t.Errorf("cosmeticOptions key %s has no expect: cosmetic entry in a panel conformance catalog", key)
	}
}

func uncataloguedCosmeticOptions(options map[string]cosmeticOption, areas []*conformanceArea) []string {
	catalogued := map[string]bool{}
	for _, a := range areas {
		if !a.rootIsPanel() {
			continue
		}
		for _, e := range a.catalog.Entries {
			if e.expectation() == expectCosmetic {
				catalogued[normalizePath(e.Path)] = true
			}
		}
	}
	var missing []string
	for key := range options {
		schemaKey := strings.HasPrefix(key, "options.") || strings.HasPrefix(key, "fieldConfig.")
		if schemaKey && !catalogued[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

func conformanceRoot(t *testing.T) *os.Root {
	t.Helper()
	root, err := os.OpenRoot("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func TestConformancePathSetter(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		path    string
		value   any
		want    string
		wantErr string
	}{
		{"creates nested objects", `{}`, "a.b.c", 1, `{"a":{"b":{"c":1}}}`, ""},
		{"replaces an existing key", `{"a":{"b":1,"c":2}}`, "a.b", "x", `{"a":{"b":"x","c":2}}`, ""},
		{"addresses an existing element", `{"l":[{"k":1},{"k":2}]}`, "l[1].k", 9, `{"l":[{"k":1},{"k":9}]}`, ""},
		{"appends at the length", `{"l":[1,2]}`, "l[2]", 3, `{"l":[1,2,3]}`, ""},
		{"creates a list for index zero", `{}`, "l[0].k", true, `{"l":[{"k":true}]}`, ""},
		{"refuses an index past the end", `{"l":[1]}`, "l[3]", 3, "", "leaves a gap"},
		{"refuses an index in a missing list", `{}`, "l[2]", 1, "", "leaves a gap"},
		{"refuses a dot inside a key", `{}`, "a..b", 1, "", "not a key"},
		{"refuses a bracket inside a key", `{}`, "a[b].c", 1, "", "not a key"},
		{"refuses to pass through a string", `{"a":"s"}`, "a.b", 1, "", "addressed as an object"},
		{"refuses an index on an object", `{"a":{}}`, "a[0]", 1, "", "addressed on a"},
		{"refuses a key on a list", `{"a":[1]}`, "a.b", 1, "", "addressed as an object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := decodeConformanceDoc([]byte(tc.doc))
			if err != nil {
				t.Fatal(err)
			}
			err = applyAt(doc, tc.path, tc.value)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("doc = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestConformanceClassify(t *testing.T) {
	clean := conformanceObservation{clean: true}
	reported := conformanceObservation{detail: "reported: note dashboard options: not supported yet weekStart"}
	noted := conformanceObservation{
		detail: "reported: note variable v: Some intervals are not kept. p.q",
		spec:   []byte(`{}`),
		issues: []conformanceIssue{{paths: []string{"p.q"}, reasons: []string{"Some intervals are not kept."}}},
	}
	withIssue := func(issue conformanceIssue) conformanceObservation {
		return conformanceObservation{detail: "reported: elsewhere", spec: []byte(`{}`), issues: []conformanceIssue{issue}}
	}
	decided := withIssue(conformanceIssue{paths: []string{"p"}, reasons: []string{decisions[0].reason}})
	elsewhere := withIssue(conformanceIssue{paths: []string{"other"}, reasons: []string{"Not kept."}})
	failed := conformanceObservation{detail: "datasource not matched"}
	convertedClean := conformanceObservation{clean: true, spec: []byte(`{}`)}
	cases := []struct {
		name     string
		expect   string
		variant  conformanceObservation
		changed  bool
		cosmetic bool
		want     string
	}{
		{"convert passes when the Spec changes", "", clean, true, false, ""},
		{"convert defaults to convert", expectConvert, clean, true, false, ""},
		{"convert fails when reported", expectConvert, reported, false, false, reported.detail},
		{"convert fails when silently dropped", expectConvert, clean, false, false, "silently dropped"},
		{"cosmetic passes when listed", expectCosmetic, clean, false, true, ""},
		{"cosmetic fails when reported", expectCosmetic, reported, false, true, reported.detail},
		{"cosmetic fails when the Spec changes", expectCosmetic, clean, true, true, "changed the Spec but is marked cosmetic"},
		{"cosmetic fails when not listed", expectCosmetic, clean, false, false, "not cosmetic: isCosmetic is false for p=1"},
		{"bookkeeping passes when nothing changes", expectBookkeeping, clean, false, false, ""},
		{"bookkeeping fails when reported", expectBookkeeping, reported, false, false, reported.detail},
		{"bookkeeping fails when the Spec changes", expectBookkeeping, clean, true, false, "changed the Spec but is marked bookkeeping"},
		{"excluded passes when reported", expectExcluded, reported, false, false, ""},
		{"excluded fails when imported cleanly", expectExcluded, clean, true, false, "imported cleanly but is excluded"},
		{"convert_noted passes when the Spec changes and the option is noted", expectConvertNoted, noted, true, false, ""},
		{"convert_noted passes when the note is on a child of the option", expectConvertNoted, withIssue(conformanceIssue{paths: []string{"p.q.r"}, reasons: []string{"Not kept."}}), true, false, ""},
		{"convert_noted passes when the note is on a parent of the option", expectConvertNoted, withIssue(conformanceIssue{paths: []string{"p"}, reasons: []string{"Not kept."}}), true, false, ""},
		{"convert_noted fails when the note is missing", expectConvertNoted, convertedClean, true, false, "converted without a note"},
		{"convert_noted fails when the note is missing and the Spec is unchanged", expectConvertNoted, convertedClean, false, false, "converted without a note"},
		{"convert_noted fails when the Spec does not change", expectConvertNoted, noted, false, false, "silently dropped"},
		{"convert_noted fails when the note is for another option", expectConvertNoted, elsewhere, true, false, "reports an issue away from p"},
		{"convert_noted fails when the note is an exclusion decision", expectConvertNoted, decided, true, false, "exclusion decision"},
		{"convert_noted fails when the import fails", expectConvertNoted, failed, false, false, failed.detail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyConformance(conformanceEntry{ID: "p=1", Path: "p", Value: 1, Expect: tc.expect}, "p", tc.variant, tc.changed, tc.cosmetic)
			if tc.want == "" && got != "" || !strings.Contains(got, tc.want) {
				t.Fatalf("outcome = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConformanceObservesRealConversions(t *testing.T) {
	area := &conformanceArea{
		name: "inline",
		base: []byte(`{"title":"T","schemaVersion":41,"panels":[{"id":1,"type":"timeseries","title":"Up","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"},"targets":[{"refId":"A","expr":"up","datasource":{"type":"prometheus","uid":"prom"}}]}]}`),
	}
	cases := []struct {
		name  string
		entry conformanceEntry
		want  string
	}{
		{"a converted option passes", conformanceEntry{ID: "graphTooltip=1", Path: "graphTooltip", Value: 1}, ""},
		{"a reported option fails with its path", conformanceEntry{ID: "snapshot=internal", Path: "snapshot", Value: "internal"}, "reported: note dashboard options"},
		{"a dropped option is named silent", conformanceEntry{ID: "style=light", Path: "style", Value: "light"}, "silently dropped"},
		{"bookkeeping does not change the Spec", conformanceEntry{ID: "uid=x", Path: "uid", Value: "abc", Expect: expectBookkeeping}, ""},
		{"an excluded option must be reported", conformanceEntry{ID: "annotations.list[0]=q", Path: "annotations.list[0]", Value: map[string]any{"name": "q", "expr": "up"}, Expect: expectExcluded}, ""},
		{"a with key equal to the path is a catalog error", conformanceEntry{ID: "a=1", Path: "weekStart", Value: "monday", With: map[string]any{"weekStart": "sunday"}}, "catalog error"},
		{"a path through a scalar is a catalog error", conformanceEntry{ID: "a=1", Path: "title.x", Value: 1}, "catalog error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := area.run(tc.entry)
			if tc.want == "" && got != "" || !strings.Contains(got, tc.want) {
				t.Fatalf("outcome = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConformanceNotedOptionsAgainstRealConversions(t *testing.T) {
	area := &conformanceArea{
		name:    "inline",
		catalog: conformanceCatalog{Root: "templating.list[0]"},
		base:    []byte(`{"title":"T","schemaVersion":41,"templating":{"list":[{"name":"v","type":"interval","query":"1m,5m"}]},"panels":[{"id":1,"type":"timeseries","title":"Up","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"},"targets":[{"refId":"A","expr":"up","datasource":{"type":"prometheus","uid":"prom"}}]}]}`),
	}
	cases := []struct {
		name  string
		entry conformanceEntry
		want  string
	}{
		{"a kept interval with a dropped member passes as noted", conformanceEntry{ID: "query=bad", Path: "query", Value: "1m,abc", Expect: expectConvertNoted}, ""},
		{"a dropped member fails as convert", conformanceEntry{ID: "query=bad", Path: "query", Value: "1m,abc"}, "reported: note variable v"},
		{"a clean conversion fails as noted", conformanceEntry{ID: "query=ok", Path: "query", Value: "30s,5m", Expect: expectConvertNoted}, "converted without a note"},
		{"a bookkeeping option fails as noted", conformanceEntry{ID: "refresh=1", Path: "refresh", Value: 1, Expect: expectConvertNoted}, "converted without a note"},
		{"a note on another option fails", conformanceEntry{ID: "auto_count=10", Path: "auto_count", Value: 10, With: map[string]any{"query": "1m,abc", "auto": true}, Expect: expectConvertNoted}, "reports an issue away from templating.list[0].auto_count"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := area.run(tc.entry)
			if tc.want == "" && got != "" || !strings.Contains(got, tc.want) {
				t.Fatalf("outcome = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConformanceCatalogProblems(t *testing.T) {
	base := []byte(`{"title":"T","panels":[{"id":1,"type":"timeseries"}]}`)
	good := conformanceCatalog{Grafana: conformanceVersion, Sources: []string{"s"}, Entries: []conformanceEntry{{ID: "a=1", Path: "a", Value: 1}}}
	cases := []struct {
		name   string
		area   conformanceArea
		change func(*conformanceArea)
		want   string
	}{
		{"a well formed area has no problems", conformanceArea{name: "ok", base: base, catalog: good}, nil, ""},
		{"wrong grafana version", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) { a.catalog.Grafana = "v12.0.0" }, "grafana is"},
		{"no sources", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) { a.catalog.Sources = nil }, "sources is empty"},
		{"bad area name", conformanceArea{name: "Bad_Name", base: base, catalog: good}, nil, "area name"},
		{"duplicate ids", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) {
			a.catalog.Entries = append(a.catalog.Entries, a.catalog.Entries[0])
		}, "duplicate id"},
		{"bad id", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) { a.catalog.Entries[0].ID = "a b" }, "id does not match"},
		{"unknown expect", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) { a.catalog.Entries[0].Expect = "maybe" }, "is not convert"},
		{"convert_noted is a known expect", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) { a.catalog.Entries[0].Expect = expectConvertNoted }, ""},
		{"cosmetic outside a panel root", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) { a.catalog.Entries[0].Expect = expectCosmetic }, "only for areas whose root is a panel"},
		{"missing root", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) { a.catalog.Root = "panels[3]" }, "does not exist in base.json"},
		{"a gap file is refused", conformanceArea{name: "ok", base: base, catalog: good, gaps: []string{"a=1"}}, nil, "every option needs a conformance outcome now that M2 is closed"},
		{"a gap file is refused whatever its name", conformanceArea{name: "ok", base: base, catalog: good, gaps: []string{"zzz"}}, nil, "gaps/zzz: every option needs a conformance outcome"},
		{"default value in a panel area", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) {
			a.catalog.Root = "panels[0]"
			a.catalog.Entries = []conformanceEntry{{ID: "options.legend.showLegend=true", Path: "options.legend.showLegend", Value: true}}
		}, "importer's Grafana default"},
		{"default value in a panel area with convert_noted", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) {
			a.catalog.Root = "panels[0]"
			a.catalog.Entries = []conformanceEntry{{ID: "options.legend.showLegend=true", Path: "options.legend.showLegend", Value: true, Expect: expectConvertNoted}}
		}, "importer's Grafana default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.area
			a.catalog.Entries = slices.Clone(good.Entries)
			if tc.change != nil {
				tc.change(&a)
			}
			problems := strings.Join(a.problems(), "\n")
			if tc.want == "" && problems != "" || !strings.Contains(problems, tc.want) {
				t.Fatalf("problems = %q, want one containing %q", problems, tc.want)
			}
		})
	}
}

func TestUncataloguedCosmeticOptions(t *testing.T) {
	panelArea := func(root string, entries ...conformanceEntry) *conformanceArea {
		return &conformanceArea{catalog: conformanceCatalog{Root: root, Entries: entries}}
	}
	cosmetic := func(path string) conformanceEntry {
		return conformanceEntry{ID: path + "=1", Path: path, Value: 1, Expect: expectCosmetic}
	}
	options := map[string]cosmeticOption{
		"options.zoomMode":                  {why: "w"},
		"options.layers[].name":             {why: "w"},
		"fieldConfig.defaults.custom.width": {why: "w"},
		"fillGradient":                      {why: "w"},
	}
	cases := []struct {
		name  string
		areas []*conformanceArea
		want  []string
	}{
		{"every schema key catalogued", []*conformanceArea{
			panelArea("panels[0]", cosmetic("options.zoomMode"), cosmetic("options.layers[0].name"), cosmetic("fieldConfig.defaults.custom.width")),
		}, nil},
		{"legacy keys without a prefix are exempt", []*conformanceArea{
			panelArea("panels[0]", cosmetic("options.zoomMode"), cosmetic("options.layers[0].name")),
		}, []string{"fieldConfig.defaults.custom.width"}},
		{"a convert entry does not count", []*conformanceArea{
			panelArea("panels[0]", conformanceEntry{ID: "options.zoomMode=1", Path: "options.zoomMode", Value: 1}, cosmetic("options.layers[0].name"), cosmetic("fieldConfig.defaults.custom.width")),
		}, []string{"options.zoomMode"}},
		{"an area outside a panel root does not count", []*conformanceArea{
			panelArea("", cosmetic("options.zoomMode"), cosmetic("options.layers[0].name"), cosmetic("fieldConfig.defaults.custom.width")),
		}, []string{"fieldConfig.defaults.custom.width", "options.layers[].name", "options.zoomMode"}},
		{"keys are collected across areas", []*conformanceArea{
			panelArea("panels[0]", cosmetic("options.zoomMode")),
			panelArea("panels[0]", cosmetic("options.layers[2].name"), cosmetic("fieldConfig.defaults.custom.width")),
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := uncataloguedCosmeticOptions(options, tc.areas); !slices.Equal(got, tc.want) {
				t.Fatalf("uncatalogued = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestConformanceRefusesGapFilesOnDisk(t *testing.T) {
	dir := t.TempDir()
	areaDir := path.Join(dir, conformanceDir, "ok")
	if err := os.MkdirAll(path.Join(areaDir, "gaps"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"base.json":    `{"title":"T","panels":[{"id":1,"type":"timeseries"}]}`,
		"catalog.json": `{"grafana":"v12.3.0","sources":["s"],"root":"","entries":[{"id":"a=1","path":"a","value":1}]}`,
		"gaps/a=1":     "",
	}
	for name, content := range files {
		if err := os.WriteFile(path.Join(areaDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })

	area := loadConformanceArea(t, root, "ok")
	problems := strings.Join(area.problems(), "\n")
	for _, want := range []string{"gaps/a=1", "every option needs a conformance outcome now that M2 is closed"} {
		if !strings.Contains(problems, want) {
			t.Errorf("problems = %q, want one containing %q", problems, want)
		}
	}
	if got := area.gapProblems(); len(got) != 1 {
		t.Errorf("gapProblems = %v, want one problem", got)
	}
}
