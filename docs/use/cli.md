---
title: CLI
description: Build and use the observe command line client.
---

Build the `observe` command, which needs only Go:

    make cli

It writes `bin/observe`.
Sign in once; it prompts for the key without echoing it:

    bin/observe login --server https://observe.example.com

To pass the key on standard input instead, for scripts:

    printf '%s' "$OBSERVE_TOKEN" | bin/observe login --server https://observe.example.com --with-token

`login` checks the key with the server and then saves the server and key to `~/.config/slashobserve/cli.json` (under `$XDG_CONFIG_HOME` when that is an absolute path), readable by you only.
Then give it a prompt:

    bin/observe "checkout p99 latency, last 6h"

It prints progress and then the dashboard address.
`bin/observe dashboards list` lists dashboards, and `bin/observe --help` shows the rest.
`bin/observe import dashboard.json` imports a Grafana dashboard and prints its address; `--map '<grafana>=<uid>'` picks our datasource for a Grafana one, `--folder <title>` chooses the folder (see [Import Grafana dashboards](import-grafana.md)) and `--preview` shows the report without saving.
The import report, the `Folder:` line and the folder note, if any, go to standard error, so standard output stays the address.
To import a directory of exports into one folder:

    for f in dashboards/*.json; do bin/observe import "$f" --folder Platform || echo "failed: $f" >&2; done

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

A refusal with the code `forbidden_role` prints a hint that an Admin can change the key's role on the API keys page.
The same code also covers a dashboard or folder the key cannot see or change, so check the folder access before the role.

A saved key is sent only to the server it was saved for.
To use another server, pass `--token` or set `OBSERVE_TOKEN`.
Prefer `OBSERVE_TOKEN` to `--token`, since flags end up in shell history and the process list.
Add `--json` to any command to get the API response as it is.
