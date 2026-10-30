---
title: CLI
description: Build and use the observe command line client.
---

Build the `observe` command, which needs only Go:

```sh
make cli
```

It writes `bin/observe`.
Sign in once; it prompts for the key without echoing it:

```sh
bin/observe login --server https://observe.example.com
```

To pass the key on standard input instead, for scripts:

```sh
printf '%s' "$OBSERVE_TOKEN" | bin/observe login --server https://observe.example.com --with-token
```

`login` checks the key with the server and then saves the server and key to `~/.config/slashobserve/cli.json` (under `$XDG_CONFIG_HOME` when that is an absolute path), readable by you only.
Then give it a prompt:

```sh
bin/observe "checkout p99 latency, last 6h"
```

It prints progress and then the dashboard address.
`bin/observe dashboards list` lists dashboards, and `bin/observe --help` shows the rest.
`bin/observe dashboards delete <dashboard-id> --yes` deletes a dashboard with every version, comment and agent request, for everyone; it needs Edit on the dashboard, cannot be undone, and refuses to run without `--yes`.
`bin/observe import dashboard.json` imports a Grafana dashboard and prints its address; `--map '<grafana>=<uid>'` picks our datasource for a Grafana one, `--folder <title>` chooses the folder (see [Import Grafana dashboards](import-grafana.md)) and `--preview` shows the report without saving.
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
