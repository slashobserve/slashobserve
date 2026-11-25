import type { ReactNode } from "react"
import { act, screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { inboxKeys } from "@/api/keys"
import { InboxBell } from "@/shell/inbox-bell"
import { PanelHostContext, SidePanel } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import type { InboxItem } from "./inbox-item"
import { InboxPanel } from "./inbox-panel"
import { useInboxPanelStore } from "./inbox-panel-store"
import { ActivityStreamProvider } from "@/live/activity-stream"
import { FakeEventSource } from "@/test/fake-event-source"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock("sonner", () => ({ toast: toasts }))

const dashboardId = "a1b2c3d4-0b5a-4c7d-9e11-2a6b8c4d5e71"
const threadId = "3f2c8d1e-0b5a-4c7d-9e11-2a6b8c4d5e70"

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

const mention = item({
  seq: 12,
  type: "thread.mentioned",
  target: { kind: "thread", id: threadId },
  data: withData({ object: { kind: "dashboard", id: dashboardId } }),
  reason: "mention",
  unread: true,
})
const activity = item({ seq: 9 })
const longNote =
  "The deploy at 14:02 moved the checkout pool to the new region. ".repeat(6)
const note = item({
  seq: 7,
  type: "annotation.created",
  target: { kind: "annotation", id: "8d1e3f2c-0b5a-4c7d-9e11-2a6b8c4d5e72" },
  dashboard_id: null,
  data: withData({ text: longNote }),
  excerpt: longNote,
  unread: true,
})

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function mockServer({
  items = [mention, activity],
  unread = 1,
  nextPage,
}: {
  items?: InboxItem[]
  unread?: number
  nextPage?: () => Response
} = {}) {
  const reads: unknown[] = []
  const states: { path: string; body: unknown }[] = []
  const views: (string | null)[] = []
  const filters: (string | null)[] = []
  const cursors: string[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/inbox/read") {
        reads.push(await request.clone().json())
        return new Response(null, { status: 204 })
      }
      if (url.pathname.startsWith("/api/v1/inbox/items/")) {
        states.push({ path: url.pathname, body: await request.clone().json() })
        return new Response(null, { status: 204 })
      }
      const cursor = url.searchParams.get("cursor")
      if (url.pathname === "/api/v1/inbox" && cursor !== null && nextPage) {
        cursors.push(cursor)
        return nextPage()
      }
      if (url.pathname === "/api/v1/inbox") {
        views.push(url.searchParams.get("view"))
        filters.push(url.searchParams.get("filter"))
        return json({
          items,
          next_cursor: nextPage ? "page-2" : null,
          unread_count: unread,
        })
      }
      return json({ error: "unexpected", code: "unexpected" }, 500)
    })
  )
  return { reads, states, views, filters, cursors }
}

function Stream({
  streamed,
  children,
}: {
  streamed: boolean
  children: ReactNode
}) {
  return streamed ? (
    <ActivityStreamProvider>{children}</ActivityStreamProvider>
  ) : (
    children
  )
}

function Harness({
  pagePanel = false,
  streamed = false,
}: {
  pagePanel?: boolean
  streamed?: boolean
}) {
  return (
    <Stream streamed={streamed}>
      <button
        type="button"
        onClick={() => useInboxPanelStore.getState().show()}
      >
        Open from Home
      </button>
      <InboxBell className="" badgeClassName="" />
      <PanelHostContext.Provider value={document.getElementById("host")}>
        {pagePanel ? (
          <SidePanel title="History" onClose={() => {}}>
            <p>Version 3</p>
          </SidePanel>
        ) : null}
        <InboxPanel />
      </PanelHostContext.Provider>
    </Stream>
  )
}

function renderPanel(pagePanel = false, streamed = false) {
  const host = document.createElement("div")
  host.id = "host"
  document.body.append(host)
  return renderWithProviders(
    <Harness pagePanel={pagePanel} streamed={streamed} />,
    { route: { path: "/", url: "/" } }
  )
}

function findRow(seq: number): Promise<HTMLElement> {
  return waitFor(() => {
    const row = document.querySelector<HTMLElement>(`[data-inbox-row="${seq}"]`)
    if (row === null) throw new Error(`no row ${seq}`)
    return row
  })
}

function openPanel() {
  act(() => useInboxPanelStore.getState().show())
}

const intersecting = { now: false }

beforeEach(() => {
  intersecting.now = false
  useInboxPanelStore.setState({ open: false, view: "for_you", filter: "open" })
  toasts.success.mockReset()
  vi.stubGlobal("EventSource", FakeEventSource)
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      readonly callback: IntersectionObserverCallback
      constructor(callback: IntersectionObserverCallback) {
        this.callback = callback
      }
      observe() {
        if (intersecting.now)
          this.callback(
            [{ isIntersecting: true } as IntersectionObserverEntry],
            this as unknown as IntersectionObserver
          )
      }
      disconnect() {}
    }
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.getElementById("host")?.remove()
})

describe("InboxPanel", () => {
  it("opens from the bell, lists items by group and closes back to the bell on Escape", async () => {
    mockServer()
    const { user } = renderPanel()

    const bell = await screen.findByRole("button", { name: "Inbox, 1 unread" })
    expect(bell).toHaveAttribute("aria-expanded", "false")
    expect(screen.queryByRole("complementary")).toBeNull()

    await user.click(bell)
    const panel = await screen.findByRole("complementary", { name: "Inbox" })
    expect(bell).toHaveAttribute("aria-expanded", "true")
    expect(within(panel).getByRole("heading", { name: "Inbox" })).toHaveFocus()
    const needs = await within(panel).findByRole("heading", {
      name: "Needs you",
    })
    const rest = within(panel).getByRole("heading", {
      name: "Workspace activity",
    })
    expect(
      within(needs.nextElementSibling as HTMLElement).getByText("Mention")
    ).toBeTruthy()
    expect(
      within(rest.nextElementSibling as HTMLElement).getByText(
        "Ada version created"
      )
    ).toBeTruthy()

    await user.keyboard("{Escape}")
    expect(screen.queryByRole("complementary")).toBeNull()
    expect(bell).toHaveFocus()
  })

  it("asks for the view picked in the view menu", async () => {
    const server = mockServer()
    const { user } = renderPanel()
    openPanel()
    await screen.findByRole("heading", { name: "Needs you" })

    await user.click(screen.getByRole("combobox", { name: "Inbox view" }))
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "For you",
      "Proposals",
      "Mentions",
      "Everything",
    ])
    await user.click(screen.getByRole("option", { name: "Mentions" }))

    await waitFor(() => expect(server.views).toEqual(["for_you", "mentions"]))
    expect(useInboxPanelStore.getState().view).toBe("mentions")
  })

  it("opens on the view and filter it was shown with", async () => {
    const server = mockServer({ items: [], unread: 0 })
    renderPanel()
    act(() =>
      useInboxPanelStore.getState().show({ view: "mentions", filter: "done" })
    )

    expect(await screen.findByText("Nothing marked done.")).toBeTruthy()
    expect(server.views).toEqual(["for_you", "mentions"])
    expect(server.filters).toEqual(["open", "done"])
  })

  it("marks an item read and closes when the person opens it", async () => {
    const server = mockServer()
    const { user } = renderPanel()
    openPanel()
    const needs = await screen.findByRole("heading", { name: "Needs you" })
    const row = within(needs.nextElementSibling as HTMLElement).getByRole(
      "link"
    )

    await user.click(row)

    expect(useInboxPanelStore.getState().open).toBe(false)
    await waitFor(() =>
      expect(server.states).toEqual([
        { path: "/api/v1/inbox/items/12", body: { unread: false } },
      ])
    )
  })

  it("does not mark anything read when it opens", async () => {
    const server = mockServer()
    renderPanel()
    openPanel()
    await screen.findByRole("heading", { name: "Needs you" })

    expect(server.reads).toEqual([])
    expect(server.states).toEqual([])
  })

  it("posts the newest seq when marking all read", async () => {
    const server = mockServer({ unread: 2 })
    const { user } = renderPanel()
    openPanel()

    await user.click(
      await screen.findByRole("button", { name: "Mark all read" })
    )

    await waitFor(() => expect(server.reads).toEqual([{ seq: 12 }]))
  })

  it("marks an item done from its row and offers to undo it", async () => {
    const server = mockServer()
    const { user } = renderPanel()
    openPanel()
    await screen.findByRole("heading", { name: "Needs you" })

    await user.click(screen.getAllByRole("button", { name: "Mark done" })[0])

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

  it("hides the page's side panel while open and shows it again on close", async () => {
    mockServer()
    const { user } = renderPanel(true)
    const history = await screen.findByRole("complementary", {
      name: "History",
    })
    expect(history).toBeVisible()

    openPanel()
    await screen.findByRole("complementary", { name: "Inbox" })
    expect(history).not.toBeVisible()
    expect(history).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Close Inbox" }))
    expect(screen.queryByRole("complementary", { name: "Inbox" })).toBeNull()
    expect(history).toBeVisible()
    expect(within(history).getByText("Version 3")).toBeTruthy()
  })

  it("stops loading more after a failed page and retries from a button below the list", async () => {
    intersecting.now = true
    let failures = 1
    const server = mockServer({
      nextPage: () =>
        failures-- > 0
          ? json({ error: "The inbox is busy.", code: "unavailable" }, 503)
          : json({ items: [], next_cursor: null, unread_count: 1 }),
    })
    const { user } = renderPanel()
    openPanel()

    const problem = await screen.findByText("Could not load more of the inbox")
    expect(server.cursors).toEqual(["page-2"])
    expect(screen.getByRole("heading", { name: "Needs you" })).toBeTruthy()

    await user.click(
      within(problem.parentElement as HTMLElement).getByRole("button", {
        name: "Retry",
      })
    )
    await waitFor(() =>
      expect(screen.queryByText("Could not load more of the inbox")).toBeNull()
    )
    expect(server.cursors).toEqual(["page-2", "page-2"])
  })

  it("expands an item with no page to open in place, marking it read", async () => {
    const server = mockServer({ items: [mention, note] })
    const { user } = renderPanel()
    openPanel()
    const row = await findRow(7)
    expect(row).toHaveAttribute("aria-expanded", "false")
    expect(within(row).getByText(longNote.trim())).toHaveClass("line-clamp-2")

    await user.click(row)

    expect(row).toHaveAttribute("aria-expanded", "true")
    expect(within(row).getByText(longNote.trim())).not.toHaveClass(
      "line-clamp-2"
    )
    expect(useInboxPanelStore.getState().open).toBe(true)
    await waitFor(() =>
      expect(server.states).toEqual([
        { path: "/api/v1/inbox/items/7", body: { unread: false } },
      ])
    )

    await user.click(row)
    expect(row).toHaveAttribute("aria-expanded", "false")
  })

  it("focuses, highlights and expands the item it was opened on", async () => {
    mockServer({ items: [mention, note] })
    renderPanel()
    act(() => useInboxPanelStore.getState().show({ item: 7 }))

    const row = await findRow(7)
    expect(row).toHaveAttribute("aria-expanded", "true")
    expect(row).toHaveAttribute("aria-current", "true")
    await waitFor(() => expect(row).toHaveFocus())
    expect(row).toHaveAttribute("tabindex", "0")
  })

  it("focuses and highlights a linked item it was opened on without expanding it", async () => {
    mockServer()
    renderPanel()
    act(() => useInboxPanelStore.getState().show({ item: 12 }))

    const needs = await screen.findByRole("heading", { name: "Needs you" })
    const row = within(needs.nextElementSibling as HTMLElement).getByRole(
      "link"
    )
    expect(row).toHaveAttribute("aria-current", "true")
    expect(row).not.toHaveAttribute("aria-expanded")
    await waitFor(() => expect(row).toHaveFocus())
  })

  it.each(["proposals", "mentions"] as const)(
    "offers no Mark all read in %s",
    async (view) => {
      mockServer({ unread: 2 })
      renderPanel()
      act(() => useInboxPanelStore.getState().show({ view }))
      await screen.findByRole("heading", { name: "Needs you" })

      expect(screen.queryByRole("button", { name: "Mark all read" })).toBeNull()
    }
  )

  it("offers Mark all read in Everything", async () => {
    mockServer({ unread: 2 })
    renderPanel()
    act(() => useInboxPanelStore.getState().show({ view: "everything" }))

    expect(
      await screen.findByRole("button", { name: "Mark all read" })
    ).toBeTruthy()
  })

  it.each([
    { streamed: false, interval: 15_000 },
    { streamed: true, interval: false },
  ])(
    "polls its own list only outside the shell (streamed $streamed)",
    async ({ streamed, interval }) => {
      mockServer()
      const { queryClient } = renderPanel(false, streamed)
      act(() => useInboxPanelStore.getState().show({ view: "mentions" }))
      await screen.findByRole("heading", { name: "Needs you" })

      const observer = queryClient
        .getQueryCache()
        .find({ queryKey: inboxKeys.list("mentions", "open") })?.observers[0]
      expect(observer?.options.refetchInterval).toBe(interval)
    }
  )

  it("returns focus on close to what opened it, or to the bell when that is gone", async () => {
    mockServer()
    const { user } = renderPanel()
    const opener = await screen.findByRole("button", { name: "Open from Home" })

    await user.click(opener)
    await user.click(await screen.findByRole("button", { name: "Close Inbox" }))
    expect(opener).toHaveFocus()

    const gone = document.createElement("button")
    document.body.append(gone)
    gone.focus()
    act(() => useInboxPanelStore.getState().show())
    await screen.findByRole("complementary", { name: "Inbox" })
    gone.remove()
    await user.keyboard("{Escape}")
    expect(
      screen.getByRole("button", { name: "Inbox, 1 unread" })
    ).toHaveFocus()
  })
})
