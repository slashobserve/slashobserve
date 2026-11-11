---
title: What is SlashObserve
description: What SlashObserve does and what you run to host it.
---

SlashObserve is an open-source observability workspace where a team and its agents work questions together and keep what they learn, on the stores they already run: Prometheus, ClickHouse, Loki, Tempo, Elasticsearch and OpenSearch.
Type `/observe checkout health by pod, last 6h` and the built-in agent builds a verified dashboard that you refine in plain language.
Outside agents such as Claude Code connect over MCP and act as the person who connected them.

## What you run

SlashObserve is one container plus one Postgres database.
The container is a single Go binary with the web UI embedded; it has no other runtime dependencies.
Postgres holds everything the server owns: dashboards and their versions, findings, comment threads, the activity log, saved searches, folders and their access lists, users and sessions, service accounts and key hashes, connected agents, webhooks, and the agent request queue.
Your Prometheus, ClickHouse, Loki, Tempo, Elasticsearch and OpenSearch servers stay where they are; SlashObserve only reads from them.

You also need:

- An OpenID Connect (OIDC) identity provider for sign-in.
  The demo stack uses Dex, a development-only provider.
- Optionally, an LLM provider.
  Without one the server runs, but the built-in agent is off (see [LLM providers](../connect/llm-providers.md)).

The image is published as `ghcr.io/slashobserve/slashobserve`; see [Run your own deployment](install.md).
To try it first on your own machine with sample data, use the [quick start](quick-start.md).
