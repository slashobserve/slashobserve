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

function mockServer() {
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
          return json(envelope)
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
  it("comments on the hovered panel with c", async () => {
    const { user, router } = renderDashboard()
    const note = await screen.findByRole("article", { name: "Note" })
    await user.hover(note)
    await user.keyboard("c")
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({
        tab: "activity",
        panel: "p_note",
      })
    )
    expect(useActivityStore.getState().askingAgent).toBe(false)
  })

  it("asks about the hovered data panel with a", async () => {
    useActivityStore.setState({ askingAgent: false })
    const asked: boolean[] = []
    const stop = useActivityStore.subscribe((state) =>
      asked.push(state.askingAgent)
    )
    const { user, router } = renderDashboard()
    const latency = await screen.findByRole("article", { name: "Latency" })
    await user.hover(latency)
    await waitFor(async () => {
      await user.keyboard("a")
      expect(router.state.location.search).toMatchObject({
        panel: "p_latency",
      })
    })
    stop()
    expect(asked).toContain(true)
  })

  it("does not ask about a panel without data", async () => {
    useActivityStore.setState({ askingAgent: false })
    const { user, router } = renderDashboard()
    const note = await screen.findByRole("article", { name: "Note" })
    await waitFor(() => expect(agentRequested()).toBe(true))
    await act(async () => {})
    await user.hover(note)
    await user.keyboard("a")
    expect(router.state.location.search).not.toHaveProperty("panel")
  })

  it("acts on the focused panel when none is hovered", async () => {
    const { user, router } = renderDashboard()
    await screen.findByRole("article", { name: "Latency" })
    const title = await screen.findByRole("button", {
      name: "Select panel: Latency",
    })
    title.focus()
    await user.keyboard("c")
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({
        panel: "p_latency",
      })
    )
  })
})
