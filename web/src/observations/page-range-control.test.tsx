import { cleanup, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { describeRange } from "@/lib/time"
import { renderWithProviders } from "@/test/render"

import { PageRangeControl, pageRangeValue } from "./page-range-control"
import type { Observation } from "./queries"

function observation(over: Partial<Observation>): Observation {
  return {
    id: "o1",
    title: "Checkout 5xx",
    from: null,
    to: null,
    allowed_actions: ["update"],
    ...over,
  } as Observation
}

function stubServer() {
  const bodies: unknown[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "PATCH") bodies.push(await request.clone().json())
      return new Response(null, { status: 599 })
    })
  )
  return bodies
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe("pageRangeValue", () => {
  it("reads no stored range as the last hour", () => {
    expect(pageRangeValue({ from: null, to: null })).toEqual({
      from: "now-1h",
      to: "now",
    })
  })
})

describe("PageRangeControl", () => {
  it("says the page shows the last hour live when it has no range", () => {
    stubServer()
    renderWithProviders(<PageRangeControl observation={observation({})} />)

    expect(
      screen.getByRole("button", { name: "Time range: Last hour, live" })
    ).toBeTruthy()
  })

  it("stores a chosen quick range as the absolute window it means now", async () => {
    const bodies = stubServer()
    vi.spyOn(Date, "now").mockReturnValue(Date.parse("2026-11-20T19:26:00Z"))
    const { user } = renderWithProviders(
      <PageRangeControl observation={observation({})} />
    )

    await user.click(screen.getByRole("button", { name: /^Time range/ }))
    await user.click(screen.getByRole("button", { name: "Last 6 hours" }))

    await waitFor(() =>
      expect(bodies).toEqual([
        { from: "2026-11-20T13:26:00.000Z", to: "2026-11-20T19:26:00.000Z" },
      ])
    )
  })

  it("clears the stored range when the last hour is chosen", async () => {
    const bodies = stubServer()
    const { user } = renderWithProviders(
      <PageRangeControl
        observation={observation({
          from: "2026-11-20T10:00:00Z",
          to: "2026-11-20T11:00:00Z",
        })}
      />
    )

    await user.click(screen.getByRole("button", { name: /^Time range/ }))
    await user.click(screen.getByRole("button", { name: "Last hour, live" }))

    await waitFor(() => expect(bodies).toEqual([{ from: null, to: null }]))
  })

  it("shows the range without a control to someone who cannot edit", () => {
    stubServer()
    renderWithProviders(
      <PageRangeControl
        observation={observation({
          from: "2026-11-20T10:00:00Z",
          to: "2026-11-20T11:00:00Z",
          allowed_actions: [],
        })}
      />
    )

    expect(
      screen.getByText(
        describeRange("2026-11-20T10:00:00Z", "2026-11-20T11:00:00Z")
      )
    ).toBeTruthy()
    expect(screen.queryByRole("button")).toBeNull()
  })
})
