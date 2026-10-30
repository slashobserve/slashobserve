package clickhouse

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const (
	columnCacheTTL  = time.Minute
	columnCacheCost = 1 << 20
)

var settingLiteralEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

type tableName struct{ database, table string }

func (d *Datasource) qualify(ref string) tableName {
	if database, table, ok := strings.Cut(ref, "."); ok {
		return tableName{database, table}
	}
	return tableName{d.database, ref}
}

func (t tableName) key() string { return t.database + "." + t.table }

func (d *Datasource) tableFilters(ctx context.Context, text string, filters []datasource.AdhocFilter) (ch.Settings, error) {
	if len(filters) == 0 {
		return nil, nil
	}
	refs := macros.ClickHouseTables(text)
	if len(refs) == 0 {
		slog.DebugContext(ctx, "clickhouse ad hoc filters skipped", slog.String("reason", "no table recognised"))
		return nil, nil
	}
	var tables []tableColumns
	seen := map[tableName]bool{}
	for _, ref := range refs {
		name := d.qualify(ref)
		if seen[name] {
			continue
		}
		seen[name] = true
		columns, err := d.columnsOf(ctx, name)
		if err != nil {
			return nil, err
		}
		tables = append(tables, tableColumns{name, columns})
	}
	return tableFilterSettings(tables, filters)
}

type tableColumns struct {
	name    tableName
	columns []string
}

func tableFilterSettings(tables []tableColumns, filters []datasource.AdhocFilter) (ch.Settings, error) {
	var entries []string
	for _, t := range tables {
		applicable := filtersFor(t.name, t.columns, filters)
		if len(applicable) == 0 {
			continue
		}
		condition, ok := macros.ClickHouseAdhocCondition(applicable)
		if !ok {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "invalid ad hoc filter")
		}
		entries = append(entries, settingLiteral(t.name.key())+": "+settingLiteral(condition))
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return ch.Settings{"additional_table_filters": "{" + strings.Join(entries, ", ") + "}"}, nil
}

func settingLiteral(s string) string { return "'" + settingLiteralEscaper.Replace(s) + "'" }

func filtersFor(name tableName, columns []string, filters []datasource.AdhocFilter) []datasource.AdhocFilter {
	var out []datasource.AdhocFilter
	for _, f := range filters {
		for _, prefix := range []string{"", name.table + ".", name.key() + "."} {
			if column, ok := strings.CutPrefix(f.Key, prefix); ok && slices.Contains(columns, column) {
				f.Key = column
				out = append(out, f)
				break
			}
		}
	}
	return out
}

func (d *Datasource) columnsOf(ctx context.Context, name tableName) ([]string, error) {
	if columns, _, ok := d.columns.Get(name); ok {
		return columns, nil
	}
	budget := d.clampTimeout(discoverTimeout)
	timeout := effectiveTimeout(ctx, budget)
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	rs, err := d.discoverQuery(ctx, "SELECT name FROM system.columns WHERE database = {database:String} AND table = {table:String}",
		ch.Parameters{"database": name.database, "table": name.table})
	if err != nil {
		return nil, mapError(ctx, err, timeout)
	}
	columns := make([]string, len(rs.rows))
	cost := int64(len(name.key()))
	for i, row := range rs.rows {
		columns[i] = stringOf(row[0])
		cost += int64(len(columns[i]))
	}
	d.columns.Put(name, columns, cost)
	return columns, nil
}
