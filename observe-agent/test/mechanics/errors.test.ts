import { describe, expect, test } from "bun:test"
import { APICallError } from "ai"
import {
  isPlainToken,
  scrubMessage,
  toCallError,
  type ErrorContext,
} from "../../src/session/mechanics/errors"
import { CallError } from "../../src/session/types"

const live = new AbortController().signal

function context(overrides: Partial<ErrorContext> = {}): ErrorContext {
  return {
    provider: "anthropic",
    apiKey: "sk-secret",
    logProviderErrors: false,
    requestIdHeader: "request-id",
    turn: live,
    call: live,
    ...overrides,
  }
}

function apiError(
  statusCode: number,
  fields: {
    message?: string
    type?: string
    code?: string | number
    param?: string
    headers?: Record<string, string>
  } = {}
) {
  return new APICallError({
    message: "SECRET-PROMPT-TEXT https://api.example/v1?key=sk-secret",
    url: "https://api.example/v1/messages?key=sk-secret",
    requestBodyValues: { prompt: "SECRET-PROMPT-TEXT" },
    statusCode,
    responseHeaders: fields.headers ?? {},
    responseBody: '{"error":{"message":"SECRET-BODY"}}',
    data: {
      error: {
        message: fields.message ?? "prompt fragment UNIQUE",
        type: fields.type,
        code: fields.code,
        param: fields.param,
      },
    },
  })
}

function callError(error: unknown, ctx = context()): CallError {
  const mapped = toCallError(error, ctx, undefined)
  expect(mapped).toBeInstanceOf(CallError)
  return mapped as CallError
}

describe("scrubMessage", () => {
  const key = "sk-secret-key"
  const cases: Array<[string, string, string]> = [
    [
      "key removed everywhere",
      `bad ${key} and ${key} again`,
      "bad [redacted] and [redacted] again",
    ],
    [
      "newlines and tabs become spaces",
      "line one\nline two\r\n\tline three",
      "line one line two   line three",
    ],
    [
      "control characters become spaces",
      "a\u0000b\u001bc\u0085d e",
      "a b c d e",
    ],
    ["surrounding space is trimmed", "  padded  ", "padded"],
    ["lone surrogates are dropped", "ok\ud800end\udc00", "okend"],
    ["empty stays empty", "", ""],
  ]
  test.each(cases)("%s", (_name, message, want) => {
    expect(scrubMessage(message, key)).toBe(want)
  })

  const capped: Array<[string, string, string]> = [
    ["ascii at the cap", "a".repeat(1024), "a".repeat(1024)],
    ["ascii over the cap", "a".repeat(3000), "a".repeat(1024)],
    ["three byte runes are not split", "€".repeat(1000), "€".repeat(341)],
    ["four byte runes are not split", "😀".repeat(500), "😀".repeat(256)],
    [
      "two byte rune straddling the cap",
      "a".repeat(1023) + "é",
      "a".repeat(1023),
    ],
  ]
  test.each(capped)("capped on a rune boundary: %s", (_name, message, want) => {
    const got = scrubMessage(message, "")
    expect(got).toBe(want)
    expect(got).not.toContain("�")
  })

  test("the cap applies after the key is removed", () => {
    const long = "k".repeat(2000)
    expect(scrubMessage("x" + long, long)).toBe("x[redacted]")
  })
})

describe("toCallError", () => {
  test("the provider message is dropped when logging is off", () => {
    expect(callError(apiError(400)).providerMessage).toBe("")
  })

  test("the provider message is kept scrubbed when logging is on, and never enters the error text", () => {
    const err = callError(
      apiError(400, { message: "bad sk-secret UNIQUE-2" }),
      context({ logProviderErrors: true })
    )
    expect(err.providerMessage).toBe("bad [redacted] UNIQUE-2")
    for (const text of [err.message, String(err), JSON.stringify(err)]) {
      expect(text).not.toContain("UNIQUE-2")
      expect(text).not.toContain("SECRET")
      expect(text).not.toContain("sk-secret")
    }
  })

  test("429 is rate_limited with its Retry-After and the other statuses are provider_error", () => {
    const limited = callError(
      apiError(429, { headers: { "retry-after": "7" } })
    )
    expect(limited.failure).toBe("rate_limited")
    expect(limited.status).toBe(429)
    expect(limited.retryAfterMs).toBe(7_000)
    for (const status of [400, 401, 500, 529]) {
      const err = callError(apiError(status))
      expect(err.failure).toBe("provider_error")
      expect(err.status).toBe(status)
    }
  })

  test("type, code, param and request id are kept only when plain tokens", () => {
    const kept = callError(
      apiError(400, {
        type: "invalid_request_error",
        code: "context_length_exceeded",
        param: "messages.0",
        headers: { "Request-Id": "req_01ABC" },
      })
    )
    expect(kept).toMatchObject({
      type: "invalid_request_error",
      code: "context_length_exceeded",
      param: "messages.0",
      requestId: "req_01ABC",
    })
    const dropped = callError(
      apiError(400, {
        type: "has space",
        code: "x".repeat(129),
        param: "line\nbreak",
        headers: { "request-id": "bad id" },
      })
    )
    expect(dropped).toMatchObject({
      type: "",
      code: "",
      param: "",
      requestId: "",
    })
  })

  test("a numeric code is kept as its digits", () => {
    expect(callError(apiError(400, { code: 429 })).code).toBe("429")
  })

  test("the request id header is the provider's", () => {
    const err = callError(
      apiError(500, { headers: { "x-request-id": "req_9" } }),
      context({ requestIdHeader: "x-request-id" })
    )
    expect(err.requestId).toBe("req_9")
  })

  test("a body that only parses as JSON still yields its fields", () => {
    const error = new APICallError({
      message: "m",
      url: "u",
      requestBodyValues: {},
      statusCode: 400,
      responseBody:
        '{"error":{"type":"invalid_request_error","message":"parsed"}}',
    })
    const err = callError(error, context({ logProviderErrors: true }))
    expect(err.type).toBe("invalid_request_error")
    expect(err.providerMessage).toBe("parsed")
  })

  test("an error with no status is a provider_error with no detail", () => {
    const err = callError(new TypeError("fetch failed"))
    expect(err).toMatchObject({
      failure: "provider_error",
      status: 0,
      idle: false,
    })
    expect(err.message).toBe("llm provider error: anthropic request failed")
  })

  test("an aborted call is a call_timeout", () => {
    const call = AbortSignal.abort(
      new DOMException("timed out", "TimeoutError")
    )
    const err = callError(new Error("aborted"), context({ call }))
    expect(err).toMatchObject({
      failure: "call_timeout",
      status: 0,
      idle: false,
    })
  })

  test("an aborted turn gives back its reason, not a call error", () => {
    const reason = new Error("run ended")
    const turn = AbortSignal.abort(reason)
    expect(toCallError(new Error("x"), context({ turn }), undefined)).toBe(
      reason
    )
  })

  test("an idle failure wins over the error", () => {
    const idle = new CallError("anthropic", "call_timeout", 0, true)
    expect(toCallError(apiError(500), context(), idle)).toBe(idle)
  })
})

test("isPlainToken accepts the Go token alphabet only", () => {
  expect(isPlainToken("a_b.c:d-e9")).toBe(true)
  expect(isPlainToken("")).toBe(false)
  expect(isPlainToken("a b")).toBe(false)
  expect(isPlainToken("a".repeat(128))).toBe(true)
  expect(isPlainToken("a".repeat(129))).toBe(false)
})
