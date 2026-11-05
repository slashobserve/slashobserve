package grafana

import "testing"

func TestMatchDatasourceAppliesTheImportRules(t *testing.T) {
	onePrometheus := []Datasource{{UID: "prom", Type: "prometheus"}, {UID: "ch", Type: "clickhouse"}}
	twoPrometheus := []Datasource{{UID: "prom", Type: "prometheus"}, {UID: "prom2", Type: "prometheus", Aliases: []string{"Metrics"}}}
	cases := []struct {
		name, uid, dsName, pluginID string
		ours                        []Datasource
		want                        string
	}{
		{"same uid wins", "prom2", "Anything", "prometheus", twoPrometheus, "prom2"},
		{"alias by name", "P1809F7CD0C75ACF3", "Metrics", "prometheus", twoPrometheus, "prom2"},
		{"alias by uid", "Metrics", "Other", "prometheus", twoPrometheus, "prom2"},
		{"only datasource of the kind", "P1809F7CD0C75ACF3", "Prometheus", "prometheus", onePrometheus, "prom"},
		{"clickhouse plugin", "ch-uid", "ClickHouse", "grafana-clickhouse-datasource", onePrometheus, "ch"},
		{"two candidates and no alias", "P1809F7CD0C75ACF3", "Prometheus", "prometheus", twoPrometheus, ""},
		{"unsupported kind", "loki", "Loki", "loki", onePrometheus, ""},
		{"no datasource of the kind", "ch-uid", "ClickHouse", "grafana-clickhouse-datasource", twoPrometheus, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchDatasource(tc.uid, tc.dsName, tc.pluginID, tc.ours); got != tc.want {
				t.Errorf("MatchDatasource = %q, want %q", got, tc.want)
			}
		})
	}
}
