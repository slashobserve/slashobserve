import type { ReactNode } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import { renderHook, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, test, vi } from "vitest"

import { testQueryClient } from "@/test/render"

import type { HealthItem, HealthProblem } from "./health-labels"
import { usePanelHealth } from "./use-dashboard-health"

const dashboardId = "d1"

function problem(fields: Partial<HealthProblem>): HealthProblem {
  return {
    kind: "query_failed",
    panel_id: null,
    panel_title: null,
    variable: null,
    code: null,
    message: null,
    ...fields,
  }
}

const item = {
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
  problems: [
    problem({ panel_id: "1", panel_title: "Orders" }),
    problem({ kind: "variable_failed", variable: "env" }),
  ],
  repair: null,
} as HealthItem

function serve(dashboards: HealthItem[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const body = new URL(request.url).pathname.endsWith("/agent")
        ? { available: true }
        : { dashboards, next_cursor: null }
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      })
    })
  )
}

function renderPanelHealth(drawnVersion: number, blockedReason: string | null) {
  const queryClient = testQueryClient()
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return renderHook(
    (props: { drawnVersion: number; blockedReason: string | null }) =>
      usePanelHealth(dashboardId, props.drawnVersion, props.blockedReason),
    { wrapper, initialProps: { drawnVersion, blockedReason } }
  )
}

afterEach(() => vi.unstubAllGlobals())

describe("usePanelHealth", () => {
  test("maps the panels with problems and nothing else once loaded", async () => {
    serve([item])
    const { result } = renderPanelHealth(4, null)
    expect(result.current.size).toBe(0)
    await waitFor(() => expect([...result.current.keys()]).toEqual(["1"]))
    expect(result.current.get("1")?.item.id).toBe(dashboardId)
    expect(result.current.get("1")?.problems).toHaveLength(1)
    expect(result.current.get("2")).toBeUndefined()
  })

  test("is empty when another version than the checked one is drawn", async () => {
    serve([item])
    const { result, rerender } = renderPanelHealth(4, null)
    await waitFor(() => expect(result.current.size).toBe(1))
    rerender({ drawnVersion: 5, blockedReason: null })
    expect(result.current.size).toBe(0)
  })

  test("keeps the same map across renders and changes it with the blocked reason", async () => {
    serve([item])
    const { result, rerender } = renderPanelHealth(4, null)
    await waitFor(() => expect(result.current.size).toBe(1))
    const loaded = result.current
    rerender({ drawnVersion: 4, blockedReason: null })
    expect(result.current).toBe(loaded)
    rerender({ drawnVersion: 4, blockedReason: "finish the layout first" })
    expect(result.current).not.toBe(loaded)
    expect(result.current.get("1")?.blockedReason).toBe(
      "finish the layout first"
    )
  })

  test("returns the same empty map every render for a healthy dashboard", async () => {
    serve([])
    const first = renderPanelHealth(4, null)
    const second = renderPanelHealth(4, null)
    await waitFor(() => expect(first.result.current.size).toBe(0))
    expect(second.result.current).toBe(first.result.current)
  })
})
