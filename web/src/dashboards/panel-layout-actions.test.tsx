import type { ReactNode } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, test, vi } from "vitest"

import { testQueryClient } from "@/test/render"
import type { Dashboard, LayoutPanel } from "@/spec"

import { usePanelLayoutActions } from "./panel-layout-actions"

const panel = {
  id: "a",
  type: "text",
  title: "A",
  gridPos: { x: 0, y: 0, w: 12, h: 4 },
  content: "a",
} as LayoutPanel

const spec = {
  specVersion: "0",
  title: "Shop",
  time: { from: "now-1h", to: "now" },
  panels: [panel],
} as Dashboard

afterEach(() => vi.unstubAllGlobals())

describe("usePanelLayoutActions", () => {
  test("duplicates a panel whose query returns no data as an unverified copy", async () => {
    const bodies: unknown[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        bodies.push(await request.json())
        return new Response(JSON.stringify({}), {
          status: 500,
          headers: { "Content-Type": "application/json" },
        })
      })
    )
    const queryClient = testQueryClient()
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => usePanelLayoutActions("d1", spec, 3), {
      wrapper,
    })
    act(() => result.current.duplicate(panel))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).toMatchObject({ accept_empty: true, parent_version: 3 })
  })
})
