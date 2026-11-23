---
title: What is SlashObserve
description: What SlashObserve does, who it is for, how to try it in minutes, and what you run to host it.
---

SlashObserve is an open-source workspace for dashboards and investigations.
You ask a question in plain language, and a built-in agent builds a dashboard that has been checked against your data.
It reads the monitoring and logging systems you already run: Prometheus, ClickHouse, Loki, Tempo, Jaeger, Elasticsearch and OpenSearch.

## Who it is for

SlashObserve is for a team that runs one or more of those systems and wants to:

- get a dashboard by describing it, then change it by talking to the agent;
- investigate an incident together, and keep what the team learns on a page;
- let an AI agent such as Claude Code work in the same workspace, through MCP or the CLI;
- see who or what changed each dashboard, with every change saved as a version;
- move dashboards over from Grafana.

It does not send alerts, and it reads only the seven datasource types above; [Known limitations](known-limitations.md) lists what to check before you adopt it.

## See it work

Type `/observe checkout health by pod, last 6h` and the agent builds a dashboard.
You refine it in plain language.
Outside agents connect over MCP and act as the person who connected them.

To try it on your own machine with sample data, follow the [quick start](quick-start.md).
It needs Docker, git and a model API key.
To import dashboards you already have, see [Switch from Grafana](../use/switch-from-grafana.md).

## Ask the agent where you are working

The agent answers a question without changing the dashboard until you choose **Revise the dashboard**.
You can ask in three places:

- In a comment thread, by mentioning `@observe` or addressing the message to it, so the team reads the answer ([Comments, agent requests and versions](../use/review.md#ask-the-agent-from-a-comment)).
- In a private chat in the dashboard's side pane, which only you and Admins read ([Chat and follow-ups](../use/chat.md#in-the-dashboards-chat-tab)).
- By pointing at a spot on a panel in Ask mode, or by commenting on that spot in Comment mode.
  A spot is a point, a range, a row or a value ([Ask and comment on a dashboard](../use/ask-and-comment.md)).

## What you run

SlashObserve is one container plus one Postgres database.

- The container holds the Go server with the web UI embedded.
  It also holds `observe-agent`, the built-in agent, as a second binary that the server starts and supervises whenever a model provider is configured.
- Postgres holds everything the server owns:
  - dashboards and their versions, observations and proposals, and comment threads;
  - the activity log, the inbox's read position and saved searches;
  - folders and their access lists, users and sessions, service accounts and key hashes, and connected agents;
  - webhooks and the agent request queue.
- Your Prometheus, ClickHouse, Loki, Tempo, Jaeger, Elasticsearch and OpenSearch servers stay where they are.
  SlashObserve only reads from them.

You also need:

- An OpenID Connect (OIDC) identity provider, so people can sign in with a browser.
  The demo stack uses Dex, a development-only provider.
  Without one, only service-account keys can sign in; see [Run with no browser sign-in](../operate/service-account-keys.md#run-with-no-browser-sign-in).
- Optionally, an LLM provider.
  Without one the server runs, but the built-in agent is off; see [LLM providers](../connect/llm-providers.md).

The image is published as `ghcr.io/slashobserve/slashobserve`; see [Run your own deployment](install.md).
