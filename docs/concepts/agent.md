---
title: How the agent builds a dashboard
description: The tool loop the built-in agent runs, what a save checks, and whose permissions it uses.
---

The built-in agent turns a prompt into a saved dashboard version by calling tools against your datasources.
It writes no dashboard text of its own.
The only thing a run produces is a complete [ObserveSpec](observespec.md) document saved through a tool.
The agent is on only when an LLM provider is configured (see [LLM providers](../connect/llm-providers.md)).

A run starts from a prompt in the chat home, from `observe <prompt>`, from a follow-up on an existing dashboard, or from a comment that asks for a revision.
The conversation page shows the steps the tools report and the result, never the model's own text.

## The tool loop

The model starts with the prompt, the configured datasources and whether each is healthy, the authoring guide, the JSON Schema and an example.
For a revision it also gets the current version of the dashboard.
It then works in this order:

1. **Discover.**
   `list_metrics`, `list_labels` and `list_label_values` find real metric names, label names and label values on Prometheus and on the OpenTelemetry tables of ClickHouse.
   `get_schema` and `sample_rows` list the tables and columns of a ClickHouse datasource and read a few rows.
   The model calls independent discovery tools together in one turn, and uses only names it found in this run or that are already in the dashboard.
2. **Draft** the panels.
3. **Lint.**
   `lint_query` parses each new or changed query and reports syntax and policy problems without running it.
4. **Run.**
   `query_range` and `query_instant` run a query and return a compact summary, so the model can check a query returns data.
   These ad hoc queries run with tighter limits than a saved dashboard (see [Datasources file](../reference/datasources-file.md)).
5. **Save.**
   `create_dashboard` saves a new dashboard and `update_dashboard` saves a new version of an existing one, each with a full document.
   When the prompt contains Grafana dashboard JSON, the model calls `import_grafana_dashboard` instead of rewriting it, and the agent may import only JSON that the prompt contains.
6. **Repair**, if the save is rejected.

A successful save ends the run.
A run makes at most 20 model calls, runs one save per turn and is stopped after 3 minutes.
On its last model call only a save can run.
A run that ends without an accepted save stores nothing.

## What a save checks

Before it stores a version, the server runs the queries itself, whatever the model checked earlier.
Besides validating the document, it runs every query of every panel, hidden queries included, over the dashboard's default time range and with each variable at its default, and it runs every query variable.

The save is rejected when:

- a query fails, or a datasource uid is not configured.
- none of the queries of a panel returns data, except on an `alertlist` panel, where no firing alerts is normal.
- the document breaks a rule of the format, such as overlapping panels or an undeclared variable.

The rejection lists each problem with the path of the panel or variable and a code.
The model fixes only the panels it names and saves again.
A run makes the first save plus up to three repairs, so at most four saves.
If the third repair is also rejected, the run ends with an error that names the last issue codes.

When the dashboard changed while the agent was revising it, the run restarts once from the latest version.
If it changes again, the run fails.

A revision starts from the current document and keeps panel ids, positions and everything the prompt does not ask to change, so [comments](versions.md#comments) stay attached to their panels.
A comment on a panel asks to change that panel unless it says otherwise.
The save tools reject a document that creates or edits a placeholder panel, and the model is told not to delete an unverified panel or its queries to make a save pass (see [Import Grafana dashboards](../use/import-grafana.md)).

## Whose permissions it uses

The agent acts as the person or key that asked.
The server checks the request when it is queued and again when a worker starts it.
A new dashboard needs the Editor role, and a follow-up or revision needs Edit on the dashboard's folder.
A new dashboard goes to the folder the requester last created a dashboard in, else General (see [Folders and access](../teams/folders.md)).

The saved version records the requester as its author and the request that asked for it, and the version menu on the dashboard shows "Agent" as the one who saved it.
The tool-level roles do not apply inside a run, because the requester was already checked.

The built-in agent has 15 of the 19 tools: the 11 above, plus `list_datasources`, `list_dashboards`, `get_dashboard` and `get_spec_guide`.
The comment tools and `list_folders` are for MCP clients only, so the agent never writes or resolves a comment reply.
When a revision request comes from a comment, the server resolves that thread itself once a version is saved.

The agent sends what its tools return to the LLM provider, including names, label values, sampled rows and query results (see [LLM providers](../connect/llm-providers.md)).

## MCP clients use their own model

An MCP client such as Claude Code runs the loop itself, with its own model, against the same tools at `<public url>/mcp`.
The server needs no LLM provider for that.
Each call is checked against the role of the key's service account and the folders it can see, and a save goes through the same validation and query check as above.
The client can also read and answer comments and pick a folder, which the built-in agent cannot.
A version saved this way records `mcp` as its origin and the client's name.
See [MCP](../use/mcp.md) for the tools and the setup.
