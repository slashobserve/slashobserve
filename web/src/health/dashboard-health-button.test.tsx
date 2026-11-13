import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, test, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { DashboardHealthButton } from "./dashboard-health-button"
import type { HealthItem, HealthProblem } from "./health-labels"

const dashboardId = "d1"

const queryFailed: HealthProblem = {
  kind: "query_failed",
  panel_id: "1",
  panel_title: "Orders",
  variable: null,
  code: "bad_data",
  message: "parse error",
}

const noData: HealthProblem = {
  kind: "no_data",
  panel_id: "2",
  panel_title: "Latency",
  variable: null,
  code: "query_empty",
  message: "query returned no data",
}

const unopened: HealthProblem = {
  kind: "unopened",
  panel_id: null,
  panel_title: null,
  variable: null,
  code: null,
  message: null,
}

function item(fields: Partial<HealthItem> = {}): HealthItem {
  return {
    id: dashboardId,
    title: "Checkout",
    url: "/d/d1/checkout",
    folder: { id: "f1", title: "Shop" },
    can_edit: true,
    state: "failing",
    checked_version: 4,
    checked_at: "2026-11-06T10:00:00Z",
    last_viewed_at: null,
    skipped: null,
    problems: [queryFailed, noData],
    repair: null,
    ...fields,
  } as HealthItem
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(dashboards: HealthItem[]) {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async (request) => {
      const { pathname } = new URL(request.url)
      if (pathname === "/api/v1/agent") return json({ available: true })
      if (pathname === "/api/v1/health/repairs") {
        return json({ queued: [dashboardId], skipped: [] })
      }
      return json({ dashboards, next_cursor: null })
    }
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function renderButton(blockedReason: string | null = null) {
  return renderWithProviders(
    <DashboardHealthButton
      dashboardId={dashboardId}
      blockedReason={blockedReason}
    />,
    { route: { path: "/", url: "/" } }
  )
}

afterEach(() => vi.unstubAllGlobals())

describe("DashboardHealthButton", () => {
  test("renders nothing for a dashboard the check found no problem in", async () => {
    serve([])
    const { container, queryClient } = renderButton()
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    expect(container.querySelector("button")).toBeNull()
  })

  test("renders nothing for a dashboard whose only problem is unopened", async () => {
    serve([item({ problems: [unopened] })])
    const { container, queryClient } = renderButton()
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    expect(container.querySelector("button")).toBeNull()
  })

  test("renders nothing while loading or when the load fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {}))
    )
    const loading = renderButton()
    expect(loading.container.querySelector("button")).toBeNull()
    loading.unmount()

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => json({ error: "x", code: "internal" }, 500))
    )
    const failed = renderButton()
    await waitFor(() => expect(failed.queryClient.isFetching()).toBe(0))
    expect(failed.container.querySelector("button")).toBeNull()
  })

  test("counts the problems on the button", async () => {
    serve([item({ problems: [queryFailed, noData, unopened] })])
    renderButton()
    expect(
      await screen.findByRole("button", { name: "2 problems" })
    ).toBeTruthy()
  })

  test.each([
    ["queued", "Repair queued"],
    ["running", "Repairing"],
  ] as const)("reads %s repair on the button", async (outcome, text) => {
    serve([
      item({
        repair: {
          state: outcome,
          outcome,
          request_id: "r1",
          version: null,
          error: null,
          updated_at: "2026-11-06T10:05:00Z",
        },
      }),
    ])
    renderButton()
    expect(await screen.findByRole("button", { name: text })).toBeTruthy()
  })

  test("opens with Enter, says when and on which version it checked, and returns focus on Escape", async () => {
    serve([item({ state: "outdated" })])
    const { user } = renderButton()
    const trigger = await screen.findByRole("button", { name: "2 problems" })
    trigger.focus()
    await user.keyboard("{Enter}")
    expect(await screen.findByText("Health")).toBeTruthy()
    expect(
      screen.getByText(/on version 4, before the latest version/)
    ).toBeTruthy()
    expect(screen.getByText("Query fails")).toBeTruthy()
    await user.keyboard("{Escape}")
    await waitFor(() => expect(screen.queryByText("Health")).toBeNull())
    expect(document.activeElement).toBe(trigger)
  })

  test("repairs the dashboard and offers Repair all for several repairable problems", async () => {
    const fetch = serve([item()])
    const { user } = renderButton()
    await user.click(await screen.findByRole("button", { name: "2 problems" }))
    await user.click(await screen.findByRole("button", { name: "Repair all" }))
    await waitFor(() =>
      expect(
        fetch.mock.calls.filter(([r]) => r.method === "POST")
      ).toHaveLength(1)
    )
    const post = fetch.mock.calls.find(([r]) => r.method === "POST")?.[0]
    expect(new URL(post?.url ?? "").pathname).toBe("/api/v1/health/repairs")
    expect(await post?.json()).toEqual({ dashboard_ids: [dashboardId] })
  })

  test("offers plain Repair for one repairable problem", async () => {
    serve([item({ problems: [queryFailed] })])
    const { user } = renderButton()
    await user.click(await screen.findByRole("button", { name: "1 problem" }))
    expect(await screen.findByRole("button", { name: "Repair" })).toBeTruthy()
  })

  test("disables Repair with the blocked reason", async () => {
    const fetch = serve([item()])
    const { user } = renderButton("finish the layout first")
    await user.click(await screen.findByRole("button", { name: "2 problems" }))
    const repair = await screen.findByRole("button", { name: "Repair all" })
    expect(repair.getAttribute("aria-disabled")).toBe("true")
    expect(repair.getAttribute("title")).toBe("finish the layout first")
    await user.click(repair)
    expect(fetch.mock.calls.some(([r]) => r.method === "POST")).toBe(false)
  })
})
