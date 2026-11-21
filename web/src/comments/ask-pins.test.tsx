import { screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useCanvasTool } from "@/dashboards/canvas-tools"
import { createAnchorLayoutStore } from "@/panels/anchor-layout"
import type { Anchor } from "@/panels/anchors"
import { renderWithProviders } from "@/test/render"

import { AskPins } from "./ask-pins"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "admin" } }),
}))

const onRow: Anchor = {
  kind: "row",
  pos: { x: 0.5, y: 0.5 },
  row: { service: "payments" },
  shown: "440",
}

function summary(fields: Record<string, unknown>) {
  return {
    chat_id: "c1",
    panel_id: "p1",
    anchor: onRow,
    requested_by: { kind: "user", id: "u1", name: "Ada" },
    created_at: "2026-11-21T14:31:00Z",
    read_only: false,
    status: "succeeded",
    ...fields,
  }
}

function stubRequests(requests: Record<string, unknown>[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ requests, next_cursor: null }))
  )
}

function renderPins() {
  return renderWithProviders(
    <AskPins
      dashboardId="d1"
      panelId="p1"
      panelTitle="Slow services"
      surfaceId="s1"
      store={createAnchorLayoutStore()}
      body={{ width: 400, height: 200 }}
      dataShown
    />,
    { route: { path: "/", url: "/" } }
  )
}

beforeEach(() => {
  useCanvasTool.setState({
    tool: "select",
    draft: null,
    openThreadId: null,
    postedThreadId: null,
    openTurn: null,
  })
})

afterEach(() => vi.unstubAllGlobals())

describe("AskPins", () => {
  it("draws the viewer's own question as a named pin and opens its turn on click", async () => {
    stubRequests([
      summary({ id: "r1" }),
      summary({ id: "r2", requested_by: { kind: "user", id: "u2" } }),
      summary({ id: "r3", read_only: true }),
    ])
    const { user } = renderPins()
    const pin = await screen.findByRole("button", {
      name: "Your question on Slow services, The row payments, 440",
    })
    expect(screen.getAllByRole("button")).toHaveLength(1)
    expect(pin).toHaveAttribute("aria-expanded", "false")
    await user.click(pin)
    expect(useCanvasTool.getState().openTurn).toMatchObject({
      requestId: "r1",
      chatId: "c1",
      panelId: "p1",
      anchor: onRow,
      surface: "s1",
    })
    expect(pin).toHaveAttribute("aria-expanded", "true")
  })
})
