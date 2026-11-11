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

The server calls `api.anthropic.com` for `anthropic` and `api.openai.com` for `openai`, over HTTPS, unless `SLASHOBSERVE_LLM_BASE_URL` names another server.
These calls are not covered by the [outbound allowlist](../operate/outbound-access.md), so a firewall that restricts egress must allow the host.
Neither adapter reads the provider's own environment variables, such as `ANTHROPIC_API_KEY` or `OPENAI_BASE_URL`; set every value through `SLASHOBSERVE_LLM_*`.
A call that fails with a rate limit, a timeout, a server error or a dropped connection is tried up to 4 times with backoff, inside the request timeout.

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
`SLASHOBSERVE_LLM_EFFORT` sets the model's effort level: `low`, `medium`, `high`, `xhigh` or `max`, and `medium` when unset.
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
[Local models](local-models.md) covers running the agent on a model you serve yourself.

`SLASHOBSERVE_LLM_EFFORT` sets the reasoning effort sent with each call: `none`, `minimal`, `low`, `medium`, `high` or `xhigh`.
Unset, nothing is sent and the server uses its own default.
A reasoning model spends output tokens on reasoning before it answers, so a lower effort such as `low` can shorten each call on some gateways.
A gateway that does not accept the parameter answers every call with a provider error; unset the variable if that happens.

## Run cost

Every finished request reports `usage.cost_usd`, an estimate in US dollars of what the attempt that finished it cost.
It comes from the token counts of the attempt's model calls and the server's price table, so it is not an invoice.
A request that is retried after a lost lease reports the attempt that finished, not the sum of its attempts.
The cost shows in the web app, in `observe status`, in the request's `usage` and on the `run.succeeded`, `run.failed` and `run.canceled` events of the [activity log](../use/activity.md).

The built-in table lists these models, in US dollars per million tokens:

| Model               | Input | Output | Cache read | Cache write |
| ------------------- | ----- | ------ | ---------- | ----------- |
| `claude-opus-5-5`   | 4.00  | 20.00  | 0.20       | 5.00        |
| `claude-sonnet-5-5` | 2.00  | 10.00  | 0.20       | 2.50        |
| `claude-haiku-5-5`  | 0.10  | 0.50   | 0.01       | 0.125       |

A model is priced by its exact name, and otherwise by the longest listed prefix, so a dated id such as `claude-haiku-5-5-20261001` is priced too.
A token class the model used that has no rate leaves the cost empty, never zero, and a model that is not in the table has no cost.
Haiku's long-prompt pricing tier is not modelled, so its cost is underestimated for prompts above that tier's threshold.

`SLASHOBSERVE_LLM_PRICES_FILE` replaces the whole table with a file of the same shape, so an OpenAI-compatible model can be priced.
The file is not merged with the built-in table; list every model you use.
Each entry gives `input`, `output`, `cache_read` and `cache_write` rates, and an optional `match` list of name prefixes:

```yaml
models:
  llama3.1:
    { input: 0.2, output: 0.6, cache_read: 0, cache_write: 0, match: [llama3.1] }
```

The server refuses to start when the file is unreadable, malformed, has a negative rate or lists a prefix twice.
