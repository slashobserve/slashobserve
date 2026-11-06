package public

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type Datasource struct {
	UID  string
	Type string
}

func publicSpec(d *spec.Dashboard) (*spec.Dashboard, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	out := &spec.Dashboard{}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	for _, e := range out.AllPanels() {
		for i := range e.Panel.Queries {
			e.Panel.Queries[i].Query = ""
		}
	}
	for i := range out.Variables {
		out.Variables[i].Query = ""
	}
	return out, nil
}

func referencedDatasources(d *spec.Dashboard, types map[string]string) []Datasource {
	uids := map[string]bool{}
	literal := func(ref string) {
		if _, isVariable := spec.DatasourceVariableName(ref); ref != "" && !isVariable {
			uids[ref] = true
		}
	}
	variableTypes := map[string]bool{}
	for _, v := range d.Variables {
		literal(v.Datasource)
		if v.Type == spec.DatasourceVariable {
			variableTypes[string(v.DatasourceType)] = true
		}
	}
	for _, e := range d.AllPanels() {
		literal(e.Panel.Datasource)
		for _, q := range e.Panel.Queries {
			literal(q.Datasource)
		}
	}
	out := []Datasource{}
	for uid, typ := range types {
		if uids[uid] || variableTypes[typ] {
			out = append(out, Datasource{UID: uid, Type: typ})
		}
	}
	slices.SortFunc(out, func(a, b Datasource) int { return strings.Compare(a.UID, b.UID) })
	return out
}
