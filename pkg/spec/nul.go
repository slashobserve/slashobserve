package spec

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

const maxNULIssues = 50

func NULIssues(canonical []byte) []Issue {
	dec := json.NewDecoder(bytes.NewReader(canonical))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil
	}
	var issues []Issue
	walkStrings(tree, "", func(pointer, kind string) bool {
		issues = append(issues, Issue{Path: pointer, Code: CodeSchema, Message: kind + " must not contain the NUL character"})
		return len(issues) < maxNULIssues
	})
	return issues
}

func walkStrings(node any, pointer string, report func(pointer, kind string) bool) bool {
	switch v := node.(type) {
	case string:
		if strings.ContainsRune(v, 0) {
			return report(pointer, "value")
		}
	case []any:
		for i, child := range v {
			if !walkStrings(child, pointer+"/"+strconv.Itoa(i), report) {
				return false
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			childPointer := pointer + "/" + escapePointer(k)
			if strings.ContainsRune(k, 0) && !report(childPointer, "property name") {
				return false
			}
			if !walkStrings(v[k], childPointer, report) {
				return false
			}
		}
	}
	return true
}
