import { createOpenAICompatible } from "@ai-sdk/openai-compatible"
import { log } from "../log"
import type { ModelSettings } from "../protocol"
import { isPlainToken } from "./mechanics/errors"
import { ModelSession, type Adapter, type Finish } from "./model"
import type { SessionDeps } from "./anthropic"
import type { Session, Start, StopReason } from "./types"

export const openaiName = "openai"
export const openaiApiRoot = "https://api.openai.com/v1"

function stopReason(finish: Finish, hasToolCalls: boolean): StopReason {
  if (hasToolCalls && finish.raw !== "length") {
    return "tool_use"
  }
  switch (finish.raw) {
    case "stop":
    case "tool_calls":
      return "end_turn"
    case "length":
      return "max_tokens"
    case "content_filter":
      return "refusal"
    default:
      log("warn", "llm finish reason treated as end of turn", {
        provider: openaiName,
        ...(finish.raw !== undefined && isPlainToken(finish.raw)
          ? { finish_reason: finish.raw }
          : {}),
      })
      return "end_turn"
  }
}

function withCompletionTokens(
  body: Record<string, unknown>
): Record<string, unknown> {
  const { max_tokens: maxTokens, ...rest } = body
  return maxTokens === undefined
    ? rest
    : { ...rest, max_completion_tokens: maxTokens }
}

export function createOpenAISession(
  settings: ModelSettings,
  start: Start,
  signal: AbortSignal,
  deps: SessionDeps = {}
): Session {
  signal.throwIfAborted()
  const model =
    deps.model ??
    createOpenAICompatible({
      name: openaiName,
      baseURL: settings.base_url === "" ? openaiApiRoot : settings.base_url,
      ...(settings.api_key === "" ? {} : { apiKey: settings.api_key }),
      includeUsage: true,
      transformRequestBody: withCompletionTokens,
      ...(deps.fetch === undefined ? {} : { fetch: deps.fetch }),
    }).chatModel(settings.model)
  const adapter: Adapter = {
    name: openaiName,
    model,
    apiKey: settings.api_key,
    logProviderErrors: settings.log_provider_errors,
    requestIdHeader: "x-request-id",
    errorStyle: "label",
    keepsReasoning: false,
    instructions: (system) => (system === "" ? undefined : system),
    providerOptions: (tuning) => ({
      [openaiName]:
        tuning.effort === "" ? {} : { reasoningEffort: tuning.effort },
    }),
    breakpoint: (messages) => messages,
    stopReason,
    usage: (usage) => ({
      inputTokens: Math.max(
        0,
        usage.inputTokenDetails.noCacheTokens ?? usage.inputTokens ?? 0
      ),
      outputTokens: usage.outputTokens ?? 0,
      cacheReadTokens: usage.inputTokenDetails.cacheReadTokens ?? 0,
      cacheWriteTokens: 0,
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
