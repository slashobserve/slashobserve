import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { InboxFilter, InboxView } from "@/api/keys"
import { renderWithProviders } from "@/test/render"

import type { InboxItem } from "./inbox-item"
import { InboxPage, type InboxState } from "./inbox-page"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock("sonner", () => ({ toast: toasts }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

const dashboardId = "a1b2c3d4-0b5a-4c7d-9e11-2a6b8c4d5e71"
const proposalId = "3f2c8d1e-0b5a-4c7d-9e11-2a6b8c4d5e70"

const ada = {
  kind: "user" as const,
  id: "5f0a8e3c-2b7d-4e0b-a1c4-7d6f0e9b2c11",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function withData(data: Record<string, unknown>): InboxItem["data"] {
  return data as InboxItem["data"]
}

function item(over: Partial<InboxItem>): InboxItem {
  return {
    seq: 1,
    type: "dashboard.version_created",
    target: { kind: "dashboard", id: dashboardId },
    dashboard_id: dashboardId,
    folder_id: null,
    actor: ada,
    data: withData({ title: "Checkout" }),
    created_at: new Date().toISOString(),
    reason: null,
    unread: false,
    target_title: null,
    excerpt: null,
    question: null,
    ...over,
  }
}

const needsYou = item({
  seq: 12,
  type: "thread.mentioned",
  target: { kind: "thread", id: proposalId },
  data: withData({ object: { kind: "dashboard", id: dashboardId } }),
  reason: "mention",
  unread: true,
})
const activity = item({ seq: 9 })

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

type Server = {
  items?: InboxItem[]
  unread?: number
  forbidden?: boolean
  thread?: {
    messages: { id: string; body: string; who: string; answer?: unknown }[]
  }
}

function mockServer({
  items = [needsYou, activity],
  unread = 1,
  forbidden,
  thread,
}: Server = {}) {
  const reads: unknown[] = []
  const states: { path: string; body: unknown }[] = []
  const views: (string | null)[] = []
  const filters: (string | null)[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/inbox/read") {
        reads.push(await request.clone().json())
        return new Response(null, { status: 204 })
      }
      if (url.pathname.startsWith("/api/v1/inbox/items/")) {
        states.push({
          path: url.pathname,
          body: await request.clone().json(),
        })
        return new Response(null, { status: 204 })
      }
      if (url.pathname === "/api/v1/threads" && thread !== undefined) {
        return json({
          threads: [threadRecord],
          messages: thread.messages.map((entry, index) => ({
            id: entry.id,
            thread_id: proposalId,
            body: entry.body,
            actor: { ...ada, id: entry.id, name: entry.who, label: entry.who },
            created_at: `2020-11-10T09:0${index}:00Z`,
            updated_at: `2020-11-10T09:0${index}:00Z`,
            edited_at: null,
            mentions: [],
            can_edit: false,
            can_delete: false,
            block_id: null,
            reply_to: null,
            ...(entry.answer === undefined ? {} : { answer: entry.answer }),
          })),
          next_cursor: null,
          can_comment: true,
        })
      }
      if (url.pathname === "/api/v1/inbox") {
        views.push(url.searchParams.get("view"))
        filters.push(url.searchParams.get("filter"))
        if (forbidden) {
          return json({ error: "no", code: "user_session_required" }, 403)
        }
        return json({ items, next_cursor: null, unread_count: unread })
      }
      return json({ error: "unexpected", code: "unexpected" }, 500)
    })
  )
  return { reads, views, filters, states }
}

const threadRecord = {
  id: proposalId,
  target: { kind: "dashboard", id: dashboardId },
  dashboard_id: dashboardId,
  version: 1,
  panel_id: null,
  panel_present: true,
  status: "open",
  allowed_actions: ["reply", "resolve"],
  created_at: "2020-11-10T09:00:00Z",
  updated_at: "2020-11-10T09:05:00Z",
  resolved_at: null,
  resolved_actor: null,
  resolved_by_version: null,
  revision: null,
}

function stubWide(wide: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: wide,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }))
}

function renderPage(
  props: {
    view?: InboxView
    filter?: InboxFilter
    grouped?: boolean
    selected?: string
    onChange?: (next: InboxState) => void
  } = {}
) {
  return renderWithProviders(
    <InboxPage
      view={props.view ?? "for_you"}
      filter={props.filter ?? "open"}
      grouped={props.grouped ?? false}
      selected={props.selected}
      onChange={props.onChange ?? (() => {})}
    />,
    { route: { path: "/", url: "/" } }
  )
}

beforeEach(() => {
  toasts.success.mockClear()
  toasts.error.mockClear()
  stubWide(false)
  vi.stubGlobal(
    "EventSource",
    class {
      static CLOSED = 2
      readyState = 0
      onerror = null
      addEventListener() {}
      close() {}
    }
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("InboxPage", () => {
  it("lists an item with a reason under Needs you and the rest under Workspace activity", async () => {
    mockServer()
    renderPage()

    const needs = await screen.findByRole("heading", { name: "Needs you" })
    const rest = screen.getByRole("heading", { name: "Workspace activity" })
    const needsList = needs.nextElementSibling as HTMLElement
    const restList = rest.nextElementSibling as HTMLElement
    expect(within(needsList).getByText("Ada mentioned you on")).toBeTruthy()
    expect(within(needsList).getByText("Mention")).toBeTruthy()
    expect(within(needsList).getByText("Unread")).toBeTruthy()
    expect(within(restList).getByText("Ada version created")).toBeTruthy()
    expect(within(restList).queryByText("Unread")).toBeNull()
    expect(
      screen.getByRole("heading", { level: 1, name: "Inbox" })
    ).toBeTruthy()
  })

  it("posts the newest seq when marking all read and says so", async () => {
    const server = mockServer({ unread: 2 })
    const { user } = renderPage()

    await user.click(
      await screen.findByRole("button", { name: "Mark all read" })
    )

    await waitFor(() => expect(server.reads).toEqual([{ seq: 12 }]))
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith("Marked as read")
    )
  })

  it("offers Mark all read only while something is unread", async () => {
    mockServer({ unread: 0 })
    renderPage()

    await screen.findByRole("heading", { name: "Needs you" })
    expect(screen.queryByRole("button", { name: "Mark all read" })).toBeNull()
  })

  it("asks for the chosen view and switches views through the tabs", async () => {
    const server = mockServer()
    const changes: InboxState[] = []
    const { user } = renderPage({
      view: "mentions",
      onChange: (next) => changes.push(next),
    })

    await screen.findByRole("heading", { name: "Needs you" })
    expect(server.views).toEqual(["mentions"])
    expect(
      screen.getByRole("tab", { name: "Mentions", selected: true })
    ).toBeTruthy()

    await user.click(screen.getByRole("tab", { name: "Proposals" }))
    expect(changes).toEqual([
      { view: "proposals", filter: "open", grouped: false, item: undefined },
    ])
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "For you",
      "Everything",
      "Proposals",
      "Mentions",
    ])
  })

  it.each([
    ["for_you", "Nothing needs you right now."],
    ["everything", "No activity in the last 30 days."],
    ["proposals", "No proposals waiting for you."],
    ["mentions", "No mentions or replies."],
  ] as const)("says %s is empty in its own words", async (view, text) => {
    mockServer({ items: [], unread: 0 })
    renderPage({ view })

    expect(await screen.findByText(text)).toBeTruthy()
  })

  it("tells a key session the inbox is for signed-in people", async () => {
    mockServer({ forbidden: true })
    renderPage()

    expect(
      await screen.findByText("The inbox is for signed-in people.")
    ).toBeTruthy()
  })

  it("under 1100 px a row is a link to the item's page and no detail pane shows", async () => {
    mockServer({
      items: [
        item({
          seq: 3,
          type: "proposal.opened",
          target: { kind: "proposal", id: proposalId },
          reason: "proposal",
        }),
      ],
      unread: 0,
    })
    renderPage()

    const link = await screen.findByRole("link", {
      name: /proposed Checkout/,
    })
    expect(link.getAttribute("href")).toBe(`/proposals/${proposalId}`)
    expect(screen.queryByRole("region", { name: "Selected item" })).toBeNull()
  })

  it("from 1100 px a row selects the item and the pane prompts when none is chosen", async () => {
    stubWide(true)
    mockServer()
    const changes: { view: InboxView; item: string | undefined }[] = []
    const { user } = renderPage({ onChange: (next) => changes.push(next) })

    const pane = await screen.findByRole("region", { name: "Selected item" })
    expect(
      within(pane).getByText("Select an item to see it here.")
    ).toBeTruthy()

    await user.click(
      await screen.findByRole("button", { name: /mentioned you on/ })
    )
    expect(changes).toEqual([
      { view: "for_you", filter: "open", grouped: false, item: "12" },
    ])
  })

  it("from 1100 px the pane shows the selected item and a link to its page", async () => {
    stubWide(true)
    mockServer()
    renderPage({ selected: "9" })

    const pane = await screen.findByRole("region", { name: "Selected item" })
    expect(
      await within(pane).findByRole("heading", {
        level: 2,
        name: "Ada version created Checkout",
      })
    ).toBeTruthy()
    expect(
      within(pane).getByRole("link", { name: "Open" }).getAttribute("href")
    ).toBe(`/d/${dashboardId}/checkout`)
    expect(
      screen
        .getByRole("button", { name: /version created/ })
        .getAttribute("aria-current")
    ).toBe("true")
  })

  it("moves the selection with the arrow keys", async () => {
    stubWide(true)
    mockServer()
    const changes: { view: InboxView; item: string | undefined }[] = []
    const { user } = renderPage({
      selected: "12",
      onChange: (next) => changes.push(next),
    })

    const first = await screen.findByRole("button", {
      name: /mentioned you on/,
    })
    first.focus()
    await user.keyboard("{ArrowDown}")

    expect(changes).toEqual([
      { view: "for_you", filter: "open", grouped: false, item: "9" },
    ])
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: /version created/ })
    )
  })

  it("names the target by its title and quotes the comment in the row", async () => {
    mockServer({
      items: [
        item({
          seq: 12,
          type: "thread.mentioned",
          target: { kind: "thread", id: proposalId },
          data: withData({ object: { kind: "dashboard", id: dashboardId } }),
          reason: "mention",
          unread: true,
          target_title: "Checkout latency",
          excerpt: "@Ada can you look at the p99?",
        }),
      ],
    })
    renderPage()

    const row = await screen.findByRole("link", { name: /mentioned you on/ })
    expect(within(row).getByText("Checkout latency")).toBeTruthy()
    expect(within(row).getByText("@Ada can you look at the p99?")).toBeTruthy()
  })

  it("marks one item read or unread from its row without moving the rest", async () => {
    const server = mockServer()
    const { user } = renderPage()

    await user.click(
      await screen.findByRole("button", { name: "Mark as read" })
    )
    await user.click(screen.getByRole("button", { name: "Mark as unread" }))

    await waitFor(() =>
      expect(server.states).toEqual([
        { path: "/api/v1/inbox/items/12", body: { unread: false } },
        { path: "/api/v1/inbox/items/9", body: { unread: true } },
      ])
    )
    expect(server.reads).toEqual([])
  })

  it("marks an item done from its row and offers to undo it", async () => {
    const server = mockServer()
    const { user } = renderPage()

    const buttons = await screen.findAllByRole("button", { name: "Mark done" })
    await user.click(buttons[0])

    await waitFor(() =>
      expect(server.states).toEqual([
        { path: "/api/v1/inbox/items/12", body: { done: true } },
      ])
    )
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith(
        "Marked done",
        expect.objectContaining({
          action: expect.objectContaining({ label: "Undo" }),
        })
      )
    )
  })

  it("asks for the chosen filter and moves a done item back to the inbox", async () => {
    const server = mockServer()
    const { user } = renderPage({ filter: "done" })

    await screen.findByRole("heading", { name: "Needs you" })
    expect(server.filters).toEqual(["done"])
    expect(
      screen.getByRole("button", { name: "Done", pressed: true })
    ).toBeTruthy()

    await user.click(
      screen.getAllByRole("button", { name: "Move back to the inbox" })[0]
    )
    await waitFor(() =>
      expect(server.states).toEqual([
        { path: "/api/v1/inbox/items/12", body: { done: false } },
      ])
    )
  })

  it("reports the filter and grouping the person picks", async () => {
    mockServer()
    const changes: InboxState[] = []
    const { user } = renderPage({ onChange: (next) => changes.push(next) })

    await screen.findByRole("heading", { name: "Needs you" })
    await user.click(screen.getByRole("button", { name: "Unread" }))
    await user.click(
      screen.getByRole("button", { name: "Group by dashboard or observation" })
    )

    expect(changes).toEqual([
      { view: "for_you", filter: "unread", grouped: false, item: undefined },
      { view: "for_you", filter: "open", grouped: true, item: undefined },
    ])
  })

  it("groups the list by dashboard when asked", async () => {
    mockServer()
    renderPage({ grouped: true })

    await screen.findAllByRole("listitem")
    expect(screen.getAllByRole("heading", { level: 2 })).toHaveLength(1)
    expect(screen.queryByRole("heading", { name: "Needs you" })).toBeNull()
  })

  it("says nothing is unread in the unread filter", async () => {
    mockServer({ items: [], unread: 0 })
    renderPage({ filter: "unread" })

    expect(await screen.findByText("Nothing unread.")).toBeTruthy()
  })

  it("marks an item read once when it opens in the pane", async () => {
    stubWide(true)
    const server = mockServer({ thread: { messages: [] } })
    renderPage({ selected: "12" })

    await waitFor(() =>
      expect(server.states).toEqual([
        { path: "/api/v1/inbox/items/12", body: { unread: false } },
      ])
    )
  })

  it("shows the thread and a reply box in the pane for a mention", async () => {
    stubWide(true)
    mockServer({
      thread: {
        messages: [
          { id: "m1", body: "Why did p99 double?", who: "Grace" },
          { id: "m2", body: "It was the deploy.", who: "Ada" },
        ],
      },
    })
    renderPage({ selected: "12" })

    const pane = await screen.findByRole("region", { name: "Selected item" })
    expect(await within(pane).findByText("Why did p99 double?")).toBeTruthy()
    expect(within(pane).getByText("It was the deploy.")).toBeTruthy()
    expect(within(pane).getByRole("textbox", { name: "Reply" })).toBeTruthy()
    expect(within(pane).getByRole("link", { name: "Open" })).toBeTruthy()
  })

  it("shows an agent answer as its card without the stub reply text", async () => {
    stubWide(true)
    const runId = "7c1d2e3f-0b5a-4c7d-9e11-2a6b8c4d5e99"
    mockServer({
      thread: {
        messages: [
          { id: "m1", body: "Why did p99 double?", who: "Grace" },
          {
            id: "m2",
            body: "Answered.",
            who: "Observe Agent",
            answer: {
              can_save: false,
              finding: {
                id: "f1",
                observation_id: null,
                agent_request_id: runId,
                dashboard_id: dashboardId,
                folder_id: null,
                ask_on: null,
                restricted: false,
                title: "p99 doubled",
                markdown: "The deploy **doubled** p99.",
                evidence: [],
                from: "2020-11-10T08:00:00Z",
                to: "2020-11-10T09:00:00Z",
                as_of: null,
                annotation_id: null,
                created_by: ada,
                actor: ada,
                created_at: "2020-11-10T09:01:00Z",
                url: null,
                suggested_change: null,
                observe_worthy: false,
              },
            },
          },
        ],
      },
    })
    renderPage({ selected: "12" })

    const pane = await screen.findByRole("region", { name: "Selected item" })
    expect(await within(pane).findByText("doubled")).toBeTruthy()
    expect(within(pane).queryByText("Answered.")).toBeNull()
  })

  it("keeps the reply text when the answer is not visible to the reader", async () => {
    stubWide(true)
    mockServer({
      thread: {
        messages: [
          { id: "m1", body: "Why did p99 double?", who: "Grace" },
          { id: "m2", body: "Answered.", who: "Observe Agent" },
        ],
      },
    })
    renderPage({ selected: "12" })

    const pane = await screen.findByRole("region", { name: "Selected item" })
    expect(await within(pane).findByText("Answered.")).toBeTruthy()
  })
})
