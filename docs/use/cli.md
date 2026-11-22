---
title: CLI
description: Install and use the observe command line client.
---

## Install

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

### Install by hand or build from source

On Windows, or to install by hand, download the `observe` archive for your platform and `checksums.txt` from the releases page, then check the archive against the checksums:

```sh
sha256sum --check --ignore-missing checksums.txt
```

On macOS, use `shasum -a 256 --check --ignore-missing checksums.txt`.
Unpack the archive and put `observe` on your `PATH`.
To build it yourself instead, which needs only Go, run `make cli`; it writes `bin/observe`.
The examples below use `bin/observe`; with an installed binary, run `observe`.

## Sign in

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
`login` takes the server from `--server` or `OBSERVE_SERVER`, and it does not take `--token`, so a key never lands in the shell history.
The server must be an `https` address, or an `http` address on `localhost`, `127.0.0.1` or `::1`; any other `http` address is refused.

## Send a prompt

Then give it a prompt:

```sh
bin/observe "checkout p99 latency, last 6h"
```

While the agent builds, it prints a line for each panel the build adds, as `Panel 3 of 8: Checkout p99 (timeseries)`; the count after `of` is the number of panels the agent planned, and it is left out when the agent gave no estimate or built more than it planned.
A follow-up prints only the panels it adds, not the ones the dashboard already had.
Only the key that made the request can read its build, so `status --wait` on someone else's request shows the steps without panel lines.
It ends with `Saved version N  https://observe.example.com/d/<dashboard id>/<slug>`, the address described in [Addresses in the web app](web.md).

### Output modes

In a terminal, the progress goes to standard output with the last line: the agent's latest step shows in one status line that rewrites in place, the panel lines stay, and the status line is cleared before the last line.
When standard output is not a terminal, as in `url=$(bin/observe "checkout p99 latency")` or a pipe, standard output holds only the address, and the steps, the panel lines and `Saved version N` go to standard error, one per line.
`-q` hides the steps, the panel lines and `Saved version N`, and prints only the address.
`--json` prints the finished request object of `GET /api/v1/observe/{id}` instead, and the steps and panel lines still go to standard error.
A prompt that only asks a question ends with an answer that stays in the chat, which the CLI prints as `observe investigate` does, followed by the hint `observe save-answer <request-id>`.
`bin/observe save-answer <request-id>` files that answer as an [observation](observations.md) and prints its address; only the person who asked can, and saving twice prints the same address.
When the prompt asked to change one existing dashboard, the answer ends with that dashboard's title and the `observe --revise --dashboard <id> '<prompt>'` command that asks for the change there.
Colour is used only in a terminal, and setting `NO_COLOR` to any value turns it off.

The CLI sends no time zone, so the agent writes the times in its answer in UTC, where the web app uses the zone of your browser.

`--open` opens the dashboard address in the browser once it is saved, with `open` on macOS, `xdg-open` on Linux and the URL handler on Windows; when the opener is missing or fails, the CLI prints a warning and still exits 0.
It opens only `http` and `https` addresses.
`status --wait`, `continue`, `run` and `import` accept `--open` too.

### Follow-ups and waiting

A prompt whose first word is a command name (`access`, `activity`, `annotate`, `annotations`, `cancel`, `chats`, `comments`, `completion`, `continue`, `dashboards`, `datasources`, `doctor`, `folders`, `grafana`, `health`, `help`, `import`, `investigate`, `login`, `logout`, `logs`, `metrics`, `o`, `observations`, `open`, `people`, `proposals`, `requests`, `retry`, `run`, `save-answer`, `saved-searches`, `search`, `share-answer`, `skill`, `status`, `teams`, `traces`, `version` or `webhooks`) must be quoted as one argument.
A prompt of one bare word, or of words whose first word is one letter or two away from a command name, such as `observe dashbords list`, is refused with exit code 2 before anything is sent, because it is almost always a mistyped command and a run costs money; quote the words as one argument to send them anyway.
A prompt sent with `--dashboard <dashboard-id>` is never refused, since a command name does not take that flag, so `observe --dashboard <id> faster` works.

`--dashboard <dashboard-id>` asks about that dashboard privately, as `observe investigate --private` does, and with `--revise` it sends the prompt as a follow-up that revises that dashboard, as described in [Chat and follow-ups](chat.md); `--revise` without `--dashboard` exits 2.
`--apply <finding-id>` with `--revise` and `--dashboard`, and no prompt, revises the dashboard with the suggested change of that answer instead of a prompt you write.
`--chat <chat-id>` sends it as the next turn of that chat of yours, from `observe chats list`, and may be combined with `--dashboard` when the chat is about that dashboard.
`--new-chat` starts a new chat even when `--revise` would join your latest chat on that dashboard, and exits 2 with `--chat`.
`--at '<anchor JSON>'` points the prompt at one spot on the `--dashboard` dashboard, as the anchor JSON object, for example `--at '{"kind":"position","pos":{"x":0.4,"y":0.3}}'`, or as a short form such as `--at pos:0.4,0.3`.

The short forms are `row:service=payments,span=authorize_card`, `time:2026-11-20T23:56Z`, `range:23:50Z..23:58Z`, `series:service=checkout` and `pos:0.4,0.3`, and every `--at` of the CLI takes them.
A time of day such as `23:50Z` means its latest occurrence not after now, and a short form that names no position points at the middle of the panel.
A time of day may carry seconds, as in `23:56:10Z`, and a range given as times of day must end in the past, so `range:10:00Z..09:00Z` asked at noon is refused.
A label value in a short form cannot contain a comma, because the comma separates labels; the JSON form takes any value.

`--no-wait` prints the request id and returns at once; `bin/observe status <request-id>` shows that request, and `--wait` makes it wait the way a prompt does.
An Admin can show anyone's request with `observe status`, read only.
`--timeout` (2 hours by default, so a large build can finish while you wait) stops the waiting and not the request, which keeps running on the server; `bin/observe status <request-id> --wait` waits again.
While a request waits for a worker, `observe status` prints `Queued, position N`, its place among the queued requests.
The order can shift, because a request is skipped while another revision of its dashboard is running.

`observe status` shows `Investigation yes` for a run started as an investigation that writes into an observation and `no` for an ask, and `--json` carries it as `investigation`; it never changes once the run is queued.
When the request points at a spot, `observe status` prints `Points at`, and `On block` for an observation block, which it prints alone for a question about a whole block.
`observe status` also shows the request's trigger, what started it: `person` for a prompt, a follow-up or an investigation the requester asked for, and `mention` for a run that a comment asked for, as `--to agent` does.
It also prints the request's `Title` and `Prompt`, so a request found in `observe requests list` can be told apart without opening the browser; for a request from someone else's private chat both read `hidden, asked in a private chat`.
An ask about an observation adds `About observation <id>`, also after its answer is saved into another observation.
Once a request has finished, `observe status` shows its `Cost`, the server's estimate in US dollars (`about $0.12`, `under $0.01`, or `-` when the model has no price).

While a request is running, `observe status` also prints a `Live text` section with what the agent is writing, the same text the Working row shows in the browser; it is not kept, so it is gone once the run ends, and `--json` carries it as `live_text`.

### Your requests

`bin/observe requests list` prints your agent requests, newest first with pinned ones on top, as a table of id, state, intent, dashboard, the spot it points at and title.
The state reads as in the web app, Running, Done or Stopped, and `--json` keeps the server's `queued`, `running`, `canceling`, `succeeded`, `failed` and `canceled`, which is also what `--state` takes.
`--state` (repeatable, or comma-separated), `--q` (a word of the title or prompt), `--pinned` or `--pinned=false`, `--dashboard <id>` and `--observation <id>`, the requests asked about that observation and the ones that wrote into it, narrow it.

`--anchored` keeps the requests you asked about a spot, and the server refuses it with `--scope all`, even for an Admin.
The `AT` column shows that spot in words, a dash when the request points at nothing, and `--json` carries it as `anchor` and `block_id`.
Only the person who asked a request sees its spot, so anyone else reads `null`.
`--scope all` lists every principal's requests and needs the Admin role.
It pages like every list: `--limit`, `--cursor` and `--all`, and the footer gives the total that matches.
Archived requests are left out.
A request that failed before it had a dashboard shows only here, and `observe status <request-id>` reads it.

`requests pin <id>`, `requests unpin <id>`, `requests rename <id> <title>` (`--clear` takes the title back to the start of the prompt) and `requests archive <id>` change one of your own requests.
`requests rename` and `o edit --title` also rename the chat or observation that shares the title ([Chat](chat.md#one-title-for-a-chat-and-its-observation)), and say so, or say that the other title was left as it is.
Each can be repeated without effect, so a retry after a dropped connection is safe.
An archived request stays readable with `observe status`.

`requests archive <id> --delete-observation --yes` first deletes the observation the request saved, as `observe o delete` does and with the same permission (Manage on its folder, or Edit when you started the observation), and archives the request only after that delete succeeds.
The two differ: the request is only archived and still reads with `observe status`, while the observation is deleted for everyone, with its blocks, RCA revisions and discussion, and cannot be undone.
A request that saved no observation is refused and left as it was.

### Your chats

Every prompt you send is a turn of a chat, which you and Admins can read and nobody else sees.
`bin/observe chats list` prints your chats, pinned ones first and then the one most recently asked in, as a table of id, whether it is pinned, the state of the last turn, the number of turns, when it was last asked, its dashboard and its title.
`--q` (a word of the title or any prompt), `--pinned` or `--pinned=false`, `--dashboard <id>` and `--observation <id>` narrow it, and it pages with `--limit` (at most 50), `--cursor` and `--all`.
`--scope all` lists every person's chats with an `OWNER` column and needs an Admin key.
`chats show <chat-id>` prints the chat and its turns, oldest first, each with its request id, state and the first line of its prompt, and a `points at` line under a turn that pointed at a spot, or an `is about block` line under one about a whole block; `observe status <request-id>` prints one turn in full.
An Admin may show anyone's chat, read only.

Every show is recorded in the audit trail as `chat.view` with the owner's id and every list as `chat.list` with its scope, each once per reader in 10 minutes.

`chats pin <chat-id>` and `chats unpin <chat-id>` pin a chat of yours to the top of the list and take it back, as Pin and Unpin do on the Chats page; pinning a pinned chat changes nothing, and only the chat's owner can do it.
The id must be a chat id from `chats list`; the id of a later turn or of any other request is refused with `is not one of your chats`.
`chats archive <chat-id> --yes` archives the chat and every request in it, which stay readable with `observe status`; dashboards and observations they made are kept.
It needs `--yes` because nothing brings an archived chat back to the lists, and only the chat's owner can do it: an Admin who can read it gets `chat_read_only`.

### Discovering what a datasource holds

A script or an agent with only a shell finds what to query with `bin/observe datasources`.
`list` prints each datasource's uid, type, name and health; the uid is what `--datasource` takes.
`metrics <uid>` lists metric names with their type and help (the OpenTelemetry metric names and their table on ClickHouse), `labels <uid>` lists label names, and `label-values <uid> <label>` lists one label's values.
`--search` filters by substring and `--limit` takes 1 to 500; when more exist the command says so and names the two ways to see them.
On ClickHouse, `labels` and `label-values` need `--table` or `--metric`, and `--from` with `--to` narrows the window Loki and Tempo look in.

`services [uid]` lists the services a Loki, Tempo, Jaeger, ClickHouse, Elasticsearch or OpenSearch datasource has seen in the last hour, with the label, column or attribute that holds them; `--window` takes a duration such as 15m, up to 6h, and `--signal` picks `logs` or `traces`.
Without a uid it asks every such datasource and prints one row per source and signal, and a source that cannot answer in time says so while the others still answer.
`--service checkout` keeps only the sources that hold a service whose name contains the text, case-insensitive, which answers which source holds a service.
The server caches an answer for about a minute and lists at most 200 services per source.

`schema <uid>` lists the tables of a ClickHouse, Elasticsearch or OpenSearch datasource, and `--table` lists one table's columns with their types.
A cut list names the `--offset` that reads the next page; a Tempo or Jaeger datasource has no tables, so `schema` lists its services and span names, and on Tempo its attribute names.
`sample <uid>` prints up to 20 recent rows or log lines (5 by default, `--limit` sets it), cut to 300 bytes a cell, so the real values show before a query is written.
ClickHouse needs `--table`; on Loki `--table` is a stream selector and leaving it out reads every stream.
They answer what the agent's `get_schema` and `sample_rows` tools answer.

`lint <uid> <query>` checks a query without running it and prints each diagnostic with the byte range it covers; it exits 1 only when a diagnostic is an error.
`log-volume <uid> <query>` counts every line a logs query matches per bucket, by level, and says from when it counted when the datasource ran out of time, and `trace-stats <uid> [<filter>]` counts spans, errors and the p50, p95 and p99 durations per bucket.
`trace-logs <uid> <trace-id>` prints the log lines of a Loki, ClickHouse, Elasticsearch or OpenSearch datasource that carry a trace id, or one span with `--span`; `--service` narrows the Loki streams it reads, and `--limit` takes 1 to 1000 lines (100 by default).
Each takes `--from` and `--to` (`now-1h` and `now` by default), and the two counting commands take `--buckets` (1 to 200); `--json` prints the server's answer unchanged.

### Exit codes

The CLI exits with these codes:

| Code | When |
| ---- | ---- |
| `0` | The command succeeded, including when `--open` could not open the browser and when `observe logs --tail` ends with Ctrl-C. |
| `1` | The server refused or failed the request, the agent request failed, the waiting stopped at `--timeout` or an interrupt, `observe open` could not open the browser, an `observe doctor` check failed, an `observe grafana import` had a dashboard fail, or `observe health repair` queued no repair. |
| `2` | The command line is wrong: an unknown command or flag, a missing argument, an invalid id, or a file or directory named on the command line that cannot be read. |

An error is one line on standard error, with the server's code in parentheses and any failing paths of an invalid Spec or panel below it.
With `--json` it is one JSON object, `{"error": "<message>", "code": "<code>"}`, so a script can branch on the code.
The code is the server's own (`not_found`, `forbidden_role`, `version_conflict` and the like), or `usage` for a wrong command line, `unauthenticated` for a missing or refused key, `unreachable` when the server cannot be reached, `interrupted`, `http_<status>` when the server sent no code, and `failed` for anything else.
An address that answers with a web page instead of JSON is reported as probably not a SlashObserve server, and a version that does not exist is reported with the latest version of the dashboard.

## Cancel and continue a request

`bin/observe cancel <request-id>` cancels a queued or running request and prints it.
A queued request stops at once, and a running one within a few seconds, once the call it is in returns.
A running request shows the state `canceling` until its worker stops, and one whose worker is gone ends at once.
The request ends `failed` with the code `canceled`, and what it built so far is kept.

`bin/observe continue <request-id>` starts a new request that takes over the kept build of a stopped one, waits the way a prompt does and prints the dashboard address.
It accepts `--no-wait` and `--timeout` like a prompt, and `--note <text>` gives the continued run an instruction; the note follows the same rule as a prompt (1 to 4000 characters, no control characters; see [Text rules](../reference/text-rules.md)) and the server's `400` message is printed as it is.
A stopped request with a build prints `Save it in the browser, or continue it: observe continue <request-id>` in `observe status`, and one that was already continued prints `Continued by <request-id>`.

`bin/observe retry <request-id>` starts a finished request again as a new request with the same prompt, waits the way a prompt does and prints the result.
It accepts `--no-wait`, `--open` and `--timeout` like a prompt.
Anyone who can see the request and may start that kind of request can retry it, and the new request is theirs and counts against their limits; a failed or canceled request can be retried once by the person or key that asked, while anyone else's retry is a request of their own that leaves it retryable, and a succeeded request queues a new request on every call.
A request that has not finished or was already retried fails with `request <request-id> is <state> and cannot be retried`, and a comment or suggestion request cannot be retried.
`observe status` prints the request's `State` line, one of `queued`, `running`, `canceling`, `succeeded`, `failed` or `canceled`, and a failed or canceled request with no build to continue prints `Run it again: observe retry <request-id>`, or `Retried by <request-id>` once it was retried.

Only the person who made a request can cancel or continue it; anyone else gets `only the requester can cancel or continue <request-id> (forbidden_role)`.
Canceling a request that already finished, or continuing one that was saved, already continued or has no build, fails with a one-line message naming the request.
Continuing a revision fails when its dashboard has a newer version than the build started from.

## Lists and paging

The commands that list rows in pages take `--limit` (a default and a maximum that its help gives, 50 rows by default for most lists and 20 for `activity` and `webhooks deliveries`), `--cursor` and `--all`.
They are `dashboards list`, `versions` and `revisions`, `comments list`, `annotations list`, `health`, `people list` and `invites`, `saved-searches list`, `proposals list`, `o list`, `requests list`, `search`, `activity` and `webhooks deliveries`.
When more rows exist, the table is followed on standard error by `Showing 5 of 23. More: --cursor <cursor>, or --all for every page.`; the total is printed where the server counts it, as `dashboards list` does, and otherwise the line says that more exist.
`--cursor` continues from the `next_cursor` a page ended with, which `--json` shows too, and a cursor the server does not recognise is refused with `invalid_request` and a hint to pass it exactly as printed.
`--all` follows the cursor to the last page, up to 1000 pages, and prints one list; with `--json` it is one object whose list holds every row, with `next_cursor` null and any `total_count` kept.
`--all` cannot be combined with `--cursor`, `activity --follow` cannot be combined with either, and `--limit` outside its range exits 2 before anything is sent.
The MCP list tools take their own, smaller page sizes and the same `next_cursor`.

## Dashboards

| Command | What it does |
| --- | --- |
| `bin/observe dashboards list [--order updated\|views\|name] [--exclude-audit]` | Lists dashboards, newest update first, with `--order views` most viewed in the last 30 days first, or with `--order name` by title, 50 at a time, up to 200, with a `VIEWS` column; when there are more it prints the total and the `--cursor` value for the next page, which only works with the same `--order`, and `--all` reads every page; `--exclude-audit` leaves out the dashboards in the Audit folder. |
| `bin/observe dashboards show <dashboard-id>` | Lists the panels of the latest version, or of one given with `--version N`, and how many views the dashboard had in the last 30 days; the folder is in the details, and a version that does not exist says what the latest one is; `--json` adds `view_count` and `window_days`. |
| `bin/observe dashboards versions <dashboard-id>` | Lists its versions. |
| `bin/observe dashboards viewers <dashboard-id>` | Lists, by name, the other people who have the dashboard open now; it needs View and prints nothing when nobody is looking. |
| `bin/observe dashboards revisions <dashboard-id> [<revision-id>]` | Lists the agent requests that changed it, newest first, or shows one in full. |
| `bin/observe dashboards tags [--exclude-audit]` | Lists every tag with the number of dashboards that carry it; `--exclude-audit` counts only dashboards outside the Audit folder. |
| `bin/observe dashboards move <dashboard-id> --folder <folder-id>` | Moves a dashboard to another folder without saving a version; it needs Edit on the dashboard and on the folder, and a personal folder takes no moves. |
| `bin/observe dashboards duplicate <dashboard-id> --title <title> [--folder <folder-id>]` | Saves the latest version as version 1 of a new dashboard and prints `Duplicated as <id>`; the copy goes to `--folder`, by default the dashboard's own folder when you may create there, else your default folder, and its queries are not run. |
| `bin/observe dashboards restore <dashboard-id> --version <n> [--parent <n>] [--summary <text>]` | Saves an earlier version as the newest version and prints `Restored version <n> as version <m>`. |
| `bin/observe dashboards panels <dashboard-id> --summary <text> [--file <patch.json>] [--remove <panel-id>]...` | Saves a new version that adds, replaces, removes or moves only the panels it names, runs only their queries, and prints `Saved version <n> of <title>`; the file holds `upsert`, `delete`, `layout`, `variables` and `dashboard`, the fields of the `update_panels` MCP tool. |
| `bin/observe dashboards diff <dashboard-id> [--from <n>] [--to <n>]` | Lists the changes between two versions as `OP`, `PATH`, `FROM` and `TO` columns, with values clipped to 60 characters; `--to` is by default the latest version and `--from` the one before it, and `--from 0` compares with an empty dashboard; a version that does not exist answers `version_not_found` with the latest version. |
| `bin/observe dashboards export <folder-id-or-title> --dir <dir>` | Writes the latest version of each dashboard in a folder to `<dir>/<folder>/<name>.json`; `--dir` defaults to the current directory. |
| `bin/observe dashboards apply --dir <dir>` | Saves every `.json` file of a directory back as dashboards, a file in a subdirectory into the folder of that name and a file directly in `--dir` into General, never deleting one, and exits 1 when any file failed, 2 when `--dir` is missing, is a file or cannot be read, and prints a warning and exits 0 when it holds no `.json` file (see [Dashboards as code](dashboards-as-code.md)). |
| `bin/observe dashboards schema` | Prints the JSON Schema every dashboard file must satisfy, the document the `get_spec_guide` MCP tool returns; it is part of the program and needs no server. |
| `bin/observe dashboards sample` | Prints a complete example Spec with variables, several panel types and a layout, to save as a `.json` file and edit before `apply`; it needs no server. |
| `bin/observe dashboards delete <dashboard-id> --yes` | Deletes a dashboard with every version, comment and agent request, for everyone; it needs Edit on the dashboard, cannot be undone, and refuses to run without `--yes`. |

It fails with `409` and the code `dashboard_being_created` while the agent request that is creating the dashboard is still queued or running.

`restore` reads the latest version for `--parent` when it is omitted, and answers `version_conflict` when someone saved since, so read the dashboard again and rerun it.
`panels` reads the latest version for `--parent` the same way, and a change that touches only other panels than a newer version merges onto it.
To duplicate a panel, upsert a copy under a new id with a `gridPos` below the original.
`bin/observe dashboards diff <dashboard-id> --from 2 --to 3` shows what changed between versions 2 and 3.
Dashboard pins (stars) are a signed-in person's and cannot be set with a key, so the CLI has no command for them; pin dashboards in the web app or over MCP.
`chats pin` and `requests pin` pin a chat or a request of yours, and `o pin` pins an observation to a dashboard as an annotation.

The lists and the panel table print as columns aligned with spaces.
`bin/observe --help` shows the rest.

### Import a Grafana dashboard

`bin/observe import dashboard.json` (or `-` for standard input) imports a Grafana dashboard of up to 8 MiB and prints its address; `--map '<grafana>=<uid>'` picks our datasource for a Grafana one, `--folder <title>` chooses the folder (see [Import Grafana dashboards](import-grafana.md)) and `--preview` shows the report without saving.
The import report, the `Folder:` line and the folder note, if any, go to standard error, so standard output stays the address.
A file whose Grafana uid an earlier import already holds, in a dashboard you can see, is not imported twice: the command exits 1, names that dashboard and its version, and says which flag to pass.
`--update <dashboard id>` saves the file as the next version of that dashboard (it needs Edit and keeps the folder) and `--as-new` makes another dashboard on purpose; `--preview` shows the same dashboard in an `Already imported as` line.
`-q` hides the report and the `Folder:` line, but the folder note always prints, because it says the folder you asked for was not used.

To import a directory of exports into one folder:

```sh
for f in dashboards/*.json; do bin/observe import "$f" --folder Platform || echo "failed: $f" >&2; done
```

Running the loop again reports each file an earlier import already made as failed, instead of making a second copy.

## Folders

`bin/observe folders` manages the [folders](../teams/folders.md) that hold dashboards, observations and saved searches:

```sh
bin/observe folders list
bin/observe folders create Team Payments
bin/observe folders rename <folder-id> Team Billing
bin/observe folders delete <folder-id>
bin/observe folders permissions show <folder-id>
bin/observe folders permissions grant <folder-id> --email ada@example.com --level edit
bin/observe folders permissions revoke <folder-id> --group payments
bin/observe folders permissions open <folder-id> --yes
bin/observe folders permissions admins-only <folder-id> --yes
```

`list` prints `ID`, `TITLE`, `DASHBOARDS` and `LEVEL` columns for the folders the key can see, and marks the General folder `(General)`, your My Dashboards `(yours)`, an Admins-only folder `(Only Admins)` and the [Audit folder](activity.md#audit-folder) `(Audit folder)`.
`create` joins its words into the title, needs the Editor role and prints `Created folder <title> (<id>)`; a title that another folder uses answers `folder_exists`.
`rename` takes everything after the id as the new title, so flags such as `--json` or `-q` must come before the id, and the General folder answers `general_folder`.
`delete` works only on an empty folder, asks for no `--yes`, and answers `folder_not_empty` when anything is still filed in it.

`permissions show` lists who the folder is restricted to, users first and then groups, or says the folder is open to everyone by role.
`permissions grant` adds a person (`--user <id>` or `--email <address>`) or a group (`--group <name>`) at a `--level` of `view`, `comment`, `edit` or `admin`, or changes the level of the entry that names the same person or group, and keeps every other entry; the first grant restricts the folder.
`permissions revoke` removes one entry and keeps the others; it answers that the name is not on the list when it is not, and needs `--yes` when it removes the last entry, because the folder is then open to everyone by role again.
`permissions open --yes` clears the whole list in one step.
`permissions admins-only --yes` clears the list and lets only Admins read the folder, its creator included; it needs an Admin key, and `permissions show` then says so.

Changing the list needs Manage on the folder, which an Admin key has on every folder except another person's personal one, and the General folder cannot be restricted.
Each command is one read followed by one replace of the whole list, so two people changing the same folder at once can overwrite each other.

### Ask for comment access

`bin/observe access` asks for [comment access](../teams/folders.md#ask-for-comment-access) to a dashboard or observation you can view, and lists the requests that wait for you:

```sh
bin/observe access request dashboard:<dashboard-id> --note "I am on call for checkout this week"
bin/observe access request observation:<observation-id>
bin/observe access requests
```

`request` takes the object as `<kind>:<id>`, with `dashboard` or `observation` as the kind, and an optional `--note` of one line, at most 200 characters.
It prints `Asked for comment access to <kind> <title>; the people who manage its folder will see it in their inbox` and changes no access.
A request that is still open answers `request_open` with the time it was made, an object you can already comment on answers `already_can_comment`, and one you cannot see answers `not_found`.
A service-account key may ask too; its request is granted by raising the service account's role.
`requests` prints `ASKED`, `WHO`, `WHO ID`, `OBJECT`, `TITLE`, `FOLDER` and `NOTE` columns for the open requests on the folders you manage, newest first and at most 100, or `No open access requests on folders you manage.`
Grant one with `bin/observe folders permissions grant <folder-id> --user <id> --level comment`, or by raising the role.
General takes no folder entry, so for a request whose `FOLDER` is General the command adds a note on standard error: raise the person's role on the People page, or move the object to a folder first.

## Saved prompts

`bin/observe run <name>` runs a saved prompt that someone shared with everyone on the server, waits like a prompt does and prints the dashboard address; the name ignores case and its words need no quotes, and `bin/observe run` alone lists the shared prompts.
A key never sees private prompts, so share a prompt from the chat home before running it from the CLI.

## Investigate

`bin/observe investigate <question...>` asks the agent a question about your data, as in [Investigate](investigate.md), and needs an Editor key:

```sh
bin/observe investigate why did checkout errors rise after 10:00
bin/observe investigate --dashboard <dashboard-id> --panel <panel-id> --from now-6h --to now what changed
bin/observe investigate --observation <observation-id> why did checkout errors rise
```

`--dashboard` makes it about one dashboard, `--panel` about one of its panels, and `--from` and `--to` set the window, together, as relative times such as `now-6h` or epoch milliseconds.
`--observation` names an existing observation to write into, by its id.
`--at '<anchor JSON>'` points the question at one spot of the dashboard or observation, as the anchor JSON object, for example `--at '{"kind":"position","pos":{"x":0.4,"y":0.3}}'`, or as a short form such as `--at series:service=checkout`, and `--block <block-id>` with `--observation` puts that spot in one of its blocks, or without `--at` points the question at that whole block.
`--panel` without `--dashboard`, one of `--from` and `--to` alone, an `--observation` that is not an id, or `--private` without exactly one of `--dashboard` and `--observation`, exits 2.

`--private` asks privately about one of `--dashboard` or `--observation`: it needs only Comment on it, writes nothing into it and opens no observation, and the answer is read only by you and, read only, by Admins (see [Private asks](../concepts/agent.md#private-asks)).
`bin/observe share-answer <request-id>` then shows that answer to everyone who can read what it was about: it opens a comment thread there with your question and the answer, and on an observation it also writes the answer onto the page; `--thread <thread-id>` adds both to that thread instead.
Only the person who asked can share, a run is shared once, and sharing again prints the same thread.
`bin/observe status <request-id>` and `bin/observe chats show <chat-id>` then name the thread, and `bin/observe comments withdraw <thread-id>` takes it back to only you while nobody else has written in that thread.

It shows the steps like a prompt, then prints the finding's title, its answer as plain text and its address.
Without `--observation`, each investigation opens a new observation that shows its run and receives its conclusion, and `--json` prints the finished request, which carries the `observation_id` of that observation, or null when no folder allowed one (see [Observations](observations.md#investigations-write-into-an-observation)).
With `--observation` the run and the conclusion go into that observation.
It must be one you can see, or the server answers `not_found`, and one you may comment on, or it answers `forbidden_role`.

`--open` opens the finding in the browser, `--no-wait` prints the request id and returns at once, and `--timeout` stops the waiting as it does for a prompt.
It exits 1 when the investigation fails or ends without a finding.
`bin/observe status <request-id>` on an investigation shows the finding's title and address, and with `--wait` prints the finding.

## Observations

`bin/observe o` works with observations, the pages of Markdown, panel and snapshot blocks that people and agents build about one question.
The group is named `o`, with `observations` as an alias, so a prompt that starts with the single word `o` or `observations` must be quoted.

`bin/observe o new <title...>` starts an observation and prints `Created <title> <address>`, even with `-q`.
It files the observation in `--folder <id>`, by default your My Dashboards, and `--from` and `--to`, RFC 3339 times that go together, set the range its panels show.
`--text <markdown>` or `--text-file <path>`, never both, also adds a first text block.

`bin/observe o show <observation-id>` prints the title, state, summary and address, then one line for each block with its position, id, kind, state, version, width, height (`auto` or pixels), range and, for a text block, its first line, or for a panel block its title.
The range of a panel block is its own window as `from..to` (RFC 3339, to the minute), or `page` when it follows the observation's range; the other blocks show `-`.
A block that someone who can only comment added shows the state `requested`.

`bin/observe o add <observation-id>` adds one block and prints `Added <kind> block <id> (live)`, or `(requested)` when you can only comment.
Give exactly one of `--text <markdown>`, `--text-file <path>`, `--panel-file <path>`, a JSON file with one panel whose query must return data, and `--snapshot-of <block-id>`, which freezes the data of a panel block.
`--from` and `--to` give a panel its own range, `--width third|half|two_thirds|full` (or `--half`) sets the width, `--height <pixels>` from 120 to 960 sets the height of a panel or snapshot block and `--position <n>` places it instead of appending it.
A panel whose queries use variables needs `--var name=value` for each, repeated for more values of the same name, and without one the block is refused with `variable_missing` and the name of the variable.
The server runs the check the MCP `add_block` tool runs: a panel whose query is invalid, or returns nothing in its range, is refused with the failing paths below the error, exits 1 and adds no block; `o edit-block --panel-file` is checked the same way.

`bin/observe o resolve <observation-id> --summary <line>` closes an observation and prints `Resolved <title>`.

`bin/observe o reopen <observation-id>` reopens a resolved observation and prints `Reopened <title>`.

`bin/observe o pin <observation-id>` pins an observation to a dashboard as an annotation, to `--dashboard <id>` or by default the dashboard its run was about, and prints `Pinned to dashboard <id> as annotation <id>`.
Pinning it again to the same dashboard prints the same annotation, and an observation with no dashboard to pin to exits 1 with `observation_has_no_dashboard`.

`bin/observe o dashboard <observation-id>` makes a dashboard in My Dashboards from its live panel blocks, or from one block with `--block <block-id>`, and prints `Dashboard <id>: <title>`.
Running it again prints the dashboard you made first; `--new` makes another.
It re-runs every query first, and an observation with no live panel block answers `observation_has_no_panels`.

`bin/observe o edit <observation-id>` changes the title with `--title <title>`, the range its panels show with `--from <time> --to <time>`, RFC 3339 times that go together, or removes the range with `--clear-range`, and prints `Updated <title>`.
Giving nothing, one of `--from` and `--to` alone, or a range with `--clear-range` exits 2.

`bin/observe o move <observation-id> --folder <folder-id>` files the observation in another folder and prints `Moved <title> to folder <id>`.
It needs Edit on the observation and on the folder, and My Dashboards takes no moves.

`bin/observe o delete <observation-id> --yes` deletes the observation for everyone, with its blocks, snapshots, RCA revisions, the public links of those revisions and its discussion, and prints `Deleted observation <id>`.
It needs Manage on the observation's folder, or Edit on it when you started the observation, and it cannot be undone, so without `--yes` it exits 2.
No MCP tool deletes anything, so an agent cannot do this.

`bin/observe o publish <observation-id>` publishes the observation as the next RCA revision and prints `Published revision <n> of <title>`, as in [Observations](observations.md#publish-as-rca).
It needs Edit on the observation's folder, an observation in My Dashboards is private and cannot be published until you move it to a shared folder, and it never creates a public link.

`bin/observe o withdraw <observation-id> <revision> --yes` deletes one published revision and prints `Withdrew revision <n> of <observation-id>`, as in [Observations](observations.md#withdraw-a-revision).
It needs Edit on the observation's folder, answers `rca_shared` while the revision has a live public link, and cannot be undone, so without `--yes` it exits 2.
No MCP tool deletes anything, so an agent cannot do this.

`bin/observe o rcas <observation-id>` lists the published revisions, newest first, as `REVISION`, `TITLE`, `STATE`, `PUBLISHED BY` and `PUBLISHED` columns.
`bin/observe o rcas <observation-id> <revision>` prints one revision: its title, its summary, each text block's Markdown and a line `snapshot <id>` for each snapshot block.

`bin/observe o list` prints the observations you can see, most recently changed first, with their id, title, state and last change; `--folder <id>`, `--state open|resolved` and `--title <text>` (a title that contains the text, ignoring case) narrow it, and `--limit` and `--cursor` page through the result.

`bin/observe o keep <block-id> --version <n>` makes a requested block live and needs the block's version from `o show`, and `bin/observe o discard <block-id>` deletes a requested block.
Keeping needs Edit on the observation's folder, and discarding is open to an editor and to whoever asked for the block.

Every subcommand takes `--json`, which prints the response of its API call unchanged, so `o discard --json` prints an empty line.
The commands need the folder access the web app needs for the same work, and a refusal prints the server's message and exits 1.
`bin/observe o edit-block <block-id> --version <n>` changes a live block with `--text` or `--text-file` (a text block), `--panel-file` (a panel block, with `--from` and `--to` for its own range), `--width third|half|two_thirds|full`, `--half` or `--full` (its width), `--height <pixels>` (a panel or snapshot block only, 120 to 960, or 0 for the automatic height), `--position <n>` (which moves the blocks in between) and `--follow-page` (which drops a panel block's own range, so it shows the observation's range again); it needs something to change.
`bin/observe o arrange <observation-id> <block-id>[:<width>[:<height>]]...` sets the order of the blocks you name, with an optional width and, for a panel or snapshot block, a height in pixels or `auto` after each id; a block you leave out keeps its place after the block it followed, and an id that is not a block of the observation is refused by name.
It takes no `--version`, because a layout change never conflicts with a content edit.
`bin/observe o remove-block <block-id> --version <n>` deletes a live block and the blocks after it close the gap.
Both need the block's current version from `o show`; a block that changed since answers `version_conflict`, so read it again and retry.
A snapshot block's body cannot change, and a requested block is kept or discarded instead.

## Saved searches

`bin/observe saved-searches list` prints the logs, trace and metrics searches you can view, most recently changed first, with their id, kind, title, folder, datasource and last change.
`--kind logs|traces|metrics` keeps one kind, `--folder <id>` one folder, and `--limit` and `--cursor` page through the result.

`bin/observe saved-searches save --kind logs|traces|metrics --title <title> --datasource <uid>` files a search in your My Dashboards, or in `--folder <id>`, and prints the address that reopens it, even with `-q`.
A service-account key has no My Dashboards, so a search saved with a key lands in the folder General, which everyone who can see General can read; pass `--folder` to file it elsewhere.
The search's id is printed on standard error as `Saved search <id>`, which `-q` hides, and `--json` prints it too.
A query with a lint error is refused with `query_invalid` and nothing is saved, and a search you already saved with the same title, datasource, query, filters, range and folder prints `Search <id> was already saved` and its address instead of a second copy; `--json` shows `replayed: true`.
`--from` and `--to` default to `now-1h` and `now` and are kept as written.
A logs search needs `--query` and may set `--columns a,b`; a trace search may set `--service`, `--span`, `--min`, `--max` and `--errors`, and `--query` on a Tempo datasource (TraceQL), a Jaeger datasource (`key=value` pairs, such as `service=checkout minDuration=1s`) or a ClickHouse datasource (SQL, which reopens the search in the SQL view); a metrics search needs `--query` on a Prometheus datasource and may set `--mode graph|table`.
`--min` and `--max` are span durations: a trace matches when one span that also fits the service and span name is that long.
A flag of another kind is refused with exit code 2 before any request is sent.

`bin/observe saved-searches update <id>` changes what a saved search runs and keeps its kind, datasource, title and folder.
`--query` and `--from` or `--to` replace one value each, and any filter flag (`--columns`, `--service`, `--span`, `--min`, `--max`, `--errors`, `--mode`) replaces all the filters, so repeat the ones to keep; `--clear-filters` removes them all and `--query ""` removes the query of a trace search.
Only the flags you give change, a call with none exits 2, and the result must pass the checks a save does, so a trace filter on a logs search is refused with the server's message and exit 1.

`bin/observe saved-searches rename <id> <title...>` takes everything after the id as the new title, so flags such as `--json` or `-q` must come before the id.
`bin/observe saved-searches move <id> --folder <folder-id>` files the search in another folder and shares it with that folder's readers; My Dashboards takes no moves, so save the search there instead.
`bin/observe saved-searches delete <id>` removes it.
Update, rename, move and delete need Edit on the saved search, and a move also needs Edit on the folder.
Their text output names what was done, and `-q` hides it.

`--json` prints the response of the matching `/api/v1/saved-searches` route unchanged, and nothing for `delete`.
The kind and datasource of a saved search never change: save a new one instead.

## Comments

Comments on a dashboard, an annotation or a proposal can be worked from the terminal, and so can the discussion of an observation.
A target is written `<kind>:<id>` with kind `dashboard`, `annotation`, `observation` or `proposal`, for example `dashboard:0a1b2c3d-0000-4000-8000-000000000001`.

- `bin/observe comments list <dashboard-id>` lists the comments of open threads, replies included, oldest first; `--status resolved` shows resolved threads instead, `--status all` shows both, and `--panel <panel-id>` narrows it to one panel.
  `bin/observe comments list --on <kind>:<id>` lists the comments of any target the same way, and exactly one of the dashboard id and `--on` is required; a dashboard id gives what `--on dashboard:<id>` gives.
  `--json` prints the server's page of `threads` and `messages`: a thread carries its `anchor`, the spot it points at in the shape `--at` takes as JSON, and a run's reply carries its `answer` when you can read it.
  Breaking: the dashboard id form used to print the legacy `{"comments": [...], "next_cursor": ...}` object, and now prints the same thread page as `--on`, with `threads`, `messages`, `next_cursor` and `can_comment`, so `comments` is gone.
  A script that read `jq '.comments[]'` reads `jq '.messages[]'` instead: a message's `thread_id` names its thread in `threads` and replaces `parent_id` (a reply is a message whose `thread_id` is not its own `id`), its `actor.label` replaces `author.name`, and `status`, `panel_id`, `panel_present` and `resolved_by_version` move from the comment to its thread.
  In the table, the AUTHOR column of the dashboard id form now shows the actor label, such as `Observe Agent for Ada` or `Ada (via Claude Code)`, instead of the bare name, and the COMMENT column of a run's reply shows the answer's title after `Answered.`.
- `bin/observe comments add --on <kind>:<id> <text...>` starts a thread and prints `Started thread <id> on <kind>:<id>`, even with `-q`.
  On a dashboard, `--panel <panel-id>` anchors it to a panel and `--version N` to a version, the latest by default; the server refuses both on an observation, an annotation or a proposal.
  On an observation, `--block <block-id>` anchors the comment to one of its blocks, and without it the comment is about the whole observation; the server refuses `--block` on any other target.
  `--at '<anchor JSON>'` points the thread at one spot, for example `--at '{"kind":"position","pos":{"x":0.4,"y":0.3}}'` or `--at time:2026-11-20T23:56Z`, and on an observation with `--block` at a spot in that block.
  Everything after the flags is the comment text, so `--on`, `--panel`, `--block`, `--at`, `--version`, `--to`, `--json` and `-q` must come before it.
  A comment is for everyone and asks no agent unless you add `--to agent`; see [Ask an agent from a comment](#ask-an-agent-from-a-comment).
- `bin/observe comments reply <comment-id> <text...>` adds a reply to a thread, and the id can be any comment of the thread.
  `bin/observe comments reply <dashboard-id> <comment-id> <text...>` does the same on a dashboard through the dashboard comment endpoint; when the first two arguments are both ids, the first is the dashboard id.
  Everything after the ids is the reply text, so flags such as `--json`, `-q`, `--block`, `--reply-to` or `--to` must come before the ids.
  `--block <block-id>` anchors the reply to one block of an observation, and `--reply-to <comment-id>` names the comment of the same thread the reply answers, which the web app shows quoted.
  `--to`, `--block` and `--reply-to` work only with the comment id alone, because the dashboard comment endpoint takes none of them.
  Without `--json` it always prints the thread and the new reply id, even with `-q`.
- `bin/observe comments edit <comment-id> <text...>` replaces the text of your own comment, while you can still comment on its target; everyone else gets 403 `not_author`.
  `-q` hides its confirmation.
- `bin/observe comments delete <comment-id>` deletes one comment: its author may while they can still comment, unless an agent run wrote it, and anyone who manages the target's folder may delete any comment, including an agent's reply.
  Deleting the first comment of a thread deletes the whole thread, which its author may do only while nobody else has written in it; everyone else gets 403 `not_author`.
  `-q` hides its confirmation.
- `bin/observe comments withdraw <comment-id>` withdraws an answer you shared with `share-answer` back to only you: it deletes the thread when the share opened it and otherwise only the shared question and its answer, takes the id of the thread or of any of its comments, and works only for the person who asked while every comment in the thread is theirs or their run's reply.
  On an observation the answer's text that was written onto the page stays there, while the run and its answer leave the observation's lists, the answer can be shared again later, and `-q` hides its confirmation.
- `bin/observe comments apply <comment-id> <finding-id>` makes the change an answer in a dashboard thread suggests: the finding id is the answer's, from `bin/observe status <request-id> --json`, and the answer must have been given in that thread.
  It posts "Revise the dashboard: <suggested change>" in the thread as you and starts a run, yours and taken by the agent that answered, which saves a new version when you can edit the dashboard and opens a proposal when you can only comment.
  Applying the same answer again while that run is queued or running, or after it succeeded, starts nothing and prints `Already applied by run <id>`; an answer without a suggested change is refused with `no_suggested_change`.
- `bin/observe comments people --on <kind>:<id>` lists up to 20 people, other than you, who can see the target, with the mention to paste into a comment.
  `--search <text>` keeps those whose name or email contains the text, and a notice says when more people match.
  A person who cannot see the target is not listed, and mentioning one is refused with `invalid_mention`.
- `bin/observe comments resolve <comment-id>` resolves a thread, and `bin/observe comments resolve <dashboard-id> <comment-id>` does the same through the dashboard endpoint.
  A Commenter gets `not_starter` on a thread someone else started; an Editor resolves any.
  `--version N` records the version that fixed a dashboard thread.
  `-q` hides its confirmation.
  If the thread was already resolved, `--version` is not applied and the command exits 1.
- `bin/observe comments reopen <comment-id>` reopens a resolved thread, and `-q` hides its confirmation.

Mention a person in the text as `@[Name](user:<id>)`, the built-in agent as `@observe`, one of your own connected agents as `@[Name](grant:<id>)` and a service account that takes runs as `@[Name](service_account:<id>)`, at most 20 mentions in a comment.
A mention shows in the [activity log](activity.md) and asks no agent unless you add `--to agent`.

### Ask an agent from a comment

`--to agent` on `comments add` or `comments reply` asks the one agent the text mentions, else the thread's agent, else `@observe`, in the same transaction as the comment, and that agent becomes the thread's agent.
Without it the comment is for everyone, asks no agent and turns the thread's agent off, so the web app's plain replies stop asking it until someone mentions an agent again.
The agent's token is removed from the text to make the run's prompt, and the earlier messages of the thread, the last 20 and at most 8000 characters, go with it as context.
A dashboard thread starts a read-only ask, which a Commenter may start and which answers in the thread without changing the dashboard; an observation thread starts a read-only ask, which needs Comment on the observation and answers in the thread without writing on the page; an annotation or a proposal thread starts none.
Mentioning more than one agent is refused with `too_many_agents` and nothing is written.

An observation has one discussion, so `comments add` on an observation that has one adds the comment to that discussion, unless `--at` points it at a spot of its own.
You can mention only people who can see the target, your own connected agents and service accounts that take runs; anything else is refused with `invalid_mention`, and a service-account key can never mention a connected agent.
When the comment is stored but the run cannot start, the command succeeds and prints `No run started (<code>)` on stderr with the code `mention_target_unsupported`, `agent_unavailable` (`@observe` on a server with no model provider, or a thread agent that was disconnected), `agent_not_yours` (the thread's agent is someone else's connected agent), `forbidden_role` or `too_many_runs`.
When it starts, the command prints `Started run <id>, taken by <builtin|grant|service_account>`.

When the run ends, it posts one reply in the same thread and never resolves the thread or reopens a resolved one.
The reply reads "Observe Agent for <your name>" when the built-in agent took the run, "<your name> (via <client>)" when one of your connected agents did, and the service account's own name when a service account did.
The reply is a status line the server writes, not the model's text: `Saved version N of this dashboard.`, `Opened a proposal: <title>.`, `Answered.` when the run saved an answer, which the thread carries with the reply so everyone who can read the thread reads it, and `comments list` prints the answer's title after the line, `No agent picked this up within 10 minutes.`, `The run failed: <reason>.` or `The run was stopped before it finished.`
The run's own page has the detail.
A retry or a continuation of the run answers in the same thread and goes to the same agent; when that connected agent was revoked, has expired or is not yours, or the service account no longer takes runs, the command fails with `assignee_unavailable` and starts nothing.
Someone who cannot comment on the observation of an observation thread, and retries an answer from it that was saved as an observation, gets a private ask of their own about the saved observation, and nothing is posted in the thread.

Add `--json` to any of them to print the API response unchanged.

## Proposals

A proposal is a change that someone who cannot edit a dashboard, or whose connection is in propose mode, saved for a person who can edit to merge; see [Proposals](../concepts/proposals.md).

`bin/observe proposals list <dashboard-id>` prints the open proposals of a dashboard, newest first, with their id, state, revision, `behind main` when the dashboard has a newer version than the one the proposal builds on, and title.
`--state merged`, `--state discarded` or `--state all` shows the closed ones, and `--limit` and `--cursor` page through the result.

`bin/observe proposals show <proposal-id>` prints the proposal's summary, then `Changes`, one line for each panel, variable or setting it adds, changes, removes or moves, such as `added panel p4`, then `Conflicts` with the units a merge onto the latest version would hit, and the actions you may take now.

`bin/observe proposals merge <proposal-id> --revision <n>` merges the proposal as the next version of its dashboard and prints `Merged as version <n>`, or `Already in the dashboard` when the dashboard already held its change.
When a `--take` keeps the dashboard's side, it also prints which units the proposal's change was dropped for, and `Merged without a new version` replaces `Already in the dashboard` if nothing was saved.
`--revision` is the revision `show` printed, and a proposal that gained a newer one is refused with `proposal_changed`.
For each conflict, `--take <kind>:<id>=ours|theirs` names the side that wins, `ours` keeping the dashboard's unit and `theirs` taking the proposal's, and it repeats: `--take panel:p1=theirs --take setting:title=ours`.
A `--take` that is not `<kind>:<id>=ours|theirs` with kind `panel`, `variable` or `setting` exits 2.
Merging needs Edit on the dashboard and a connection that is not in propose mode, and it runs the queries of the panels it changes like any save.

`bin/observe proposals discard <proposal-id> [--reason <text>]` closes the proposal without saving it and prints `Discarded`; `--reason` (up to 500 characters) is shown on the proposal, and printed by `proposals show`.
Its author may discard it, and so may anyone who may merge it.

`-q` hides the confirmation of `merge` and `discard`, and `--json` prints the API response unchanged.

## Annotations

`bin/observe annotate <text...>` adds an [annotation](annotations.md), such as a deploy marker, at the server's current time on every dashboard:

```sh
bin/observe annotate "Deploy checkout v1.4.2" --tags deploy,checkout
bin/observe annotate "Database failover" --dashboard <dashboard-id> --at now-15m
```

`--tags` takes comma-separated tags, and `--dashboard` puts the annotation on one dashboard instead of every dashboard.
`--at` places it in the past, as `now-<N>` with `s`, `m`, `h` or `d`, or as an RFC 3339 time such as `2026-11-01T10:00:00Z`.
It prints `Added annotation <id> at <time> on every dashboard`, which `-q` hides; a global annotation needs a key with the Editor role.
An `--at` more than a day in the future still saves the annotation and prints a warning on standard error, because a wrong year is the usual cause.

`bin/observe annotations list` prints annotations newest first as `ID`, `TIME`, `DASHBOARD`, `TAGS` and `TEXT` columns:

```sh
bin/observe annotations list --dashboard <dashboard-id> --from 2026-11-01T00:00:00Z --tag deploy
bin/observe annotations update <annotation-id> --text "Deploy checkout v1.4.3" --tags deploy
bin/observe annotations delete <annotation-id>
```

Without `--dashboard` it lists only the global annotations, and with it that dashboard's annotations and the global ones.
`--from` and `--to` are RFC 3339 times, `--tag` can be repeated to match any of several tags, and `--limit` and `--cursor` page through the result.

`bin/observe annotations update <annotation-id>` changes the text with `--text`, the tags with `--tags` (a comma-separated list that replaces the old tags, and `--tags ""` removes them) or both, prints `Updated annotation <id>`, and keeps the annotation's time, dashboard and author; only its creator or an Admin may.

`bin/observe annotations delete <annotation-id>` prints `Deleted annotation <id>`.

## Dashboard health

`bin/observe health` lists the dashboards with [health problems](dashboard-health.md) in the folders the key can see, one row per problem:

```sh
bin/observe health
bin/observe health --kind no_data --folder <folder-id>
bin/observe health repair <dashboard-id> <dashboard-id>
bin/observe health run
```

```text
ID                                    DASHBOARD  FOLDER  STATE    KIND             PANEL OR VARIABLE  CHECKED            REPAIR
0a1b2c3d-0000-4000-8000-000000000001  Checkout   Shop    failing  no_data          Requests (p_rps)   2026-11-06T03:00Z  -
0a1b2c3d-0000-4000-8000-000000000001  Checkout   Shop    failing  variable_failed  $pod               2026-11-06T03:00Z  -
Last check finished 2026-11-06T03:04Z: succeeded.
```

`--kind` takes `no_data`, `query_failed`, `variable_failed`, `duplicate_title` or `unopened`, `--folder` takes a folder id, and `--limit` lists up to 100 dashboards (50 by default), with `--cursor` and `--all` for the rest.
`STATE` is the dashboard's health state and `REPAIR` the outcome of its latest repair, both described in [Dashboard health](dashboard-health.md#from-the-cli).
A variable shows as `$<name>`, and the last line says how the last daily check went.
A dashboard listed with no problem shows one row with `-` as its kind and target, and when a listed dashboard is `outdated`, a line after the table says its problems belong to an earlier version and a recheck is pending.
Listing exits 0 whatever it finds, and `--json` prints the response of `GET /api/v1/health`.

### Repair dashboards and run a check

`bin/observe health repair <dashboard-id>...` asks the agent to repair 1 to 50 dashboards and prints one row per dashboard with `queued` and the request id, or the reason it was skipped: `active`, `nothing_to_repair`, `not_found`, `forbidden_role`, `provisioned` (a starter dashboard, which only a release changes) or `queue_full`.
It needs an Editor key and a model on the server, and exits 1 when it queued no repair.
`bin/observe health run` starts a check of every dashboard now and needs an Admin key; results appear as each dashboard is checked.

## Open an address

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

## Doctor

`bin/observe doctor` explains why a setup does not work, one check per row:

```text
CHECK       STATUS  DETAIL
server      ok      https://observe.example.com is ready
key         ok      ci-deploy (service_account)
role        ok      editor
version     ok      CLI v0.2.0, server v0.2.0
datasource  ok      prom (prometheus)
datasource  fail    logs (loki): The datasource is not answering (The server could not connect to the datasource's address)
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

## Datasources

`bin/observe datasources list` prints each datasource's uid, type, name, health, source (`file` or `app`), who can query it (`everyone` or `admins`, in an Audience column only an Admin key sees), the dashboards that use it and when it was last checked.
An unhealthy one shows the failure and, when the server knows it, the cause.
`bin/observe datasources recheck <uid>` skips the server's 10 second health cache, prints the new status and exits 1 while the datasource is unhealthy.
Any role can list and recheck; `--json` prints the API response.

An Admin can also run `datasources get <uid>` (address, login and which credentials are set, never their values), `add <uid> --type <type> --address <address>`, `edit <uid>`, `test <uid> [--saved]` and `remove <uid>`.
They manage the datasources added in the app; one from the datasources file cannot be changed here.
`--type` is `prometheus`, `loki`, `tempo`, `jaeger`, `clickhouse`, `elasticsearch` or `opensearch`, and `--address` is an `http` or `https` URL that passes the [outbound allowlist](../operate/outbound-access.md), or `host:port` for ClickHouse.
`add` and `edit` also take `--name`, `--audience` (`everyone`, the default, or `admins`, who alone can then query it), `--username`, `--timeout`, the limits `--max-series`, `--max-points`, `--max-rows` and `--max-concurrent-queries`, `--grafana-uid` (repeatable, the Grafana datasource uids this one stands for on import), and the settings of one type: `--scrape-interval` for Prometheus, `--tenant-id` for Loki, `--database` and `--tls` for ClickHouse, and `--index`, `--time-field`, `--query-language` (`kql` or `lucene`) and the `--field-level`, `--field-message`, `--field-service`, `--field-span-id` and `--field-trace-id` index fields for Elasticsearch and OpenSearch.
Credentials come from `--password-file`, `--bearer-token-file`, `--api-key-file` or `--header NAME=FILE`, where `-` reads standard input once, and `edit --clear` removes a stored one (`password`, `bearer_token`, `api_key` or `header:<name>`), while a credential you do not pass keeps its stored value.
`test` runs the health check on the definition you give and saves nothing; with `--saved` it starts from the saved datasource, so its stored credentials are used.
`test` exits 1 while the datasource is unhealthy, and `remove` is refused with the list of dashboards and saved searches that still use the datasource.
See [Add datasources in the app](../connect/datasources-in-the-app.md).

## Logs

`bin/observe logs [<query>] --datasource <uid>` runs a log query on a Loki, ClickHouse, Elasticsearch or OpenSearch datasource and prints the lines oldest first, in the language of the datasource: LogQL, SQL, or KQL with the `lucene:` prefix and Query DSL forms of [Logs and traces](logs-and-traces.md#query-languages-on-elasticsearch-and-opensearch):

```sh
bin/observe logs '{app="shop"} |= "error"' --datasource loki --from now-15m
bin/observe logs '{app="shop"}' --datasource loki --tail
```

`--from` and `--to` default to `now-1h` and `now` and take the same times as a dashboard, and `--limit` keeps the newest 1 to 1000 lines (100 by default).
Each line shows its time in UTC, its level when the lines have one, the labels whose values differ between the lines, and the message.
The message is the first column named `body`, `message`, `msg`, `line` or `log`, else the first text column, as in the web app's logs panel.
On ClickHouse the query is SQL that reads the time range with `$__timeFilter(Timestamp)` and selects `Timestamp` and `Body`; the rows come back as they are.
A ClickHouse result with no `Timestamp` or message column prints as a table instead of log lines.
On Elasticsearch and OpenSearch the query is KQL, `lucene:` or a Query DSL body, and the lines come from the same search the logs page runs, newest first from the server and printed oldest first here; `--limit` is at most 500 there, because a search page holds at most 500 lines.
`--json` on these two stores lists the rows oldest first too, the newest `--limit` of them.

`--tail` keeps asking every 2 seconds and prints only the lines it has not printed, with every label, until Ctrl-C, which exits 0.
On Elasticsearch and OpenSearch `--tail` asks from the newest line it has printed, so a line indexed late with an older time is not printed.
`--json` prints the response body, or with `--tail` one JSON object per line with `time`, `level`, `labels` and `line`.
An invalid query exits 1 with the datasource's message.

### Facets, patterns and context

Three flags read the lines differently, as the logs page does, and print the response body with `--json`:

```sh
bin/observe logs '{app="shop"}' --datasource loki --facets
bin/observe logs '{app="shop"}' --datasource loki --patterns
bin/observe logs '{app="shop"}' --datasource loki --context 2026-11-01T10:00:00.123456789Z
```

`--facets` counts the top values of each label or field instead of printing lines.
Each field prints a header `<name> (<kind>, exact|sampled)`, with `bytes` added for a Loki index label, which counts bytes, then one `<count>  <value>` row per value.
A sampled field counts the newest lines only, and `--field <name>`, which can be repeated, counts just the fields you name.

`--patterns` groups the lines into patterns and prints a `COUNT` and `PATTERN` table, with every variable part written `<*>`; the lines matched go to standard error, with the number sampled when the server read the lines itself and `counted by loki` when Loki's own pattern API counted them.
`--limit` is the most patterns to print, up to 100.

`--context <time>` prints the lines around one line: the lines before it oldest first, a `-- <time> --` line, then the lines after it.
Give the line's time as nanoseconds since the epoch, or as an RFC 3339 time with nanoseconds, so that two lines in the same millisecond stay apart.
`--before` and `--after` set how many lines to read on each side, 0 to 500 and 50 by default, and a note on standard error says when more exist.

`--facets`, `--patterns`, `--context` and `--tail` cannot be combined, and `--field` needs `--facets`, `--before` and `--after` need `--context`; each mistake exits 2.

### Ask for logs in plain language

`--ask` takes a question instead of the query, and the server writes the query with [Ask](logs-and-traces.md#ask-in-plain-language):

```sh
bin/observe logs --ask "checkout errors" --datasource loki --from now-6h
bin/observe logs --ask "checkout logs mentioning payment" --datasource clickhouse --dry-run
```

It prints `Query: <query>`, the explanation and one `Warning: <warning>` line per warning on standard error, then runs the query as if you had typed it, `--tail` included.
`--dry-run` prints them and runs nothing, and needs `--ask`.
A question the server cannot turn into a query exits 1 with the reason, and so does a server with no model.

## Traces

`bin/observe traces [<trace id>] --datasource <uid>` prints the spans of one trace from a Tempo, Jaeger or ClickHouse datasource as a tree:

```sh
bin/observe traces 4bf92f3577b34da6a3ce929d0e0e4736 --datasource tempo
bin/observe traces 4bf92f3577b34da6a3ce929d0e0e4736 --datasource clickhouse --from now-7d
```

The id is 16 or 32 hexadecimal characters, and `--from` and `--to` (`now-24h` and `now` by default) bound the search.
On ClickHouse it reads the OpenTelemetry `otel_traces` table.
It prints a header with the span count, the total duration and the root span, then one row per span with its service, its name indented under its parent, its start after the trace start, its duration and `ok`, `error` or `-`.
A trace with no spans in the range exits 1, and `--json` prints the response body.

### Ask for traces in plain language

`--ask` takes a question instead of the trace id and prints 100 matching traces, the newest unless `--sort` says otherwise:

```sh
bin/observe traces --ask "checkout requests slower than 2s with errors" --datasource tempo
bin/observe traces --ask "slow checkout requests" --datasource clickhouse --dry-run
bin/observe traces --ask "checkout requests with errors" --datasource clickhouse --sort -duration
```

It prints the answer on standard error, as `Filters: service=checkout min=2s errors` or `Query: <TraceQL>`, with the explanation and any warnings.
Then it runs the same search as the Traces page: TraceQL on Tempo, `key=value` pairs on Jaeger, and on ClickHouse a query over `otel_traces` grouped by trace, so each row is the whole trace with its root, its span count and its full length.
`--sort start|service|span|duration|spans|errors` orders the traces, with a minus in front for descending, such as `-duration`: ClickHouse sorts everything that matches before it takes the 100, and Tempo and Jaeger sort only the 100 they return.
Each row shows the trace's start, service, span, duration, span count and error count, and a column the datasource does not return stays blank.
When the list is incomplete, a `Note:` line on standard error says why and how to get more, such as that the page is full, when it tells you to narrow `--from` and `--to` or the question, or that Tempo stopped before it searched everything; the same sentence is `meta.note` in the response body and `note` in the MCP query tools' result.
`--dry-run` prints the answer and searches nothing, and `--json` prints the search's response body.

### Run SQL over ClickHouse traces

`--query` takes SQL over `otel_traces` on a ClickHouse datasource instead of the trace id, and runs it as the SQL view of the Traces page does:

```sh
bin/observe traces --query 'SELECT TraceId, ServiceName, Duration FROM otel_traces WHERE $__timeFilter(Timestamp) ORDER BY Duration DESC' --datasource clickhouse
```

It goes through the same checks as that view and the SQL of `observe logs`: one read-only `SELECT`, the range of `--from` and `--to` read through a macro such as `$__timeFilter(Timestamp)`, and the 15 second limit, and a query that breaks one exits 1 with the server's sentence.
It prints the first 100 rows as a table with a column per selected column and times in RFC 3339, says on standard error when more rows match, and `--json` prints the response body.
The rows come back as the SQL wrote them, with its own `ORDER BY`, `LIMIT` and columns even when it selects `Timestamp`, and a `LIMIT` above 100 is cut to the first 100 rows.
Single quotes keep the shell from expanding `$__timeFilter`.
`--query` works on ClickHouse only, takes no trace id and cannot be combined with `--ask`, `--dry-run`, `--sort` or `--compare`; each mistake exits 2.

### Compare traces

`--compare` explains what sets some traces apart from others, as the compare view of the Traces page does:

```sh
bin/observe traces --compare 4bf92f3577b34da6a3ce929d0e0e4736,5c1d7a2e9f8b4a3d --baseline 6d2e8b3fa09c4b4e9f7a1c2d3e4f5061 --datasource tempo
```

`--compare` takes the trace ids to explain and `--baseline` the trace ids to compare them with, each comma separated or repeated, and a trace id is in only one of them.
It prints `ATTRIBUTE`, `VALUE`, `SELECTED` and `BASELINE` columns: the attribute values whose share of the selected traces and share of the baseline traces differ by at least 10 percentage points, the largest gap first.
`--limit` is the most differences to print, 1 to 100 and 20 by default, and `--from` and `--to` bound where the traces are read.
A note on standard error says when some traces were not found in the range or more differences exist than `--limit` allows, and `--json` prints the response body.
`--compare` takes no trace id argument and cannot be combined with `--ask`, and `--baseline` and `--limit` need `--compare`; each mistake exits 2.

## Metrics

`bin/observe metrics query <promql> --datasource <uid>` runs a PromQL query on a Prometheus datasource and prints one line per series:

```sh
bin/observe metrics query 'sum by (service) (rate(http_requests_total[5m]))' --datasource prom --from now-6h
bin/observe metrics query 'up == 0' --datasource prom --instant
bin/observe metrics query --ask "requests per second by service" --datasource prom --dry-run
```

`--from` and `--to` default to `now-1h` and `now` and take the same times as a dashboard.
Each line shows the series name with its labels, then the number of points and the last, minimum, maximum and mean value.
`--instant` evaluates the query once at the end of the range and prints the value of each series.
`--ask` takes a question instead of the query, and the server writes the PromQL with [Ask](metrics.md#ask-in-plain-language) and prints it to stderr with its explanation and warnings before it runs.
`--dry-run` prints them and runs nothing, and needs `--ask`; giving a query and `--ask` together exits 2.
`--json` prints the response body.
An invalid query exits 1 with the datasource's message, and so does a query that hits a server limit on time, series or points; the message names the limit.
The [Metrics](metrics.md) page runs the same queries in the web app.

## Grafana connections

### Connect to a Grafana

`bin/observe grafana connect <url> --token-stdin` stores a Grafana connection for [Switch from Grafana](switch-from-grafana.md) and tests it, and needs an Admin key.
The token comes only from standard input, so it never lands in shell history, and `--name` defaults to the URL's host:

```sh
printf '%s' "$GRAFANA_TOKEN" | bin/observe grafana connect https://grafana.example.com --token-stdin --name "Ops Grafana"
```

It prints `Connected <name>: <id>` and the test message, and `--json` prints the tested connection.
A failed test exits 1 with the message and the connection id; the connection stays stored, and you fix or remove it on the Grafana settings page.

### Import from a Grafana

`bin/observe grafana import <connection id>` imports dashboards from that Grafana, with `--folders` for some folders or `--all` for every one:

```sh
bin/observe grafana import <connection id>
bin/observe grafana import <connection id> --folders general,platform
bin/observe grafana import <connection id> --all
```

`--folders` takes comma-separated Grafana folder uids, each with its subfolders, and `general` stands for the dashboards outside any folder.
Without either flag it prints the folders as a table of uid, title and dashboard count and exits 2.
It then waits for the import, showing `Imported <done> of <total>` on a terminal, and prints how many dashboards were done, skipped and failed, a table of the failed ones with their error, and a notice for dashboards with panels that did not verify.
It exits 1 when any dashboard failed or the import failed or was canceled, and `--json` prints the final import.
The server keeps a connection's last 20 imports, and an interrupt stops the waiting while the import continues on the server.

## Search

`bin/observe search` finds dashboards, annotations, observations, comments and saved searches by words, ranked by relevance, as the [search](search.md) of the web app does:

```sh
bin/observe search checkout latency
bin/observe search "connection reset" --kind comment --kind observation -- -staging
```

```text
dashboard  Checkout latency  General  https://observe.example.com/d/0b6c...
    Slow checkout requests by pod
```

Every word must match, and the page explains the rest of the syntax: quoted phrases, `or`, `-word` and a prefix on the last word.
Write words that start with `-` after `--`, so the command does not read them as flags.
A line holds the kind, the title, the folder and the address of the page that opens the result, with `no folder` or `no page` when it has none, and the snippet is indented on the line below.
A comment shows the title of what it is on.
`--kind` takes `dashboard`, `annotation`, `observation`, `comment` or `saved_search` and repeats, `--limit` prints 1 to 50 results (20 by default) and `--cursor` continues from the cursor a full page prints.
`--json` prints the response of `GET /api/v1/search` unchanged, and no match prints `No results.`
You find only what the key can read.

## Activity

`bin/observe activity` prints the newest events of the [activity log](activity.md), oldest first, one line each with the time, who did it and what happened, comment threads included:

```sh
bin/observe activity
bin/observe activity --dashboard <dashboard-id> --since 24h
bin/observe activity --follow --type run.failed --type run.canceled
bin/observe activity --actor <person-id> --since 7d
```

```text
2026-11-01T10:00:00Z  Observe Agent for Sam  run failed (no_progress)
2026-11-01T10:05:09Z  Sam  saved version 12 of "Checkout"
```

Observation events read as sentences too, such as `started the observation "Slow checkout"` and `requested a panel block on the observation "Slow checkout"`.
`--dashboard`, `--folder`, `--observation` and `--actor` take ids, `--type` takes an event type and repeats, and `--since` and `--until` take a duration such as `24h` or an RFC 3339 time.
An Admin key also gets the [admin changes](activity.md#admin-changes), who changed a role, a key, a webhook, a connected agent, an outbound domain or a Grafana connection; `bin/observe audit search` reads the [audit trail](../reference/audit-queries.md).
`--limit` prints up to 100 events (20 by default), and `--json` prints the response of `GET /api/v1/activity` unchanged.
You see only the events of the folders the key can see, and an empty log prints `No activity yet`.

`--follow` prints that page and then keeps printing new events until you press Ctrl-C, which exits 0.
It reads `GET /api/v1/activity/stream` from the newest event it printed, so no event is skipped or repeated, and with `--json` it prints one event object per line, the first page included.
When the connection drops, or nothing arrives for 45 seconds (not even the server's 15 second pings), it reconnects after the server's retry delay from the last event it printed, and it exits 1 after 3 failed attempts in a row.
If the server no longer knows that event it prints `Some activity may have been missed.` and continues from the newest event; run `bin/observe activity` again to read what happened in between.

## Audit trail

`bin/observe audit` reads the [audit trail](../reference/audit-queries.md) with an Admin key; every other key gets 403 `forbidden_role`.

```sh
bin/observe audit search --query 'outcome:denied and origin:mcp' --from now-7d
bin/observe audit search --follow --query 'outcome:denied'
bin/observe audit show <event-id>
bin/observe audit facets actor_name --query 'action:dashboard.*'
bin/observe audit histogram --query outcome:failed --buckets 48
bin/observe audit aggregate --query '{"aggs":{"by":{"terms":{"field":"origin"}}}}'
bin/observe audit fields
bin/observe audit export --from now-30d --format csv --file audit.csv
```

`--query` takes KQL, `lucene:` and a Query DSL body, and `--from` and `--to` take an RFC 3339 time, epoch milliseconds or a relative time such as `now-7d`, the last 24 hours by default and at most 366 days.
`search` prints one line per event, newest first, with the time, who, through what, the action, the outcome and the target, 50 a page by default and at most 1,000, and pages with `--cursor` or `--all`.
`show` prints one event and the other events of its request and its agent run, `facets` the most frequent values of a field, `histogram` the counts over time, `aggregate` the rows of a Query DSL `aggs` body and `fields` what a query can name.
`--json` prints the API response unchanged.

`export` streams NDJSON (the default) or CSV to standard output, or to `--file`, and warns on standard error when it stopped at the server's limit of 1,000,000 rows and more events matched.
With `--file` it writes to `<file>.partial` and renames it to the name you gave once the export is complete; if the server or the connection ends the export early, it exits 1, says how many events arrived and leaves them in `<file>.partial`.

`search --follow` reads `GET /api/v1/audit/stream` and prints each matching event recorded from then on, in the order it committed, until Ctrl-C; with `--json` it prints one event object per line.
It cannot be combined with `--all`, `--cursor`, `--limit`, `--from` or `--to`, and exits 1 when the key is no longer an Admin or the server ends the stream because the query is too expensive to follow.
When the connection drops it reconnects from the last event it printed and prints every matching event committed since, down to 10 minutes before that event's time; when that event is more than 24 hours old it exits 1 and asks you to read the gap with `search --from`.

## Webhooks

`bin/observe webhooks` manages the [webhooks](webhooks.md) that send workspace activity to an outside address as signed requests.
Every verb needs an Admin key.

```sh
bin/observe webhooks add --name "Failed runs" --url https://hooks.example.com/slashobserve --event run.failed
bin/observe webhooks list
bin/observe webhooks test <webhook id>
```

- `list` prints each webhook's id, name, address, number of event types and status, which is `sending`, `retrying` or `disabled (<reason>)`.
  A retrying webhook also shows how many events wait behind the failing one and when the next attempt is due, such as `retrying (3 waiting, next 2026-11-11T09:15Z)`; `100+` means more than 100.
- `add --name <name> --url <https address> --event <type>` creates a webhook, and `--event` repeats for several types.
  The address must be `https` on port 443.
  It prints the webhook and its signing secret once, and a notice on standard error says the secret is not shown again, so copy it then; `-q` hides the notice but not the secret.
  An address whose domain is not on the [outbound allowlist](../operate/outbound-access.md) fails with `outbound_domain_not_allowed`.
  `--format slack` or `--format teams` sends one line of text for a Slack or Teams address instead of the signed JSON envelope (`generic`, the default).
- `update <id>` changes the `--name`, `--url`, `--format` or `--event` list you pass and leaves the rest; with none of them it exits 2.
- `enable <id>` and `disable <id>` turn a webhook on, clearing its failures, or off.
- `test <id>` sends a `webhook.test` event now and prints its outcome, the receiver's status, the duration and the error code.
  It exits 1 when the event was not delivered.
- `skip <id>` passes a retrying webhook over the oldest event it is stuck on, so the events behind it go out; the skipped event is never sent to that receiver and shows in `deliveries` as `skipped`.
  It exits 1 with `nothing_to_skip` when the webhook is not retrying, and with `webhook_busy` while a server is delivering for it.
- `rotate-secret <id>` makes a new signing secret and prints it once; the old one stops working at once.
- `delete <id>` deletes the webhook and its recorded attempts.
- `deliveries <id>` prints the newest attempts first as a table of time, event, outcome, status, duration and error code, up to `--limit` of them (1 to 100, 20 by default), with `--cursor` and `--all`; the server keeps attempts for 7 days.

`--json` prints the response body of each request unchanged.
`update`, `enable`, `disable` and `rotate-secret` keep the webhook's owner.
`update --take-ownership` makes the caller the owner, and `enable` does the same for a webhook disabled with the reason `owner`, as the [HTTP API](webhooks.md#manage-webhooks-over-http) does.

## People

`bin/observe people` lists everyone who has signed in and invites people by email with the role they get at their first sign-in, as described in [People](../teams/people.md#invite-people).
Every verb needs an Admin key.
No email is sent: send the printed sign-in link yourself.

```sh
bin/observe people list
bin/observe people invite new@example.com --role viewer
bin/observe people invites
bin/observe people uninvite <invite id>
```

- `list` prints everyone who has signed in, newest first, as a table of id, name, email, role, status (`active` or `deactivated`) and when they were last active, up to `--limit` of them (1 to 500); `--cursor` prints the next page and `--all` every page.
- `invite <email> --role <role>` records a pending invite and prints it with its sign-in link; `--role` is `admin`, `editor`, `commenter` or `viewer`.
  Inviting an email that already has a pending invite changes its role.
  An email that has already signed in fails with `already_member`, and one outside `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` with `email_domain_not_allowed`.
- `invites` prints the pending invites, newest first, as a table of id, email, role, who invited and when, up to `--limit` of them (1 to 500); `--cursor` prints the next page and `--all` every page.
- `uninvite <invite id>` withdraws a pending invite.

Deactivating and reactivating people is done by an Admin in the web app, because a key never ends another person's sessions.

## Teams

`bin/observe teams` manages the [teams](../teams/people.md#teams) whose names share folders with their members.
Every verb needs an Admin key.

```sh
bin/observe teams create sre
bin/observe teams add <team id> <person id>
bin/observe teams list
```

- `list` prints each team's id, name, number of members and their names.
- `create <name>` creates a team with no members; a name another team has, in any case, fails with `team_exists`.
- `add <team id> <person id>` and `remove <team id> <person id>` change the members and print the team; `observe people list` prints the person ids.
- `delete <team id>` deletes the team.

## Agent skill

`bin/observe skill install` writes the `/observe` skill, the guide that outside agents follow to build and change dashboards, investigate a question, search logs and traces, answer comments, keep observations, review proposals and take assigned runs, to `~/.agents/skills/observe/SKILL.md` and `~/.claude/skills/observe/SKILL.md`.
Claude Code and other agents that read Agent Skills then offer it as `/observe`.
It replaces an earlier copy, prints one `Installed <path>` line per file and needs no server, sign-in or network.
`--dir <path>` writes only `<path>/observe/SKILL.md`.

The skill is part of the CLI program and is never fetched from a server, so the text always comes from the release you installed; run `bin/observe skill install` again after you upgrade the CLI.
The same guide reaches MCP clients as the [`observe` prompt](mcp.md#the-observe-prompt).

## Shell completion

`bin/observe completion bash`, `zsh` or `fish` prints a completion script for commands and flags.
Load it for the current shell, or save it where your shell reads completions:

```sh
source <(bin/observe completion bash)
bin/observe completion zsh > "${fpath[1]}/_observe"
bin/observe completion fish > ~/.config/fish/completions/observe.fish
```

## Settings and saved keys

The CLI reads its settings in this order: flags, the environment, then the saved file.

| Setting | Flag       | Environment variable |
| ------- | ---------- | -------------------- |
| Server  | `--server` | `OBSERVE_SERVER`     |
| Key     | `--token`  | `OBSERVE_TOKEN`      |

A saved key is sent only to the server it was saved for.
To use another server, pass `--token` or set `OBSERVE_TOKEN`.
Prefer `OBSERVE_TOKEN` to `--token`, since flags end up in shell history and the process list.

`bin/observe version`, or `--version`, prints the CLI's version.
Add `--json` to any command to get the API response as it is.
`bin/observe logout` removes the saved key and keeps the saved server; the key stays valid on the server until someone revokes it.

## Permission errors

A refusal with the code `forbidden_role` and the message "your role does not allow this" gets a hint that an Admin can change the key's role on the API keys page.

A dashboard the key cannot see answers `not_found`, like a dashboard that does not exist.
A key matches no access list entry, because entries name people and groups, so it sees a restricted folder only as an Admin service account or as the service account that created the folder, as described in [Folders](../teams/folders.md).
