import { afterEach, describe, expect, test, vi } from "vitest"

import { api, setRoleRefusedHandler, setUnauthorizedHandler } from "./client"
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

  test("tells the role handler about a forbidden_role refusal and no other 403", async () => {
    const handler = vi.fn()
    setRoleRefusedHandler(handler)
    const reply = (body: object) =>
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify(body), {
          status: 403,
          headers: { "Content-Type": "application/json" },
        })
      )
    vi.stubGlobal("fetch", reply({ error: "no", code: "csrf_rejected" }))
    await api.GET("/auth/session", { baseUrl })
    expect(handler).not.toHaveBeenCalled()
    vi.stubGlobal("fetch", reply({ error: "no", code: "forbidden_role" }))
    await api.GET("/auth/session", { baseUrl })
    expect(handler).toHaveBeenCalledTimes(1)
    setRoleRefusedHandler(() => {})
  })

  test("tells the unauthorized handler about a 401 once and about no other status", async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    const reply = (status: number) =>
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "no", code: "x" }), {
          status,
          headers: { "Content-Type": "application/json" },
        })
      )
    for (const status of [200, 403]) {
      vi.stubGlobal("fetch", reply(status))
      await api.GET("/auth/session", { baseUrl })
      expect(handler).not.toHaveBeenCalled()
    }
    vi.stubGlobal("fetch", reply(401))
    await api.GET("/auth/session", { baseUrl })
    expect(handler).toHaveBeenCalledTimes(1)
    setUnauthorizedHandler(() => {})
  })
})
