package grafana

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxVariables          = 20
	maxVariableNameLength = 40
	maxVariableOptions    = 200
	maxIntervalOptions    = 50
	maxQueryLength        = 20000
	maxStaticOptions      = 50
)

var (
	variableNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,39}$`)
	intervalPattern     = regexp.MustCompile(`^[1-9][0-9]{0,4}(ms|[smhdwy])$`)
	legacyReference     = regexp.MustCompile(`\[\[([A-Za-z0-9_]+)(?::([A-Za-z0-9_]+))?\]\]`)
	customPair          = regexp.MustCompile(`^\s*(.+)\s:\s(.+)$`)
	labelValuesCall     = regexp.MustCompile(`(?s)^label_values\s*\((.*)\)$`)
	labelNamePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

var variableSilentKeys = map[string]bool{
	"name": true, "type": true, "label": true, "description": true, "hide": true,
	"skipUrlSync": true, "id": true, "index": true, "state": true, "global": true,
	"rootStateKey": true, "definition": true, "current": true, "options": true,
	"error": true, "queryValue": true, "tags": true, "tagsQuery": true,
	"tagValuesQuery": true, "useTags": true, "datasource": true, "refresh": true,
}

var variableHandledKeys = map[string][]string{
	"query":      {"query", "regex", "regexApplyTo", "sort", "refresh", "multi", "includeAll", "allValue", "allowCustomValue", "staticOptions", "staticOptionsOrder"},
	"custom":     {"query", "multi", "includeAll", "allValue", "allowCustomValue"},
	"constant":   {"query"},
	"interval":   {"query", "auto", "auto_count", "auto_min"},
	"textbox":    {"query"},
	"datasource": {"query", "regex", "multi", "includeAll"},
	"adhoc":      {"filters", "baseFilters", "enableGroupBy", "defaultKeys", "allowCustomValue"},
	"groupby":    {"multi", "defaultValue"},
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
	if len(paths) == 0 {
		c.notes = append(c.notes, Note{Subject: subject, Status: statusPartial, Reason: reason, Paths: []string{}})
		return
	}
	var order []string
	groups := map[string][]string{}
	reasons := map[string]string{}
	for _, path := range paths {
		key, groupReason := "", reason
		if excluded, ok := c.exclusionAt(path); ok {
			key, groupReason = excluded.id, excluded.reason
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
			reasons[key] = groupReason
		}
		groups[key] = append(groups[key], path)
	}
	for _, key := range order {
		c.notes = append(c.notes, Note{Subject: subject, Status: statusPartial, Reason: reasons[key], Paths: groups[key]})
		for _, path := range groups[key] {
			c.options = append(c.options, optionEntry{path: path, outcome: reportedOutcome(reasons[key]), reason: reasons[key]})
		}
	}
}

func (c *converter) convertVariables() {
	templating, _ := asObject(c.src.dash["templating"])
	list, _ := asArray(templating["list"])
	seen := map[string]bool{}
	taken := declaredNames(list)
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
		if v.Type == spec.AdhocVariable {
			c.adhocGroupBy(raw, path, v, taken)
		}
	}
	c.resolveUndeclaredReferences()
	var moved []string
	c.vars, moved = orderByDependency(c.vars)
	if len(moved) > 0 {
		c.note("variables", "Variables were reordered so that each one comes after the variables its query uses.", moved...)
	}
}

func (c *converter) resolveUndeclaredReferences() {
	known := c.declaredVariables()
	declared := slices.Clone(c.vars)
	kept := c.vars[:0]
	for _, v := range declared {
		missing := undeclaredReferences(v.v, known)
		if len(missing) == 0 {
			kept = append(kept, v)
			continue
		}
		if targets, ok := listingTargets(declared, v, missing, known); ok {
			v.v.Query = renameReferences(v.v.Query, targets)
			for _, name := range missing {
				c.note("variable "+v.v.Name, renamedReferenceReason(name, targets[name]), v.path+".query")
			}
			kept = append(kept, v)
			continue
		}
		c.droppedVars[v.v.Name] = true
		c.note("variable "+v.v.Name, undeclaredReferencesReason(missing), v.path)
	}
	c.vars = kept
}

func listingTargets(declared []*builtVar, from *builtVar, missing []string, known map[string]bool) (map[string]string, bool) {
	targets := map[string]string{}
	for _, name := range missing {
		var listing []*builtVar
		for _, other := range declared {
			if label, ok := listedLabel(other.v); ok && label == name && other != from {
				listing = append(listing, other)
			}
		}
		if len(listing) != 1 || len(undeclaredReferences(listing[0].v, known)) > 0 || dependsOn(declared, listing[0], from.v.Name) {
			return nil, false
		}
		targets[name] = listing[0].v.Name
	}
	return targets, true
}

func dependsOn(declared []*builtVar, v *builtVar, name string) bool {
	byName := map[string]*builtVar{}
	for _, other := range declared {
		byName[other.v.Name] = other
	}
	seen := map[string]bool{}
	var reaches func(v *builtVar) bool
	reaches = func(v *builtVar) bool {
		if seen[v.v.Name] {
			return false
		}
		seen[v.v.Name] = true
		refs, err := spec.ScanQuery(v.v.Query)
		if err != nil {
			return false
		}
		for _, ref := range spec.FlattenReferences(refs) {
			if ref.Kind != spec.VariableRef {
				continue
			}
			if ref.Name == name {
				return true
			}
			if dep, ok := byName[ref.Name]; ok && reaches(dep) {
				return true
			}
		}
		return false
	}
	return reaches(v)
}

func listedLabel(v spec.Variable) (string, bool) {
	if v.Type != spec.QueryVariable {
		return "", false
	}
	call := labelValuesCall.FindStringSubmatch(strings.TrimSpace(v.Query))
	if call == nil {
		return "", false
	}
	args := spec.SplitMacroArgs(call[1])
	if len(args) != 2 || args[0] == "" || !labelNamePattern.MatchString(args[1]) {
		return "", false
	}
	return args[1], true
}

func renameReferences(query string, targets map[string]string) string {
	refs, err := spec.ScanQuery(query)
	if err != nil {
		return query
	}
	flat := spec.FlattenReferences(refs)
	slices.SortFunc(flat, func(a, b spec.Reference) int { return b.Start - a.Start })
	for _, ref := range flat {
		target, ok := targets[ref.Name]
		if !ok || ref.Kind != spec.VariableRef {
			continue
		}
		replacement := "$" + target
		if strings.HasPrefix(query[ref.Start:], "${") {
			replacement = "${" + target
			if ref.Format != "" {
				replacement += ":" + ref.Format
			}
			replacement += "}"
		}
		query = query[:ref.Start] + replacement + query[ref.End:]
	}
	return query
}

func renamedReferenceReason(undeclared, declared string) string {
	return fmt.Sprintf("Its query used $%s, which this dashboard does not declare; SlashObserve uses the variable %s, which lists that label, instead.", undeclared, declared)
}

func undeclaredReferences(v spec.Variable, known map[string]bool) []string {
	if v.Type != spec.QueryVariable {
		return nil
	}
	refs, err := spec.ScanQuery(v.Query)
	if err != nil {
		return nil
	}
	var missing []string
	for _, ref := range spec.FlattenReferences(refs) {
		if ref.Kind == spec.VariableRef && !known[ref.Name] && !slices.Contains(missing, ref.Name) {
			missing = append(missing, ref.Name)
		}
	}
	return missing
}

func undeclaredReferencesReason(missing []string) string {
	names := make([]string, len(missing))
	for i, name := range missing {
		names[i] = "$" + name
	}
	that := "that name"
	if len(missing) > 1 {
		that = "those names"
	}
	return fmt.Sprintf("Its query uses %s, but this dashboard declares no variable with %s, and SlashObserve cannot keep a reference to an undeclared variable.", strings.Join(names, ", "), that)
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
	v.SkipURLSync = truthy(raw["skipUrlSync"])
	if hide, ok := asInt(raw["hide"]); ok {
		switch {
		case hide == 1:
			v.Hide = "label"
		case hide == 2:
			v.Hide = "variable"
		case hide == 3 && v.Type != spec.ConstantVariable:
			c.note(subject, variableOptionsNote, path+".hide")
		}
	}
}

const variableOptionsNote = "Some Grafana options of this variable are not supported yet."

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
		c.note(subject, variableOptionsNote, paths...)
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
	case "groupby":
		return c.groupByVariable(raw, path, name)
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
	v.AllowCustomValue = allowsCustomValue(raw)
	if all := asString(raw["allValue"]); all != "" {
		if _, tooLong := cutRunes(all, 200); tooLong {
			c.note(subject, "The custom All value is longer than 200 characters and is not kept.", path+".allValue")
		} else {
			v.AllValue = all
		}
	}
}

var grafanaSortCodes = map[int]string{
	1: "alpha", 2: "alphaDesc",
	3: "num", 4: "numDesc",
	5: "alphaCaseInsensitive", 6: "alphaCaseInsensitiveDesc",
	7: "natural", 8: "naturalDesc",
}

func sortOf(raw map[string]any) string {
	n, _ := asInt(raw["sort"])
	return grafanaSortCodes[n]
}

func allowsCustomValue(raw map[string]any) bool {
	allowed, ok := raw["allowCustomValue"].(bool)
	return !ok || allowed
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
	v := spec.Variable{Type: spec.QueryVariable, Datasource: ref.src.ref(), Query: text, Sort: sortOf(raw)}
	c.queryVariableRegex(&v, raw, path, subject)
	v.StaticOptions = c.staticOptions(raw, path, subject)
	if len(v.StaticOptions) > 0 {
		v.StaticOrder = staticOrderOf(raw)
	}
	if refresh, ok := asInt(raw["refresh"]); ok && refresh == 2 {
		v.Refresh = "timeRange"
	} else if ok && refresh == 0 {
		c.neverRefresh(&v, raw, path, subject)
	}
	c.listFlags(&v, raw, path, subject)
	v.Default = c.variableDefault(raw, path, subject, &v, nil)
	return v, true
}

func staticOrderOf(raw map[string]any) string {
	switch order := asString(raw["staticOptionsOrder"]); order {
	case "before", "after", "sorted":
		return order
	}
	return ""
}

func (c *converter) staticOptions(raw map[string]any, path, subject string) []spec.StaticOption {
	list, _ := asArray(raw["staticOptions"])
	options, dropped := keepStaticOptions(nil, list)
	if dropped {
		c.note(subject, "Some static options have no text and no value, are too long or are past the first 50, and are not kept.", path+".staticOptions")
	}
	return options
}

func (c *converter) neverRefresh(v *spec.Variable, raw map[string]any, path, subject string) {
	saved, _ := asArray(raw["options"])
	options, dropped := keepStaticOptions(v.StaticOptions, saved)
	if len(options) == len(v.StaticOptions) {
		c.note(subject, "Grafana never refreshes this variable's values; SlashObserve loads them with the dashboard.", path+".refresh")
		return
	}
	v.Refresh = spec.RefreshNever
	v.StaticOptions = options
	if dropped {
		c.note(subject, "Some saved options have no text and no value, are too long or are past the first 50, and are not kept.", path+".options")
	}
}

func keepStaticOptions(kept []spec.StaticOption, list []any) (options []spec.StaticOption, dropped bool) {
	options = slices.Clone(kept)
	for _, item := range list {
		raw, ok := asObject(item)
		if !ok {
			continue
		}
		text, value := asString(raw["text"]), asString(raw["value"])
		if text == "" {
			text = value
		}
		_, textTooLong := cutRunes(text, 200)
		_, valueTooLong := cutRunes(value, 500)
		switch {
		case value == spec.SelectAll || slices.ContainsFunc(options, func(o spec.StaticOption) bool { return o.Value == value }):
		case text == "" || textTooLong || valueTooLong || len(options) >= maxStaticOptions:
			dropped = true
		default:
			options = append(options, spec.StaticOption{Text: text, Value: value})
		}
	}
	return options, dropped
}

const regexOnTextReason = "Grafana applies the regular expression to the option text, and options here carry a value only, so it is not applied."

func (c *converter) queryVariableRegex(v *spec.Variable, raw map[string]any, path, subject string) {
	if asString(raw["regexApplyTo"]) == "text" && asString(raw["regex"]) != "" {
		c.note(subject, regexOnTextReason, path+".regexApplyTo")
		return
	}
	c.variableRegex(v, raw, path, subject)
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
	var saved []string
	list, _ := asArray(raw["options"])
	for _, item := range list {
		if option, ok := asObject(item); ok {
			if value, ok := option["value"].(string); ok {
				saved = append(saved, value)
			}
		}
	}
	if len(saved) == 0 {
		if current, ok := asObject(raw["current"]); ok {
			switch value := current["value"].(type) {
			case string:
				saved = []string{value}
			case []any:
				saved = stringList(value)
			}
		}
	}
	options := c.cleanOptions(spec.PlainOptions(saved...), subject, path)
	if len(options) == 0 {
		c.dropVariable(subject, path, fmt.Sprintf("It queries %s, which is not supported yet, and it has no saved values to keep.", ref.label()))
		return spec.Variable{}, false
	}
	c.note(subject, fmt.Sprintf("It queries %s, which is not supported yet; its saved values are kept as a fixed list.", ref.label()), path+".datasource")
	v := spec.Variable{Type: spec.CustomVariable, Options: options}
	c.listFlags(&v, raw, path, subject)
	v.Default = c.variableDefault(raw, path, subject, &v, spec.OptionValues(options))
	return v, true
}

func (c *converter) cleanOptions(options []spec.VariableOption, subject, path string) []spec.VariableOption {
	out := make([]spec.VariableOption, 0, len(options))
	seen := map[string]bool{}
	dropped := false
	for _, option := range options {
		_, valueTooLong := cutRunes(option.Value, 200)
		_, textTooLong := cutRunes(option.Text, 200)
		switch {
		case option.Value == spec.SelectAll:
		case option.Value == "" || option.Text == "" || valueTooLong || textTooLong:
			dropped = true
		case !seen[option.Value]:
			seen[option.Value] = true
			out = append(out, option)
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
	var options []spec.VariableOption
	paired := false
	for _, part := range splitCustom(asString(raw["query"])) {
		part = strings.TrimSpace(part)
		option := spec.PlainOption(part)
		if m := customPair.FindStringSubmatch(part); m != nil {
			option = spec.VariableOption{Text: strings.TrimSpace(m[1]), Value: strings.TrimSpace(m[2])}
			paired = true
		}
		if option.Value != "" {
			options = append(options, option)
		}
	}
	options = c.cleanOptions(options, subject, path)
	if len(options) == 0 {
		c.dropVariable(subject, path+".query", "It has no options.")
		return spec.Variable{}, false
	}
	values := spec.OptionValues(options)
	if !paired && c.promotesToInterval(raw, name, values) {
		return spec.Variable{Type: spec.IntervalVariable, Options: options, Default: intervalDefault(raw, values)}, true
	}
	v := spec.Variable{Type: spec.CustomVariable, Options: options}
	c.listFlags(&v, raw, path, subject)
	v.Default = c.variableDefault(raw, path, subject, &v, values)
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
	v := spec.Variable{Type: spec.IntervalVariable, Options: spec.PlainOptions(options...), Auto: truthy(raw["auto"])}
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
	v.Default = intervalDefault(raw, options)
	return v, true
}

func intervalDefault(raw map[string]any, options []string) *spec.Selection {
	current, _ := asObject(raw["current"])
	if value := asString(current["value"]); slices.Contains(options, value) {
		return &spec.Selection{Values: []string{value}}
	}
	return nil
}

func (c *converter) promotesToInterval(raw map[string]any, name string, options []string) bool {
	return !truthy(raw["multi"]) && !truthy(raw["includeAll"]) &&
		len(options) <= maxIntervalOptions &&
		!slices.ContainsFunc(options, func(option string) bool { return !intervalPattern.MatchString(option) }) &&
		usedAsMinStep(c.src.dash, name)
}

func (c *converter) textboxVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	v := spec.Variable{Type: spec.TextboxVariable}
	text, textPath := asString(raw["query"]), path+".query"
	if current, _ := asObject(raw["current"]); asString(current["value"]) != "" {
		text, textPath = asString(current["value"]), path+".current"
	}
	if text != "" {
		cut, wasCut := cutRunes(text, 200)
		if wasCut {
			c.note("variable "+name, "The default text was cut to 200 characters.", textPath)
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
	if allowed, ok := raw["allowCustomValue"].(bool); ok && !allowed {
		c.note(subject, variableOptionsNote, path+".allowCustomValue")
	}
	if src := c.reg.byKey["${"+name+"}"]; src != nil && src.match != "" && matchSatisfies(v.Regex, src.match) {
		v.Default = &spec.Selection{Values: []string{src.match}}
	}
	return v, true
}

func matchSatisfies(regex, match string) bool {
	if regex == "" {
		return true
	}
	re, _ := spec.CompileVariableRegex(regex)
	return re.MatchString(match)
}

func (c *converter) adhocVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	subject := "variable " + name
	ref := c.reg.lookup(raw["datasource"])
	if !ref.supported() {
		c.dropVariable(subject, path+".datasource", fmt.Sprintf("It filters %s, which is not supported yet.", ref.label()))
		return spec.Variable{}, false
	}
	c.adhocSources[name] = ref.src
	v := spec.Variable{Type: spec.AdhocVariable, Datasource: ref.src.ref(), ApplyToAll: ref.src.kind == kindClickHouse}
	if filters := c.adhocFilterList(raw, "filters", path, subject, ref.src.kind); len(filters) > 0 {
		v.Default = &spec.Selection{Filters: filters}
	}
	v.BaseFilters = c.adhocFilterList(raw, "baseFilters", path, subject, ref.src.kind)
	c.adhocPicker(&v, raw, path, subject, ref.src.kind)
	return v, true
}
