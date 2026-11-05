package grafana

import (
	"slices"
	"testing"
)

func TestChooseDatasourceAppliesTheImportRules(t *testing.T) {
	onePrometheus := []Datasource{{UID: "prom", Type: "prometheus"}, {UID: "ch", Type: "clickhouse"}}
	twoPrometheus := []Datasource{{UID: "prom2", Type: "prometheus", Aliases: []string{"Metrics"}}, {UID: "prom", Type: "prometheus"}}
	cases := []struct {
		name, uid, dsName, pluginID string
		ours                        []Datasource
		want                        DatasourceChoice
	}{
		{"same uid wins", "prom2", "Anything", "prometheus", twoPrometheus, DatasourceChoice{Kind: "prometheus", Candidates: []string{"prom", "prom2"}, Match: "prom2"}},
		{"alias by name", "P1809F7CD0C75ACF3", "Metrics", "prometheus", twoPrometheus, DatasourceChoice{Kind: "prometheus", Candidates: []string{"prom", "prom2"}, Match: "prom2"}},
		{"alias by uid", "Metrics", "Other", "prometheus", twoPrometheus, DatasourceChoice{Kind: "prometheus", Candidates: []string{"prom", "prom2"}, Match: "prom2"}},
		{"only datasource of the kind", "P1809F7CD0C75ACF3", "Prometheus", "prometheus", onePrometheus, DatasourceChoice{Kind: "prometheus", Candidates: []string{"prom"}, Match: "prom"}},
		{"clickhouse plugin", "ch-uid", "ClickHouse", "grafana-clickhouse-datasource", onePrometheus, DatasourceChoice{Kind: "clickhouse", Candidates: []string{"ch"}, Match: "ch"}},
		{"victoriametrics is prometheus", "vm", "VM", "victoriametrics-datasource", onePrometheus, DatasourceChoice{Kind: "prometheus", Candidates: []string{"prom"}, Match: "prom"}},
		{"two candidates and no alias", "P1809F7CD0C75ACF3", "Prometheus", "prometheus", twoPrometheus, DatasourceChoice{Kind: "prometheus", Candidates: []string{"prom", "prom2"}}},
		{"unsupported plugin", "loki", "Loki", "loki", onePrometheus, DatasourceChoice{Candidates: []string{}}},
		{"no datasource of the kind", "ch-uid", "ClickHouse", "grafana-clickhouse-datasource", twoPrometheus, DatasourceChoice{Kind: "clickhouse", Candidates: []string{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ChooseDatasource(tc.uid, tc.dsName, tc.pluginID, tc.ours)
			if got.Kind != tc.want.Kind || got.Match != tc.want.Match || got.Candidates == nil || !slices.Equal(got.Candidates, tc.want.Candidates) {
				t.Errorf("ChooseDatasource = %+v, want %+v", got, tc.want)
			}
		})
	}
}
