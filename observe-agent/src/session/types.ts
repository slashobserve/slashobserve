import type { ModelSettings } from "../protocol"

export type ToolSpec = {
  name: string
  description: string
  inputSchema: Record<string, unknown>
}

export type ToolCall = { id: string; name: string; input: string }

export type ToolResult = {
  callId: string
  content: string
  isError: boolean
  prunable: boolean
}

export type StopReason = "tool_use" | "end_turn" | "max_tokens" | "refusal"

export type TurnUsage = {
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  calls: number
}

export type Turn = {
  text: string
  toolCalls: ToolCall[]
  stop: StopReason
  usage: TurnUsage
}

export type Pruning = { keepTurns: number; stepBytes: number }

export type Compaction = {
  message: string
  tailTurns: number
  tailBytes: number
  resultBytes: number
}

export type Start = {
  system: string
  tools: ToolSpec[]
  prompt: string
  userMessage: string
  buildSaves: number
  pruning: Pruning
}

export type TurnIO = { signal: AbortSignal; onText: (text: string) => void }

export type TurnAnswer = { turn: Turn; attempts: number }

export interface Session {
  next(results: ToolResult[] | undefined, io: TurnIO): Promise<TurnAnswer>
  compact(c: Compaction): Promise<void>
}

export type SessionFactory = (
  settings: ModelSettings,
  start: Start,
  signal: AbortSignal
) => Promise<Session>

export type CallFailure = "call_timeout" | "rate_limited" | "provider_error"

export class CallError extends Error {
  readonly failure: CallFailure
  readonly provider: string
  readonly status: number
  readonly idle: boolean

  constructor(
    provider: string,
    failure: CallFailure,
    status = 0,
    idle = false
  ) {
    super(callErrorText(provider, failure, status))
    this.name = "CallError"
    this.failure = failure
    this.provider = provider
    this.status = status
    this.idle = idle
  }
}

function callErrorText(
  provider: string,
  failure: CallFailure,
  status: number
): string {
  const detail =
    failure === "call_timeout"
      ? `${provider} request timed out`
      : status > 0
        ? `${provider} returned status ${status}`
        : `${provider} request failed`
  return `llm provider error: ${detail}`
}

export class NoFixtureError extends Error {
  constructor() {
    super("no replay fixture")
    this.name = "NoFixtureError"
  }
}

export class SessionError extends Error {
  constructor(message: string) {
    super(message)
    this.name = "SessionError"
  }
}

export class ReplayError extends Error {
  constructor(message: string) {
    super(message)
    this.name = "ReplayError"
  }
}
