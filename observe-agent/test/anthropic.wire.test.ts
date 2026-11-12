import { afterEach, describe, expect, test } from "bun:test"
import { configureLog } from "../src/log"
import { createAnthropicSession } from "../src/session/anthropic"
import { CallError } from "../src/session/types"
import {
  fakeFetch,
  freshEnv,
  io,
  jsonError,
  result,
  settings,
  sse,
  start,
} from "./sdk"

configureLog("error")

const live = new AbortController().signal

function messageStart(usage: Record<string, number> = {}) {
  return {
    type: "message_start",
    message: {
      id: "msg_1",
      type: "message",
      role: "assistant",
      model: "claude-test",
      content: [],
      stop_reason: null,
      stop_sequence: null,
      usage: { input_tokens: 10, output_tokens: 1, ...usage },
    },
  }
}

function block(index: number, content_block: object, deltas: object[] = []) {
  return [
    { type: "content_block_start", index, content_block },
    ...deltas.map((delta) => ({ type: "content_block_delta", index, delta })),
    { type: "content_block_stop", index },
  ]
}

function messageEnd(stop_reason: string, output_tokens = 20) {
  return [
    {
      type: "message_delta",
      delta: { stop_reason, stop_sequence: null },
      usage: { output_tokens },
    },
    { type: "message_stop" },
  ]
}

const thinkingAndToolUse = [
  messageStart({ cache_creation_input_tokens: 3, cache_read_input_tokens: 4 }),
  ...block(0, { type: "thinking", thinking: "", signature: "" }, [
    { type: "thinking_delta", thinking: "let me " },
    { type: "thinking_delta", thinking: "think" },
    { type: "signature_delta", signature: "sig-abc" },
  ]),
  ...block(1, { type: "text", text: "" }, [
    { type: "text_delta", text: "on it" },
  ]),
  ...block(2, { type: "tool_use", id: "toolu_1", name: "list", input: {} }, [
    { type: "input_json_delta", partial_json: '{"q":' },
    { type: "input_json_delta", partial_json: '"a"}' },
  ]),
  ...messageEnd("tool_use"),
]

const plainEnd = [
  messageStart(),
  ...block(0, { type: "text", text: "" }, [
    { type: "text_delta", text: "done" },
  ]),
  ...messageEnd("end_turn", 2),
]

describe("the request the AI SDK sends matches the Go adapter", () => {
  test("first call: model, limits, effort, cache breakpoints, tools and no thinking switch", async () => {
    const http = fakeFetch(() => sse(plainEnd))
    const s = createAnthropicSession(settings(), start(), live, {
      fetch: http.fetch,
      env: freshEnv(),
    })
    await s.next(undefined, io())
    const [request] = http.requests
    expect(request?.url).toBe("https://api.anthropic.com/v1/messages")
    expect(request?.headers.get("x-api-key")).toBe("sk-test-secret")
    expect(request?.body).toEqual({
      model: "claude-test",
      max_tokens: 16000,
      stream: true,
      output_config: { effort: "medium" },
      system: [
        {
          type: "text",
          text: "the system text",
          cache_control: { type: "ephemeral" },
        },
      ],
      messages: [
        {
          role: "user",
          content: [
            {
              type: "text",
              text: "the first user message",
              cache_control: { type: "ephemeral" },
            },
          ],
        },
      ],
      tool_choice: { type: "auto" },
      tools: [
        {
          name: "list",
          description: "lists things",
          input_schema: {
            type: "object",
            properties: { q: { type: "string" } },
          },
        },
        { name: "save", input_schema: { type: "object" } },
      ],
    })
  })

  test("second call: thinking with its signature, tool use, an error result with the breakpoint, lowered limit and effort", async () => {
    const http = fakeFetch((_request, nth) =>
      sse(nth === 0 ? thinkingAndToolUse : plainEnd)
    )
    const s = createAnthropicSession(settings(), start(), live, {
      fetch: http.fetch,
      env: freshEnv(),
    })
    const first = await s.next(undefined, io())
    expect(first.turn).toEqual({
      text: "on it",
      toolCalls: [{ id: "toolu_1", name: "list", input: '{"q":"a"}' }],
      stop: "tool_use",
      usage: {
        inputTokens: 10,
        outputTokens: 20,
        cacheReadTokens: 4,
        cacheWriteTokens: 3,
        calls: 1,
      },
    })
    await s.next([result("toolu_1", "boom", { isError: true })], io())
    expect(http.requests[1]?.body).toMatchObject({
      max_tokens: 8192,
      output_config: { effort: "low" },
      messages: [
        {
          role: "user",
          content: [{ type: "text", text: "the first user message" }],
        },
        {
          role: "assistant",
          content: [
            {
              type: "thinking",
              thinking: "let me think",
              signature: "sig-abc",
            },
            { type: "text", text: "on it" },
            {
              type: "tool_use",
              id: "toolu_1",
              name: "list",
              input: { q: "a" },
            },
          ],
        },
        {
          role: "user",
          content: [
            {
              type: "tool_result",
              tool_use_id: "toolu_1",
              content: "boom",
              is_error: true,
              cache_control: { type: "ephemeral" },
            },
          ],
        },
      ],
    })
    const sent = JSON.stringify(http.requests[1]?.body)
    expect(sent).not.toContain("eager_input_streaming")
    expect(sent).not.toContain('"thinking":{')
  })

  test("redacted thinking comes back as redacted thinking", async () => {
    const http = fakeFetch((_request, nth) =>
      sse(
        nth === 0
          ? [
              messageStart(),
              ...block(0, { type: "redacted_thinking", data: "opaque" }),
              ...block(
                1,
                { type: "tool_use", id: "toolu_1", name: "save", input: {} },
                [{ type: "input_json_delta", partial_json: "{}" }]
              ),
              ...messageEnd("tool_use"),
            ]
          : plainEnd
      )
    )
    const s = createAnthropicSession(settings(), start(), live, {
      fetch: http.fetch,
      env: freshEnv(),
    })
    await s.next(undefined, io())
    await s.next([result("toolu_1")], io())
    const assistant = (
      http.requests[1]?.body as { messages: Array<{ content: object[] }> }
    ).messages[1]
    expect(assistant?.content[0]).toEqual({
      type: "redacted_thinking",
      data: "opaque",
    })
  })

  test("a configured base URL is the server root, the SDK path is appended once", async () => {
    const http = fakeFetch(() => sse(plainEnd))
    const s = createAnthropicSession(
      settings({ base_url: "http://llm.local:9000/proxy/" }),
      start(),
      live,
      { fetch: http.fetch, env: freshEnv() }
    )
    await s.next(undefined, io())
    expect(http.requests[0]?.url).toBe(
      "http://llm.local:9000/proxy/v1/messages"
    )
  })

  describe("the provider's own environment variables are never read", () => {
    const names = [
      "ANTHROPIC_BASE_URL",
      "ANTHROPIC_API_KEY",
      "ANTHROPIC_AUTH_TOKEN",
    ]
    const saved = names.map((name) => [name, process.env[name]] as const)
    afterEach(() => {
      for (const [name, value] of saved) {
        if (value === undefined) {
          delete process.env[name]
        } else {
          process.env[name] = value
        }
      }
    })

    test("the base URL and key come from the settings", async () => {
      process.env["ANTHROPIC_BASE_URL"] = "http://elsewhere.invalid"
      process.env["ANTHROPIC_API_KEY"] = "env-key"
      process.env["ANTHROPIC_AUTH_TOKEN"] = "env-token"
      const http = fakeFetch(() => sse(plainEnd))
      const s = createAnthropicSession(settings(), start(), live, {
        fetch: http.fetch,
        env: freshEnv(),
      })
      await s.next(undefined, io())
      expect(http.requests[0]?.url).toBe(
        "https://api.anthropic.com/v1/messages"
      )
      expect(http.requests[0]?.headers.get("x-api-key")).toBe("sk-test-secret")
      expect(http.requests[0]?.headers.get("authorization")).toBeNull()
    })
  })
})

describe("over HTTP", () => {
  test("a 429 with retry-after retries and the next attempt succeeds", async () => {
    const env = freshEnv()
    const http = fakeFetch((_request, nth) =>
      nth === 0
        ? jsonError(
            429,
            {
              type: "error",
              error: { type: "rate_limit_error", message: "slow down" },
            },
            { "retry-after": "3" }
          )
        : sse(plainEnd)
    )
    const s = createAnthropicSession(settings(), start(), live, {
      fetch: http.fetch,
      env,
    })
    const answer = await s.next(undefined, io())
    expect(answer.attempts).toBe(2)
    expect(http.requests).toHaveLength(2)
    expect(env.waits[0]).toBeGreaterThanOrEqual(3_000)
  })

  test("a 400 keeps the plain error fields and nothing else", async () => {
    const http = fakeFetch(() =>
      jsonError(
        400,
        {
          type: "error",
          error: {
            type: "invalid_request_error",
            message: "your prompt sk-test-secret was bad",
          },
        },
        { "request-id": "req_011CX" }
      )
    )
    const s = createAnthropicSession(settings(), start(), live, {
      fetch: http.fetch,
      env: freshEnv(),
    })
    const failure = await s.next(undefined, io()).catch((e: unknown) => e)
    expect(failure).toBeInstanceOf(CallError)
    expect(failure).toMatchObject({
      failure: "provider_error",
      status: 400,
      type: "invalid_request_error",
      requestId: "req_011CX",
      providerMessage: "",
    })
    expect(http.requests).toHaveLength(1)
    const text = JSON.stringify(failure) + String((failure as Error).message)
    expect(text).not.toContain("sk-test-secret")
    expect(text).not.toContain("the first user message")
  })

  test("with logging on the message is kept and the key is scrubbed", async () => {
    const http = fakeFetch(() =>
      jsonError(400, {
        type: "error",
        error: {
          type: "invalid_request_error",
          message: "your key sk-test-secret was bad",
        },
      })
    )
    const s = createAnthropicSession(
      settings({ log_provider_errors: true }),
      start(),
      live,
      { fetch: http.fetch, env: freshEnv() }
    )
    const failure = await s.next(undefined, io()).catch((e: unknown) => e)
    expect(failure).toMatchObject({
      providerMessage: "your key [redacted] was bad",
    })
  })

  test("a refusal is a refusal", async () => {
    const http = fakeFetch(() =>
      sse([
        messageStart(),
        ...block(0, { type: "text", text: "" }, [
          { type: "text_delta", text: "no" },
        ]),
        ...messageEnd("refusal", 1),
      ])
    )
    const s = createAnthropicSession(settings(), start(), live, {
      fetch: http.fetch,
      env: freshEnv(),
    })
    const answer = await s.next(undefined, io())
    expect(answer.turn.stop).toBe("refusal")
  })
})
