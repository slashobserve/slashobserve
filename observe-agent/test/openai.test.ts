import { describe, expect, test } from "bun:test"
import { APICallError } from "ai"
import { MockLanguageModelV4 } from "ai/test"
import { configureLog } from "../src/log"
import { createOpenAISession } from "../src/session/openai"
import { prunedResult } from "../src/session/mechanics/compact"
import { CallError } from "../src/session/types"
import {
  fakeFetch,
  freshEnv,
  io,
  jsonError,
  modelTurn,
  result,
  settings,
  sse,
  start,
  type ModelTurn,
} from "./sdk"

configureLog("error")

const live = new AbortController().signal

const openai = (overrides: Parameters<typeof settings>[0] = {}) =>
  settings({ provider: "openai", model: "gpt-test", ...overrides })

function mockSession(
  turns: Array<ModelTurn | Error>,
  options: {
    settings?: Parameters<typeof settings>[0]
    start?: Parameters<typeof start>[0]
  } = {}
) {
  const queue = [...turns]
  const model = new MockLanguageModelV4({
    doStream: async () => {
      const next = queue.shift()
      if (next instanceof Error) {
        throw next
      }
      return modelTurn(next ?? { raw: "stop" })
    },
  })
  const env = freshEnv()
  return {
    model,
    env,
    session: createOpenAISession(
      openai(options.settings),
      start(options.start),
      live,
      { model, env }
    ),
  }
}

const toolTurn = (id: string, extra: Partial<ModelTurn> = {}): ModelTurn => ({
  calls: [{ id, name: "list", input: '{"q":"a"}' }],
  raw: "tool_calls",
  ...extra,
})

describe("what the mock model is sent", () => {
  test("the system text is the instruction, effort steps up on the first call and the limit halves later", async () => {
    const { session, model } = mockSession([toolTurn("c1"), toolTurn("c2")])
    await session.next(undefined, io())
    await session.next([result("c1")], io())
    const [first, second] = model.doStreamCalls
    expect(first?.prompt[0]).toEqual({
      role: "system",
      content: "the system text",
    })
    expect(first?.providerOptions).toEqual({
      openai: { reasoningEffort: "medium" },
    })
    expect(first?.maxOutputTokens).toBe(16000)
    expect(second?.providerOptions).toEqual({
      openai: { reasoningEffort: "low" },
    })
    expect(second?.maxOutputTokens).toBe(8192)
  })

  test("no reasoning effort is sent when none is configured", async () => {
    const { session, model } = mockSession([toolTurn("c1")], {
      settings: { effort: "" },
    })
    await session.next(undefined, io())
    expect(model.doStreamCalls[0]?.providerOptions).toEqual({ openai: {} })
  })

  test("an error result gets the ERROR: label and no reasoning is kept in the history", async () => {
    const { session, model } = mockSession([
      toolTurn("c1", {
        reasoning: { text: "hidden", metadata: { signature: "s" } },
        text: ["ok"],
      }),
      toolTurn("c2"),
    ])
    await session.next(undefined, io())
    await session.next([result("c1", "boom", { isError: true })], io())
    const prompt = model.doStreamCalls[1]!.prompt
    expect(prompt[2]).toEqual({
      role: "assistant",
      content: [
        { type: "text", text: "ok" },
        {
          type: "tool-call",
          toolCallId: "c1",
          toolName: "list",
          input: { q: "a" },
        },
      ],
    })
    expect(prompt[3]).toEqual({
      role: "tool",
      content: [
        {
          type: "tool-result",
          toolCallId: "c1",
          toolName: "list",
          output: { type: "text", value: "ERROR: boom" },
        },
      ],
    })
  })

  test("a stub keeps the ERROR: label of the result it replaces", async () => {
    const { session, model } = mockSession(
      [toolTurn("c1"), toolTurn("c2"), toolTurn("c3")],
      { start: { pruning: { keepTurns: 1, stepBytes: 10 } } }
    )
    await session.next(undefined, io())
    await session.next([result("c1", "x".repeat(50))], io())
    await session.next([result("c2", "y".repeat(50))], io())
    const values = model.doStreamCalls[2]!.prompt.filter(
      (m) => m.role === "tool"
    ).flatMap((m) => m.content.map((p) => p.type === "tool-result" && p.output))
    expect(values).toEqual([
      { type: "text", value: prunedResult },
      { type: "text", value: "y".repeat(50) },
    ])
  })

  test("a cut-off under the lowered limit resends once and sums usage", async () => {
    const { session, model } = mockSession([
      toolTurn("c1"),
      {
        text: ["partial"],
        raw: "length",
        unified: "length",
        usage: { noCache: 100, cacheRead: 5, output: 8192 },
      },
      toolTurn("c2", { usage: { noCache: 30, cacheRead: 2, output: 700 } }),
    ])
    await session.next(undefined, io())
    const answer = await session.next([result("c1")], io())
    expect(model.doStreamCalls.map((c) => c.maxOutputTokens)).toEqual([
      16000, 8192, 16000,
    ])
    expect(answer.turn.usage).toEqual({
      inputTokens: 130,
      outputTokens: 8892,
      cacheReadTokens: 7,
      cacheWriteTokens: 0,
      calls: 1,
    })
    expect(answer.attempts).toBe(2)
  })

  test("results are checked with the openai texts", async () => {
    const { session } = mockSession([toolTurn("c1")])
    await session.next(undefined, io())
    await expect(session.next([], io())).rejects.toThrow(
      "openai session: got 0 tool results for 1 tool calls"
    )
  })

  test("compaction keeps the tail and puts the message first", async () => {
    const { session, model } = mockSession([
      toolTurn("c1"),
      toolTurn("c2"),
      toolTurn("c3"),
    ])
    await session.next(undefined, io())
    await session.next([result("c1", "a".repeat(40))], io())
    await session.compact({
      message: "compacted",
      tailTurns: 1,
      tailBytes: 1 << 20,
      resultBytes: 10,
    })
    await session.next([result("c2", "b")], io())
    const prompt = model.doStreamCalls[2]!.prompt.filter(
      (m) => m.role !== "system"
    )
    expect(prompt.map((m) => m.role)).toEqual(["user", "assistant", "tool"])
    expect(prompt[0]).toEqual({
      role: "user",
      content: [{ type: "text", text: "compacted" }],
    })
  })

  test("a 429 with retry-after retries, a 400 does not and keeps no body", async () => {
    const limited = new APICallError({
      message: "SECRET",
      url: "u",
      requestBodyValues: {},
      statusCode: 429,
      responseHeaders: { "retry-after": "5" },
    })
    const retried = mockSession([limited, toolTurn("c1")])
    const answer = await retried.session.next(undefined, io())
    expect(answer.attempts).toBe(2)
    expect(retried.env.waits[0]).toBeGreaterThanOrEqual(5_000)

    const bad = new APICallError({
      message: "SECRET",
      url: "u",
      requestBodyValues: {},
      statusCode: 400,
      data: {
        error: {
          message: "SECRET-BODY",
          type: "invalid_request_error",
          code: "bad_param",
          param: "messages",
        },
      },
    })
    const rejected = mockSession([bad, toolTurn("c1")])
    const failure = await rejected.session
      .next(undefined, io())
      .catch((e: unknown) => e)
    expect(failure).toBeInstanceOf(CallError)
    expect(failure).toMatchObject({
      failure: "provider_error",
      status: 400,
      type: "invalid_request_error",
      code: "bad_param",
      param: "messages",
      providerMessage: "",
    })
    expect(JSON.stringify(failure)).not.toContain("SECRET")
    expect(rejected.model.doStreamCalls).toHaveLength(1)
  })
})

describe("the stop reason", () => {
  const cases: Array<
    [string, boolean, NonNullable<ModelTurn["unified"]>, string]
  > = [
    ["stop", false, "stop", "end_turn"],
    ["tool_calls", false, "tool-calls", "end_turn"],
    ["tool_calls", true, "tool-calls", "tool_use"],
    ["stop", true, "stop", "tool_use"],
    ["length", false, "length", "max_tokens"],
    ["length", true, "length", "max_tokens"],
    ["content_filter", false, "content-filter", "refusal"],
    ["something_new", false, "stop", "end_turn"],
  ]
  test.each(cases)(
    "finish reason %s with tool calls %p is %s",
    async (raw, withCalls, unified, want) => {
      const calls = withCalls
        ? [{ id: "c1", name: "list", input: "{}" }]
        : undefined
      const { session } = mockSession(
        [
          {
            raw,
            unified,
            text: ["x"],
            ...(calls === undefined ? {} : { calls }),
          },
        ],
        { settings: { max_tokens: 0 } }
      )
      const answer = await session.next(undefined, io())
      expect(answer.turn.stop).toBe(want as never)
    }
  )
})

const chunk = (choices: object[], usage?: object) => ({
  id: "chatcmpl-1",
  object: "chat.completion.chunk",
  created: 1,
  model: "gpt-test",
  choices,
  ...(usage === undefined ? {} : { usage }),
})

const usage = { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 }

const toolCallDelta = {
  index: 0,
  id: "call_1",
  type: "function",
  function: { name: "list", arguments: '{"q":"che' },
}

describe("the stop reason survives the usage chunk", () => {
  const cases: Array<[string, object[], string]> = [
    [
      "length then the empty-delta usage chunk",
      [
        chunk([{ index: 0, delta: { role: "assistant", content: "partial" } }]),
        chunk([{ index: 0, delta: {}, finish_reason: "length" }]),
        chunk([{ index: 0, delta: {} }], usage),
      ],
      "max_tokens",
    ],
    [
      "length then the empty-choices usage chunk",
      [
        chunk([{ index: 0, delta: { role: "assistant", content: "partial" } }]),
        chunk([{ index: 0, delta: {}, finish_reason: "length" }]),
        chunk([], usage),
      ],
      "max_tokens",
    ],
    [
      "stop then the empty-delta usage chunk",
      [
        chunk([{ index: 0, delta: { role: "assistant", content: "done" } }]),
        chunk([{ index: 0, delta: {}, finish_reason: "stop" }]),
        chunk([{ index: 0, delta: {} }], usage),
      ],
      "end_turn",
    ],
    [
      "length with a cut-off tool call is a cut-off, not tool use",
      [
        chunk([
          {
            index: 0,
            delta: { role: "assistant", tool_calls: [toolCallDelta] },
          },
        ]),
        chunk([{ index: 0, delta: {}, finish_reason: "length" }]),
        chunk([{ index: 0, delta: {} }], usage),
      ],
      "max_tokens",
    ],
    [
      "tool_calls then the empty-delta usage chunk",
      [
        chunk([
          {
            index: 0,
            delta: {
              role: "assistant",
              tool_calls: [
                {
                  ...toolCallDelta,
                  function: { name: "list", arguments: '{"q":"a"}' },
                },
              ],
            },
          },
        ]),
        chunk([{ index: 0, delta: {}, finish_reason: "tool_calls" }]),
        chunk([{ index: 0, delta: {} }], usage),
      ],
      "tool_use",
    ],
  ]
  test.each(cases)("%s", async (_name, chunks, want) => {
    const http = fakeFetch(() => sse(chunks, true))
    const session = createOpenAISession(
      openai({ max_tokens: 0 }),
      start(),
      live,
      { fetch: http.fetch, env: freshEnv() }
    )
    const answer = await session.next(undefined, io())
    expect(answer.turn.stop).toBe(want as never)
    expect(answer.turn.usage.outputTokens).toBe(5)
    expect(answer.turn.usage.inputTokens).toBe(10)
  })
})

describe("the request the AI SDK sends matches the Go adapter", () => {
  const done = () =>
    sse(
      [
        chunk([{ index: 0, delta: { role: "assistant", content: "ok" } }]),
        chunk([{ index: 0, delta: {}, finish_reason: "stop" }]),
        chunk([], { ...usage, prompt_tokens_details: { cached_tokens: 4 } }),
      ],
      true
    )

  test("completion tokens, reasoning effort, usage streaming, tools and the default root", async () => {
    const http = fakeFetch(done)
    const session = createOpenAISession(openai(), start(), live, {
      fetch: http.fetch,
      env: freshEnv(),
    })
    const answer = await session.next(undefined, io())
    const [request] = http.requests
    expect(request?.url).toBe("https://api.openai.com/v1/chat/completions")
    expect(request?.headers.get("authorization")).toBe("Bearer sk-test-secret")
    expect(request?.body).toEqual({
      model: "gpt-test",
      max_completion_tokens: 16000,
      reasoning_effort: "medium",
      stream: true,
      stream_options: { include_usage: true },
      messages: [
        { role: "system", content: "the system text" },
        { role: "user", content: "the first user message" },
      ],
      tool_choice: "auto",
      tools: [
        {
          type: "function",
          function: {
            name: "list",
            description: "lists things",
            parameters: {
              type: "object",
              properties: { q: { type: "string" } },
            },
          },
        },
        {
          type: "function",
          function: { name: "save", parameters: { type: "object" } },
        },
      ],
    })
    expect(answer.turn.usage).toEqual({
      inputTokens: 6,
      outputTokens: 5,
      cacheReadTokens: 4,
      cacheWriteTokens: 0,
      calls: 1,
    })
  })

  test("a base URL is used as the API root and no key means no authorization header", async () => {
    const http = fakeFetch(done)
    const session = createOpenAISession(
      openai({
        base_url: "http://localhost:11434/v1",
        api_key: "",
        effort: "",
      }),
      start(),
      live,
      { fetch: http.fetch, env: freshEnv() }
    )
    await session.next(undefined, io())
    const [request] = http.requests
    expect(request?.url).toBe("http://localhost:11434/v1/chat/completions")
    expect(request?.headers.get("authorization")).toBeNull()
    expect(request?.body).not.toHaveProperty("reasoning_effort")
  })

  test("the second call carries the assistant tool calls and the labelled error result", async () => {
    const http = fakeFetch((_request, nth) =>
      nth === 0
        ? sse(
            [
              chunk([
                {
                  index: 0,
                  delta: {
                    role: "assistant",
                    tool_calls: [
                      {
                        ...toolCallDelta,
                        function: { name: "list", arguments: '{"q":"a"}' },
                      },
                    ],
                  },
                },
              ]),
              chunk([{ index: 0, delta: {}, finish_reason: "tool_calls" }]),
              chunk([], usage),
            ],
            true
          )
        : done()
    )
    const session = createOpenAISession(openai(), start(), live, {
      fetch: http.fetch,
      env: freshEnv(),
    })
    await session.next(undefined, io())
    await session.next([result("call_1", "boom", { isError: true })], io())
    expect(http.requests[1]?.body).toMatchObject({
      max_completion_tokens: 8192,
      reasoning_effort: "low",
      messages: [
        { role: "system", content: "the system text" },
        { role: "user", content: "the first user message" },
        {
          role: "assistant",
          content: null,
          tool_calls: [
            {
              id: "call_1",
              type: "function",
              function: { name: "list", arguments: '{"q":"a"}' },
            },
          ],
        },
        { role: "tool", tool_call_id: "call_1", content: "ERROR: boom" },
      ],
    })
    expect(http.requests[1]?.body).not.toHaveProperty("max_tokens")
  })

  test("a 429 with retry-after retries over HTTP", async () => {
    const env = freshEnv()
    const http = fakeFetch((_request, nth) =>
      nth === 0
        ? jsonError(
            429,
            {
              error: {
                message: "slow",
                type: "rate_limit_error",
                code: "rate_limit_exceeded",
              },
            },
            { "retry-after": "2", "x-request-id": "req_1" }
          )
        : done()
    )
    const session = createOpenAISession(openai(), start(), live, {
      fetch: http.fetch,
      env,
    })
    const answer = await session.next(undefined, io())
    expect(answer.attempts).toBe(2)
    expect(env.waits[0]).toBeGreaterThanOrEqual(2_000)
  })

  test("a 401 maps to provider_error with its plain fields and the request id", async () => {
    const http = fakeFetch(() =>
      jsonError(
        401,
        {
          error: {
            message: "bad key sk-test-secret",
            type: "invalid_request_error",
            code: "invalid_api_key",
          },
        },
        { "x-request-id": "req_77" }
      )
    )
    const session = createOpenAISession(openai(), start(), live, {
      fetch: http.fetch,
      env: freshEnv(),
    })
    const failure = await session.next(undefined, io()).catch((e: unknown) => e)
    expect(failure).toMatchObject({
      failure: "provider_error",
      status: 401,
      type: "invalid_request_error",
      code: "invalid_api_key",
      requestId: "req_77",
      providerMessage: "",
    })
    expect(http.requests).toHaveLength(1)
  })
})
