---
title: Local models
description: Run the agent on a model served on your own machine or network through an OpenAI-compatible server such as Ollama, and what to expect from a small model.
---

The agent can run on a model you serve yourself, so prompts and query results never leave your network.
It talks to the model through the OpenAI-compatible adapter, which calls any server that implements the Chat Completions API with tool calling.
Ollama, vLLM, llama.cpp's server and LM Studio are examples; the project does not vouch for any of them or for any model.

## Settings

```text
SLASHOBSERVE_LLM_PROVIDER=openai
SLASHOBSERVE_LLM_BASE_URL=http://localhost:11434/v1
SLASHOBSERVE_LLM_MODEL=<model name>
```

- `SLASHOBSERVE_LLM_PROVIDER` is `openai` for every OpenAI-compatible server, not only OpenAI's own.
- `SLASHOBSERVE_LLM_BASE_URL` is the server's API root, the part before `/chat/completions`, such as `http://localhost:11434/v1` for Ollama.
  It is an absolute `http` or `https` URL and may have a path.
- `SLASHOBSERVE_LLM_MODEL` is required, because there is no safe default; use the name the server lists, such as the name you pulled into Ollama.
- `SLASHOBSERVE_LLM_API_KEY` stays unset for a server that needs no key, as Ollama does.

These calls are not covered by the [outbound allowlist](../operate/outbound-access.md), so the server needs a network path to the model server and nothing else.

## Tool calling

The model must support tool calling.
The agent works only through tools: it reads metrics, labels and rows with them and saves every panel with one, and its plain text is discarded.
A model that answers in text instead of calling a tool saves nothing, and the request fails with `no_tool_call` or `no_progress` (see [Read a turn](../use/chat.md#read-a-turn)).
Check that the model you pick is listed as tool-capable by its server, and that the server is started with tool calling turned on when it has such a switch.

The adapter accepts a tool call whose `type` is missing, which some servers leave out, and reads a call with empty arguments as `{}`.

## Effort

`SLASHOBSERVE_LLM_EFFORT` is sent as the reasoning effort only when it is set: `none`, `minimal`, `low`, `medium`, `high` or `xhigh`.
Unset, nothing is sent and the server uses its own default, which is what most local servers expect.
The first call of a new build runs one step higher (`low` becomes `medium`, `medium` becomes `high`).
A server that does not accept the parameter answers every call with a provider error; unset the variable if that happens.

## Run it with the demo

From the demo stack, `localhost` is the server container itself, so point it at the host's side of Docker:

```sh
export SLASHOBSERVE_LLM_PROVIDER=openai
export SLASHOBSERVE_LLM_BASE_URL=http://host.docker.internal:11434/v1
export SLASHOBSERVE_LLM_MODEL=<model name>
make demo
```

`host.docker.internal` reaches the host on Docker Desktop.
On a Linux Docker engine, use the host's address on the Docker bridge, such as `http://172.17.0.1:11434/v1`, and make the model server listen on it.
The demo forwards `SLASHOBSERVE_LLM_PROVIDER`, `SLASHOBSERVE_LLM_MODEL`, `SLASHOBSERVE_LLM_API_KEY`, `SLASHOBSERVE_LLM_BASE_URL` and `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS` from your shell, so `SLASHOBSERVE_LLM_EFFORT` needs a line in the server's `environment` in `deploy/compose/compose.yaml`.
Run `make demo` again after changing them.

## What to expect from a small model

Dashboard quality depends heavily on the model, and small models are much weaker at it than large hosted ones.
Expect them to call tools with arguments that fail the checks, to stop before every planned panel is saved, or to answer in text instead of calling a tool.
The server checks every save by running its queries, so a weak model produces fewer panels or a failed request, never a dashboard whose queries do not run.

A few settings help a slow or small model:

- Give the model the largest context window the server allows, because the agent's instructions, tool definitions and tool results all go into it; with Ollama, set `OLLAMA_CONTEXT_LENGTH` before you start it.
- `SLASHOBSERVE_LLM_IDLE_TIMEOUT`, 180 seconds by default, retries a call that receives nothing from the server for that long; raise it for a model that takes longer than that before its first token.
- `SLASHOBSERVE_LLM_REQUEST_TIMEOUT`, 10 minutes by default, bounds each call with its retries.
- `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS=true` adds the server's own error message to the log line of a failed call, which shows why a server refused a request.

Name the metric, service or table in the prompt, so the model has less to find.
[Configuration](../reference/configuration.md#llm) lists every LLM setting, and [LLM providers](llm-providers.md) covers Anthropic and hosted OpenAI-compatible servers.
