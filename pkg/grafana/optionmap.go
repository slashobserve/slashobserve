package grafana

import (
	_ "embed"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
)

//go:embed optionmap.json
var optionMapJSON []byte

type optionFields struct {
	Fields []string            `json:"fields"`
	Values map[string][]string `json:"values,omitempty"`
}

type optionMap map[optionScope]map[string]optionFields

var loadedOptionMap = sync.OnceValue(func() optionMap {
	var m optionMap
	if err := json.Unmarshal(optionMapJSON, &m); err != nil {
		panic("optionmap.json: " + err.Error())
	}
	return m.withWildcards()
})

func unionSorted(lists ...[]string) []string {
	out := []string{}
	for _, l := range lists {
		out = append(out, l...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (m optionMap) withWildcards() optionMap {
	out := make(optionMap, len(m)*2)
	for scope, options := range m {
		out[scope] = options
	}
	for scope, options := range m {
		wild := scope.wildcard()
		if wild == scope {
			continue
		}
		if out[wild] == nil {
			out[wild] = map[string]optionFields{}
		}
		for rel, entry := range options {
			merged := out[wild][rel]
			merged.Fields = unionSorted(merged.Fields, entry.Fields)
			for key, fields := range entry.Values {
				if merged.Values == nil {
					merged.Values = map[string][]string{}
				}
				merged.Values[key] = unionSorted(merged.Values[key], fields)
				merged.Fields = unionSorted(merged.Fields, fields)
			}
			out[wild][rel] = merged
		}
	}
	return out
}

func ancestors(rel string) []string {
	var out []string
	for rel != "" {
		out = append(out, rel)
		cut := strings.LastIndexAny(rel, ".[")
		if cut < 0 {
			break
		}
		rel = rel[:cut]
	}
	return out
}

func discriminated(values map[string][]string) bool {
	if len(values) == 0 {
		return false
	}
	for key := range values {
		if !strings.HasPrefix(key, "id:") && !strings.HasPrefix(key, "type:") {
			return false
		}
	}
	return true
}

func (m optionMap) specFields(p scopedPath, valueAt func(rel string) (any, bool)) (fields []string, option string, found bool) {
	for _, candidate := range ancestors(p.rel) {
		key := normalizePath(candidate)
		entry, ok := m[p.scope][key]
		if !ok {
			entry, ok = m[p.scope.wildcard()][key]
		}
		if !ok {
			continue
		}
		value, _ := valueAt(candidate)
		if refined, ok := entry.Values[valueKey(value)]; ok {
			return refined, candidate, true
		}
		if discriminated(entry.Values) {
			return nil, candidate, true
		}
		return entry.Fields, candidate, true
	}
	return nil, "", false
}

func valueKey(v any) string {
	switch x := v.(type) {
	case string:
		return "s:" + x
	case bool:
		return "b:" + strconv.FormatBool(x)
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return ""
		}
		return numberKey(f)
	case float64:
		return numberKey(x)
	case int:
		return numberKey(float64(x))
	case map[string]any:
		if id, ok := x["id"].(string); ok {
			return "id:" + id
		}
		if typ, ok := x["type"].(string); ok {
			return "type:" + typ
		}
	}
	return ""
}

func numberKey(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ""
	}
	return "n:" + strconv.FormatFloat(f, 'f', -1, 64)
}
