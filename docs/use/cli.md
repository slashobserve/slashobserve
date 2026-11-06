---
title: CLI
description: Install and use the observe command line client.
---

Install the latest `observe` on macOS or Linux (amd64 or arm64) with one line:

```sh
curl -fsSL https://raw.githubusercontent.com/slashobserve/slashobserve/main/install.sh | sh
```

The script downloads the archive for your platform from the [releases page](https://github.com/slashobserve/slashobserve/releases), checks its SHA-256 against `checksums.txt`, and refuses to install on a mismatch.
It installs into `/usr/local/bin` when you can write there and into `~/.local/bin` otherwise, and it never uses `sudo`.
Set `OBSERVE_INSTALL_DIR` to choose the directory and `OBSERVE_VERSION` (for example `v0.1.0`) to install a specific release:

```sh
curl -fsSL https://raw.githubusercontent.com/slashobserve/slashobserve/main/install.sh | OBSERVE_VERSION=v0.1.0 OBSERVE_INSTALL_DIR="$HOME/bin" sh
```

On Windows, or to install by hand, download the `observe` archive for your platform and `checksums.txt` from the releases page, then check the archive against the checksums:

```sh
sha256sum --check --ignore-missing checksums.txt
```

On macOS, use `shasum -a 256 --check --ignore-missing checksums.txt`.
Unpack the archive and put `observe` on your `PATH`.
To build it yourself instead, which needs only Go, run `make cli`; it writes `bin/observe`.
The examples below use `bin/observe`; with an installed binary, run `observe`.

Sign in once; `login` prompts for the key without echoing it:

```sh
bin/observe login --server https://observe.example.com
```

To pass the key on standard input instead, for scripts:

```sh
printf '%s' "$OBSERVE_TOKEN" | bin/observe login --server https://observe.example.com --with-token
```

The key is a service-account key; see [Service-account keys](../operate/service-account-keys.md).
`login` checks the key with the server and then saves the server and key to `~/.config/slashobserve/cli.json` (under `$XDG_CONFIG_HOME` when that is an absolute path), readable by you only.
Then give it a prompt:

```sh
bin/observe "checkout p99 latency, last 6h"
```

While the agent builds, it prints a line for each panel the build adds, as `Panel 3 of 8: Checkout p99 (timeseries)`; the count after `of` is the number of panels the agent planned, and it is left out when the agent gave no estimate or built more than it planned.
A follow-up prints only the panels it adds, not the ones the dashboard already had.
Only the key that made the request can read its build, so `status --wait` on someone else's request shows the steps without panel lines.
It ends with `Saved version N  https://observe.example.com/d/<dashboard id>/<slug>`, the address described in [Addresses in the web app](web.md).

In a terminal, the progress goes to standard output with the last line: the agent's latest step shows in one status line that rewrites in place, the panel lines stay, and the status line is cleared before the last line.
When standard output is not a terminal, as in `url=$(bin/observe "checkout p99 latency")` or a pipe, standard output holds only the address, and the steps, the panel lines and `Saved version N` go to standard error, one per line.
`-q` hides the steps, the panel lines and `Saved version N`, and prints only the address.
`--json` prints the finished request object of `GET /api/v1/observe/{id}` instead, and the steps and panel lines still go to standard error.
Colour is used only in a terminal, and setting `NO_COLOR` to any value turns it off.
`--open` opens the dashboard address in the browser once it is saved, with `open` on macOS, `xdg-open` on Linux and the URL handler on Windows; when the opener is missing or fails, the CLI prints a warning and still exits 0.
It opens only `http` and `https` addresses.
`status --wait`, `continue`, `run` and `import` accept `--open` too.

A prompt whose first word is a command name (`dashboards`, `comments`, `import`, `run`, `status`, `cancel`, `continue`, `annotate`, `health`, `open`, `doctor`, `logs`, `traces`, `grafana`, `login`, `logout`, `version`, `completion` or `help`) must be quoted as one argument.
`--dashboard <dashboard-id>` sends the prompt as a follow-up that revises that dashboard, as described in [Chat and follow-ups](chat.md).
`--no-wait` prints the request id and returns at once; `bin/observe status <request-id>` shows that request, and `--wait` makes it wait the way a prompt does.
`--timeout` (2 hours by default, so a large build can finish while you wait) stops the waiting and not the request, which keeps running on the server; `bin/observe status <request-id> --wait` waits again.
While a request waits for a worker, `observe status` prints `Queued, position N`, its place among the queued requests.
The order can shift, because a request is skipped while another revision of its dashboard is running.
The CLI exits with these codes:

| Code | When |
| ---- | ---- |
| `0` | The command succeeded, including when `--open` could not open the browser and when `observe logs --tail` ends with Ctrl-C. |
| `1` | The server refused or failed the request, the agent request failed, the waiting stopped at `--timeout` or an interrupt, `observe open` could not open the browser, an `observe doctor` check failed, an `observe grafana import` had a dashboard fail, or `observe health repair` queued no repair. |
| `2` | The command line is wrong: an unknown command or flag, a missing argument or an invalid id. |

`bin/observe cancel <request-id>` cancels a queued or running request and prints it.
A queued request stops at once, and a running one within a few seconds, once the call it is in returns.
The request ends `failed` with the code `canceled`, and what it built so far is kept.
`bin/observe continue <request-id>` starts a new request that takes over the kept build of a stopped one, waits the way a prompt does and prints the dashboard address.
It accepts `--no-wait` and `--timeout` like a prompt, and `--note <text>` gives the continued run an instruction; the note follows the same rule as a prompt (1 to 4000 characters, no control characters) and the server's `400` message is printed as it is.
A stopped request with a build prints `Save it in the browser, or continue it: observe continue <request-id>` in `observe status`, and one that was already continued prints `Continued by <request-id>`.
Only the person who made a request can cancel or continue it; anyone else gets `only the requester can cancel or continue <request-id>`.
Canceling a request that already finished, or continuing one that was saved, already continued or has no build, fails with a one-line message naming the request.
Continuing a revision fails when its dashboard has a newer version than the build started from.

`bin/observe dashboards list` lists dashboards, newest update first, 50 at a time; when there are more it prints the `--cursor` value for the next page.
`bin/observe dashboards show <dashboard-id>` lists the panels of the latest version, or of one given with `--version N`, and `bin/observe dashboards versions <dashboard-id>` lists its versions.
The lists and the panel table print as columns aligned with spaces.
`bin/observe --help` shows the rest.
`bin/observe dashboards export <folder-id-or-title> --dir <dir>` writes the latest version of each dashboard in a folder to `<dir>/<folder>/<name>.json`, and `bin/observe dashboards apply --dir <dir>` saves every `.json` file of a directory back as dashboards, never deleting one, and exits 1 when any file failed (see [Dashboards as code](dashboards-as-code.md)).
`bin/observe dashboards delete <dashboard-id> --yes` deletes a dashboard with every version, comment and agent request, for everyone; it needs Edit on the dashboard, cannot be undone, and refuses to run without `--yes`.
It fails with `409` and the code `dashboard_being_created` while the agent request that is creating the dashboard is still queued or running.
`bin/observe import dashboard.json` (or `-` for standard input) imports a Grafana dashboard of up to 8 MiB and prints its address; `--map '<grafana>=<uid>'` picks our datasource for a Grafana one, `--folder <title>` chooses the folder (see [Import Grafana dashboards](import-grafana.md)) and `--preview` shows the report without saving.
The import report, the `Folder:` line and the folder note, if any, go to standard error, so standard output stays the address.
`-q` hides the report and the `Folder:` line, but the folder note always prints, because it says the folder you asked for was not used.
To import a directory of exports into one folder:

```sh
for f in dashboards/*.json; do bin/observe import "$f" --folder Platform || echo "failed: $f" >&2; done
```

`bin/observe run <name>` runs a saved prompt that someone shared with everyone on the server, waits like a prompt does and prints the dashboard address; the name ignores case and its words need no quotes, and `bin/observe run` alone lists the shared prompts.
A key never sees private prompts, so share a prompt from the chat home before running it from the CLI.

Comments on a dashboard can be worked from the terminal:

- `bin/observe comments list <dashboard-id>` lists the comments of open threads, replies included, oldest first; `--status resolved` shows resolved threads instead, `--status all` shows both, and `--panel <panel-id>` narrows it to one panel.
- `bin/observe comments reply <dashboard-id> <comment-id> <text...>` adds a reply to a thread.
  Everything after the two ids is the reply text, so flags such as `--json` or `-q` must come before the ids.
  Without `--json` it always prints the thread and the new reply id, even with `-q`.
- `bin/observe comments resolve <dashboard-id> <comment-id>` resolves a thread, and `--version N` records the version that fixed it.
  `-q` hides its confirmation.
  If the thread was already resolved, `--version` is not applied and the command exits 1.

Add `--json` to any of them to print the API response unchanged.

`bin/observe annotate <text...>` adds an [annotation](annotations.md), such as a deploy marker, at the server's current time on every dashboard:

```sh
bin/observe annotate "Deploy checkout v1.4.2" --tags deploy,checkout
bin/observe annotate "Database failover" --dashboard <dashboard-id> --at now-15m
```

`--tags` takes comma-separated tags, and `--dashboard` puts the annotation on one dashboard instead of every dashboard.
`--at` places it in the past, as `now-<N>` with `s`, `m`, `h` or `d`, or as an RFC 3339 time such as `2026-11-01T10:00:00Z`.
It prints `Added annotation <id> at <time> on every dashboard`, which `-q` hides; a global annotation needs a key with the Editor role.

`bin/observe health` lists the dashboards with [health problems](dashboard-health.md) in the folders the key can see, one row per problem:

```sh
bin/observe health
bin/observe health --kind no_data --folder <folder-id>
bin/observe health repair <dashboard-id> <dashboard-id>
bin/observe health run
```

```text
ID                                    DASHBOARD  FOLDER  KIND             PANEL OR VARIABLE  CHECKED
0a1b2c3d-0000-4000-8000-000000000001  Checkout   Shop    no_data          Requests (p_rps)   2026-11-06T03:00Z
0a1b2c3d-0000-4000-8000-000000000001  Checkout   Shop    variable_failed  $pod               2026-11-06T03:00Z
Last check finished 2026-11-06T03:04Z: succeeded.
```

`--kind` takes `no_data`, `query_failed`, `variable_failed`, `duplicate_title` or `unopened`, `--folder` takes a folder id, and `--limit` lists up to 100 dashboards (50 by default).
A variable shows as `$<name>`, and the last line says how the last daily check went.
Listing exits 0 whatever it finds, and `--json` prints the response of `GET /api/v1/health`.

`bin/observe health repair <dashboard-id>...` asks the agent to repair 1 to 50 dashboards and prints one row per dashboard with `queued` and the request id, or the reason it was skipped: `active`, `nothing_to_repair`, `not_found`, `forbidden_role` or `queue_full`.
It needs an Editor key and a model on the server, and exits 1 when it queued no repair.
`bin/observe health run` starts a check of every dashboard now and needs an Admin key; results appear as each dashboard is checked.

`bin/observe open` prints an address on the server and opens it in the browser:

```sh
bin/observe open
bin/observe open <dashboard-id>
bin/observe open checkout-overview
```

With no argument it opens the server itself.
A dashboard id opens that dashboard, and so does the uid of a dashboard imported from Grafana, which the server resolves to the imported one.
An `http` or `https` address opens only when its scheme and host are the server's.
It needs only the server, not a key, and `--json` prints `{"url": "<address>"}` instead of the address.
Unlike `--open`, it exits 1 when the opener is missing or fails, after printing the address.

`bin/observe doctor` explains why a setup does not work, one check per row:

```text
CHECK       STATUS  DETAIL
server      ok      https://observe.example.com is ready
key         ok      ci-deploy (service_account)
role        ok      editor
version     ok      CLI v0.2.0, server v0.2.0
datasource  ok      prom (prometheus)
datasource  fail    logs (loki): connection refused
model       ok      anthropic claude-haiku-5-5, effort low
```

- `server` asks `/readyz` and names the readiness checks that fail; when the server cannot be reached, every later check is `skip`.
- `key` reads `/api/v1/me` with the saved or given key; with no key or a refused key, the later checks are `skip`.
- `role` warns for a Viewer or Commenter key, because those roles cannot create dashboards or annotations.
- `version` warns when the CLI and the server are different releases; a local build such as `dev` never warns.
- `datasource` is one row per datasource with its health, and a warning when there are none.
- `model` fails when the server has no model configured, because prompts cannot run.

`doctor` exits 1 when any check fails and 0 otherwise, since a warning never fails it.
`--json` prints `{"ok": <bool>, "checks": [{"check", "status", "detail"}]}` instead of the table, and colour marks `ok` and `fail` only in a terminal.

`bin/observe logs <query> --datasource <uid>` runs a log query on a Loki or ClickHouse datasource and prints the lines oldest first:

```sh
bin/observe logs '{app="shop"} |= "error"' --datasource loki --from now-15m
bin/observe logs '{app="shop"}' --datasource loki --tail
```

`--from` and `--to` default to `now-1h` and `now` and take the same times as a dashboard, and `--limit` keeps the newest 1 to 1000 lines (100 by default).
Each line shows its time in UTC, its level when the lines have one, the labels whose values differ between the lines, and the message.
The message is the first column named `body`, `message`, `msg`, `line` or `log`, else the first text column, as in the web app's logs panel.
`--tail` keeps asking every 2 seconds and prints only the lines it has not printed, with every label, until Ctrl-C, which exits 0.
`--json` prints the response body, or with `--tail` one JSON object per line with `time`, `level`, `labels` and `line`.
An invalid query exits 1 with the datasource's message.

`bin/observe traces <trace id> --datasource <uid>` prints the spans of one trace from a Tempo or ClickHouse datasource as a tree:

```sh
bin/observe traces 4bf92f3577b34da6a3ce929d0e0e4736 --datasource tempo
bin/observe traces 4bf92f3577b34da6a3ce929d0e0e4736 --datasource clickhouse --from now-7d
```

The id is 16 or 32 hexadecimal characters, and `--from` and `--to` (`now-24h` and `now` by default) bound the search.
On ClickHouse it reads the OpenTelemetry `otel_traces` table.
It prints a header with the span count, the total duration and the root span, then one row per span with its service, its name indented under its parent, its start after the trace start, its duration and `ok`, `error` or `-`.
A trace with no spans in the range exits 1, and `--json` prints the response body.

`bin/observe grafana connect <url> --token-stdin` stores a Grafana connection for [Switch from Grafana](switch-from-grafana.md) and tests it, and needs an Admin key.
The token comes only from standard input, so it never lands in shell history, and `--name` defaults to the URL's host:

```sh
printf '%s' "$GRAFANA_TOKEN" | bin/observe grafana connect https://grafana.example.com --token-stdin --name "Ops Grafana"
```

It prints `Connected <name>: <id>` and the test message, and `--json` prints the tested connection.
A failed test exits 1 with the message and the connection id; the connection stays stored, and you fix or remove it on the Grafana settings page.

`bin/observe grafana import <connection id>` imports dashboards from that Grafana, with `--folders` for some folders or `--all` for every one:

```sh
bin/observe grafana import <connection id>
bin/observe grafana import <connection id> --folders general,platform
bin/observe grafana import <connection id> --all
```

Without either flag it prints the folders as a table of uid, title and dashboard count and exits 2.
It then waits for the import, showing `Imported <done> of <total>` on a terminal, and prints how many dashboards were done, skipped and failed, a table of the failed ones with their error, and a notice for dashboards with panels that did not verify.
It exits 1 when any dashboard failed or the import failed or was canceled, and `--json` prints the final import.
The server keeps an import only until it restarts, and an interrupt stops the waiting while the import continues on the server.

`bin/observe completion bash`, `zsh` or `fish` prints a completion script for commands and flags.
Load it for the current shell, or save it where your shell reads completions:

```sh
source <(bin/observe completion bash)
bin/observe completion zsh > "${fpath[1]}/_observe"
bin/observe completion fish > ~/.config/fish/completions/observe.fish
```

The CLI reads its settings in this order: flags, the environment, then the saved file.

| Setting | Flag       | Environment variable |
| ------- | ---------- | -------------------- |
| Server  | `--server` | `OBSERVE_SERVER`     |
| Key     | `--token`  | `OBSERVE_TOKEN`      |

A refusal with the code `forbidden_role` comes in two kinds, and the CLI prints a hint for each.
When the message is "your role does not allow this", the hint says an Admin can change the key's role on the API keys page.
When the message is "you do not have access to this dashboard", the key cannot see that dashboard or its folder.
The hint then says that a folder Admin or an instance Admin decides who sees a folder, but that a key matches no access list entry, because entries name people and groups.
Use a key of an Admin service account or of the service account that created the folder, as described in [Folders](../teams/folders.md).

A saved key is sent only to the server it was saved for.
To use another server, pass `--token` or set `OBSERVE_TOKEN`.
Prefer `OBSERVE_TOKEN` to `--token`, since flags end up in shell history and the process list.
Add `--json` to any command to get the API response as it is.
`bin/observe logout` removes the saved key and keeps the saved server; the key stays valid on the server until someone revokes it.
