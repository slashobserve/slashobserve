import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { InboxView } from "@/api/keys"
import { renderWithProviders } from "@/test/render"

import type { InboxItem } from "./inbox-item"
import { InboxPage } from "./inbox-page"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock("sonner", () => ({ toast: toasts }))

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
}

function mockServer({
  items = [needsYou, activity],
  unread = 1,
  forbidden,
}: Server = {}) {
  const reads: unknown[] = []
  const views: (string | null)[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/inbox/read") {
        reads.push(await request.clone().json())
        return new Response(null, { status: 204 })
      }
      if (url.pathname === "/api/v1/inbox") {
        views.push(url.searchParams.get("view"))
        if (forbidden) {
          return json({ error: "no", code: "user_session_required" }, 403)
        }
        return json({ items, next_cursor: null, unread_count: unread })
      }
      return json({ error: "unexpected", code: "unexpected" }, 500)
    })
  )
  return { reads, views }
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
    selected?: string
    onChange?: (next: { view: InboxView; item: string | undefined }) => void
  } = {}
) {
  return renderWithProviders(
    <InboxPage
      view={props.view ?? "for_you"}
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
    const changes: { view: InboxView; item: string | undefined }[] = []
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
    expect(changes).toEqual([{ view: "proposals", item: undefined }])
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
      name: /proposed a change to/,
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
    expect(changes).toEqual([{ view: "for_you", item: "12" }])
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

    expect(changes).toEqual([{ view: "for_you", item: "9" }])
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: /version created/ })
    )
  })
})
