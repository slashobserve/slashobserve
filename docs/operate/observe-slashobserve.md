---
title: Observe SlashObserve
description: The internals dashboard that watches the server itself, what each row answers, and how compose and the Helm chart provision it.
---

The internals dashboard shows the server's own [metrics](metrics.md): its API, the agent and the model provider, the query proxy, the database pool and the scheduler.
It is an ordinary provisioned dashboard titled SlashObserve internals, in the folder SlashObserve.

## What it needs

Every panel queries the datasource with uid `prom`, so that datasource must be a Prometheus that scrapes the server's `/metrics`.
The queries have no `job` selector, so a Prometheus that scrapes several SlashObserve servers shows their sum.
Until Prometheus has scraped the server a few times, the panels return no data and the dashboard is not saved yet.
The provisioner tries it again every 30 seconds, so it appears within a minute or two of the first scrape; until then `GET /api/v1/provision` lists the file with its error.

## What each row answers

The top line holds six stats: API requests per second, the share of API requests that failed with a server error, the 95th percentile request time, agent runs and model tokens in the dashboard's range, and the share of the database pool in use.

| Row | What it shows |
| --- | --- |
| Is the API answering quickly? | Requests per second by status, request time, and the 95th percentile request time by route. |
| Is the agent working? | Agent runs and model calls per minute by outcome, the 95th percentile model call time by model, and model tokens per minute by direction. |
| Are datasource queries fast? | The 95th percentile datasource query time by datasource type, queries per second by outcome, and result cache lookups by hit or miss. |
| Is the database pool keeping up? | Pool connections by state, acquires per second, and the average wait for a connection. |
| Is the scheduler running its jobs? | Scheduled job runs in the range by job and outcome, and the jobs running now. |

A server with no model configured shows zero agent runs and model calls rather than empty panels.

## How it is provisioned

- **The compose demo** mounts `deploy/compose/dashboards` as its [provisioning directory](provisioning.md), and its Prometheus scrapes the server as the `slashobserve` job behind the `prom` datasource.
- **The Helm chart** carries its own copy of the file and mounts it as the folder `SlashObserve` while `provisioning.internals` is `true`, the default; see [Run on Kubernetes](kubernetes.md).
  Add a datasource with uid `prom` under `datasources` that points at a Prometheus scraping the pod.
  `make lint` fails when the chart's copy differs from the compose copy.
- **Any other install** can copy `deploy/compose/dashboards/SlashObserve/internals.json` into a `SlashObserve` directory of its provisioning directory.

The dashboard is provisioned like any other file, so an edit made in the app is replaced the next time the file changes, and stays in the version history.
