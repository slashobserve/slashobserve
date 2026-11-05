# Demo shop

One Go binary, `shop`, that plays three services (`frontend`, `checkout`, `payments`) so the demo stack has live metrics, logs and traces.
It is a separate Go module so the OpenTelemetry SDK and the Prometheus client stay out of the server's dependency graph.

## Run modes

| Command | What it does |
| --- | --- |
| `shop serve` | Runs one service, chosen by `SHOP_SERVICE`. |
| `shop loadgen` | Sends seeded traffic to the frontend on a daily curve, or at a fixed rate with `LOADGEN_CURVE=flat`. |
| `shop synth` | Serves the series listed in `SYNTH_FILE` on `/metrics`, for the evals stack. |
| `shop healthcheck` | Exits 0 when `http://127.0.0.1<SHOP_ADDR>/healthz` returns 200. |

## Topology

| Service | Pods | Routes |
| --- | --- | --- |
| `frontend` | `frontend-0` | `GET /api/products`, `GET /api/products/{id}`, `POST /api/checkout`, `GET /api/orders/{id}` |
| `checkout` | `checkout-0`, `checkout-1` | `POST /checkout`, `GET /orders/{id}` |
| `payments` | `payments-0` | `POST /charge` |

The frontend round-robins the checkout pods, and checkout calls payments.

## Environment

| Variable | Used by | Meaning |
| --- | --- | --- |
| `SHOP_SERVICE` | serve | `frontend`, `checkout` or `payments` (required). |
| `SHOP_POD` | serve | Pod name, for example `checkout-0` (required); also seeds the pod's random source. |
| `SHOP_ADDR` | serve, synth, healthcheck | Listen address, default `:8080`. |
| `SYNTH_FILE` | synth | Path to a JSON list of `{"name","kind","labels"}` (required); see Synthetic series. |
| `SHOP_CHECKOUT_URLS` | frontend | Comma-separated checkout base URLs (required). |
| `SHOP_PAYMENTS_URL` | checkout | Payments base URL (required). |
| `SHOP_QUEUE_WORKERS` | checkout | Workers that drain the pod's orders queue, 1 to 100, default `4`. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | serve | OTLP/HTTP endpoint, for example `http://otel-collector:4318`; without it only stdout logging runs. |
| `LOADGEN_TARGET` | loadgen | Frontend base URL (required). |
| `LOADGEN_RPS` | loadgen | Requests per second, 1 to 1000, default `20`. |
| `LOADGEN_SEED` | loadgen | Seed for the route and cart choices, default `42`. |
| `LOADGEN_CURVE` | loadgen | `daily` (default) scales `LOADGEN_RPS` from 0.4 at 04:00 to 1.6 at 14:00 local time, mean 1.0; `flat` keeps it constant. The Prometheus backfill reads the same variable. |
| `LOADGEN_START_FILE` | loadgen | Optional path to a file holding a Unix time in seconds; loadgen waits for the file and sends its first request at that time. |

Every service exposes Prometheus metrics on `/metrics` and liveness on `/healthz`.

## Metrics

| Metric | Type | Labels | Emitted by |
| --- | --- | --- | --- |
| `http_requests_total` | counter | `service`, `route`, `pod`, `status` | every service |
| `http_request_duration_seconds` | histogram | `service`, `route`, `pod` | every service |
| `shop_orders_queue_depth` | gauge | `service`, `pod` | checkout |
| `shop_orders_queue_oldest_age_seconds` | gauge | `service`, `pod` | checkout |
| `shop_orders_processed_total` | counter | `service`, `pod`, `result` (`ok`, `failed`) | checkout |
| `shop_order_processing_seconds` | histogram | `service`, `pod` | checkout |
| `shop_product_cache_requests_total` | counter | `service`, `pod`, `result` (`hit`, `miss`) | frontend |
| `shop_db_pool_in_use` | gauge | `service`, `pod` | frontend, checkout |
| `shop_db_pool_max` | gauge | `service`, `pod` | frontend, checkout |
| `shop_db_query_duration_seconds` | histogram | `service`, `pod`, `query` (`get_product`, `reserve_stock`) | frontend, checkout |

## Orders queue

Each order a checkout pod creates goes onto that pod's in-memory orders queue, and checkout answers `201` without waiting for it, so the queue never changes the checkout route's latency.
`SHOP_QUEUE_WORKERS` workers drain it; a worker spends 800 to 1600 ms on an order, and 1% of orders fail.
Each order gets a `process_order` span under the checkout request's trace and a log line `order processed` with `order_id`, `queue_wait_ms` and `result`, at error level when it failed.
`shop_orders_queue_oldest_age_seconds` is 0 when the queue is empty.
The `queue` fault pauses workers rounded up, so with 4 workers `pct=30` pauses 2 and `pct=100` pauses all 4; with 2 of 4 paused the queue grows at the default 20 requests per second.
The queue holds at most 10000 orders; an order that finds it full is counted as `failed` and logged as `order queue full`.

## Product cache and inventory database

`GET /api/products/{id}` looks the product up in the frontend's product cache, which hits 85% of the time by the pod's seeded random source.
A miss runs a `get_product` query on the pod's simulated inventory database, and checkout's `reserve_inventory` step runs a `reserve_stock` query.
A cache lookup or query takes as long as the step it replaces (10 to 35 ms for a product, 20 to 40 ms for a reservation), so the routes keep the latency buckets the Prometheus backfill is calibrated on.
Each pod's database has a pool of 5 connections, and a query holds one for its whole duration, waiting for a free one when all are taken.
`shop_db_query_duration_seconds` includes that wait.
Each query gets a `db.query` span with `db.system=postgresql`, `db.namespace=inventory` and `db.operation.name`, and a query over 200 ms logs `slow query` at warn level with `duration_ms`.

## Fault API

Every pod keeps its own fault state, so a fault affects one pod.

| Request | Effect |
| --- | --- |
| `POST /fault?type=errors&pct=N` | N% of business requests return 500 and log `database connection refused`. |
| `POST /fault?type=latency&pct=N` | N% of business requests take 500 to 1500 ms longer. |
| `POST /fault?type=queue&pct=N` | N% of the pod's queue workers, rounded up, pause, so the orders queue's depth and oldest age rise; only checkout pods have a queue. |
| `POST /fault?type=slow_db&pct=N` | N% of the pod's database queries take 300 to 900 ms longer and hold their pool connection meanwhile, so `shop_db_pool_in_use` climbs; frontend and checkout pods have a database. |
| `GET /fault` | Returns `{"pod","service","errors_pct","latency_pct","queue_pct","slow_db_pct"}`. |
| `DELETE /fault` | Resets every fault to 0. |

`pct` is an integer from 0 to 100 and defaults to 30.
A bad `type` or `pct` returns 400 with `{"error":"..."}`.

## Synthetic series

`shop synth` reads `SYNTH_FILE`, a JSON list of `{"name","kind","labels"}`, and serves the text exposition format on `/metrics` and liveness on `/healthz`.
`kind` is `counter` or `gauge`, and `labels` is a list of label maps, one series each; an empty list gives one series with no labels.
A counter grows by 1 per second since the process started, so `rate()` is a stable 1; a gauge is always 1.
Unknown fields, a bad kind, a bad name and duplicate metric names are rejected at start.

## Kubernetes

`make demo-k8s` runs the same shop on a local three-node kind cluster named `slashobserve-demo`, next to kube-state-metrics, node-exporter and a Prometheus that carries the kube-prometheus recording rules, so the public Kubernetes dashboards have real data.
The shop keeps its contract there: one Deployment per pod name (`frontend-0`, `checkout-0`, `checkout-1`, `payments-0`) plus `loadgen`, each with the fixed `SHOP_POD`, so every pod keeps its own fault state.
The fault APIs of `checkout-0` and `checkout-1` are on http://127.0.0.1:38081 and http://127.0.0.1:38082, and Prometheus is on http://127.0.0.1:39090.

    scripts/demo-k8s.sh fault checkout-1 errors 30
    scripts/demo-k8s.sh fault checkout-1 errors 0

`make demo-k8s-down` deletes only that cluster.
`demo/k8s/README.md` lists what runs, the scrape jobs and the recording rules.
