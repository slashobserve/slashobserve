---
name: observe
description: Use for work in SlashObserve, the observability workspace - building or changing a dashboard, investigating why a metric, log or trace changed, searching logs or traces, answering comments on dashboards, observations and annotations, or doing a run assigned to you - through the slashobserve MCP tools or the observe CLI.
---

# /observe

SlashObserve keeps a team's dashboards, observations, comment threads and saved log and trace searches, checks every save by running its queries, and records who made each change in one activity log.
A client connected through OAuth acts as the signed-in person, with that person's role and folder access, re-read on every call, and a key acts as its service account with that account's role.
Your changes are recorded under that name followed by your client's name, as "<Name> (via <client>)".
Service-account keys are for headless bots; an agent working for a person connects through OAuth and acts as that person.

Do what the person asked: the words after /observe, or their message.
First pick the kind of work - build, change, investigate, search logs or traces, answer comments, or take assigned runs - and say in one line which you picked.

## Start from what exists

Search before you build: search_workspace finds dashboards, observations, annotations, comments and saved searches by their words.
A dashboard or observation that already answers the ask is the answer: open it with get_dashboard or get_observation and say so.
list_activity shows what changed recently and who changed it.
list_inbox shows what your person is asked to do: proposals to review, observation blocks waiting for them, mentions, replies and their runs that failed.
It works only for a person's connection, never for a key.

## Build a dashboard

1. Call get_spec_guide once before your first save; every save is checked against its guide and JSON Schema.
2. Take a datasource uid from list_datasources and find real names with list_metrics, list_labels, list_label_values, get_schema and sample_rows; never guess a metric, label or column.
3. Check each query with lint_query and run it with query_range, or with query_instant for an instant query or a table.
4. Save with create_dashboard; list_folders gives a folder_id, and create_folder makes a new folder.
   A save rejects the panels whose queries fail or return no data and lists them in failing_panels: fix only those and save again.

## Change a dashboard

Read it with get_dashboard, or only the panels you need with get_panels, and send just the change with update_panels.
Pass the version you read as parent_version and a one-line summary of the change; both are required, and so are parent_version and dashboard_id for restore_dashboard_version.
A save on an older version merges when it changed other panels; when it changed the same ones it answers version_conflict with the conflicts, so read the latest version and apply your change again.
A save you may not make directly becomes a proposal for an Editor, and so does every save while your connection is in propose mode, which only your person can set: the result says proposed and is a success, not an error.
Tell the person the proposal waits for review.
While your connection is in propose mode, a save that removes panels or variables is refused as proposal_required and names them: ask the person to remove them.
In direct mode update_panels with delete, or a spec without a panel, removes it as a new version, so delete only what the person asked you to, and restore_dashboard_version brings the old version back.
Never delete a panel the dashboard lists as unverified unless the person asked.
list_dashboard_health names the panels that fail; fix them with update_panels.
A finding from a run made on Home may carry ask_on, the dashboard_id and title of the one dashboard the person meant to change: read it with get_run, then change that dashboard with update_panels, or ask SlashObserve's own agent there with start_run, sending kind revise, ask_on.dashboard_id as dashboard_id and the run's prompt.

## Review proposals

list_proposals shows the proposals on a dashboard, and get_proposal reads one with its changes and the conflicts a merge would hit.
merge_proposal needs the revision you read and, for each conflict, ours (the dashboard's) or theirs (the proposal's); it needs Edit, and is refused while your connection is in propose mode.
discard_proposal closes one: its author may, and so may an Editor.
Merge or discard only what your person asked you to.

## Investigate a question

Find the signal with the discovery tools and query_range, and look for causes beside it: list_annotations for deploys and other markers, list_activity for dashboard changes and agent runs.
A filter that returns nothing matched nothing: check the values it filters on before you say there were no errors (a ClickHouse span status is `Error`), and make the first sentence of the answer match the numbers under it.
Read earlier observations with list_observations and get_observation first, and cite the one you used by its title and /o/<id> link.
Save the answer in an observation: create_observation files one in a folder (list_folders gives a folder_id), and add_block adds a text block (Markdown, no HTML), a panel block whose query returns data in its range, or a snapshot that freezes a panel block.
Put a one-sentence answer first, then the evidence.
No tool queues an investigation on the server; you do the work with your own model.

## Keep an observation

get_observation shows each block's kind, state and version, and list_observations lists the open or resolved ones.
Send a block's version to update_block or keep_block, and a stale one answers version_conflict.
arrange_blocks sets the order, width (a third, half, two thirds or the full page) and height of the blocks in one call without versions, and a person who can only comment cannot use it.
A person who can only comment gets a requested block, which an editor keeps with keep_block or discards with discard_block; it is not live until kept.
resolve_observation closes an observation with a one-line summary of what was found.
publish_rca freezes it as an RCA revision without query text; it never makes a public link, because a person does that in the web app.
pin_observation pins an observation to a dashboard as an annotation, and make_dashboard turns its panel blocks into a dashboard in My Dashboards; both can be repeated safely.

## Search logs and traces

draft_query turns a question into a checked query for one datasource, PromQL on a Prometheus datasource; run it with query_instant.
For logs, log_patterns groups lines, log_facets counts the values of fields and log_context reads the lines around one line.
For traces, get_trace reads one trace, trace_stats summarizes a filter, compare_traces finds what sets some traces apart and logs_for_trace reads the log lines of a trace or span.
save_search keeps a logs, traces or metrics search the team will reuse and refuses a query with a lint_query error; take the datasource from list_datasources as datasource, and draft_query writes a query.
list_saved_searches lists the saved ones and update_saved_search changes the query, filters or range of one; no tool renames or deletes one.

## Read the audit trail

Admins read who did what with search_audit_events, get_audit_event, list_audit_facets, get_audit_histogram, aggregate_audit_events and list_audit_fields, which take KQL, `lucene:` or a Query DSL body.
The audit trail is also the datasource `slashobserve-audit`, which list_datasources shows only to Admins: query it with query_instant and in panels like an OpenSearch datasource, and read get_spec_guide for its fields.
A dashboard or observation that reads it must live in a folder only Admins can read, such as Audit, which starts with four dashboards to read or copy.

## Answer comments

list_comments reads the threads on a dashboard, an observation, an annotation or a proposal.
Fix what a comment asks, then reply_to_comment with your answer and resolve_comment with the version you saved; only resolve_comment takes a version.
create_comment opens a thread on any of them, edit_comment changes a comment you wrote and reopen_comment reopens a resolved thread.
A comment that mentions @observe addresses SlashObserve's own agent, and the mention is recorded in the activity log; a comment is for everyone and by default starts no run.
Set to: agent on create_comment or reply_to_comment to ask the one agent the text mentions, else the thread's agent, else @observe: one of your own connected agents or a service account that takes runs, written as the token the person gave you.
Give each comment a client_id you make, so a retry after a timeout returns the comment already posted instead of a second one.
A thread on a dashboard or an observation starts a read-only ask that answers in the thread and changes nothing; the result's run_refusal says why none started.
A run you claim that a message started carries thread_context, the earlier messages of that thread, oldest first: read it before you answer.
The server posts the run's status in the thread when it ends, so do not reply there to say you are done.

## Take assigned runs

A run reaches you only when a comment mentions you and starts one, so nothing arrives unless you are an agent your person connected over OAuth or a service account that takes runs.
Poll with claim_run while you are idle; it waits up to 30 seconds and answers no run when none is assigned, and then you call it again.
A claimed run has its prompt, its target, a session url and token, the kind (build or investigate) and the person it acts for.
1. Read the observe prompt with that kind: it is the system text for the run, and get_run_brief adds the first message of a model session.
2. Open a second MCP connection to the session url with its token as the Bearer, and work with the tools it lists: they act for the person who asked and carry the same guards as SlashObserve's own agent.
3. Call report_run at least every 30 seconds with the run's id and attempt, optionally with one line saying what you do; the lease ends 60 seconds after the last call, and stop true means the run was canceled or has ended, so stop and call finish_run.
4. Call finish_run with outcome succeeded after you saved the result, or failed with a message.
   A run that saved a version, a proposal or a finding ends succeeded whatever you send.
   A question's finding stays in the chat; call save_run_answer with the run id to file it as an observation.
A call with a run that is not yours is refused, and a connection in propose mode turns the run's dashboard saves into a proposal.

## Limits

Lists show only what you may see, and not_found can mean you may not see it.
Every refused tool call answers with `code`, a stable word to branch on, and `message`, what to change; some add fields such as issues, latest_version or allowed_actions.
Moves, duplicates and restores of a dashboard have tools, but no tool deletes a dashboard, renames or deletes a saved search, makes a public link, or manages people, keys, who can see a folder, webhooks or connected agents: tell the person to do it in the web app.
create_folder and rename_folder make and rename folders.
start_run, cancel_run, retry_run and get_run_build hand work to the built-in agent and follow it; use them only when the person asks, because each start spends the server's model and is not safe to repeat without a client_id.
list_runs finds the person's earlier runs, which the web app calls chats, by state, dashboard or a word of the title or answer; rename_run, pin_run, unpin_run and archive_run tidy them, only for runs the person started.
Every run is a turn of a chat: list_chats and get_chat read the person's chats, start_run with chat_id asks the next turn, and a fresh client_id per question makes a resend safe.
Investigations are a person's decision, and run_dashboard_health is for Admins.
Never write HTML into a text panel, create a placeholder panel, or add or change actions; the server refuses them.

## With only a shell

Without the MCP tools, use the observe CLI after `observe login`:
`observe <ask>` asks SlashObserve's own agent to build or change a dashboard, `observe investigate <question>` investigates, `observe search <words>` searches the workspace, `observe logs` and `observe traces` query a datasource, `observe o new`, `observe o add`, `observe o list` and `observe o resolve` keep observations, `observe proposals list` and `observe proposals merge` review proposals, `observe saved-searches list`, `observe saved-searches save` and `observe saved-searches update` keep searches, `observe datasources list`, `observe datasources metrics`, `observe datasources labels`, `observe datasources label-values`, `observe datasources services`, `observe datasources schema` and `observe datasources sample` find the real names and values a query needs, `observe dashboards export` and `observe dashboards apply --dir <directory>` keep dashboards as Spec files (apply takes a directory, never a single file), `observe dashboards schema` and `observe dashboards sample` print the JSON Schema and a complete example of one, `observe audit search` reads the audit trail for an Admin, `observe comments list` reads threads, `observe comments add --to agent` hands a run to an agent a comment mentions and `observe activity` lists recent changes.
