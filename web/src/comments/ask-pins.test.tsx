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
    prompt: "Why is payments slow?",
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
  const listed: URL[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      listed.push(new URL(request.url))
      return Response.json({ requests, next_cursor: null })
    })
  )
  return listed
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
    const listed = stubRequests([
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
    expect(listed[0].pathname).toBe("/api/v1/observe")
    expect(Object.fromEntries(listed[0].searchParams)).toMatchObject({
      dashboard_id: "d1",
      anchored: "true",
    })
    expect(listed[0].searchParams.has("scope")).toBe(false)
  })

  it("stacks questions at one spot into a counted marker that lists them", async () => {
    stubRequests([
      summary({ id: "r1" }),
      summary({ id: "r2", chat_id: "c2", prompt: "And yesterday?" }),
    ])
    const { user } = renderPins()
    await user.click(
      await screen.findByRole("button", { name: "2 of your questions here" })
    )
    const asked = await screen.findAllByRole("button", {
      name: "Your question on Slow services, The row payments, 440",
    })
    expect(asked).toHaveLength(2)
    expect(asked[1]).toHaveTextContent("And yesterday?")
    await user.click(asked[1])
    expect(useCanvasTool.getState().openTurn).toMatchObject({
      requestId: "r2",
      chatId: "c2",
    })
  })
})
