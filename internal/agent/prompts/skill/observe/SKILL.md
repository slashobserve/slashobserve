---
name: observe
description: Use for work in SlashObserve, the observability workspace - building or changing a dashboard, investigating why a metric, log or trace changed, searching logs or traces, or answering comments on dashboards, findings and annotations - through the slashobserve MCP tools or the observe CLI.
---

# /observe

SlashObserve keeps a team's dashboards, findings, comment threads and saved log and trace searches, checks every save by running its queries, and records who made each change in one activity log.
A client connected through OAuth acts as the signed-in person, with that person's role and folder access, re-read on every call, and a key acts as its service account with that account's role.
Your changes are recorded under that name followed by your client's name, as "<Name> (via <client>)".
Service-account keys are for headless bots; an agent working for a person connects through OAuth and acts as that person.

Do what the person asked: the words after /observe, or their message.
First pick the kind of work - build, change, investigate, search logs or traces, or answer comments - and say in one line which you picked.

## Start from what exists

Search before you build: search_workspace finds dashboards, findings, annotations, comments and saved searches by their words.
A dashboard or finding that already answers the ask is the answer: open it with get_dashboard or list_findings and say so.
list_activity shows what changed recently and who changed it.

## Build a dashboard

1. Call get_spec_guide once before your first save; every save is checked against its guide and JSON Schema.
2. Take a datasource uid from list_datasources and find real names with list_metrics, list_labels, list_label_values, get_schema and sample_rows; never guess a metric, label or column.
3. Check each query with lint_query and run it with query_range, or with query_instant for an instant query or a table.
4. Save with create_dashboard; list_folders gives a folder_id.
   A save rejects the panels whose queries fail or return no data and lists them in failing_panels: fix only those and save again.

## Change a dashboard

Read it with get_dashboard, or only the panels you need with get_panels, and send just the change with update_panels.
Pass the version you read as parent_version.
A save on an older version merges when it changed other panels; when it changed the same ones it answers version_conflict with the conflicts, so read the latest version and apply your change again.
A save you may not make directly becomes a proposal for an Editor: the result says proposed, and list_proposals and get_proposal show it.
Never delete a panel the dashboard lists as unverified unless the person asked.
list_dashboard_health names the panels that fail; fix them with update_panels.

## Investigate a question

Find the signal with the discovery tools and query_range, and look for causes beside it: list_annotations for deploys and other markers, list_activity for dashboard changes and agent runs.
Save the answer with create_finding: at most 400 words, with up to four evidence panels whose queries return data.
A finding without a dashboard is filed in a folder; move_finding moves it into one the team can see.
No tool queues an investigation on the server; you do the work with your own model.

## Search logs and traces

draft_query turns a question into a checked query for one datasource; run it with query_instant.
For logs, log_patterns groups lines, log_facets counts the values of fields and log_context reads the lines around one line.
For traces, get_trace reads one trace, trace_stats summarizes a filter, compare_traces finds what sets some traces apart and logs_for_trace reads the log lines of a trace or span.
save_search keeps a logs or traces search the team will reuse, after you check its query with lint_query or draft_query, and list_saved_searches lists the saved ones.

## Answer comments

list_comments reads the threads on a dashboard, a finding or an annotation.
Fix what a comment asks, then reply_to_comment and resolve_comment with the version you saved.
create_comment opens a thread on a dashboard, a finding or an annotation, edit_comment changes a comment you wrote and reopen_comment reopens a resolved thread.
A comment that mentions @observe addresses SlashObserve's own agent, and the mention is recorded in the activity log and by default starts no run.
Set start_runs on create_comment or reply_to_comment, with exactly one agent mentioned, to start a run for it: @observe, one of your own connected agents or a service account that takes runs, written as the token the person gave you.
A thread on a dashboard starts a comment run, a thread on an observation starts an investigation, and the result's run_refusal says why none started.

## Limits

Lists show only what you may see, and not_found can mean you may not see it.
No tool deletes, moves or duplicates a dashboard, renames or deletes a saved search, makes a public link, or manages people, keys, folders, webhooks or connected agents: tell the person to do it in the web app.
Never write HTML into a text panel, create a placeholder panel, or add or change actions; the server refuses them.

## With only a shell

Without the MCP tools, use the observe CLI after `observe login`:
`observe <ask>` asks SlashObserve's own agent to build or change a dashboard, `observe investigate <question>` investigates, `observe search <words>` searches the workspace, `observe logs` and `observe traces` query a datasource, `observe findings list` and `observe findings move` read and file findings, `observe saved-searches list` and `observe saved-searches save` keep searches, `observe comments list` reads threads and `observe activity` lists recent changes.
