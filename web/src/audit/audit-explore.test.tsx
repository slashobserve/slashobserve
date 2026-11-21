import { createRouter } from "@tanstack/react-router"
import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { logsPath } from "@/lib/links"
import { routeTree } from "@/routeTree.gen"
import { Route } from "@/routes/_authed/_shell/logs/slashobserve-audit"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { auditDatasourceUid } from "./audit-source"

const session = vi.hoisted(() => ({ role: "admin" }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: session.role } }),
}))
vi.mock("@/logs/logs-page", () => ({
  LogsPage: ({ uid }: { uid: string }) => <p>logs page for {uid}</p>,
}))

type Options = {
  component: () => React.ReactNode
  loader: (options: {
    context: { principal: { role: string }; queryClient: unknown }
  }) => unknown
  staticData: { crumbs: { label: string; to?: string }[] }
}
const options = Route.options as unknown as Options
const Page = options.component

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

describe("the audit explorer route", () => {
  it("is the address of the audit datasource", () => {
    const router = createRouter({
      routeTree,
      context: { queryClient: testQueryClient() },
    })
    expect(router.routesByPath[logsPath(auditDatasourceUid) as never]).toBe(
      Route
    )
  })

  it("opens the logs explorer on the audit datasource for an Admin", async () => {
    session.role = "admin"
    const fetch = stubFetch()
    renderWithProviders(<Page />)

    expect(
      await screen.findByText(`logs page for ${auditDatasourceUid}`)
    ).toBeInTheDocument()
    await options.loader({
      context: {
        principal: { role: "admin" },
        queryClient: testQueryClient(),
      },
    })
    expect(fetch).toHaveBeenCalled()
  })

  it.each(["editor", "commenter", "viewer"])(
    "shows a %s the Admin-only note and requests nothing",
    async (role) => {
      session.role = role
      const fetch = stubFetch()
      renderWithProviders(<Page />)

      expect(
        screen.getByText("Only Admins can open this page.")
      ).toBeInTheDocument()
      expect(screen.queryByText(/logs page for/)).toBeNull()
      await options.loader({
        context: { principal: { role }, queryClient: testQueryClient() },
      })
      expect(fetch).not.toHaveBeenCalled()
    }
  )

  it("reads Audit then Explore in the breadcrumb", () => {
    expect(options.staticData.crumbs).toEqual([
      { label: "Audit", to: "/audit" },
      { label: "Explore" },
    ])
  })
})
