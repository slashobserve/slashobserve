import { describe, expect, test } from "bun:test"
import { APICallError } from "ai"
import { MockLanguageModelV4 } from "ai/test"
import { configureLog } from "../src/log"
import { createAnthropicSession } from "../src/session/anthropic"
import { prunedResult } from "../src/session/mechanics/compact"
import { CallError, SessionError } from "../src/session/types"
import {
  freshEnv,
  io,
  modelTurn,
  result,
  settings,
  stalled,
  start,
  type ModelTurn,
} from "./sdk"

configureLog("error")

const live = new AbortController().signal

function session(
  turns: Array<ModelTurn | Error>,
  options: {
    settings?: Parameters<typeof settings>[0]
    start?: Parameters<typeof start>[0]
    env?: ReturnType<typeof freshEnv>
  } = {}
) {
  const queue = [...turns]
  const model = new MockLanguageModelV4({
    doStream: async () => {
      const next = queue.shift()
      if (next === undefined) {
        throw new Error("the mock model has no more turns")
      }
      if (next instanceof Error) {
        throw next
      }
      return modelTurn(next)
    },
  })
  const env = options.env ?? freshEnv()
  return {
    model,
    env,
    session: createAnthropicSession(
      settings(options.settings),
      start(options.start),
      live,
      { model, env }
    ),
  }
}

const toolTurn = (id: string, extra: Partial<ModelTurn> = {}): ModelTurn => ({
  calls: [{ id, name: "list", input: '{"q":"a"}' }],
  raw: "tool_use",
  ...extra,
})

type Call = MockLanguageModelV4["doStreamCalls"][number]

function anthropicOptions(call: Call) {
  return (call.providerOptions as { anthropic: Record<string, unknown> })
    .anthropic
}

describe("tuning per call", () => {
  test("the first call of a session steps the effort up and uses the full output limit, later calls halve it", async () => {
    const { session: s, model } = session([toolTurn("c1"), toolTurn("c2")])
    await s.next(undefined, io())
    await s.next([result("c1")], io())
    const [first, second] = model.doStreamCalls
    expect(first?.maxOutputTokens).toBe(16000)
    expect(anthropicOptions(first!)["effort"]).toBe("medium")
    expect(second?.maxOutputTokens).toBe(8192)
    expect(anthropicOptions(second!)["effort"]).toBe("low")
  })

  test("a session that already saved starts on the lowered limit", async () => {
    const { session: s, model } = session([toolTurn("c1")], {
      start: { buildSaves: 2 },
    })
    await s.next(undefined, io())
    expect(model.doStreamCalls[0]?.maxOutputTokens).toBe(8192)
    expect(anthropicOptions(model.doStreamCalls[0]!)["effort"]).toBe("low")
  })

  test("no effort is sent when none is configured, and tool streaming is off", async () => {
    const { session: s, model } = session([toolTurn("c1")], {
      settings: { effort: "" },
    })
    await s.next(undefined, io())
    expect(anthropicOptions(model.doStreamCalls[0]!)).toEqual({
      toolStreaming: false,
    })
  })

  test("a cut-off under the lowered limit resends once with the configured limit and sums usage", async () => {
    const { session: s, model } = session([
      toolTurn("c1"),
      {
        text: ["partial"],
        raw: "max_tokens",
        unified: "length",
        usage: { noCache: 100, output: 8192, cacheRead: 4, cacheWrite: 1 },
      },
      toolTurn("c2", {
        usage: { noCache: 30, output: 700, cacheRead: 6, cacheWrite: 2 },
      }),
    ])
    await s.next(undefined, io())
    const answer = await s.next([result("c1")], io())
    expect(model.doStreamCalls.map((c) => c.maxOutputTokens)).toEqual([
      16000, 8192, 16000,
    ])
    expect(answer.turn.stop).toBe("tool_use")
    expect(answer.turn.usage).toEqual({
      inputTokens: 130,
      outputTokens: 8892,
      cacheReadTokens: 10,
      cacheWriteTokens: 3,
      calls: 1,
    })
    expect(answer.attempts).toBe(2)
  })

  test("a cut-off at the configured limit does not resend and is not added to the history", async () => {
    const { session: s, model } = session([
      { text: ["partial"], raw: "max_tokens", unified: "length" },
      toolTurn("c1"),
    ])
    const cut = await s.next(undefined, io())
    expect(cut.turn.stop).toBe("max_tokens")
    expect(model.doStreamCalls).toHaveLength(1)
    await s.next(undefined, io())
    const prompt = model.doStreamCalls[1]!.prompt
    expect(prompt.map((m) => m.role)).toEqual(["system", "user"])
  })
})

describe("results are checked", () => {
  test("against the pending calls, with the Go texts", async () => {
    const { session: s } = session([toolTurn("c1")])
    await s.next(undefined, io())
    await expect(s.next([], io())).rejects.toThrow(
      "anthropic session: got 0 tool results for 1 tool calls"
    )
    await expect(s.next([result("zz")], io())).rejects.toThrow(
      'anthropic session: tool result 0 answers "zz", want "c1"'
    )
    await expect(s.next([result("zz")], io())).rejects.toBeInstanceOf(
      SessionError
    )
  })

  test("a session that ended with a plain answer takes no more turns", async () => {
    const { session: s } = session([{ text: ["done"], raw: "end_turn" }])
    await s.next(undefined, io())
    await expect(s.next(undefined, io())).rejects.toThrow(
      "anthropic session: the previous turn made no tool calls, so the session has ended"
    )
  })
})

describe("what is sent", () => {
  test("the system text is one block with the first user message after it, tools carry no execute", async () => {
    const { session: s, model } = session([toolTurn("c1")])
    await s.next(undefined, io())
    const call = model.doStreamCalls[0]!
    expect(call.prompt).toEqual([
      {
        role: "system",
        content: "the system text",
        providerOptions: { anthropic: { cacheControl: { type: "ephemeral" } } },
      },
      {
        role: "user",
        content: [
          {
            type: "text",
            text: "the first user message",
            providerOptions: {
              anthropic: { cacheControl: { type: "ephemeral" } },
            },
          },
        ],
      },
    ])
    expect(
      call.tools?.map((t) => (t.type === "function" ? t.name : ""))
    ).toEqual(["list", "save"])
    const [list, save] = call.tools ?? []
    expect(list).toMatchObject({
      description: "lists things",
      inputSchema: { type: "object", properties: { q: { type: "string" } } },
    })
    expect(save).toMatchObject({ inputSchema: { type: "object" } })
    expect(
      save && "description" in save ? save.description : undefined
    ).toBeUndefined()
  })

  test("an empty system text sends no system message", async () => {
    const { session: s, model } = session([toolTurn("c1")], {
      start: { system: "" },
    })
    await s.next(undefined, io())
    expect(model.doStreamCalls[0]?.prompt.map((m) => m.role)).toEqual(["user"])
  })

  test("the cache breakpoint moves to the last tool result and error results are flagged", async () => {
    const { session: s, model } = session([toolTurn("c1"), toolTurn("c2")])
    await s.next(undefined, io())
    await s.next([result("c1", "boom", { isError: true })], io())
    const prompt = model.doStreamCalls[1]!.prompt
    expect(prompt.map((m) => m.role)).toEqual([
      "system",
      "user",
      "assistant",
      "tool",
    ])
    expect(prompt[1]).toEqual({
      role: "user",
      content: [{ type: "text", text: "the first user message" }],
    })
    expect(prompt[3]).toEqual({
      role: "tool",
      content: [
        {
          type: "tool-result",
          toolCallId: "c1",
          toolName: "list",
          output: { type: "error-text", value: "boom" },
          providerOptions: {
            anthropic: { cacheControl: { type: "ephemeral" } },
          },
        },
      ],
    })
  })

  test("thinking blocks come back with their signature on the assistant message", async () => {
    const { session: s, model } = session([
      toolTurn("c1", {
        reasoning: { text: "let me think", metadata: { signature: "sig-1" } },
        text: ["on it"],
      }),
      toolTurn("c2"),
    ])
    await s.next(undefined, io())
    await s.next([result("c1")], io())
    const assistant = model.doStreamCalls[1]!.prompt[2]
    expect(assistant).toEqual({
      role: "assistant",
      content: [
        {
          type: "reasoning",
          text: "let me think",
          providerOptions: { anthropic: { signature: "sig-1" } },
        },
        { type: "text", text: "on it" },
        {
          type: "tool-call",
          toolCallId: "c1",
          toolName: "list",
          input: { q: "a" },
        },
      ],
    })
  })
})

describe("the turn", () => {
  test("text deltas reach onText in order, as they arrive", async () => {
    const { session: s } = session([
      { text: ["one ", "two ", "three"], raw: "end_turn" },
    ])
    const sink = io()
    const answer = await s.next(undefined, sink)
    expect(sink.texts).toEqual(["one ", "two ", "three"])
    expect(answer.turn).toMatchObject({
      text: "one two three",
      toolCalls: [],
      stop: "end_turn",
    })
  })

  test("tool calls come in order with the provider ids and the input as JSON text", async () => {
    const { session: s } = session([
      {
        calls: [
          { id: "toolu_1", name: "list", input: '{"q": "a"}' },
          { id: "toolu_2", name: "save", input: "{}" },
        ],
        raw: "tool_use",
        usage: { noCache: 12, cacheRead: 3, cacheWrite: 4, output: 9 },
      },
    ])
    const answer = await s.next(undefined, io())
    expect(answer.turn.toolCalls).toEqual([
      { id: "toolu_1", name: "list", input: '{"q":"a"}' },
      { id: "toolu_2", name: "save", input: "{}" },
    ])
    expect(answer.turn.usage).toEqual({
      inputTokens: 12,
      outputTokens: 9,
      cacheReadTokens: 3,
      cacheWriteTokens: 4,
      calls: 1,
    })
    expect(answer.attempts).toBe(1)
  })

  test("a tool call whose input is not JSON keeps its text so the run can answer it", async () => {
    const { session: s } = session([
      {
        calls: [{ id: "c1", name: "list", input: "{not json" }],
        raw: "tool_use",
      },
    ])
    const answer = await s.next(undefined, io())
    expect(answer.turn.toolCalls).toEqual([
      { id: "c1", name: "list", input: "{not json" },
    ])
  })

  const stops: Array<[string, NonNullable<ModelTurn["unified"]>, string]> = [
    ["end_turn", "stop", "end_turn"],
    ["stop_sequence", "stop", "end_turn"],
    ["max_tokens", "length", "max_tokens"],
    ["model_context_window_exceeded", "length", "max_tokens"],
    ["refusal", "content-filter", "refusal"],
  ]
  test.each(stops)(
    "the stop reason %s maps to %s",
    async (raw, unified, want) => {
      const { session: s } = session([{ text: ["x"], raw, unified }], {
        settings: { max_tokens: 0 },
      })
      const answer = await s.next(undefined, io())
      expect(answer.turn.stop).toBe(want as never)
    }
  )

  test("an unexpected stop reason is a provider error", async () => {
    const { session: s } = session([
      { text: ["x"], raw: "pause_turn", unified: "stop" },
    ])
    const error = await s.next(undefined, io()).catch((e: unknown) => e)
    expect(error).toBeInstanceOf(CallError)
    expect(error).toMatchObject({
      failure: "provider_error",
      provider: "anthropic",
    })
  })
})

describe("old results are pruned", () => {
  test("results before the boundary are replaced by stubs in what is sent, and the history keeps them", async () => {
    const big = "r".repeat(300)
    const { session: s, model } = session(
      [toolTurn("c1"), toolTurn("c2"), toolTurn("c3"), toolTurn("c4")],
      { start: { pruning: { keepTurns: 1, stepBytes: 500 } } }
    )
    await s.next(undefined, io())
    await s.next([result("c1", big)], io())
    await s.next([result("c2", big)], io())
    await s.next([result("c3", big)], io())
    const outputs = (index: number) =>
      model.doStreamCalls[index]!.prompt.filter(
        (m) => m.role === "tool"
      ).flatMap((m) =>
        m.content.map((p) => p.type === "tool-result" && p.output)
      )
    expect(outputs(1)).toEqual([{ type: "text", value: big }])
    expect(outputs(2)).toEqual([
      { type: "text", value: big },
      { type: "text", value: big },
    ])
    expect(outputs(3)).toEqual([
      { type: "text", value: prunedResult },
      { type: "text", value: prunedResult },
      { type: "text", value: big },
    ])
  })
})

describe("compaction", () => {
  const compaction = {
    message: "the compaction message",
    tailTurns: 2,
    tailBytes: 1 << 20,
    resultBytes: 10,
  }

  test("keeps the tail, cuts results, drops older thinking and puts the message first", async () => {
    const { session: s, model } = session([
      toolTurn("c1", {
        reasoning: { text: "old", metadata: { signature: "s1" } },
      }),
      toolTurn("c2", {
        reasoning: { text: "mid", metadata: { signature: "s2" } },
      }),
      toolTurn("c3", {
        reasoning: { text: "new", metadata: { signature: "s3" } },
      }),
      toolTurn("c4"),
    ])
    await s.next(undefined, io())
    await s.next([result("c1", "a".repeat(40))], io())
    await s.next([result("c2", "b".repeat(40))], io())
    await s.compact(compaction)
    await s.next([result("c3", "c".repeat(40))], io())
    const prompt = model.doStreamCalls[3]!.prompt.filter(
      (m) => m.role !== "system"
    )
    expect(prompt.map((m) => m.role)).toEqual([
      "user",
      "assistant",
      "tool",
      "assistant",
      "tool",
    ])
    expect(prompt[0]).toEqual({
      role: "user",
      content: [{ type: "text", text: "the compaction message" }],
    })
    const kinds = (message: (typeof prompt)[number]) =>
      message.role === "assistant" && typeof message.content !== "string"
        ? message.content.map((part) => part.type)
        : []
    expect(kinds(prompt[1]!)).toEqual(["tool-call"])
    expect(kinds(prompt[3]!)).toEqual(["reasoning", "tool-call"])
    const cut = prompt[2]
    expect(cut?.role === "tool" && cut.content[0]).toMatchObject({
      output: {
        value: `${"b".repeat(10)}\n[cut from 40 bytes; call the tool again for the full result]`,
      },
    })
  })

  test("a plain answer at the end is dropped and the session continues", async () => {
    const { session: s, model } = session([
      toolTurn("c1"),
      { text: ["I am done"], raw: "end_turn" },
      toolTurn("c2"),
    ])
    await s.next(undefined, io())
    await s.next([result("c1")], io())
    await s.compact(compaction)
    await s.next(undefined, io())
    const roles = model.doStreamCalls[2]!.prompt.map((m) => m.role)
    expect(roles).toEqual(["system", "user", "assistant", "tool"])
  })

  test("the next turn after a compaction takes the results it would have", async () => {
    const { session: s } = session([toolTurn("c1"), toolTurn("c2")])
    await s.next(undefined, io())
    await s.compact(compaction)
    await expect(s.next(undefined, io())).rejects.toThrow(
      "got 0 tool results for 1 tool calls"
    )
    const answer = await s.next([result("c1")], io())
    expect(answer.turn.toolCalls[0]?.id).toBe("c2")
  })
})

describe("failures", () => {
  function apiError(status: number, headers: Record<string, string> = {}) {
    return new APICallError({
      message: "SECRET-PROMPT-TEXT",
      url: "https://api.anthropic.com/v1/messages",
      requestBodyValues: { prompt: "SECRET-PROMPT-TEXT" },
      statusCode: status,
      responseHeaders: headers,
      responseBody: '{"error":{"message":"SECRET-BODY"}}',
      data: {
        type: "error",
        error: { type: "overloaded_error", message: "SECRET-BODY" },
      },
    })
  }

  test("a 429 with retry-after retries and returns two attempts", async () => {
    const env = freshEnv()
    const { session: s, model } = session(
      [apiError(429, { "retry-after": "7" }), toolTurn("c1")],
      { env }
    )
    const answer = await s.next(undefined, io())
    expect(answer.attempts).toBe(2)
    expect(model.doStreamCalls).toHaveLength(2)
    expect(env.waits).toHaveLength(1)
    expect(env.waits[0]).toBeGreaterThanOrEqual(7_000)
  })

  test("four failed attempts end in rate_limited", async () => {
    const error = apiError(429)
    const { session: s, model } = session([error, error, error, error])
    const failure = await s.next(undefined, io()).catch((e: unknown) => e)
    expect(failure).toMatchObject({ failure: "rate_limited", status: 429 })
    expect(model.doStreamCalls).toHaveLength(4)
  })

  test("a 400 does not retry and maps to provider_error without the body when logging is off", async () => {
    const { session: s, model } = session([apiError(400), toolTurn("c1")])
    const failure = await s.next(undefined, io()).catch((e: unknown) => e)
    expect(failure).toBeInstanceOf(CallError)
    expect(failure).toMatchObject({
      failure: "provider_error",
      status: 400,
      providerMessage: "",
    })
    for (const text of [
      String((failure as Error).message),
      JSON.stringify(failure),
    ]) {
      expect(text).not.toContain("SECRET")
    }
    expect(model.doStreamCalls).toHaveLength(1)
  })

  test("the provider message is kept, scrubbed of the key, only when logging is on", async () => {
    const error = new APICallError({
      message: "m",
      url: "u",
      requestBodyValues: {},
      statusCode: 400,
      data: {
        type: "error",
        error: {
          type: "invalid_request_error",
          message: "bad key sk-test-secret",
        },
      },
    })
    const { session: s } = session([error], {
      settings: { log_provider_errors: true },
    })
    const failure = await s.next(undefined, io()).catch((e: unknown) => e)
    expect(failure).toMatchObject({
      type: "invalid_request_error",
      providerMessage: "bad key [redacted]",
    })
  })

  test("an aborted signal stops the call and gives back its reason", async () => {
    const controller = new AbortController()
    const model = new MockLanguageModelV4({
      doStream: async ({ abortSignal }) => stalled(abortSignal),
    })
    const s = createAnthropicSession(settings(), start(), live, {
      model,
      env: freshEnv(),
    })
    const reason = new Error("run ended")
    setTimeout(() => controller.abort(reason), 30)
    const failure = await s
      .next(undefined, io(controller.signal))
      .catch((e: unknown) => e)
    expect(failure).toBe(reason)
    expect(model.doStreamCalls).toHaveLength(1)
  })

  test("a signal that is already aborted never calls the model", async () => {
    const { session: s, model } = session([toolTurn("c1")])
    const failure = await s
      .next(undefined, io(AbortSignal.abort(new Error("gone"))))
      .catch((e: unknown) => e)
    expect((failure as Error).message).toBe("gone")
    expect(model.doStreamCalls).toHaveLength(0)
  })

  test("a stream that goes quiet is cut by the idle watch, retried, and succeeds", async () => {
    let calls = 0
    const model = new MockLanguageModelV4({
      doStream: async ({ abortSignal }) =>
        ++calls === 1 ? stalled(abortSignal) : modelTurn(toolTurn("c1")),
    })
    const env = freshEnv()
    const s = createAnthropicSession(
      settings({ idle_timeout_seconds: 1 }),
      start(),
      live,
      { model, env }
    )
    const started = performance.now()
    const answer = await s.next(undefined, io())
    expect(performance.now() - started).toBeLessThan(3_000)
    expect(answer.attempts).toBe(2)
    expect(answer.turn.stop).toBe("tool_use")
  })

  test("a call that outlasts the request timeout ends in call_timeout and is not retried", async () => {
    const model = new MockLanguageModelV4({
      doStream: async ({ abortSignal }) => stalled(abortSignal),
    })
    const s = createAnthropicSession(
      settings({ request_timeout_seconds: 1 }),
      start(),
      live,
      { model, env: freshEnv() }
    )
    const failure = await s.next(undefined, io()).catch((e: unknown) => e)
    expect(failure).toMatchObject({ failure: "call_timeout", idle: false })
    expect(model.doStreamCalls).toHaveLength(1)
  })

  test("a bad tool schema fails the session at start", () => {
    expect(() =>
      createAnthropicSession(
        settings(),
        start({
          tools: [
            { name: "bad", description: "", inputSchema: { type: "string" } },
          ],
        }),
        live,
        { model: new MockLanguageModelV4(), env: freshEnv() }
      )
    ).toThrow(SessionError)
  })
})
