import { describe, expect, it, test, vi } from "vitest"

import {
  ApiError,
  failedQueryText,
  isRetryable,
  maxRetryDelayMs,
  NetworkError,
  problemText,
  resendOnceOffline,
  sentence,
  toApiError,
  unreachableText,
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

  test("reads when an open request was made", () => {
    expect(
      toApiError(response(409), {
        error: "open",
        code: "request_open",
        requested_at: "2026-11-20T09:00:00Z",
      }).requestedAt
    ).toBe("2026-11-20T09:00:00Z")
    expect(
      toApiError(response(409), { error: "x", code: "y" }).requestedAt
    ).toBe(undefined)
  })

  test("says which folder level or role a forbidden_role refusal needs", () => {
    const refusal = {
      error: "your role does not allow this",
      code: "forbidden_role",
    }
    expect(
      toApiError(response(403), {
        ...refusal,
        needed_access: "edit",
        folder_title: "Payments",
      }).message
    ).toBe("your role does not allow this; it needs Edit on Payments")
    expect(
      toApiError(response(403), { ...refusal, needed_access: "manage" }).message
    ).toBe("your role does not allow this; it needs Manage on the folder")
    expect(
      toApiError(response(403), { ...refusal, needed_role: "editor" }).message
    ).toBe("your role does not allow this; it needs the Editor role")
    expect(toApiError(response(403), refusal).message).toBe(refusal.error)
  })

  test("leaves the message of any other code alone", () => {
    expect(
      toApiError(response(404), {
        error: "gone",
        code: "not_found",
        needed_access: "edit",
      }).message
    ).toBe("gone")
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

  test("reads the merge conflicts of a stale save", () => {
    const conflict = {
      kind: "panel",
      id: "latency",
      reason: "both_changed",
      message: "Both changed the query",
    }
    const error = toApiError(response(409), {
      error: "stale",
      code: "version_conflict",
      latest_version: 4,
      conflicts: [
        conflict,
        { kind: "panel" },
        "x",
        { ...conflict, kind: "row" },
      ],
    })
    expect(error.conflicts).toEqual([conflict])
  })

  test("has no conflicts when the field is missing or not a list", () => {
    for (const conflicts of [undefined, null, "x", {}]) {
      expect(toApiError(response(409), { conflicts }).conflicts).toEqual([])
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

  test("does not retry a query that timed out on the server", () => {
    expect(isRetryable(new ApiError(504, "query_timeout", "timed out"))).toBe(
      false
    )
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

describe("resendOnceOffline", () => {
  it("resends once after a network failure when the send has a client id", async () => {
    const send = vi
      .fn<() => Promise<string>>()
      .mockRejectedValueOnce(new NetworkError("offline"))
      .mockResolvedValueOnce("sent")
    await expect(resendOnceOffline("k1", send)).resolves.toBe("sent")
    expect(send).toHaveBeenCalledTimes(2)
  })

  it("does not resend without a client id or after a server answer", async () => {
    const offline = vi
      .fn<() => Promise<string>>()
      .mockRejectedValue(new NetworkError("offline"))
    await expect(resendOnceOffline(undefined, offline)).rejects.toThrow(
      NetworkError
    )
    expect(offline).toHaveBeenCalledOnce()
    const refused = vi
      .fn<() => Promise<string>>()
      .mockRejectedValue(new Error("refused"))
    await expect(resendOnceOffline("k1", refused)).rejects.toThrow("refused")
    expect(refused).toHaveBeenCalledOnce()
  })
})

describe("sentence", () => {
  it("capitalizes and ends with a period", () => {
    expect(sentence("prompt is too long")).toBe("Prompt is too long.")
  })

  it("keeps existing terminal punctuation", () => {
    expect(sentence("Nope!")).toBe("Nope!")
    expect(sentence("really?")).toBe("Really?")
  })

  it("trims trailing whitespace and returns empty text unchanged", () => {
    expect(sentence("done  \n")).toBe("Done.")
    expect(sentence("  ")).toBe("")
  })
})

describe("problemText", () => {
  const texts = {
    codes: {
      taken: "That name is taken.",
      changed: (error: ApiError) => `${error.message} Reload it.`,
    },
    notFound: "This thing no longer exists.",
    fallback: "The thing could not be saved.",
  }

  it("reads a code from the table before the status", () => {
    expect(problemText(new ApiError(404, "taken", "x"), texts)).toBe(
      "That name is taken."
    )
    expect(
      problemText(new ApiError(409, "changed", "It changed."), texts)
    ).toBe("It changed. Reload it.")
  })

  it("names a missing thing on a 404 without a listed code", () => {
    expect(problemText(new ApiError(404, "not_found", "x"), texts)).toBe(
      "This thing no longer exists."
    )
  })

  it("falls back to the server's sentence, then to the fallback", () => {
    expect(problemText(new ApiError(422, "bad", "name is bad"), texts)).toBe(
      "Name is bad."
    )
    expect(problemText(new ApiError(500, "internal", " "), texts)).toBe(
      "The thing could not be saved."
    )
    expect(problemText(new Error("boom"), texts)).toBe(
      "The thing could not be saved."
    )
    expect(problemText(null)).toBe("Something went wrong. Try again.")
  })

  it("says the server is unreachable on a network failure", () => {
    expect(problemText(new NetworkError("Failed to fetch"), texts)).toBe(
      unreachableText
    )
  })

  it("does not read inherited keys as codes", () => {
    expect(problemText(new ApiError(400, "toString", "bad input"))).toBe(
      "Bad input."
    )
  })
})
