package grafana

import (
	"fmt"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func (c *converter) noteClickHouseAdhoc() {
	for _, v := range c.vars {
		src := c.adhocSources[v.v.Name]
		if v.v.Type != spec.AdhocVariable || src == nil || src.kind != kindClickHouse {
			continue
		}
		unfiltered, count := c.queriesWithout(v.v.Name, src)
		if count == 0 {
			continue
		}
		c.note("variable "+v.v.Name, fmt.Sprintf("Grafana applies these filters to every query on this datasource; SlashObserve applies them only in queries that use $%s, so %s not filtered.", v.v.Name, unfilteredQueries(count)), unfiltered...)
	}
}

func unfilteredQueries(count int) string {
	if count == 1 {
		return "1 query is"
	}
	return fmt.Sprintf("%d queries are", count)
}

func (c *converter) queriesWithout(variable string, src *sourceDS) ([]string, int) {
	var paths []string
	count := 0
	visit := func(bp *builtPanel) {
		for _, bq := range bp.queries {
			if bq.src != src || usesVariable(bq.q.Query, variable) {
				continue
			}
			count++
			if len(paths) < maxPanelPaths {
				paths = append(paths, bp.node.path+fmt.Sprintf(".targets[%d]", bq.index))
			}
		}
	}
	for _, bp := range c.built {
		visit(bp)
		for _, child := range bp.children {
			visit(child)
		}
	}
	return paths, count
}

func usesVariable(query, name string) bool {
	refs, err := spec.ScanQuery(query)
	if err != nil {
		return false
	}
	for _, ref := range spec.FlattenReferences(refs) {
		if ref.Kind == spec.VariableRef && ref.Name == name {
			return true
		}
	}
	return false
}
