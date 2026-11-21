import { cleanup, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Block } from "@/observations/queries"
import { renderWithProviders } from "@/test/render"

import { PanelBlock } from "./panel-block"

vi.mock("@/dashboards/panel-container", () => ({
  PanelContainer: ({
    onPinRange,
  }: {
    onPinRange?: (fromMs: number, toMs: number) => void
  }) => (
    <button
      type="button"
      onClick={() =>
        onPinRange?.(
          Date.parse("2026-11-20T10:00:00Z"),
          Date.parse("2026-11-20T10:30:00Z")
        )
      }
    >
      Drag across the chart
    </button>
  ),
}))

vi.mock("react-grid-layout", () => ({
  useContainerWidth: () => ({
    width: 600,
    mounted: true,
    containerRef: { current: null },
  }),
}))

const block = {
  id: "b1",
  observation_id: "o1",
  version: 3,
  kind: "panel",
  body: {
    panel: {
      type: "timeseries",
      title: "Error rate",
      datasource: "prom-prod",
      queries: [{ query: "up" }],
    },
  },
  allowed_actions: ["update"],
  created_by: {
    kind: "user",
    id: "u1",
    name: "Priya",
    origin: "web",
    client: null,
    run_id: null,
    label: "Priya",
  },
} as unknown as Block

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe("PanelBlock drag to pin", () => {
  it("ignores a second drag while the first pin is still being saved", async () => {
    const fetch = vi.fn<(request: Request) => Promise<Response>>(
      async (request) =>
        request.method === "PATCH"
          ? new Promise<Response>(() => {})
          : new Response(JSON.stringify({ available: false }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            })
    )
    vi.stubGlobal("fetch", fetch)
    const { user } = renderWithProviders(
      <PanelBlock block={block} height={240} />,
      { route: { path: "/o/$observationId", url: "/o/o1" } }
    )

    const drag = await screen.findByRole("button", {
      name: "Drag across the chart",
    })
    await user.click(drag)
    await user.click(drag)

    const writes = fetch.mock.calls.filter(
      ([request]) => request.method === "PATCH"
    )
    expect(writes).toHaveLength(1)
  })
})
