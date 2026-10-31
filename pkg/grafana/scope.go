package grafana

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type optionScope string

const scopeDashboard optionScope = "dashboard"

func panelScope(t spec.PanelType) optionScope { return optionScope("panel:" + string(t)) }

func queryScope(k queryKind) optionScope { return optionScope("query:" + string(k)) }

func transformScope(id string) optionScope { return optionScope("transform:" + id) }

func variableScope(typ string) optionScope { return optionScope("variable:" + typ) }

func (s optionScope) kind() string {
	kind, _, _ := strings.Cut(string(s), ":")
	return kind
}

func (s optionScope) wildcard() optionScope {
	if s == scopeDashboard {
		return s
	}
	return optionScope(s.kind() + ":*")
}

type scopedPath struct {
	scope optionScope
	root  string
	rel   string
}

var (
	variableRoot       = regexp.MustCompile(`^templating\.list\[([0-9]+)\]`)
	targetRoot         = regexp.MustCompile(`^targets\[([0-9]+)\]`)
	transformationRoot = regexp.MustCompile(`^transformations\[([0-9]+)\]`)
)

func isUnder(path, root string) bool {
	if !strings.HasPrefix(path, root) {
		return false
	}
	rest := path[len(root):]
	return rest == "" || rest[0] == '.' || rest[0] == '['
}

func relativeTo(path, root string) string {
	return strings.TrimPrefix(path[len(root):], ".")
}

func rootMatch(re *regexp.Regexp, path string) (root string, index int, ok bool) {
	m := re.FindStringSubmatch(path)
	if m == nil || !isUnder(path, m[0]) {
		return "", 0, false
	}
	index, _ = strconv.Atoi(m[1])
	return m[0], index, true
}

func (s *source) scopeOf(path string) scopedPath {
	if root, _, ok := rootMatch(variableRoot, path); ok {
		item, _ := lookup(s.dash, root)
		variable, _ := asObject(item)
		return scopedPath{variableScope(asString(variable["type"])), root, relativeTo(path, root)}
	}
	var deepest *panelNode
	s.walk(func(n *panelNode) {
		if isUnder(path, n.path) && (deepest == nil || len(n.path) > len(deepest.path)) {
			deepest = n
		}
	})
	if deepest == nil {
		return scopedPath{scopeDashboard, "", path}
	}
	rest := relativeTo(path, deepest.path)
	if local, k, ok := rootMatch(targetRoot, rest); ok {
		return targetScope(deepest, k, path, deepest.path+"."+local)
	}
	if local, k, ok := rootMatch(transformationRoot, rest); ok {
		root := deepest.path + "." + local
		return scopedPath{transformScope(transformationID(deepest, k)), root, relativeTo(path, root)}
	}
	return scopedPath{deepest.scope(), deepest.path, rest}
}

func (n *panelNode) scope() optionScope {
	switch {
	case n.row:
		return panelScope(spec.Row)
	case n.sourceType != "":
		return panelScope(spec.PanelType(n.sourceType))
	}
	return panelScope(spec.PanelType(n.typ))
}

func targetScope(n *panelNode, k int, path, root string) scopedPath {
	var ref dsRef
	if k < len(n.ds.targets) {
		ref = n.ds.targets[k]
	}
	items, _ := asArray(n.raw["targets"])
	var target map[string]any
	if k < len(items) {
		target, _ = asObject(items[k])
	}
	return scopedPath{queryScope(queryKindOf(ref, target)), root, relativeTo(path, root)}
}

func transformationID(n *panelNode, k int) string {
	items, _ := asArray(n.raw["transformations"])
	if k >= len(items) {
		return ""
	}
	transformation, _ := asObject(items[k])
	return asString(transformation["id"])
}

func (s *source) valueAt(root, rel string) (any, bool) {
	switch {
	case root == "" && rel == "":
		return s.dash, true
	case rel == "":
		return lookup(s.dash, root)
	case root == "":
		return lookup(s.dash, rel)
	case rel[0] == '[':
		return lookup(s.dash, root+rel)
	}
	return lookup(s.dash, root+"."+rel)
}
