# Kubernetes demo

The shop and a metrics stack on a local three-node kind cluster, so the public Kubernetes dashboards have real `kube_*`, kubelet, cAdvisor, control-plane and node data to verify against.
This is a local demo: nothing here is hardened or meant for a shared cluster.
`scripts/demo-k8s.sh up` (or `make demo-k8s`) creates the cluster `slashobserve-demo`, builds the shop image, loads it into the nodes and applies these files; `down` deletes only that cluster.

## What runs

| File | Namespace | What |
| --- | --- | --- |
| `shop.yaml` | `default` | One Deployment and one Service per shop pod name (`frontend-0`, `checkout-0`, `checkout-1`, `payments-0`), plus `loadgen`. |
| `kube-state-metrics.yaml` | `kube-system` | kube-state-metrics with its ServiceAccount and read-only ClusterRole. |
| `node-exporter.yaml` | `kube-system` | node-exporter as a DaemonSet on every node, the control-plane included. |
| `prometheus.yaml` | `monitoring` | Prometheus with its scrape configuration, and the namespace itself. |
| `rules/*.yml` | `monitoring` | Recording rules, loaded by the script into the ConfigMap `prometheus-rules`, one key per file. |

The shop keeps the compose contract: the same pod names, routes and metric labels.
Each pod is its own Deployment with one replica because the fault API is per pod and `SHOP_POD` seeds the pod's random source.
The shop runs in `default` because the kube-prometheus dashboards open on the first namespace in alphabetical order, which is `default`, and a namespace with no workload leaves their namespace, pod and workload panels empty.
The shop image is `slashobserve-demo-shop:k8s`, built from `demo/` and loaded into the nodes, so the Deployments use `imagePullPolicy: IfNotPresent`.
Nothing is sent over OTLP; the shop logs to stdout.

## Ports

| Service | NodePort | Host address |
| --- | --- | --- |
| `checkout-0` fault API | 30081 | `127.0.0.1:38081` |
| `checkout-1` fault API | 30082 | `127.0.0.1:38082` |
| Prometheus | 30090 | `127.0.0.1:39090` |

The script publishes the NodePorts of the control-plane node on the host ports above; `SLASHOBSERVE_DEMO_K8S_SHOP_PORT`, `SLASHOBSERVE_DEMO_K8S_SHOP1_PORT` and `SLASHOBSERVE_DEMO_K8S_PROMETHEUS_PORT` override them.

## Faults

The fault API is the one described in `demo/README.md`, and every pod keeps its own fault state.

```
scripts/demo-k8s.sh fault checkout-1 errors 30
scripts/demo-k8s.sh fault checkout-1 errors 0
```

## Scrape jobs

Prometheus uses the job names the public dashboards filter on.
Every series carries `cluster="slashobserve-demo"`, set by a relabel rule in each job and by `external_labels`, because the recording rules group by `cluster`.

| Job | Source |
| --- | --- |
| `kube-state-metrics` | The kube-state-metrics service, keeping its own `namespace` and `pod` labels. |
| `node-exporter` | The DaemonSet pods, with `instance` set to the node name. |
| `kubelet` | Each node's `/metrics`, and its `/metrics/cadvisor` under the same job name; `metrics_path` tells them apart. |
| `apiserver` | The `kubernetes` service. |
| `kube-controller-manager`, `kube-scheduler` | The control-plane node on ports 10257 and 10259. |
| `kube-proxy` | The kube-proxy pods on port 10249. |
| `coredns` | The CoreDNS pods on port 9153. |
| `shop` | The shop pods every 5 seconds, keeping the application's own `pod` label. |
| `prometheus` | The Prometheus pod itself, for the Prometheus dashboard. |

The control-plane components and kube-proxy only answer on the node address because `demo/k8s/kind.yaml` binds their metrics to `0.0.0.0`.
Prometheus keeps 6 hours of data in an `emptyDir`, capped at 400 MB, so a restart starts empty.

## Recording rules

The files in `rules/` hold the recording rule groups of the kube-prometheus manifests `kubernetesControlPlane-prometheusRule.yaml`, `nodeExporter-prometheusRule.yaml` and `kubePrometheus-prometheusRule.yaml` at the commit recorded in `pkg/grafana/testdata/corpus/SOURCES.md`.
Each group is copied unchanged.
Only the `PrometheusRule` wrapper is replaced by a plain `groups:` key, and groups that hold alerts are left out.
`NOTICE` credits them.
