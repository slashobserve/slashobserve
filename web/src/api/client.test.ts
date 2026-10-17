import { afterEach, describe, expect, test, vi } from "vitest"

import { api } from "./client"
import { NetworkError } from "./errors"

const baseUrl = "http://localhost/api/v1"

describe("api client", () => {
  afterEach(() => vi.unstubAllGlobals())

  test("wraps a fetch TypeError in a NetworkError", async () => {
    const cause = new TypeError("Failed to fetch")
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(cause))
    const attempt = api.GET("/auth/session", { baseUrl })
    await expect(attempt).rejects.toBeInstanceOf(NetworkError)
    await expect(attempt).rejects.toMatchObject({ cause })
  })

  test("lets other fetch failures through unchanged", async () => {
    const abort = new DOMException("aborted", "AbortError")
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(abort))
    await expect(api.GET("/auth/session", { baseUrl })).rejects.toBe(abort)
  })
})
