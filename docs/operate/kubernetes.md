---
title: Run on Kubernetes
description: Install the server with the Helm chart, on an external Postgres, with every secret taken from a Secret you create.
---

Use the Helm chart in `deploy/helm/slashobserve` to run the server on Kubernetes.
It runs the server as one Deployment with a Service, and it provisions the [internals dashboard](observe-slashobserve.md) by default.
It ships no Postgres and creates no Secret: you bring the database and the credentials, and the chart only references them.

## Install the chart

1. Check the prerequisites:
   - A Postgres database the cluster can reach, and its connection string.
     See [Upgrades and backups](upgrades-and-backups.md) for backing it up.
   - Helm 3 or later and `kubectl` pointed at the cluster.
     The chart is an `apiVersion: v2` chart, and its checks run on Helm 4.
   - For browser sign-in, an OIDC client at your identity provider with the redirect URI `<public url>/api/v1/auth/callback`; see [Sign-in with OIDC](../connect/oidc.md).
   - A server that only tools use needs no OIDC client; see [Run with no browser sign-in](#run-with-no-browser-sign-in).
     The chart refuses an install that has neither sign-in nor a service-accounts Secret.
   - A Prometheus that scrapes the pod, and a datasource with uid `prom` that reads from it, if you want the internals dashboard to show data.

2. Write each secret value to a file, so it stays out of your shell history, then create the Secrets in the release namespace:

   ```sh
   kubectl create namespace slashobserve
   kubectl -n slashobserve create secret generic slashobserve-database --from-file=url=./database-url
   kubectl -n slashobserve create secret generic slashobserve-oidc --from-file=client-secret=./oidc-client-secret
   kubectl -n slashobserve create secret generic slashobserve-secret-key --from-file=secret-key=./secret-key
   kubectl -n slashobserve create secret generic slashobserve-llm --from-file=api-key=./llm-api-key
   ```

   Delete the files afterwards.
   [The Secrets](#the-secrets) lists what each one holds.

3. Write a values file, here with browser sign-in:

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

4. Install from a checkout of the release tag you run:

   ```sh
   helm install slashobserve deploy/helm/slashobserve --namespace slashobserve -f values.yaml --wait
   ```

5. Check readiness.
   The chart's notes print the command, which port-forwards the Service and requests `/readyz`:

   ```sh
   kubectl -n slashobserve port-forward svc/slashobserve 8080:8080
   curl http://127.0.0.1:8080/readyz
   ```

   You see `{"status":"ok","checks":{"database":"ok","migrations":"ok"}}` once Postgres answers and every migration has run.

`values.schema.json` in the chart describes every value.
The install fails before it reaches the cluster when a required value is missing, or when the OIDC values are incomplete.
`image`, `publicURL` and `database` are required, `publicURL` must be `http` or `https` with a host and no path, and `database` needs `existingSecret` and `urlKey`.

## Set the image

The image defaults to `ghcr.io/slashobserve/slashobserve` at the chart's `appVersion`.
Set `image.tag` to run another tag, and `image.digest` to pin the digest too.

## Turn on the agent

Set these values to enable the built-in agent:

- `llm.provider`
- `llm.model`, when the provider needs one
- `llm.apiKey.existingSecret`
- `llm.effort`, if you want a different effort level

See [LLM providers](../connect/llm-providers.md).
The chart mounts the key as a file for `SLASHOBSERVE_LLM_API_KEY_FILE`.
The server starts `observe-agent` from the image whenever `llm.provider` is set.
That process makes the model calls from the pod, so the pod needs a network path to the provider.

## Set any other server setting

The chart sets the environment variables of the [configuration reference](../reference/configuration.md) from its values.
Add any other `SLASHOBSERVE_*` setting under `env`, in Kubernetes `EnvVar` form.
For example, `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` has no chart value, so set it under `env` with the number quoted, because `value` is a string.
It takes a whole number from 6 to 60 and defaults to 12.

```yaml
env:
  - name: SLASHOBSERVE_AGENT_INVESTIGATION_CALLS
    value: "24"
```

Each `env` name must start with `SLASHOBSERVE_`.

## Record the real client address

Set `trustedProxies` to the narrowest addresses or ranges your Ingress controller connects from, not the pod network.
The audit trail then records the real client address; see [Put TLS in front](production.md#put-tls-in-front).

```yaml
trustedProxies:
  - 10.0.4.0/28
```

## Run with no browser sign-in

A server that only tools use can run without an identity provider, as [Run with no browser sign-in](service-account-keys.md#run-with-no-browser-sign-in) describes.

1. Leave `oidc.issuerURL`, `oidc.clientID` and `oidc.existingSecret` empty.
2. Create a service-accounts Secret and set `serviceAccounts.existingSecret`; see [Service accounts from a file](#service-accounts-from-a-file).

```yaml
publicURL: https://slashobserve.example.com
database:
  existingSecret: slashobserve-database
serviceAccounts:
  existingSecret: slashobserve-service-accounts
```

The chart then sets none of the three OIDC variables and references no client secret.
Every request authenticates with a key from the service-accounts file.

The render fails in these cases:

- Only some of the three OIDC values are set.
  The message names the missing ones, as the server refuses to start with only some of them.
- All three are empty and `serviceAccounts.existingSecret` is empty, because nobody could sign in.
- All three are empty and `auth.allowedEmailDomains` or `auth.adminEmails` is set, because only browser sign-in reads them.

## Add datasources

`datasources.datasources` holds the entries of the [datasources file](../reference/datasources-file.md), which the chart renders into a ConfigMap.
`datasources.existingConfigMap` names a ConfigMap of your own instead, with the file under the key `datasources.key` (`datasources.yaml` by default).

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
A change to the rendered ConfigMap changes a checksum annotation on the pod, so `helm upgrade` restarts it.
A change to your own ConfigMap needs `kubectl rollout restart`.

The values and the file are optional.
An Admin can add datasources in the app instead, which stores them in Postgres.
That needs `secretKey.existingSecret` for their credentials; see [Add datasources in the app](../connect/datasources-in-the-app.md).

## Service accounts from a file

To create [service accounts with keys from files](service-account-keys.md#declare-service-accounts-in-a-file), put the service-accounts file and every key file in one Secret.

1. Write `service-accounts.yaml`, with a `serviceAccounts` list.
   Each `keyFile` names a path under `/etc/slashobserve/service-accounts`, where the chart mounts the Secret.

   ```yaml
   serviceAccounts:
     - name: deploy-bot
       role: editor
       keyFile: /etc/slashobserve/service-accounts/deploy-bot.key
   ```

2. Create the Secret:

   ```sh
   kubectl -n slashobserve create secret generic slashobserve-service-accounts \
     --from-file=service-accounts.yaml=./service-accounts.yaml --from-file=deploy-bot.key=./deploy-bot.key
   ```

3. Set `serviceAccounts.existingSecret` to the Secret.
   Set `serviceAccounts.fileKey` too if the file's key in the Secret is not `service-accounts.yaml`.

The chart points `SLASHOBSERVE_SERVICE_ACCOUNTS_FILE` at the file.
The block in step 1 is that file's content, not a chart value.

## Provision dashboards

The chart mounts an empty provisioning directory and one ConfigMap per folder beneath it, so each ConfigMap is one folder of [provisioned dashboards](provisioning.md).
Each key of the ConfigMap is a file name ending in `.json`, and its value is an ObserveSpec document.

1. Create a ConfigMap from a directory of dashboards:

   ```sh
   kubectl -n slashobserve create configmap payments-dashboards --from-file=dashboards/payments/
   ```

2. List it in your values:

   ```yaml
   provisioning:
     folders:
       - directory: Payments
         configMap: payments-dashboards
   ```

3. Run `helm upgrade`.

`directory` names the folder, with the same rules as a subdirectory of the provisioning directory.
Files directly in the provisioning directory, which would go to General, are not available on Kubernetes; put them in a folder.
`provisioning.internals: true`, the default, adds the internals dashboard as the folder `SlashObserve`.

Mount each folder as its own plain ConfigMap volume, as the chart does.
A projected volume that nests several ConfigMaps creates symlinked directories that the provisioner does not walk into, and a `subPath` mount never sees updates.
A changed ConfigMap reaches the pod within about a minute, and the provisioner saves the change within 30 seconds after that.

## Metrics

The server serves `/metrics` without sign-in; see [Metrics](metrics.md).
The pod carries `prometheus.io/scrape`, `prometheus.io/port` and `prometheus.io/path` annotations for a Prometheus that discovers pods by annotation.
Set `metrics.podAnnotations: false` to remove them.

With the Prometheus Operator, set `metrics.serviceMonitor.enabled: true`.
Add the label your Prometheus selects ServiceMonitors by under `metrics.serviceMonitor.labels`, and change the scrape interval with `metrics.serviceMonitor.interval` (30 seconds by default).

The Ingress below does not route `/metrics`.
That path goes to a Service with no endpoints, so the controller answers with an error, and Prometheus reaches the pod or Service inside the cluster.
The server answers `404` to a percent-encoded spelling of the path such as `/%6Detrics`, so a controller that matches the raw path cannot route around that rule.
Set `ingress.exposeMetrics: true` to route `/metrics` through the Ingress as well, and then block it at the controller or load balancer when the Ingress is public.

## Ingress

Set `ingress.enabled: true` to create an Ingress for the host of `publicURL`.
Set `ingress.tlsSecretName` to add a TLS block.
The Ingress adds a second Service, `<release>-no-metrics`, which only holds the `/metrics` path back; it selects no pods.

The controller must follow the proxy rules in [Run in production](production.md#put-tls-in-front).
That means no response buffering and no short read timeout on the Server-Sent Events streams, and request bodies of at least 8 MiB.
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

## Upgrade and roll back

Take a database backup before every upgrade, as [Upgrades and backups](upgrades-and-backups.md) describes, then upgrade:

```sh
helm upgrade slashobserve deploy/helm/slashobserve --namespace slashobserve -f values.yaml --wait
```

The new pod runs pending migrations at startup and turns ready once they are done.
The chart sets no Deployment strategy, so Kubernetes rolls the pods out by default.
For a moment the old pod and the new one run together against the migrated database.

When a release's notes in [Upgrades and backups](upgrades-and-backups.md) say to stop the old servers first, do this:

1. Scale the Deployment to 0 with `kubectl`.
2. Run `helm upgrade`.
3. Check that the Deployment has the replica count from your values.

To roll back, run `helm rollback slashobserve <revision> --wait`, which returns to an earlier revision's image and values.
The server runs migrations forward only, so a rollback across a release that ran a migration also needs the database restored from the backup you took before the upgrade.

## Run more than one replica

Replicas share Postgres and behave as one server for jobs, events, agent runs, webhooks, tokens and provisioning.
Two servers behind PgBouncer are checked before each release that changes them.
See [Run more than one server](scaling.md) for what to set, the limits each server keeps, and how to check the servers share the work.

## Try it on kind

Run `make helm-kind` in a checkout to try the chart locally.
It creates the local kind cluster `slashobserve-helm`, builds the server image from your checkout and deploys a throwaway Postgres and Prometheus from `deploy/helm/kind`.
It then installs the chart with generated Secrets, checks it, and upgrades and rolls it back.
`make helm-kind-down` deletes that cluster.

## Reference

### The Secrets

The key names are the chart's defaults.
`database.urlKey`, `oidc.clientSecretKey`, `secretKey.key` and `llm.apiKey.key` change them.

| Secret key | Holds |
| --- | --- |
| `url` in `slashobserve-database` | The connection string. Required. |
| `direct-url` in the same Secret | A connection string that bypasses your pooler. Optional. |
| `client-secret` in `slashobserve-oidc` | The client secret from your identity provider. Needed for browser sign-in. |
| `secret-key` in `slashobserve-secret-key` | The key that encrypts what the server stores for you. Optional. |
| `api-key` in `slashobserve-llm` | The model provider's key. Optional. |

Details of each Secret:

- `url` looks like `postgres://slashobserve:<password>@<host>:5432/slashobserve?sslmode=require`.
- `direct-url` goes in the same Secret with `--from-file=direct-url=./database-direct-url`.
  Name it with `database.directUrlKey: direct-url`; see [Behind a connection pooler](production.md#behind-a-connection-pooler).
- `secret-key` encrypts the credentials of datasources added in the app, Grafana connection tokens and webhook signing secrets.
  Set it before anyone adds one, generate it with `openssl rand -hex 32`, and keep it with your database backups.
- `api-key` is mounted as a file for `SLASHOBSERVE_LLM_API_KEY_FILE`.

### Chart values

| Value | Sets |
| --- | --- |
| `logLevel` | `SLASHOBSERVE_LOG_LEVEL`, one of `debug`, `info`, `warn`, `error`. Default `info`. |
| `logFormat` | `SLASHOBSERVE_LOG_FORMAT`, `json` or `text`. Default `json`. |
| `outboundDomains` | `SLASHOBSERVE_OUTBOUND_DOMAINS`; see [Outbound access](outbound-access.md). |
| `trustedProxies` | `SLASHOBSERVE_TRUSTED_PROXIES`. |
| `metrics.enabled` | `SLASHOBSERVE_METRICS`. Default true. |
| `auth.allowedEmailDomains` | `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`. Required with browser sign-in; list the domains or set `["*"]`. |
| `auth.adminEmails` | `SLASHOBSERVE_ADMIN_EMAILS`. |
| `replicaCount` | The Deployment's replica count. Default 1. |

The chart also has `imagePullSecrets`, `service.type` and `service.port` (default `ClusterIP` and 8080), `serviceAccount.create` and `serviceAccount.name`, `podAnnotations`, `resources`, `nodeSelector`, `tolerations` and `affinity`.

### Pod defaults

- The pod runs as the image's non-root user 65532 with a read-only root filesystem, every capability dropped and no Kubernetes API token mounted.
- The liveness probe requests `/healthz` and the readiness probe `/readyz`.
- A startup probe on `/readyz` allows 5 minutes before the liveness probe starts, so a long migration at boot is not mistaken for a hung server.
- Set `resources` to fit your load; the demo stack limits the server to 512 MiB of memory.
