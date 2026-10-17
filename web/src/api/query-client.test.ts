import { describe, expect, test } from "vitest"

import { ApiError } from "./errors"
import { createQueryClient } from "./query-client"

function queryDefaults() {
  const queries = createQueryClient().getDefaultOptions().queries
  if (typeof queries?.retry !== "function") throw new Error("retry missing")
  if (typeof queries.retryDelay !== "function") {
    throw new Error("retryDelay missing")
  }
  return { retry: queries.retry, retryDelay: queries.retryDelay }
}

describe("createQueryClient", () => {
  test("retries a transient failure at most twice", () => {
    const { retry } = queryDefaults()
    const unavailable = new ApiError(503, "unavailable", "try later")
    expect(retry(0, unavailable)).toBe(true)
    expect(retry(1, unavailable)).toBe(true)
    expect(retry(2, unavailable)).toBe(false)
  })

  test("never retries a client error other than 429", () => {
    const { retry } = queryDefaults()
    for (const status of [400, 401, 403, 404, 409]) {
      expect(retry(0, new ApiError(status, "x", "x"))).toBe(false)
    }
    expect(retry(0, new ApiError(429, "x", "x"))).toBe(true)
  })

  test("waits for Retry-After when the server sent one", () => {
    const { retryDelay } = queryDefaults()
    expect(
      retryDelay(0, new ApiError(429, "x", "x", [], undefined, 2500))
    ).toBe(2500)
  })

  test("backs off exponentially up to eight seconds otherwise", () => {
    const { retryDelay } = queryDefaults()
    const error = new ApiError(503, "x", "x")
    expect([0, 1, 2, 3, 4, 10].map((n) => retryDelay(n, error))).toEqual([
      1000, 2000, 4000, 8000, 8000, 8000,
    ])
  })

  test("never retries a mutation", () => {
    expect(createQueryClient().getDefaultOptions().mutations?.retry).toBe(false)
  })
})
