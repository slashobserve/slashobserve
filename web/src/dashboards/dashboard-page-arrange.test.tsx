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
import { activityButtonId, activityCardId } from "./pane-openers"
import { useSelectionStore } from "./selection-store"

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

let canEdit = true
let runbookRemoved = false

function servedEnvelope() {
  const spec = runbookRemoved
    ? {
        ...envelope.version.spec,
        panels: envelope.version.spec.panels.filter(
          (panel) => panel.id !== "p_runbook"
        ),
      }
    : envelope.version.spec
  const version = runbookRemoved ? 2 : 1
  return {
    dashboard: {
      ...envelope.dashboard,
      latest_version: version,
      permissions: { ...envelope.dashboard.permissions, edit: canEdit },
    },
    version: { ...envelope.version, version, spec },
  }
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
          return json(servedEnvelope())
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
  return router
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
  canEdit = true
  runbookRemoved = false
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  Reflect.deleteProperty(HTMLElement.prototype, "offsetParent")
  useCanvasTool.setState({ tool: "select" })
  useActivityStore.setState({ open: true })
  useSelectionStore.getState().clear()
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

  describe("with the Activity pane open beside the grid", () => {
    function activityPane(): HTMLElement {
      const pane = document.getElementById(activityCardId)
      if (!pane) throw new Error("no Activity pane")
      return pane
    }

    beforeEach(() => {
      vi.spyOn(window, "innerWidth", "get").mockReturnValue(1440)
      useActivityStore.setState({ open: true })
    })

    it("closes the pane on the press that selects a panel to arrange, and the press still drags", async () => {
      renderDashboard()
      const title = await screen.findByRole("button", {
        name: "Select panel: Runbook",
      })
      const pane = activityPane()
      expect(pane).toBeVisible()

      fireEvent.pointerDown(title, { button: 0 })
      await waitFor(() => expect(pane).not.toBeVisible())
      fireEvent.mouseDown(title, { button: 0, clientX: 700, clientY: 20 })
      fireEvent.mouseMove(document, { clientX: 760, clientY: 60 })
      await waitFor(() =>
        expect(gridItemOf("Runbook")).toHaveClass("react-draggable-dragging")
      )
      fireEvent.mouseUp(document, { clientX: 760, clientY: 60 })
    })

    it("reopens the same pane from its button while the panel stays selected", async () => {
      renderDashboard()
      const title = await screen.findByRole("button", {
        name: "Select panel: Runbook",
      })
      const pane = activityPane()
      fireEvent.click(title)
      await waitFor(() => expect(pane).not.toBeVisible())
      const toggle = document.getElementById(activityButtonId)
      expect(toggle).toHaveAttribute("aria-pressed", "false")

      fireEvent.click(toggle!)

      await waitFor(() => expect(activityPane()).toBeVisible())
      expect(activityPane()).toBe(pane)
      expect(articleOf("Runbook")).toHaveAttribute("data-selected")
    })

    async function setPaneAsideBySelecting(title: string) {
      const button = await screen.findByRole("button", {
        name: `Select panel: ${title}`,
      })
      const pane = activityPane()
      fireEvent.click(button)
      await waitFor(() => expect(pane).not.toBeVisible())
      return pane
    }

    function reopenPane() {
      fireEvent.click(document.getElementById(activityButtonId)!)
    }

    it("brings the pane back without focus once nothing is selected", async () => {
      renderDashboard()
      const pane = await setPaneAsideBySelecting("Runbook")
      const title = screen.getByRole("button", {
        name: "Select panel: Runbook",
      })
      title.focus()

      act(() => useSelectionStore.getState().clear())

      await waitFor(() => expect(pane).toBeVisible())
      expect(title).toHaveFocus()
    })

    it("keeps a pane opened again while the selection moves to another panel", async () => {
      renderDashboard()
      const pane = await setPaneAsideBySelecting("Runbook")
      reopenPane()
      await waitFor(() => expect(pane).toBeVisible())

      fireEvent.click(
        screen.getByRole("button", { name: "Select panel: Note" })
      )

      await waitFor(() =>
        expect(articleOf("Note")).toHaveAttribute("data-selected")
      )
      expect(pane).toBeVisible()
    })

    it.each([
      ["viewing a panel", { viewPanel: "p_note" }],
      ["TV mode", { kiosk: "1" }],
    ])(
      "does not set a reopened pane aside again when the selection returns after %s",
      async (_, detour) => {
        const router = renderDashboard()
        const pane = await setPaneAsideBySelecting("Runbook")
        reopenPane()
        await waitFor(() => expect(pane).toBeVisible())

        await act(() =>
          router.navigate({
            to: ".",
            search: (state: Record<string, unknown>) => ({
              ...state,
              ...detour,
            }),
          })
        )
        await act(() =>
          router.navigate({
            to: ".",
            search: (state: Record<string, unknown>) => ({
              ...state,
              viewPanel: undefined,
              kiosk: undefined,
            }),
          })
        )

        await waitFor(() =>
          expect(articleOf("Runbook")).toHaveAttribute("data-selected")
        )
        expect(activityPane()).toBeVisible()
      }
    )

    it("ends the session and brings the pane back when the selected panel leaves the dashboard", async () => {
      const router = renderDashboard()
      const pane = await setPaneAsideBySelecting("Runbook")

      runbookRemoved = true
      await act(() => router.options.context.queryClient.invalidateQueries())

      await waitFor(() =>
        expect(
          screen.queryByRole("button", { name: "Select panel: Runbook" })
        ).toBeNull()
      )
      await waitFor(() => expect(pane).toBeVisible())
    })

    it("shows the pane when the address points it at a thread during the session", async () => {
      const router = renderDashboard()
      const pane = await setPaneAsideBySelecting("Runbook")

      await act(() =>
        router.navigate({
          to: ".",
          search: (state: Record<string, unknown>) => ({
            ...state,
            thread: "t1",
          }),
        })
      )

      await waitFor(() => expect(activityPane()).toBeVisible())
      expect(activityPane()).toBe(pane)
      expect(articleOf("Runbook")).toHaveAttribute("data-selected")
    })

    it("shows the pane for any explicit open, even with nothing in the address changing", async () => {
      renderDashboard()
      const pane = await setPaneAsideBySelecting("Runbook")

      act(() => useActivityStore.getState().setOpen(true))

      await waitFor(() => expect(pane).toBeVisible())
      expect(articleOf("Runbook")).toHaveAttribute("data-selected")
    })

    it("keeps the pane open when a viewer who cannot edit selects a panel", async () => {
      canEdit = false
      renderDashboard()
      const title = await screen.findByRole("button", {
        name: "Select panel: Runbook",
      })
      fireEvent.pointerDown(title, { button: 0 })
      await waitFor(() =>
        expect(articleOf("Runbook")).toHaveAttribute("data-selected")
      )
      expect(activityPane()).toBeVisible()
    })

    it.each(["comment", "ask"] as const)(
      "keeps the pane open for a press in %s mode",
      async (tool) => {
        renderDashboard()
        const title = await screen.findByRole("button", {
          name: "Select panel: Runbook",
        })
        act(() => useCanvasTool.getState().setTool(tool))
        fireEvent.pointerDown(title, { button: 0 })
        expect(activityPane()).toBeVisible()
      }
    )
  })
})
