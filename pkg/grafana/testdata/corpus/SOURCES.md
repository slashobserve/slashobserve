# Grafana golden corpus sources

Real-world Grafana dashboards the importer is tested against.
Every file is fetched from `https://raw.githubusercontent.com/<repository>/<commit>/<path>` at the commit recorded below.
Files are kept byte-for-byte as fetched, except the kube-prometheus entries, which are re-serialized from YAML.
Each source directory carries the upstream `LICENSE`; none of the upstream repositories has a `NOTICE` file.
Dashboards from grafana.com carry no licence and are excluded.
Files from the AGPL-3.0 grafana/grafana repository are excluded.
A new corpus file needs a section here and a `LICENSE` in its source directory first.

## rfmoz

- Repository: https://github.com/rfmoz/grafana-dashboards
- Commit: b173daed1bb163ca320cc3624206e07832219113
- Licence: Apache-2.0
- Fetched: 2026-10-19
- Path in repository: `prometheus/<file>.json`
- Files:
  - `apache-full.json`
  - `bind9-full.json`
  - `haproxy.json`
  - `nfs-full.json`
  - `node-exporter-bsd.json`
  - `node-exporter-full-old.json`
  - `node-exporter-full.json`
  - `unbound-full.json`

## kube-prometheus

- Repository: https://github.com/prometheus-operator/kube-prometheus
- Commit: f81c076ab8f542c457840cd4c62656b8d6b4f43a
- Licence: Apache-2.0
- Fetched: 2026-10-19
- Path in repository: `manifests/grafana-dashboardDefinitions.yaml`
- Each file is one `data` entry named `<name>.json` of that ConfigMapList, parsed and re-serialized with `json.dumps(obj, indent=2)` plus a trailing newline.
- Entries whose name contains `windows`, `aix` or `darwin` are excluded: `k8s-resources-windows-cluster.json`, `k8s-resources-windows-namespace.json`, `k8s-resources-windows-pod.json`, `k8s-windows-cluster-rsrc-use.json`, `k8s-windows-node-rsrc-use.json`, `nodes-aix.json`, `nodes-darwin.json`.
- Files:
  - `alertmanager-overview.json`
  - `apiserver.json`
  - `cluster-total.json`
  - `controller-manager.json`
  - `grafana-overview.json`
  - `k8s-resources-cluster.json`
  - `k8s-resources-multicluster.json`
  - `k8s-resources-namespace.json`
  - `k8s-resources-node.json`
  - `k8s-resources-nodes-overview.json`
  - `k8s-resources-pod.json`
  - `k8s-resources-workload.json`
  - `k8s-resources-workloads-namespace.json`
  - `kubelet.json`
  - `namespace-by-pod.json`
  - `namespace-by-workload.json`
  - `node-cluster-rsrc-use.json`
  - `node-rsrc-use.json`
  - `nodes.json`
  - `persistentvolumesusage.json`
  - `pod-total.json`
  - `prometheus-remote-write.json`
  - `prometheus.json`
  - `proxy.json`
  - `scheduler.json`
  - `workload-total.json`

## clickhouse-datasource

- Repository: https://github.com/grafana/clickhouse-datasource
- Commit: bd6c2af35bb87dba963d84dd6a331d54192284d6
- Licence: Apache-2.0
- Fetched: 2026-10-19
- Path in repository: `src/dashboards/<file>.json`
- Files:
  - `cluster-analysis.json`
  - `data-analysis.json`
  - `otel-logs-explorer.json`
  - `otel-service-dashboard.json`
  - `otel-traces-explorer.json`
  - `query-analysis.json`
  - `system-dashboards.json`

## altinity

- Repository: https://github.com/Altinity/clickhouse-grafana
- Commit: 1ae997e5dff4a2b74c6ac707b14bf79ec324d830
- Licence: MIT
- Fetched: 2026-10-19
- Path in repository: `docker/grafana/dashboards/<file>.json`
- Files:
  - `clickhouse_dashboard.json`
  - `conditionalTest_variables_issue_485.json`
  - `grafanalabs-dashboard.json`
  - `interval_variables_dashboard.json`
  - `macros_window_functions.json`
  - `repeat_by_variable_issue_712.json`
  - `single_stat_panel_403.json`
  - `table_examples_dashboard.json`
  - `test_logs_support.json`
  - `test_timeFilterMs_and_timeSeriesMs.json`

## dotdc

- Repository: https://github.com/dotdc/grafana-dashboards-kubernetes
- Commit: e4faf59e017a18e1b06fa64f11623bef15ce7507
- Licence: Apache-2.0
- Fetched: 2026-10-19
- Path in repository: `dashboards/<file>.json`
- Files:
  - `k8s-addons-prometheus.json`
  - `k8s-system-api-server.json`
  - `k8s-system-coredns.json`
  - `k8s-views-global.json`
  - `k8s-views-namespaces.json`
  - `k8s-views-nodes.json`
  - `k8s-views-pods.json`

## slashobserve

- Repository: https://github.com/slashobserve/slashobserve (this repository)
- Licence: Apache-2.0
- Authored: 2026-10-19
- Files:
  - `demo-shop.json`
- It is a classic Grafana 11 dashboard written for the demo stack and checked in unmodified.
- The ClickHouse targets set `format` to the sqlds `FormatQueryOption` value: 0 for time series, 1 for table, 2 for logs, 3 for trace.
- The `src/types/` directory of grafana/clickhouse-datasource at the commit above declares no `Format` enum.
- Its `CHQueryBase.format` is a bare `number` whose comment points at `FormatQueryOption` in https://github.com/grafana/sqlds (`query.go`, commit dda2dc0a54b128961fc9f7885baabf555f3ddfdc), which is where the values come from.
- The corpus agrees: time series panels use 0, tables use 1 and logs panels use 2.

## Panel types in the corpus

Counted over every corpus JSON file: top-level `panels`, nested row `panels`, and legacy `rows[].panels`.
The corpus holds 59 dashboards and 1311 panels including rows.
Three files exceed 100 panels including rows: `rfmoz/node-exporter-full-old.json` (191), `rfmoz/node-exporter-full.json` (141) and `rfmoz/haproxy.json` (113).

### As written

| Type | Count |
| --- | --- |
| timeseries | 762 |
| graph | 173 |
| row | 141 |
| stat | 93 |
| table | 60 |
| gauge | 27 |
| bargauge | 21 |
| singlestat | 12 |
| logs | 7 |
| heatmap | 5 |
| barchart | 3 |
| piechart | 2 |
| text | 2 |
| histogram | 1 |
| nodeGraph | 1 |
| state-timeline | 1 |

### With `graph` folded into `timeseries`, `singlestat` into `stat` and `table-old` into `table`

No `table-old` panel occurs in the corpus.

| Type | Count |
| --- | --- |
| timeseries | 935 |
| row | 141 |
| stat | 105 |
| table | 60 |
| gauge | 27 |
| bargauge | 21 |
| logs | 7 |
| heatmap | 5 |
| barchart | 3 |
| piechart | 2 |
| text | 2 |
| histogram | 1 |
| nodeGraph | 1 |
| state-timeline | 1 |
