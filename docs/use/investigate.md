---
title: Investigate
description: Ask the agent a question about your data and get an answer in an observation, a short written conclusion with up to four evidence panels, that you can pin to a dashboard or turn into one.
---

An investigation asks the agent a question instead of asking it for a dashboard, such as "why did checkout errors rise after 10:00?".
The agent reads your datasources, then ends by saving its answer, a finding, into an [observation](observations.md).
It needs a model; see [LLM providers](../connect/llm-providers.md).

A plain question sent without the command, such as "how many pods are up?", is answered in the chat without building a dashboard.
No observation is made until you choose **Create observation** on the answer, or run `observe save-answer <request-id>`; see [Chat and follow-ups](chat.md).

## Start an investigation

You need the Editor role or above.

1. Open a chat box.
   Use Home, or a chat about a dashboard.
2. Type `/investigate`, a space, and your question.
   The Investigate chip shows above the box.
3. Send it.
   The agent asks the question instead of building a dashboard.

In a chat about a dashboard, the investigation is about that dashboard and its answer appears in the same conversation.
Without a dashboard, the send opens a new chat page for it.

Every investigation also opens an observation.
The observation shows the run while it works and receives the finding's conclusion and evidence when it is saved.
[Investigations write into an observation](observations.md#investigations-write-into-an-observation) says where it goes and how to write into an existing one with `observation_id`.

### Ask about a panel

On a dashboard, **Ask** in a panel's menu opens the [ask box](ask-and-comment.md#ask-mode) on the whole panel.
It asks the agent privately in your chat about that dashboard.
To ask in a thread the team reads, choose the "To:" chip in a comment; see [Ask the agent from a comment](review.md#ask-the-agent-from-a-comment).

The panel menu item shows only on data panels, when you can comment on the dashboard and a model is connected.
While you have unsaved layout changes it reads "Ask (finish the layout first)" and cannot be chosen.

The activity pane has no Investigate chip and takes no slash command.
A message there that asks the agent is a request in a comment thread; see [Comments, agent requests and versions](review.md).
To start an investigation about a dashboard, type `/investigate` in a chat about that dashboard.

### Start one from a terminal

```sh
observe investigate why did checkout errors rise after 10:00
```

The command asks the same question and prints the finding with its title, its answer and its address.
`--dashboard`, `--panel`, `--from` and `--to` set what it is about.
See [CLI](cli.md) and [CLI flags for investigate](#cli-flags-for-investigate).

### Start one from an MCP client

An agent in your own editor can do the same over [MCP](mcp.md).
It investigates with its own model and the discovery and query tools, and reads deploy markers with `list_annotations`.
It saves the answer into an observation with `create_observation` and `add_block`, and reads earlier ones with `list_observations` and `get_observation`.

## Read the finding

In the chat, an investigation's turn shows its steps like a build, with the status "Investigating" while it runs.
When it ends, the turn shows the answer: the conclusion, the evidence panels, and when and over which window it was found.

On a dashboard, your running investigations show in the activity pane as running entries.
Each finding about the dashboard appears there as "Agent found" with the same card and an **Open observation** link.

The page is the observation at `/o/<id>`, with the conclusion and the evidence panels as blocks, the discussion and who wrote what; see [Observations](observations.md#open-an-observation).
The evidence panels always show the finding's own window, not the dashboard's current range.
Each evidence panel has an **Open in Explore** link that opens its query on its datasource over that window in Metrics, Logs or Traces, with a query that uses variables opening as the server ran it; a panel whose datasource has no explorer for its query has no link.
In the steps, the save reads "Saved the answer" and links to the observation, or scrolls to the answer when it is not in an observation yet.
When the agent sends an answer back for a fix, the step says so in a sentence, such as "The answer was sent back for a fix: it said a metric is missing without checking for it", while the agent still receives the full guidance.
A tool call whose input was refused reads "The <tool> call was sent back for a fix", with a plain reason when it is a known kind such as an option the tool does not take; the agent, MCP and the CLI still get the full message and its code.
In an evidence table, a trace id in a `TraceId` cell opens that trace over the finding's window, and a Tempo, Jaeger or ClickHouse `Duration` column shows its time unit.

## Comment on a finding

The comments on an answer are the observation's discussion, one thread for the whole page.
You write in it as [Observations](observations.md#who-can-do-what) describes.

- Mention people and the agent the way you do on a dashboard; see [Edit a comment and mention people](review.md#edit-a-comment-and-mention-people).
- A mention is recorded in the [activity log](activity.md).
- A comment that asks an agent (`to: agent` over the API, MCP or the CLI) starts an investigation for it; see [Ask an agent from a comment](cli.md#ask-an-agent-from-a-comment).
- Commenting follows who can see the observation.
  You need Comment on its folder, and anyone else who can read it sees the discussion but no box and no buttons.

Threads that people opened on a finding before the discussion existed were merged into it when the server upgraded.
The comment API takes only the observation as a target: a `finding:<id>` target is refused as an invalid target.

## Pin a finding to the dashboard

1. Open the finding's card on the dashboard or in the chat.
2. Choose **Pin to dashboard**.

The finding becomes an annotation at the end of the finding's window, tagged `finding`.
Its text is the finding's first line and a link to it.
Charts on the dashboard then show it as a marker; see [Annotations](annotations.md).

The card then reads "Pinned to the dashboard" with an **Open dashboard** link, and pinning again changes nothing.
Pinning needs Edit on the dashboard, so the button is not shown to a Commenter or Viewer.
They see "Pinned to the dashboard" once someone else has pinned it.
Over the API, `POST /api/v1/findings/{id}/pin` without Edit answers `403` `forbidden_role` with `needed_access` `edit`.

## Share a finding

Move the observation to another folder to share it.
Choose **More actions**, then **Move to folder**, as [Observations](observations.md#rename-resolve-reopen-and-move) describes.
Everyone who can view that folder can then read the answer, and My Dashboards takes no moves.

The finding move tool, command and route are gone.
Only earlier `finding.moved` events remain in the activity log.

## Make a dashboard from a finding

Choose **Make a dashboard** on the finding.
It saves the evidence panels as a new dashboard in your My Dashboards and opens it.
The server runs every evidence query again before it saves.

Anyone who can read the finding can do this, and a finding without evidence panels has no such button.

### What happens to the chat

From a chat, the dashboard opens with that chat in the Chat tab of its side panel, where you can keep asking.
The server attaches the chat to the new dashboard when it makes it, whichever way you made it: the button, the API route or the `make_dashboard` tool.

- The chat stays private to you, and Admins read it without writing.
  Nobody else sees it on the dashboard.
- A chat that is already about another dashboard or observation stays where it is, and the new dashboard opens without it.
- A build that is still running in the same chat keeps the chat on whichever dashboard attached it first.
  Make the dashboard after the build finishes if you want the chat on the built one.

## When there is no finding

When the agent ends without saving a finding, the turn reads "The question could not be answered".
It then says "The agent ended without writing a finding." and "Ask a narrower question, with the service and the time range."
**Try again** asks the same question again.

A run that finished but saved nothing reads "The run finished without a finding."
The other failures read as they do for a build; see [Read a turn](chat.md#read-a-turn).

## Investigate a past incident

An Admin can run an investigation as of a past time, to replay an incident that is over or to check a finding against what was known then.

```sh
observe investigate --as-of 2026-11-20T10:30:00Z --from now-1h --to now why did checkout errors rise
```

`--as-of <time>`, or `as_of` on `POST /api/v1/observe`, sets the run's now.
It is an RFC 3339 time such as `2026-11-20T10:30:00Z`, or epoch milliseconds.
Every relative time in the run resolves to it.
`--from now-1h --to now` is the hour before that time, and so is every `now` in a query the agent runs.

The agent is told the time, and the request and its finding carry it as `as_of`.
A retry or continue keeps it for an Admin, so an Editor who retries the run gets `403` `forbidden_role`.
Each turn of a chat passes `as_of` again; a later turn without it runs on the live clock.

These cases answer `400` `invalid_request`:

- a relative time;
- a time in the future;
- a `--to` later than it;
- a run that builds or revises a dashboard.

Anyone below Admin gets `403` `forbidden_role`.
`as_of` is for an investigation and for an ask about a dashboard or an observation.

## Who can investigate

Starting an investigation needs the Editor role or above, and view access on the dashboard it is about.
Viewers can read the observations in the folders they can view.

Over the API, `POST /api/v1/observe` with `kind` `investigate` from a Viewer or Commenter answers `403` `forbidden_role` with `needed_role` `editor`.

**Ask** in a panel's menu, the panel toolbar and the panel's header, and [Ask mode](ask-and-comment.md#ask-mode) in the bottom tool bar, ask a question as a private turn of your chat.
They do not start an investigation, so they need only Comment on the dashboard and a connected model.
A Viewer does not see them.

## Reference

### What a finding is

A finding is the agent's answer, written as Markdown of at most 400 words.
It carries up to four evidence panels, each a panel whose query the server ran and checked before the finding was saved.
It covers one window of time, shown under the answer with when it was found.

A finding starts with one plain sentence, names the window it used, and says what it did not check.
When your request names a length or a form, such as two sentences or a list, the text is exactly that, with no extra evidence prose and no link to the page you asked on.
A read that was refused, failed or empty still gets one short clause, so a short answer stays correct.
The evidence panels are not part of that limit and still come with an answer that gives counts.
When two pods or services tie, it leads with the one with the most errors.

The answer is saved into an observation, as a text block with the conclusion and one panel block for each evidence panel.
Its address is the observation's, `/o/<id>`, which you can share with anyone who can read the observation.
The observation is in a folder and follows that folder's access; see [Observations](observations.md#who-can-do-what).

An investigation about a dashboard files its observation in that dashboard's folder when you can edit it.
Otherwise it files it in your My Dashboards, or in General when you have none.
It stays there when the dashboard moves.

### What the agent reads

- It reads your datasources the way it does for a build.
- It reads the [annotations](annotations.md) of the dashboard and the global ones, so it can see a deploy or a change beside a spike.
  It cites the ones that fit with their times.
- It reads earlier [observations](observations.md) in the folders you can open, as you.
  When one bears on the question, it cites it by title and `/o/<id>` link and says its numbers are from an earlier time.

### MCP sources

An MCP source is an MCP server an Admin registered, such as a documentation index or a ticket system; see [MCP sources](../operate/mcp-sources.md).
In an investigation or a question the agent can list the sources you may use and call the tools an Admin approved.

- It cannot call a tool that is not approved, changed since it was approved, or belongs to a source set to Admins only when you are not an Admin.
- A call counts toward the run's call limit, stops after 20 seconds and shows as a step.
- What comes back is text from a server outside SlashObserve.
  The agent treats it as information and not as an instruction, and it is never shown as an evidence panel.
  The finding may quote it and say where it came from.
- A source that is down gives the agent a refusal, and the investigation goes on without it.

### Time zones

The web app sends the time zone you read times in.
That is the dashboard's zone, or the one in its time zone box, on a dashboard, and your browser's elsewhere.
The agent writes every time in the finding in that zone with its label, such as 19:52 IST.
The agent is also told the current time, in that zone or in UTC when none is known, and asked to state the window it queried.
A finding that labels its times UTC instead is sent back to the agent to convert.

Over the CLI and MCP no zone is known, so times are written in UTC with the word UTC.
An API caller can name one with `time_zone`.

### Suggested change and observe-worthy

An answer about a dashboard may also carry `suggested_change`, one plain sentence describing the change the person asked for, or the nearest safe one when that change is unsafe or impossible.
The agent does not make the change itself.
Nothing applies it until someone starts a revision with it.
For an answer given in a dashboard thread, someone can apply it from that thread with `POST /threads/{id}/apply`, `apply_suggested_change` or `observe comments apply`.
That posts "Revise the dashboard: <suggested change>" there and starts the revision as them.

An answer you can read, such as one in your chat on the dashboard, is also applied as a revision of its dashboard.
The revision takes the suggested change as its prompt, with `kind` `revise` and `applies_finding_id` on `POST /api/v1/observe`, `start_run` or `observe --revise --dashboard <id> --apply <finding-id>`.
A second apply of the same answer while the first is queued or running, or after it succeeded, returns that run with `replayed` true when it is yours.
It answers `already_applied` when the run is someone else's.

`observe_worthy` is true when the agent judged that the answer shows a jump, a drop or an error spike worth keeping as an observation.
It saves nothing by itself.

Both are fields of the run's `finding` in `GET /observe/{id}`, `get_run`, `get_chat` and `observe status --json`.

### Structured fields

An investigation's answer may also carry a few fields that sum it up, all optional.
The agent fills them when it saves an investigation, and they must agree with the Markdown answer.

- `causes` lists up to 6 causes, most confident first; see [Causes](#causes) below.
- `root_cause` has a `component`, a `kind` and a `summary`: the first stated cause that is not a knock-on, kept for readers of a single cause, and null when there is none.
  The `kind` is one of `deploy`, `config_or_flag`, `dependency`, `resource`, `data`, `traffic`, `infrastructure` or `unknown`.
  A save that sends only `root_cause` is stored as a one-cause list, and a save that sends both is refused unless `root_cause` names the component and kind of the first cause.
- `onset` is the absolute time the problem began, in RFC 3339.
- `impact` is one or two sentences with numbers.
- `confidence` is `high`, `medium` or `low`.
- `ruled_out` lists at most 5 causes that were checked and excluded, and `next_steps` lists at most 3 things to do.
- `no_incident` is true when nothing was wrong.
  It cannot be set together with `root_cause` or `causes`.
  In an incident investigation whose preflight brief listed candidates, it is accepted only when every candidate was closed as background, transient or unrelated; see [Closing every candidate](../concepts/investigate.md#closing-every-candidate).

A component is at most 120 characters, a sentence at most 300 and a list line at most 200, each on one line.
A save that breaks a rule is refused with the reason, and the agent fixes it and saves again.
The agent is offered these fields only in an investigation.
A save outside an investigation that sets any of them is still saved, without them, and its result says they were dropped.

The observation shows the fields as a compact summary above the answer, and shows nothing extra when none is set.
They are fields of the run's `finding` in `GET /observe/{id}`, `get_run`, `get_chat` and `observe status --json`.

#### Causes

Each entry of `causes` has:

- `component`: the `kind` (`service`, `pod`, `deployment`, `config`, `flag`, `datastore`, `dependency`, `node` or `other`), the `name` as the telemetry or the system spells it, and an optional `namespace`.
- `kind`: the same kinds as `root_cause`.
- `mechanism`: one sentence on how the component failed, with the changed value.
- `trigger`: optional, the change that set it off, with a `kind` (`feature_flag`, `config_value`, `deploy`, `traffic`, `dependency` or `unknown`), a `key` exactly as the system spells it, such as a flag key, a setting or a version, and an optional `value`.
- `onset`: optional, when this cause began, in RFC 3339.
- `affected`: up to 8 components it affects, in the same form as `component`.
- `relation` and `knock_on_of`: `knock_on` with the index of the listed cause it follows, or `independent`; a knock-on must follow an independent cause, never itself.
- `evidence`: the `facts` (up to 12 fact ids of the run) and `trace_ids` (up to 4) the agent cited, and the `queries` those facts came from.
- `estimate`: the agent's own `high`, `medium` or `low`.
- `confidence`, `band` and `stated`: the server's score from 0 to 1, its band and whether the finding states the cause; see [Causes and confidence](../concepts/investigate.md#causes-and-confidence).

The observation's answer block ends with a Causes section listing each stated cause with its band, mechanism, trigger and onset, and the unconfirmed ones under their own heading; the summary above the answer lists them the same way when there is more than one or any is scored.
`observe investigate` prints the root cause line, then each other stated cause and each unconfirmed one on a line of its own.
A finding saved before causes existed reads its `root_cause` as its only cause, stated and unscored.
`GET /observations/{id}` and `get_observation` return them as `findings`, one entry per saved answer with its `run_id`, and only for answers the caller can see.
`observe investigate` and `observe observations show` print them as lines before the answer.
Search finds an observation by the name of its root cause component.

### CLI flags for investigate

| Flag | Meaning |
| --- | --- |
| `--dashboard <id>` | The dashboard the question is about. |
| `--panel <panel-id>` | One panel of `--dashboard`. |
| `--observation <id>` | The observation to write into; without it a new observation is made. |
| `--at <anchor>` | The spot the question is about; needs `--dashboard` or `--observation`. |
| `--block <id>` | A block of `--observation` the question is about. |
| `--from <time>` and `--to <time>` | The window to look at, such as `now-6h` and `now`; they go together. |
| `--as-of <time>` | Admin only: the time to investigate as of. |
| `--private` | Answer privately about `--dashboard` or `--observation`; needs exactly one of them. |
| `--no-wait` | Print the request id and exit without waiting. |
| `--timeout <duration>` | Stop waiting after this long; the investigation keeps running on the server. |
| `--open` | Open the finding in the browser once it is saved. |

Details of the flags:

- `--at` takes the anchor JSON object or a short form such as `pos:0.4,0.3`.
- `--block` comes from `observe o show`; with `--at`, it is the block `--at` points into.
- `--as-of` takes epoch milliseconds or an RFC 3339 time in the past, and `--to` must not be later.
- `--panel` needs `--dashboard`, and `--block` needs `--observation`.

### Deprecated finding routes

The `/api/v1/findings` routes that remain are deprecated adapters over observations: list, read, pin, make a dashboard and panel data.
Use the observation's own routes.
The finding tools and commands are gone.
