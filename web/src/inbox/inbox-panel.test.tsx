import { act, screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { InboxBell } from "@/shell/inbox-bell"
import { PanelHostContext, SidePanel } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import type { InboxItem } from "./inbox-item"
import { InboxPanel } from "./inbox-panel"
import { useInboxPanelStore } from "./inbox-panel-store"

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

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function mockServer({
  items = [mention, activity],
  unread = 1,
}: { items?: InboxItem[]; unread?: number } = {}) {
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
        states.push({ path: url.pathname, body: await request.clone().json() })
        return new Response(null, { status: 204 })
      }
      if (url.pathname === "/api/v1/inbox") {
        views.push(url.searchParams.get("view"))
        filters.push(url.searchParams.get("filter"))
        return json({ items, next_cursor: null, unread_count: unread })
      }
      return json({ error: "unexpected", code: "unexpected" }, 500)
    })
  )
  return { reads, states, views, filters }
}

function Harness({ pagePanel = false }: { pagePanel?: boolean }) {
  return (
    <>
      <InboxBell place="top" className="" badgeClassName="" />
      <PanelHostContext.Provider value={document.getElementById("host")}>
        {pagePanel ? (
          <SidePanel title="History" onClose={() => {}}>
            <p>Version 3</p>
          </SidePanel>
        ) : null}
        <InboxPanel />
      </PanelHostContext.Provider>
    </>
  )
}

function renderPanel(pagePanel = false) {
  const host = document.createElement("div")
  host.id = "host"
  document.body.append(host)
  return renderWithProviders(<Harness pagePanel={pagePanel} />, {
    route: { path: "/", url: "/" },
  })
}

function openPanel() {
  act(() => useInboxPanelStore.getState().show())
}

beforeEach(() => {
  useInboxPanelStore.setState({ open: false, view: "for_you", filter: "open" })
  toasts.success.mockReset()
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
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
})
