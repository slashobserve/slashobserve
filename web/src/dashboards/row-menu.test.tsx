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
import type { ReactNode } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { components } from "@/api/schema.gen"
import { testQueryClient } from "@/test/render"

import { DashboardRowMenu } from "./row-menu"

type Summary = components["schemas"]["DashboardListItem"]

const shopId = "22222222-2222-4222-8222-222222222222"

const folders = [
  {
    id: shopId,
    title: "Shop",
    general: false,
    personal: false,
    kind: "open",
    allowed_actions: [
      "create_in",
      "move_into",
      "rename",
      "set_permissions",
      "delete",
    ],
    permissions: { comment: true, edit: true, manage: true, create: true },
  },
]

function dashboard(over: Partial<Summary>): Summary {
  return {
    id: "44444444-4444-4444-8444-444444444444",
    title: "Audit overview",
    latest_version: 2,
    folder: { id: shopId, title: "Shop", general: false, personal: false },
    permissions: { comment: true, edit: true, manage: true, create: true },
    provisioned: false,
    ...over,
  } as Summary
}

function stubServer() {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ folders }), {
          headers: { "Content-Type": "application/json" },
        })
    )
  )
}

function renderMenu(ui: ReactNode) {
  const root = createRootRoute()
  const authed = createRoute({
    getParentRoute: () => root,
    id: "_authed",
    beforeLoad: () => ({
      principal: { kind: "user", id: "9", name: "Ana", role: "admin" },
    }),
  })
  const page = createRoute({
    getParentRoute: () => authed,
    path: "/",
    component: () => ui,
  })
  const router = createRouter({
    routeTree: root.addChildren([authed.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={testQueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { user }
}

afterEach(() => vi.unstubAllGlobals())

describe("DashboardRowMenu", () => {
  it("offers a starter Duplicate and Delete but no Move", async () => {
    stubServer()
    const { user } = renderMenu(
      <DashboardRowMenu
        dashboard={dashboard({
          provisioned: true,
          permissions: {
            comment: true,
            edit: false,
            manage: true,
            create: true,
          },
        })}
      />
    )
    await user.click(
      await screen.findByRole("button", { name: "More for Audit overview" })
    )
    expect(
      await screen.findByRole("menuitem", { name: "Duplicate..." })
    ).toBeVisible()
    expect(
      screen.getByRole("menuitem", { name: "Delete dashboard..." })
    ).toBeVisible()
    expect(screen.queryByText("Move to")).toBeNull()
    expect(screen.queryByRole("menuitemradio")).toBeNull()
  })

  it("offers an ordinary dashboard with Edit the folder choices too", async () => {
    stubServer()
    const { user } = renderMenu(<DashboardRowMenu dashboard={dashboard({})} />)
    await user.click(
      await screen.findByRole("button", { name: "More for Audit overview" })
    )
    expect(await screen.findAllByRole("menuitemradio")).not.toHaveLength(0)
  })
  it("moves focus to the next dashboard of its own grid after a delete, not a row of another grid", async () => {
    stubServer()
    const first = dashboard({ id: "d0", title: "First" })
    const rowAttributes = (id: string, index: number) => ({
      role: "row",
      "row-id": id,
      "row-index": String(index),
    })
    const { user } = renderMenu(
      <>
        <div role="grid" aria-label="Dashboards">
          <div {...rowAttributes("d0", 0)}>
            <DashboardRowMenu dashboard={first} />
          </div>
          <div {...rowAttributes("d1", 1)}>
            <a href="/d/d1">Second</a>
          </div>
        </div>
        <div role="grid" aria-label="Observations">
          <div {...rowAttributes("o1", 0)}>
            <a href="/o/o1">Login failures</a>
          </div>
        </div>
      </>
    )
    await user.click(
      await screen.findByRole("button", { name: "More for First" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Delete dashboard..." })
    )
    await user.click(
      await screen.findByRole("button", { name: "Delete dashboard" })
    )
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "Second" })).toHaveFocus()
    )
  })
})
