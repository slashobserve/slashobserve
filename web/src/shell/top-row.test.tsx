import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { PageHeader } from "@/components/page-header"
import { parseSearch, stringifySearch } from "@/lib/search"
import { testQueryClient } from "@/test/render"

import { AppShell } from "./app-shell"
import type { Crumb } from "./crumbs"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({
    principal: { id: "u1", name: "Ana Lima", email: "a@x.io", role: "admin" },
  }),
}))

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            items: [],
            datasources: [],
            observations: [],
            next_cursor: null,
            unread_count: 0,
          }),
          { status: 200, headers: { "content-type": "application/json" } }
        )
    )
  )
  return () => vi.unstubAllGlobals()
})

async function renderPage(crumbs: Crumb[] | undefined, heading: string) {
  const root = createRootRoute({ component: AppShell })
  const page = createRoute({
    getParentRoute: () => root,
    path: "/page",
    staticData: crumbs === undefined ? {} : { crumbs },
    component: () => <PageHeader title={heading} />,
  })
  const router = createRouter({
    routeTree: root.addChildren([page]),
    history: createMemoryHistory({ initialEntries: ["/page"] }),
    parseSearch,
    stringifySearch,
  })
  render(
    <QueryClientProvider client={testQueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole("heading", { level: 1, name: heading })
}

function topRow() {
  return screen.getAllByRole("banner").find((header) => {
    return header.querySelector("[data-slot=top-row-actions]") !== null
  })!
}

describe("top row title", () => {
  it("shows no crumb when a lone crumb only repeats the page heading", async () => {
    await renderPage([{ label: "Dashboards" }], "Dashboards")
    expect(
      within(topRow()).queryByRole("navigation", { name: "Breadcrumb" })
    ).toBeNull()
    expect(within(topRow()).queryByText("Dashboards")).toBeNull()
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1)
  })

  it("keeps the crumb when the heading says something else", async () => {
    await renderPage([{ label: "Health" }], "Dashboard health")
    expect(within(topRow()).getByText("Health")).toBeTruthy()
  })

  it("shows no breadcrumb and collapses the row on a page without crumbs, such as Home", async () => {
    await renderPage(undefined, "What are you working on?")
    expect(
      within(topRow()).queryByRole("navigation", { name: "Breadcrumb" })
    ).toBeNull()
    expect(within(topRow()).queryByText("Home")).toBeNull()
    expect(topRow().hasAttribute("data-crumb-hidden")).toBe(true)
    expect(
      topRow().querySelector("[data-slot=top-row-actions]")?.childElementCount
    ).toBe(0)
  })

  it("keeps a trail of several crumbs", async () => {
    await renderPage(
      [{ label: "Dashboards", to: "/dashboards" }, { label: "Folder" }],
      "Dashboards"
    )
    expect(within(topRow()).getByText("Folder")).toBeTruthy()
  })
})
