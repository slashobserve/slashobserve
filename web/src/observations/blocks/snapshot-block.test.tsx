import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { observationKeys } from "@/api/keys"
import type { Block } from "@/observations/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { SnapshotBlock } from "./snapshot-block"

const snapshotId = "5c1f2d9e-0b53-4c43-9d3a-0e6b8f4c7a21"

const agent = {
  kind: "user",
  id: "u1",
  name: "Priya",
  origin: "web",
  client: null,
  run_id: "r1",
  label: "Observe Agent for Priya",
}

function snapshotBlock(body: Record<string, unknown>): Block {
  return {
    id: "b1",
    kind: "snapshot",
    body,
    created_by: agent,
  } as unknown as Block
}

const timeFrom = new Date(2026, 10, 6, 10, 0).toISOString()
const timeTo = new Date(2026, 10, 6, 11, 30).toISOString()

function frozen(frames: unknown[]) {
  return {
    id: snapshotId,
    observation_id: "o1",
    panel: { type: "timeseries", title: "Error rate", datasource: "prom-prod" },
    query: [{ query: "up" }],
    data: { frames, meta: {} },
    user_scoped: false,
    time_from: timeFrom,
    time_to: timeTo,
    created_by: agent,
    created_at: timeTo,
  }
}

function renderSnapshot(block: Block, seeded?: unknown) {
  const queryClient = testQueryClient()
  if (seeded !== undefined) {
    queryClient.setQueryData(observationKeys.snapshot(snapshotId), seeded)
  }
  return renderWithProviders(<SnapshotBlock block={block} height={240} />, {
    queryClient,
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("SnapshotBlock", () => {
  it("shows the frozen range and credits Observe Agent without asking for new data", async () => {
    const fetch = vi.fn()
    vi.stubGlobal("fetch", fetch)
    renderSnapshot(snapshotBlock({ snapshot_id: snapshotId }), frozen([]))

    expect(
      await screen.findByText("Frozen, 2026-11-06 10:00 to 2026-11-06 11:30")
    ).toBeTruthy()
    expect(screen.getByText("From Observe Agent")).toBeTruthy()
    expect(screen.getByText("No data")).toBeTruthy()
    expect(fetch).not.toHaveBeenCalled()
  })

  it("reads the snapshot from the snapshot route", async () => {
    const fetch = vi.fn<(request: Request) => Promise<Response>>(
      async () =>
        new Response(JSON.stringify(frozen([])), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
    )
    vi.stubGlobal("fetch", fetch)
    renderSnapshot(snapshotBlock({ snapshot_id: snapshotId }))

    expect(await screen.findByText(/^Frozen, /)).toBeTruthy()
    expect(new URL(fetch.mock.calls[0][0].url).pathname).toBe(
      `/api/v1/snapshots/${snapshotId}`
    )
  })

  it("says the block could not be read when it names no snapshot", () => {
    renderSnapshot(snapshotBlock({}))
    expect(screen.getByText("This block could not be read.")).toBeTruthy()
  })
})
