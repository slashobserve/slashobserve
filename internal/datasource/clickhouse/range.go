package clickhouse

import (
	"slices"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	needsRange       = "a query without Timestamp must read the time range, for example WHERE $__timeFilter(EventTime)"
	timestampInRange = "$__timeFilter(Timestamp)"
)

var rangeMacros = map[string]bool{
	"__timeFilter": true, "__timeFilter_ms": true, "__dateFilter": true, "__dateTimeFilter": true, "__dt": true,
	"__fromTime": true, "__toTime": true, "__fromTime_ms": true, "__toTime_ms": true, "__from": true, "__to": true,
}

func readsRange(text string) bool {
	refs, err := spec.ScanQuery(text)
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
	return datasource.Errorf(datasource.ErrInvalidQuery, "%s", needsRange)
}

func timestampRange(env macros.Env) (string, error) {
	exp, err := macros.Expand(macros.ClickHouse, timestampInRange, env)
	if err != nil {
		return "", err
	}
	return exp.Text, nil
}
