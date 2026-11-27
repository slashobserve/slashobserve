---
title: Configuration
description: Every `SLASHOBSERVE_*` environment variable with its default, range and effect, and the settings a server needs to start.
---

You configure the server with environment variables named `SLASHOBSERVE_*`.
Two are required to start, and everything else has a default.

The server reads the variables once at startup, so restart it after a change.
It trims whitespace around each value, and an empty value counts as unset.
A value that does not parse, or falls outside its range, stops the server at startup, and the error names the variable.

## Start a server with the minimum

1. Set the two required variables:

   ```sh
   export SLASHOBSERVE_DATABASE_URL='postgres://slashobserve:slashobserve@postgres:5432/slashobserve?sslmode=disable'
   export SLASHOBSERVE_PUBLIC_URL='http://localhost:8080'
   ```

2. Decide how people sign in.
   For browser sign-in, also set `SLASHOBSERVE_OIDC_ISSUER_URL`, `SLASHOBSERVE_OIDC_CLIENT_ID`, `SLASHOBSERVE_OIDC_CLIENT_SECRET` and `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`; see [Sign-in](#sign-in).
   Without browser sign-in, only service-account keys authenticate; see [Run with no browser sign-in](../operate/service-account-keys.md#run-with-no-browser-sign-in).
3. Optionally set an LLM provider so the built-in agent runs; see [LLM](#llm).

The image sets `SLASHOBSERVE_HTTP_ADDR=:8080`.
[Run your own deployment](../get-started/install.md) shows a full start.

## Server and logging

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_HTTP_ADDR` | `:8080` | Listen address as `host:port`. |
| `SLASHOBSERVE_DATABASE_URL` | none | Required. Postgres connection string. |
| `SLASHOBSERVE_DATABASE_DIRECT_URL` | unset | Connection string straight to Postgres, for use behind a pooler. |
| `SLASHOBSERVE_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |
| `SLASHOBSERVE_LOG_FORMAT` | `json` | `json` or `text`. Logs go to standard error. |
| `SLASHOBSERVE_TRUSTED_PROXIES` | unset | Addresses and ranges of the proxies in front of the server. |
| `SLASHOBSERVE_METRICS` | `true` | `true` serves `/metrics` without sign-in; `false` removes the route. |

`SLASHOBSERVE_LOG_LEVEL` and `SLASHOBSERVE_LOG_FORMAT` ignore case, but `SLASHOBSERVE_METRICS` and the other true-or-false settings accept only lowercase `true` or `false`.
Requests to `/healthz`, `/readyz` and `/metrics` are logged at `debug` only.
See [Metrics](../operate/metrics.md).

### Details of `SLASHOBSERVE_HTTP_ADDR`

The port must be 1 to 65535.
While a model provider is set, the host must be empty, an unspecified address such as `0.0.0.0`, `localhost` or a loopback address.
The built-in agent reaches the server on loopback, and the server refuses to start otherwise.

### Details of `SLASHOBSERVE_DATABASE_URL`

The value is a URL or key/value connection string that pgx parses.
The pool opens up to the largest of three numbers: 16, 8 plus 2 per agent worker, and the CPU count.
Add `pool_max_conns=N` to the string to set the size yourself.

### Details of `SLASHOBSERVE_DATABASE_DIRECT_URL`

Set it when `SLASHOBSERVE_DATABASE_URL` goes through a transaction-mode pooler.
Live updates (`LISTEN`) and migrations with their lock use this URL, and everything else uses the pooled one.
Both must reach the same database, or the server refuses to start.
Unset, everything uses `SLASHOBSERVE_DATABASE_URL`.

### Details of `SLASHOBSERVE_TRUSTED_PROXIES`

The value is a comma-separated list of addresses and CIDR ranges, such as `10.0.0.0/8, 2001:db8::/32`.
List the reverse proxies or load balancers in front of the server.
The server records the client address as `client_ip` on audit events; see [Put TLS in front](../operate/production.md#put-tls-in-front).

- Unset, the client address is the address the connection came from, and forwarding headers are ignored.
- Set, the server reads `X-Forwarded-For` only when the connection comes from a listed address.
  The client address is then the rightmost address in the header that is not itself listed.
- An entry that is not an address or a range stops the server at startup, naming the entry.
  So does a range with no network bits, such as `0.0.0.0/0`.
- List the narrowest range of the proxy.
  A client inside a listed range can set its own address.
- A proxy that writes `ip:port` into `X-Forwarded-For` always records its own address.

## Sign-in

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_PUBLIC_URL` | none | Required. The `http` or `https` origin people use. |
| `SLASHOBSERVE_OIDC_ISSUER_URL` | none | The identity provider's issuer URL. |
| `SLASHOBSERVE_OIDC_CLIENT_ID` | none | The client ID registered with the provider. |
| `SLASHOBSERVE_OIDC_CLIENT_SECRET` | none | The client secret. Never logged. |
| `SLASHOBSERVE_OIDC_INTERNAL_URL` | unset | Origin the server calls instead of the issuer's host. |
| `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` | none | Comma-separated domains, or `*` for every account. |
| `SLASHOBSERVE_AUTH_PROVIDER_NAME` | unset | Provider display name, up to 64 characters. |
| `SLASHOBSERVE_ADMIN_EMAILS` | unset | Comma-separated Admin email addresses, at most 100. |
| `SLASHOBSERVE_OIDC_GROUP_ROLES` | unset | Comma-separated `group=role` entries, at most 100. |
| `SLASHOBSERVE_SESSION_TTL` | `168h` | A session lasts this long from sign-in, a Go duration from `5m` to `720h`. |

Browser sign-in is all or nothing.
Set the issuer URL, client ID and client secret together, or set none of them.
With none, only service-account keys authenticate, and setting any of `SLASHOBSERVE_OIDC_INTERNAL_URL`, `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`, `SLASHOBSERVE_ADMIN_EMAILS`, `SLASHOBSERVE_OIDC_GROUP_ROLES` or `SLASHOBSERVE_AUTH_PROVIDER_NAME` stops the server at startup, because only browser sign-in reads them.
With all three, `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` is also required.

### Details of `SLASHOBSERVE_PUBLIC_URL`

The value is an `http` or `https` origin with no path, query or user info.
See [Put TLS in front](../operate/production.md#put-tls-in-front).
It is also the OAuth issuer for MCP clients and the base of the MCP resource URL.
Changing it disconnects every connected agent, and clients outside this machine need an `https` value.

### Details of `SLASHOBSERVE_OIDC_ISSUER_URL`

The value is an absolute `http` or `https` URL.
The server uses it exactly as written, because it compares it exactly with the identity provider's issuer.

### Details of `SLASHOBSERVE_OIDC_INTERNAL_URL`

The value is an origin with no path.
The server uses its scheme and host instead of the issuer's when it calls the identity provider, for example when the provider has a different name inside a container network.

### Details of `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`

The value is a list of valid domain names, or a single `*`, which cannot be combined with domains.
Duplicates are ignored.
See [Sign-in with OIDC](../connect/oidc.md).

### Details of `SLASHOBSERVE_AUTH_PROVIDER_NAME`

Signed-in clients show the name next to the sign-in rules.

### Details of `SLASHOBSERVE_ADMIN_EMAILS`

A person is an Admin when they sign in with a listed address that the provider reports as verified.
The comparison ignores case, each entry must be an email address, and duplicates are ignored.
Unset, the earliest person to sign in is the Admin.
See [Roles](../teams/roles.md).

### Details of `SLASHOBSERVE_OIDC_GROUP_ROLES`

Each entry maps an identity-provider group to a role, as `group=role`.
The role is `admin`, `editor`, `commenter` or `viewer`.
The group name is 1 to 256 bytes without control characters, and the server splits the entry at its last `=`.
See [Roles](../teams/roles.md).

## Datasources

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_DATASOURCES_FILE` | unset | Path to the [datasources file](datasources-file.md). |

Unset means no datasources from a file.
An Admin can still [add them in the app](../connect/datasources-in-the-app.md).

## MCP sources

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_MCP_SOURCES_FILE` | unset | Path to a YAML file of [MCP sources](../operate/mcp-sources.md#provision-sources-from-a-file) the server keeps in step with its entries. |

Unset means no MCP sources from a file.
An Admin can still add them in the app.

The file holds one `mcpSources` list:

```yaml
mcpSources:
  - uid: docs
    name: Company docs
    description: Runbooks and design notes
    url: https://docs.example.com/mcp
    audience: admins
    secretHeader: Authorization
    secretEnv: SLASHOBSERVE_DOCS_TOKEN
```

| Field | Rule |
| --- | --- |
| `uid` | Required. Starts with a letter or digit and has at most 40 letters, digits, `_` and `-`. |
| `url` | Required. An `https` MCP address with a domain name. |
| `name` | At most 100 characters without control or format characters. Defaults to the `uid`. |
| `description` | Optional. At most 300 characters on one line, without control or format characters. |
| `audience` | `everyone` (the default) or `admins`. |
| `secretHeader` | Required with a secret, and refused without one. |
| `secretEnv`, `secretFile` | One of them holds the secret. `secretEnv` names a variable that starts with `SLASHOBSERVE_`. |

## Service accounts

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_SERVICE_ACCOUNTS_FILE` | unset | Path to a YAML file of service accounts. |

Unset or empty means no accounts from a file.
The server creates or updates each account in the file at every start, with its key read from the account's `keyFile`.
An account from the file answers 409 `provisioned` to role changes, key creation and key revocation in the web app and the API.
To rotate its key, write a new one to the file and restart.
See [Service-account keys](../operate/service-account-keys.md#declare-service-accounts-in-a-file).

## Activity

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_ACTIVITY_RETENTION` | `4320h` | How long to keep events, a Go duration from `24h` to `87600h`. |

`4320h` is 180 days.
The hourly `activity_prune` job deletes [activity](../use/activity.md) events older than this.

## Audit

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_AUDIT_RETENTION` | `8760h` | How long to keep audit events, a Go duration from `24h` to `87600h`. |
| `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT` | unset | OTLP/HTTP receiver that gets every audit event. Unset turns export off. |
| `SLASHOBSERVE_AUDIT_OTLP_HEADERS_FILE` | unset | File of request headers sent with every export. Needs the endpoint. |

### Details of `SLASHOBSERVE_AUDIT_RETENTION`

`8760h` is one year.
The server stores audit events in one Postgres partition per UTC day.
Events copied from the activity log at upgrade sit in one partition per UTC month, or per UTC year when older than two years.
The daily `audit_partitions` job drops each partition whose end is older than the retention, and records an `audit.prune` event.

### Details of `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT`

The value is an absolute `http` or `https` URL with no query, fragment or user info.
The server posts every audit event as an OTLP log record to this URL followed by `/v1/logs`.
See [Audit export](../operate/audit-export.md).

### Details of `SLASHOBSERVE_AUDIT_OTLP_HEADERS_FILE`

Each line is `Name: value`, and blank lines are skipped.
A header name may appear once, a value must not be empty or hold a control character, and the file needs at least one header.
Setting the file without the endpoint stops the server at startup.

## Dashboard provisioning

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_PROVISION_DIR` | unset | Directory of ObserveSpec JSON files to load as dashboards. |

The server loads the files at startup and again whenever one changes, checking every 30 seconds.
Unset turns provisioning off.
See [Provisioning](../operate/provisioning.md).

## Outbound access

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_OUTBOUND_DOMAINS` | unset | Locks the allowed domains: `none` or a comma-separated list. |

Unset, Admins manage the allowed domains in Settings.
`none` (any case) or a list locks the setting, and a list holds at most 100 domains, duplicates ignored.
See [Outbound access](../operate/outbound-access.md).

## Stored secrets

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_SECRET_KEY` | unset | Key that encrypts credentials the server stores. |

Generate one with `openssl rand -hex 32`.
The key is at least 32 random bytes written as hex or base64.
A value that is valid hex is read as hex.
The server checks it at startup and never logs it.

### Details of `SLASHOBSERVE_SECRET_KEY`

The key encrypts three kinds of stored secret:

- the credentials of [datasources added in the app](../connect/datasources-in-the-app.md);
- the tokens of the [Grafana connection](../use/switch-from-grafana.md);
- the signing secrets of [webhooks](../use/webhooks.md).

Saving one of these without the key answers `409 secret_key_missing`.

Changing the key makes the stored secrets unreadable until each one is entered again.
A datasource shows as Unavailable.
Every webhook is disabled with the reason `secret_unreadable` until you rotate its secret and enable it.

## LLM

All LLM settings are optional.
Unset `SLASHOBSERVE_LLM_PROVIDER` turns the built-in agent off.
See [LLM providers](../connect/llm-providers.md) for how the settings combine.

| Variable | Default | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_LLM_PROVIDER` | unset | `anthropic`, `openai` or `replay`, in any case. |
| `SLASHOBSERVE_LLM_MODEL` | `claude-sonnet-5-5` for `anthropic` | Model name. Required for `openai`. |
| `SLASHOBSERVE_LLM_API_KEY` | unset | Provider credential. |
| `SLASHOBSERVE_LLM_API_KEY_FILE` | unset | File that holds the credential. |
| `SLASHOBSERVE_LLM_BASE_URL` | the provider's endpoint | Endpoint URL, with a path if needed. |
| `SLASHOBSERVE_LLM_EFFORT` | unset | Reasoning effort, checked against the provider's list; unset on `anthropic`, an investigation and a question about a dashboard or an observation run at `high`. |
| `SLASHOBSERVE_LLM_MAX_TOKENS` | `16000` | Output token cap of a model call, `1024` to `64000`. |
| `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` | `10m` | Limit for one model call, `10s` to `60m`, or `0` for none. |
| `SLASHOBSERVE_LLM_IDLE_TIMEOUT` | `180s` | Silence limit while a call streams, `10s` to `60m`, or `0` for none. |
| `SLASHOBSERVE_AGENT_RUN_TIMEOUT` | `0` | Cap on one agent run. `0` is no limit. |
| `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` | `0` | Cost cap of a run in US dollars. `0` is no limit. |
| `SLASHOBSERVE_AGENT_WORKERS` | `8` | Agent requests this server runs at once, `1` to `64`. |
| `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` | unset: by task type, `12` to `30` | Model calls an investigation makes, `6` to `60`. |
| `SLASHOBSERVE_AGENT_INVESTIGATION_LOOP` | `true` | Gives investigations of the built-in agent a hypothesis ledger, stages, and a call limit that grows with progress. |
| `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX` | `40`, or `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` when that is higher | The most model calls a growing investigation limit can reach, from `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` (or `6`) to `60`. |
| `SLASHOBSERVE_AGENT_INVESTIGATION_BRIEF` | `on` | Sweeps the datasources before an investigation's first model call and gives the agent the result as a starting point: `on` or `off`. |
| `SLASHOBSERVE_AGENT_FINDING_GROUNDING` | `enforce` | Numbers what reads return as facts and checks a run's finding against what the run read: `off`, `report` or `enforce`. |
| `SLASHOBSERVE_AGENT_SESSION_RENEW_KIB` | `256` | Session size that triggers shortening, `32` to `4096` KiB. |
| `SLASHOBSERVE_LLM_PRICES_FILE` | the built-in table | YAML file of model prices. |
| `SLASHOBSERVE_LLM_REPLAY_DIR` | unset | Folder of recorded model scripts. Required for `replay`. |
| `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS` | `false` | `true` logs the provider's own error message. |

### Details of the provider, model and credential

- `anthropic` needs a credential in `SLASHOBSERVE_LLM_API_KEY` or `SLASHOBSERVE_LLM_API_KEY_FILE`.
  Its model defaults to `claude-sonnet-5-5`.
- `openai` needs `SLASHOBSERVE_LLM_MODEL`, and its credential is optional.
- `replay` needs `SLASHOBSERVE_LLM_REPLAY_DIR` and ignores the model.
  It exists for end-to-end tests, not for real use.
- Set at most one of `SLASHOBSERVE_LLM_API_KEY` and `SLASHOBSERVE_LLM_API_KEY_FILE`.
  The file's contents are trimmed of whitespace, and an empty or unreadable file stops the server at startup.
- The server never logs the credential or echoes it in an error.

### Details of `SLASHOBSERVE_LLM_BASE_URL`

The value is an absolute `http` or `https` URL.
It may have a path, but no query, fragment or user info.
For `anthropic`, give the root without `/v1`, which the agent adds.
For `openai`, the URL is used as written.

### Details of `SLASHOBSERVE_LLM_EFFORT`

- For `anthropic`: `low`, `medium`, `high`, `xhigh` or `max`.
  Unset means `high` for an investigation and a question asked about a dashboard or an observation, and `medium` for every other run.
- For `openai`: `none`, `minimal`, `low`, `medium`, `high` or `xhigh`.
  Unset sends nothing to the provider, for every run, because an OpenAI-compatible server may not take a reasoning effort.
- A value that is set applies to every run.
- `replay` ignores the value.
- The first call of a new build runs one step higher: `low` becomes `medium`, `medium` becomes `high`, and anything else stays.
  Every later call runs at the value.

### Details of `SLASHOBSERVE_LLM_MAX_TOKENS`

The first call of a new build uses the value as it is.
Later calls cap their output at half of it, but at least `8192` and never above it.
A call cut off at that lower cap is sent once more at the full value.

### Details of the timeouts

- `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` applies to each model call, including its retries of up to 4 attempts with backoff.
  A call that hits the limit fails with `call_timeout`.
- `SLASHOBSERVE_LLM_IDLE_TIMEOUT` applies because model calls stream.
  An attempt that receives no bytes from the provider for this long, from its start or since the last byte, is retried with backoff within the request timeout.
- With both timeouts and `SLASHOBSERVE_AGENT_RUN_TIMEOUT` at `0`, a model call that stalls runs until someone cancels the run, so keep at least one bound.
- `SLASHOBSERVE_AGENT_RUN_TIMEOUT` set to `0` means a run ends at the agent's own limits, a cancel or a shutdown.
  Set a Go duration of at least `1m` to cap each run of an agent request.
  A run that hits the cap fails with `run_timeout`.

### Details of `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD`

The value is a number of US dollars above 0, at most 10000.
Once an attempt's estimated cost reaches it, the agent starts no further model call, and the request fails with `budget_exhausted` with the build kept.
See [Run cost](../connect/llm-providers.md#run-cost).
The call that crosses the limit still finishes, so a run can pass the limit by one call.
The server does not start when the value is set and the configured model has no price for every token class.

### Details of `SLASHOBSERVE_AGENT_WORKERS`

The value is how many agent requests this server runs at once.
Each server of a fleet runs its own workers, so a fleet runs replicas times this many.
One request per dashboard still runs at a time, and the model provider's rate limits stay the real ceiling.
The metric `slashobserve_agent_queue_wait_seconds` shows whether more workers would help.

### Details of `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS`

The value is how many model calls an investigation makes before only `save_finding` can run.
Unset, each [task type](../concepts/investigate.md#task-types) has its own: `measure` 16, `logs` 20, `traces` 20, `incident` 30, `inventory` 12, `explain` 12, and 30 for a question no rule sorts; a message in a chat with no dashboard or observation gets 12 until its first panel is saved.
A set value applies to every investigation and to that chat message alike.

The call before the limit gets a note to save the finding next, and reads in the same call as the finding still run.
It covers every agent that runs an investigation, the built-in one and connected ones.
Suggest runs keep 12 and dashboard runs keep 6.
A run keeps the limit it started with, so a changed setting reaches new runs only.
Raise it for questions that need many reads.
A longer investigation costs more model calls, so `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` is the cap on spend.

### Details of `SLASHOBSERVE_AGENT_INVESTIGATION_LOOP`

With `true`, the built-in agent gets the tool `note_hypotheses` in investigations and in questions asked on a dashboard or an observation, and the prompt of an investigation, a question that opens an observation, gains a short section that tells it to keep a ledger of candidate causes.
A plain question gets the tool but not the section, and the server does not write a ledger back into its brief.
The server keeps the latest ledger and writes it back into the agent's brief as a `<hypotheses>` block whenever the conversation is shortened.
The finished run returns the last ledger as `usage.hypotheses` to its requester and to Admins.
An investigation that saves a finding with `root_cause` or `ruled_out` while every hypothesis in its ledger is still open, or it has none, gets that save sent back once with the request to update the ledger first; the second save is accepted as it is, and the run counts the first as a nudge.
It is on by default.
With `false`, the tool is not offered, the prompt has no section and the brief has no block, so a run is the same as it was before the setting existed.
A connected agent in an investigate run, started with a run token, gets the tool and the prompt section as well, through `get_run_brief`; a connected agent outside a run never gets the tool.

The server also works out the built-in agent's stage, tells it after each turn, and lets its call limit grow when the ledger shows progress, up to `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX`.
A connected agent never sees stage lines and keeps the fixed limit.
Changing the setting needs a server restart.

### Details of `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX`

The value is the ceiling the call limit of an investigation can grow to when `SLASHOBSERVE_AGENT_INVESTIGATION_LOOP` is `true`.
It must be at least `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` when that is set, else at least `6`, and at most `60`, and the server refuses to start with a value outside that range.
Unset, it is `40`, or `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` when that is higher; a starting limit above the ceiling never grows.
Without the loop setting it has no effect.
The cost cap `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` and the run timeout still end an investigation at any limit.

### Details of `SLASHOBSERVE_AGENT_INVESTIGATION_BRIEF`

With `on`, the server reads every datasource the run may read before an investigation's first model call, with no model, in at most 40 reads and 20 seconds, and puts the service map, the per-service rates on a 1 hour, 6 hour and 24 hour ladder, the onsets, error origins, changed log patterns and change events into the agent's first message as numbered candidates.
The sweep runs once per investigation and is kept with the run session.
A plain question, a chat question and dashboard builds do not sweep, and any other value than `on` or `off` stops the server at start-up.
See [The preflight brief](../concepts/investigate.md#the-preflight-brief).

### Details of `SLASHOBSERVE_AGENT_FINDING_GROUNDING`

With `report` or `enforce`, the server keeps a bounded record of what an investigation read, and checks the finding against it when the agent saves.
The record holds up to 2,000 numbers, rounded to 3 significant figures and kept per read, the names the run saw, the time ranges it queried and the times its results showed.
A cited fact always passes, a typed figure within 1% of a fact passes and one further from it than 1% but within the tolerance below does not, a trace or span id passes only whole and read, and figures that are not measurements, such as a number from the question, a version, a port, an HTTP status, a window or step duration or a year, are not checked; otherwise a figure in the Markdown or the impact passes when it is within 2% of a number the run read, or within the precision it was written with but never more than 10% away, or when it is the difference, sum, ratio or percentage change of two numbers from the same read; a percentage is compared as a fraction.
Times, dates, whole numbers of 10 or less with no unit, code spans and links are not checked, a small number with a unit such as `3 s` is, and `ms` and `s` and `K` and `M` are read as units.
`root_cause.component` passes when it matches a name the run discovered or saw as a label value or a trace's service, as a whole or by whole words, and `onset` passes when it falls inside a time range the run queried or next to a time a result showed, give or take a minute.
Each entry of `causes` is checked the same way: its component name, its trigger key, its onset, each fact id of its evidence, which must be one the run's results gave, and each trace id, which must be one it read.

With `report` or `enforce`, the server also scores each cause from 0 to 1 from what it can check, and that score decides whether the finding states the cause; see [Investigate](../concepts/investigate.md#causes-and-confidence).
With `off`, causes are not scored and every cause is stated.

The measurement and trace reads also number their values as facts (`"facts":{"value":"f3"}` beside the value), and the run keeps the newest 400 facts with their full-precision values, labels, windows and queries; see [Investigate](../concepts/investigate.md#facts).

- `off` records nothing and checks nothing, so a run is the same as it was before the setting existed.
- `enforce` is the default.
- `report` stores the items that did not pass with the finding and changes nothing else.
- `enforce` sends the finding back once, listing each item with how to fix it, such as "read it, correct it, or remove it"; the second save is accepted, and the items that still did not pass are stored with it.

A send-back is not a rejected save: it does not count toward the 3 rejected saves in a row, and the call that answers it may read past the call limit.
The check never fails a run.
It covers every run that can save a finding, an investigation, a question asked on Home and a revision, of the built-in agent and of connected agents, and a run keeps the mode it started with across a restart; `save_finding` also takes `answers`, one fact per quantity the question asks for, and two facts for one quantity are an item of the check.
The items are returned as `unverified` on the finding and on the observation's `findings`, and the observation shows one line under its header.
Changing the setting needs a server restart.

### Details of `SLASHOBSERVE_AGENT_SESSION_RENEW_KIB`

After a save to a build, the agent checks the size of its session's tool calls, results and text.
Past this size it shortens its conversation to its last few turns, the build and the names it discovered, so a long build stays inside the model's context window.
Lower the value for a model with a small context window.

### Details of `SLASHOBSERVE_LLM_PRICES_FILE`

The file holds model prices in US dollars per million tokens and replaces the built-in table.
A request's estimated cost (`usage.cost_usd`) then covers other models.
The server refuses to start when the file is unreadable or invalid.
See [Run cost](../connect/llm-providers.md#run-cost).

### Details of `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS`

The value is `true` or `false`.
`true` adds the provider's error message to the `llm call finished` line of a failed call, as `provider_error_message`.
The server removes the API key, replaces control characters and keeps at most 1024 bytes.
A provider can echo parts of the prompt back in that message.
Use it for debugging, and not on a server whose logs many people read.
See [Logging](../operate/logging.md).

## Testing only

`SLASHOBSERVE_API_CONTRACT_CHECK` (`true` or `false`, default `false`) makes the server validate every `/api/v1` request and response against the OpenAPI document.
It is for the end-to-end test stack only, so never set it in production.
It also turns a request with a query parameter the document does not list into a 500, as [API behavior](api-behavior.md#query-parameters) describes.
