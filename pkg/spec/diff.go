package spec

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
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

var keyedArrays = map[string]string{
	"/panels":    "id",
	"/variables": "name",
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
			if key, keyed := keyedArrays[path]; keyed && d.walkKeyed(path, key, f, t) {
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
			d.changes = append(d.changes, Change{Op: OpRemove, Path: child, From: raw(f)})
		case !inFrom && inTo:
			d.changes = append(d.changes, Change{Op: OpAdd, Path: child, To: raw(t)})
		default:
			d.walk(child, f, t)
		}
	}
}

func (d *differ) walkKeyed(path, key string, from, to []any) bool {
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

func indexByKey(items []any, key string) ([]string, map[string]any, bool) {
	keys := make([]string, 0, len(items))
	byKey := make(map[string]any, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, nil, false
		}
		k, ok := obj[key].(string)
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
