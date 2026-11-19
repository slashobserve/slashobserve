import { waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { MadeByMenu } from "./list-filter-menus"

const session = vi.hoisted(() => ({ role: "admin" }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: session.role } }),
}))

function stubUsers() {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async () =>
      new Response(JSON.stringify({ users: [], next_cursor: null }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("MadeByMenu", () => {
  it("lists people for an Admin", async () => {
    session.role = "admin"
    const fetch = stubUsers()
    renderWithProviders(<MadeByMenu value={undefined} onChange={() => {}} />)
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce())
    expect(new URL(fetch.mock.calls[0]![0].url).pathname).toBe("/api/v1/users")
  })

  it.each(["editor", "commenter", "viewer"])(
    "does not ask a %s for the list of people",
    async (role) => {
      session.role = role
      const fetch = stubUsers()
      renderWithProviders(<MadeByMenu value={undefined} onChange={() => {}} />)
      await new Promise((resolve) => setTimeout(resolve, 50))
      expect(fetch).not.toHaveBeenCalled()
    }
  )
})
