import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { RecentChats } from "./recent-chats"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => vi.fn(),
}))

afterEach(() => vi.unstubAllGlobals())

describe("RecentChats", () => {
  it("lists one row per chat with its turns and dashboard", async () => {
    const urls: string[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string, init?: RequestInit) => {
        const req = input instanceof Request ? input : new Request(input, init)
        urls.push(req.url)
        const body = req.url.includes("/api/v1/dashboards")
          ? {
              dashboards: [
                {
                  id: "d1",
                  title: "Checkout health",
                  permissions: { edit: true },
                  updated_by: { id: "u1" },
                },
              ],
            }
          : {
              chats: [
                {
                  id: "t1",
                  title: "Checkout health dashboard",
                  dashboard_id: "d1",
                  observation_id: null,
                  pinned_at: null,
                  created_at: "2026-11-01T10:00:00Z",
                  last_turn_at: "2026-11-02T10:00:00Z",
                  turns: 3,
                  last_turn: { id: "t3", state: "succeeded", prompt: "x" },
                  owner: { kind: "user", name: "Ada" },
                  read_only: false,
                },
              ],
              next_cursor: null,
            }
        return new Response(JSON.stringify(body), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      })
    )
    renderWithProviders(<RecentChats />, { route: { path: "/", url: "/" } })
    expect(
      await screen.findByText("Checkout health dashboard")
    ).toBeInTheDocument()
    expect(screen.getByText("3 turns")).toBeInTheDocument()
    expect(await screen.findByText("Checkout health")).toBeInTheDocument()
    expect(urls.some((url) => url.includes("/api/v1/chats?limit=5"))).toBe(true)
  })
})
