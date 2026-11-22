import { act, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useCanvasTool } from "@/dashboards/canvas-tools"
import {
  createAnchorLayoutStore,
  type AnchorLayoutStore,
} from "@/panels/anchor-layout"
import type { Anchor } from "@/panels/anchors"
import { renderWithProviders } from "@/test/render"

import { AskPins } from "./ask-pins"
import { useCanvasSurface } from "./canvas-composer"
import type { CanvasSubject } from "./canvas-host"
import type { PinView } from "./canvas-target"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "admin" } }),
}))

const outOfRange = vi.hoisted(() => ({
  hitTest: () => null,
  locate: () => ({ x: 0, y: 0, width: 400, height: 200 }),
  hides: (_layout: never, anchor: { kind: string }) => anchor.kind === "range",
}))

vi.mock("@/panels/anchor-layout", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/panels/anchor-layout")>()),
  useAnchorsModule: (type: string | null) =>
    type === "timeseries" ? outOfRange : null,
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
    trigger: "person",
    observation_id: null,
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

function StoredPins({
  store,
  view,
  subject,
  ownerId,
}: {
  store: AnchorLayoutStore
  view: PinView
  subject: CanvasSubject
  ownerId: string
}) {
  return (
    <AskPins
      subject={subject}
      ownerId={ownerId}
      panelTitle="Slow services"
      surfaceId="s1"
      surface={useCanvasSurface(store)}
      body={{ width: 400, height: 200 }}
      view={view}
    />
  )
}

const onDashboard: CanvasSubject = { kind: "dashboard", id: "d1" }

function renderPins(
  view: PinView = "data",
  store = createAnchorLayoutStore(),
  { subject = onDashboard, ownerId = "p1" } = {}
) {
  return renderWithProviders(
    <StoredPins
      store={store}
      view={view}
      subject={subject}
      ownerId={ownerId}
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
      name: "Your question on Slow services, Row payments, 440",
    })
    expect(screen.getAllByRole("button")).toHaveLength(1)
    expect(pin).toHaveAttribute("aria-expanded", "false")
    await user.click(pin)
    expect(useCanvasTool.getState().openTurn).toMatchObject({
      requestId: "r1",
      chatId: "c1",
      ownerId: "p1",
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

  it("pins only private asks, not an investigation or a mention that answers on the observation", async () => {
    const listed = stubRequests([
      summary({ id: "r1", panel_id: null, block_id: "b1" }),
      summary({
        id: "r2",
        panel_id: null,
        block_id: "b1",
        observation_id: "o1",
        anchor: { ...onRow, row: { service: "checkout" } },
      }),
      summary({
        id: "r3",
        panel_id: null,
        block_id: "b1",
        trigger: "mention",
        anchor: { ...onRow, row: { service: "search" } },
      }),
    ])
    renderPins("data", createAnchorLayoutStore(), {
      subject: { kind: "observation", id: "o1" },
      ownerId: "b1",
    })
    await screen.findByRole("button", {
      name: "Your question on Slow services, Row payments, 440",
    })
    expect(screen.getAllByRole("button")).toHaveLength(1)
    expect(Object.fromEntries(listed[0].searchParams)).toMatchObject({
      observation_id: "o1",
      anchored: "true",
    })
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
    expect(
      await screen.findByRole("button", {
        name: "Why is payments slow? Your question on Slow services, Row payments, 440",
      })
    ).toBeInTheDocument()
    await user.click(
      screen.getByRole("button", {
        name: "And yesterday? Your question on Slow services, Row payments, 440",
      })
    )
    expect(useCanvasTool.getState().openTurn).toMatchObject({
      requestId: "r2",
      chatId: "c2",
    })
  })

  it("does not poll while a question works, since a pin does not change with its answer", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const listed = stubRequests([summary({ id: "r1", status: "running" })])
      renderPins()
      await screen.findByRole("button", {
        name: "Your question on Slow services, Row payments, 440",
      })
      await act(() => vi.advanceTimersByTimeAsync(30_000))
      expect(listed).toHaveLength(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it("draws no pin while the panel is not showing its data", async () => {
    const listed = stubRequests([summary({ id: "r1" })])
    renderPins("hidden")
    await waitFor(() => expect(listed).toHaveLength(1))
    expect(screen.queryByRole("button")).toBeNull()
  })

  it("draws no pin on a spot the panel does not show, such as a range outside the shown time", async () => {
    const listed = stubRequests([
      summary({ id: "r1" }),
      summary({
        id: "r2",
        chat_id: "c2",
        anchor: {
          kind: "range",
          pos: { x: 0.2, y: 0.5 },
          from: "2026-11-20T10:00:00Z",
          to: "2026-11-20T11:00:00Z",
        },
      }),
    ])
    const store = createAnchorLayoutStore()
    store.publish("timeseries", {})
    renderPins("data", store)
    await waitFor(() => expect(listed).toHaveLength(1))
    expect(
      await screen.findByRole("button", {
        name: "Your question on Slow services, Row payments, 440",
      })
    ).toBeInTheDocument()
    expect(screen.getAllByRole("button")).toHaveLength(1)
  })
})
