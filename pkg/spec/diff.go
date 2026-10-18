package spec

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

type ChangeOp string

const (
	OpAdd     ChangeOp = "add"
	OpRemove  ChangeOp = "remove"
	OpReplace ChangeOp = "replace"
	OpReorder ChangeOp = "reorder"
)

type Change struct {
	Op   ChangeOp        `json:"op"`
	Path string          `json:"path"`
	From json.RawMessage `json:"from,omitempty"`
	To   json.RawMessage `json:"to,omitempty"`
}

type keyFunc func(i int, item map[string]any) (string, bool)

func stringMember(name string) keyFunc {
	return func(_ int, item map[string]any) (string, bool) {
		k, ok := item[name].(string)
		return k, ok
	}
}

func queryRefKey(i int, item map[string]any) (string, bool) {
	var ref string
	if v, present := item["ref"]; present {
		s, ok := v.(string)
		if !ok {
			return "", false
		}
		ref = s
	}
	if ref == "" && i > 25 {
		return "", false
	}
	return resolveQueryRef(ref, i), true
}

func overrideMatchKey(_ int, item map[string]any) (string, bool) {
	match, ok := item["match"].(map[string]any)
	if !ok || len(match) != 1 {
		return "", false
	}
	for member, value := range match {
		s, ok := value.(string)
		if !ok {
			return "", false
		}
		return member + ":" + s, true
	}
	return "", false
}

func pathTokens(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path[1:], "/")
}

func matchesPattern(tokens []string, pattern ...string) bool {
	if len(tokens) != len(pattern) {
		return false
	}
	for i, p := range pattern {
		if p != "*" && p != tokens[i] {
			return false
		}
	}
	return true
}

func trimRowPrefix(tokens []string) []string {
	if len(tokens) >= 3 && tokens[0] == "panels" && tokens[2] == "panels" {
		return tokens[2:]
	}
	return tokens
}

func arrayKey(path string) (keyFunc, bool) {
	tokens := trimRowPrefix(pathTokens(path))
	switch {
	case matchesPattern(tokens, "panels"):
		return stringMember("id"), true
	case matchesPattern(tokens, "variables"):
		return stringMember("name"), true
	case matchesPattern(tokens, "panels", "*", "queries"):
		return queryRefKey, true
	case matchesPattern(tokens, "panels", "*", "overrides"):
		return overrideMatchKey, true
	}
	return nil, false
}

func positionalArray(path string) bool {
	return matchesPattern(trimRowPrefix(pathTokens(path)), "panels", "*", "transform")
}

func Diff(from, to *Dashboard) []Change {
	d := differ{changes: []Change{}}
	d.walk("", decodeTree(from), decodeTree(to))
	return d.changes
}

func decodeTree(d *Dashboard) any {
	data, err := json.Marshal(d)
	if err != nil {
		panic("spec: marshal dashboard: " + err.Error())
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		panic("spec: decode dashboard: " + err.Error())
	}
	return tree
}

type differ struct {
	changes []Change
}

func (d *differ) walk(path string, from, to any) {
	switch f := from.(type) {
	case map[string]any:
		if t, ok := to.(map[string]any); ok {
			d.walkObject(path, f, t)
			return
		}
	case []any:
		if t, ok := to.([]any); ok {
			if key, keyed := arrayKey(path); keyed && d.walkKeyed(path, key, f, t) {
				return
			}
			if positionalArray(path) && len(f) == len(t) {
				for i := range f {
					d.walk(path+"/"+strconv.Itoa(i), f[i], t[i])
				}
				return
			}
		}
	}
	if !reflect.DeepEqual(from, to) {
		d.changes = append(d.changes, Change{Op: OpReplace, Path: path, From: raw(from), To: raw(to)})
	}
}

func (d *differ) walkObject(path string, from, to map[string]any) {
	keys := make([]string, 0, len(from)+len(to))
	for k := range from {
		keys = append(keys, k)
	}
	for k := range to {
		if _, ok := from[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		child := path + "/" + escapePointer(k)
		f, inFrom := from[k]
		t, inTo := to[k]
		switch {
		case inFrom && !inTo:
			if !d.walkMissingKeyed(child, f, nil) {
				d.changes = append(d.changes, Change{Op: OpRemove, Path: child, From: raw(f)})
			}
		case !inFrom && inTo:
			if !d.walkMissingKeyed(child, nil, t) {
				d.changes = append(d.changes, Change{Op: OpAdd, Path: child, To: raw(t)})
			}
		default:
			d.walk(child, f, t)
		}
	}
}

func (d *differ) walkMissingKeyed(path string, from, to any) bool {
	key, keyed := arrayKey(path)
	if !keyed {
		return false
	}
	f, t := []any{}, []any{}
	if from != nil {
		arr, ok := from.([]any)
		if !ok {
			return false
		}
		f = arr
	}
	if to != nil {
		arr, ok := to.([]any)
		if !ok {
			return false
		}
		t = arr
	}
	return d.walkKeyed(path, key, f, t)
}

func (d *differ) walkKeyed(path string, key keyFunc, from, to []any) bool {
	fromKeys, fromByKey, ok := indexByKey(from, key)
	if !ok {
		return false
	}
	toKeys, toByKey, ok := indexByKey(to, key)
	if !ok {
		return false
	}
	for _, k := range fromKeys {
		if _, ok := toByKey[k]; !ok {
			d.changes = append(d.changes, Change{Op: OpRemove, Path: path + "/" + escapePointer(k), From: raw(fromByKey[k])})
		}
	}
	for _, k := range toKeys {
		child := path + "/" + escapePointer(k)
		if f, ok := fromByKey[k]; ok {
			d.walk(child, f, toByKey[k])
		} else {
			d.changes = append(d.changes, Change{Op: OpAdd, Path: child, To: raw(toByKey[k])})
		}
	}
	fromCommon := commonKeys(fromKeys, toByKey)
	toCommon := commonKeys(toKeys, fromByKey)
	if !slices.Equal(fromCommon, toCommon) {
		d.changes = append(d.changes, Change{Op: OpReorder, Path: path, From: raw(fromCommon), To: raw(toCommon)})
	}
	return true
}

func indexByKey(items []any, key keyFunc) ([]string, map[string]any, bool) {
	keys := make([]string, 0, len(items))
	byKey := make(map[string]any, len(items))
	for i, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, nil, false
		}
		k, ok := key(i, obj)
		if !ok {
			return nil, nil, false
		}
		if _, dup := byKey[k]; dup {
			return nil, nil, false
		}
		keys = append(keys, k)
		byKey[k] = item
	}
	return keys, byKey, true
}

func commonKeys(keys []string, other map[string]any) []string {
	common := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, ok := other[k]; ok {
			common = append(common, k)
		}
	}
	return common
}

func escapePointer(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}

func raw(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic("spec: marshal diff value: " + err.Error())
	}
	return data
}
