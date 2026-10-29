---
title: What is SlashObserve
description: What SlashObserve does and what you run to host it.
---

SlashObserve turns a prompt into a live, Grafana-style dashboard.
Type `/observe checkout health by pod, last 6h`, get a dashboard backed by your Prometheus and ClickHouse data, and refine it in plain language.

## What you run

SlashObserve is one container plus one Postgres database.
The container is a single Go binary with the web UI embedded; it has no other runtime dependencies.
Postgres holds everything the server owns: dashboards and their versions, folders and their access lists, comments, users and sessions, service accounts and key hashes, and the agent request queue.
Your Prometheus and ClickHouse servers stay where they are; SlashObserve only reads from them.

You also need:

- An OpenID Connect (OIDC) identity provider for sign-in.
  The demo stack uses Dex, a development-only provider.
- Optionally, an LLM provider.
  Without one the server runs, but the built-in agent is off (see [LLM providers](../connect/llm-providers.md)).

This repository does not reference a published container image.
Build the image from the repository root (see [Run your own deployment](install.md)).
