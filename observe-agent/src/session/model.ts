import {
  jsonSchema,
  streamText,
  tool,
  type Instructions,
  type JSONSchema7,
  type LanguageModel,
  type LanguageModelUsage,
  type ModelMessage,
  type ToolSet,
} from "ai"
import { Pruner, tailStart } from "./mechanics/compact"
import { toCallError, type ErrorContext } from "./mechanics/errors"
import {
  assistantMessage,
  checkResults,
  cutToolResults,
  endsInPlainAnswer,
  isAssistant,
  pruneView,
  toolMessage,
  userMessage,
  weigh,
  withoutReasoning,
  withStubs,
  type ErrorStyle,
} from "./mechanics/history"
import { IdleWatch } from "./mechanics/idle"
import { realEnv, retry, type RetryEnv } from "./mechanics/retry"
import { parseToolSchema } from "./mechanics/schema"
import { cutOffEarly, forCall, type Tuning } from "./mechanics/tuning"
import {
  CallError,
  type Compaction,
  type Session,
  type Start,
  type StopReason,
  type ToolCall,
  type ToolResult,
  type TurnAnswer,
  type TurnIO,
  type TurnUsage,
} from "./types"

type StreamOptions = Parameters<typeof streamText>[0]

export type ProviderOptions = NonNullable<StreamOptions["providerOptions"]>

type AssistantPart = Extract<ModelMessage, { role: "assistant" }>["content"]

type AssistantParts = Exclude<AssistantPart, string>

export type Finish = { raw: string | undefined; unified: string }

export type Adapter = {
  name: "anthropic" | "openai"
  model: LanguageModel
  apiKey: string
  logProviderErrors: boolean
  requestIdHeader: string
  errorStyle: ErrorStyle
  keepsReasoning: boolean
  instructions: (system: string) => Instructions | undefined
  providerOptions: (tuning: Tuning) => ProviderOptions
  breakpoint: (messages: ModelMessage[]) => ModelMessage[]
  stopReason: (finish: Finish, hasToolCalls: boolean) => StopReason
  usage: (usage: LanguageModelUsage) => Omit<TurnUsage, "calls">
}

export type SessionConfig = {
  effort: string
  maxTokens: number
  requestTimeoutMs: number
  idleTimeoutMs: number
}

type Streamed = {
  text: string
  calls: ToolCall[]
  parts: AssistantParts
  finish: Finish
  usage: LanguageModelUsage
}

type Exchange = {
  streamed: Streamed
  stop: StopReason
  usage: TurnUsage
  attempts: number
}

type Mutable<T> = { -readonly [K in keyof T]: T[K] }

type ReasoningPart = Mutable<
  Extract<AssistantParts[number], { type: "reasoning" }>
>

type TextPart = Mutable<Extract<AssistantParts[number], { type: "text" }>>

function mergeMetadata(
  target: Record<string, Record<string, unknown>>,
  metadata: Record<string, Record<string, unknown>> | undefined
): void {
  for (const [provider, values] of Object.entries(metadata ?? {})) {
    target[provider] = { ...target[provider], ...values }
  }
}

function addUsage(a: TurnUsage, b: TurnUsage): TurnUsage {
  return {
    inputTokens: a.inputTokens + b.inputTokens,
    outputTokens: a.outputTokens + b.outputTokens,
    cacheReadTokens: a.cacheReadTokens + b.cacheReadTokens,
    cacheWriteTokens: a.cacheWriteTokens + b.cacheWriteTokens,
    calls: a.calls + b.calls,
  }
}

function buildTools(start: Start): ToolSet {
  const tools: ToolSet = {}
  for (const spec of start.tools) {
    tools[spec.name] = tool({
      ...(spec.description === "" ? {} : { description: spec.description }),
      inputSchema: jsonSchema(parseToolSchema(spec) as JSONSchema7),
    })
  }
  return tools
}

export class ModelSession implements Session {
  private readonly adapter: Adapter
  private readonly config: SessionConfig
  private readonly env: RetryEnv
  private readonly instructions: Instructions | undefined
  private readonly tools: ToolSet
  private readonly buildSaves: number
  private readonly pruner: Pruner
  private history: ModelMessage[]
  private calls = 0
  private pending: ToolCall[] = []
  private ended = false

  constructor(
    adapter: Adapter,
    config: SessionConfig,
    start: Start,
    env: RetryEnv = realEnv
  ) {
    this.adapter = adapter
    this.config = config
    this.env = env
    this.tools = buildTools(start)
    this.instructions = adapter.instructions(start.system)
    this.buildSaves = start.buildSaves
    this.pruner = new Pruner(start.pruning)
    this.history = [userMessage(start.userMessage)]
  }

  async next(
    results: ToolResult[] | undefined,
    io: TurnIO
  ): Promise<TurnAnswer> {
    io.signal.throwIfAborted()
    const answered = results ?? []
    checkResults(
      this.adapter.name,
      this.ended,
      this.pending.map((call) => call.id),
      answered
    )
    this.pruner.note(answered)
    const messages =
      answered.length > 0
        ? [
            ...this.history,
            toolMessage(this.adapter.errorStyle, this.pending, answered),
          ]
        : this.history

    const { requestTimeoutMs } = this.config
    const timeout =
      requestTimeoutMs > 0 ? AbortSignal.timeout(requestTimeoutMs) : undefined
    const call = AbortSignal.any(
      timeout === undefined ? [io.signal] : [io.signal, timeout]
    )
    const deadline =
      requestTimeoutMs > 0 ? this.env.now() + requestTimeoutMs : undefined
    const scope = { turn: io.signal, call, deadline, onText: io.onText }

    const configured: Tuning = {
      effort: this.config.effort,
      maxTokens: this.config.maxTokens,
    }
    const tuning = forCall(
      configured,
      this.calls === 0 && this.buildSaves === 0
    )
    const sent = this.pruned(messages)
    let exchange = await this.exchange(sent, tuning, scope)
    if (cutOffEarly(configured, tuning, exchange.stop)) {
      const cutOff = exchange
      exchange = await this.exchange(sent, configured, scope)
      exchange = {
        ...exchange,
        usage: { ...addUsage(cutOff.usage, exchange.usage), calls: 1 },
        attempts: cutOff.attempts + exchange.attempts,
      }
    }
    const turn = {
      text: exchange.streamed.text,
      toolCalls: exchange.streamed.calls,
      stop: exchange.stop,
      usage: exchange.usage,
    }
    if (exchange.stop === "max_tokens") {
      return { turn, attempts: exchange.attempts }
    }
    this.history = [...messages, assistantMessage(exchange.streamed.parts)]
    this.calls++
    this.pending = exchange.streamed.calls
    this.ended = this.pending.length === 0
    return { turn, attempts: exchange.attempts }
  }

  async compact(c: Compaction): Promise<void> {
    let rest = this.history.slice(1)
    if (endsInPlainAnswer(rest)) {
      rest = rest.slice(0, -1)
      this.ended = false
    }
    const kept = rest.slice(
      tailStart(weigh(rest, c.resultBytes), c.tailTurns, c.tailBytes)
    )
    const lastAssistant = kept.findLastIndex(isAssistant)
    this.history = [
      userMessage(c.message),
      ...kept.map((message, i) => {
        if (message.role === "tool") {
          return cutToolResults(message, c.resultBytes)
        }
        return i === lastAssistant ? message : withoutReasoning(message)
      }),
    ]
    this.pruner.reset()
  }

  private pruned(messages: ModelMessage[]): ModelMessage[] {
    const boundary = this.pruner.boundary(pruneView(messages))
    return withStubs(messages, boundary, this.pruner, this.adapter.errorStyle)
  }

  private async exchange(
    messages: ModelMessage[],
    tuning: Tuning,
    scope: Scope
  ): Promise<Exchange> {
    const sent = this.adapter.breakpoint(messages)
    const retried = await retry(
      () => this.streamOnce(sent, tuning, scope),
      scope.call,
      scope.deadline === undefined
        ? { env: this.env }
        : { deadline: scope.deadline, env: this.env }
    ).catch((error: unknown) => {
      scope.turn.throwIfAborted()
      throw error
    })
    const { value: streamed, attempts } = retried
    const stop = this.adapter.stopReason(
      streamed.finish,
      streamed.calls.length > 0
    )
    return {
      streamed,
      stop,
      attempts,
      usage: { ...this.adapter.usage(streamed.usage), calls: 1 },
    }
  }

  private async streamOnce(
    messages: ModelMessage[],
    tuning: Tuning,
    scope: Scope
  ): Promise<Streamed> {
    const { adapter } = this
    const watch = new IdleWatch(this.config.idleTimeoutMs)
    watch.touch()
    const signal = AbortSignal.any([scope.call, watch.signal])
    const result = streamText({
      model: adapter.model,
      ...(this.instructions === undefined
        ? {}
        : { instructions: this.instructions }),
      messages,
      tools: this.tools,
      ...(tuning.maxTokens > 0 ? { maxOutputTokens: tuning.maxTokens } : {}),
      providerOptions: adapter.providerOptions(tuning),
      maxRetries: 0,
      abortSignal: signal,
      includeRawChunks: true,
      onError: () => undefined,
    })

    let text = ""
    const calls: ToolCall[] = []
    const parts: AssistantParts = []
    const open = new Map<string, TextPart | ReasoningPart>()
    let finish: Finish | undefined
    let usage: LanguageModelUsage | undefined
    let failed = false
    let failure: unknown
    try {
      for await (const part of result.fullStream) {
        watch.touch()
        switch (part.type) {
          case "text-start": {
            const textPart: TextPart = { type: "text", text: "" }
            open.set(part.id, textPart)
            parts.push(textPart)
            break
          }
          case "text-delta": {
            const textPart = open.get(part.id)
            if (textPart?.type === "text") {
              textPart.text += part.text
            }
            text += part.text
            if (part.text !== "") {
              scope.onText(part.text)
            }
            break
          }
          case "reasoning-start": {
            const reasoning: ReasoningPart = {
              type: "reasoning",
              text: "",
              providerOptions: {},
            }
            mergeMetadata(
              reasoning.providerOptions ?? {},
              part.providerMetadata
            )
            open.set(part.id, reasoning)
            if (adapter.keepsReasoning) {
              parts.push(reasoning)
            }
            break
          }
          case "reasoning-delta":
          case "reasoning-end": {
            const reasoning = open.get(part.id)
            if (reasoning?.type === "reasoning") {
              if (part.type === "reasoning-delta") {
                reasoning.text += part.text
              }
              mergeMetadata(
                reasoning.providerOptions ?? {},
                part.providerMetadata
              )
            }
            break
          }
          case "tool-call": {
            if (part.providerExecuted === true) {
              break
            }
            const invalid = part.invalid === true
            const input =
              typeof part.input === "string"
                ? part.input
                : JSON.stringify(part.input)
            calls.push({ id: part.toolCallId, name: part.toolName, input })
            parts.push({
              type: "tool-call",
              toolCallId: part.toolCallId,
              toolName: part.toolName,
              input: invalid ? {} : part.input,
            })
            break
          }
          case "finish":
            finish = { raw: part.rawFinishReason, unified: part.finishReason }
            usage = part.totalUsage
            break
          case "error":
            if (!failed) {
              failed = true
              failure = part.error
            }
            break
          case "abort":
            failed = true
            break
        }
      }
    } catch (error) {
      failed = true
      failure = error
    } finally {
      watch.stop()
    }

    const context = this.errorContext(scope)
    if (failed || signal.aborted) {
      throw toCallError(
        failure,
        context,
        watch.failure(scope.turn, scope.call, adapter.name)
      )
    }
    if (finish === undefined || usage === undefined) {
      throw new CallError(adapter.name, "provider_error")
    }
    return {
      text,
      calls,
      parts: parts.filter((part) => part.type !== "text" || part.text !== ""),
      finish,
      usage,
    }
  }

  private errorContext(scope: Scope): ErrorContext {
    return {
      provider: this.adapter.name,
      apiKey: this.adapter.apiKey,
      logProviderErrors: this.adapter.logProviderErrors,
      requestIdHeader: this.adapter.requestIdHeader,
      turn: scope.turn,
      call: scope.call,
    }
  }
}

type Scope = {
  turn: AbortSignal
  call: AbortSignal
  deadline: number | undefined
  onText: (text: string) => void
}
