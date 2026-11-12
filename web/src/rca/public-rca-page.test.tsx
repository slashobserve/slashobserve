import { Suspense } from "react"
import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { PublicRcaPage, publicRows } from "./public-rca-page"
import type { RcaBlock } from "./queries"

const token = `pub_${"a".repeat(43)}`
const snapshotId = "5c1f2d9e-0b53-4c43-9d3a-0e6b8f4c7a21"
const leakedQuery = "sum(rate(http_requests_total[5m]))"

const from = new Date(2026, 10, 6, 10, 0).toISOString()
const to = new Date(2026, 10, 6, 11, 30).toISOString()

const view = {
  revision: 2,
  title: "Checkout outage",
  summary: "A bad deploy doubled the error rate.",
  observation_state: "resolved",
  from,
  to,
  published_by: "Priya",
  created_at: to,
  blocks: [
    {
      kind: "text",
      markdown: "## Cause\n\nThe **new build** timed out.",
      width: "full",
    },
    {
      kind: "snapshot",
      snapshot_id: snapshotId,
      title: "Error rate",
      panel: { type: "timeseries", query: leakedQuery },
      width: "full",
    },
  ],
}

const frames = {
  frames: [],
  meta: {
    kind: "range",
    from,
    to,
    step_ms: 60000,
    queries: [],
    cached: false,
    ran_at: to,
  },
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

function mockServer() {
  const paths: string[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn((request: Request) => {
      const path = new URL(request.url).pathname
      paths.push(path)
      if (path === `/api/v1/public/${token}/rca`) {
        return Promise.resolve(json(view))
      }
      if (path === `/api/v1/public/${token}/rca/snapshots/${snapshotId}`) {
        return Promise.resolve(json(frames))
      }
      return Promise.resolve(new Response(null, { status: 401 }))
    })
  )
  return paths
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("PublicRcaPage", () => {
  it("shows the frozen revision, its text and one panel, and asks only the public routes", async () => {
    const paths = mockServer()
    const { container } = renderWithProviders(
      <Suspense fallback={null}>
        <PublicRcaPage token={token} />
      </Suspense>
    )

    expect(
      await screen.findByRole("heading", { name: "Checkout outage", level: 1 })
    ).toBeTruthy()
    expect(screen.getByText(/^Revision 2, published by Priya, /)).toBeTruthy()
    expect(screen.getByText("Resolved")).toBeTruthy()
    expect(
      screen.getByText("A bad deploy doubled the error rate.")
    ).toBeTruthy()
    expect(screen.getByRole("heading", { name: "Cause" })).toBeTruthy()
    expect(screen.getByText("new build").tagName).toBe("STRONG")
    expect(
      await screen.findByText("Frozen, 2026-11-06 10:00 to 2026-11-06 11:30")
    ).toBeTruthy()
    expect(await screen.findByText("Error rate")).toBeTruthy()
    expect(await screen.findByText("No data")).toBeTruthy()
    expect(container.innerHTML).not.toContain("http_requests_total")
    expect(paths.length).toBeGreaterThan(0)
    for (const path of paths) {
      expect(path.startsWith("/api/v1/public/")).toBe(true)
    }
  })
})

describe("publicRows", () => {
  function block(kind: "text" | "snapshot", width: "full" | "half"): RcaBlock {
    return kind === "text"
      ? { kind, markdown: "text", width }
      : { kind, snapshot_id: snapshotId, title: "Panel", panel: {}, width }
  }

  it("pairs consecutive half blocks and leaves a lone half block on its own", () => {
    const rows = publicRows([
      block("text", "full"),
      block("snapshot", "half"),
      block("snapshot", "half"),
      block("snapshot", "half"),
      block("text", "full"),
    ])
    expect(rows.map((row) => row.length)).toEqual([1, 2, 1, 1])
  })
})
