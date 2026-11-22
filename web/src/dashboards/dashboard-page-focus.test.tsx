import { QueryClientProvider } from "@tanstack/react-query"
import {
  RouterProvider,
  createMemoryHistory,
  createRouter,
} from "@tanstack/react-router"
import { act, cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useActivityStore } from "@/activity/activity-store"
import { useCanvasTool } from "@/dashboards/canvas-tools"
import { useLayoutStore } from "@/dashboards/edit/layout-store"
import { parseSearch, stringifySearch } from "@/lib/search"
import { routeTree } from "@/routeTree.gen"
import { testQueryClient } from "@/test/render"

const dashboardId = "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01"
const folderId = "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77"
const admin = {
  kind: "user",
  id: "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f",
  email: "ana@example.com",
  name: "Ana",
  role: "admin",
} as const
const actor = {
  kind: "user",
  id: admin.id,
  name: admin.name,
  origin: "web",
  client: null,
  run_id: null,
  label: admin.name,
}

const envelope = {
  dashboard: {
    id: dashboardId,
    title: "Checkout",
    tags: [],
    latest_version: 1,
    created_at: "2026-11-19T09:00:00Z",
    updated_at: "2026-11-19T09:00:00Z",
    updated_by: { kind: "user", id: admin.id, name: admin.name },
    creator: {
      author: { kind: "user", id: admin.id, name: admin.name },
      origin: "web",
      origin_client: null,
      via: "direct",
      actor,
    },
    folder: { id: folderId, title: "Shop", general: false, personal: false },
    permissions: { comment: true, edit: true, manage: true, create: true },
  },
  version: {
    version: 1,
    parent_version: null,
    restored_from: null,
    summary: "First version",
    author: { kind: "user", id: admin.id, name: admin.name },
    created_at: "2026-11-19T09:00:00Z",
    origin: "web",
    origin_client: null,
    agent_request_id: null,
    actor,
    merged_proposal_id: null,
    spec: {
      specVersion: "0",
      title: "Checkout",
      time: { from: "now-1h", to: "now" },
      panels: [
        {
          id: "p_note",
          type: "text",
          title: "Note",
          gridPos: { x: 0, y: 0, w: 12, h: 4 },
          content: "Checkout notes",
        },
        {
          id: "p_latency",
          type: "timeseries",
          title: "Latency",
          gridPos: { x: 0, y: 4, w: 12, h: 4 },
          datasource: "prom",
          queries: [{ query: "up" }],
        },
      ],
    },
    unverified: { panels: [], variables: [] },
  },
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

class IdleEventSource {
  static readonly CLOSED = 2
  readyState = 1
  onerror: (() => void) | null = null
  onopen: (() => void) | null = null
  addEventListener() {}
  removeEventListener() {}
  close() {}
}

function mockServer(served: unknown = envelope) {
  vi.stubGlobal("EventSource", IdleEventSource)
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string) => {
      const url = new URL(
        typeof input === "string" ? input : input.url,
        "http://localhost"
      )
      switch (url.pathname) {
        case "/api/v1/auth/session":
          return json({ authenticated: true, principal: admin })
        case `/api/v1/dashboards/${dashboardId}`:
          return json(served)
        case "/api/v1/agent":
          return json({ available: true })
        case "/api/v1/datasources":
          return json({ datasources: [] })
        case "/api/v1/folders":
          return json({ folders: [] })
      }
      return json({ error: "not found", code: "not_found" }, 404)
    })
  )
}

function renderDashboard(search = "") {
  const queryClient = testQueryClient()
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({
      initialEntries: [`/d/${dashboardId}/checkout${search}`],
    }),
    parseSearch,
    stringifySearch,
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { user: userEvent.setup(), router }
}

beforeEach(() => {
  mockServer()
  vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => 0)
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  useActivityStore.setState({ open: true })
})

describe("dashboard page panes", () => {
  it("returns focus to Share when Escape closes the Share pane, even when no animation frame runs", async () => {
    useActivityStore.setState({ open: false })
    const { user, router } = renderDashboard()
    const share = await screen.findByRole("button", { name: "Share" })

    await user.click(share)
    const heading = await screen.findByRole("heading", { name: "Share" })
    await waitFor(() => expect(heading).toHaveFocus())
    await user.tab()
    expect(screen.getByRole("button", { name: "Close panel" })).toHaveFocus()
    await act(() => user.keyboard("{Escape}"))

    await waitFor(() =>
      expect(router.state.location.search).not.toHaveProperty("share")
    )
    expect(
      screen.queryByRole("heading", { name: "Share" })
    ).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Share" })).toHaveFocus()
  })

  it("puts focus on the Activity button when Escape closes an Activity pane that was open on load", async () => {
    const { user } = renderDashboard()
    const heading = await screen.findByRole("heading", { name: "Activity" })
    const close = screen.getByRole("button", { name: "Close panel" })
    close.focus()
    expect(document.activeElement).not.toBe(heading)

    await act(() => user.keyboard("{Escape}"))

    await waitFor(() =>
      expect(
        screen.queryByRole("heading", { name: "Activity" })
      ).not.toBeInTheDocument()
    )
    expect(
      screen.getByRole("button", { name: /^Activity/, pressed: false })
    ).toHaveFocus()
  })

  it("returns focus to the toggle when the pane's Close button is pressed from the keyboard", async () => {
    useActivityStore.setState({ open: false })
    const { user } = renderDashboard()
    const toggle = await screen.findByRole("button", { name: /^Activity/ })

    await user.click(toggle)
    await screen.findByRole("heading", { name: "Activity" })
    screen.getByRole("button", { name: "Close panel" }).focus()
    await act(() => user.keyboard("{Enter}"))

    await waitFor(() =>
      expect(
        screen.queryByRole("heading", { name: "Activity" })
      ).not.toBeInTheDocument()
    )
    expect(screen.getByRole("button", { name: /^Activity/ })).toHaveFocus()
  })

  it("returns focus to the More button when Escape closes the Spec JSON pane, even when no animation frame runs", async () => {
    vi.mocked(window.requestAnimationFrame).mockRestore()
    useActivityStore.setState({ open: false })
    const { user } = renderDashboard()
    await user.click(await screen.findByRole("button", { name: /^More:/ }))
    await user.click(
      await screen.findByRole("menuitem", { name: /^Edit Spec JSON/ })
    )
    const close = await screen.findByRole("button", {
      name: "Close dashboard JSON",
    })
    vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => 0)
    close.focus()
    await act(() => user.keyboard("{Escape}"))

    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Close dashboard JSON" })
      ).not.toBeInTheDocument()
    )
    expect(screen.getByRole("button", { name: /^More:/ })).toHaveFocus()
  })
})

function agentRequested() {
  return vi
    .mocked(fetch)
    .mock.calls.some(([input]) =>
      (typeof input === "string" ? input : (input as Request).url).endsWith(
        "/api/v1/agent"
      )
    )
}

describe("dashboard page panel keys", () => {
  it("opens the whole-panel composer on the hovered panel with c", async () => {
    const { user } = renderDashboard()
    const note = await screen.findByRole("article", { name: "Note" })
    await user.hover(note)
    await user.keyboard("c")
    const composer = await screen.findByRole("dialog", {
      name: "Comment on the canvas",
    })
    expect(composer).toHaveAttribute("data-opened-by", "key")
    expect(composer).toHaveTextContent("Note, whole panel")
  })

  it("comments on a repeated panel's copy in the activity pane, since copies take no canvas comments", async () => {
    const spec = envelope.version.spec
    mockServer({
      ...envelope,
      version: {
        ...envelope.version,
        spec: {
          ...spec,
          variables: [
            {
              name: "env",
              type: "custom",
              options: ["a", "b"],
              multi: true,
              default: ["a", "b"],
            },
          ],
          panels: [spec.panels[0], { ...spec.panels[1], repeat: "env" }],
        },
      },
    })
    const { user, router } = renderDashboard()
    await waitFor(() =>
      expect(screen.getAllByRole("article", { name: "Latency" })).toHaveLength(
        2
      )
    )
    const copy = screen.getAllByRole("article", { name: "Latency" })[1]
    await user.hover(copy)
    await user.keyboard("c")
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({
        tab: "activity",
        panel: "p_latency",
      })
    )
    expect(
      screen.queryByRole("dialog", { name: "Comment on the canvas" })
    ).not.toBeInTheDocument()
  })

  it("closes the canvas composer while the layout has unsaved moves and brings it back with its text", async () => {
    const { user } = renderDashboard()
    const note = await screen.findByRole("article", { name: "Note" })
    await user.hover(note)
    await user.keyboard("c")
    const field = await screen.findByRole("textbox", {
      name: "Write a comment",
    })
    await user.type(field, "half a thought")
    act(() =>
      useLayoutStore
        .getState()
        .ensure(dashboardId, 1, envelope.version.spec as never)
    )
    const session = useLayoutStore.getState().session!
    act(() =>
      useLayoutStore
        .getState()
        .setLayout(
          session.layout.map((item, index) =>
            index === 0 ? { ...item, x: item.x + 1 } : item
          )
        )
    )
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Comment on the canvas" })
      ).not.toBeInTheDocument()
    )
    act(() => useLayoutStore.getState().setLayout(session.baseline))
    expect(
      await screen.findByRole("textbox", { name: "Write a comment" })
    ).toHaveValue("half a thought")
    useLayoutStore.getState().end()
  })

  it("enters Comment mode with c over no panel and leaves it with c again", async () => {
    const { user } = renderDashboard()
    await screen.findByRole("article", { name: "Note" })
    const tools = await screen.findByRole("toolbar", { name: "Canvas tools" })
    await user.keyboard("c")
    await waitFor(() => expect(tools).toHaveAttribute("data-tool", "comment"))
    await user.keyboard("c")
    await waitFor(() => expect(tools).toHaveAttribute("data-tool", "select"))
  })

  it("opens the ask box on the whole hovered data panel with a", async () => {
    const { user } = renderDashboard()
    const latency = await screen.findByRole("article", { name: "Latency" })
    await waitFor(() => expect(agentRequested()).toBe(true))
    await user.hover(latency)
    await waitFor(async () => {
      await user.keyboard("a")
      expect(
        screen.getByRole("dialog", { name: "Ask about Latency, whole panel" })
      ).toHaveAttribute("data-opened-by", "key")
    })
  })

  it("asks about a repeated panel's copy in the Chat tab, since copies take no canvas questions", async () => {
    const spec = envelope.version.spec
    mockServer({
      ...envelope,
      version: {
        ...envelope.version,
        spec: {
          ...spec,
          variables: [
            {
              name: "env",
              type: "custom",
              options: ["a", "b"],
              multi: true,
              default: ["a", "b"],
            },
          ],
          panels: [spec.panels[0], { ...spec.panels[1], repeat: "env" }],
        },
      },
    })
    const { user, router } = renderDashboard()
    await waitFor(() =>
      expect(screen.getAllByRole("article", { name: "Latency" })).toHaveLength(
        2
      )
    )
    await waitFor(() => expect(agentRequested()).toBe(true))
    const copy = screen.getAllByRole("article", { name: "Latency" })[1]
    await user.hover(copy)
    await waitFor(async () => {
      await user.keyboard("a")
      expect(router.state.location.search).toMatchObject({ tab: "chat" })
    })
    expect(screen.queryByRole("dialog")).toBeNull()
  })

  it("hides the ask box while the layout has unsaved moves and brings it back with the typed question", async () => {
    const { user } = renderDashboard()
    const latency = await screen.findByRole("article", { name: "Latency" })
    await waitFor(() => expect(agentRequested()).toBe(true))
    await user.hover(latency)
    await waitFor(async () => {
      await user.keyboard("a")
      expect(
        screen.getByRole("dialog", { name: "Ask about Latency, whole panel" })
      ).toBeInTheDocument()
    })
    const field = await screen.findByRole("textbox", { name: "Ask about this" })
    await user.type(field, "why the spike")
    act(() =>
      useLayoutStore
        .getState()
        .ensure(dashboardId, 1, envelope.version.spec as never)
    )
    const session = useLayoutStore.getState().session!
    act(() =>
      useLayoutStore
        .getState()
        .setLayout(
          session.layout.map((item, index) =>
            index === 0 ? { ...item, x: item.x + 1 } : item
          )
        )
    )
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Ask about Latency, whole panel" })
      ).not.toBeInTheDocument()
    )
    act(() => useLayoutStore.getState().setLayout(session.baseline))
    expect(
      await screen.findByRole("textbox", { name: "Ask about this" })
    ).toHaveValue("why the spike")
    useLayoutStore.getState().end()
  })

  it("points Ask mode only at data panels, as the a key and the Ask buttons do", async () => {
    renderDashboard()
    const note = await screen.findByRole("article", { name: "Note" })
    const latency = await screen.findByRole("article", { name: "Latency" })
    await waitFor(() => expect(agentRequested()).toBe(true))
    const pointed = (panel: HTMLElement, cursor: string) =>
      panel.closest(`.${cursor}`) !== null
    act(() => useCanvasTool.getState().setTool("ask"))
    await waitFor(() =>
      expect(pointed(latency, "cursor-canvas-ask")).toBe(true)
    )
    expect(pointed(note, "cursor-canvas-ask")).toBe(false)
    act(() => useCanvasTool.getState().setTool("comment"))
    await waitFor(() =>
      expect(pointed(note, "cursor-canvas-comment")).toBe(true)
    )
    expect(pointed(latency, "cursor-canvas-ask")).toBe(false)
    act(() => useCanvasTool.getState().setTool("select"))
  })

  it.each([
    ["reads", { comment: true, edit: true, manage: true, create: true }, true],
    [
      "does not read",
      { comment: false, edit: false, manage: false, create: false },
      false,
    ],
  ])(
    "%s the anchored questions for pins by whether the viewer can ask",
    async (_, permissions, fetched) => {
      mockServer({
        ...envelope,
        dashboard: { ...envelope.dashboard, permissions },
      })
      renderDashboard()
      await screen.findByRole("article", { name: "Latency" })
      await waitFor(() => expect(agentRequested()).toBe(true))
      const anchored = () =>
        vi
          .mocked(fetch)
          .mock.calls.some(([input]) =>
            (typeof input === "string"
              ? input
              : (input as Request).url
            ).includes("anchored=true")
          )
      if (fetched) await waitFor(() => expect(anchored()).toBe(true))
      else {
        await act(async () => {})
        expect(anchored()).toBe(false)
      }
    }
  )

  it("enters Ask mode with a over no panel and leaves it with a again", async () => {
    const { user } = renderDashboard()
    await screen.findByRole("article", { name: "Note" })
    const tools = await screen.findByRole("toolbar", { name: "Canvas tools" })
    await waitFor(async () => {
      await user.keyboard("a")
      expect(tools).toHaveAttribute("data-tool", "ask")
    })
    await user.keyboard("a")
    await waitFor(() => expect(tools).toHaveAttribute("data-tool", "select"))
  })

  it("does not ask about a panel without data", async () => {
    const { user, router } = renderDashboard()
    const note = await screen.findByRole("article", { name: "Note" })
    await waitFor(() => expect(agentRequested()).toBe(true))
    await act(async () => {})
    await user.hover(note)
    await user.keyboard("a")
    expect(router.state.location.search).not.toHaveProperty("panel")
    expect(screen.queryByRole("dialog")).toBeNull()
  })

  it("acts on the focused panel when none is hovered", async () => {
    const { user } = renderDashboard()
    await screen.findByRole("article", { name: "Latency" })
    const title = await screen.findByRole("button", {
      name: "Select panel: Latency",
    })
    title.focus()
    await user.keyboard("c")
    expect(
      await screen.findByRole("dialog", { name: "Comment on the canvas" })
    ).toHaveTextContent("Latency, whole panel")
  })
})
