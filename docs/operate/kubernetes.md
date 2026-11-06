---
title: Run on Kubernetes
description: Install the server with the Helm chart, on an external Postgres, with every secret taken from a Secret you create.
---

The Helm chart in `deploy/helm/slashobserve` runs the server as one Deployment with a Service.
It ships no Postgres and creates no Secret: you bring the database and the credentials, and the chart only references them.
It provisions the [internals dashboard](observe-slashobserve.md) by default.

## Prerequisites

- A Postgres database the cluster can reach, and its connection string.
  See [Upgrades and backups](upgrades-and-backups.md) for backing it up.
- An OIDC client at your identity provider with the redirect URI `<public url>/api/v1/auth/callback`; see [Sign-in with OIDC](../connect/oidc.md).
- Helm 3 or 4 and `kubectl` pointed at the cluster.
- A Prometheus that scrapes the pod, and a datasource with uid `prom` that reads from it, if you want the internals dashboard to show data.

## Create the Secrets

Create each Secret in the release namespace before installing.
Write each value to a file first, so it stays out of your shell history, and delete the files afterwards.

```sh
kubectl create namespace slashobserve
kubectl -n slashobserve create secret generic slashobserve-database --from-file=url=./database-url
kubectl -n slashobserve create secret generic slashobserve-oidc --from-file=client-secret=./oidc-client-secret
kubectl -n slashobserve create secret generic slashobserve-secret-key --from-file=secret-key=./secret-key
kubectl -n slashobserve create secret generic slashobserve-llm --from-file=api-key=./llm-api-key
```

- `database-url` holds the connection string, such as `postgres://slashobserve:<password>@<host>:5432/slashobserve?sslmode=require`.
- `oidc-client-secret` holds the client secret from your identity provider.
- `secret-key` is optional and encrypts stored Grafana connection tokens; generate it with `openssl rand -hex 32`.
- `llm-api-key` is optional and holds the model provider's key, which the chart mounts as a file for `SLASHOBSERVE_LLM_API_KEY_FILE`.

The key names above are the chart's defaults; `database.urlKey`, `oidc.clientSecretKey`, `secretKey.key` and `llm.apiKey.key` change them.

## Install

Write a values file with the required settings:

```yaml
publicURL: https://slashobserve.example.com
database:
  existingSecret: slashobserve-database
oidc:
  issuerURL: https://idp.example.com
  clientID: slashobserve
  existingSecret: slashobserve-oidc
auth:
  allowedEmailDomains: [example.com]
  adminEmails: [you@example.com]
secretKey:
  existingSecret: slashobserve-secret-key
datasources:
  datasources:
    - uid: prom
      type: prometheus
      name: Prometheus
      prometheus:
        url: http://prometheus.monitoring:9090
```

Install from a checkout of the release tag you run:

```sh
helm install slashobserve deploy/helm/slashobserve --namespace slashobserve -f values.yaml --wait
```

The image defaults to `ghcr.io/slashobserve/slashobserve` at the chart's `appVersion`; set `image.tag`, and `image.digest` to pin the digest too.
`values.schema.json` in the chart describes every value, and the install fails before it reaches the cluster when a required one is missing.
The chart sets the environment variables of the [configuration reference](../reference/configuration.md) from the values; add any other `SLASHOBSERVE_*` setting under `env`, in Kubernetes `EnvVar` form.

To enable the agent, set `llm.provider`, `llm.model` when the provider needs one, and `llm.apiKey.existingSecret`.

## Datasources

`datasources.datasources` holds the entries of the [datasources file](../reference/datasources-file.md), which the chart renders into a ConfigMap; `datasources.existingConfigMap` names a ConfigMap of your own instead, with the file under the key `datasources.key`.
The file never holds a secret.
Reference a password by an `*Env` name and set that variable from a Secret under `env`:

```yaml
env:
  - name: SLASHOBSERVE_CLICKHOUSE_PASSWORD
    valueFrom:
      secretKeyRef:
        name: slashobserve-clickhouse
        key: password
```

The server reads the datasources file only at startup.
A change to the rendered ConfigMap changes a checksum annotation on the pod, so `helm upgrade` restarts it; a change to your own ConfigMap needs `kubectl rollout restart`.

## Service accounts from a file

To create [service accounts with keys from files](service-account-keys.md#declare-service-accounts-in-a-file), put the service-accounts file and every key file in one Secret and set `serviceAccounts.existingSecret`.
The chart mounts that Secret at `/etc/slashobserve/service-accounts`, so each `keyFile` names a path there:

```yaml
serviceAccounts:
  - name: deploy-bot
    role: editor
    keyFile: /etc/slashobserve/service-accounts/deploy-bot.key
```

```sh
kubectl -n slashobserve create secret generic slashobserve-service-accounts \
  --from-file=service-accounts.yaml=./service-accounts.yaml --from-file=deploy-bot.key=./deploy-bot.key
```

## Provision dashboards

The chart mounts an empty provisioning directory and one ConfigMap per folder beneath it, so each ConfigMap is one folder of [provisioned dashboards](provisioning.md).
Each key of the ConfigMap is a file name ending in `.json` and its value is an ObserveSpec document.

```sh
kubectl -n slashobserve create configmap payments-dashboards --from-file=dashboards/payments/
```

```yaml
provisioning:
  folders:
    - directory: Payments
      configMap: payments-dashboards
```

`directory` names the folder, with the same rules as a subdirectory of the provisioning directory.
Files directly in the provisioning directory, which would go to General, are not available on Kubernetes; put them in a folder.
`provisioning.internals: true`, the default, adds the internals dashboard as the folder `SlashObserve`.

Mount each folder as its own plain ConfigMap volume, as the chart does.
A projected volume that nests several ConfigMaps creates symlinked directories that the provisioner does not walk into, and a `subPath` mount never sees updates.
A changed ConfigMap reaches the pod within about a minute, and the provisioner saves the change within 30 seconds after that.

## Metrics

The server serves `/metrics` without sign-in; see [Metrics](metrics.md).
The pod carries `prometheus.io/scrape`, `prometheus.io/port` and `prometheus.io/path` annotations for a Prometheus that discovers pods by annotation; `metrics.podAnnotations: false` removes them.
With the Prometheus Operator, set `metrics.serviceMonitor.enabled: true` and add the label your Prometheus selects ServiceMonitors by under `metrics.serviceMonitor.labels`.
The Ingress below routes every path to the server, `/metrics` included, so block `/metrics` at the controller or load balancer when the Ingress is public.

## Ingress

`ingress.enabled: true` creates an Ingress for the host of `publicURL`, with a TLS block when `ingress.tlsSecretName` is set.
The controller must follow the proxy rules in [Run in production](production.md#put-tls-in-front): no response buffering and no short read timeout on `GET /api/v1/dashboards/{id}/events`, which is a Server-Sent Events stream, and request bodies of at least 8 MiB.
For ingress-nginx, for example:

```yaml
ingress:
  enabled: true
  className: nginx
  tlsSecretName: slashobserve-tls
  annotations:
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-body-size: 8m
```

## Pod defaults

The pod runs as the image's non-root user 65532 with a read-only root filesystem, every capability dropped and no Kubernetes API token mounted.
The liveness probe requests `/healthz` and the readiness probe `/readyz`.
A startup probe on `/readyz` allows 5 minutes before the liveness probe starts, so a long migration at boot is not mistaken for a hung server.
Set `resources` to fit your load; the demo stack limits the server to 256 MiB of memory.

## Upgrade and roll back

Take a database backup before every upgrade, as [Upgrades and backups](upgrades-and-backups.md) describes, then:

```sh
helm upgrade slashobserve deploy/helm/slashobserve --namespace slashobserve -f values.yaml --wait
```

The new pod runs pending migrations at startup and turns ready once they are done.
`helm rollback slashobserve <revision> --wait` returns to an earlier revision's image and values.
The server runs migrations forward only, so a rollback across a release that ran a migration also needs the database restored from the backup you took before the upgrade.

## More than one replica

One replica is the tested setup.
More replicas share the work that runs through Postgres: migrations take a Postgres lock, so one pod migrates while the others wait, and the scheduler and the agent workers lease their jobs through the database.
The limits on concurrent MCP requests per key, on event streams and on in-flight datasource queries are kept per process, so each replica allows its own share.

## Try it on kind

`make helm-kind` creates the local kind cluster `slashobserve-helm`, builds the server image from your checkout, deploys a throwaway Postgres and Prometheus from `deploy/helm/kind`, installs the chart with generated Secrets, checks it and upgrades and rolls it back.
`make helm-kind-down` deletes that cluster.
