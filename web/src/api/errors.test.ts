import { describe, expect, test } from "vitest"

import {
  ApiError,
  failedQueryText,
  isRetryable,
  maxRetryDelayMs,
  NetworkError,
  toApiError,
  unwrap,
} from "./errors"

function response(status: number, headers?: Record<string, string>) {
  return new Response(null, { status, headers })
}

describe("toApiError", () => {
  test("reads the documented error fields", () => {
    const error = toApiError(response(409), {
      error: "stale version",
      code: "version_conflict",
      latest_version: 4,
      issues: [{ path: "/panels/0", code: "required", message: "missing" }],
    })
    expect(error).toBeInstanceOf(ApiError)
    expect(error.status).toBe(409)
    expect(error.code).toBe("version_conflict")
    expect(error.message).toBe("stale version")
    expect(error.latestVersion).toBe(4)
    expect(error.issues).toEqual([
      { path: "/panels/0", code: "required", message: "missing" },
    ])
  })

  test("falls back to defaults for a body that is not an error object", () => {
    for (const body of [undefined, null, "oops", 42, ["x"]]) {
      const error = toApiError(response(500), body)
      expect(error.message).toBe("Request failed (500)")
      expect(error.code).toBe("unknown")
      expect(error.issues).toEqual([])
      expect(error.latestVersion).toBeUndefined()
    }
  })

  test("ignores fields with the wrong type", () => {
    const error = toApiError(response(400), {
      error: 7,
      code: false,
      issues: [{ path: "/a" }, "x", null],
      latest_version: "3",
    })
    expect(error.message).toBe("Request failed (400)")
    expect(error.code).toBe("unknown")
    expect(error.issues).toEqual([])
    expect(error.latestVersion).toBeUndefined()
  })

  test("reads the failed query's ref", () => {
    const error = toApiError(response(502), {
      error: "Datasource unavailable",
      code: "datasource_unavailable",
      ref: "B",
    })
    expect(error.ref).toBe("B")
  })

  test("leaves ref unset when it is missing, empty or not a string", () => {
    for (const ref of [undefined, "", 7, null]) {
      expect(toApiError(response(502), { error: "x", ref }).ref).toBeUndefined()
    }
  })

  test("converts Retry-After seconds to milliseconds", () => {
    expect(
      toApiError(response(429, { "Retry-After": "3" }), {}).retryAfterMs
    ).toBe(3000)
  })

  test("ignores a Retry-After that is not a number of seconds", () => {
    for (const value of ["", "soon", "-1"]) {
      expect(
        toApiError(response(429, { "Retry-After": value }), {}).retryAfterMs
      ).toBeUndefined()
    }
    expect(toApiError(response(429), {}).retryAfterMs).toBeUndefined()
  })
})

describe("unwrap", () => {
  test("returns data for a successful result", () => {
    expect(unwrap({ data: { id: "a" }, response: response(200) })).toEqual({
      id: "a",
    })
  })

  test("throws an ApiError for an error result", () => {
    const attempt = () =>
      unwrap({
        error: { error: "not found", code: "not_found" },
        response: response(404),
      })
    expect(attempt).toThrow(ApiError)
    expect(attempt).toThrow("not found")
  })

  test("throws when the response is not ok even without an error body", () => {
    expect(() => unwrap({ response: response(502) })).toThrow(
      "Request failed (502)"
    )
  })
})

describe("isRetryable", () => {
  test("retries network failures", () => {
    const cause = new TypeError("Failed to fetch")
    expect(isRetryable(new NetworkError(cause.message, { cause }))).toBe(true)
  })

  test("does not retry a TypeError thrown by application code", () => {
    expect(isRetryable(new TypeError("x is not a function"))).toBe(false)
  })

  test("retries a Retry-After up to the ceiling and no longer", () => {
    const withDelay = (ms: number) =>
      new ApiError(429, "x", "x", [], undefined, ms)
    expect(isRetryable(withDelay(0))).toBe(true)
    expect(isRetryable(withDelay(maxRetryDelayMs))).toBe(true)
    expect(isRetryable(withDelay(maxRetryDelayMs + 1))).toBe(false)
  })

  test("retries 429, 502, 503 and 504 only", () => {
    for (const status of [429, 502, 503, 504]) {
      expect(isRetryable(new ApiError(status, "x", "x"))).toBe(true)
    }
    for (const status of [400, 401, 403, 404, 409, 422, 500]) {
      expect(isRetryable(new ApiError(status, "x", "x"))).toBe(false)
    }
  })

  test("does not retry other errors", () => {
    expect(isRetryable(new Error("boom"))).toBe(false)
    expect(isRetryable("boom")).toBe(false)
  })
})

describe("failedQueryText", () => {
  test("names the query when the error has a ref", () => {
    const error = new ApiError(
      502,
      "datasource_unavailable",
      "Datasource unavailable",
      [],
      undefined,
      undefined,
      "A"
    )
    expect(failedQueryText(error)).toBe(
      "Query A failed: datasource unavailable"
    )
  })

  test("is the message when the error has no ref", () => {
    expect(
      failedQueryText(new ApiError(502, "x", "Datasource unavailable"))
    ).toBe("Datasource unavailable")
  })
})
