import { afterEach, describe, expect, it, vi } from "vitest"
import { QueryClient } from "@tanstack/react-query"

import { catchUpWithSession } from "./session-sync"

const principal = {
  kind: "user",
  id: "u1",
  name: "Una",
  email: "una@example.com",
}

function serveRole(role: string) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json({ authenticated: true, principal: { ...principal, role } })
    )
  )
}

afterEach(() => vi.unstubAllGlobals())

describe("catchUpWithSession", () => {
  it("reloads the route and every query once the person's role has changed", async () => {
    const router = { invalidate: vi.fn(() => Promise.resolve()) }
    const queryClient = new QueryClient()
    const invalidate = vi.spyOn(queryClient, "invalidateQueries")
    serveRole("editor")
    catchUpWithSession(router, queryClient)
    await vi.waitFor(() => expect(router.invalidate).toHaveBeenCalledOnce())
    expect(invalidate).toHaveBeenCalledOnce()
    serveRole("editor")
    catchUpWithSession(router, queryClient)
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce())
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(router.invalidate).toHaveBeenCalledOnce()
  })
})
