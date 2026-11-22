import { QueryClientProvider } from "@tanstack/react-query"
import {
  RouterProvider,
  createMemoryHistory,
  createRouter,
} from "@tanstack/react-router"
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useActivityStore } from "@/activity/activity-store"
import { parseSearch, stringifySearch } from "@/lib/search"
import { routeTree } from "@/routeTree.gen"
import { testQueryClient } from "@/test/render"

import { useCanvasTool } from "./canvas-tools"

const dashboardId = "3c1f9a7e-2b4d-4e6f-8a1c-5d7e9f0a2b3c"
const folderId = "8e2d4f6a-1c3b-4d5e-9f7a-0b2c4d6e8f1a"
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
          id: "p_runbook",
          type: "text",
          title: "Runbook",
          gridPos: { x: 12, y: 0, w: 12, h: 4 },
          content: "Restart the queue",
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
        case "/api/v1/datasources":
          return json({ datasources: [] })
        case "/api/v1/folders":
          return json({ folders: [] })
      }
      return json({ error: "not found", code: "not_found" }, 404)
    })
  )
}

function renderDashboard() {
  const queryClient = testQueryClient()
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({
      initialEntries: [`/d/${dashboardId}/checkout`],
    }),
    parseSearch,
    stringifySearch,
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

function gridItemOf(title: string): HTMLElement {
  const item = screen
    .getByRole("button", { name: `Select panel: ${title}` })
    .closest<HTMLElement>(".react-grid-item")
  if (!item) throw new Error(`no grid item for ${title}`)
  return item
}

function articleOf(title: string): HTMLElement {
  const article = gridItemOf(title).querySelector<HTMLElement>("article")
  if (!article) throw new Error(`no article for ${title}`)
  return article
}

function layOutAtDesktopWidth() {
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1280)
  Object.defineProperty(HTMLElement.prototype, "offsetParent", {
    configurable: true,
    get(this: HTMLElement) {
      return this.parentElement
    },
  })
  const computed = window.getComputedStyle.bind(window)
  vi.spyOn(window, "getComputedStyle").mockImplementation((element, pseudo) => {
    const style = computed(element, pseudo)
    return new Proxy(style, {
      get(target, key) {
        if (key === "paddingLeft" || key === "paddingRight") {
          return target[key] || "0px"
        }
        const value: unknown = Reflect.get(target, key)
        return typeof value === "function" ? value.bind(target) : value
      },
    })
  })
}

beforeEach(() => {
  mockServer()
  vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => 0)
  layOutAtDesktopWidth()
  useActivityStore.setState({ open: false })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  Reflect.deleteProperty(HTMLElement.prototype, "offsetParent")
  useCanvasTool.setState({ tool: "select" })
  useActivityStore.setState({ open: true })
})

describe("arranging panels on the dashboard grid", () => {
  it("starts dragging an unselected panel from its first press on the title", async () => {
    renderDashboard()
    const title = await screen.findByRole("button", {
      name: "Select panel: Runbook",
    })
    expect(articleOf("Runbook")).not.toHaveAttribute("data-selected")

    fireEvent.pointerDown(title, { button: 0 })
    await waitFor(() =>
      expect(articleOf("Runbook")).toHaveAttribute("data-selected")
    )
    fireEvent.mouseDown(title, { button: 0, clientX: 700, clientY: 20 })
    fireEvent.mouseMove(document, { clientX: 760, clientY: 60 })

    await waitFor(() =>
      expect(gridItemOf("Runbook")).toHaveClass("react-draggable-dragging")
    )
    expect(gridItemOf("Note")).not.toHaveClass("react-draggable-dragging")
    fireEvent.mouseUp(document, { clientX: 760, clientY: 60 })
  })

  it.each(["comment", "ask"] as const)(
    "offers no move or resize on the selected panel while a press is in %s mode",
    async (tool) => {
      renderDashboard()
      const title = await screen.findByRole("button", {
        name: "Select panel: Runbook",
      })
      fireEvent.pointerDown(title, { button: 0 })
      await waitFor(() =>
        expect(
          articleOf("Runbook").querySelector("button.panel-drag-handle")
        ).not.toBeNull()
      )
      expect(gridItemOf("Runbook")).not.toHaveClass("react-resizable-hide")

      act(() => useCanvasTool.getState().setTool(tool))

      expect(gridItemOf("Runbook")).toHaveClass("react-resizable-hide")
      expect(
        articleOf("Runbook").querySelector("button.panel-drag-handle")
      ).toBeNull()
      expect(articleOf("Runbook").querySelector("header")).not.toHaveClass(
        "touch-none"
      )
    }
  )
})
