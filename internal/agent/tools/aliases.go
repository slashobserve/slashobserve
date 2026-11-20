package tools

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

type aliasRule struct {
	documented, alias string
	applies           func(tool string) bool
}

func named(word string) func(string) bool {
	return func(tool string) bool { return slices.Contains(strings.Split(tool, "_"), word) }
}

func only(tool string) func(string) bool {
	return func(name string) bool { return name == tool }
}

func everyTool(string) bool { return true }

var aliasRules = []aliasRule{
	{"datasource", "datasource_uid", everyTool},
	{"dashboard_id", "id", named("dashboard")},
	{"run_id", "id", named("run")},
	{"observation_id", "id", named("observation")},
	{"from", "from_version", only(nameDiffDashboardVersions)},
	{"to", "to_version", only(nameDiffDashboardVersions)},
	{"ids", "panel_ids", only(nameGetPanels)},
}

func aliasesOf(tool string, fields []string) map[string]string {
	aliases := map[string]string{}
	for _, rule := range aliasRules {
		if !rule.applies(tool) {
			continue
		}
		hasDocumented, hasAlias := slices.Contains(fields, rule.documented), slices.Contains(fields, rule.alias)
		switch {
		case hasDocumented && !hasAlias:
			aliases[rule.alias] = rule.documented
		case hasAlias && !hasDocumented:
			aliases[rule.documented] = rule.alias
		}
	}
	return aliases
}

func (r *Registry) canonicalInput(name string, input json.RawMessage) (json.RawMessage, Result, bool) {
	aliases := r.aliases[name]
	if len(aliases) == 0 {
		return input, Result{}, true
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(input, &fields) != nil {
		return input, Result{}, true
	}
	changed := false
	for _, alias := range slices.Sorted(maps.Keys(aliases)) {
		value, sent := fields[alias]
		if !sent {
			continue
		}
		documented := aliases[alias]
		if other, both := fields[documented]; both && !sameValue(other, value) {
			return input, invalid("%q and %q name the same field with different values; send only %q", documented, alias, documented), false
		}
		delete(fields, alias)
		fields[documented] = value
		changed = true
	}
	if !changed {
		return input, Result{}, true
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return input, Result{}, true
	}
	return out, Result{}, true
}

func sameValue(a, b json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b))
}
