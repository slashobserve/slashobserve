package db_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const evolvedDatasourceSpec = `{
  "specVersion": "0",
  "title": "Datasources",
  "time": {"from": "now-1h", "to": "now"},
  "variables": [
    {"name": "pod", "type": "query", "datasource": "var-ds", "query": "label_values(up, pod)"},
    {"name": "filters", "type": "adhoc", "datasource": "A-adhoc"},
    {"name": "env", "type": "custom", "options": ["a", "b"]}
  ],
  "panels": [
    {"id": "p1", "type": "timeseries", "title": "p1", "gridPos": {"x": 0, "y": 0, "w": 12, "h": 8}, "datasource": "prom",
      "queries": [{"query": "up"}, {"query": "up", "datasource": "b_over"}, {"query": "up", "datasource": "prom"}]},
    {"id": "p2", "type": "stat", "title": "p2", "gridPos": {"x": 12, "y": 0, "w": 12, "h": 8}, "datasource": "a-ds",
      "queries": [{"query": "up"}]},
    {"id": "t1", "type": "text", "gridPos": {"x": 0, "y": 8, "w": 12, "h": 4}, "content": "hello"},
    {"id": "row", "type": "row", "title": "row", "gridPos": {"x": 0, "y": 12, "w": 24, "h": 1}, "panels": [
      {"id": "c1", "type": "timeseries", "title": "c1", "gridPos": {"x": 0, "y": 13, "w": 12, "h": 8}, "datasource": "child_ds",
        "queries": [{"query": "up"}, {"query": "up", "datasource": "a_over"}]},
      {"id": "c2", "type": "gauge", "title": "c2", "gridPos": {"x": 12, "y": 13, "w": 12, "h": 8}, "datasource": "prom",
        "queries": [{"query": "up"}]}
    ]}
  ]
}`

func TestDatasourceUIDsFunctionReadsEveryEvolvedPathInByteOrder(t *testing.T) {
	if _, err := spec.Parse([]byte(evolvedDatasourceSpec)); err != nil {
		t.Fatalf("the fixture must be a valid evolved Spec: %v", err)
	}
	sqlDB := dbtest.New(t).SQLDB(t)
	tests := []struct {
		name string
		spec string
		want []string
	}{
		{"every evolved path, sorted in byte order and deduplicated", evolvedDatasourceSpec,
			[]string{"A-adhoc", "a-ds", "a_over", "b_over", "child_ds", "prom", "var-ds"}},
		{"empty object", `{}`, []string{}},
		{"empty panels", `{"panels":[]}`, []string{}},
		{"legacy object datasources are not read", `{"panels":[{"datasource":{"type":"prometheus","uid":"old"}}],"variables":[{"datasource":{"uid":"old"}}]}`, []string{}},
		{"empty uids are skipped", `{"panels":[{"datasource":"","queries":[{"datasource":""}]}]}`, []string{}},
		{"mixed case and separators follow the C collation", `{"panels":[
			{"datasource":"b"},{"datasource":"a_x"},{"datasource":"B"},{"datasource":"a-x"},{"datasource":"A-x"},{"datasource":"a-x"},{"datasource":"1z"}]}`,
			[]string{"1z", "A-x", "B", "a-x", "a_x", "b"}},
		{"a referenced datasource variable lists its default", `{"variables":[{"name":"ds","type":"datasource","default":"prom2"}],"panels":[{"datasource":"${ds}"}]}`, []string{"prom2"}},
		{"a reference on a query override pulls in the default", `{"variables":[{"name":"ds","type":"datasource","default":"p"}],"panels":[{"queries":[{"datasource":"${ds}"}]}]}`, []string{"p"}},
		{"a reference on a row child query pulls in the default", `{"variables":[{"name":"ds","type":"datasource","default":"p"}],"panels":[{"panels":[{"queries":[{"datasource":"${ds}"}]}]}]}`, []string{"p"}},
		{"a reference on a row child panel pulls in the default", `{"variables":[{"name":"ds","type":"datasource","default":"p"}],"panels":[{"panels":[{"datasource":"${ds}"}]}]}`, []string{"p"}},
		{"a reference on a query variable pulls in the default", `{"variables":[{"name":"ds","type":"datasource","default":"p"},{"name":"pod","type":"query","datasource":"${ds}"}]}`, []string{"p"}},
		{"a reference on an adhoc variable pulls in the default", `{"variables":[{"name":"ds","type":"datasource","default":"p"},{"name":"f","type":"adhoc","datasource":"${ds}"}]}`, []string{"p"}},
		{"a default that nothing references is not listed", `{"variables":[{"name":"ds","type":"datasource","default":"p"}],"panels":[{"type":"text"}]}`, []string{}},
		{"a referenced variable without a default adds nothing", `{"variables":[{"name":"ds","type":"datasource"}],"panels":[{"datasource":"${ds}"},{"datasource":"prom"}]}`, []string{"prom"}},
		{"a reference to an undeclared variable is not listed", `{"panels":[{"datasource":"${ds}"}]}`, []string{}},
		{"a literal uid equal to a referenced default is listed once", `{"variables":[{"name":"ds","type":"datasource","default":"prom"}],"panels":[{"datasource":"${ds}"},{"datasource":"prom"}]}`, []string{"prom"}},
		{"a non-string default adds nothing", `{"variables":[{"name":"ds","type":"datasource","default":["a"]}],"panels":[{"datasource":"${ds}"}]}`, []string{}},
		{"a default on a variable that is not a datasource variable adds nothing", `{"variables":[{"name":"ds","type":"custom","default":"p"}],"panels":[{"datasource":"${ds}"}]}`, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw string
			err := sqlDB.QueryRowContext(t.Context(), `SELECT array_to_json(observespec_datasource_uids($1::jsonb))::text`, tt.spec).Scan(&raw)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
