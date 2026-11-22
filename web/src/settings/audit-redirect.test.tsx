import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
  type AnyRoute,
} from "@tanstack/react-router"
import { render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"

import { parseSearch, stringifySearch } from "@/lib/search"
import { Route as SettingsRoute } from "@/routes/_authed/_shell/settings/index"

const { beforeLoad } = SettingsRoute.options as unknown as {
  beforeLoad: (options: {
    location: { hash: string; search: Record<string, unknown> }
  }) => void
}

async function open(url: string) {
  const root = createRootRoute()
  const settings: AnyRoute = createRoute({
    getParentRoute: () => root,
    path: "/settings",
    beforeLoad: ({ location }) => beforeLoad({ location }),
    component: () => <p>settings page</p>,
  })
  const audit: AnyRoute = createRoute({
    getParentRoute: () => root,
    path: "/audit",
    component: () => <p>audit page</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([settings, audit]),
    history: createMemoryHistory({ initialEntries: [url] }),
    parseSearch,
    stringifySearch,
  })
  render(<RouterProvider router={router} />)
  await screen.findByText(/page$/)
  return router
}

afterEach(() => document.body.replaceChildren())

describe("the old Settings audit log address", () => {
  it("redirects #audit-log to /audit and keeps the query, range and open event", async () => {
    const router = await open(
      "/settings?q=outcome%3Adenied&from=now-7d&to=now-1d&event=3f1c9a52-7d0e-4b8a-9c21-5e6f7a8b9c01#audit-log"
    )

    expect(router.state.location.pathname).toBe("/audit")
    expect(router.state.location.search).toEqual({
      q: "outcome:denied",
      from: "now-7d",
      to: "now-1d",
      event: "3f1c9a52-7d0e-4b8a-9c21-5e6f7a8b9c01",
    })
    expect(router.state.location.hash).toBe("")
  })

  it("drops search values the audit page would not read", async () => {
    const router = await open(
      "/settings?q=a&panel=webhook-add&from=bogus#audit-log"
    )

    expect(router.state.location.pathname).toBe("/audit")
    expect(router.state.location.search).toEqual({ q: "a" })
  })

  it("leaves every other Settings section where it is", async () => {
    const router = await open("/settings#webhooks")

    expect(router.state.location.pathname).toBe("/settings")
    expect(screen.getByText("settings page")).toBeInTheDocument()
  })
})
