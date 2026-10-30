package clickhouse

import (
	"reflect"
	"testing"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestTableFilterSettings(t *testing.T) {
	logs := tableColumns{tableName{"otel", "otel_logs"}, []string{"Timestamp", "ServiceName", "Body"}}
	traces := tableColumns{tableName{"otel", "otel_traces"}, []string{"Timestamp", "ServiceName", "SpanName"}}
	service := datasource.AdhocFilter{Key: "ServiceName", Op: "=", Value: "cart"}
	span := datasource.AdhocFilter{Key: "SpanName", Op: "!~", Value: "GET .*"}
	tests := []struct {
		name    string
		tables  []tableColumns
		filters []datasource.AdhocFilter
		want    ch.Settings
	}{
		{
			"one table",
			[]tableColumns{logs}, []datasource.AdhocFilter{service},
			ch.Settings{"additional_table_filters": `{'otel.otel_logs': '(ServiceName = \'cart\')'}`},
		},
		{
			"two tables share a filter and each keeps its own",
			[]tableColumns{logs, traces}, []datasource.AdhocFilter{service, span},
			ch.Settings{"additional_table_filters": `{'otel.otel_logs': '(ServiceName = \'cart\')', 'otel.otel_traces': '(ServiceName = \'cart\' AND NOT match(SpanName, \'^(?s:GET .*)$\'))'}`},
		},
		{
			"a table without a matching column is skipped",
			[]tableColumns{logs, traces}, []datasource.AdhocFilter{span},
			ch.Settings{"additional_table_filters": `{'otel.otel_traces': '(NOT match(SpanName, \'^(?s:GET .*)$\'))'}`},
		},
		{
			"no table with a matching column sets nothing",
			[]tableColumns{logs}, []datasource.AdhocFilter{span},
			nil,
		},
		{
			"a table with no columns known sets nothing",
			[]tableColumns{{tableName{"otel", "gone"}, nil}}, []datasource.AdhocFilter{service},
			nil,
		},
		{
			"a quote and a backslash in a value are escaped for both literals",
			[]tableColumns{logs}, []datasource.AdhocFilter{{Key: "Body", Op: "=", Value: `it's a\b`}},
			ch.Settings{"additional_table_filters": `{'otel.otel_logs': '(Body = \'it\'\'s a\\\\b\')'}`},
		},
		{
			"a key qualified by the table name applies to that table only",
			[]tableColumns{logs, traces}, []datasource.AdhocFilter{{Key: "otel_traces.SpanName", Op: "=", Value: "x"}},
			ch.Settings{"additional_table_filters": `{'otel.otel_traces': '(SpanName = \'x\')'}`},
		},
		{
			"a key qualified by database and table applies to that table only",
			[]tableColumns{logs, traces}, []datasource.AdhocFilter{{Key: "otel.otel_logs.Body", Op: "=", Value: "x"}},
			ch.Settings{"additional_table_filters": `{'otel.otel_logs': '(Body = \'x\')'}`},
		},
		{
			"a key qualified by another table applies nowhere",
			[]tableColumns{logs}, []datasource.AdhocFilter{{Key: "other.Body", Op: "=", Value: "x"}},
			nil,
		},
		{
			"no filters sets nothing",
			[]tableColumns{logs}, nil,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tableFilterSettings(tt.tables, tt.filters)
			if err != nil {
				t.Fatalf("tableFilterSettings: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("settings = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTableFilterSettingsRefuseAnInvalidFilter(t *testing.T) {
	logs := tableColumns{tableName{"otel", "otel_logs"}, []string{"Body"}}
	_, err := tableFilterSettings([]tableColumns{logs}, []datasource.AdhocFilter{{Key: "Body", Op: "<", Value: "x"}})
	if err == nil {
		t.Fatal("err = nil, want an invalid filter error")
	}
}

func TestQualifyUsesTheConfiguredDatabaseForABareTable(t *testing.T) {
	d := &Datasource{database: "otel"}
	tests := []struct{ ref, database, table string }{
		{"otel_logs", "otel", "otel_logs"},
		{"other.events", "other", "events"},
	}
	for _, tt := range tests {
		if got := d.qualify(tt.ref); got != (tableName{tt.database, tt.table}) {
			t.Errorf("qualify(%q) = %+v, want %s.%s", tt.ref, got, tt.database, tt.table)
		}
	}
}
