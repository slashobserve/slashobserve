import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { MakeDashboardAction, PinAction } from "./finding-actions"
import type { Finding } from "./queries"

const permissions = vi.hoisted(() => ({ edit: false, provisioned: false }))

vi.mock("@/dashboards/permissions", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/dashboards/permissions")>()),
  useLatestProvisioned: () => permissions.provisioned,
  useLatestPermissions: () => ({
    comment: true,
    edit: permissions.edit,
    manage: false,
    create: false,
  }),
}))

function finding(patch: Partial<Finding> = {}): Finding {
  return {
    id: "f1",
    observation_id: "o9",
    agent_request_id: "r1",
    dashboard_id: null,
    folder_id: null,
    ask_on: null,
    restricted: false,
    title: "Checkout errors rose after the deploy",
    markdown: "The error rate doubled at 10:02.",
    evidence: [
      { id: "evidence-1", type: "timeseries", title: "Error rate" },
    ] as unknown as Finding["evidence"],
    from: "2026-11-06T10:00:00Z",
    to: "2026-11-06T11:00:00Z",
    as_of: null,
    annotation_id: null,
    created_by: { kind: "user", id: "u1", name: "Ada" },
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: "r1",
      label: "Observe Agent for Ada",
    },
    created_at: "2026-11-06T11:05:00Z",
    url: "",
    suggested_change: null,
    observe_worthy: false,
    ...patch,
  }
}

const route = { route: { path: "/", url: "/" } }

describe("MakeDashboardAction", () => {
  it("opens a made dashboard with its chat in the side panel", async () => {
    const dashboardId = "33333333-3333-4333-8333-333333333333"
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              dashboard: { id: dashboardId, title: "Checkout errors" },
            }),
            { status: 201, headers: { "Content-Type": "application/json" } }
          )
      )
    )
    const { user, router } = renderWithProviders(
      <MakeDashboardAction finding={finding()} chatId="c1" />,
      route
    )
    await user.click(
      await screen.findByRole("button", { name: "Make a dashboard" })
    )
    await vi.waitFor(() =>
      expect(router?.state.location.pathname).toBe(
        `/d/${dashboardId}/checkout-errors`
      )
    )
    expect(router?.state.location.search).toEqual({ tab: "chat", chat: "c1" })
    vi.unstubAllGlobals()
  })

  it.each([
    ["moves the chat into the side panel with a view transition", "c1", true],
    ["opens a dashboard with no chat to move without one", undefined, false],
  ])("%s", async (_name, chatId, viewTransition) => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              dashboard: {
                id: "33333333-3333-4333-8333-333333333333",
                title: "A",
              },
            }),
            { status: 201, headers: { "Content-Type": "application/json" } }
          )
      )
    )
    const { user, router } = renderWithProviders(
      <MakeDashboardAction finding={finding()} chatId={chatId} />,
      route
    )
    const navigate = vi.spyOn(router!, "navigate")
    await user.click(
      await screen.findByRole("button", { name: "Make a dashboard" })
    )
    await vi.waitFor(() => expect(navigate).toHaveBeenCalledOnce())
    expect(navigate.mock.calls[0][0]).toMatchObject({ viewTransition })
    vi.unstubAllGlobals()
  })
})

describe("PinAction", () => {
  it("offers Pin to dashboard only with Edit on the dashboard", async () => {
    const onDashboard = finding({ dashboard_id: "d1" })
    permissions.edit = false
    const view = renderWithProviders(<PinAction finding={onDashboard} />, route)
    expect(
      screen.queryByRole("button", { name: "Pin to dashboard" })
    ).not.toBeInTheDocument()
    view.unmount()
    permissions.edit = true
    renderWithProviders(<PinAction finding={onDashboard} />, route)
    expect(
      await screen.findByRole("button", { name: "Pin to dashboard" })
    ).toBeVisible()
  })

  it("offers Pin to dashboard on a starter dashboard, where Edit is never shown", async () => {
    permissions.edit = false
    permissions.provisioned = true
    try {
      renderWithProviders(
        <PinAction finding={finding({ dashboard_id: "d1" })} />,
        route
      )
      expect(
        await screen.findByRole("button", { name: "Pin to dashboard" })
      ).toBeVisible()
    } finally {
      permissions.provisioned = false
    }
  })

  it("still shows a pinned finding to someone who cannot pin", async () => {
    permissions.edit = false
    renderWithProviders(
      <PinAction
        finding={finding({ dashboard_id: "d1", annotation_id: "a1" })}
      />,
      route
    )
    expect(await screen.findByText("Pinned to the dashboard")).toBeVisible()
  })
})
