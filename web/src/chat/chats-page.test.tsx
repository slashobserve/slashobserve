import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { ChatsPage } from "./chats-page"

const session = vi.hoisted(() => ({ role: "editor", id: "u1" }))
const observation = vi.hoisted(() => ({ allowed: [] as string[] }))

function observationReply(url: string): Response | null {
  const path = new URL(url, "http://localhost").pathname
  const id = /^\/api\/v1\/observations\/([^/]+)$/.exec(path)?.[1]
  if (id === undefined) return null
  return new Response(
    JSON.stringify({
      observation: {
        id,
        title: "p99 latency",
        allowed_actions: observation.allowed,
      },
    }),
    { status: 200, headers: { "Content-Type": "application/json" } }
  )
}

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
      async (input: Request | string) =>
        observationReply(input instanceof Request ? input.url : input) ??
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

function stubRecordedList(requests: unknown[]) {
  const calls: { method: string; url: string; body: string }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(input, init)
      const method = req.method
      if (method !== "GET") {
        calls.push({
          method,
          url: new URL(req.url).pathname,
          body: await req.text(),
        })
        return new Response(JSON.stringify({}), {
          status: method === "DELETE" ? 204 : 200,
          headers: { "Content-Type": "application/json" },
        })
      }
      const reply = observationReply(req.url)
      if (reply !== null) return reply
      return new Response(
        JSON.stringify({
          requests,
          next_cursor: null,
          summary: { total: requests.length, people: null },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    })
  )
  return calls
}

function twoTurnChat() {
  return [
    {
      ...request("t2", "u1", "make it blue"),
      source: "prompt",
      dashboard_id: "d1",
      chat_id: "t1",
    },
    {
      ...request("t1", "u1", "Checkout health"),
      dashboard_id: "d1",
      chat_id: "t1",
    },
  ]
}

function render() {
  return renderWithProviders(<ChatsPage search={{}} onSearch={vi.fn()} />, {
    route: { path: "/", url: "/" },
  })
}

beforeEach(() => {
  session.role = "editor"
  session.id = "u1"
  observation.allowed = []
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
    expect(screen.queryByRole("checkbox")).toBeNull()
  })

  it("deletes the saved observation too when the person chooses it", async () => {
    observation.allowed = ["delete"]
    const calls = stubRecordedList([request("r1", "u1", "what does p99 mean")])
    const { user } = render()
    await user.click(
      await screen.findByRole("button", {
        name: "Chat actions: what does p99 mean",
      })
    )
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }))
    const box = await screen.findByRole("checkbox", {
      name: 'Also delete the observation "p99 latency"',
    })
    expect(box).not.toBeChecked()
    await user.click(box)
    expect(
      screen.getByText(/The observation "p99 latency" is deleted for everyone/)
    ).toBeInTheDocument()
    await user.click(
      screen.getByRole("button", { name: "Delete chat and observation" })
    )
    await waitFor(() => expect(calls).toHaveLength(2))
    expect(calls.map((call) => `${call.method} ${call.url}`)).toEqual([
      "DELETE /api/v1/observations/o1",
      "DELETE /api/v1/observe/r1",
    ])
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

  it("renames the first ask of a chat, whose title the row shows", async () => {
    const calls = stubRecordedList(twoTurnChat())
    const { user } = render()
    await user.click(
      await screen.findByRole("button", {
        name: "Chat actions: Checkout health",
      })
    )
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }))
    const field = await screen.findByRole("textbox", { name: "Chat title" })
    await user.clear(field)
    await user.type(field, "Checkout status{Enter}")
    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0].method).toBe("PATCH")
    expect(calls[0].url).toBe("/api/v1/observe/t1")
  })

  it("deletes every turn of the chat the row stands for", async () => {
    const calls = stubRecordedList(twoTurnChat())
    const { user } = render()
    await user.click(
      await screen.findByRole("button", {
        name: "Chat actions: Checkout health",
      })
    )
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }))
    expect(await screen.findByText(/All 2 messages/)).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Delete chat" }))
    await waitFor(() => expect(calls).toHaveLength(2))
    expect(calls.map((call) => call.url).sort()).toEqual([
      "/api/v1/observe/t1",
      "/api/v1/observe/t2",
    ])
  })

  it("saves the first prompt of the chat, not its latest message", async () => {
    stubList(twoTurnChat())
    render()
    const link = await screen.findByRole("link", { name: /Checkout health/ })
    expect(
      within(link.parentElement as HTMLElement).getByRole("button", {
        name: "Save prompt",
      })
    ).toBeInTheDocument()
  })
})

describe("ChatsPage footer", () => {
  it("counts the conversations listed, not their requests", async () => {
    stubList([...twoTurnChat(), request("r3", "u1", "what does p99 mean")])
    render()
    expect(await screen.findByText("2 chats.")).toBeInTheDocument()
  })
})
