---
title: Local models
description: Run the agent on a model served on your own machine or network through an OpenAI-compatible server such as Ollama, and what to expect from a small model.
---

The built-in agent can use a model you serve yourself, so prompts and query results never leave your network.
Use it when your data must stay on your side, or when you want to try the agent without a hosted account.
Ollama, vLLM, llama.cpp's server and LM Studio all work if they implement the Chat Completions API with tool calling.
The project does not vouch for any of them or for any model.

## Connect a local model

1. Start your model server and load a model that supports tool calling.
   With Ollama, for example, pull the model and keep Ollama running.
2. Set these variables on the SlashObserve server, with your model's name, and restart it:

   ```text
   SLASHOBSERVE_LLM_PROVIDER=openai
   SLASHOBSERVE_LLM_BASE_URL=http://localhost:11434/v1
   SLASHOBSERVE_LLM_MODEL=<model name>
   ```

   Use the name your model server lists.
   Leave `SLASHOBSERVE_LLM_API_KEY` unset when the server needs no key, as Ollama does.
3. Check that the server sees the model:

   ```sh
   observe doctor
   ```

   The model check passes and shows the provider, the model and the effort.
   Without a model it fails with "no model is configured; prompts cannot run".
4. In the web app, open Home and use **Ask Observe Agent**, and name the metric, service or table you want so the model has less to find.

The model calls come from the `observe-agent` process the server starts, so the server's host must be able to reach the model server's address.
The [outbound allowlist](../operate/outbound-access.md) does not cover these calls, so the server needs a network path to the model server and nothing else.

## Run it with the demo

From the demo stack, `localhost` is the server container itself, so point the base URL at the host's side of Docker:

```sh
export SLASHOBSERVE_LLM_PROVIDER=openai
export SLASHOBSERVE_LLM_BASE_URL=http://host.docker.internal:11434/v1
export SLASHOBSERVE_LLM_MODEL=<model name>
make demo
```

`host.docker.internal` reaches the host on Docker Desktop.
On a Linux Docker engine, use the host's address on the Docker bridge, such as `http://172.17.0.1:11434/v1`, and make the model server listen on it.
Run `make demo` again after changing a variable.

The demo forwards only `SLASHOBSERVE_LLM_PROVIDER`, `SLASHOBSERVE_LLM_MODEL`, `SLASHOBSERVE_LLM_API_KEY`, `SLASHOBSERVE_LLM_BASE_URL` and `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS` from your shell.
Any other LLM setting, such as `SLASHOBSERVE_LLM_EFFORT` or `SLASHOBSERVE_LLM_IDLE_TIMEOUT`, needs a line in the server's `environment` in `deploy/compose/compose.yaml`.

## Help a small or slow model

Dashboard quality depends heavily on the model, and small models are much weaker at it than large hosted ones.
Expect them to call tools with arguments that fail the checks, to stop before every planned panel is saved, or to answer in text instead of calling a tool.
The server checks every save by running its queries, so a weak model produces fewer panels or a failed request, never a dashboard whose queries do not run.

These changes help:

<<<<<<< HEAD
- Give the model the largest context window the server allows, because the agent's instructions, tool definitions and tool results all go into it.
  With Ollama, set `OLLAMA_CONTEXT_LENGTH` before you start it.
- Raise `SLASHOBSERVE_LLM_IDLE_TIMEOUT` for a model that takes longer than 180 seconds before its first token.
- Set `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS=true` to see why a server refused a request; it adds the server's own error message to the log line of a failed call.
- Name the metric, service or table in the prompt.

## Fix a run that fails with no_tool_call

The agent works only through tools: it reads metrics, labels and rows with them and saves every panel with one.
Its plain text is only the live text shown while it works, never a saved result.
A model that answers in text instead of calling a tool saves nothing, and the request fails with `no_tool_call` or `no_progress`; [Read a turn](../use/chat.md#read-a-turn) lists the failure codes.
To fix it:

1. Check that your model server lists the model as tool-capable.
2. Start the server with tool calling turned on, when it has such a switch.
3. Try a model that is better at tool calls.

## Cost of a local model

A local model has no price in the built-in table, so the run cost shows as "Not priced".
The server refuses to start when `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` is set and the model has no price.
To price a local model, see [Price a model that is not in the table](llm-providers.md#price-a-model-that-is-not-in-the-table).

## Reference
=======
- Give the model the largest context window the server allows, because the agent's instructions, tool definitions and tool results all go into it; with Ollama, set `OLLAMA_CONTEXT_LENGTH` before you start it.
- `SLASHOBSERVE_LLM_IDLE_TIMEOUT`, 180 seconds by default, retries a call that receives nothing from the server for that long; raise it for a model that takes longer than that before its first token, or set `0` for no limit.
- `SLASHOBSERVE_LLM_REQUEST_TIMEOUT`, 10 minutes by default, bounds each call with its retries; `0` sets no limit.
- With both timeouts at `0` and `SLASHOBSERVE_AGENT_RUN_TIMEOUT` at its default `0`, a model call that stalls keeps running until someone cancels the run, so keep at least one bound.
- `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS=true` adds the server's own error message to the log line of a failed call, which shows why a server refused a request.
>>>>>>> origin/main

[Configuration](../reference/configuration.md#llm) lists every LLM setting, and [LLM providers](llm-providers.md) covers Anthropic and hosted OpenAI-compatible servers.

### Settings for a local server

| Variable | Value for a local server |
| --- | --- |
| `SLASHOBSERVE_LLM_PROVIDER` | `openai`, the value for every OpenAI-compatible server, not only OpenAI's own. |
| `SLASHOBSERVE_LLM_BASE_URL` | The server's API root. |
| `SLASHOBSERVE_LLM_MODEL` | Required, because there is no safe default. |
| `SLASHOBSERVE_LLM_API_KEY` | Unset for a server that needs no key. |
| `SLASHOBSERVE_LLM_EFFORT` | Unset, unless the server accepts a reasoning effort. |
| `SLASHOBSERVE_LLM_IDLE_TIMEOUT` | `180s` by default. |
| `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` | `10m` by default. |
| `SLASHOBSERVE_LLM_MAX_TOKENS` | `16000` by default. |
| `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS` | `false` by default. |

Details of these settings:

- The base URL is the part before `/chat/completions`, such as `http://localhost:11434/v1` for Ollama.
  It must be an absolute `http` or `https` URL, and it may have a path.
  It must not have a query, a fragment or user info, and the server refuses to start with one that does.
- The provider name is not case sensitive.
  Leave the base URL unset to call OpenAI itself.
- `SLASHOBSERVE_LLM_IDLE_TIMEOUT` retries a call that receives nothing from the server for that long.
  A call that fails with a rate limit, a timeout, a server error or a dropped connection is tried up to 4 times with backoff.
- `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` bounds each call with its retries.
- `SLASHOBSERVE_LLM_MAX_TOKENS` is the output cap of a call, `1024` to `64000`.
  After the first call of a run, calls are capped at half of it, but not below `8192`.
  A call that stops at that lower cap runs again with the full cap.

### How the agent calls the server

- It calls the Chat Completions endpoint with streaming on and asks the server to include usage in the stream.
- It sends the output cap as `max_completion_tokens`, not `max_tokens`.
- It sends the tool definitions with every call and reads tool calls from the stream.
- The model server needs to be reachable from the host that runs `observe-agent`; the server gives that process the model settings, including the key, when it claims a run.

### Effort

`SLASHOBSERVE_LLM_EFFORT` is sent as the reasoning effort only when it is set.
It must be one of `none`, `minimal`, `low`, `medium`, `high` or `xhigh`, in any case, or the server refuses to start.
Unset, nothing is sent and the server uses its own default, which is what most local servers expect.
The first call of a run that has saved nothing yet runs one step higher: `low` becomes `medium`, and `medium` becomes `high`.
A server that does not accept the parameter answers every call with a provider error, so unset the variable if that happens.
