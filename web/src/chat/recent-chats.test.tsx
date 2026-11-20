import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { RecentChats } from "./recent-chats"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => vi.fn(),
}))

function turn(id: string, source: string, title: string, createdAt: string) {
  return {
    id,
    kind: "build",
    title,
    pinned_at: null,
    source,
    trigger: "person",
    prompt: title,
    status: "succeeded",
    state: "succeeded",
    result_kind: "version",
    allowed_actions: [],
    dashboard_id: "d1",
    dashboard_title: "Checkout health",
    panel_id: null,
    result_version: null,
    url: null,
    error: null,
    failure_code: null,
    build: null,
    build_saved_at: null,
    continues_id: null,
    continued_by: null,
    retried_by: null,
    continue_note: null,
    observation_id: null,
    observation_title: null,
    chat_id: "t1",
    origin: "web",
    origin_client: null,
    created_at: createdAt,
    started_at: null,
    finished_at: null,
    requested_by: { kind: "user", id: "u1", name: "Ada" },
  }
}

afterEach(() => vi.unstubAllGlobals())

describe("RecentChats", () => {
  it("names a chat by its latest request and lists every source", async () => {
    const urls: string[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string, init?: RequestInit) => {
        const req = input instanceof Request ? input : new Request(input, init)
        urls.push(req.url)
        return new Response(
          JSON.stringify({
            requests: [
              turn("t2", "prompt", "undo that", "2026-11-02T10:00:00Z"),
              turn("t1", "observe", "Checkout health", "2026-11-01T10:00:00Z"),
            ],
            next_cursor: null,
            summary: { total: 2, people: null },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } }
        )
      })
    )
    renderWithProviders(<RecentChats />, { route: { path: "/", url: "/" } })
    expect(
      await screen.findByText("Revised Checkout health")
    ).toBeInTheDocument()
    expect(screen.queryByText("Saved Checkout health")).not.toBeInTheDocument()
    expect(urls.every((url) => !url.includes("source="))).toBe(true)
  })
})
