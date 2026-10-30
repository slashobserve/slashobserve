package grafana

import (
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	kindPrometheus = "prometheus"
	kindClickHouse = "clickhouse"

	unmatchedUID  = "unmatched"
	defaultSource = "default"
	maxSamples    = 20
)

type refKind int

const (
	refNone refKind = iota
	refMixed
	refSpecial
	refSource
)

type dsRef struct {
	kind    refKind
	special string
	src     *sourceDS
}

type panelDS struct {
	own     dsRef
	targets []dsRef
}

type sourceDS struct {
	key        string
	name       string
	typ        string
	panels     int
	samples    []map[string]any
	variable   bool
	fromUID    bool
	kind       string
	match      string
	matchedBy  string
	candidates []string
}

type dsInput struct{ label, pluginID string }

type registry struct {
	list   []*sourceDS
	byKey  map[string]*sourceDS
	inputs map[string]dsInput
	dsVars map[string]string
}

var specialSources = map[string]struct{ key, typ string }{
	"grafana":    {"-- Grafana --", "grafana"},
	"dashboard":  {"-- Dashboard --", "dashboard"},
	"expression": {"__expr__", "__expr__"},
}

var variableForm = regexp.MustCompile(`^(?:\$\{([A-Za-z0-9_]+)(?::[^}]*)?\}|\$([A-Za-z0-9_]+)|\[\[([A-Za-z0-9_]+)(?::[^\]]*)?\]\])$`)

func kindOf(pluginID string) string {
	switch {
	case strings.Contains(pluginID, "prometheus"),
		pluginID == "victoriametrics-datasource",
		pluginID == "victoriametrics-metrics-datasource":
		return kindPrometheus
	case pluginID == "grafana-clickhouse-datasource", pluginID == "vertamedia-clickhouse-datasource":
		return kindClickHouse
	}
	return ""
}

func inferType(samples []map[string]any) string {
	for _, t := range samples {
		if _, ok := t["expr"]; ok {
			return "prometheus"
		}
	}
	for _, t := range samples {
		if t["rawSql"] != nil || t["builderOptions"] != nil || t["queryType"] != nil {
			return "grafana-clickhouse-datasource"
		}
	}
	for _, t := range samples {
		if t["query"] != nil && (t["dateTimeColDataType"] != nil || t["table"] != nil) {
			return "vertamedia-clickhouse-datasource"
		}
	}
	return ""
}

func newRegistry(src *source) *registry {
	r := &registry{byKey: map[string]*sourceDS{}, inputs: map[string]dsInput{}, dsVars: map[string]string{}}
	r.readInputs(src.dash)
	dsVarNames := r.readDatasourceVariables(src.dash)
	src.walk(func(n *panelNode) {
		if !n.row {
			r.scanPanel(n)
		}
	})
	r.scanVariables(src.dash)
	for _, name := range dsVarNames {
		src := r.get("${"+name+"}", name, r.dsVars[name])
		_, isInput := r.inputs[name]
		src.variable = !isInput
	}
	for _, s := range r.list {
		if s.typ == "" {
			s.typ = inferType(s.samples)
		}
		s.kind = kindOf(s.typ)
	}
	r.inferVariableOnlyDefault()
	return r
}

func (r *registry) inferVariableOnlyDefault() {
	s := r.byKey[defaultSource]
	if s == nil || s.panels > 0 || s.typ != "" {
		return
	}
	var typ string
	for _, other := range r.list {
		switch {
		case other.kind == "":
		case typ == "":
			typ = other.typ
		case kindOf(typ) != other.kind:
			return
		}
	}
	s.typ, s.kind = typ, kindOf(typ)
}

func (r *registry) special(name string) dsRef {
	info := specialSources[name]
	return dsRef{kind: refSpecial, special: name, src: r.get(info.key, "", info.typ)}
}

func templatingList(dash map[string]any) []any {
	templating, _ := asObject(dash["templating"])
	list, _ := asArray(templating["list"])
	return list
}

func (r *registry) readInputs(dash map[string]any) {
	inputs, _ := asArray(dash["__inputs"])
	for _, item := range inputs {
		in, ok := asObject(item)
		if !ok || asString(in["name"]) == "" || (in["type"] != nil && asString(in["type"]) != "datasource") {
			continue
		}
		r.inputs[asString(in["name"])] = dsInput{label: asString(in["label"]), pluginID: asString(in["pluginId"])}
	}
}

func (r *registry) readDatasourceVariables(dash map[string]any) []string {
	var names []string
	for _, item := range templatingList(dash) {
		v, ok := asObject(item)
		name := asString(v["name"])
		if !ok || asString(v["type"]) != "datasource" || name == "" {
			continue
		}
		if _, dup := r.dsVars[name]; !dup {
			names = append(names, name)
		}
		r.dsVars[name] = asString(v["query"])
	}
	return names
}

func (r *registry) scanVariables(dash map[string]any) {
	for _, item := range templatingList(dash) {
		v, ok := asObject(item)
		if !ok {
			continue
		}
		switch asString(v["type"]) {
		case "query", "adhoc", "groupby":
			r.variableRef(v["datasource"])
		}
	}
}

func (r *registry) get(key, name, typ string) *sourceDS {
	s, ok := r.byKey[key]
	if !ok {
		s = &sourceDS{key: key}
		r.byKey[key] = s
		r.list = append(r.list, s)
	}
	if s.name == "" {
		s.name = name
	}
	if s.typ == "" {
		s.typ = typ
	}
	return s
}

func (r *registry) classify(v any) dsRef {
	switch d := v.(type) {
	case string:
		return r.classifyParts(d, "", true)
	case map[string]any:
		uid := asString(d["uid"])
		typ := asString(d["type"])
		if uid == "" {
			switch typ {
			case "":
				return dsRef{}
			case "grafana":
				return r.special("grafana")
			case "__expr__":
				return r.special("expression")
			case "mixed":
				return dsRef{kind: refMixed}
			case "datasource":
				return dsRef{}
			}
			return dsRef{kind: refSource, src: r.get(defaultSource, "", typ)}
		}
		return r.classifyParts(uid, typ, false)
	}
	return dsRef{}
}

func (r *registry) classifyParts(id, typ string, legacyName bool) dsRef {
	switch id {
	case "":
		return dsRef{}
	case "-- Mixed --":
		return dsRef{kind: refMixed}
	case "-- Dashboard --":
		return r.special("dashboard")
	case "-- Grafana --", "grafana":
		return r.special("grafana")
	case "__expr__":
		return r.special("expression")
	}
	if m := variableForm.FindStringSubmatch(id); m != nil {
		name := m[1] + m[2] + m[3]
		if in, ok := r.inputs[name]; ok {
			return dsRef{kind: refSource, src: r.get("${"+name+"}", in.label, in.pluginID)}
		}
		if pluginID, ok := r.dsVars[name]; ok {
			src := r.get("${"+name+"}", name, pluginID)
			src.variable = true
			return dsRef{kind: refSource, src: src}
		}
	}
	if legacyName {
		return dsRef{kind: refSource, src: r.get(id, id, "")}
	}
	src := r.get(id, "", typ)
	src.fromUID = src.fromUID || id != defaultSource
	return dsRef{kind: refSource, src: src}
}

func (r *registry) variableRef(v any) dsRef {
	ref := r.classify(v)
	if ref.kind == refNone {
		ref = dsRef{kind: refSource, src: r.get(defaultSource, "", "")}
	}
	return ref
}

func (r *registry) scanPanel(n *panelNode) {
	if spec.PanelType(n.typ) == spec.AlertList {
		r.scanAlertList(n)
		return
	}
	own := r.classify(n.raw["datasource"])
	n.ds.own = own
	targets, _ := asArray(n.raw["targets"])
	counted := map[*sourceDS]bool{}
	count := func(ref dsRef, target map[string]any) {
		if ref.src == nil {
			return
		}
		if !counted[ref.src] {
			counted[ref.src] = true
			ref.src.panels++
		}
		if target != nil && len(ref.src.samples) < maxSamples {
			ref.src.samples = append(ref.src.samples, target)
		}
	}
	for _, item := range targets {
		target, _ := asObject(item)
		ref := r.classify(target["datasource"])
		if ref.kind == refNone || ref.kind == refMixed {
			ref = own
			if ref.kind == refNone || ref.kind == refMixed {
				ref = dsRef{kind: refSource, src: r.get(defaultSource, "", "")}
			}
		}
		n.ds.targets = append(n.ds.targets, ref)
		count(ref, target)
	}
	if len(targets) == 0 {
		count(own, nil)
	}
}

func (r *registry) lookup(v any) dsRef {
	ref := r.classify(v)
	if ref.kind == refNone {
		ref = dsRef{kind: refSource, src: r.byKey[defaultSource]}
		if ref.src == nil {
			return dsRef{}
		}
	}
	return ref
}

func strip(key string) string {
	return strings.TrimSuffix(strings.TrimPrefix(key, "${"), "}")
}

func groupOurs(datasources []Datasource) map[string][]Datasource {
	ours := map[string][]Datasource{}
	for _, d := range datasources {
		if kind := kindOfOurs(d.Type); kind != "" && d.UID != "" {
			ours[kind] = append(ours[kind], d)
		}
	}
	for kind := range ours {
		slices.SortFunc(ours[kind], func(a, b Datasource) int { return strings.Compare(a.UID, b.UID) })
		ours[kind] = slices.CompactFunc(ours[kind], func(a, b Datasource) bool { return a.UID == b.UID })
	}
	return ours
}

func (r *registry) resolve(opts Options) error {
	ours := groupOurs(opts.Datasources)
	for _, s := range r.list {
		for _, d := range ours[s.kind] {
			s.candidates = append(s.candidates, d.UID)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(opts.Mapping)) {
		s, ok := r.byKey[key]
		if !ok {
			return inputErrorf(codeInvalidMapping, "this dashboard does not use datasource %q", key)
		}
		if s.kind == "" {
			return inputErrorf(codeInvalidMapping, "datasource %q is not supported yet and cannot be mapped", key)
		}
		uid := opts.Mapping[key]
		if !slices.Contains(s.candidates, uid) {
			return inputErrorf(codeInvalidMapping, "mapping for %q names %q, which is not a configured %s datasource", key, uid, s.kind)
		}
		s.match, s.matchedBy = uid, "mapping"
	}
	for _, s := range r.list {
		if s.kind != "" && s.match == "" {
			s.autoMatch(ours[s.kind])
		}
	}
	return nil
}

func (s *sourceDS) autoMatch(ours []Datasource) {
	if s.fromUID && slices.Contains(s.candidates, s.key) {
		s.match, s.matchedBy = s.key, "uid"
		return
	}
	names := []string{s.key, strip(s.key)}
	if s.name != "" {
		names = append(names, s.name)
	}
	var aliased []string
	for _, d := range ours {
		if slices.ContainsFunc(d.Aliases, func(a string) bool { return slices.Contains(names, a) }) {
			aliased = append(aliased, d.UID)
		}
	}
	switch {
	case len(aliased) == 1:
		s.match, s.matchedBy = aliased[0], "alias"
	case len(aliased) == 0 && len(s.candidates) == 1:
		s.match, s.matchedBy = s.candidates[0], "only"
	}
}

func kindOfOurs(t string) string {
	switch t {
	case kindPrometheus, kindClickHouse:
		return t
	}
	return ""
}

func (r *registry) ready() bool {
	for _, s := range r.list {
		if s.kind != "" && s.match == "" {
			return false
		}
	}
	return true
}

func (r *registry) sources() []SourceDatasource {
	out := make([]SourceDatasource, 0, len(r.list))
	for _, s := range r.list {
		line := SourceDatasource{Key: s.key, Panels: s.panels, Candidates: append([]string{}, s.candidates...)}
		if s.name != "" {
			line.Name = ptr(s.name)
		}
		if s.typ != "" {
			line.Type = ptr(s.typ)
		}
		if s.kind != "" {
			line.Kind = ptr(s.kind)
		}
		if s.match != "" {
			line.Match = ptr(s.match)
			line.MatchedBy = ptr(s.matchedBy)
		}
		out = append(out, line)
	}
	return out
}

func (s *sourceDS) uid() string {
	if s.match != "" {
		return s.match
	}
	return unmatchedUID
}

func (s *sourceDS) ref() string {
	if s.variable && variableNamePattern.MatchString(strip(s.key)) {
		return s.key
	}
	return s.uid()
}

func (s *sourceDS) label() string {
	switch {
	case s.name != "":
		return s.name
	case s.typ != "":
		return capitalize(s.typ)
	}
	return "This datasource"
}

func (ref dsRef) label() string {
	switch ref.kind {
	case refSource:
		return ref.src.label()
	case refSpecial:
		if ref.special == "expression" {
			return "Server-side expressions"
		}
		return capitalize(ref.special)
	}
	return "This datasource"
}

func (ref dsRef) isDashboard() bool {
	return ref.kind == refSpecial && ref.special == "dashboard"
}

func (ref dsRef) supported() bool {
	return ref.kind == refSource && ref.src.kind != ""
}
