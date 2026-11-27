---
title: CLI
description: Install the observe command line client, sign in with a service-account key, ask the agent for a dashboard, query logs, traces and metrics, and manage a workspace from a terminal or a script.
---

`observe` is the SlashObserve command line client.
Use it to send a prompt to the agent and get back a dashboard address, to query your datasources from a terminal, and to script what you do in the web app.

It calls the server's HTTP API with a service-account key, so it can do what that key's role allows.
Most commands accept `--json`, which prints the API response unchanged, so a script or an agent can read it.
This page starts with your first ten minutes: install, sign in and a first prompt.
Later sections group the commands by what you do with them, and [Reference](#reference) lists every command and flag.

## Install

1. Install the latest `observe` on macOS or Linux (amd64 or arm64) with one line:

   ```sh
   curl -fsSL https://raw.githubusercontent.com/slashobserve/slashobserve/main/install.sh | sh
   ```

   You see `Downloading observe v<version> for <os>/<arch>`, then `Installed <version> to <directory>/observe`.

2. Check that the command runs:

   ```sh
   observe version
   ```

The script needs `curl` or `wget`.
It downloads the archive for your platform from the [releases page](https://github.com/slashobserve/slashobserve/releases), checks its SHA-256 against `checksums.txt`, and refuses to install on a mismatch.
It installs into `/usr/local/bin` when you can write there and into `~/.local/bin` otherwise, and it never uses `sudo`.
It warns on standard error when the directory is not on your `PATH`.

Two environment variables change what it installs.
`OBSERVE_INSTALL_DIR` chooses the directory, and the script creates it if it is missing.
`OBSERVE_VERSION` installs one release, written `v0.1.0` or `0.1.0`:

```sh
curl -fsSL https://raw.githubusercontent.com/slashobserve/slashobserve/main/install.sh | OBSERVE_VERSION=v0.1.0 OBSERVE_INSTALL_DIR="$HOME/bin" sh
```

`OBSERVE_RELEASES_URL` points the script at another release location, which the installer's own tests use.
The script fails when `HOME` is unset, `OBSERVE_INSTALL_DIR` is not set and `/usr/local/bin` is not writable.

### Install by hand or build from source

On Windows, or to install by hand, download the `observe` archive for your platform and `checksums.txt` from the releases page.
The Windows archive is a `.zip` and the others are `.tar.gz`.
Each archive also holds `LICENSE` and `NOTICE`.
Check the archive against the checksums:

```sh
sha256sum --check --ignore-missing checksums.txt
```

On macOS, use `shasum -a 256 --check --ignore-missing checksums.txt`.
Unpack the archive and put `observe` on your `PATH`.

To build it yourself, which needs only Go, run `make cli`; it writes `bin/observe`.
This page writes `observe`; with a binary you built, run `bin/observe`.

## Sign in

You sign in once with a service-account key.
An Admin creates keys on the API keys page of the server; see [Service-account keys](../operate/service-account-keys.md).

1. Run `login` with your server's address:

   ```sh
   observe login --server https://observe.example.com
   ```

2. Paste the key when it asks.
   The terminal does not echo it.
   You see `Signed in to https://observe.example.com as <key name>`.

`login` checks the key with the server and then saves the server and key to `~/.config/slashobserve/cli.json`.
The file is readable by you only.
The location is under `$XDG_CONFIG_HOME` instead when that variable is an absolute path.

To pass the key on standard input, for a script:

```sh
printf '%s' "$OBSERVE_TOKEN" | observe login --server https://observe.example.com --with-token
```

`login` takes the server from `--server`, then `OBSERVE_SERVER`, then the server already saved in `cli.json`.
With none of them it exits 1 and says `no server configured: pass --server, set OBSERVE_SERVER, or run observe login --server URL`.
It does not take `--token`, so a key never lands in your shell history, and it exits 2 if you pass one.
Without `--with-token`, standard input must be a terminal, or `login` exits 2.
A key must be printable ASCII without spaces and at most 4096 bytes.
A wrong key answers `the server did not accept that key`.

The server must be an `https` address, or an `http` address on `localhost`, `127.0.0.1` or `::1`.
Any other `http` address is refused with `refusing plain http to <host>: use https, or a loopback address for local stacks`.
An address with credentials, a query or a fragment in it is refused too.
The CLI drops the default ports `:443` and `:80` and a trailing `/`, and it keeps a path prefix.

`observe logout` removes the saved key and keeps the saved server.
The key stays valid on the server until someone revokes it.

## Send a prompt

A prompt is a sentence that tells the agent what dashboard you want.
The agent runs on the server's model, so each prompt costs model usage.

1. Send the prompt as one quoted argument:

   ```sh
   observe "checkout p99 latency, last 6h"
   ```

2. Watch the build.
   The agent's latest step shows on one status line, and each panel it adds prints a line:

   ```text
   Panel 3 of 8: Checkout p99 (timeseries)
   ```

3. Read the last line.
   On a terminal it is one line with `Saved version N` and the dashboard address:

   ```text
   Saved version 1  https://observe.example.com/d/<dashboard id>/<slug>
   ```

The number after `of` is how many panels the agent planned.
It is left out when the agent gave no estimate or built more than it planned.
A follow-up prints only the panels it adds, and their numbers continue after the panels the dashboard already has.
The address is described in [Addresses in the web app](web.md).

Only the key that made the request can read its build.
So `status --wait` on someone else's request shows the steps without panel lines.

### Output modes

In a terminal, the progress goes to standard output.
The latest step shows on one status line that rewrites in place, the panel lines stay, and the status line clears before the last line.

When standard output is not a terminal, it holds only the address.
That is the case in `url=$(observe "checkout p99 latency")` or a pipe.
The steps, the panel lines and `Saved version N` go to standard error, one per line.

These flags change what prints:

- `-q` hides the steps, the panel lines and `Saved version N`, and prints only the address.
- `--json` prints the finished request object of `GET /api/v1/observe/{id}`, and the steps and panel lines still go to standard error.
- `--open` opens the dashboard address in the browser once it is saved.

`--open` uses `open` on macOS, `xdg-open` on Linux and the URL handler on Windows.
It opens only `http` and `https` addresses.
When the opener is missing or fails, the CLI prints a warning and still exits 0.
`status --wait`, `continue`, `retry`, `run` and `import` accept `--open` too.

Colour shows only in a terminal, and setting `NO_COLOR` to any non-empty value turns it off.
The CLI sends no time zone, so the agent writes the times in its answer in UTC, where the web app uses the zone of your browser.

### Ask a question in a prompt

A prompt that only asks a question ends with an answer that stays in the chat.
The CLI prints it the way `observe investigate` does, followed by the hint `observe save-answer <request-id>`.
`observe save-answer <request-id>` files that answer as an [observation](observations.md) and prints `Saved as an observation: <address>`.
Only the person who asked can do this.
Saving twice prints `Already an observation: <address>`.

When the prompt asked to change one existing dashboard, the answer ends with that dashboard's title and the `observe --revise --dashboard <id> '<prompt>'` command that asks for the change there.

### Quote a prompt that looks like a command

A prompt whose first word is a command name must be quoted as one argument.
The command names are `access`, `activity`, `annotate`, `annotations`, `audit`, `cancel`, `chats`, `comments`, `completion`, `continue`, `dashboards`, `datasources`, `doctor`, `folders`, `grafana`, `health`, `help`, `import`, `investigate`, `login`, `logout`, `logs`, `metrics`, `o`, `observations`, `open`, `people`, `proposals`, `requests`, `retry`, `run`, `save-answer`, `saved-searches`, `search`, `share-answer`, `skill`, `status`, `teams`, `traces`, `version` and `webhooks`.

The CLI also refuses a prompt that is probably a mistyped command, with exit code 2 and before anything is sent.
A run costs money, and this is almost always a typo.
The refusal applies in two cases:

- A prompt of one bare word, such as `observe latency`.
  Add a second word to send it.
- A prompt of several words whose first word is close to a command name, such as `observe dashbords list`.
  The first word needs at least 4 characters, and it is close when it differs from a command name by one letter, or by two when it is longer than 6 characters.
  Quote the words as one argument to send them anyway.

A prompt sent with `--dashboard <dashboard-id>` is never refused, because a command name does not take that flag.
So `observe --dashboard <id> faster` works.

## Check that your setup works

`observe doctor` explains why a setup does not work, one check per row:

```sh
observe doctor
```

```text
CHECK       STATUS  DETAIL
server      ok      https://observe.example.com is ready
key         ok      ci-deploy (service_account)
role        ok      editor
version     ok      CLI 0.2.0, server 0.2.0
datasource  ok      prom (prometheus)
datasource  fail    logs (loki): The datasource is not answering (The server could not connect to the datasource's address)
model       ok      anthropic claude-haiku-5-5, effort low
```

`doctor` exits 1 when any check fails and 0 otherwise, since a warning never fails it.
It takes no flags.
`--json` prints `{"ok": <bool>, "checks": [{"check", "status", "detail"}]}` instead of the table.
Colour marks `ok`, `warn` and `fail` only in a terminal and when `NO_COLOR` is not set.

Details of the checks:

- `server` asks `/readyz` and names the readiness checks that fail.
  When this check is not `ok`, whether the server cannot be reached or answers not ready, every later check is `skip` with no detail.
- `key` reads `/api/v1/me` with the saved or given key.
  With no key it fails with `no key; run observe login`, and with a refused key with `the server refused the key; run observe login`.
  After a failed `key` check, the later checks are `skip` too.
- `role` warns for a Viewer or Commenter key, because those roles cannot create dashboards or annotations.
- `version` warns when the CLI and the server are different releases.
  A local build such as `dev` never warns.
- `datasource` is one row per datasource with its health, and a warning when there are none.
- `model` fails when the server has no model configured, because prompts cannot run.

## Open an address

`observe open` prints an address on the server and opens it in the browser:

```sh
observe open
observe open <dashboard-id>
observe open checkout-overview
```

With no argument it opens the server itself.
A dashboard id opens that dashboard, and so does the uid of a dashboard imported from Grafana, which the server resolves to the imported one.
An `http` or `https` address opens only when its scheme and host are the server's, and another address exits 2 with `<address> is not on <server>`.
More than one argument exits 2, and so does an argument that is not a dashboard id, a Grafana uid or an `http(s)` address.

It needs only the server, not a key.
`--json` prints `{"url": "<address>"}` instead of the address.
Unlike `--open`, it exits 1 when the opener is missing or fails, after printing the address.

## Work with agent requests

Every prompt becomes a request, and every request is a turn of a chat.
This section covers follow-ups, waiting, stopping and finding your earlier requests.
[Chat and follow-ups](chat.md) describes the same work in the web app.

### Send a follow-up to a dashboard

Use `--dashboard <dashboard-id>` to ask about a dashboard you already have:

```sh
observe --dashboard <dashboard-id> "why is the p99 panel flat after 10:00?"
observe --revise --dashboard <dashboard-id> "add a panel for errors by pod"
```

Without `--revise`, `--dashboard` asks about that dashboard privately, as `observe investigate --private` does.
With `--revise`, the prompt is a follow-up that changes that dashboard, as [Chat and follow-ups](chat.md) describes.
`--revise` without `--dashboard` exits 2.

Four more flags shape a follow-up:

- `--apply <finding-id>`, with `--revise` and `--dashboard` and no prompt, revises the dashboard with the suggested change of that answer instead of a prompt you write.
- `--chat <chat-id>` sends the prompt as the next turn of that chat of yours, from `observe chats list`.
  You may combine it with `--dashboard` when the chat is about that dashboard.
- `--new-chat` starts a new chat even when `--revise` would join your latest chat on that dashboard.
  It exits 2 with `--chat`.
- `--at '<anchor>'` points the prompt at one spot on the `--dashboard` dashboard.
  The server refuses `--at` without `--dashboard`.

The anchor is a JSON object, for example `--at '{"kind":"position","pos":{"x":0.4,"y":0.3}}'`, or one of these short forms:

| Short form | Points at |
| --- | --- |
| `row:service=payments,span=authorize_card` | A table row, named by its labels. |
| `time:2026-11-20T23:56Z` | One moment. |
| `range:23:50Z..23:58Z` | A time range. |
| `series:service=checkout` | One series, named by its labels. |
| `pos:0.4,0.3` | A position on the panel, as x and y. |

Every `--at` of the CLI takes these forms, except `annotate --at`, which takes a time.
A time of day such as `23:50Z` means its latest occurrence not after now, and it may carry seconds, as in `23:56:10Z`.
A range given as times of day must end in the past, so `range:10:00Z..09:00Z` asked at noon is refused.
A short form that names no position points at the middle of the panel.
A label value in a short form cannot contain a comma, because the comma separates labels.
The JSON form takes any value.

### Follow-ups and waiting

A prompt waits for the build and prints the address.
These flags change the waiting:

- `--no-wait` prints the request id and returns at once.
- `--timeout` stops the waiting after the time you give, 2 hours by default, so a large build can finish while you wait.
  It stops the waiting and not the request, which keeps running on the server.

To see a request, or to wait for it again, use `status`:

```sh
observe status <request-id>
observe status <request-id> --wait
```

`status` without `--wait` exits 0 even when the request failed.
With `--wait` it exits 1 when the request failed, was canceled or the waiting stopped at `--timeout`.
A failed request prints `observe failed: <reason>` and a canceled one `observe canceled: <reason>`, each followed by the hint line below when the server allows a next move.
With `--json` that error keeps the code `failed` and adds `run_id` and `run_state` (`failed` or `canceled`).
The wait reads the request's `state`, and the older `status` when a server sends a state the CLI does not know.
Ctrl-C during a wait exits 1 with `still running on the server: observe status <id>`, under the code `interrupted`; the request keeps running.
When polling fails 3 times in a row, the CLI exits 1 and says `the request may still be running: observe status <id>`.
An Admin can show anyone's request with `observe status`, read only.

While a request waits for a worker, `status` prints `Queued, position N`, its place among the queued requests.
The order can shift, because a request is skipped while another revision of its dashboard is running.
While a request runs, `status` prints a `Live text` section with what the agent is writing, the same text the Working row shows in the browser.
The text is not kept and is gone once the run ends, and `--json` carries it as `live_text`.

Details of what `observe status` prints.
The fields are `ID`, `Status`, `State`, `Intent`, `Investigation`, `Trigger`, `Cost`, `Dashboard`, `Version`, `URL`, `Error`, `Title` and `Prompt`, then the steps, with these rules:

- `State` reads as in the web app: Queued, Running, Stopping, Done, Failed or Canceled.
  `--json` carries it as `state`, with the server's `queued`, `running`, `canceling`, `succeeded`, `failed` and `canceled`.
  `Status` is the older `queued`, `running`, `succeeded` or `failed`, in which a canceled request reads `failed`.
- `Investigation` is `yes` for a run started as an investigation that writes into an observation and `no` for an ask.
  `--json` carries it as `investigation`, and it never changes once the run is queued.
- `Trigger` says what started the request.
  It is `person` for a prompt, a follow-up or an investigation the requester asked for, and `mention` for a run that a comment asked for, as `--to agent` does.
- `Title` and `Prompt` tell requests apart without opening the browser.
  For a request from someone else's private chat, both read `hidden, asked in a private chat`.
- `Points at` shows the spot a request points at, and `On block` an observation block, which it prints alone for a question about a whole block.
- `About observation <id>` shows for an ask about an observation, also after its answer is saved into another observation.
- `Finding` and `Finding URL` show the finding of an investigation, and `Shared in thread` the thread an answer was shared in.
- `Cost` shows once the request has finished.
  It is the server's estimate in US dollars, such as `about $0.12`, `under $0.01` or `$0.00`, and `-` when the model has no price or no usage was recorded.
- Note lines follow the fields.
  A queued request prints `Queued, position N`, a request that was continued prints `Continued by <request-id>`, and one that was retried prints `Retried by <request-id>`.
- A hint line then says what to do next, taken from the moves the server allows you in `allowed_actions`.
  A stopped request whose build you may save or continue prints `Save it in the browser, or continue it: observe continue <request-id>`, and one you may only continue prints `Continue it: observe continue <request-id>`.
  A failed or canceled request you may only retry prints `Run it again: observe retry <request-id>`.

### Cancel, continue or retry a request

Only the person who made a request can cancel or continue it.
Anyone else gets `only the requester can cancel or continue <request-id> (forbidden)`.

`observe cancel <request-id>` cancels a queued or running request and prints it.
A queued request stops at once, and a running one within a few seconds, once the call it is in returns.
A running request shows the state `canceling` until its worker stops, and one whose worker is gone ends at once.
The request ends `failed` with the code `canceled`, and what it built so far is kept.
Canceling a request that is already canceled or canceling succeeds and prints it, and canceling a finished request fails with a one-line message naming it.

`observe continue <request-id>` starts a new request that takes over the kept build of a stopped one.
It waits the way a prompt does and prints the dashboard address.
It accepts `--no-wait`, `--timeout` and `--open` like a prompt, and `--note <text>` gives the continued run an instruction.
The note follows the same rule as a prompt: 1 to 4000 characters and no control characters, see [Text rules](../reference/text-rules.md).
The server's `400` message is printed as it is.
Continuing a request that was saved, was already continued or has no build fails with a one-line message naming it.
Continuing a revision whose dashboard has a newer version than the build started from carries the build onto that version.

`observe retry <request-id>` starts a finished request again as a new request with the same prompt.
It waits the way a prompt does and prints the result, and it accepts `--no-wait`, `--open` and `--timeout`.
Anyone who can see the request and may start that kind of request can retry it, and the new request is theirs and counts against their limits.
A request that has not finished or was already retried fails with `request <request-id> is <state> and cannot be retried`.
A comment or suggestion request cannot be retried.

Details of how retry counts:

- A failed or canceled request can be retried once by the person or key that asked.
- Anyone else's retry is a request of their own and leaves the original retryable.
- A succeeded request queues a new request on every call.

### Your requests

`observe requests list` prints your agent requests, newest first with pinned ones on top, as a table of id, state, intent, dashboard, the spot it points at and title.
Archived requests are left out.
A request that failed before it had a dashboard shows only here, and `observe status <request-id>` reads it.

The state reads as in the web app: Queued, Running, Stopping, Done, Failed or Canceled.
`--json` keeps the server's `queued`, `running`, `canceling`, `succeeded`, `failed` and `canceled`, which is also what `--state` takes.

These flags narrow the list:

- `--state` repeats, or takes a comma-separated list.
- `--q <word>` matches a word of the title or prompt.
- `--pinned` or `--pinned=false` keeps pinned or unpinned requests.
- `--dashboard <id>` keeps the requests asked about a dashboard.
- `--observation <id>` keeps the requests asked about that observation and the ones that wrote into it.
- `--anchored` keeps the requests you asked about a spot.
  The server refuses it with `--scope all`, even for an Admin.
- `--scope all` lists every principal's requests and needs the Admin role.

The `AT` column shows the spot in words, and a dash when the request points at nothing.
`--json` carries it as `anchor` and `block_id`.
Only the person who asked a request sees its spot, so anyone else reads `null`.
The list pages like every list; see [Lists and paging](#lists-and-paging).

Four commands change one of your own requests:

```sh
observe requests pin <request-id>
observe requests unpin <request-id>
observe requests rename <request-id> "New title"
observe requests archive <request-id>
```

`requests rename` takes a title of 1 to 120 characters on one line, and `--clear` takes the title back to the start of the prompt.
`requests rename` and `o edit --title` also rename the chat or observation that shares the title, as [Chat](chat.md#one-title-for-a-chat-and-its-observation) describes, and say so or say that the other title was left as it is.
You can repeat each command without effect, so a retry after a dropped connection is safe.
An archived request stays readable with `observe status`.
Archiving the last request of a chat archives the chat too.

`requests archive <id> --delete-observation --yes` first deletes the observation the request saved, as `observe o delete` does and with the same permission.
That is Manage on its folder, or Edit when you started the observation.
It archives the request only after that delete succeeds.
The two differ: the request is only archived and still reads with `observe status`, while the observation is deleted for everyone, with its blocks, RCA revisions and discussion, and that cannot be undone.
A request that saved no observation is refused and left as it was.

### Your chats

Every prompt you send is a turn of a chat, which you and Admins can read and nobody else sees.
`observe chats list` prints your chats, pinned ones first and then the one most recently asked in.
The table shows the id, whether it is pinned, the state of the last turn, the number of turns, when it was last asked, its dashboard and its title.

```sh
observe chats list --q checkout
observe chats show <chat-id>
observe chats pin <chat-id>
observe chats archive <chat-id> --yes
```

`chats list` takes `--q` (a word of the title or any prompt), `--pinned` or `--pinned=false`, `--dashboard <id>` and `--observation <id>`.
It pages with `--limit` (at most 50), `--cursor` and `--all`.
`--scope all` lists every person's chats with an `OWNER` column and needs an Admin key.

`chats show <chat-id>` prints the chat and its turns, oldest first.
Each turn has its request id, state and the first line of its prompt.
A `points at` line follows a turn that pointed at a spot, an `is about block` line follows one about a whole block, and `Request <id> is shared in thread <id>` names a shared answer.
`observe status <request-id>` prints one turn in full.
An Admin may show anyone's chat, read only.

Every show is recorded in the audit trail as `chat.view` with the owner's id, and every list as `chat.list` with its scope, each once per reader in 10 minutes.

`chats pin <chat-id>` and `chats unpin <chat-id>` pin a chat of yours to the top of the list and take it back, as Pin and Unpin do on the Chats page.
Pinning a pinned chat changes nothing, and only the chat's owner can do it.
The id must be a chat id from `chats list`.
The id of a later turn or of any other request is refused with `is not one of your chats`.

`chats archive <chat-id> --yes` archives the chat and every request in it, which stay readable with `observe status`.
Dashboards and observations they made are kept.
It needs `--yes` because nothing brings an archived chat back to the lists.
Only the chat's owner can do it, and an Admin who can read the chat gets `chat_read_only`.

## Investigate a question

`observe investigate <question...>` asks the agent a question about your data, as in [Investigate](investigate.md), and needs an Editor key:

```sh
observe investigate why did checkout errors rise after 10:00
observe investigate --dashboard <dashboard-id> --panel <panel-id> --from now-6h --to now what changed
observe investigate --observation <observation-id> why did checkout errors rise
```

It shows the steps like a prompt, then prints the finding's title, its answer as plain text and its address.
Without `--observation`, each investigation opens a new observation that shows its run and receives its conclusion.
`--json` then prints the finished request, which carries the `observation_id` of that observation, or null when no folder allowed one; see [Observations](observations.md#investigations-write-into-an-observation).
With `--observation` the run and the conclusion go into that observation.
It must be one you can see, or the server answers `not_found`, and one you may comment on, or it answers `forbidden_role`.

When the agent filled the structured fields, the command prints them between the title and the answer, such as `Root cause: <component> (<kind>) - <summary>`, `Onset`, `Impact`, `Confidence`, `Ruled out` and `Next steps`, or `No incident found.`.
`observe observations show` prints the same lines for each answer in the observation, each group starting with `Answer of run <run-id>`.
When no field is set, nothing extra is printed.

When the answer has no saved address, the command prints `This answer is kept in the chat only. Save it as an observation: observe save-answer <id>`.
When the answer suggests a change, it prints `Suggested change: ...` and an `observe --revise --dashboard <id> '<change>'` line.
When that change belongs on another dashboard, it first prints `This change is made on the dashboard <title>. Ask there:`.

`investigate` exits 1 when the investigation fails or ends without a finding.
`observe status <request-id>` on an investigation shows the finding's title and address, and with `--wait` prints the finding.

An instance runs at most 10 investigations at once, and one caller at most 3 queued or running.
Beyond that the server answers 429 `agent_busy` or `too_many_requests`.
Private asks do not count.

### Scope an investigation

These flags narrow what the investigation looks at:

- `--dashboard <id>` makes it about one dashboard, and `--panel <id>` about one of its panels.
- `--from` and `--to` set the window, together, as relative times such as `now-6h` or epoch milliseconds.
- `--observation <id>` names an existing observation to write into.
- `--as-of <time>`, for an Admin key, runs the investigation as of a past time, an RFC 3339 time or epoch milliseconds.
  The server requires that time to be in the past, and `--to` to be no later than it.
  So `--from now-1h --to now` is the hour before it; see [Investigate a past incident](investigate.md#investigate-a-past-incident).
- `--at '<anchor>'` points the question at one spot of the dashboard or observation, in the JSON or short forms of [Send a follow-up to a dashboard](#send-a-follow-up-to-a-dashboard).
  The server refuses it without `--dashboard` or `--observation`.
- `--block <block-id>` with `--observation` puts that spot in one of its blocks, or without `--at` points the question at that whole block.
- `--open`, `--no-wait` and `--timeout` work as they do for a prompt.

The command exits 2 for these mistakes:

- `--panel` without `--dashboard`.
- One of `--from` and `--to` alone.
- An `--observation`, `--dashboard` or `--block` that is not an id.
- `--block` without `--observation`.
- An unparsable `--at`.
- `--private` without exactly one of `--dashboard` and `--observation`.
- An empty question, or a `--timeout` of 0 or less.

### Ask privately and share the answer

`--private` asks privately about one of `--dashboard` or `--observation`.
It needs only Comment on it, writes nothing into it and opens no observation.
Only you read the answer, and Admins read it read only; see [Private asks](../concepts/agent.md#private-asks).

`observe share-answer <request-id>` then shows that answer to everyone who can read what it was about.
It opens a comment thread there with your question and the answer, and prints `Shared in thread <id>`.
On an observation it also writes the answer onto the page.
`--thread <thread-id>` adds both to that thread instead.

- Only the person who asked can share, and a run is shared once.
  Sharing again prints `Already shared in thread <id>`.
- `observe status <request-id>` and `observe chats show <chat-id>` then name the thread.
- `observe comments withdraw <comment-id>` takes it back to only you while nobody else has written in that thread.
- The server can refuse with `restricted_answer`, `no_answer` or `not_shareable`, or answer that the run `was not found, or it is not yours`.

## Run a saved prompt

`observe run <name>` runs a saved prompt that someone shared with everyone on the server.
It waits like a prompt does and prints the dashboard address, and it accepts `--no-wait`, `--timeout` and `--open`.
The name ignores case and its words need no quotes.

```sh
observe run
observe run Checkout health
```

`observe run` alone prints a table of the shared prompts by name and who shared them, or `no shared prompts yet; share one from the chat home in the web app`.
An unknown name fails with `no shared prompt named "<name>"; run observe run to list them`.
A key never sees private prompts, so share a prompt from the chat home before you run it from the CLI.

## Work with dashboards

`observe dashboards` reads, changes and files dashboards.
The most common commands are these:

```sh
observe dashboards list
observe dashboards show <dashboard-id>
observe dashboards versions <dashboard-id>
observe dashboards diff <dashboard-id> --from 2 --to 3
observe dashboards restore <dashboard-id> --version 2
```

The lists and the panel table print as columns aligned with spaces.
`observe --help` shows the other commands, and `--json` prints the API response.

### Read a dashboard and its history

`dashboards list` prints dashboards with the columns `ID`, `TITLE`, `VERSION`, `VIEWS`, `UPDATED`, `UPDATED BY` and `FOLDER`.
It shows the newest update first, 50 at a time, and up to 200 with `--limit`.
`--order views` puts the most viewed in the last 30 days first, and `--order name` sorts by title.
When more exist, it prints the total and the `--cursor` value for the next page.
That cursor works only with the same `--order`, and `--all` reads every page.
`--exclude-audit` leaves out the dashboards in the Audit folder.

`dashboards show <dashboard-id>` lists the panels of the latest version, or of the version you give with `--version N`.
It also prints how many views the dashboard had in the last 30 days, and the folder is in the details.
`Managed by` says what writes the dashboard: `starter`, `file`, `apply` or `workspace`, the [`managed_by`](../reference/states.md#dashboard-managed-by) of the API.
A version that does not exist answers `version_not_found` and says what the latest one is.
`--json` adds `view_count` and `window_days`.

Four more commands read around a dashboard:

- `dashboards versions <dashboard-id>` lists its versions with `VERSION`, `PARENT`, `AUTHOR`, `CREATED` and `SUMMARY`.
- `dashboards revisions <dashboard-id> [<revision-id>]` lists the agent requests that changed it, newest first, or shows one in full.
  The columns are `ID`, `STATE`, `INTENT`, `BASE`, `SAVED` and `PROMPT`.
- `dashboards viewers <dashboard-id>` lists, by name, the other people who have had the dashboard open in the last 45 seconds.
  It needs View and prints nothing when nobody is looking.
- `dashboards tags [--exclude-audit]` lists every tag with the number of dashboards that carry it.
  `--exclude-audit` counts only dashboards outside the Audit folder.

`dashboards diff <dashboard-id>` lists the changes between two versions as `OP`, `PATH`, `FROM` and `TO` columns, with values clipped to 60 characters.
`--to` is the latest version by default and `--from` the one before it.
`--from 0` compares with an empty dashboard.
A version that does not exist answers `version_not_found` with the latest version.

### Restore an earlier version

`observe dashboards restore <dashboard-id> --version <n>` saves an earlier version as the newest version.
It prints `Restored version <n> as version <m>`.
`--summary <text>` sets the version's summary.
When a provisioning file or `observe dashboards apply` writes the dashboard, `restore`, `panels`, `proposals merge` and `import --update` also print the server's warning on stderr: [what replaces the edit](../operate/provisioning.md#edits-made-in-the-app).

`restore` reads the latest version for `--parent` when you leave it out, and `--parent <n>` names it yourself.
It answers `version_conflict` when someone saved since, so read the dashboard again and rerun it.

### Change some panels

`observe dashboards panels` saves a new version that adds, replaces, removes or moves only the panels you name.
It runs only their queries and prints `Saved version <n> of <title>`.

```sh
observe dashboards panels <dashboard-id> --summary "Add errors panel" --file patch.json
observe dashboards panels <dashboard-id> --summary "Drop the old panel" --remove <panel-id>
```

`--summary` is required.
`--file` names a JSON patch of up to 4 MiB with the fields `upsert`, `delete`, `layout`, `variables` and `dashboard`, the same fields as the `update_panels` MCP tool.
The CLI rejects unknown fields, and `--remove <panel-id>` repeats.
Like `restore`, it reads the latest version for `--parent` when you omit it, and a change that touches only other panels than a newer version merges onto it.
To duplicate a panel, upsert a copy under a new id with a `gridPos` below the original.

### Move, copy or delete a dashboard

- `observe dashboards move <dashboard-id> --folder <folder-id>` moves a dashboard to another folder without saving a version.
  It needs Edit on the dashboard and on the folder, and a personal folder takes no moves.
- `observe dashboards duplicate <dashboard-id> --title <title> [--folder <folder-id>]` saves the latest version as version 1 of a new dashboard and prints `Duplicated as <id>`.
  The copy goes to `--folder`, by default the dashboard's own folder when you may create there, else your default folder.
  Its queries are not run.
- `observe dashboards delete <dashboard-id> --yes` deletes a dashboard with every version, comment and agent request, for everyone.
  It needs Edit on the dashboard, cannot be undone, and exits 2 without `--yes`.
  While the agent request that is creating the dashboard is still queued or running, it fails with 409 `dashboard_being_created`.

Dashboard pins (stars) belong to a signed-in person and a key cannot set them, so the CLI has no command for them.
Pin dashboards in the web app or over MCP.
`chats pin` and `requests pin` pin a chat or a request of yours, and `o pin` pins an observation to a dashboard as an annotation.

### Keep dashboards as files

`dashboards export` and `dashboards apply` move dashboards to and from a directory; see [Dashboards as code](dashboards-as-code.md).

```sh
observe dashboards export Platform --dir ./dashboards
observe dashboards apply --dir ./dashboards
```

`export <folder-id-or-title> --dir <dir>` writes the latest version of each dashboard in a folder to `<dir>/<folder>/<name>.json`.
`--dir` defaults to the current directory.
A folder title matches without regard to case, and an ambiguous title exits 2.
The folder directory name escapes `%` as `%25` and `/` as `%2F`.

`apply --dir <dir>` saves every `.json` file of a directory back as dashboards.
It never deletes one, and it skips files and directories whose names start with a dot.
A file in a subdirectory goes to the folder of that name, and a file directly in `--dir` goes to General.

For each file, `apply` updates the dashboard it applied before, else adopts the dashboard with the same title in that folder, else makes a new one.
It saves a version only when the file changed.
It exits 1 when any file failed, and exits 2 when `--dir` is missing, is a file or cannot be read.
When the directory holds no `.json` file it prints a warning and exits 0, but it still needs a signed-in client, so with no key it exits 1.

Two commands need no server:

- `observe dashboards schema` prints the JSON Schema every dashboard file must satisfy, the document the `get_spec_guide` MCP tool returns.
- `observe dashboards sample` prints a complete example Spec with variables, several panel types and a layout.
  Save it as a `.json` file and edit it before `apply`.

### Import a Grafana dashboard

`observe import dashboard.json` imports a Grafana dashboard of up to 8 MiB and prints its address.
Use `-` instead of the file name to read standard input.
`--preview` shows the report without saving; [Import Grafana dashboards](import-grafana.md) describes the report.

```sh
observe import dashboard.json --folder Platform
observe import dashboard.json --map 'prometheus=prom'
```

- `--map '<grafana>=<uid>'` picks our datasource for a Grafana one, and repeats.
  A bad value or a repeated key exits 2.
  A datasource with no match answers 422 `datasources_unresolved`, with a table and a `--map` hint.
- `--folder <title>` chooses the folder.
- `--open` opens the result in the browser.
- `-q` hides the report and the `Folder:` line, but the folder note always prints, because it says the folder you asked for was not used.

The report, the `Folder:` line and the folder note go to standard error, so standard output stays the address.
The command exits 1 for an empty file, a file over 8 MiB, or a failed read of standard input, and exits 2 for a file that cannot be read.

A file whose Grafana uid an earlier import already holds, in a dashboard you can see, is not imported twice.
The command exits 1, names that dashboard and its version, and says which flag to pass:

- `--update <dashboard id>` saves the file as the next version of that dashboard.
  It needs Edit and keeps the folder.
- `--as-new` makes another dashboard on purpose.

`--update` with `--as-new` exits 2.
`--preview` shows the same dashboard in an `Already imported as` line.

To import a directory of exports into one folder:

```sh
for f in dashboards/*.json; do observe import "$f" --folder Platform || echo "failed: $f" >&2; done
```

Running the loop again reports each file an earlier import already made as failed, instead of making a second copy.

## Query logs, traces and metrics

These commands run the same queries as the Logs, Traces and Metrics pages.
They are the way for a script or an agent with only a shell to read your data.
Each takes `--datasource <uid>`, and `--json` prints the response body.
[Discovering what a datasource holds](#discovering-what-a-datasource-holds) shows how to find the uid and what to query.

### Logs

`observe logs [<query>] --datasource <uid>` runs a log query on a Loki, ClickHouse, Elasticsearch or OpenSearch datasource.
It prints the lines oldest first, in the language of the datasource: LogQL, SQL, or KQL with the `lucene:` prefix and Query DSL forms of [Logs and traces](logs-and-traces.md#query-languages-on-elasticsearch-and-opensearch).

```sh
observe logs '{app="shop"} |= "error"' --datasource loki --from now-15m
observe logs '{app="shop"}' --datasource loki --tail
```

`--from` and `--to` default to `now-1h` and `now` and take the same times as a dashboard.
`--limit` keeps the newest 1 to 1000 lines, 100 by default.
A search that finds nothing prints `No log lines between <from> and <to>` on standard error.
An invalid query exits 1 with the datasource's message.

Each line shows its time in UTC, its level when the lines have one, the labels whose values differ between the lines, and the message.
The level comes from `severitytext`, `severity`, `level`, `lvl` or `detected_level`, and is `-` when absent.
The message is the first column found among `body`, `message`, `msg`, `line` and `log`, tried in that order and without regard to case.
Without any of them it is the first text column, as in the web app's logs panel.

Details by datasource:

- On ClickHouse the query is SQL that reads the time range with `$__timeFilter(Timestamp)` and selects `Timestamp` and `Body`.
  The rows come back as they are, and a result with no `Timestamp` or message column prints as a table instead of log lines.
- On Elasticsearch and OpenSearch the query is KQL, `lucene:` or a Query DSL body.
  The lines come from the same search the logs page runs, newest first from the server and printed oldest first here.
  `--limit` is at most 500 there, because a search page holds at most 500 lines, and a larger value exits 2.
  `--json` lists the rows oldest first too, the newest `--limit` of them.

#### Follow new lines

`--tail` keeps asking every 2 seconds and prints only the lines it has not printed, with every label, until Ctrl-C, which exits 0.
Each poll asks from the newest line printed so far, so a line indexed late with an older time is not printed.
When a poll returns `--limit` lines, the newer ones may have been skipped, and `--tail` says so on standard error; raise `--limit` or narrow the query.
The command exits after 3 failed polls in a row.
With `--json`, `--tail` prints one JSON object per line with `time`, `level`, `labels` and `line`.

#### Facets, patterns and context

Three flags read the lines differently, as the logs page does, and print the response body with `--json`:

```sh
observe logs '{app="shop"}' --datasource loki --facets
observe logs '{app="shop"}' --datasource loki --patterns
observe logs '{app="shop"}' --datasource loki --context 2026-11-01T10:00:00.123456789Z
```

`--facets` counts the top values of each label or field instead of printing lines.
Each field prints a header `<name> (<kind>, exact|sampled)`, then one `<count>  <value>` row per value.
A Loki index label counts bytes, so its header adds `bytes`.
A sampled field counts the newest lines only.
`--field <name>` counts only the fields you name, and repeats.

`--patterns` groups the lines into patterns and prints a `COUNT` and `PATTERN` table, with every variable part written `<*>`.
`--limit` is the most patterns to print, up to 100.
A notice on standard error, which `-q` hides, says how many lines matched.
It adds the number sampled when the server read the lines itself, and `counted by <datasource type>` when the datasource's own pattern API counted them.

`--context <time>` prints the lines around one line: the lines before it oldest first, a `-- <time> --` line, then the lines after it.
Give the line's time as nanoseconds since the epoch, or as an RFC 3339 time with nanoseconds, so that two lines in the same millisecond stay apart.
`--before` and `--after` set how many lines to read on each side, 0 to 500 and 50 by default.
A note on standard error says when more exist.
The server reads at most 1 hour on each side of the line.

`--facets`, `--patterns`, `--context` and `--tail` cannot be combined.
`--field` needs `--facets`, and `--before` and `--after` need `--context`.
Each mistake exits 2.

#### Ask for logs in plain language

`--ask` takes a question instead of the query, and the server writes the query with [Ask](logs-and-traces.md#ask-in-plain-language):

```sh
observe logs --ask "checkout errors" --datasource loki --from now-6h
observe logs --ask "checkout logs mentioning payment" --datasource clickhouse --dry-run
```

It prints `Query: <query>`, the explanation and one `Warning: <warning>` line per warning on standard error.
Then it runs the query as if you had typed it, `--tail` included.
`--dry-run` prints them and runs nothing, and needs `--ask`.
A question the server cannot turn into a query exits 1 with the reason, and so does a server with no model.

### Traces

`observe traces <trace id> --datasource <uid>` prints the spans of one trace from a Tempo, Jaeger or ClickHouse datasource as a tree:

```sh
observe traces 4bf92f3577b34da6a3ce929d0e0e4736 --datasource tempo
observe traces 4bf92f3577b34da6a3ce929d0e0e4736 --datasource clickhouse --from now-7d
```

The id is 16 or 32 hexadecimal characters.
`--from` and `--to` bound the search and default to `now-24h` and `now`.
On ClickHouse it reads the OpenTelemetry `otel_traces` table.

It prints a header with the span count, the total duration and the root span.
Then one row per span shows its service, its name indented under its parent, its start after the trace start, its duration and `ok`, `error` or `-`.
When a span has a status message, it follows `error:`, such as `error: card declined`.
A trace with no spans in the range exits 1.

#### Search traces in plain language

`--ask` takes a question instead of the trace id and prints 100 matching traces, the newest unless `--sort` says otherwise:

```sh
observe traces --ask "checkout requests slower than 2s with errors" --datasource tempo
observe traces --ask "slow checkout requests" --datasource clickhouse --dry-run
observe traces --ask "checkout requests with errors" --datasource clickhouse --sort -duration
```

It prints the answer on standard error, as `Filters: service=checkout min=2s errors` or `Query: <TraceQL>`, with the explanation and any warnings.
Then it runs the same search as the Traces page.
That is TraceQL on Tempo and `key=value` pairs on Jaeger.
On ClickHouse it is a query over `otel_traces` grouped by trace, so each row is the whole trace with its root, its span count and its full length.

Each row shows the trace's start, service, span, duration, span count and error count, and a column the datasource does not return stays blank.
`--dry-run` prints the answer and searches nothing, and `--json` prints the search's response body.
When the list is incomplete, a `Note:` line on standard error says why and how to get more.
It may say the page is full and tell you to narrow `--from` and `--to` or the question, or that Tempo stopped before it searched everything.
The same sentence is `meta.note` in the response body and `note` in the MCP query tools' result.

`--sort start|service|span|duration|spans|errors` orders the traces, with a minus in front for descending, such as `-duration`.
ClickHouse sorts everything that matches before it takes the 100, and Tempo and Jaeger sort only the 100 they return.

#### Search traces with your own query

`--query` takes a search instead of the trace id, in the language of the datasource, and needs no model:

```sh
observe traces --query 'service=checkout error=true' --datasource jaeger --sort -duration
observe traces --query '{ resource.service.name = "checkout" && status = error }' --datasource tempo
observe traces --query 'SELECT TraceId, ServiceName, Duration FROM otel_traces WHERE $__timeFilter(Timestamp) ORDER BY Duration DESC' --datasource clickhouse
```

On Jaeger it takes the `key=value` pairs of the Traces page: `service`, `operation`, `minDuration`, `maxDuration`, `error` and tags.
On Tempo it takes TraceQL.
Both run as the search of `--ask` does and print the same table of 100 traces.
`--sort` applies to the 100 traces returned.
A `Note:` line on standard error says when the list is incomplete.

On ClickHouse it takes SQL over `otel_traces` and runs it as the SQL view of the Traces page does.
It goes through the same checks as that view and the SQL of `observe logs`.
That means one read-only `SELECT`, the range of `--from` and `--to` read through a macro such as `$__timeFilter(Timestamp)`, and the 15 second limit.
A query that breaks one exits 1 with the server's sentence.

- It prints the first 100 rows as a table with a column per selected column and times in RFC 3339 with nanoseconds.
- A numeric column named `Duration`, in any case, prints as a duration such as `1.20s`, `350.0ms` or `120µs`, not as raw nanoseconds.
- When more rows match, it says so on standard error, and an empty result prints `No rows between <from> and <to>`.
- The rows come back as the SQL wrote them, with its own `ORDER BY`, `LIMIT` and columns even when it selects `Timestamp`.
  A `LIMIT` above 100 is cut to the first 100 rows.
- Single quotes keep the shell from expanding `$__timeFilter`.

`--query` takes no trace id and cannot be combined with `--ask`, `--dry-run` or `--compare`.
`--sort` does not apply to SQL, which has its own `ORDER BY`.
Each mistake exits 2.

#### Compare traces

`--compare` explains what sets some traces apart from others, as the compare view of the Traces page does:

```sh
observe traces --compare 4bf92f3577b34da6a3ce929d0e0e4736,5c1d7a2e9f8b4a3d --baseline 6d2e8b3fa09c4b4e9f7a1c2d3e4f5061 --datasource tempo
```

`--compare` takes the trace ids to explain, up to 50, and `--baseline` the trace ids to compare them with, up to 150.
Each is comma separated or repeated, and a trace id can be in only one of them.
It works on Tempo and ClickHouse datasources only, and a Jaeger datasource answers 400.

It prints `ATTRIBUTE`, `VALUE`, `SELECTED` and `BASELINE` columns.
They show the attribute values whose share of the selected traces and share of the baseline traces differ by at least 10 percentage points, the largest gap first.
`--limit` is the most differences to print, 1 to 100 and 20 by default.
`--from` and `--to` bound where the traces are read.
A note on standard error says when some traces were not found in the range or more differences exist than `--limit` allows.

`--compare` takes no trace id argument and cannot be combined with `--ask`, `--dry-run` or `--query`.
`--baseline` and `--limit` need `--compare`.
Each mistake exits 2.
A trace id in both sets is a server 400 and exits 1.

Two more rules apply to every trace form.
`--sort` needs `--ask` or `--query`, a trace id cannot go with `--ask`, and more than one id exits 2.
The datasource must be Tempo, Jaeger or ClickHouse, or the command exits 2.

### Metrics

`observe metrics query <promql> --datasource <uid>` runs a PromQL query on a Prometheus datasource and prints one line per series:

```sh
observe metrics query 'sum by (service) (rate(http_requests_total[5m]))' --datasource prom --from now-6h
observe metrics query 'up == 0' --datasource prom --instant
observe metrics query --ask "requests per second by service" --datasource prom --dry-run
```

`--from` and `--to` default to `now-1h` and `now` and take the same times as a dashboard.
The output is a table with the columns `SERIES`, `POINTS`, `LAST`, `MIN`, `MAX` and `MEAN`, one row per series.
`--instant` evaluates the query once at the end of the range and prints `SERIES` and `VALUE`.
A query with no series prints `No series between <from> and <to>` on standard error.

`--ask` takes a question instead of the query.
The server writes the PromQL with [Ask](metrics.md#ask-in-plain-language) and prints it to standard error with its explanation and warnings before it runs.
`--dry-run` prints them and runs nothing, and needs `--ask`.
Giving a query and `--ask` together exits 2.

An invalid query exits 1 with the datasource's message.
So does a query that hits a server limit on time, series or points, and the message names the limit.
`observe metrics` alone prints help.
The [Metrics](metrics.md) page runs the same queries in the web app.

### Discovering what a datasource holds

A script or an agent with only a shell finds what to query with `observe datasources`.
`list` prints each datasource's uid, and the uid is what `--datasource` takes:

```sh
observe datasources list
observe datasources metrics prom --search http
observe datasources labels prom
observe datasources label-values prom service
```

The commands answer what the agent's own tools answer, such as `get_schema` and `sample_rows`.
Every command takes `--json`, which prints the server's answer unchanged.

#### List names, labels and values

- `metrics <uid>` lists metric names with `NAME`, `TYPE`, `UNIT`, `TABLE` and `HELP` columns, for a Prometheus datasource or, as OpenTelemetry metric names with their table, a ClickHouse one.
- `labels <uid>` lists label names, and `label-values <uid> <label>` lists one label's values.

All three take `--search` to filter by substring and `--limit` to cap the list at 1 to 500, 100 by default.
When more exist, the command says so and names the two ways to see them.
`labels` and `label-values` also take `--metric` and `--table`, and `--from` with `--to` narrows the window that Loki, Tempo, Elasticsearch and OpenSearch look in.
On ClickHouse, `labels` and `label-values` need `--table` or `--metric`.
`--from` or `--to` alone exits 2.

#### See which services a datasource has

`services [uid]` lists the services a Loki, Tempo, Jaeger, ClickHouse, Elasticsearch, OpenSearch or Prometheus datasource has seen in the last hour.
For each it shows the label, column or attribute that holds them.

- `--window` takes a duration such as 15m, from 1 minute up to 6h, and the default is 1h.
- `--signal` picks `logs`, `traces` or `metrics`.
- `--service checkout` keeps only the sources that hold a service whose name contains the text, without regard to case.
  That answers which source holds a service.

Without a uid it asks every such datasource and prints one row per source and signal.
A source that cannot answer in time says so while the others still answer.
`--json` then prints `{"sources": [...]}`.
The server caches an answer for about a minute and lists at most 200 services per source.

#### Look at the schema and a few rows

`schema <uid>` lists the tables of a ClickHouse, Elasticsearch or OpenSearch datasource, and `--table` lists one table's columns with their types.
A cut list names the `--offset` that reads the next page.
A Tempo or Jaeger datasource has no tables, so `schema` lists its services and span names, and on Tempo its attribute names.

`sample <uid>` prints recent rows or log lines, cut to 300 bytes a cell, so the real values show before a query is written.
`--limit` takes 1 to 20 and the default is 5.
ClickHouse needs `--table`.
On Loki, `--table` is a stream selector, and leaving it out reads every stream.

#### Check a query and count what it matches

- `lint <uid> <query>` checks a query without running it and prints each diagnostic with the byte range it covers.
  It exits 1 only when a diagnostic is an error.
- `log-volume <uid> <query>` counts every line a logs query matches per bucket, by level.
  When the datasource ran out of time, it says from when it counted.
- `trace-stats <uid> [<filter>]` counts spans, errors and the p50, p95 and p99 durations per bucket.
- `trace-logs <uid> <trace-id>` prints the log lines of a Loki, ClickHouse, Elasticsearch or OpenSearch datasource that carry a trace id, or one span with `--span`.
  `--service` narrows the Loki streams, or the Elasticsearch and OpenSearch documents, it reads.
  `--limit` takes 1 to 1000 lines, 100 by default.

`log-volume`, `trace-stats` and `trace-logs` take `--from` and `--to`, `now-1h` and `now` by default.
The two counting commands take `--buckets`, 1 to 200 and 60 by default.

## Find and save searches

### Search the workspace

`observe search` finds dashboards, annotations, observations, comments and saved searches by words, ranked by relevance, as the [search](search.md) of the web app does:

```sh
observe search checkout latency
observe search "connection reset" --kind comment --kind observation -- -staging
```

```text
dashboard  Checkout latency  General  https://observe.example.com/d/0b6c...
    Slow checkout requests by pod
```

Every word must match.
[Search](search.md) explains the rest of the syntax: quoted phrases, `or`, `-word` and a prefix on the last word.
Write words that start with `-` after `--`, so the command does not read them as flags.
You find only what the key can read.

A line holds the kind, the title, the folder and the address of the page that opens the result, with `no folder` or `no page` when it has none.
The snippet is indented on the line below, and a comment shows the title of what it is on.
No match prints `No results.`

- `--kind` takes `dashboard`, `annotation`, `observation`, `comment` or `saved_search`, and repeats.
- `--limit` prints 1 to 50 results, 20 by default.
- `--cursor` continues from the cursor a full page prints, and `--all` reads every page and prints one list.
  `--all` with `--cursor` exits 2.
- `--json` prints the response of `GET /api/v1/search` unchanged.

### Save a search

`observe saved-searches` works with the logs, trace and metrics searches you save from the Logs, Traces and Metrics pages.

```sh
observe saved-searches list --kind logs
observe saved-searches save --kind logs --title "Shop errors" --datasource loki --query '{app="shop"} |= "error"'
```

`list` prints the searches you can view, most recently changed first, with their id, kind, title, folder, datasource and last change.
`--kind logs|traces|metrics` keeps one kind, `--folder <id>` one folder, and `--limit`, `--cursor` and `--all` page through the result.

`save --kind logs|traces|metrics --title <title> --datasource <uid>` files a search in your My Dashboards, or in `--folder <id>`.
The title is 1 to 120 characters.
It prints the address that reopens the search, even with `-q`.
The search's id is printed on standard error as `Saved search <id>`, which `-q` hides, and `--json` prints it too.
`--from` and `--to` default to `now-1h` and `now` and are kept as written.

A service-account key has no My Dashboards, so a search saved with a key lands in the folder General, which everyone who can see General can read.
Pass `--folder` to file it elsewhere.
For a service-account key, the server also lints the query.
A query with a lint error is refused with `query_invalid` and nothing is saved.
It also finds a search you already saved with the same title, datasource, query, filters, range and folder.
Then it prints `Search <id> was already saved` and its address instead of a second copy, and `--json` shows `replayed: true`.

What a search needs depends on its kind:

- A logs search needs `--query`, runs on a Loki, ClickHouse, Elasticsearch or OpenSearch datasource, and may set `--columns a,b`.
- A trace search may set `--service`, `--span`, `--min`, `--max` and `--errors`.
  It may also set `--query` on a Tempo datasource (TraceQL) or a ClickHouse datasource (SQL, which reopens the search in the SQL view).
  The help text of the command names Tempo and ClickHouse for traces, and the code also accepts a Jaeger datasource with `key=value` pairs such as `service=checkout minDuration=1s`.
- A metrics search needs `--query` on a Prometheus datasource and may set `--mode graph|table`.

`--min` and `--max` are span durations: a trace matches when one span that also fits the service and span name is that long.
A flag of another kind is refused with exit code 2 before any request is sent.

### Change a saved search

```sh
observe saved-searches update <id> --query '{app="shop"} |= "timeout"'
observe saved-searches rename <id> New title
observe saved-searches move <id> --folder <folder-id>
observe saved-searches delete <id>
```

`update <id>` changes what a saved search runs and keeps its kind, datasource, title and folder.
`--query`, `--from` and `--to` replace one value each.
Any filter flag (`--columns`, `--service`, `--span`, `--min`, `--max`, `--errors`, `--mode`) replaces all the filters, so repeat the ones to keep.
`--clear-filters` removes them all, and `--query ""` removes the query of a trace search.
Only the flags you give change, and a call with none exits 2.
The result must pass the checks a save does, so a trace filter on a logs search is refused with the server's message and exit 1.

`rename <id> <title...>` takes everything after the id as the new title, so flags such as `--json` or `-q` must come before the id.
`move <id> --folder <folder-id>` files the search in another folder and shares it with that folder's readers.
My Dashboards takes no moves, so save the search there instead.
`delete <id>` removes it.

Update, rename, move and delete need Edit on the saved search, and a move also needs Edit on the folder.
The text output names what was done, such as `Updated saved search <id>`, and `-q` hides it.
`--json` prints the response of the matching `/api/v1/saved-searches` route unchanged, and nothing for `delete`.
The kind and datasource of a saved search never change, so save a new one instead.

## Write up and discuss what you find

### Observations

`observe o` works with observations, the pages of Markdown, panel and snapshot blocks that people and agents build about one question.
The group is named `o`, with `observations` as an alias, so a prompt that starts with the single word `o` or `observations` must be quoted.
Every subcommand takes `--json`, which prints the response of its API call unchanged, so `o discard --json` prints an empty line.
The commands need the folder access the web app needs for the same work, and a refusal prints the server's message and exits 1.

```sh
observe o new "Slow checkout" --text "Started after the 10:00 deploy."
observe o list --state open
observe o show <observation-id>
observe o add <observation-id> --text "p99 doubled on checkout-1."
observe o resolve <observation-id> --summary "Rolled back the deploy."
```

#### Start and read an observation

`observe o new <title...>` starts an observation and prints `Created <title> <address>`, even with `-q`.
It files the observation in `--folder <id>`, by default your My Dashboards.
`--from` and `--to` are RFC 3339 times that go together, and they set the range its panels show.
`--text <markdown>` or `--text-file <path>`, never both, also adds a first text block.

`o list` prints the observations you can see, most recently changed first, with their id, title, state and last change.
`--folder <id>`, `--state open|resolved` and `--title <text>` narrow it, and `--title` matches a title that contains the text, without regard to case.
`--limit` takes 1 to 100 and the default is 50, and `--cursor` and `--all` page through the result.

`o show <observation-id>` prints the title, state, summary and address.
Then it prints one line for each block with its position, id, kind, state, version, width, height (`auto` or pixels), range and, for a text block, its first line, or for a panel block its title.
The range of a panel block is its own window as `from..to` (RFC 3339, to the minute), or `page` when it follows the observation's range, and the other blocks show `-`.
A block that someone who can only comment added shows the state `requested`.

#### Add and change blocks

`o add <observation-id>` adds one block and prints `Added <kind> block <id> (live)`, or `(requested)` when you can only comment.
Give exactly one of these:

- `--text <markdown>` or `--text-file <path>` adds a text block.
- `--panel-file <path>` adds a panel block from a JSON file with one panel whose query must return data.
- `--snapshot-of <block-id>` freezes the data of a panel block.

These flags shape the block:

- `--from` and `--to` give a panel its own range.
- `--width third|half|two_thirds|full`, or `--half`, sets the width.
- `--height <pixels>` from 120 to 960 sets the height of a panel or snapshot block.
- `--position <n>` places it instead of appending it.
- `--var name=value` sets a variable the panel's queries use, repeated for more values of the same name.
  Without one the block is refused with `variable_missing` and the name of the variable.

The server runs the check the MCP `add_block` tool runs.
A panel whose query is invalid, or returns nothing in its range, is refused with the failing paths below the error, exits 1 and adds no block.
`o edit-block --panel-file` is checked the same way.

`o edit-block <block-id> --version <n>` changes a live block and prints `Changed <kind> block <id>`.
It needs the block's current version from `o show`, and a block that changed since answers `version_conflict`, so read it again and retry.
It takes these flags, and it needs at least one:

- `--text` or `--text-file` for a text block.
- `--panel-file` for a panel block, with `--from` and `--to` for its own range.
  `--text` and `--panel-file` cannot be combined, and `--from` and `--to` need `--panel-file`.
- `--width third|half|two_thirds|full`, `--half` or `--full` for the width, and only one of them.
- `--height <pixels>` for a panel or snapshot block only, 120 to 960, or 0 for the automatic height.
- `--position <n>`, which moves the blocks in between and exits 2 below 0.
- `--follow-page`, which drops a panel block's own range so it shows the observation's range again.
  It cannot go with `--text`, `--text-file` or `--panel-file`.

A snapshot block's body cannot change.
A requested block is kept or discarded instead.

`o arrange <observation-id> <block-id>[:<width>[:<height>]]...` sets the order of the blocks you name and prints `Arranged <n> block(s)`.
After each id you may add a width and, for a panel or snapshot block, a height in pixels or `auto`.
An empty width keeps the width, as in `id::auto`.
A block you leave out keeps its place after the block it followed.
An id that is not a block of the observation is refused by name, and a bad height or more than three `:` parts exits 2.
It takes no `--version`, because a layout change never conflicts with a content edit.

`o remove-block <block-id> --version <n>` deletes a live block and prints `Removed block <id>`.
The blocks after it close the gap.
It needs the block's current version from `o show`, like `edit-block`.

`o keep <block-id> --version <n>` makes a requested block live, and it needs the block's version from `o show`.
`o discard <block-id>` deletes a requested block.
Keeping needs Edit on the observation's folder, and discarding is open to an editor and to whoever asked for the block.

#### Resolve, reopen and change an observation

- `o resolve <observation-id> --summary <line>` closes an observation and prints `Resolved <title>`.
- `o reopen <observation-id>` reopens a resolved observation and prints `Reopened <title>`.
- `o edit <observation-id>` changes the title with `--title <title>`, and the range its panels show with `--from <time> --to <time>`, RFC 3339 times that go together.
  `--clear-range` removes the range.
  It prints `Updated <title>`, and the chat the observation came from may be named after it.
  The message says either that it renamed that chat or that the chat keeps its title because only the person who started it may rename it.
  Giving nothing, one of `--from` and `--to` alone, or a range with `--clear-range` exits 2.
- `o move <observation-id> --folder <folder-id>` files the observation in another folder and prints `Moved <title> to folder <id>`.
  It needs Edit on the observation and on the folder, and My Dashboards takes no moves.

#### Pin, publish and turn an observation into a dashboard

`o pin <observation-id>` pins an observation to a dashboard as an annotation and prints `Pinned to dashboard <id> as annotation <id>`.
It pins to `--dashboard <id>`, or by default the dashboard its run was about.
Pinning it again to the same dashboard prints the same annotation.
An observation with no dashboard to pin to exits 1 with `observation_has_no_dashboard`.

`o dashboard <observation-id>` makes a dashboard in My Dashboards from its live panel blocks, or from one block with `--block <block-id>`.
It prints `Dashboard <id>: <title>`.
It re-runs every query first, and an observation with no live panel block answers `observation_has_no_panels`.
Running it again prints the dashboard you made first, and `--new` makes another.

`o publish <observation-id>` publishes the observation as the next RCA revision and prints `Published revision <n> of <title>`, as in [Observations](observations.md#publish-as-rca).
It needs Edit on the observation's folder.
An observation in My Dashboards is private and cannot be published until you move it to a shared folder, and publishing never creates a public link.

`o rcas <observation-id>` lists the published revisions, newest first, as `REVISION`, `TITLE`, `STATE`, `PUBLISHED BY` and `PUBLISHED` columns.
`o rcas <observation-id> <revision>` prints one revision: its title, its summary, each text block's Markdown and a line `snapshot <id>` for each snapshot block.

#### Delete an observation or a revision

Two commands delete for good.
No MCP tool deletes anything, so an agent cannot do either.

- `o delete <observation-id> --yes` deletes the observation for everyone, with its blocks, snapshots, RCA revisions, the public links of those revisions and its discussion.
  It prints `Deleted observation <id>`.
  It needs Manage on the observation's folder, or Edit on it when you started the observation.
  It cannot be undone, so without `--yes` it exits 2.
  While an investigation on the observation is queued or running, it answers 409 `run_in_progress`.
- `o withdraw <observation-id> <revision> --yes` deletes one published revision and prints `Withdrew revision <n> of <observation-id>`, as in [Observations](observations.md#withdraw-a-revision).
  It needs Edit on the observation's folder, answers `rca_shared` while the revision has a live public link, and cannot be undone.
  Without `--yes` it exits 2.

### Comments

You can work the comments on a dashboard, an annotation or a proposal from the terminal, and so the discussion of an observation.
A target is written `<kind>:<id>`, with kind `dashboard`, `annotation`, `observation` or `proposal`.
An example is `dashboard:0a1b2c3d-0000-4000-8000-000000000001`.

```sh
observe comments list <dashboard-id>
observe comments add --on dashboard:<dashboard-id> --panel <panel-id> "The p99 line jumps at 10:00"
observe comments reply <comment-id> "Rolled back at 10:20"
observe comments resolve <comment-id>
```

Add `--json` to any of these to print the API response unchanged.
Add `-q` to hide the confirmation of `edit`, `delete`, `withdraw`, `resolve` and `reopen`.
`add` and `reply` print their result even with `-q`.

#### List comments

`comments list` lists the comments of open threads, replies included, oldest first.
`comments list <dashboard-id>` and `comments list --on <kind>:<id>` list the same way, and a dashboard id gives what `--on dashboard:<id>` gives.
Exactly one of the dashboard id and `--on` is required, and a single argument that contains a colon counts as `--on`.

- `--status resolved` shows resolved threads instead, and `--status all` shows both.
  The default is `open`.
- `--panel <panel-id>` narrows the list to one panel.
- `--limit` takes up to 500 and the default is 50, and `--cursor` and `--all` page through the result.

The table has the columns `ID`, `THREAD`, `PANEL`, `STATUS`, `AUTHOR`, `CREATED` and `COMMENT`.
`THREAD` shows `-` for a thread's first comment, and `PANEL` shows the target kind, or `<panel> (removed)`.
`AUTHOR` shows the actor label, such as `Observe Agent for Ada` or `Ada (via Claude Code)`.
`COMMENT` is cut at the first line and 80 characters, and for a run's reply it shows the answer's title after `Answered.`

`--json` prints the server's page of `threads` and `messages`, with `next_cursor` and `can_comment`.
A script reads the messages with `jq '.messages[]'`, which replaces `.comments[]`:

- A thread carries its `anchor`, the spot it points at in the shape `--at` takes as JSON, and its `status`, `panel_id`, `panel_present` and `resolved_by_version`, which moved there from the comment.
- A message's `thread_id` names its thread in `threads` and replaces `parent_id`.
  A reply is a message whose `thread_id` is not its own `id`.
- A message's `actor.label` replaces `author.name` and is the text the AUTHOR column shows.
- A run's reply carries its `answer` when you can read it.

#### Start a thread

`comments add --on <kind>:<id> <text...>` starts a thread and prints `Started thread <id> on <kind>:<id>`, even with `-q`.
Everything after the flags is the comment text.
So `--on`, `--panel`, `--block`, `--at`, `--version`, `--to`, `--json` and `-q` must come before it.

- On a dashboard, `--panel <panel-id>` anchors the thread to a panel, and `--version N` to a version, the latest by default.
  The server refuses both on an observation, an annotation or a proposal.
- On an observation, `--block <block-id>` anchors the comment to one of its blocks.
  Without it the comment is about the whole observation.
  The server refuses `--block` on any other target.
- `--at '<anchor>'` points the thread at one spot, in the JSON or short forms of [Send a follow-up to a dashboard](#send-a-follow-up-to-a-dashboard).
  An example is `--at time:2026-11-20T23:56Z`.
  On an observation it points at a spot in the `--block` block.
- `--to agent` or `--to everyone` sets who the comment is for.
  The default is `everyone`, and anything else exits 2.
  See [Ask an agent from a comment](#ask-an-agent-from-a-comment).

A comment is for everyone and asks no agent unless you add `--to agent`.

#### Reply, edit and delete

- `comments reply <comment-id> <text...>` adds a reply to a thread, and the id can be any comment of the thread.
  It prints `Replied to thread <id> as <id>`, even with `-q`.
  - `comments reply <dashboard-id> <comment-id> <text...>` does the same on a dashboard through the dashboard comment endpoint.
    When the first two arguments are both ids, the first is the dashboard id.
  - Everything after the ids is the reply text, so flags such as `--json`, `-q`, `--block`, `--reply-to` or `--to` must come before the ids.
  - `--block <block-id>` anchors the reply to one block of an observation, and `--reply-to <comment-id>` names the comment of the same thread that the reply answers, which the web app shows quoted.
  - `--to`, `--block` and `--reply-to` work only with the comment id alone, because the dashboard comment endpoint takes none of them.
- `comments edit <comment-id> <text...>` replaces the text of your own comment, while you can still comment on its target.
  Everyone else gets 403 `not_author`.
  It prints `Edited comment <id>`, and the flags go before the id, as for `add`.
- `comments delete <comment-id>` deletes one comment and prints `Deleted comment <id>`.
  - Its author may delete it while they can still comment, unless an agent run wrote it.
  - Anyone who manages the target's folder may delete any comment, including an agent's reply.
  - Deleting the first comment of a thread deletes the whole thread.
    Its author may do that only while nobody else has written in it, and everyone else gets 403 `not_author`.

#### Resolve and reopen threads

`comments resolve <comment-id>` resolves a thread, and `comments resolve <dashboard-id> <comment-id>` does the same through the dashboard endpoint.
It prints `Resolved thread <id>`, or `Resolved thread <id> by version N` with `--version N`, which records the version that fixed a dashboard thread.
A Commenter gets `not_starter` on a thread someone else started, and an Editor resolves any.
When the thread was already resolved, `--version` is not applied and the command exits 1.

`comments reopen <comment-id>` reopens a resolved thread and prints `Reopened thread <id>`.

#### Mention people and agents

Mention a person in the text as `@[Name](user:<id>)`, and the built-in agent as `@observe`.
Mention one of your own connected agents as `@[Name](grant:<id>)`, and a service account that takes runs as `@[Name](service_account:<id>)`.
A comment may hold at most 20 mentions.
A mention shows in the [activity log](activity.md) and asks no agent unless you add `--to agent`.

`comments people --on <kind>:<id>` lists up to 20 people, other than you, who can see the target, with the mention to paste into a comment.
You need Comment on the target, or the server answers 403.
`--search <text>` keeps those whose name or email contains the text, and `--limit` takes up to 20.
A notice says when more people match.
A person who cannot see the target is not listed, and mentioning one is refused with `invalid_mention`.

#### Take back an answer you shared

`comments withdraw <comment-id>` takes an answer you shared with `share-answer` back to only you, and prints `Withdrew the answer shared in thread <id>`.
It takes the id of the thread or of any of its comments.
It deletes the thread when the share opened it, and otherwise only the shared question and its answer.

- It works only for the person who asked, while every comment in the thread is theirs or their run's reply.
  When someone else wrote in it, the server answers 409 `others_wrote`.
  When nothing of yours is shared, it answers 404.
- On an observation, the answer's text that was written onto the page stays there.
  The run and its answer leave the observation's lists, and you can share the answer again later.

#### Apply an answer's suggested change

`comments apply <comment-id> <finding-id>` makes the change that an answer in a dashboard thread suggests.
The finding id is the answer's, from `observe status <request-id> --json`, and the answer must have been given in that thread.
Another kind of thread answers 400 `invalid_thread`, and a finding that is not in that thread answers `not_found`.

It posts `Revise the dashboard: <suggested change>` in the thread as you, and prints `Asked to revise the dashboard in thread <id>`.
Then it starts a run, yours and taken by the agent that answered.
The run saves a new version when you can edit the dashboard, and opens a proposal when you can only comment.

Applying the same answer again while that run is queued or running, or after it succeeded, starts nothing and prints `Already applied by run <id>`.
An answer without a suggested change is refused with `no_suggested_change`.

### Ask an agent from a comment

`--to agent` on `comments add` or `comments reply` asks an agent in the same transaction as the comment.
It picks the one agent the text mentions, else the thread's agent, else `@observe`, and that agent becomes the thread's agent.
Without `--to agent`, the comment is for everyone, asks no agent and turns the thread's agent off.
Then the web app's plain replies stop asking it until someone mentions an agent again.

The agent's token is removed from the text to make the run's prompt.
The earlier messages of the thread, the last 20 and at most 8000 characters, go with it as context.
An earlier answer of the agent appears as its finding, its title and text.
What the run may do depends on the thread:

- A dashboard thread starts a read-only ask, which a Commenter may start and which answers in the thread without changing the dashboard.
- An observation thread starts a read-only ask, which needs Comment on the observation and answers in the thread without writing on the page.
- An annotation or a proposal thread starts none.

Mentioning more than one agent is refused with `too_many_agents` and nothing is written.
You can mention only people who can see the target, your own connected agents and service accounts that take runs.
Anything else is refused with `invalid_mention`, and a service-account key can never mention a connected agent.

An observation has one discussion.
So `comments add` on an observation that has one adds the comment to that discussion, unless `--at` points it at a spot of its own.

When the comment is stored but the run cannot start, the command succeeds and prints `No run started (<code>): <message>` on standard error, which `-q` does not hide.
The code is one of these:

- `mention_target_unsupported`.
- `agent_unavailable`, for `@observe` on a server with no model provider, or a thread agent that was disconnected.
- `agent_not_yours`, when the thread's agent is someone else's connected agent.
- `forbidden_role`.
- `too_many_runs`.

When the run starts, the command prints `Started run <id>, taken by <builtin|grant|service_account>`.

#### What the reply says

When the run ends, it posts one reply in the same thread.
It never resolves the thread or reopens a resolved one.
The reply reads `Observe Agent for <your name>` when the built-in agent took the run, `<your name> (via <client>)` when one of your connected agents did, and the service account's own name when a service account did.

The reply is a status line the server writes, not the model's text.
It is one of these:

- `Saved version N of this dashboard.`
- `Opened a proposal: <title>.`, or `Opened a proposal.` when it has no title.
- `Answered.` when the run saved an answer.
  The thread carries the answer with the reply, so everyone who can read the thread reads it, and `comments list` prints the answer's title after the line.
- `The run finished.` when it succeeded with no version, proposal or answer.
- `No agent picked this up within 10 minutes.`
- `The run failed: <reason>.`
- `The run was stopped before it finished.`
- A line that says the answer read data only Admins can query, such as the audit trail, so it is not shown in the thread.
  Only whoever asked and Admins read it, in the run.

The run's own page has the detail.

A retry or a continuation of the run answers in the same thread and goes to the same agent.
It fails with `assignee_unavailable` and starts nothing when that connected agent was revoked, has expired or is not yours, or when the service account no longer takes runs.
Someone who cannot comment on the observation of an observation thread, and retries an answer from it that was saved as an observation, gets a private ask of their own about the saved observation.
Nothing is posted in the thread.

### Proposals

A proposal is a change that someone who cannot edit a dashboard, or whose connection is in propose mode, saved for a person who can edit to merge; see [Proposals](../concepts/proposals.md).

```sh
observe proposals list <dashboard-id>
observe proposals show <proposal-id>
observe proposals merge <proposal-id> --revision 2
observe proposals discard <proposal-id> --reason "Superseded"
```

`proposals list <dashboard-id>` prints the open proposals of a dashboard, newest first.
The columns are `ID`, `STATE`, `REVISION`, `BEHIND` and `TITLE`, and `BEHIND` reads `behind main` when the dashboard has a newer version than the one the proposal builds on.
`--state merged`, `--state discarded` or `--state all` shows the closed ones, and `--limit`, `--cursor` and `--all` page through the result.
The default is 50 and the most is 100.

`proposals show <proposal-id>` prints the proposal's details, then `Changes`, one line for each panel, variable or setting it adds, changes, removes or moves, such as `added panel p4`.
It then prints `Conflicts` with the units a merge onto the latest version would hit, and the actions you may take now.

`proposals merge <proposal-id> --revision <n>` merges the proposal as the next version of its dashboard and prints `Merged as version <n>`.
It prints `Already in the dashboard` when the dashboard already held its change.
`--revision` is required and is the revision `show` printed, and a proposal that gained a newer one is refused with `proposal_changed`.
Merging needs Edit on the dashboard and a connection that is not in propose mode, which otherwise gets 403 `propose_mode`.
It runs the queries of the panels it changes like any save.

For each conflict, `--take <kind>:<id>=ours|theirs` names the side that wins, and it repeats:

```sh
observe proposals merge <proposal-id> --revision 2 --take panel:p1=theirs --take setting:title=ours
```

`ours` keeps the dashboard's unit and `theirs` takes the proposal's.
The kind is `panel`, `variable` or `setting`, and anything else exits 2.
When a `--take` keeps the dashboard's side, the command also prints which units the proposal's change was dropped for.
If nothing was saved, `Merged without a new version` replaces `Already in the dashboard`.

`proposals discard <proposal-id> [--reason <text>]` closes the proposal without saving it and prints `Discarded`.
`--reason` takes up to 500 characters, and it is shown on the proposal and printed by `proposals show`.
Its author may discard it, and so may anyone who may merge it.

`-q` hides the confirmation of `merge` and `discard`, and `--json` prints the API response unchanged.

### Annotations

`observe annotate <text...>` adds an [annotation](annotations.md), such as a deploy marker, at the server's current time on every dashboard:

```sh
observe annotate "Deploy checkout v1.4.2" --tags deploy,checkout
observe annotate "Database failover" --dashboard <dashboard-id> --at now-15m
```

It prints `Added annotation <id> at <time> on every dashboard`, which `-q` hides.
A global annotation needs a key with the Editor role.

- `--tags` takes comma-separated tags.
- `--dashboard` puts the annotation on one dashboard instead of every dashboard.
  It prints `Added annotation <id> at <time> on dashboard <id>` and needs Edit on that dashboard.
- `--at` places it in the past, as `now`, as `now-<N>` with `s`, `m`, `h` or `d` and one to six digits, or as an RFC 3339 time such as `2026-11-01T10:00:00Z`.
  It takes no short form like `time:`.

An `--at` more than a day in the future still saves the annotation and prints a warning on standard error, because a wrong year is the usual cause.

`observe annotations list` prints annotations newest first as `ID`, `TIME`, `DASHBOARD`, `TAGS` and `TEXT` columns, with the text cut at 60 characters:

```sh
observe annotations list --dashboard <dashboard-id> --from 2026-11-01T00:00:00Z --tag deploy
observe annotations update <annotation-id> --text "Deploy checkout v1.4.3" --tags deploy
observe annotations delete <annotation-id>
```

Without `--dashboard` it lists only the global annotations, and with it that dashboard's annotations and the global ones.
`--from` and `--to` are RFC 3339 times, and `--tag` repeats to match any of several tags.
`--limit` takes up to 500 and the default is 50, and `--cursor` and `--all` page through the result.

`annotations update <annotation-id>` changes the text with `--text`, the tags with `--tags`, or both, and prints `Updated annotation <id>`.
`--tags` is a comma-separated list that replaces the old tags, and `--tags ""` removes them.
Giving neither flag exits 2.
The annotation keeps its time, dashboard and author.

`annotations delete <annotation-id>` prints `Deleted annotation <id>`.
Only an annotation's creator or an Admin may update or delete it.

## Watch what changes and what is wrong

### Activity

`observe activity` prints the newest events of the [activity log](activity.md), oldest first, one line each with the time, who did it and what happened.
Comment threads are included:

```sh
observe activity
observe activity --dashboard <dashboard-id> --since 24h
observe activity --follow --type run.failed --type run.canceled
observe activity --actor <person-id> --since 168h
```

```text
2026-11-01T10:00:00Z  Observe Agent for Sam  run failed (no_progress)
2026-11-01T10:05:09Z  Sam  saved version 12 of "Checkout"
```

Observation events read as sentences too, such as `started the observation "Slow checkout"` and `requested a panel block on the observation "Slow checkout"`.
You see only the events of the folders the key can see, and an empty log prints `No activity yet`.

- `--dashboard`, `--folder`, `--observation` and `--actor` take ids.
- `--type` takes an event type and repeats.
- `--since` and `--until` take a Go duration such as `24h`, or an RFC 3339 time.
  A duration counts hours at most, so a week is `168h`, and `7d` exits 2.
- `--limit` prints up to 100 events, 20 by default, and `--cursor` and `--all` page through the result.
  When a full page prints, standard error says `Showing N, and more exist. More: --cursor <cursor>, or --all for every page.`
- `--json` prints the response of `GET /api/v1/activity` unchanged, except with `--all`, which merges the pages.

An Admin key also gets the [admin changes](activity.md#admin-changes): who changed a role, a key, a webhook, a connected agent, an outbound domain or a Grafana connection.
The code also names people invites and deactivations, service accounts and their keys, webhook secret rotations, datasources, and access requests.
`observe audit search` reads the [audit trail](../reference/audit-queries.md).

#### Follow events as they happen

`--follow` prints that page and then keeps printing new events until you press Ctrl-C, which exits 0.
It reads `GET /api/v1/activity/stream` from the newest event it printed, so no event is skipped or repeated.
With `--json` it prints one event object per line, the first page included.
`--follow` cannot be combined with `--all` or `--cursor`, and either exits 2.

When the connection drops, or nothing arrives for 45 seconds, not even the server's 15 second pings, it reconnects.
It waits the server's retry delay and resumes from the last event it printed.
It exits 1 after 3 failed attempts in a row.
If the server no longer knows that event, it prints `Some activity may have been missed.` and continues from the newest event.
Run `observe activity` again to read what happened in between.

### Dashboard health

`observe health` lists the dashboards with [health problems](dashboard-health.md) in the folders the key can see, one row per problem:

```sh
observe health
observe health --kind no_data --folder <folder-id>
observe health repair <dashboard-id> <dashboard-id>
observe health run
```

```text
ID                                    DASHBOARD  FOLDER  STATE    KIND             PANEL OR VARIABLE  CHECKED            REPAIR
0a1b2c3d-0000-4000-8000-000000000001  Checkout   Shop    failing  no_data          Requests (p_rps)   2026-11-06T03:00Z  -
0a1b2c3d-0000-4000-8000-000000000001  Checkout   Shop    failing  variable_failed  $pod               2026-11-06T03:00Z  -
Last check finished 2026-11-06T03:04Z: succeeded.
```

- `--kind` takes `no_data`, `query_failed`, `variable_failed`, `duplicate_title` or `unopened`.
- `--folder` takes a folder id.
- `--limit` lists up to 100 dashboards, 50 by default, and `--cursor` and `--all` read the rest.

`STATE` is the dashboard's health state and `REPAIR` the outcome of its latest repair, both described in [Dashboard health](dashboard-health.md#from-the-cli).
A variable shows as `$<name>`, and the last line says how the last daily check went.
A dashboard listed with no problem shows one row with `-` as its kind and target.
When a listed dashboard is `outdated`, a line after the table says its problems belong to an earlier version and a recheck is pending.
When no dashboard has a problem, it prints `No dashboard has a health problem.`, then the line about the last check.
That line can also read `A health check is running now.`, `Last check started <time> and did not finish.` or `No health check has run yet.`
Listing exits 0 whatever it finds, and `--json` prints the response of `GET /api/v1/health`.

#### Repair dashboards and run a check

`health repair <dashboard-id>...` asks the agent to repair 1 to 50 dashboards.
It prints one row per dashboard with `queued` and the request id, or the reason it was skipped.
The reasons are `active`, `nothing_to_repair`, `not_found`, `forbidden_role`, `provisioned` (a starter dashboard, which only a release changes) and `queue_full`.
It needs an Editor key and a model on the server, and exits 1 when it queued no repair.

`health run` starts a check of every dashboard now and needs an Admin key.
Results appear as each dashboard is checked.

### Audit trail

`observe audit` reads the [audit trail](../reference/audit-queries.md) with an Admin key.
Every other key gets 403 `forbidden_role`.

```sh
observe audit search --query 'outcome:denied and origin:mcp' --from now-7d
observe audit search --follow --query 'outcome:denied'
observe audit show <event-id>
observe audit facets actor_name --query 'action:dashboard.*'
observe audit histogram --query outcome:failed --buckets 48
observe audit aggregate --query '{"aggs":{"by":{"terms":{"field":"origin"}}}}'
observe audit fields
observe audit export --from now-30d --format csv --file audit.csv
```

The subcommands that read a window of events share three flags:

- `--query` takes KQL, `lucene:` and a Query DSL body, and matches everything by default.
- `--from` and `--to` take an RFC 3339 time, epoch milliseconds or a relative time such as `now-7d`.
  The window is the last 24 hours by default and at most 366 days.

`--json` prints the API response unchanged.
The subcommands are these:

- `search` prints a table of events, newest first, with `WHEN`, `WHO`, `VIA`, `ACTION`, `OUTCOME`, `TARGET` and `ID`.
  `WHO` reads `<agent> for <name>` for an agent, `VIA` is the origin and tool, and `OUTCOME` includes the reason.
  It prints 50 a page by default and at most 1,000, and pages with `--cursor` or `--all`.
  With no match it prints `No audit events match in the window.` on standard error.
- `show <event-id>` prints one event and the other events of its request and its agent run.
- `facets <field>` prints the most frequent values of a field, and `--limit` takes 1 to 100, 10 by default.
- `histogram` prints the counts over time, and `--buckets` takes 1 to 200, 60 by default.
  It adds `N events, one bucket per <step>.` on standard error.
- `aggregate` prints the rows of a Query DSL `aggs` body.
  `--query` must start with `{`, or the command exits 2.
  It allows up to 3 levels and the metrics `cardinality`, `value_count`, `min`, `max`, `avg`, `sum` and `percentiles`.
- `fields` prints what a query can name.
- `export` streams NDJSON, the default, or CSV with `--format csv`.

#### Export the trail

`export` writes to standard output, or to `--file`.
It warns on standard error when it stopped at the server's limit of 1,000,000 rows and more events matched.

With `--file` it writes to `<file>.partial` and renames it to the name you gave once the export is complete.
It then prints `Wrote N events to <file>.`, which `-q` hides.
If the server or the connection ends the export early, it exits 1, says how many events arrived and leaves them in `<file>.partial`.

#### Follow the trail

`search --follow` reads `GET /api/v1/audit/stream` and prints each matching event recorded from then on, in the order it committed, until Ctrl-C.
With `--json` it prints one event object per line.
It cannot be combined with `--all`, `--cursor`, `--limit`, `--from` or `--to`.

It exits 1 when the key is no longer an Admin, or when the server ends the stream because the query is too expensive to follow.
When the connection drops, it reconnects after the server's retry delay and from the last event it printed.
It prints every matching event committed since, down to 10 minutes before that event's time.
When that event is more than 24 hours old, it exits 1 and asks you to read the gap with `search --from`.
It exits 1 after 3 failed attempts in a row, and a nothing-received gap of 45 seconds counts as a drop.

## Manage your workspace

Most commands here need an Editor or Admin key.
Each says which.

### Folders

`observe folders` manages the [folders](../teams/folders.md) that hold dashboards, observations and saved searches:

```sh
observe folders list
observe folders create Team Payments
observe folders rename <folder-id> Team Billing
observe folders delete <folder-id>
observe folders permissions show <folder-id>
observe folders permissions grant <folder-id> --email ada@example.com --level edit
observe folders permissions revoke <folder-id> --group payments
observe folders permissions open <folder-id> --yes
observe folders permissions admins-only <folder-id> --yes
```

`list` prints `ID`, `TITLE`, `DASHBOARDS` and `LEVEL` columns for the folders the key can see.
It marks the General folder `(General)`, your My Dashboards `(yours)`, an Admins-only folder `(Admins only)` and the [Audit folder](activity.md#audit-folder) `(Audit folder)`.

`create` joins its words into the title, needs the Editor role and prints `Created folder <title> (<id>)`.
A title that another folder uses answers `folder_exists`.
`rename` joins the words after the id into the new title and prints `Renamed folder to <title>`.
The General folder answers `general_folder`.
`delete` prints `Deleted folder <id>`.
It works only on an empty folder, asks for no `--yes`, and answers `folder_not_empty` when anything is still filed in it.
The General folder cannot be deleted either.

#### Set who can open a folder

`permissions show` lists who the folder is restricted to, users first and then groups, or says the folder is open to everyone by role.
The other subcommands change the list:

- `permissions grant` adds a person, with `--user <id>` or `--email <address>`, or a group with `--group <name>`, at a `--level` of `view`, `comment`, `edit` or `admin`.
  It changes the level of the entry that names the same person or group, and keeps every other entry.
  The first grant restricts the folder.
  It prints `Granted <level> on folder <id> to <who>`.
- `permissions revoke` removes one entry and keeps the others, and prints `Removed <who> from folder <id>`.
  It answers that the name is not on the list when it is not.
  It needs `--yes` when it removes the last entry, because the folder is then open to everyone by role again.
- `permissions open --yes` clears the whole list in one step and prints `Folder <id> is open to everyone by role`.
- `permissions admins-only --yes` clears the list and lets only Admins read the folder, its creator included.
  It prints `Only Admins can read folder <id>`, and `permissions show` then says so.
  It needs an Admin key and never applies to a personal folder.

Changing the list needs Manage on the folder.
An Admin key has Manage on every folder except another person's personal one, and the General folder cannot be restricted.
Each command is one read followed by one replace of the whole list, so two people changing the same folder at once can overwrite each other.

The server can also refuse with `restricted_content` or `restricted_answer` when items or answers would become unreadable.
It answers `admins_only_folder` when closing a folder that holds an unexpired public link, and `general_folder` for the General folder.

### Ask for comment access

`observe access` asks for [comment access](../teams/folders.md#ask-for-comment-access) to a dashboard or observation you can view, and lists the requests that wait for you:

```sh
observe access request dashboard:<dashboard-id> --note "I am on call for checkout this week"
observe access request observation:<observation-id>
observe access requests
```

`request` takes the object as `<kind>:<id>`, with `dashboard` or `observation` as the kind.
The optional `--note` is one line of at most 200 characters, and a bad note answers `invalid_note`.
It prints `Asked for comment access to <kind> <title>; the people who manage its folder will see it in their inbox` and changes no access.
`-q` does not hide that line.

Three answers are possible when the request cannot open:

- A request that is still open answers `request_open` with the time it was made.
- An object you can already comment on answers `already_can_comment`.
- One you cannot see answers `not_found`.

A request closes when Comment is granted, when a recipient marks it done, or after 30 days.
A service-account key may ask too, and its request is granted by raising the service account's role.

`requests` prints `ASKED`, `WHO`, `WHO ID`, `OBJECT`, `TITLE`, `FOLDER` and `NOTE` columns for the open requests on the folders you manage.
It lists the newest first and at most 100, or prints `No open access requests on folders you manage.`
Grant one with `observe folders permissions grant <folder-id> --user <id> --level comment`, or by raising the role.
General takes no folder entry.
For a request whose `FOLDER` is General, the command adds a note on standard error that says to raise the person's role on the People page, or to move the object to a folder first.
`-q` hides that note.

### Datasources

`observe datasources list` prints each datasource with its uid, type, name and health.
It also prints its source (`file` or `app`), the dashboards that use it, and when it was last checked.
An `AUDIENCE` column shows who can query it, `everyone` or `admins`, and only an Admin key sees it.
An unhealthy datasource shows the failure and, when the server knows it, the cause.
With none, it prints `No datasources yet` on standard error.

`observe datasources recheck <uid>` skips the server's 10 second health cache and prints the new status.
It exits 1 while the datasource is unhealthy.
Any role can list and recheck, and neither takes flags.
`--json` prints the API response.

For finding what a datasource holds, see [Discovering what a datasource holds](#discovering-what-a-datasource-holds).

#### Add and change datasources

An Admin can manage the datasources added in the app, with these commands:

```sh
observe datasources get prom
observe datasources add prom --type prometheus --address https://prometheus.example.com
observe datasources edit prom --name "Production Prometheus"
observe datasources test prom --saved
observe datasources remove prom
```

A datasource from the datasources file cannot be changed here.
See [Add datasources in the app](../connect/datasources-in-the-app.md).

- `get <uid>` prints the address, login and which credentials are set, never their values.
- `add <uid> --type <type> --address <address>` creates one.
  `--type` is `prometheus`, `loki`, `tempo`, `jaeger`, `clickhouse`, `elasticsearch` or `opensearch`.
  `--address` is an `http` or `https` URL that passes the [outbound allowlist](../operate/outbound-access.md), or `host:port` for ClickHouse.
- `edit <uid>` changes the settings you pass, and needs at least one.
- `test <uid>` runs the health check on the definition you give and saves nothing.
  It takes `--saved` and every flag of `add`, and `--type` is optional.
  With `--saved` it starts from the saved datasource, so its stored credentials are used.
  It prints `The datasource answered the health check` or `The check failed: <reason>`, and exits 1 while the datasource is unhealthy.
- `remove <uid>` prints `Removed datasource <uid>`.
  It is refused with the list of dashboards and saved searches that still use the datasource.
  The list says `and N more in folders you cannot see` when it is longer.

`add` and `edit` print `Added datasource <uid>` or `Changed datasource <uid>`, then `Check it now: observe datasources recheck <uid>`.
An edit applies at once on the server you reached and within 15 seconds on the others.

Details of the `add` and `edit` flags:

- `--name` defaults to the uid.
- `--audience` is `everyone`, the default, or `admins`, who alone can then query the datasource.
- `--username` and `--timeout` set the login and the request timeout.
- `--max-series`, `--max-points`, `--max-rows` and `--max-concurrent-queries` set the limits.
- `--grafana-uid` is repeatable, or comma-separated, and names the Grafana datasource uids this one stands for on import.
- `--scrape-interval` is for Prometheus, `--tenant-id` for Loki, and `--database` and `--tls` for ClickHouse.
- `--index`, `--time-field`, `--query-language` (`kql` or `lucene`), `--field-level`, `--field-message`, `--field-service`, `--field-span-id` and `--field-trace-id` are for Elasticsearch and OpenSearch.
- `--password-file`, `--bearer-token-file`, `--api-key-file` and `--header NAME=FILE` read a credential from a file, where `-` reads standard input once.
  A credential must not be empty and may be at most 4096 bytes.
  A credential you do not pass keeps its stored value.
- `edit --clear` removes a stored credential: `password`, `bearer_token`, `api_key` or `header:<name>`.

### Grafana connections

`observe grafana import` imports the dashboards of a Grafana connection for [Switch from Grafana](switch-from-grafana.md).
It needs an Admin key.
A person adds, changes, tests and removes the connection on the Grafana settings page, because a connection stores a credential and a key cannot manage it.

#### Import from a Grafana

`observe grafana import <connection id>` imports dashboards from that Grafana, with `--folders` for some folders or `--all` for every one:

```sh
observe grafana import <connection id>
observe grafana import <connection id> --folders general,platform
observe grafana import <connection id> --all
```

`--folders` takes comma-separated Grafana folder uids, each with its subfolders, and `general` stands for the dashboards outside any folder.
`--folders` with `--all` exits 2.
Without either flag it prints the folders as a table of uid, title and dashboard count and exits 2.

It then waits while the import is `running` and shows `Imported <done> of <total>` on a terminal.
It reads the import's `state`, and the older `status` when a server sends a state the CLI does not know.
At the end it prints how many dashboards were done, skipped and failed, a table of the failed ones with their error, and a notice for dashboards with panels that did not verify.
It exits 1 when any dashboard failed or the import failed or was canceled, and `--json` prints the final import, whose `state` is `finished`, `failed` or `canceled`.
An import that stops in any other state also exits 1, with that state in the error line, and `--json` prints it as it came.
An interrupt stops the waiting while the import continues on the server.
The server keeps a connection's last 20 imports.

### MCP sources

An Admin registers MCP servers for the agent and approves their tools with `observe mcp-sources`; see [MCP sources](../operate/mcp-sources.md).

```sh
printf '%s' "$DOCS_TOKEN" | observe mcp-sources add docs --url https://docs.example.com/mcp --secret-header Authorization --secret-stdin
observe mcp-sources discover docs
observe mcp-sources tools docs
observe mcp-sources approve docs search
observe mcp-sources approve docs --all-read-only
observe mcp-sources revoke docs search
```

- `list [uid]` prints every source with its state, audience and tool counts, or one source in full.
  It never prints a secret, only whether one is set and the header that carries it.
- `add <uid> --url <address>` registers a source and asks the server nothing.
  `--name`, `--description` (one line, at most 300 characters), `--audience` (`everyone` or `admins`), `--secret-header` and `--secret-stdin` complete it.
  The secret comes from standard input and needs `--secret-header`, which needs `--secret-stdin` in turn.
- `edit <uid>` changes the flags you pass, takes `--clear-secret`, and needs at least one flag.
  `--description ""` removes the description.
  A new host needs `--secret-stdin` again, or `--clear-secret`.
- `remove <uid>` deletes the source, its tools and its secret.
- `discover <uid>` lists the server's tools and prints how many are new, changed, gone or ignored.
  It exits 1 and prints the failure when the server could not be asked.
- `tools <uid>` prints each tool with its state, whether the server marks it read-only and its refusal.
- `approve <uid> <tool>` approves one tool as it is listed now, and fails when the server changed it since.
  It approves a tool the server does not mark read-only too, and then warns that the agent may call it and that it can change the system it reaches.
  `approve <uid> --all-read-only` approves every tool still to review that the server marks read-only, and leaves the others to be approved by name.
  A refused tool cannot be approved.
- `revoke <uid> <tool>` withdraws an approval.

Every command takes `--json` to print the API response, and needs an Admin key.

### Webhooks

`observe webhooks` manages the [webhooks](webhooks.md) that send workspace activity to an outside address as signed requests.
Every verb needs an Admin key.

```sh
observe webhooks add --name "Failed runs" --url https://hooks.example.com/slashobserve --event run.failed
observe webhooks list
observe webhooks test <webhook id>
```

A server holds at most 20 webhooks, and a 21st answers 422 `too_many_webhooks`.
A server with no `SLASHOBSERVE_SECRET_KEY` answers 409 `secret_key_missing`, because it cannot store a signing secret.
A webhook whose secret the current key cannot read answers 409 `secret_unreadable`, and `rotate-secret` makes a new one.
`--json` prints the response body of each request unchanged.

- `list` prints each webhook's id, name, address, number of event types and status.
  The status is `sending`, `retrying` or `disabled (<reason>)`.
  A retrying webhook also shows how many events wait behind the failing one and when the next attempt is due, such as `retrying (3 waiting, next 2026-11-11T09:15Z)`.
  `100+` means more than 100.
- `add --name <name> --url <https address> --event <type>` creates a webhook, and `--event` repeats for several types.
  All three flags are required, or it exits 2.
  The name is 1 to 100 characters, and the address is `https` on port 443, at most 2048 characters, with no credentials or fragment in it.
  An address whose domain is not on the [outbound allowlist](../operate/outbound-access.md) fails with `outbound_domain_not_allowed`.
  `--format slack` or `--format teams` sends one line of text for a Slack or Teams address instead of the signed JSON envelope, `generic`, the default.
  It prints the webhook and its signing secret once, and a notice on standard error says the secret is not shown again, so copy it then.
  `-q` hides the notice but not the secret.
- `update <id>` changes the `--name`, `--url`, `--format` or `--event` list you pass and leaves the rest.
  `--event` replaces the whole list, and with none of the flags it exits 2.
- `enable <id>` and `disable <id>` turn a webhook on, clearing its failures, or off.
- `test <id>` sends a `webhook.test` event now and prints its outcome, the receiver's status, the duration and the error code.
  It exits 1 when the event was not delivered.
- `skip <id>` passes a retrying webhook over the oldest event it is stuck on, so the events behind it go out.
  The skipped event is never sent to that receiver and shows in `deliveries` as `skipped`.
  It exits 1 with `nothing_to_skip` when the webhook is not retrying, and with `webhook_busy` while a server is delivering for it.
- `rotate-secret <id>` makes a new signing secret and prints it once.
  The old one stops working at once.
- `delete <id>` deletes the webhook and its recorded attempts.
- `deliveries <id>` prints the newest attempts first as a table of time, event, outcome, status, duration and error code.
  `--limit` takes 1 to 100, 20 by default, and `--cursor` and `--all` page through the result.
  The server keeps attempts for 7 days.

`update`, `enable`, `disable` and `rotate-secret` keep the webhook's owner.
`update --take-ownership` makes the caller the owner.
`enable` does the same for a webhook disabled with the reason `owner`, as the [HTTP API](webhooks.md#manage-webhooks-over-http) does.

### People

`observe people` lists everyone who has signed in and invites people by email with the role they get at their first sign-in, as described in [People](../teams/people.md#invite-people).
Every verb needs an Admin key.

```sh
observe people list
observe people invite new@example.com --role viewer
observe people invites
observe people uninvite <invite id>
```

No email is sent, so send the printed sign-in link yourself.
The role applies only at the person's first sign-in through your identity provider with a verified email.
An invite never admits anyone the sign-in rules refuse.

- `list` prints everyone who has signed in, newest first, as a table of id, name, email, role, status (`active` or `deactivated`) and when they were last active.
- `invite <email> --role <role>` records a pending invite and prints `Invited <email> as <role>`, its id and its sign-in link.
  `--role` is `admin`, `editor`, `commenter` or `viewer`, and a missing or invalid role exits 2.
  Inviting an email that already has a pending invite changes its role.
  An email that has already signed in fails with `already_member`, and one outside `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` with `email_domain_not_allowed`.
- `invites` prints the pending invites, newest first, as a table of id, email, role, who invited and when.
- `uninvite <invite id>` withdraws a pending invite and prints `Withdrew invite <id>`.

`list` and `invites` take `--limit` of 1 to 500, 50 by default, with `--cursor` for the next page and `--all` for every page.
An Admin deactivates and reactivates people in the web app, because a key never ends another person's sessions.

### Teams

`observe teams` manages the [teams](../teams/people.md#teams) whose names share folders with their members.
Every verb needs an Admin key.
The verbs take no flags other than `--json` and `-q`.

```sh
observe teams create sre
observe teams add <team id> <person id>
observe teams list
```

- `list` prints each team's id, name, number of members and their names.
- `create <name>` creates a team with no members.
  A name another team has, in any case, fails with `team_exists`, and a server holds at most 200 teams, so a 201st answers 422 `too_many_teams`.
- `add <team id> <person id>` and `remove <team id> <person id>` change the members and print the team.
  `observe people list` prints the person ids.
- `delete <team id>` deletes the team.

## Use the CLI from an agent

### Agent skill

`observe skill install` writes the `/observe` skill, the guide that outside agents follow, to `~/.agents/skills/observe/SKILL.md` and `~/.claude/skills/observe/SKILL.md`.
Claude Code and other agents that read Agent Skills then offer it as `/observe`.

The guide covers how to build and change dashboards, investigate a question, search logs and traces, answer comments, keep observations, review proposals, take assigned runs and read the audit trail.
The command replaces an earlier copy and prints one `Installed <path>` line per file.
It needs no server, sign-in or network.
`--dir <path>` writes only `<path>/observe/SKILL.md`.
`observe skill` alone prints help.

The skill is part of the CLI program and is never fetched from a server, so the text always comes from the release you installed.
Run `observe skill install` again after you upgrade the CLI.
The same guide reaches MCP clients as the [`observe` prompt](mcp.md#the-observe-prompt).

### Shell completion

`observe completion bash`, `zsh` or `fish` prints a completion script for commands and flags.
Load it for the current shell, or save it where your shell reads completions:

```sh
source <(observe completion bash)
observe completion zsh > "${fpath[1]}/_observe"
observe completion fish > ~/.config/fish/completions/observe.fish
```

Other shells are not supported, and `observe completion` alone prints help.

## Reference

### Settings and saved keys

The CLI reads its settings in this order: flags, the environment, then the saved file.

| Setting | Flag | Environment variable |
| --- | --- | --- |
| Server | `--server` | `OBSERVE_SERVER` |
| Key | `--token` | `OBSERVE_TOKEN` |

Prefer `OBSERVE_TOKEN` to `--token`, since flags end up in shell history and the process list.
A saved key is sent only to the server it was saved for.
To use another server, pass `--token` or set `OBSERVE_TOKEN`.
Otherwise the command fails with `the saved key belongs to <server>; pass --token or run observe login --server <current>`.

Details of the saved file:

- The file is `$XDG_CONFIG_HOME/slashobserve/cli.json` when that variable is an absolute path, else `~/.config/slashobserve/cli.json`.
- The file has mode 0600 and its directory 0700.
- Two server addresses match when their scheme, host and path match, with the default ports dropped.

`observe version`, or `--version`, prints the CLI's version, which is `dev` for a binary built without a version.
`--json` has no effect on local commands: `skill install`, `completion` and `version`.

### Global and prompt flags

These flags work on every command.

| Flag | Meaning |
| --- | --- |
| `--server` | The server address. |
| `--token` | The service-account key. |
| `--json` | Print the API response body instead of text. |
| `-q`, `--quiet` | Hide progress and confirmations. Results and errors still print. |
| `--help` | Print help for a command. |

The prompt command `observe [flags] <prompt words...>` takes these flags.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--dashboard` | none | Ask about, or with `--revise` change, a dashboard. |
| `--revise` | false | Change `--dashboard` instead of only answering. |
| `--apply` | none | Apply an answer's suggested change, by finding id. |
| `--chat` | none | Send the next turn of one of your chats. |
| `--new-chat` | false | Start a new chat instead of joining the latest. |
| `--at` | none | The spot on `--dashboard`, as JSON or a short form. |
| `--no-wait` | false | Print the request id and return. |
| `--timeout` | 2h | Stop waiting. The request keeps running. |
| `--open` | false | Open the saved dashboard in the browser. |

The command exits 2 before it sends anything in these cases:

- `--revise` without `--dashboard`.
- `--new-chat` with `--chat`.
- `--apply` with prompt words, or without `--revise`.
- A `--timeout` of 0 or less.
- No prompt words, which prints `a prompt is required` and the usage.

### Exit codes

The CLI exits with these codes.

| Code | When |
| --- | --- |
| `0` | The command succeeded. |
| `1` | The server refused or failed the request, or a check the command makes failed. |
| `2` | The command line is wrong. |

Details of code 0:

- It includes a prompt where `--open` could not open the browser, and `observe logs --tail` ending with Ctrl-C.
- `observe status <id>` without `--wait` and `observe cancel` exit 0 even when the request failed.
  Only commands that wait exit 1 for a failed request.

Details of code 1:

- The server refused or failed the request, the agent request failed or was canceled, or the waiting stopped at `--timeout` or an interrupt.
- `observe open` could not open the browser.
- An `observe doctor` check failed.
- An `observe datasources lint` diagnostic is an error, or `datasources recheck` or `datasources test` finds the datasource unhealthy.
- An `observe grafana import` had a dashboard fail, or the import failed or was canceled.
- An `observe import` refused an already imported file, or read an empty or oversized file or standard input.
- An `observe dashboards apply` had a file fail.
- An `observe health repair` queued no repair.
- A `--follow` command used up 3 failed reconnects.

Details of code 2:

- An unknown command or flag, a missing argument or an invalid id.
- A file or directory named on the command line that cannot be read.
- A flag combination the command refuses before it sends anything, such as `--all` with `--cursor` or a `--limit` outside its range.

### Error output

An error is one line on standard error.
It holds the server's message with its code in parentheses, when the server sent both.
Without a message the line reads `the server answered HTTP <status>`.
A message without a code has no parentheses.

Below the line, the failing paths of an invalid Spec or panel print, at most 10, then `and N more`.
A hint line follows some errors: `forbidden_role`, `outbound_domain_not_allowed`, `observation_thread_exists`, `restricted_content`, and a 400 that mentions a cursor.

With `--json`, the error is one JSON object on standard error, so a script can branch on the code:

```json
{"error": "<message>", "code": "<code>"}
```

The object can also carry `start` and `end`, integers, when the server sends a byte range.
An agent request that ended failed or canceled while the CLI waited carries `run_id` and `run_state` too.

Details of the codes:

- The code is the server's own, such as `not_found`, `forbidden_role` or `version_conflict`.
- `usage` is a wrong command line.
- `unauthenticated` is a missing or refused key.
- `unreachable` means the server cannot be reached.
- `interrupted` is an interrupt, a Ctrl-C during a wait included.
- `http_<status>` is a status where the server sent no code.
- `failed` is anything else.

An address that answers with a web page instead of JSON is reported as probably not a SlashObserve server.
A version that does not exist is reported with the latest version of the dashboard.

### Lists and paging

The commands that list rows in pages take `--limit`, `--cursor` and `--all`.

| Command | Default limit | Largest limit |
| --- | --- | --- |
| `dashboards list`, `dashboards versions`, `dashboards revisions` | 50 | 200 |
| `comments list`, `annotations list` | 50 | 500 |
| `people list`, `people invites` | 50 | 500 |
| `o list`, `saved-searches list`, `proposals list` | 50 | 100 |
| `requests list`, `health` | 50 | 100 |
| `chats list` | 50 | 50 |
| `audit search` | 50 | 1000 |
| `search` | 20 | 50 |
| `activity`, `webhooks deliveries` | 20 | 100 |

`--limit` outside its range exits 2 before anything is sent.
`observe <command> --help` shows the range of one command.

When more rows exist, the table is followed on standard error by `Showing 5 of 23. More: --cursor <cursor>, or --all for every page.`
The total is printed where the server counts it, as `dashboards list` does, and otherwise the line says that more exist.

`--cursor` continues from the `next_cursor` a page ended with, which `--json` shows too.
A cursor the server does not recognise is refused with `invalid_request` and a hint to pass it exactly as printed.

`--all` follows the cursor to the last page and prints one list.
With `--json` it is one object whose list holds every row, with `next_cursor` null and any `total_count` kept.
After 1000 pages, or a repeated cursor, it fails with `the server kept answering with more pages; use --cursor to read them one at a time`, exits 1 and prints no partial list.

`--all` cannot be combined with `--cursor`, and `activity --follow` cannot be combined with either.
The MCP list tools take their own, smaller page sizes and the same `next_cursor`.

### Permission errors

A refusal with the code `forbidden_role` and the message "your role does not allow this" gets a hint that an Admin can change the key's role on the API keys page.
A second hint covers 403 `forbidden_role` with "you do not have access to this dashboard".

A dashboard the key cannot see answers `not_found`, like a dashboard that does not exist.
Activity, search and observations answer 403 for something the key cannot see.
A key matches no access list entry, because entries name people and groups.
So a key sees a restricted folder only as an Admin service account or as the service account that created the folder, as [Folders](../teams/folders.md) describes.

### Roles the commands need

Most reads need the Viewer role.
These commands need more.

| Role | Commands |
| --- | --- |
| Editor | `investigate`, `folders create`, `health repair`, `annotate` without `--dashboard` |
| Admin | `audit`, `webhooks`, `people`, `teams`, `grafana`, `health run` |
| Admin | `datasources get`, `add`, `edit`, `test` and `remove` |
| Admin | `requests list --scope all`, `chats list --scope all`, `folders permissions admins-only` |
| Admin | `investigate --as-of` |
| Comment on the target | `comments add`, `comments people` |
| Edit on the target | `dashboards move`, `dashboards delete`, `annotate --dashboard` |

### Commands and flags

Each table lists the flags a command registers.
A flag with a default shows it in parentheses.
The sections above describe what each flag does.
Every command also takes the global flags.

#### Agent requests

| Command | Flags |
| --- | --- |
| `observe <prompt...>` | The prompt flags above. |
| `status <request-id>` | `--wait`, `--timeout` (2h), `--open` |
| `cancel <request-id>` | None. |
| `continue <request-id>` | `--note`, `--no-wait`, `--timeout` (2h), `--open` |
| `retry <request-id>` | `--no-wait`, `--timeout` (2h), `--open` |
| `save-answer <request-id>` | None. |
| `share-answer <request-id>` | `--thread` |
| `investigate <question...>` | `--dashboard`, `--panel`, `--from`, `--to`, `--observation`, `--block`, `--at`, `--as-of`, `--private`, `--open`, `--no-wait`, `--timeout` (2h) |
| `run [<name>]` | `--no-wait`, `--timeout` (2h), `--open` |
| `requests list` | `--scope` (mine), `--state`, `--q`, `--pinned`, `--dashboard`, `--observation`, `--anchored`, `--limit` (50), `--cursor`, `--all` |
| `requests pin <id>`, `requests unpin <id>` | None. |
| `requests rename <id> <title...>` | `--clear` |
| `requests archive <id>` | `--delete-observation`, `--yes` |
| `chats list` | `--scope` (mine), `--q`, `--pinned`, `--dashboard`, `--observation`, `--limit` (50), `--cursor`, `--all` |
| `chats show <chat-id>`, `chats pin <chat-id>`, `chats unpin <chat-id>` | None. |
| `chats archive <chat-id>` | `--yes` |

#### Setup and sessions

| Command | Flags |
| --- | --- |
| `login` | `--with-token` |
| `logout` | None. |
| `version` | None. |
| `open [<id or address>]` | None. |
| `doctor` | None. |
| `skill install` | `--dir` |
| `completion bash`, `zsh`, `fish` | None. |

#### Dashboards

| Command | Flags |
| --- | --- |
| `dashboards list` | `--order` (updated), `--exclude-audit`, `--limit` (50), `--cursor`, `--all` |
| `dashboards show <id>` | `--version` |
| `dashboards versions <id>` | `--limit` (50), `--cursor`, `--all` |
| `dashboards revisions <id> [<revision-id>]` | `--limit` (50), `--cursor`, `--all` |
| `dashboards tags` | `--exclude-audit` |
| `dashboards viewers <id>` | None. |
| `dashboards move <id>` | `--folder` (required) |
| `dashboards duplicate <id>` | `--title` (required), `--folder` |
| `dashboards restore <id>` | `--version` (required), `--parent`, `--summary` |
| `dashboards panels <id>` | `--summary` (required), `--file`, `--remove`, `--parent` |
| `dashboards diff <id>` | `--from`, `--to` |
| `dashboards export <folder>` | `--dir` (current directory) |
| `dashboards apply` | `--dir` (required) |
| `dashboards schema`, `dashboards sample` | None. |
| `dashboards delete <id>` | `--yes` |
| `import <file or ->` | `--map`, `--preview`, `--folder`, `--update`, `--as-new`, `--open` |

#### Logs, traces, metrics and datasources

| Command | Flags |
| --- | --- |
| `logs [<query>]` | `--datasource`, `--from` (now-1h), `--to` (now), `--limit` (100), `--tail`, `--ask`, `--dry-run`, `--facets`, `--field`, `--patterns`, `--context`, `--before` (50), `--after` (50) |
| `traces [<trace id>]` | `--datasource`, `--from` (now-24h), `--to` (now), `--ask`, `--query`, `--dry-run`, `--sort`, `--compare`, `--baseline`, `--limit` (20 with `--compare`) |
| `metrics query [<promql>]` | `--datasource`, `--from` (now-1h), `--to` (now), `--instant`, `--ask`, `--dry-run` |
| `datasources list`, `datasources recheck <uid>` | None. |
| `datasources metrics <uid>` | `--search`, `--limit` (100) |
| `datasources labels <uid>` | `--search`, `--limit` (100), `--metric`, `--table`, `--from`, `--to` |
| `datasources label-values <uid> <label>` | `--search`, `--limit` (100), `--metric`, `--table`, `--from`, `--to` |
| `datasources services [<uid>]` | `--signal`, `--window` (1h), `--service` |
| `datasources schema <uid>` | `--table`, `--offset` (0) |
| `datasources sample <uid>` | `--table`, `--limit` (5) |
| `datasources lint <uid> <query>` | None. |
| `datasources log-volume <uid> <query>` | `--from` (now-1h), `--to` (now), `--buckets` (60) |
| `datasources trace-stats <uid> [<filter>]` | `--from` (now-1h), `--to` (now), `--buckets` (60) |
| `datasources trace-logs <uid> <trace-id>` | `--from` (now-1h), `--to` (now), `--span`, `--service`, `--limit` (100) |
| `datasources get <uid>` | None. |
| `datasources add <uid>`, `edit <uid>`, `test <uid>` | `--type`, `--address`, `--name`, `--audience`, `--username`, `--timeout`, the limit, type and credential flags in [Add and change datasources](#add-and-change-datasources); `test` adds `--saved`, and `edit` adds `--clear` |
| `datasources remove <uid>` | None. |
| `search <words...>` | `--kind`, `--limit` (20), `--cursor`, `--all` |
| `saved-searches list` | `--kind`, `--folder`, `--limit` (50), `--cursor`, `--all` |
| `saved-searches save` | `--kind`, `--title`, `--datasource`, `--folder`, `--query`, `--from` (now-1h), `--to` (now), `--columns`, `--service`, `--span`, `--min`, `--max`, `--errors`, `--mode` |
| `saved-searches update <id>` | `--query`, `--from`, `--to`, the filter flags, `--clear-filters` |
| `saved-searches rename <id> <title...>` | None. |
| `saved-searches move <id>` | `--folder` |
| `saved-searches delete <id>` | None. |

#### Observations, comments and proposals

| Command | Flags |
| --- | --- |
| `o new <title...>` | `--folder`, `--from`, `--to`, `--text`, `--text-file` |
| `o list` | `--folder`, `--state`, `--title`, `--limit` (50), `--cursor`, `--all` |
| `o show <id>` | None. |
| `o add <id>` | `--text`, `--text-file`, `--panel-file`, `--snapshot-of`, `--from`, `--to`, `--var`, `--half`, `--width`, `--height`, `--position` |
| `o edit <id>` | `--title`, `--from`, `--to`, `--clear-range` |
| `o edit-block <block-id>` | `--version`, `--text`, `--text-file`, `--panel-file`, `--from`, `--to`, `--half`, `--full`, `--width`, `--height`, `--position`, `--follow-page` |
| `o arrange <id> <block-id>...` | None. |
| `o remove-block <block-id>` | `--version` |
| `o keep <block-id>` | `--version` |
| `o discard <block-id>` | None. |
| `o move <id>` | `--folder` |
| `o resolve <id>` | `--summary` |
| `o reopen <id>` | None. |
| `o pin <id>` | `--dashboard` |
| `o dashboard <id>` | `--block`, `--new` |
| `o publish <id>` | None. |
| `o rcas <id> [<revision>]` | None. |
| `o delete <id>` | `--yes` |
| `o withdraw <id> <revision>` | `--yes` |
| `comments list [<dashboard-id>]` | `--on`, `--status` (open), `--panel`, `--limit` (50), `--cursor`, `--all` |
| `comments add <text...>` | `--on` (required), `--panel`, `--version`, `--block`, `--at`, `--to` (everyone) |
| `comments reply <comment-id> <text...>` | `--block`, `--reply-to`, `--to` |
| `comments edit <comment-id> <text...>` | None. |
| `comments delete <comment-id>` | None. |
| `comments withdraw <comment-id>` | None. |
| `comments apply <comment-id> <finding-id>` | None. |
| `comments people` | `--on` (required), `--search`, `--limit` (20) |
| `comments resolve <comment-id>` | `--version` |
| `comments reopen <comment-id>` | None. |
| `proposals list <dashboard-id>` | `--state` (open), `--limit` (50), `--cursor`, `--all` |
| `proposals show <proposal-id>` | None. |
| `proposals merge <proposal-id>` | `--revision` (required), `--take` |
| `proposals discard <proposal-id>` | `--reason` |
| `annotate <text...>` | `--tags`, `--dashboard`, `--at` |
| `annotations list` | `--dashboard`, `--from`, `--to`, `--tag`, `--limit` (50), `--cursor`, `--all` |
| `annotations update <id>` | `--text`, `--tags` |
| `annotations delete <id>` | None. |

#### Activity, health and audit

| Command | Flags |
| --- | --- |
| `activity` | `--dashboard`, `--folder`, `--observation`, `--actor`, `--type`, `--since`, `--until`, `--limit` (20), `--cursor`, `--all`, `--follow` |
| `health` | `--kind`, `--folder`, `--limit` (50), `--cursor`, `--all` |
| `health repair <dashboard-id>...` | None. |
| `health run` | None. |
| `audit search` | `--query`, `--from`, `--to`, `--limit` (50), `--cursor`, `--all`, `--follow` |
| `audit show <event-id>`, `audit fields` | None. |
| `audit facets <field>` | `--query`, `--from`, `--to`, `--limit` (10) |
| `audit histogram` | `--query`, `--from`, `--to`, `--buckets` (60) |
| `audit aggregate` | `--query`, `--from`, `--to` |
| `audit export` | `--query`, `--from`, `--to`, `--format` (ndjson), `--file` |

#### Workspace administration

| Command | Flags |
| --- | --- |
| `folders list`, `folders delete <id>` | None. |
| `folders create <title...>`, `folders rename <id> <title...>` | None. |
| `folders permissions show <id>` | None. |
| `folders permissions grant <id>` | `--user`, `--email`, `--group`, `--level` |
| `folders permissions revoke <id>` | `--user`, `--email`, `--group`, `--yes` |
| `folders permissions open <id>`, `admins-only <id>` | `--yes` |
| `access request <kind>:<id>` | `--note` |
| `access requests` | None. |
| `grafana import <connection id>` | `--folders`, `--all` |
| `webhooks add` | `--name`, `--url`, `--event`, `--format` (generic) |
| `webhooks update <id>` | `--name`, `--url`, `--event`, `--format`, `--take-ownership` |
| `webhooks list`, `enable <id>`, `disable <id>`, `test <id>`, `skip <id>`, `rotate-secret <id>`, `delete <id>` | None. |
| `webhooks deliveries <id>` | `--limit` (20), `--cursor`, `--all` |
| `people list`, `people invites` | `--limit` (50), `--cursor`, `--all` |
| `people invite <email>` | `--role` |
| `people uninvite <invite id>` | None. |
| `teams list`, `create <name>`, `delete <id>`, `add <team> <person>`, `remove <team> <person>` | None. |
