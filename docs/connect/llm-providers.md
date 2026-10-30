---
title: LLM providers
description: Turn the built-in agent on with Anthropic or an OpenAI-compatible server.
---

The agent turns a prompt into a saved dashboard by calling tools against your datasources.
It is on only when `SLASHOBSERVE_LLM_PROVIDER` is set.
With it unset the server starts normally, and:

- `POST /api/v1/observe` and revision prompts answer `503 agent_unavailable`.
- Comments are saved but do not queue a revision, and turning the agent on later does not replay old comments.
- You can still create and edit dashboards through the API or through [MCP](../use/mcp.md) with an agent of your own.

The agent sends what its tools return to the provider: datasource and metric names, label values, sampled rows and query results.
Choose a provider whose data handling fits what your datasources contain.

## Anthropic

Anthropic is the default model family.
Only the key is required:

```text
SLASHOBSERVE_LLM_PROVIDER=anthropic
SLASHOBSERVE_LLM_API_KEY=<your Anthropic API key>
```

The default model is `claude-sonnet-5-5`.
Set `SLASHOBSERVE_LLM_MODEL` to use another, for example `claude-opus-5-5` for harder prompts or `claude-haiku-5-5` for lower cost.
The server does not validate the model name itself.
`SLASHOBSERVE_LLM_EFFORT` sets the model's effort level and applies to this provider only.
To read the key from a file, such as a Docker or Kubernetes secret, use `SLASHOBSERVE_LLM_API_KEY_FILE` instead.

## OpenAI-compatible

`openai` talks to any server that implements the Chat Completions API with tool calling.
The model is required, because there is no safe default:

```text
SLASHOBSERVE_LLM_PROVIDER=openai
SLASHOBSERVE_LLM_MODEL=<model name>
SLASHOBSERVE_LLM_API_KEY=<API key>
```

Leave `SLASHOBSERVE_LLM_BASE_URL` unset to use OpenAI.
Point it at another server to use that instead.
For a local Ollama:

```text
SLASHOBSERVE_LLM_PROVIDER=openai
SLASHOBSERVE_LLM_MODEL=llama3.1
SLASHOBSERVE_LLM_BASE_URL=http://localhost:11434/v1
```

The key is optional for servers that need none, as Ollama does.
The model must support tool calling.
Inside a container, `localhost` is the container itself; use an address the container can reach, such as `http://host.docker.internal:11434/v1` on Docker Desktop.
The project does not vouch for any particular local model, and dashboard quality depends heavily on the model.
