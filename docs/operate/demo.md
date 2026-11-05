---
title: The demo shop
description: The services, signals and faults of the demo stack's shop, so you know what to ask for and what to break.
---

The demo stack from the [quick start](../get-started/quick-start.md) runs a small shop with three services: `frontend` (pod `frontend-0`), `checkout` (pods `checkout-0` and `checkout-1`) and `payments` (pod `payments-0`).
A load generator sends it traffic all the time, so every chart has live data.
Its metrics are in the `prom` datasource, and its logs and traces are in `ch`.

## Metrics

| Metric | Type | Labels | What it shows |
| --- | --- | --- | --- |
| `http_requests_total` | counter | `service`, `route`, `pod`, `status` | Requests each route handled. |
| `http_request_duration_seconds` | histogram | `service`, `route`, `pod` | How long each route took. |
| `shop_orders_queue_depth` | gauge | `service`, `pod` | Orders waiting in a checkout pod's orders queue. |
| `shop_orders_queue_oldest_age_seconds` | gauge | `service`, `pod` | How long the oldest waiting order has waited, 0 when none waits. |
| `shop_orders_processed_total` | counter | `service`, `pod`, `result` | Orders the queue workers finished, with `result` `ok` or `failed`. |
| `shop_order_processing_seconds` | histogram | `service`, `pod` | How long a worker spent on one order. |
| `shop_product_cache_requests_total` | counter | `service`, `pod`, `result` | Frontend product cache lookups, with `result` `hit` (about 85%) or `miss`. |
| `shop_db_pool_in_use` | gauge | `service`, `pod` | Inventory database connections in use on a frontend or checkout pod. |
| `shop_db_pool_max` | gauge | `service`, `pod` | The size of that pod's connection pool, 5. |
| `shop_db_query_duration_seconds` | histogram | `service`, `pod`, `query` | How long a database query took, with `query` `get_product` (a cache miss) or `reserve_stock` (a checkout). |

## Logs and traces

Every request logs `request handled` and has a trace that follows it from the frontend through checkout to payments.
Checkout answers without waiting for the order: the order waits in the pod's orders queue, and a worker processes it in a `process_order` span under the same trace.
That span logs `order processed` with `order_id`, `queue_wait_ms` and `result`.
Every database query has a `db.query` span with `db.system=postgresql`, and a query slower than 200 ms logs `slow query` at warn level.

## Faults

Each fault affects one pod, the one whose fault API you call.
`make demo-fault TYPE=<type> PCT=N` sets a fault on `checkout-0` through http://localhost:8081/fault, and `make demo-fault-reset` clears every fault.

| Type | What `PCT=N` does |
| --- | --- |
| `errors` | N% of requests fail with 500 and log `database connection refused`. |
| `latency` | N% of requests take 500 to 1500 ms longer. |
| `queue` | N% of the pod's 4 queue workers, rounded up, stop taking orders, so the queue's depth and oldest age climb; at `PCT=30` 2 workers stop. |
| `slow_db` | N% of the pod's database queries take 300 to 900 ms longer and keep their connection meanwhile, so the pool's connections in use climb and requests that query get slower. |

`GET /fault` shows every fault's current percentage.
