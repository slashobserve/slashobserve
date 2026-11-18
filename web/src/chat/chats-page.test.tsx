import { screen, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { ChatsPage } from "./chats-page"

const session = vi.hoisted(() => ({ role: "editor", id: "u1" }))

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({
    principal: { id: session.id, role: session.role },
  }),
}))

function request(id: string, requester: string, title: string) {
  return {
    id,
    kind: "build",
    title,
    pinned_at: null,
    source: "observe",
    trigger: "person",
    prompt: title,
    status: "succeeded",
    state: "succeeded",
    result_kind: "observation",
    allowed_actions: [],
    dashboard_id: null,
    dashboard_title: null,
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
    observation_id: "o1",
    observation_title: "p99 latency",
    chat_id: id,
    origin: "web",
    origin_client: null,
    created_at: new Date().toISOString(),
    started_at: null,
    finished_at: null,
    requested_by: { kind: "user", id: requester, name: "Someone" },
  }
}

function stubList(requests: unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            requests,
            next_cursor: null,
            summary: { total: requests.length, people: null },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } }
        )
    )
  )
}

function render() {
  return renderWithProviders(<ChatsPage search={{}} onSearch={vi.fn()} />, {
    route: { path: "/", url: "/" },
  })
}

beforeEach(() => {
  session.role = "editor"
  session.id = "u1"
})

afterEach(() => vi.unstubAllGlobals())

describe("ChatsPage rows", () => {
  it("gives a chat the same actions as the History panel", async () => {
    stubList([request("r1", "u1", "what does p99 mean")])
    const { user } = render()
    const link = await screen.findByRole("link", { name: /what does p99 mean/ })
    const row = link.parentElement as HTMLElement
    expect(
      within(row).getByRole("button", { name: "Save prompt" })
    ).toBeInTheDocument()
    await user.click(
      within(row).getByRole("button", {
        name: "Chat actions: what does p99 mean",
      })
    )
    expect(
      await screen.findByRole("menuitem", { name: "Rename" })
    ).toBeInTheDocument()
    expect(screen.getByRole("menuitem", { name: "Pin" })).toBeInTheDocument()
    expect(screen.getByRole("menuitem", { name: "Delete" })).toBeInTheDocument()
  })

  it("words the delete dialog for the answer the chat left", async () => {
    stubList([request("r1", "u1", "what does p99 mean")])
    const { user } = render()
    await user.click(
      await screen.findByRole("button", {
        name: "Chat actions: what does p99 mean",
      })
    )
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }))
    expect(
      await screen.findByText(
        /The answer stays in the observation "p99 latency"/
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(/The dashboard it made stays/)).toBeNull()
  })

  it("offers no actions on a chat someone else asked", async () => {
    session.role = "admin"
    stubList([request("r2", "u2", "their question")])
    render()
    await screen.findByRole("link", { name: /their question/ })
    expect(
      screen.queryByRole("button", { name: /Chat actions/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Save prompt" })
    ).not.toBeInTheDocument()
  })

  it("keeps Save prompt from a Viewer, who cannot save one", async () => {
    session.role = "viewer"
    stubList([request("r1", "u1", "what does p99 mean")])
    render()
    await screen.findByRole("link", { name: /what does p99 mean/ })
    expect(
      screen.queryByRole("button", { name: "Save prompt" })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Chat actions: what does p99 mean" })
    ).toBeInTheDocument()
  })
})
