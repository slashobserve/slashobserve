package grafana

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxVariables       = 20
	maxVariableOptions = 200
	maxIntervalOptions = 50
	maxQueryLength     = 20000
)

var (
	variableNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,39}$`)
	intervalPattern     = regexp.MustCompile(`^[1-9][0-9]{0,4}(ms|[smhdwy])$`)
	legacyReference     = regexp.MustCompile(`\[\[([A-Za-z0-9_]+)(?::([A-Za-z0-9_]+))?\]\]`)
	customPair          = regexp.MustCompile(`^\s*(.+)\s:\s(.+)$`)
)

var variableSilentKeys = map[string]bool{
	"name": true, "type": true, "label": true, "description": true, "hide": true,
	"skipUrlSync": true, "id": true, "index": true, "state": true, "global": true,
	"rootStateKey": true, "definition": true, "current": true, "options": true,
	"error": true, "queryValue": true, "tags": true, "tagsQuery": true,
	"tagValuesQuery": true, "useTags": true, "datasource": true, "refresh": true,
}

var variableHandledKeys = map[string][]string{
	"query":      {"query", "regex", "sort", "refresh", "multi", "includeAll", "allValue"},
	"custom":     {"query", "multi", "includeAll", "allValue"},
	"constant":   {"query"},
	"interval":   {"query", "auto", "auto_count", "auto_min"},
	"textbox":    {"query"},
	"datasource": {"query", "regex", "multi", "includeAll"},
	"adhoc":      {"filters", "baseFilters"},
}

type builtVar struct {
	v    spec.Variable
	path string
}

func rewriteLegacyRefs(s string) (string, []string) {
	var rewrites []string
	out := legacyReference.ReplaceAllStringFunc(s, func(match string) string {
		m := legacyReference.FindStringSubmatch(match)
		replacement := "${" + m[1] + "}"
		if m[2] != "" {
			replacement = "${" + m[1] + ":" + m[2] + "}"
		}
		rewrites = append(rewrites, match+" -> "+replacement)
		return replacement
	})
	return out, rewrites
}

func (c *converter) note(subject, reason string, paths ...string) {
	c.notes = append(c.notes, Note{Subject: subject, Status: statusPartial, Reason: reason, Paths: append([]string{}, paths...)})
}

func (c *converter) convertVariables() {
	templating, _ := asObject(c.src.dash["templating"])
	list, _ := asArray(templating["list"])
	seen := map[string]bool{}
	for i, item := range list {
		raw, ok := asObject(item)
		if !ok {
			continue
		}
		path := fmt.Sprintf("templating.list[%d]", i)
		name := asString(raw["name"])
		subject := "variable " + name
		switch {
		case !variableNamePattern.MatchString(name):
			c.droppedVars[name] = true
			c.note(subject, "The variable name is not valid in SlashObserve: use letters, digits and underscores, start with a letter and keep it under 41 characters.", path+".name")
			continue
		case seen[name]:
			c.note(subject, "Another variable already uses this name.", path+".name")
			continue
		case len(c.vars) >= maxVariables:
			c.droppedVars[name] = true
			c.note(subject, fmt.Sprintf("A SlashObserve dashboard holds at most %d variables.", maxVariables), path)
			continue
		}
		v, ok := c.convertVariable(raw, path, name)
		if !ok {
			c.droppedVars[name] = true
			continue
		}
		seen[name] = true
		v.Name = name
		c.commonVariableFields(&v, raw, path, subject)
		c.reportUnhandledKeys(raw, path, subject)
		c.vars = append(c.vars, &builtVar{v: v, path: path})
	}
	var moved []string
	c.vars, moved = orderByDependency(c.vars)
	if len(moved) > 0 {
		c.note("variables", "Variables were reordered so that each one comes after the variables its query uses.", moved...)
	}
}

func orderByDependency(vars []*builtVar) ([]*builtVar, []string) {
	byName := map[string]*builtVar{}
	for _, v := range vars {
		byName[v.v.Name] = v
	}
	var ordered []*builtVar
	state := map[string]int{}
	var visit func(v *builtVar)
	visit = func(v *builtVar) {
		if state[v.v.Name] != 0 {
			return
		}
		state[v.v.Name] = 1
		if name, ok := spec.DatasourceVariableName(v.v.Datasource); ok {
			if dep, ok := byName[name]; ok && dep != v {
				visit(dep)
			}
		}
		if refs, err := spec.ScanQuery(v.v.Query); err == nil {
			for _, ref := range spec.FlattenReferences(refs) {
				dep, ok := byName[ref.Name]
				if !ok || ref.Kind != spec.VariableRef || dep == v {
					continue
				}
				visit(dep)
			}
		}
		state[v.v.Name] = 2
		ordered = append(ordered, v)
	}
	for _, v := range vars {
		visit(v)
	}
	return ordered, reordered(vars, ordered)
}

func reordered(declared, ordered []*builtVar) []string {
	position := map[*builtVar]int{}
	for i, v := range ordered {
		position[v] = i
	}
	var paths []string
	for i, v := range declared {
		for j, other := range declared {
			if i != j && (j < i) != (position[other] < position[v]) {
				paths = append(paths, v.path)
				break
			}
		}
	}
	return paths
}

func (c *converter) dropVariable(subject, path, reason string) {
	c.note(subject, reason, path)
}

func (c *converter) commonVariableFields(v *spec.Variable, raw map[string]any, path, subject string) {
	if label := strings.TrimSpace(asString(raw["label"])); label != "" {
		var cut bool
		v.Label, cut = cutRunes(label, 100)
		if cut {
			c.note(subject, "The label was cut to 100 characters.", path+".label")
		}
	}
	if description := strings.TrimSpace(asString(raw["description"])); description != "" {
		var cut bool
		v.Description, cut = cutRunes(description, 500)
		if cut {
			c.note(subject, "The description was cut to 500 characters.", path+".description")
		}
	}
	if hide, ok := asInt(raw["hide"]); ok {
		switch hide {
		case 1:
			v.Hide = "label"
		case 2:
			v.Hide = "variable"
		}
	}
}

func (c *converter) reportUnhandledKeys(raw map[string]any, path, subject string) {
	handled := variableHandledKeys[asString(raw["type"])]
	var paths []string
	for _, key := range sortedKeys(raw) {
		if variableSilentKeys[key] || slices.Contains(handled, key) || !present(raw[key]) {
			continue
		}
		paths = append(paths, path+"."+key)
	}
	if len(paths) > 0 {
		c.note(subject, "Some Grafana options of this variable are not supported yet.", paths...)
	}
}

func (c *converter) convertVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	switch asString(raw["type"]) {
	case "query":
		return c.queryVariable(raw, path, name)
	case "custom":
		return c.customVariable(raw, path, name)
	case "constant":
		return c.constantVariable(raw, path, name)
	case "interval":
		return c.intervalVariable(raw, path, name)
	case "textbox":
		return c.textboxVariable(raw, path, name)
	case "datasource":
		return c.datasourceVariable(raw, path, name)
	case "adhoc":
		return c.adhocVariable(raw, path, name)
	}
	c.dropVariable("variable "+name, path+".type", fmt.Sprintf("Grafana variable type %q is not supported yet.", asString(raw["type"])))
	return spec.Variable{}, false
}

func variableQueryText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if m, ok := asObject(v); ok {
		if q := asString(m["query"]); q != "" {
			return q
		}
		return asString(m["rawSql"])
	}
	return ""
}

func (c *converter) listFlags(v *spec.Variable, raw map[string]any, path, subject string) {
	v.Multi = truthy(raw["multi"])
	v.IncludeAll = truthy(raw["includeAll"])
	if all := asString(raw["allValue"]); all != "" {
		if _, tooLong := cutRunes(all, 200); tooLong {
			c.note(subject, "The custom All value is longer than 200 characters and is not kept.", path+".allValue")
		} else {
			v.AllValue = all
		}
	}
}

func sortOf(c *converter, raw map[string]any, path, subject string) string {
	n, _ := asInt(raw["sort"])
	switch n {
	case 1:
		return "alpha"
	case 2:
		return "alphaDesc"
	case 3, 7:
		return "num"
	case 4, 8:
		return "numDesc"
	case 5:
		c.note(subject, "Case-insensitive sorting is not supported; the values are sorted case-sensitively.", path+".sort")
		return "alpha"
	case 6:
		c.note(subject, "Case-insensitive sorting is not supported; the values are sorted case-sensitively.", path+".sort")
		return "alphaDesc"
	}
	return ""
}

func (c *converter) queryVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	subject := "variable " + name
	ref := c.reg.lookup(raw["datasource"])
	if !ref.supported() {
		return c.fixedVariable(raw, path, name, ref)
	}
	text := variableQueryText(raw["query"])
	if strings.TrimSpace(text) == "" {
		c.dropVariable(subject, path+".query", "It has no query.")
		return spec.Variable{}, false
	}
	text, rewrites := rewriteLegacyRefs(text)
	if len(text) > maxQueryLength {
		c.dropVariable(subject, path+".query", "Its query is longer than 20000 bytes.")
		return spec.Variable{}, false
	}
	if len(rewrites) > 0 {
		c.note(subject, "Rewrote "+strings.Join(rewrites, ", ")+" in the query.", path+".query")
	}
	v := spec.Variable{Type: spec.QueryVariable, Datasource: ref.src.ref(), Query: text, Sort: sortOf(c, raw, path, subject)}
	c.variableRegex(&v, raw, path, subject)
	if refresh, ok := asInt(raw["refresh"]); ok && refresh == 2 {
		v.Refresh = "timeRange"
	} else if ok && refresh == 0 {
		c.note(subject, "Grafana never refreshes this variable's values; SlashObserve loads them with the dashboard.", path+".refresh")
	}
	c.listFlags(&v, raw, path, subject)
	v.Default = c.variableDefault(raw, path, subject, &v, nil)
	return v, true
}

func (c *converter) variableRegex(v *spec.Variable, raw map[string]any, path, subject string) {
	regex := asString(raw["regex"])
	if regex == "" {
		return
	}
	if _, tooLong := cutRunes(regex, 500); tooLong {
		c.note(subject, "The regular expression is longer than 500 characters and is not applied.", path+".regex")
		return
	}
	if _, err := spec.CompileVariableRegex(regex); err != nil {
		c.note(subject, "The regular expression is not supported and is not applied.", path+".regex")
		return
	}
	v.Regex = regex
}

func (c *converter) fixedVariable(raw map[string]any, path, name string, ref dsRef) (spec.Variable, bool) {
	subject := "variable " + name
	var options []string
	list, _ := asArray(raw["options"])
	for _, item := range list {
		if option, ok := asObject(item); ok {
			if value, ok := option["value"].(string); ok {
				options = append(options, value)
			}
		}
	}
	if len(options) == 0 {
		if current, ok := asObject(raw["current"]); ok {
			switch value := current["value"].(type) {
			case string:
				options = []string{value}
			case []any:
				options = stringList(value)
			}
		}
	}
	options = c.cleanOptions(options, subject, path)
	if len(options) == 0 {
		c.dropVariable(subject, path, fmt.Sprintf("It queries %s, which is not supported yet, and it has no saved values to keep.", ref.label()))
		return spec.Variable{}, false
	}
	c.note(subject, fmt.Sprintf("It queries %s, which is not supported yet; its saved values are kept as a fixed list.", ref.label()), path+".datasource")
	v := spec.Variable{Type: spec.CustomVariable, Options: options}
	c.listFlags(&v, raw, path, subject)
	v.Default = c.variableDefault(raw, path, subject, &v, options)
	return v, true
}

func (c *converter) cleanOptions(values []string, subject, path string) []string {
	out := make([]string, 0, len(values))
	dropped := false
	for _, value := range values {
		_, tooLong := cutRunes(value, 200)
		switch {
		case value == spec.SelectAll:
		case value == "" || tooLong:
			dropped = true
		case !slices.Contains(out, value):
			out = append(out, value)
		}
	}
	if len(out) > maxVariableOptions {
		out = out[:maxVariableOptions]
		dropped = true
	}
	if dropped {
		c.note(subject, "Some values are empty, longer than 200 characters or past the first 200, and are not kept.", path)
	}
	return out
}

func splitCustom(query string) []string {
	var parts []string
	var current strings.Builder
	runes := []rune(query)
	for i := 0; i < len(runes); i++ {
		switch {
		case runes[i] == '\\' && i+1 < len(runes) && runes[i+1] == ',':
			current.WriteRune(',')
			i++
		case runes[i] == ',':
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(runes[i])
		}
	}
	return append(parts, current.String())
}

func (c *converter) customVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	subject := "variable " + name
	var options []string
	paired := false
	for _, part := range splitCustom(asString(raw["query"])) {
		part = strings.TrimSpace(part)
		if m := customPair.FindStringSubmatch(part); m != nil {
			part = strings.TrimSpace(m[2])
			paired = true
		}
		if part != "" {
			options = append(options, part)
		}
	}
	if paired {
		c.note(subject, "Options written as text : value keep the value; the text is not shown.", path+".query")
	}
	options = c.cleanOptions(options, subject, path)
	if len(options) == 0 {
		c.dropVariable(subject, path+".query", "It has no options.")
		return spec.Variable{}, false
	}
	v := spec.Variable{Type: spec.CustomVariable, Options: options}
	c.listFlags(&v, raw, path, subject)
	v.Default = c.variableDefault(raw, path, subject, &v, options)
	return v, true
}

func (c *converter) variableDefault(raw map[string]any, path, subject string, v *spec.Variable, options []string) *spec.Selection {
	picked, isList := currentValues(raw)
	if len(picked) == 0 || slices.Contains(picked, spec.SelectAll) {
		return nil
	}
	if options != nil {
		kept := slices.DeleteFunc(slices.Clone(picked), func(value string) bool { return !slices.Contains(options, value) })
		if len(kept) < len(picked) {
			c.note(subject, "The saved selection is not one of the options, so the default selection applies.", path+".current")
		}
		if len(kept) == 0 {
			return nil
		}
		picked = kept
	}
	if !v.Multi {
		picked = picked[:1]
		isList = false
	}
	if options != nil && !v.IncludeAll && !isList && picked[0] == options[0] {
		return nil
	}
	return &spec.Selection{Values: picked, List: v.Multi && isList}
}

func currentValues(raw map[string]any) (picked []string, isList bool) {
	current, _ := asObject(raw["current"])
	var values []string
	switch value := current["value"].(type) {
	case string:
		values = []string{value}
	case []any:
		isList = true
		values = stringList(value)
	}
	for _, value := range values {
		_, tooLong := cutRunes(value, 200)
		if value != "" && !tooLong && !slices.Contains(picked, value) {
			picked = append(picked, value)
		}
	}
	return picked, isList
}

func (c *converter) constantVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	subject := "variable " + name
	value := asString(raw["query"])
	if value == "" {
		c.dropVariable(subject, path+".query", "It has no value.")
		return spec.Variable{}, false
	}
	if _, tooLong := cutRunes(value, 2000); tooLong {
		c.dropVariable(subject, path+".query", "Its value is longer than 2000 characters.")
		return spec.Variable{}, false
	}
	return spec.Variable{Type: spec.ConstantVariable, Value: value}, true
}

func intervalOptions(query string) (options []string, invalid bool) {
	for _, part := range strings.Split(query, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case !intervalPattern.MatchString(part):
			invalid = true
		case !slices.Contains(options, part):
			options = append(options, part)
		}
	}
	if len(options) > maxIntervalOptions {
		options = options[:maxIntervalOptions]
		invalid = true
	}
	return options, invalid
}

func (c *converter) intervalVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	subject := "variable " + name
	options, invalid := intervalOptions(asString(raw["query"]))
	if invalid {
		c.note(subject, "Some intervals are not valid in SlashObserve or are past the first 50, and are not kept.", path+".query")
	}
	if len(options) == 0 {
		c.dropVariable(subject, path+".query", "It has no valid intervals.")
		return spec.Variable{}, false
	}
	v := spec.Variable{Type: spec.IntervalVariable, Options: options, Auto: truthy(raw["auto"])}
	if steps, ok := asInt(raw["auto_count"]); ok && steps >= 1 && steps <= 1000 && v.Auto {
		v.AutoSteps = steps
	}
	if floor := asString(raw["auto_min"]); floor != "" && v.Auto {
		if intervalPattern.MatchString(floor) {
			v.AutoMin = floor
		} else {
			c.note(subject, "The smallest automatic interval is not valid in SlashObserve and is not kept.", path+".auto_min")
		}
	}
	if current, ok := asObject(raw["current"]); ok {
		if value := asString(current["value"]); slices.Contains(options, value) {
			v.Default = &spec.Selection{Values: []string{value}}
		}
	}
	return v, true
}

func (c *converter) textboxVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	v := spec.Variable{Type: spec.TextboxVariable}
	if text := asString(raw["query"]); text != "" {
		cut, wasCut := cutRunes(text, 200)
		if wasCut {
			c.note("variable "+name, "The default text was cut to 200 characters.", path+".query")
		}
		v.Default = &spec.Selection{Values: []string{cut}}
	}
	return v, true
}

func (c *converter) datasourceVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	subject := "variable " + name
	pluginID := asString(raw["query"])
	kind := kindOf(pluginID)
	if kind == "" {
		c.dropVariable(subject, path+".query", fmt.Sprintf("Datasource variables of type %q are not supported yet.", pluginID))
		return spec.Variable{}, false
	}
	v := spec.Variable{Type: spec.DatasourceVariable, DatasourceType: spec.DatasourceType(kind)}
	c.variableRegex(&v, raw, path, subject)
	if truthy(raw["multi"]) || truthy(raw["includeAll"]) {
		c.note(subject, "A datasource variable selects one datasource; multiple values and All are not kept.", path)
	}
	if src := c.reg.byKey["${"+name+"}"]; src != nil && src.match != "" {
		matches := true
		if v.Regex != "" {
			re, _ := spec.CompileVariableRegex(v.Regex)
			matches = re.MatchString(src.match)
		}
		if matches {
			v.Default = &spec.Selection{Values: []string{src.match}}
		} else {
			c.note(subject, fmt.Sprintf("The matched datasource %s does not satisfy the variable's regular expression, so no default is set.", src.match), path+".regex")
		}
	}
	return v, true
}

func (c *converter) adhocVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	subject := "variable " + name
	ref := c.reg.lookup(raw["datasource"])
	if !ref.supported() {
		c.dropVariable(subject, path+".datasource", fmt.Sprintf("It filters %s, which is not supported yet.", ref.label()))
		return spec.Variable{}, false
	}
	c.adhocSources[name] = ref.src
	var saved []string
	for _, key := range []string{"filters", "baseFilters"} {
		if present(raw[key]) {
			saved = append(saved, path+"."+key)
		}
	}
	if len(saved) > 0 {
		c.note(subject, "Saved filters are not kept.", saved...)
	}
	return spec.Variable{Type: spec.AdhocVariable, Datasource: ref.src.ref()}, true
}
