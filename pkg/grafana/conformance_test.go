package grafana

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
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

var conformanceSeed = flag.Bool("conformance.seed", false, "write seed.txt and gaps for conformance areas that have no seed yet")

const (
	conformanceDir     = "conformance"
	conformanceVersion = "v12.3.0"
	expectConvert      = "convert"
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
	hasSeed bool
	seed    []string
	gaps    []string
}

func (a *conformanceArea) hasGap(id string) bool {
	return slices.Contains(a.gaps, id)
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

type conformanceObservation struct {
	clean  bool
	detail string
	spec   []byte
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
	for _, p := range res.Report.Panels {
		if p.Status == statusImported {
			continue
		}
		parts = append(parts, fmt.Sprintf("panel %s %s %s %s", p.PanelID, p.Status, strings.Join(p.Reasons, " "), strings.Join(p.Paths, " ")))
	}
	for _, n := range res.Report.Notes {
		parts = append(parts, fmt.Sprintf("note %s: %s %s", n.Subject, n.Reason, strings.Join(n.Paths, " ")))
	}
	if len(parts) > 0 {
		return conformanceObservation{detail: "reported: " + strings.Join(parts, "; "), spec: specJSON}
	}
	return conformanceObservation{clean: true, spec: specJSON}
}

func classifyConformance(e conformanceEntry, variant conformanceObservation, changed, cosmetic bool) string {
	expect := e.expectation()
	if expect == expectExcluded {
		if variant.clean {
			return "imported cleanly but is excluded"
		}
		return ""
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
	return classifyConformance(e, variant, changed, cosmetic)
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
	out = append(out, a.gapProblems(ids)...)
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
	case expectConvert, expectBookkeeping, expectExcluded:
	case expectCosmetic:
		if !a.rootIsPanel() {
			add("cosmetic is only for areas whose root is a panel")
		}
	default:
		add("expect %q is not convert, cosmetic, bookkeeping or excluded", e.Expect)
	}
	if _, _, err := a.documents(e); err != nil {
		add("%v", err)
	}
	if e.expectation() == expectConvert && a.rootIsPanel() && isGrafanaDefault(spec.PanelType(a.panelType()), e.Path, e.Value) {
		add("value is the importer's Grafana default")
	}
	return out
}

func (a *conformanceArea) gapProblems(entries map[string]conformanceEntry) []string {
	var out []string
	for _, id := range a.gaps {
		e, ok := entries[id]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("gaps/%s is not a catalog id", id))
		case e.expectation() == expectExcluded:
			out = append(out, fmt.Sprintf("gaps/%s is an excluded entry, which is never a gap", id))
		}
		if !slices.Contains(a.seed, id) {
			out = append(out, fmt.Sprintf("gaps/%s is not in seed.txt, so a new gap cannot be added", id))
		}
	}
	for i, id := range a.seed {
		if _, ok := entries[id]; !ok {
			out = append(out, fmt.Sprintf("seed.txt line %d: %s is not a catalog id", i+1, id))
		}
		if i > 0 && a.seed[i-1] >= id {
			out = append(out, fmt.Sprintf("seed.txt line %d: %s is not sorted after %s or is repeated", i+1, id, a.seed[i-1]))
		}
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
	seed, err := root.ReadFile(path.Join(dir, "seed.txt"))
	switch {
	case err == nil:
		a.hasSeed = true
		a.seed = strings.Fields(string(seed))
	case !errors.Is(err, fs.ErrNotExist):
		t.Fatal(err)
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

func (a *conformanceArea) writeSeed(t *testing.T, root *os.Root, failing []string) {
	t.Helper()
	dir := path.Join(conformanceDir, a.name)
	sort.Strings(failing)
	var seed strings.Builder
	for _, id := range failing {
		seed.WriteString(id + "\n")
	}
	if err := root.WriteFile(path.Join(dir, "seed.txt"), []byte(seed.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if len(failing) == 0 {
		return
	}
	if err := root.MkdirAll(path.Join(dir, "gaps"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, id := range failing {
		if err := root.WriteFile(path.Join(dir, "gaps", id), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOptionConformance(t *testing.T) {
	root := conformanceRoot(t)
	totalEntries, totalPass, totalGaps := 0, 0, 0
	for _, a := range loadConformanceAreas(t, root) {
		seeding := *conformanceSeed && !a.hasSeed
		if *conformanceSeed && a.hasSeed {
			t.Logf("seed exists for %s, nothing written", a.name)
		}
		pass, gaps := 0, 0
		var failing []string
		t.Run(a.name, func(t *testing.T) {
			for _, e := range a.catalog.Entries {
				t.Run(e.ID, func(t *testing.T) {
					outcome := a.run(e)
					switch {
					case outcome == "" && a.hasGap(e.ID):
						t.Errorf("converts now: delete testdata/conformance/%s/gaps/%s", a.name, e.ID)
					case outcome == "":
						pass++
					case seeding:
						failing = append(failing, e.ID)
						gaps++
					case a.hasGap(e.ID):
						gaps++
					default:
						t.Error(outcome)
					}
				})
			}
		})
		if seeding {
			a.writeSeed(t, root, failing)
		}
		t.Logf("%s: %d entries, %d pass, %d known gaps", a.name, len(a.catalog.Entries), pass, gaps)
		totalEntries += len(a.catalog.Entries)
		totalPass += pass
		totalGaps += gaps
	}
	t.Logf("total: %d entries, %d pass, %d known gaps", totalEntries, totalPass, totalGaps)
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyConformance(conformanceEntry{ID: "p=1", Path: "p", Value: 1, Expect: tc.expect}, tc.variant, tc.changed, tc.cosmetic)
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
		{"a reported option fails with its path", conformanceEntry{ID: "refresh=1d", Path: "refresh", Value: "1d"}, "reported: note refresh"},
		{"a dropped option is named silent", conformanceEntry{ID: "liveNow=true", Path: "liveNow", Value: true}, "silently dropped"},
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
		{"cosmetic outside a panel root", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) { a.catalog.Entries[0].Expect = expectCosmetic }, "only for areas whose root is a panel"},
		{"missing root", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) { a.catalog.Root = "panels[3]" }, "does not exist in base.json"},
		{"gap that is not an id", conformanceArea{name: "ok", base: base, catalog: good, hasSeed: true, seed: []string{"a=1"}, gaps: []string{"zzz"}}, nil, "gaps/zzz is not a catalog id"},
		{"gap missing from the seed", conformanceArea{name: "ok", base: base, catalog: good, hasSeed: true, gaps: []string{"a=1"}}, nil, "is not in seed.txt"},
		{"gap on an excluded entry", conformanceArea{name: "ok", base: base, catalog: good, hasSeed: true, seed: []string{"a=1"}, gaps: []string{"a=1"}}, func(a *conformanceArea) { a.catalog.Entries[0].Expect = expectExcluded }, "never a gap"},
		{"unsorted seed", conformanceArea{name: "ok", base: base, catalog: good, hasSeed: true, seed: []string{"b=1", "a=1"}}, func(a *conformanceArea) {
			a.catalog.Entries = append(a.catalog.Entries, conformanceEntry{ID: "b=1", Path: "b", Value: 1})
		}, "not sorted"},
		{"seed id not in the catalog", conformanceArea{name: "ok", base: base, catalog: good, hasSeed: true, seed: []string{"q=1"}}, nil, "q=1 is not a catalog id"},
		{"default value in a panel area", conformanceArea{name: "ok", base: base, catalog: good}, func(a *conformanceArea) {
			a.catalog.Root = "panels[0]"
			a.catalog.Entries = []conformanceEntry{{ID: "options.legend.showLegend=true", Path: "options.legend.showLegend", Value: true}}
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
