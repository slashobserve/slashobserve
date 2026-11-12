import { MockLanguageModelV4 } from "ai/test"
import { modelSettings, type ModelSettings } from "../src/protocol"
import type { RetryEnv } from "../src/session/mechanics/retry"
import type { Start, ToolResult, TurnIO } from "../src/session/types"

type Doer = MockLanguageModelV4["doStream"]
type StreamResult = Awaited<ReturnType<Doer>>
export type Part =
  StreamResult["stream"] extends ReadableStream<infer P> ? P : never
export type Usage = {
  noCache?: number
  cacheRead?: number
  cacheWrite?: number
  output?: number
}

export function settings(
  overrides: Partial<ModelSettings> = {}
): ModelSettings {
  return {
    ...modelSettings.parse({ provider: "anthropic" }),
    model: "claude-test",
    api_key: "sk-test-secret",
    effort: "low",
    max_tokens: 16000,
    ...overrides,
  }
}

export function start(overrides: Partial<Start> = {}): Start {
  return {
    system: "the system text",
    tools: [
      {
        name: "list",
        description: "lists things",
        inputSchema: { type: "object", properties: { q: { type: "string" } } },
      },
      { name: "save", description: "", inputSchema: {} },
    ],
    prompt: "make a dashboard",
    userMessage: "the first user message",
    buildSaves: 0,
    pruning: { keepTurns: 4, stepBytes: 64 << 10 },
    ...overrides,
  }
}

export function result(
  callId: string,
  content = "{}",
  extra: Partial<ToolResult> = {}
): ToolResult {
  return { callId, content, isError: false, prunable: true, ...extra }
}

export function io(
  signal: AbortSignal = new AbortController().signal
): TurnIO & { texts: string[] } {
  const texts: string[] = []
  return { signal, onText: (text) => texts.push(text), texts }
}

export function freshEnv(): RetryEnv & { waits: number[] } {
  const env = {
    waits: [] as number[],
    sleep: async (ms: number) => {
      env.waits.push(ms)
    },
    random: () => 0,
    now: Date.now,
  }
  return env
}

export function stream(parts: Part[]): StreamResult {
  return {
    stream: new ReadableStream<Part>({
      start(controller) {
        for (const part of parts) {
          controller.enqueue(part)
        }
        controller.close()
      },
    }),
  }
}

export function stalled(abortSignal: AbortSignal | undefined): StreamResult {
  return {
    stream: new ReadableStream<Part>({
      start(controller) {
        controller.enqueue({ type: "stream-start", warnings: [] })
        abortSignal?.addEventListener("abort", () =>
          controller.error(abortSignal.reason)
        )
      },
    }),
  }
}

export type ModelTurn = {
  text?: string[]
  reasoning?: { text: string; metadata: Record<string, unknown> }
  calls?: Array<{ id: string; name: string; input: string }>
  raw: string
  unified?: "stop" | "length" | "content-filter" | "tool-calls"
  usage?: Usage
}

export function modelTurn(turn: ModelTurn): StreamResult {
  const parts: Part[] = [{ type: "stream-start", warnings: [] }]
  if (turn.reasoning !== undefined) {
    parts.push(
      { type: "reasoning-start", id: "r0" },
      { type: "reasoning-delta", id: "r0", delta: turn.reasoning.text },
      {
        type: "reasoning-delta",
        id: "r0",
        delta: "",
        providerMetadata: { anthropic: turn.reasoning.metadata } as never,
      },
      { type: "reasoning-end", id: "r0" }
    )
  }
  if (turn.text !== undefined) {
    parts.push({ type: "text-start", id: "t0" })
    for (const delta of turn.text) {
      parts.push({ type: "text-delta", id: "t0", delta })
    }
    parts.push({ type: "text-end", id: "t0" })
  }
  for (const call of turn.calls ?? []) {
    parts.push({
      type: "tool-call",
      toolCallId: call.id,
      toolName: call.name,
      input: call.input,
    })
  }
  const u = turn.usage ?? {}
  const noCache = u.noCache ?? 10
  const cacheRead = u.cacheRead ?? 0
  const cacheWrite = u.cacheWrite ?? 0
  parts.push({
    type: "finish",
    finishReason: {
      unified:
        turn.unified ?? ((turn.calls?.length ?? 0) > 0 ? "tool-calls" : "stop"),
      raw: turn.raw,
    },
    usage: {
      inputTokens: {
        total: noCache + cacheRead + cacheWrite,
        noCache,
        cacheRead,
        cacheWrite,
      },
      outputTokens: {
        total: u.output ?? 5,
        text: u.output ?? 5,
        reasoning: undefined,
      },
    },
  })
  return stream(parts)
}

export type Recorded = { url: string; headers: Headers; body: unknown }

export function fakeFetch(
  respond: (request: Recorded, nth: number) => Response | Promise<Response>
): { fetch: typeof fetch; requests: Recorded[] } {
  const requests: Recorded[] = []
  const handler = async (
    input: string | URL | Request,
    init?: RequestInit
  ): Promise<Response> => {
    const url = typeof input === "string" ? input : String(input)
    const raw = init?.body
    const recorded: Recorded = {
      url,
      headers: new Headers(init?.headers),
      body: typeof raw === "string" ? JSON.parse(raw) : undefined,
    }
    requests.push(recorded)
    return respond(recorded, requests.length - 1)
  }
  return { fetch: Object.assign(handler, { preconnect: () => {} }), requests }
}

export function sse(events: unknown[], done = false): Response {
  const body =
    events
      .map((event) => {
        const name =
          typeof event === "object" && event !== null && "type" in event
            ? `event: ${String(event.type)}\n`
            : ""
        return `${name}data: ${JSON.stringify(event)}\n\n`
      })
      .join("") + (done ? "data: [DONE]\n\n" : "")
  return new Response(body, {
    status: 200,
    headers: { "content-type": "text/event-stream" },
  })
}

export function jsonError(
  status: number,
  body: unknown,
  headers: Record<string, string> = {}
): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...headers },
  })
}
