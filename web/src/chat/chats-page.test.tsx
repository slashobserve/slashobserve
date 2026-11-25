import { screen, waitFor, within } from "@testing-library/react"
import { toast } from "sonner"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { Chat } from "./chats-queries"
import { ChatsPage } from "./chats-page"
import type { ChatsSearch } from "./chats-search"

const session = vi.hoisted(() => ({ role: "editor", id: "u1" }))
const observation = vi.hoisted(() => ({ allowed: [] as string[] }))

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({
    principal: { id: session.id, role: session.role },
  }),
}))

const dashboardId = "5b0e3c55-6a8e-4e1f-9d0a-0d6f1f6f7f10"

function chat(id: string, title: string, overrides: Partial<Chat> = {}): Chat {
  return {
    id,
    title,
    dashboard_id: null,
    dashboard_title: null,
    observation_id: null,
    pinned_at: null,
    created_at: "2026-11-21T09:00:00Z",
    last_turn_at: "2026-11-21T10:00:00Z",
    turns: 1,
    last_turn: {
      id,
      state: "succeeded",
      prompt: title,
      investigation: false,
      subject_observation_id: null,
      anchor: null,
      block_id: null,
    },
    owner: { kind: "user", name: "Ada" },
    read_only: false,
    ...overrides,
  }
}

function turn(id: string, title: string) {
  return {
    id,
    kind: "build",
    title,
    pinned_at: null,
    intent: "apply",
    base_version: null,
    comment_id: null,
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
    chat_id: "c1",
    origin: "web",
    origin_client: null,
    created_at: "2026-11-21T09:00:00Z",
    started_at: null,
    finished_at: null,
    requested_by: { kind: "user", id: "u1", name: "Ada" },
  }
}

type Call = { method: string; url: string; query: string; body: string }

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stub(
  chats: Chat[],
  {
    turns = [turn("c1", "what does p99 mean")],
    written = { request: {} },
  }: { turns?: unknown[]; written?: unknown } = {}
) {
  const calls: Call[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(input, init)
      const url = new URL(req.url)
      calls.push({
        method: req.method,
        url: url.pathname,
        query: url.search,
        body: req.method === "GET" ? "" : await req.text(),
      })
      if (req.method === "DELETE") return new Response(null, { status: 204 })
      if (req.method !== "GET") return json(written)
      const observationId = /^\/api\/v1\/observations\/([^/]+)$/.exec(
        url.pathname
      )?.[1]
      if (observationId !== undefined) {
        return json({
          observation: {
            id: observationId,
            title: "p99 latency",
            allowed_actions: observation.allowed,
          },
        })
      }
      if (url.pathname === "/api/v1/observe") {
        return json({
          requests: turns,
          next_cursor: null,
          summary: { total: turns.length, people: null },
        })
      }
      return json({ chats, next_cursor: null })
    })
  )
  return calls
}

function writes(calls: readonly Call[]) {
  return calls.filter((call) => call.method !== "GET")
}

function render(search: ChatsSearch = {}, onSearch = vi.fn()) {
  return renderWithProviders(
    <ChatsPage search={search} onSearch={onSearch} />,
    { route: { path: "/", url: "/" } }
  )
}

async function openMenu(title: string) {
  const { user } = render()
  await user.click(
    await screen.findByRole("button", { name: `Chat actions: ${title}` })
  )
  return user
}

beforeEach(() => {
  session.role = "editor"
  session.id = "u1"
  observation.allowed = []
})

afterEach(() => vi.unstubAllGlobals())

describe("ChatsPage rows", () => {
  it("says who can see your chats", async () => {
    stub([chat("c1", "Why did checkout errors rise")])
    render()
    expect(
      await screen.findByText("Only you and Admins can see your chats.")
    ).toBeVisible()
  })

  it("lists one row per chat with its dashboard, turns and link", async () => {
    stub([
      chat("c1", "Why did checkout errors rise", {
        dashboard_id: dashboardId,
        dashboard_title: "Checkout health",
        turns: 3,
      }),
      chat("c2", "Card declines by issuer", { turns: 2 }),
    ])
    render()
    const link = await screen.findByRole("link", {
      name: "Why did checkout errors rise",
    })
    expect(link).toHaveAttribute("href", "/c/c1")
    const row = link.closest('[role="row"]') as HTMLElement
    expect(await within(row).findByText("Checkout health")).toBeInTheDocument()
    expect(within(row).getByText("3")).toBeInTheDocument()
    const other = screen
      .getByRole("link", { name: "Card declines by issuer" })
      .closest('[role="row"]') as HTMLElement
    expect(within(other).getByText("No dashboard")).toBeInTheDocument()
    expect(screen.getByText("2 chats.")).toBeInTheDocument()
  })

  it("names each chat's dashboard by the title the server gives it", async () => {
    const unlisted = "6c1f4d66-7b9f-4f2a-8e7a-1e7f2f7f8f21"
    stub([
      chat("c1", "Beyond the first page", {
        dashboard_id: unlisted,
        dashboard_title: "Refund volume",
      }),
      chat("c2", "Lost access", {
        dashboard_id: dashboardId,
        dashboard_title: null,
      }),
    ])
    render()
    const row = (title: string) =>
      screen
        .getByRole("link", { name: title })
        .closest('[role="row"]') as HTMLElement
    await screen.findByRole("link", { name: "Beyond the first page" })
    expect(
      within(row("Beyond the first page")).getByText("Refund volume")
    ).toBeInTheDocument()
    expect(
      within(row("Lost access")).getByText("A dashboard you cannot see")
    ).toBeInTheDocument()
  })

  it("keeps the server's order, which puts your pinned chats first", async () => {
    stub([
      chat("c1", "Pinned older chat", { pinned_at: "2026-11-20T10:00:00Z" }),
      chat("c2", "Newest chat"),
      chat("c3", "Pinned on a later page", {
        pinned_at: "2026-11-21T08:00:00Z",
      }),
    ])
    render()
    await screen.findByRole("link", { name: /Pinned older chat/ })
    const titles = screen
      .getAllByRole("row")
      .filter((row) => row.hasAttribute("row-index"))
      .sort(
        (a, b) =>
          Number(a.getAttribute("row-index")) -
          Number(b.getAttribute("row-index"))
      )
      .map((row) => within(row).getAllByRole("link")[0].textContent)
    expect(titles).toEqual([
      "Pinned older chat",
      "Newest chat",
      "Pinned on a later page",
    ])
  })

  it("offers Rename, Pin and Delete on your own chat", async () => {
    stub([chat("c1", "what does p99 mean")])
    await openMenu("what does p99 mean")
    expect(
      await screen.findByRole("menuitem", { name: "Rename" })
    ).toBeInTheDocument()
    expect(screen.getByRole("menuitem", { name: "Pin" })).toBeInTheDocument()
    expect(screen.getByRole("menuitem", { name: "Delete" })).toBeInTheDocument()
  })

  it("offers no actions on a chat it reads only as an Admin", async () => {
    session.role = "admin"
    stub([chat("c2", "their question", { read_only: true })])
    render({ scope: "all" })
    await screen.findByRole("link", { name: /their question/ })
    expect(screen.getByText("Ada")).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: /Chat actions/ })
    ).not.toBeInTheDocument()
  })

  it("renames the chat through its first request", async () => {
    const calls = stub([chat("c1", "Checkout health", { turns: 2 })])
    const user = await openMenu("Checkout health")
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }))
    const field = await screen.findByRole("textbox", { name: "Chat title" })
    await user.clear(field)
    await user.type(field, "Checkout status{Enter}")
    await waitFor(() => expect(writes(calls)).toHaveLength(1))
    expect(writes(calls)[0]).toMatchObject({
      method: "PATCH",
      url: "/api/v1/observe/c1",
    })
  })

  it("says when the observation made from the chat keeps its title", async () => {
    const info = vi.spyOn(toast, "info")
    stub([chat("c1", "Checkout health")], {
      written: { request: { observation_rename: "kept" } },
    })
    const user = await openMenu("Checkout health")
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }))
    const field = await screen.findByRole("textbox", { name: "Chat title" })
    await user.clear(field)
    await user.type(field, "Checkout status{Enter}")
    await waitFor(() =>
      expect(info).toHaveBeenCalledWith(
        "The observation made from this chat keeps its title, because you cannot edit it."
      )
    )
    info.mockRestore()
  })

  it("words the delete dialog from the chat's turns and archives the chat", async () => {
    const calls = stub([chat("c1", "what does p99 mean")])
    const user = await openMenu("what does p99 mean")
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }))
    expect(
      await screen.findByText(
        /The answer stays in the observation "p99 latency"/
      )
    ).toBeInTheDocument()
    expect(
      calls.some(
        (call) =>
          call.url === "/api/v1/observe" && call.query.includes("chat_id=c1")
      )
    ).toBe(true)
    await user.click(screen.getByRole("button", { name: "Delete chat" }))
    await waitFor(() => expect(writes(calls)).toHaveLength(1))
    expect(writes(calls)[0]).toMatchObject({
      method: "DELETE",
      url: "/api/v1/chats/c1",
    })
  })

  it("deletes the saved observation first when the person chooses it", async () => {
    observation.allowed = ["delete"]
    const calls = stub([chat("c1", "what does p99 mean")])
    const user = await openMenu("what does p99 mean")
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }))
    await user.click(
      await screen.findByRole("checkbox", {
        name: 'Also delete the observation "p99 latency"',
      })
    )
    await user.click(
      screen.getByRole("button", { name: "Delete chat and observation" })
    )
    await waitFor(() => expect(writes(calls)).toHaveLength(2))
    expect(writes(calls).map((call) => `${call.method} ${call.url}`)).toEqual([
      "DELETE /api/v1/observations/o1",
      "DELETE /api/v1/chats/c1",
    ])
  })
})

describe("ChatsPage filters", () => {
  it("reads the list from GET /chats with the dashboard chosen", async () => {
    const calls = stub([
      chat("c1", "Why did checkout errors rise", {
        dashboard_id: dashboardId,
        dashboard_title: "Checkout health",
      }),
    ])
    render({ dashboard: dashboardId, q: "checkout" })
    await screen.findByRole("link", { name: "Why did checkout errors rise" })
    const list = calls.find((call) => call.url === "/api/v1/chats")
    expect(list?.query).toContain(`dashboard_id=${dashboardId}`)
    expect(list?.query).toContain("q=checkout")
    expect(list?.query).not.toContain("source=")
  })

  it("offers the dashboards of the listed chats", async () => {
    const onSearch = vi.fn()
    stub([
      chat("c1", "Why did checkout errors rise", {
        dashboard_id: dashboardId,
        dashboard_title: "Checkout health",
      }),
      chat("c2", "Card declines by issuer"),
    ])
    const { user } = render({}, onSearch)
    await screen.findByRole("link", { name: "Card declines by issuer" })
    await user.click(screen.getByRole("combobox", { name: "Dashboard" }))
    await user.click(
      await screen.findByRole("option", { name: "Dashboard: Checkout health" })
    )
    expect(onSearch).toHaveBeenCalledWith({ dashboard: dashboardId })
  })
})
