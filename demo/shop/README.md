# Demo shop

One Go binary, `shop`, that plays three services (`frontend`, `checkout`, `payments`) so the demo stack has live metrics, logs and traces.
It is a separate Go module so the OpenTelemetry SDK and the Prometheus client stay out of the server's dependency graph.

## Run modes

| Command | What it does |
| --- | --- |
| `shop serve` | Runs one service, chosen by `SHOP_SERVICE`. |
| `shop loadgen` | Sends fixed-rate, seeded traffic to the frontend. |
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
| `SHOP_ADDR` | serve, healthcheck | Listen address, default `:8080`. |
| `SHOP_CHECKOUT_URLS` | frontend | Comma-separated checkout base URLs (required). |
| `SHOP_PAYMENTS_URL` | checkout | Payments base URL (required). |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | serve | OTLP/HTTP endpoint, for example `http://otel-collector:4318`; without it only stdout logging runs. |
| `LOADGEN_TARGET` | loadgen | Frontend base URL (required). |
| `LOADGEN_RPS` | loadgen | Requests per second, 1 to 1000, default `20`. |
| `LOADGEN_SEED` | loadgen | Seed for the route and cart choices, default `42`. |
| `LOADGEN_START_FILE` | loadgen | Optional path to a file holding a Unix time in seconds; loadgen waits for the file and sends its first request at that time. |

Every service exposes Prometheus metrics on `/metrics` (`http_requests_total` and `http_request_duration_seconds`, labelled `service`, `route`, `pod` and `status` for the counter) and liveness on `/healthz`.

## Fault API

Every pod keeps its own fault state, so a fault affects one pod.

| Request | Effect |
| --- | --- |
| `POST /fault?type=errors&pct=N` | N% of business requests return 500 and log `database connection refused`. |
| `POST /fault?type=latency&pct=N` | N% of business requests take 500 to 1500 ms longer. |
| `GET /fault` | Returns `{"pod","service","errors_pct","latency_pct"}`. |
| `DELETE /fault` | Resets both faults to 0. |

`pct` is an integer from 0 to 100 and defaults to 30.
A bad `type` or `pct` returns 400 with `{"error":"..."}`.
