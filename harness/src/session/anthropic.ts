import { createAnthropic } from "@ai-sdk/anthropic"
import type { LanguageModel, ModelMessage } from "ai"
import type { ModelSettings } from "../protocol"
import { ModelSession, type Adapter, type Finish } from "./model"
import type { RetryEnv } from "./mechanics/retry"
import { CallError, type Session, type Start, type StopReason } from "./types"

export const anthropicName = "anthropic"
export const anthropicApiRoot = "https://api.anthropic.com"

const ephemeral = { anthropic: { cacheControl: { type: "ephemeral" } } }

export type SessionDeps = {
  model?: LanguageModel
  fetch?: typeof fetch
  env?: RetryEnv
}

export function anthropicBaseUrl(baseUrl: string): string {
  const root = baseUrl === "" ? anthropicApiRoot : baseUrl.replace(/\/+$/, "")
  return `${root}/v1`
}

function stopReason(finish: Finish): StopReason {
  switch (finish.raw) {
    case "tool_use":
      return "tool_use"
    case "end_turn":
    case "stop_sequence":
      return "end_turn"
    case "max_tokens":
    case "model_context_window_exceeded":
      return "max_tokens"
    case "refusal":
      return "refusal"
    default:
      throw new CallError(anthropicName, "provider_error")
  }
}

export function withCacheBreakpoint(messages: ModelMessage[]): ModelMessage[] {
  const last = messages.at(-1)
  if (last === undefined) {
    return messages
  }
  const head = messages.slice(0, -1)
  if (last.role === "user" && typeof last.content !== "string") {
    const end = last.content.at(-1)
    if (end?.type !== "text") {
      return messages
    }
    return [
      ...head,
      {
        ...last,
        content: [
          ...last.content.slice(0, -1),
          { ...end, providerOptions: ephemeral },
        ],
      },
    ]
  }
  if (last.role === "tool") {
    const end = last.content.at(-1)
    if (end?.type !== "tool-result") {
      return messages
    }
    return [
      ...head,
      {
        ...last,
        content: [
          ...last.content.slice(0, -1),
          { ...end, providerOptions: ephemeral },
        ],
      },
    ]
  }
  return messages
}

export function createAnthropicSession(
  settings: ModelSettings,
  start: Start,
  signal: AbortSignal,
  deps: SessionDeps = {}
): Session {
  signal.throwIfAborted()
  const model =
    deps.model ??
    createAnthropic({
      apiKey: settings.api_key,
      baseURL: anthropicBaseUrl(settings.base_url),
      ...(deps.fetch === undefined ? {} : { fetch: deps.fetch }),
    })(settings.model)
  const adapter: Adapter = {
    name: anthropicName,
    model,
    apiKey: settings.api_key,
    logProviderErrors: settings.log_provider_errors,
    requestIdHeader: "request-id",
    errorStyle: "flag",
    keepsReasoning: true,
    instructions: (system) =>
      system === ""
        ? undefined
        : { role: "system", content: system, providerOptions: ephemeral },
    providerOptions: (tuning) => ({
      anthropic: {
        toolStreaming: false,
        ...(tuning.effort === "" ? {} : { effort: tuning.effort }),
      },
    }),
    breakpoint: withCacheBreakpoint,
    stopReason: (finish) => stopReason(finish),
    usage: (usage) => ({
      inputTokens:
        usage.inputTokenDetails.noCacheTokens ?? usage.inputTokens ?? 0,
      outputTokens: usage.outputTokens ?? 0,
      cacheReadTokens: usage.inputTokenDetails.cacheReadTokens ?? 0,
      cacheWriteTokens: usage.inputTokenDetails.cacheWriteTokens ?? 0,
    }),
  }
  return new ModelSession(
    adapter,
    {
      effort: settings.effort,
      maxTokens: settings.max_tokens,
      requestTimeoutMs: settings.request_timeout_seconds * 1_000,
      idleTimeoutMs: settings.idle_timeout_seconds * 1_000,
    },
    start,
    deps.env
  )
}
