package clickhouse

import (
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const timestampInRange = "$__timeFilter(Timestamp)"

var rangeMacros = map[string]bool{
	"__timeFilter": true, "__timeFilter_ms": true, "__dateFilter": true, "__dateTimeFilter": true, "__dt": true,
	"__fromTime": true, "__toTime": true, "__fromTime_ms": true, "__toTime_ms": true, "__from": true, "__to": true,
}

func readsRange(text string) bool {
	refs, err := macros.ExpandedReferences(macros.ClickHouse, text)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(spec.FlattenReferences(refs), func(r spec.Reference) bool {
		return r.Kind == spec.MacroRef && rangeMacros[r.Name]
	})
}

func requireRange(text string, selectsTimestamp bool) error {
	if selectsTimestamp || readsRange(text) {
		return nil
	}
	return datasource.Errorf(datasource.ErrInvalidQuery, "%s", needsRange(text))
}

func needsRange(text string) string {
	if column := knownTimeColumn(text); column != "" {
		return "add $__timeFilter(" + column + ") to the WHERE clause so the query reads only the selected time range"
	}
	return "add $__timeFilter(column) to the WHERE clause, naming the table's time column, so the query reads only the selected time range"
}

func knownTimeColumn(text string) string {
	tables := macros.ClickHouseTables(text)
	if len(tables) == 0 {
		return ""
	}
	return preferredTimeColumn(tableKind(tables[0][strings.LastIndex(tables[0], ".")+1:]))
}

func timestampRange(env macros.Env) (string, error) {
	exp, err := macros.Expand(macros.ClickHouse, timestampInRange, env)
	if err != nil {
		return "", err
	}
	return exp.Text, nil
}
