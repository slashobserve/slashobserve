package grafana

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"
	"testing"
)

var (
	uncataloguedSeed  = flag.Bool("uncatalogued.seed", false, "write testdata/uncatalogued when it does not exist")
	uncataloguedPrune = flag.Bool("uncatalogued.prune", false, "delete the worklist lines that are catalogued now")
)

const (
	uncataloguedDir  = "uncatalogued"
	maxReportedLines = 40
)

var exportRootKeys = []string{"__inputs", "__requires", "__elements"}

type freeKeyRule struct {
	scope  func(optionScope) bool
	parent func(normalised string) bool
}

func scopeKindIs(kind string) func(optionScope) bool {
	return func(s optionScope) bool { return s.kind() == kind }
}

func parentIs(paths ...string) func(string) bool {
	return func(p string) bool { return slices.Contains(paths, p) }
}

func transformIs(ids ...string) func(optionScope) bool {
	return func(s optionScope) bool {
		return s.kind() == "transform" && slices.Contains(ids, strings.TrimPrefix(string(s), "transform:"))
	}
}

var freeKeyRules = []freeKeyRule{
	{scope: scopeKindIs("panel"), parent: parentIs("aliasColors", "scopedVars")},
	{scope: scopeKindIs("transform"), parent: parentIs("options.renameByName", "options.indexByName", "options.excludeByName")},
	{scope: transformIs("groupBy", "groupToNestedTable", "groupingToMatrix"), parent: parentIs("options.fields")},
	{scope: transformIs("timeSeriesTable"), parent: parentIs("options")},
}

func freeKeyWidth(scope optionScope, parent string, segments []string, at int, value func(string) any) int {
	if at == 0 {
		return 0
	}
	prefix := strings.Join(segments[:at], ".")
	matched := slices.ContainsFunc(freeKeyRules, func(r freeKeyRule) bool {
		return r.scope(scope) && r.parent(parent)
	})
	if !matched {
		return 0
	}
	keys, ok := value(prefix).(map[string]any)
	if !ok {
		return 1
	}
	for n := 1; at+n <= len(segments); n++ {
		if _, found := keys[strings.Join(segments[at:at+n], ".")]; found {
			return n
		}
	}
	return 1
}

func catalogKey(scope optionScope, rel string, value func(prefix string) any) string {
	if rel == "" {
		return ""
	}
	segments := strings.Split(rel, ".")
	var normalised []string
	for i := 0; i < len(segments); {
		if n := freeKeyWidth(scope, strings.Join(normalised, "."), segments, i, value); n > 0 {
			normalised = append(normalised, "*")
			i += n
			continue
		}
		normalised = append(normalised, normalizePath(segments[i]))
		i++
	}
	return strings.Join(normalised, ".")
}

type catalogEntryNodes struct {
	expect string
	nodes  map[string]map[string]bool
}

type catalogIndex map[optionScope]map[string][]*catalogEntryNodes

type keyLink struct {
	key string
	vk  string
}

func (ix catalogIndex) add(scope optionScope, key string, entry *catalogEntryNodes) {
	if ix[scope] == nil {
		ix[scope] = map[string][]*catalogEntryNodes{}
	}
	ix[scope][key] = append(ix[scope][key], entry)
}

func (ix catalogIndex) covers(scope optionScope, key string, chain func() []keyLink) bool {
	scopes := []optionScope{scope}
	if wildcard := scope.wildcard(); wildcard != scope {
		scopes = append(scopes, wildcard)
	}
	for c := key; ; {
		for _, sc := range scopes {
			for _, entry := range ix[sc][c] {
				if c == key || entry.expect == expectExcluded || entry.expect == expectBookkeeping {
					return true
				}
				if _, below := entry.nodes[key]; below && entry.agreesWith(c, chain()) {
					return true
				}
			}
		}
		cut := strings.LastIndexAny(c, ".[")
		if cut < 0 {
			return false
		}
		c = c[:cut]
	}
}

func (e *catalogEntryNodes) agreesWith(from string, chain []keyLink) bool {
	for _, link := range chain {
		if link.vk == "" || len(link.key) < len(from) {
			continue
		}
		if !e.nodes[link.key][link.vk] {
			return false
		}
	}
	return true
}

func isDatasourceRef(key string) bool {
	return key == "datasource" || strings.HasSuffix(key, ".datasource")
}

func chainOf(scope optionScope, rel string, value func(string) any) []keyLink {
	cuts := map[int]bool{len(rel): true}
	for i := range len(rel) {
		switch rel[i] {
		case '.', '[':
			cuts[i] = true
		case ']':
			cuts[i+1] = true
		}
	}
	var chain []keyLink
	for _, cut := range slices.Sorted(maps.Keys(cuts)) {
		if cut == 0 {
			continue
		}
		prefix := rel[:cut]
		link := keyLink{key: catalogKey(scope, prefix, value)}
		if object, ok := value(prefix).(map[string]any); ok && !isDatasourceRef(link.key) {
			link.vk = valueKey(object)
		}
		chain = append(chain, link)
	}
	for i := 0; i+1 < len(chain); i++ {
		if chain[i+1].key == chain[i].key+".*" {
			chain[i].vk = ""
		}
	}
	return chain
}

func worklistLine(key string, chain []keyLink) string {
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i].vk != "" && chain[i].key != key && !namesItself(key, chain[i]) {
			return key + " " + chain[i].vk
		}
	}
	return key
}

func namesItself(key string, parent keyLink) bool {
	member, below := strings.CutPrefix(key, parent.key+".")
	return below && (member == "id" && strings.HasPrefix(parent.vk, "id:") || member == "type" && strings.HasPrefix(parent.vk, "type:"))
}

func collectNodes(rel string, v any, key func(rel string) string, nodes map[string]map[string]bool) {
	k := key(rel)
	if nodes[k] == nil {
		nodes[k] = map[string]bool{}
	}
	switch x := v.(type) {
	case map[string]any:
		if vk := valueKey(x); vk != "" {
			nodes[k][vk] = true
		}
		for name, child := range x {
			collectNodes(joinPath(rel, name), child, key, nodes)
		}
	case []any:
		for i, child := range x {
			collectNodes(fmt.Sprintf("%s[%d]", rel, i), child, key, nodes)
		}
	}
}

func buildCatalogIndex(t *testing.T, areas []*conformanceArea) catalogIndex {
	t.Helper()
	ix := catalogIndex{}
	for _, a := range areas {
		for _, e := range a.catalog.Entries {
			_, variant, err := a.documents(e)
			if err != nil {
				t.Fatalf("%s/%s: %v", a.name, e.ID, err)
			}
			src, err := load(variant)
			if err != nil {
				t.Fatalf("%s/%s: %v", a.name, e.ID, err)
			}
			if err := newRegistry(src).resolve(conformanceOptions); err != nil {
				t.Fatalf("%s/%s: %v", a.name, e.ID, err)
			}
			p := src.scopeOf(joinPath(a.catalog.Root, e.Path))
			value := func(prefix string) any {
				v, _ := src.valueAt(p.root, prefix)
				return v
			}
			key := func(rel string) string { return catalogKey(p.scope, rel, value) }
			entry := &catalogEntryNodes{expect: e.expectation(), nodes: map[string]map[string]bool{}}
			start, _ := src.valueAt(p.root, p.rel)
			heads := []string{p.rel}
			if p.rel == "" {
				whole, ok := start.(map[string]any)
				if !ok {
					t.Fatalf("%s/%s: the entry sets a whole %s that is not an object", a.name, e.ID, p.scope)
				}
				heads = sortedKeys(whole)
			}
			for _, head := range heads {
				v, _ := src.valueAt(p.root, head)
				collectNodes(head, v, key, entry.nodes)
			}
			scopes := []optionScope{p.scope}
			if a.name == "fieldconfig" || a.name == "panel" {
				scopes = append(scopes, "panel:*")
			}
			for _, scope := range scopes {
				for _, head := range heads {
					ix.add(scope, key(head), entry)
				}
			}
		}
	}
	return ix
}

func walkLeaves(v any, path string, visit func(path string)) {
	switch x := v.(type) {
	case nil:
	case string:
		if x != "" {
			visit(path)
		}
	case map[string]any:
		for _, name := range sortedKeys(x) {
			walkLeaves(x[name], joinPath(path, name), visit)
		}
	case []any:
		if len(x) == 0 {
			return
		}
		if !slices.ContainsFunc(x, func(item any) bool {
			switch item.(type) {
			case map[string]any, []any:
				return true
			}
			return false
		}) {
			visit(path)
			return
		}
		for i, item := range x {
			walkLeaves(item, fmt.Sprintf("%s[%d]", path, i), visit)
		}
	default:
		visit(path)
	}
}

func isIdentityKey(p scopedPath) bool {
	switch p.scope.kind() {
	case "panel", "variable":
		return p.rel == "type"
	case "transform":
		return p.rel == "id"
	case "dashboard":
		return slices.ContainsFunc(exportRootKeys, func(key string) bool { return isUnder(p.rel, key) })
	}
	return false
}

func uncataloguedLines(src *source, ix catalogIndex) map[optionScope]map[string]bool {
	out := map[optionScope]map[string]bool{}
	walkLeaves(src.dash, "", func(path string) {
		if slices.ContainsFunc(src.clones, func(clone string) bool { return isUnder(path, clone) }) {
			return
		}
		p := src.scopeOf(path)
		if isIdentityKey(p) {
			return
		}
		value := func(prefix string) any {
			v, _ := src.valueAt(p.root, prefix)
			return v
		}
		key := catalogKey(p.scope, p.rel, value)
		var chain []keyLink
		chainFn := func() []keyLink {
			if chain == nil {
				chain = chainOf(p.scope, p.rel, value)
			}
			return chain
		}
		if ix.covers(p.scope, key, chainFn) {
			return
		}
		if out[p.scope] == nil {
			out[p.scope] = map[string]bool{}
		}
		out[p.scope][worklistLine(key, chainFn())] = true
	})
	return out
}

func worklistFileName(scope optionScope) string {
	return strings.ReplaceAll(string(scope), ":", "-") + ".txt"
}

func corpusSources(t *testing.T) map[string]*source {
	t.Helper()
	root := conformanceRoot(t)
	paths, err := fs.Glob(root.FS(), "corpus/*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	sources := map[string]*source{}
	for _, p := range paths {
		raw, err := root.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		src, err := load(raw)
		if err != nil {
			continue
		}
		if err := newRegistry(src).resolve(conformanceOptions); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		sources[p] = src
	}
	if len(sources) == 0 {
		t.Fatal("the corpus is empty")
	}
	return sources
}

func readWorklist(t *testing.T) (map[string]map[string]bool, bool) {
	t.Helper()
	root := conformanceRoot(t)
	entries, err := fs.ReadDir(root.FS(), uncataloguedDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]map[string]bool{}, false
		}
		t.Fatal(err)
	}
	files := map[string]map[string]bool{}
	for _, entry := range entries {
		raw, err := root.ReadFile(path.Join(uncataloguedDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		lines := map[string]bool{}
		for _, line := range strings.Split(string(raw), "\n") {
			if line != "" {
				lines[line] = true
			}
		}
		files[entry.Name()] = lines
	}
	return files, true
}

func writeWorklist(t *testing.T, files map[string]map[string]bool) {
	t.Helper()
	root := conformanceRoot(t)
	if err := root.MkdirAll(uncataloguedDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for name, lines := range files {
		file := path.Join(uncataloguedDir, name)
		if len(lines) == 0 {
			if err := root.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			continue
		}
		body := strings.Join(slices.Sorted(maps.Keys(lines)), "\n") + "\n"
		if err := root.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func worklistProblems(uncovered map[string]map[string]bool, scopes map[string]optionScope, recorded map[string]map[string]bool) []string {
	var problems []string
	for _, name := range slices.Sorted(maps.Keys(uncovered)) {
		for _, line := range slices.Sorted(maps.Keys(uncovered[name])) {
			if !recorded[name][line] {
				problems = append(problems, fmt.Sprintf("%s: %s is not catalogued; add a catalog entry", scopes[name], line))
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(recorded)) {
		if len(recorded[name]) == 0 {
			problems = append(problems, fmt.Sprintf("delete %s", name))
		}
		for _, line := range slices.Sorted(maps.Keys(recorded[name])) {
			if !uncovered[name][line] {
				problems = append(problems, fmt.Sprintf("%s: %s is catalogued now; delete the line (or run with -uncatalogued.prune)", strings.TrimSuffix(name, ".txt"), line))
			}
		}
	}
	return problems
}

func TestCorpusOptionsAreCatalogued(t *testing.T) {
	ix := buildCatalogIndex(t, loadConformanceAreas(t, conformanceRoot(t)))
	uncovered := map[string]map[string]bool{}
	scopes := map[string]optionScope{}
	for _, src := range corpusSources(t) {
		for scope, lines := range uncataloguedLines(src, ix) {
			name := worklistFileName(scope)
			scopes[name] = scope
			if uncovered[name] == nil {
				uncovered[name] = map[string]bool{}
			}
			maps.Copy(uncovered[name], lines)
		}
	}
	recorded, exists := readWorklist(t)
	if *uncataloguedSeed && !exists {
		writeWorklist(t, uncovered)
		return
	}
	if *uncataloguedPrune {
		kept := map[string]map[string]bool{}
		for name, lines := range recorded {
			kept[name] = map[string]bool{}
			for line := range lines {
				if uncovered[name][line] {
					kept[name][line] = true
				}
			}
		}
		writeWorklist(t, kept)
		return
	}
	problems := worklistProblems(uncovered, scopes, recorded)
	if len(problems) > maxReportedLines {
		problems = append(problems[:maxReportedLines], fmt.Sprintf("and %d more", len(problems)-maxReportedLines))
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestCatalogKey(t *testing.T) {
	doc := map[string]any{
		"aliasColors": map[string]any{"node.cpu": "red", "plain": "blue"},
		"options": map[string]any{
			"renameByName": map[string]any{"Value": "Latency"},
			"fields":       map[string]any{"host": map[string]any{"operation": "groupby"}},
			"A":            map[string]any{"stat": "max"},
		},
	}
	value := func(prefix string) any {
		v, _ := lookup(doc, prefix)
		return v
	}
	cases := []struct {
		name  string
		scope optionScope
		rel   string
		want  string
	}{
		{"an empty path stays empty", "panel:stat", "", ""},
		{"a plain path is unchanged", "panel:stat", "options.reduceOptions.calcs", "options.reduceOptions.calcs"},
		{"indexes become brackets", "panel:timeseries", "fieldConfig.overrides[3].properties[0].value", "fieldConfig.overrides[].properties[].value"},
		{"an alias color is free", "panel:graph", "aliasColors.plain", "aliasColors.*"},
		{"an alias color with a dot is one free key", "panel:graph", "aliasColors.node.cpu", "aliasColors.*"},
		{"scoped variables are free with their value", "panel:timeseries", "scopedVars.host.value", "scopedVars.*.value"},
		{"alias colors are free on panels only", "query:prometheus", "aliasColors.plain", "aliasColors.plain"},
		{"organize rename keys are free", "transform:organize", "options.renameByName.Value", "options.renameByName.*"},
		{"rename keys are free on any transformation", "transform:calculateField", "options.renameByName.Value", "options.renameByName.*"},
		{"group by fields are free", "transform:groupBy", "options.fields.host.operation", "options.fields.*.operation"},
		{"nested table fields are free", "transform:groupToNestedTable", "options.fields.host.operation", "options.fields.*.operation"},
		{"partition by values keeps its fields", "transform:partitionByValues", "options.fields", "options.fields"},
		{"time series table refs are free", "transform:timeSeriesTable", "options.A.stat", "options.*.stat"},
		{"a transformation option keeps its name elsewhere", "transform:reduce", "options.A.stat", "options.A.stat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := catalogKey(tc.scope, tc.rel, value); got != tc.want {
				t.Fatalf("catalogKey(%s, %q) = %q, want %q", tc.scope, tc.rel, got, tc.want)
			}
		})
	}
}

func TestCatalogIndexCovers(t *testing.T) {
	nodesOf := func(links ...keyLink) map[string]map[string]bool {
		nodes := map[string]map[string]bool{}
		for _, link := range links {
			if nodes[link.key] == nil {
				nodes[link.key] = map[string]bool{}
			}
			if link.vk != "" {
				nodes[link.key][link.vk] = true
			}
		}
		return nodes
	}
	ix := catalogIndex{}
	ix.add("panel:stat", "options.reduceOptions", &catalogEntryNodes{expect: expectConvert, nodes: nodesOf(keyLink{key: "options.reduceOptions"}, keyLink{key: "options.reduceOptions.calcs"})})
	ix.add("panel:stat", "options.legacy", &catalogEntryNodes{expect: expectBookkeeping, nodes: nodesOf(keyLink{key: "options.legacy"})})
	ix.add("panel:stat", "options.hidden", &catalogEntryNodes{expect: expectCosmetic, nodes: nodesOf(keyLink{key: "options.hidden"})})
	ix.add("panel:*", "fieldConfig.overrides[].properties[]", &catalogEntryNodes{
		expect: expectConvert,
		nodes: nodesOf(
			keyLink{key: "fieldConfig.overrides[].properties[]", vk: "id:unit"},
			keyLink{key: "fieldConfig.overrides[].properties[].id"},
			keyLink{key: "fieldConfig.overrides[].properties[].value"},
		),
	})
	ix.add("panel:*", "title", &catalogEntryNodes{expect: expectConvert, nodes: nodesOf(keyLink{key: "title"})})
	override := func(vk string) func() []keyLink {
		return func() []keyLink {
			return []keyLink{{key: "fieldConfig.overrides[]"}, {key: "fieldConfig.overrides[].properties[]", vk: vk}}
		}
	}
	none := func() []keyLink { return nil }
	cases := []struct {
		name  string
		scope optionScope
		key   string
		chain func() []keyLink
		want  bool
	}{
		{"the option itself is catalogued", "panel:stat", "options.reduceOptions", none, true},
		{"a key below a catalogued value is covered", "panel:stat", "options.reduceOptions.calcs", none, true},
		{"a key the value does not hold is not covered", "panel:stat", "options.reduceOptions.limit", none, false},
		{"a sibling with a shared prefix is not covered", "panel:stat", "options.reduceOptionsX", none, false},
		{"a bookkeeping entry covers its subtree", "panel:stat", "options.legacy.anything.deeper", none, true},
		{"a cosmetic entry covers only itself", "panel:stat", "options.hidden.deeper", none, false},
		{"another scope is not consulted", "panel:gauge", "options.reduceOptions", none, false},
		{"the wildcard scope covers every panel type", "panel:graph", "title", none, true},
		{"an override property of the catalogued id is covered", "panel:graph", "fieldConfig.overrides[].properties[].value", override("id:unit"), true},
		{"an override property of another id is not covered", "panel:graph", "fieldConfig.overrides[].properties[].value", override("id:custom.hidden"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ix.covers(tc.scope, tc.key, tc.chain); got != tc.want {
				t.Fatalf("covers(%s, %q) = %v, want %v", tc.scope, tc.key, got, tc.want)
			}
		})
	}
}

func TestWalkLeaves(t *testing.T) {
	doc := map[string]any{
		"a": nil, "b": "", "c": []any{}, "d": map[string]any{},
		"scalars": []any{"x", "y"},
		"mixed":   []any{"x", map[string]any{"k": 1}},
		"zero":    0,
		"off":     false,
		"nested":  map[string]any{"list": []any{[]any{"p"}}},
	}
	var got []string
	walkLeaves(doc, "", func(path string) { got = append(got, path) })
	want := []string{"mixed[0]", "mixed[1].k", "nested.list[0]", "off", "scalars", "zero"}
	if !slices.Equal(got, want) {
		t.Fatalf("leaves = %v, want %v", got, want)
	}
}

func TestIdentityKeys(t *testing.T) {
	cases := []struct {
		name string
		p    scopedPath
		want bool
	}{
		{"a panel type", scopedPath{scope: "panel:graph", rel: "type"}, true},
		{"a variable type", scopedPath{scope: "variable:query", rel: "type"}, true},
		{"a nested type", scopedPath{scope: "panel:graph", rel: "datasource.type"}, false},
		{"a transformation id", scopedPath{scope: "transform:reduce", rel: "id"}, true},
		{"a panel id", scopedPath{scope: "panel:graph", rel: "id"}, false},
		{"a query type", scopedPath{scope: "query:expression", rel: "type"}, false},
		{"an export input", scopedPath{scope: scopeDashboard, rel: "__inputs[0].name"}, true},
		{"an export requirement", scopedPath{scope: scopeDashboard, rel: "__requires[1].id"}, true},
		{"a dashboard key", scopedPath{scope: scopeDashboard, rel: "title"}, false},
		{"a key that starts like an export key", scopedPath{scope: scopeDashboard, rel: "__inputsX"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isIdentityKey(tc.p); got != tc.want {
				t.Fatalf("isIdentityKey(%+v) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

func TestWorklistLine(t *testing.T) {
	chain := []keyLink{
		{key: "fieldConfig.overrides[]"},
		{key: "fieldConfig.overrides[].properties[]", vk: "id:custom.hidden"},
		{key: "fieldConfig.overrides[].properties[].value"},
	}
	cases := []struct {
		name  string
		key   string
		chain []keyLink
		want  string
	}{
		{"no discriminator", "title", nil, "title"},
		{"the deepest discriminator is named", "fieldConfig.overrides[].properties[].value", chain, "fieldConfig.overrides[].properties[].value id:custom.hidden"},
		{"a discriminator member does not name itself", "a.b.type", []keyLink{{key: "a.b", vk: "type:y"}}, "a.b.type"},
		{"an id member does not name itself", "a.b.id", []keyLink{{key: "a.b", vk: "id:y"}}, "a.b.id"},
		{"a type member under an id object is named", "a.b.type", []keyLink{{key: "a.b", vk: "id:y"}}, "a.b.type id:y"},
		{"the deepest of two wins", "a.b.c", []keyLink{{key: "a", vk: "type:x"}, {key: "a.b", vk: "type:y"}}, "a.b.c type:y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := worklistLine(tc.key, tc.chain); got != tc.want {
				t.Fatalf("worklistLine = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWorklistProblems(t *testing.T) {
	scopes := map[string]optionScope{"panel-graph.txt": "panel:graph"}
	set := func(lines ...string) map[string]bool {
		out := map[string]bool{}
		for _, l := range lines {
			out[l] = true
		}
		return out
	}
	cases := []struct {
		name      string
		uncovered map[string]map[string]bool
		recorded  map[string]map[string]bool
		want      []string
	}{
		{"matching lists are quiet", map[string]map[string]bool{"panel-graph.txt": set("bars")}, map[string]map[string]bool{"panel-graph.txt": set("bars")}, nil},
		{"a new line fails", map[string]map[string]bool{"panel-graph.txt": set("bars", "lines")}, map[string]map[string]bool{"panel-graph.txt": set("bars")}, []string{"panel:graph: lines is not catalogued; add a catalog entry"}},
		{"a covered line fails", map[string]map[string]bool{"panel-graph.txt": set("bars")}, map[string]map[string]bool{"panel-graph.txt": set("bars", "lines")}, []string{"panel-graph: lines is catalogued now; delete the line (or run with -uncatalogued.prune)"}},
		{"an empty file fails", map[string]map[string]bool{}, map[string]map[string]bool{"panel-graph.txt": set()}, []string{"delete panel-graph.txt"}},
		{"a file of covered lines fails per line", map[string]map[string]bool{}, map[string]map[string]bool{"panel-graph.txt": set("bars")}, []string{"panel-graph: bars is catalogued now; delete the line (or run with -uncatalogued.prune)"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := worklistProblems(tc.uncovered, scopes, tc.recorded); !slices.Equal(got, tc.want) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}
