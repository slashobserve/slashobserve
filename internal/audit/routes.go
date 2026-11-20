package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/slashobserve/slashobserve/api"
)

const (
	Extension  = "x-audit"
	APIPrefix  = "/api/v1"
	pathSource = "path."
)

type Declaration struct {
	Action Action      `json:"action,omitempty"`
	Target *TargetRule `json:"target,omitempty"`
	None   string      `json:"none,omitempty"`
}

type TargetRule struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}

func (t *TargetRule) PathParam() (string, bool) {
	if t == nil {
		return "", false
	}
	return strings.CutPrefix(t.ID, pathSource)
}

func ReadDeclaration(op *openapi3.Operation) (Declaration, bool, error) {
	raw, ok := op.Extensions[Extension]
	if !ok {
		return Declaration{}, false, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return Declaration{}, true, err
	}
	var d Declaration
	if err := json.Unmarshal(encoded, &d); err != nil {
		return Declaration{}, true, fmt.Errorf("it must be {action, target} or {none: <reason>}: %w", err)
	}
	return d, true, d.check()
}

func (d Declaration) check() error {
	switch {
	case d.Action == "" && d.None == "":
		return errors.New("it names neither an action nor none")
	case d.Action != "" && d.None != "":
		return errors.New("it names both an action and none")
	case d.None != "" && d.Target != nil:
		return errors.New("a none declaration takes no target")
	case d.Target != nil && d.Target.Kind == "":
		return errors.New("its target has no kind")
	case d.Target != nil && d.Target.ID != "" && !strings.HasPrefix(d.Target.ID, pathSource):
		return fmt.Errorf("its target id %q must name a path parameter as path.<name>", d.Target.ID)
	}
	return nil
}

type routeTable map[string]Declaration

var declaredRoutes = sync.OnceValues(func() (routeTable, error) {
	doc, err := api.Load(context.Background())
	if err != nil {
		return nil, err
	}
	routes := routeTable{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			d, ok, err := ReadDeclaration(op)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %s: %w", method, path, Extension, err)
			}
			if ok && d.Action != "" {
				routes[method+" "+APIPrefix+path] = d
			}
		}
	}
	return routes, nil
})

func (t routeTable) lookup(method, template string) (Declaration, bool) {
	d, ok := t[strings.ToUpper(method)+" "+template]
	return d, ok
}
