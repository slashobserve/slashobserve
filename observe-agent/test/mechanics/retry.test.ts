import { describe, expect, test } from "bun:test"
import {
  baseBackoffMs,
  maxRetryAfterMs,
  parseRetryAfter,
  retry,
  type RetryEnv,
} from "../../src/session/mechanics/retry"
import { CallError, type CallFailure } from "../../src/session/types"

function recordingEnv(random = 0.5): RetryEnv & { waits: number[] } {
  const waits: number[] = []
  return {
    waits,
    sleep: async (ms) => {
      waits.push(ms)
    },
    random: () => random,
    now: () => 0,
  }
}

function failing(status: number, failure: CallFailure, retryAfterMs = 0) {
  return new CallError("anthropic", failure, status, false, { retryAfterMs })
}

function attempts(...errors: unknown[]) {
  let calls = 0
  const attempt = async (): Promise<string> => {
    calls++
    const error = errors[calls - 1]
    if (error !== undefined) {
      throw error
    }
    return "ok"
  }
  return { attempt, calls: () => calls }
}

const live = new AbortController().signal

describe("retry retries only retryable failures", () => {
  const idle = new CallError("anthropic", "call_timeout", 0, true)
  const cases: Array<[string, unknown, number]> = [
    ["rate limited", failing(429, "rate_limited"), 4],
    ["idle timeout", idle, 4],
    ["request timeout status", failing(408, "provider_error"), 4],
    ["internal error", failing(500, "provider_error"), 4],
    ["bad gateway", failing(502, "provider_error"), 4],
    ["unavailable", failing(503, "provider_error"), 4],
    ["gateway timeout", failing(504, "provider_error"), 4],
    ["overloaded", failing(529, "provider_error"), 4],
    ["connection error", failing(0, "provider_error"), 4],
    ["bad request", failing(400, "provider_error"), 1],
    ["unauthorized", failing(401, "provider_error"), 1],
    ["forbidden", failing(403, "provider_error"), 1],
    ["not found", failing(404, "provider_error"), 1],
    ["our own call timeout", failing(0, "call_timeout"), 1],
    ["not a call error", new Error("boom"), 1],
  ]
  test.each(cases)("%s", async (_name, error, wantCalls) => {
    const env = recordingEnv()
    const { attempt, calls } = attempts(error, error, error, error, error)
    await expect(retry(attempt, live, { env })).rejects.toBe(error)
    expect(calls()).toBe(wantCalls)
    expect(env.waits).toHaveLength(wantCalls - 1)
  })
})

test("a later attempt succeeds and the waits back off with jitter", async () => {
  const bounds = [
    [500, 1000],
    [1000, 2000],
    [2000, 4000],
  ] as const
  for (const random of [0, 0.5, 0.999]) {
    const env = recordingEnv(random)
    const error = failing(503, "provider_error")
    const { attempt, calls } = attempts(error, error, error)
    const got = await retry(attempt, live, { env })
    expect(got).toEqual({ value: "ok", attempts: 4 })
    expect(calls()).toBe(4)
    expect(env.waits).toHaveLength(3)
    for (const [i, wait] of env.waits.entries()) {
      expect(wait).toBeGreaterThanOrEqual(bounds[i]![0])
      expect(wait).toBeLessThanOrEqual(bounds[i]![1])
    }
  }
})

describe("retry honors Retry-After up to its cap", () => {
  const cases: Array<[string, number, number]> = [
    ["larger than the backoff", 20_000, 20_000],
    ["capped", 300_000, maxRetryAfterMs],
  ]
  test.each(cases)("%s", async (_name, retryAfterMs, want) => {
    const env = recordingEnv()
    const { attempt } = attempts(failing(429, "rate_limited", retryAfterMs))
    await retry(attempt, live, { env })
    expect(env.waits).toEqual([want])
  })

  test("smaller than the backoff is ignored", async () => {
    const env = recordingEnv(0)
    const { attempt } = attempts(failing(429, "rate_limited", 1))
    await retry(attempt, live, { env })
    expect(env.waits).toEqual([baseBackoffMs / 2])
  })
})

test("retry stops before a wait that passes the deadline", async () => {
  const env = recordingEnv(0)
  const error = failing(503, "provider_error")
  const { attempt, calls } = attempts(error, error)
  await expect(retry(attempt, live, { env, deadline: 200 })).rejects.toBe(error)
  expect(calls()).toBe(1)
  expect(env.waits).toHaveLength(0)
})

test("retry never retries a cancel", async () => {
  const env = recordingEnv()
  const controller = new AbortController()
  let calls = 0
  const error = failing(503, "provider_error")
  await expect(
    retry(
      async () => {
        calls++
        controller.abort()
        throw error
      },
      controller.signal,
      { env }
    )
  ).rejects.toBe(error)
  expect(calls).toBe(1)
  expect(env.waits).toHaveLength(0)
})

test("retry stops when the wait is interrupted", async () => {
  const env: RetryEnv = {
    sleep: async () => {
      throw new Error("interrupted")
    },
    random: () => 0,
    now: () => 0,
  }
  const error = failing(503, "provider_error")
  const { attempt, calls } = attempts(error, error)
  await expect(retry(attempt, live, { env })).rejects.toBe(error)
  expect(calls()).toBe(1)
})

describe("parseRetryAfter", () => {
  const cases: Array<[string | undefined, number]> = [
    ["7", 7_000],
    ["0", 0],
    ["-3", 0],
    ["", 0],
    [undefined, 0],
    ["Wed, 21 Oct 2026 07:28:00 GMT", 0],
    ["99999999999999999999", 0],
    ["86400", maxRetryAfterMs],
  ]
  test.each(cases)("%p", (header, want) => {
    expect(parseRetryAfter(header)).toBe(want)
  })
})
