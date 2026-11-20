import type { ReactNode } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"

import { AnnotationControls } from "@/annotations/annotation-controls"
import { annotationKeys } from "@/api/keys"
import { parseSearch, stringifySearch } from "@/lib/search"
import { testQueryClient } from "@/test/render"

import { ActivityToggle } from "./activity-toggle"
import { DashboardTopRow } from "./dashboard-top-row"
import { MoreMenu } from "./more-menu"

function setNarrow(matches: boolean) {
  window.matchMedia = (query: string) =>
    ({
      matches,
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    }) as unknown as MediaQueryList
}

beforeAll(() => setNarrow(false))
afterEach(() => {
  document.querySelector("aside")?.remove()
  setNarrow(false)
  vi.unstubAllGlobals()
})

async function renderRow(
  controls: ReactNode[],
  folder: { id: string; title: string } | null = null,
  actions: ReactNode[] = [],
  views: string | null = null
) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(annotationKeys.range("d1", "now-1h", "now"), [
    { tags: ["deploy"] },
  ])
  const root = createRootRoute()
  const authed = createRoute({ getParentRoute: () => root, id: "_authed" })
  const dashboard = createRoute({
    getParentRoute: () => authed,
    path: "d/$dashboardId",
  })
  const page = createRoute({
    getParentRoute: () => dashboard,
    path: "{-$slug}",
    component: () => (
      <DashboardTopRow
        title="Checkout"
        folder={folder}
        madeBy="Web"
        views={views}
        versionControl={null}
        controls={controls}
        actions={actions}
      />
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([
      authed.addChildren([dashboard.addChildren([page])]),
    ]),
    history: createMemoryHistory({ initialEntries: ["/d/d1/checkout"] }),
    parseSearch,
    stringifySearch,
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByText("Checkout")
}

describe("DashboardTopRow", () => {
  it("shows the annotation filter only when the page passes it", async () => {
    await renderRow([<AnnotationControls key="annotations" />])
    expect(
      await screen.findByRole("button", { name: /Annotations: All/ })
    ).toBeVisible()
  })

  it("shows no annotation menu on a page that passes no controls", async () => {
    await renderRow([])
    expect(screen.queryByRole("button", { name: /Annotations/ })).toBeNull()
  })

  it("keeps the crumbs on a wide page", async () => {
    await renderRow([], { id: "f1", title: "Shop" })
    expect(screen.getByRole("link", { name: "Dashboards" })).toBeVisible()
    expect(screen.getByRole("link", { name: "Shop" })).toBeVisible()
    expect(screen.getByText("Web")).toBeVisible()
  })

  it("wraps the controls under the title when they do not fit beside it", async () => {
    await renderRow([<span key="c">Controls</span>], {
      id: "f1",
      title: "Shop",
    })
    const header = screen.getByRole("banner")
    expect(header).toHaveClass("flex-wrap")
    expect(screen.getByRole("navigation", { name: "Breadcrumb" })).toHaveClass(
      "flex-[1_1_22rem]"
    )
    expect(screen.getByText("Controls").parentElement).toHaveClass(
      "max-w-full",
      "flex-wrap"
    )
  })

  it("lets the view count and the author shrink before the title", async () => {
    await renderRow([], null, [], "1 view in the last 30 days")
    const title = screen.getByRole("heading", { level: 1 })
    expect(title).toHaveAttribute("title", "Checkout")
    expect(title).not.toHaveClass("shrink-[1000]")
    for (const text of ["Web", "1 view in the last 30 days"]) {
      const item = screen.getByText(text)
      expect(item).toHaveClass("shrink-[1000]")
      expect(item).toHaveAttribute("title", text)
    }
  })

  it("leaves the title alone with the version on a phone", async () => {
    setNarrow(true)
    await renderRow([], { id: "f1", title: "Shop" })
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Checkout"
    )
    expect(screen.queryByRole("link", { name: "Dashboards" })).toBeNull()
    expect(screen.queryByRole("link", { name: "Shop" })).toBeNull()
    expect(screen.queryByText("Web")).toBeNull()
  })

  it("closes the Controls sheet before it opens Activity on a phone", async () => {
    setNarrow(true)
    const user = userEvent.setup()
    const onToggle = vi.fn()
    await renderRow([], null, [
      <ActivityToggle
        key="activity"
        pressed={false}
        openCount={0}
        onToggle={onToggle}
      />,
    ])
    const opener = screen.getByRole("button", { name: "Controls" })
    await user.click(opener)
    await user.click(await screen.findByRole("button", { name: "Activity" }))
    expect(onToggle).toHaveBeenCalledOnce()
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull())
  })

  it("closes the Controls sheet before Edit Spec JSON opens its pane on a phone", async () => {
    setNarrow(true)
    const user = userEvent.setup()
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("{}", { status: 404 }))
    )
    const onEditJson = vi.fn()
    await renderRow([], null, [
      <MoreMenu
        key="more"
        dashboardId="d1"
        mayEdit
        detailsBlockedBy={null}
        onAddPanel={null}
        onEditJson={onEditJson}
        duplicateBlockedBy={null}
        deleteBlockedBy={null}
        exportBlockedBy={null}
        onEditDetails={() => {}}
        onDuplicate={null}
        onDelete={() => {}}
        onExport={() => {}}
        onTvMode={() => {}}
        onTvModeFolder={null}
        cursorSync="off"
        onCursorSyncChange={() => {}}
      />,
    ])
    await user.click(screen.getByRole("button", { name: "Controls" }))
    await user.click(await screen.findByRole("button", { name: /^More:/ }))
    await user.click(
      await screen.findByRole("menuitem", { name: "Edit Spec JSON" })
    )
    expect(onEditJson).toHaveBeenCalledOnce()
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull())
  })

  it("moves focus into the pane that a Controls action opened when the open sheet refused it, not back to the Controls button", async () => {
    setNarrow(true)
    const user = userEvent.setup()
    const openPane = () => {
      const pane = document.createElement("aside")
      pane.setAttribute("data-focus-pending", "")
      pane.setAttribute("aria-labelledby", "pane-heading")
      const heading = document.createElement("h2")
      heading.id = "pane-heading"
      heading.tabIndex = -1
      heading.textContent = "Activity pane"
      pane.append(heading)
      document.body.append(pane)
    }
    await renderRow([], null, [
      <ActivityToggle
        key="activity"
        pressed={false}
        openCount={0}
        onToggle={openPane}
      />,
    ])
    await user.click(screen.getByRole("button", { name: "Controls" }))
    await user.click(await screen.findByRole("button", { name: "Activity" }))
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull())
    expect(document.activeElement).toBe(
      screen.getByRole("heading", { name: "Activity pane" })
    )
  })

  it("returns focus to the Controls button when the sheet closes with no pane opened", async () => {
    setNarrow(true)
    const user = userEvent.setup()
    await renderRow([])
    const opener = screen.getByRole("button", { name: "Controls" })
    await user.click(opener)
    await user.keyboard("{Escape}")
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull())
    expect(document.activeElement).toBe(opener)
  })

  it("keeps the Controls opener findable for the panes to return focus to", async () => {
    setNarrow(true)
    await renderRow([])
    expect(screen.getByRole("button", { name: "Controls" })).toHaveAttribute(
      "id",
      "dashboard-controls-button"
    )
  })
})
