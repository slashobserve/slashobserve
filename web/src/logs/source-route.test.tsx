import { QueryClient } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { auditDatasourceUid } from "@/audit/audit-source"
import { parseSearch, stringifySearch } from "@/lib/search"
import { useCrumbs } from "@/shell/crumbs"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { LogsSource, logsSourceCrumbs, preloadLogsSource } from "./source-route"

const session = vi.hoisted(() => ({ role: "admin" }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: session.role } }),
}))
vi.mock("./logs-page", () => ({
  LogsPage: ({ uid }: { uid: string }) => <p>logs page for {uid}</p>,
}))

function stubFetch() {
  const fetch = vi.fn(
    async () =>
      new Response("{}", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

afterEach(() => vi.unstubAllGlobals())

describe("LogsSource", () => {
  it("opens any other log source for every role", () => {
    session.role = "viewer"
    renderWithProviders(<LogsSource uid="loki" />)
    expect(screen.getByText("logs page for loki")).toBeInTheDocument()
  })

  it("opens the audit datasource for an Admin", () => {
    session.role = "admin"
    renderWithProviders(<LogsSource uid={auditDatasourceUid} />)
    expect(
      screen.getByText(`logs page for ${auditDatasourceUid}`)
    ).toBeInTheDocument()
  })

  it.each(["editor", "commenter", "viewer"])(
    "shows a %s the Admin-only note instead of the audit datasource",
    (role) => {
      session.role = role
      renderWithProviders(<LogsSource uid={auditDatasourceUid} />)
      expect(
        screen.getByText("Only Admins can open this page.")
      ).toBeInTheDocument()
      expect(screen.queryByText(/logs page for/)).toBeNull()
    }
  )
})

describe("preloadLogsSource", () => {
  const client = () => testQueryClient() as QueryClient

  it("loads the sources for an Admin on the audit datasource", async () => {
    const fetch = stubFetch()
    await preloadLogsSource(client(), "admin", auditDatasourceUid)
    expect(fetch).toHaveBeenCalled()
  })

  it.each(["editor", "commenter", "viewer"] as const)(
    "requests nothing for a %s on the audit datasource",
    async (role) => {
      const fetch = stubFetch()
      await preloadLogsSource(client(), role, auditDatasourceUid)
      expect(fetch).not.toHaveBeenCalled()
    }
  )

  it("still loads the sources for a viewer on another log source", async () => {
    const fetch = stubFetch()
    await preloadLogsSource(client(), "viewer", "loki")
    expect(fetch).toHaveBeenCalled()
  })
})

describe("logsSourceCrumbs", () => {
  it("reads Audit, Log and Explorer on the audit datasource", () => {
    expect(logsSourceCrumbs({ datasourceUid: auditDatasourceUid })).toEqual([
      { label: "Audit" },
      { label: "Log", to: "/audit" },
      { label: "Explorer" },
    ])
  })

  it("reads Logs on every other source", () => {
    expect(logsSourceCrumbs({ datasourceUid: "loki" })).toEqual([
      { label: "Logs", to: "/logs" },
    ])
  })

  it("is what the breadcrumb shows for the route's own address", async () => {
    function Trail() {
      return (
        <p>
          {useCrumbs()
            .crumbs.map((crumb) => crumb.label)
            .join(" / ")}
        </p>
      )
    }
    const root = createRootRoute()
    const source = createRoute({
      getParentRoute: () => root,
      path: "/logs/$datasourceUid",
      staticData: { crumbs: logsSourceCrumbs },
      component: Trail,
    })
    const router = createRouter({
      routeTree: root.addChildren([source]),
      history: createMemoryHistory({
        initialEntries: [`/logs/${auditDatasourceUid}`],
      }),
      parseSearch,
      stringifySearch,
    })
    render(<RouterProvider router={router} />)
    expect(
      await screen.findByText("Audit / Log / Explorer")
    ).toBeInTheDocument()
  })
})
