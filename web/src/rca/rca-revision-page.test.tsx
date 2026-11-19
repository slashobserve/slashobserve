import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { RcaRevisionPage } from "./rca-revision-page"

const observationId = "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01"
const snapshotId = "5c1f2d9e-0b53-4c43-9d3a-0e6b8f4c7a21"
const from = new Date(2026, 10, 6, 10, 0).toISOString()
const to = new Date(2026, 10, 6, 11, 30).toISOString()

const rca = {
  id: "r1",
  observation_id: observationId,
  revision: 1,
  title: "Checkout outage",
  summary: "A bad deploy doubled the error rate.",
  observation_state: "open",
  from,
  to,
  created_by: {
    kind: "user",
    id: "u1",
    name: "Priya",
    origin: "web",
    client: null,
    run_id: null,
    label: "Priya",
  },
  created_at: to,
  blocks: [
    { kind: "text", markdown: "## Cause\n\nA bad deploy.", width: "full" },
    {
      kind: "snapshot",
      snapshot_id: snapshotId,
      title: "Error rate",
      panel: { type: "timeseries" },
      width: "full",
    },
  ],
}

const snapshot = {
  id: snapshotId,
  observation_id: observationId,
  panel: { type: "timeseries", title: "Error rate" },
  query: [],
  user_scoped: false,
  time_from: from,
  time_to: to,
  data: {
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
  },
}

function serve(rcaStatus: number) {
  const paths: string[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn((request: Request) => {
      const path = new URL(request.url).pathname
      paths.push(path)
      const reply = (status: number, body: unknown) =>
        Promise.resolve(
          new Response(JSON.stringify(body), {
            status,
            headers: { "Content-Type": "application/json" },
          })
        )
      if (path === `/api/v1/observations/${observationId}/rcas/1`) {
        return rcaStatus === 200
          ? reply(200, { rca })
          : reply(rcaStatus, { error: "not found", code: "not_found" })
      }
      if (path === `/api/v1/observations/${observationId}/rcas`) {
        return reply(200, {
          rcas: [
            { ...rca, revision: 2 },
            { ...rca, revision: 1 },
          ],
        })
      }
      if (path === `/api/v1/snapshots/${snapshotId}`) {
        return reply(200, snapshot)
      }
      return reply(404, { error: "not found", code: "not_found" })
    })
  )
  return paths
}

function renderPage(revision: number) {
  return renderWithProviders(
    <RcaRevisionPage observationId={observationId} revision={revision} />,
    { route: { path: "/", url: "/" } }
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("RcaRevisionPage", () => {
  it("shows the stored revision to a signed-in reader through the app's own routes", async () => {
    const paths = serve(200)
    renderPage(1)
    expect(
      await screen.findByRole("heading", { name: "Checkout outage", level: 1 })
    ).toBeTruthy()
    expect(screen.getByText(/^Revision 1, published by Priya, /)).toBeTruthy()
    expect(screen.getByText("Open at publish time")).toBeTruthy()
    expect(screen.getByRole("heading", { name: "Cause" })).toBeTruthy()
    expect(await screen.findByText("No data")).toBeTruthy()
    expect(
      screen
        .getByRole("link", { name: "Back to the observation" })
        .getAttribute("href")
    ).toBe(`/o/${observationId}?rca=1`)
    for (const path of paths)
      expect(path.startsWith("/api/v1/public/")).toBe(false)
  })

  it("points an old revision at the newest one inside the app", async () => {
    serve(200)
    renderPage(1)
    expect(
      await screen.findByText(
        /Revision 2 is newer, so this one may be out of date/
      )
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Open revision 2" }).getAttribute("href")
    ).toBe(`/o/${observationId}/rca/2`)
  })

  it("says the revision is gone after it was withdrawn", async () => {
    serve(404)
    renderPage(1)
    expect(
      await screen.findByRole("heading", { name: "Revision not found" })
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Back to the observation" })
    ).toBeTruthy()
  })

  it("does not ask the server for a revision number that cannot exist", async () => {
    const paths = serve(200)
    renderPage(Number.NaN)
    expect(
      await screen.findByRole("heading", { name: "Revision not found" })
    ).toBeTruthy()
    expect(paths).toEqual([])
  })
})
