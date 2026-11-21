import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"
import type { DataPanel } from "@/spec"

import { testQueryClient } from "@/test/render"

import {
  blockPreviewQuery,
  describeSuggestionError,
  storedBlockPanel,
  suggestionQuery,
} from "./queries"

describe("describeSuggestionError", () => {
  const rows: [number, string, string, string][] = [
    [
      429,
      "too_many_suggestions",
      "you already have 2 suggestions queued or running",
      "You already have 2 suggestions running. Wait for one to finish.",
    ],
    [
      429,
      "agent_busy",
      "the agent is busy; try again in a few minutes",
      "The agent is busy. Try again in a minute.",
    ],
    [
      503,
      "agent_unavailable",
      "no agent is configured",
      "No model is connected to this server.",
    ],
    [
      422,
      "invalid_spec",
      "the draft panel is not valid",
      "Your draft is not valid yet. Fix it before asking the agent.",
    ],
    [
      422,
      "unknown_panel",
      "panel does not exist in the latest version of the dashboard",
      "This panel is not in the version you are editing.",
    ],
    [
      413,
      "too_large",
      "the draft panel is over 65536 bytes as compact JSON",
      "The draft panel is over 65536 bytes as compact JSON.",
    ],
    [
      404,
      "not_found",
      "dashboard version not found",
      "Dashboard version not found.",
    ],
    [
      400,
      "invalid_request",
      "prompt must be 1 to 2000 characters",
      "Prompt must be 1 to 2000 characters.",
    ],
    [
      429,
      "too_many_requests",
      "you already have 3 new dashboard requests queued or running",
      "You already have 3 new dashboard requests queued or running.",
    ],
    [
      429,
      "too_many_revisions",
      "too many revisions are queued or running for this dashboard",
      "Too many revisions are queued or running for this dashboard.",
    ],
    [500, "internal", "internal error", "Internal error."],
    [499, "unknown", "Request failed (499)", "Request failed (499)."],
    [500, "agent_busy", "internal error", "Internal error."],
    [500, "agent_unavailable", "internal error", "Internal error."],
    [500, "invalid_spec", "internal error", "Internal error."],
    [500, "unknown_panel", "internal error", "Internal error."],
    [500, "too_many_suggestions", "internal error", "Internal error."],
  ]

  it.each(rows)("describes %i %s", (status, code, message, expected) => {
    expect(describeSuggestionError(new ApiError(status, code, message))).toBe(
      expected
    )
  })

  it("asks to try again for an error that did not come from the server", () => {
    expect(describeSuggestionError(new NetworkError("offline"))).toBe(
      "The request failed. Try again."
    )
    expect(describeSuggestionError("boom")).toBe(
      "The request failed. Try again."
    )
  })
})

describe("suggestionQuery polling", () => {
  type PollState = {
    status: "pending" | "success" | "error"
    error?: unknown
    data?: { status: string }
  }

  function interval(state: PollState): number | false {
    const poll = suggestionQuery("d", "s").refetchInterval as (query: {
      state: PollState
    }) => number | false
    return poll({ state })
  }

  it("polls every second while the suggestion is unfinished", () => {
    expect(interval({ status: "pending" })).toBe(1000)
    expect(interval({ status: "success", data: { status: "queued" } })).toBe(
      1000
    )
    expect(interval({ status: "success", data: { status: "running" } })).toBe(
      1000
    )
  })

  it("stops once the suggestion has finished", () => {
    for (const status of ["succeeded", "failed"]) {
      expect(interval({ status: "success", data: { status } })).toBe(false)
    }
  })

  it("stops on an error that retrying cannot fix, even with older data", () => {
    const gone = new ApiError(404, "not_found", "agent request not found")
    expect(interval({ status: "error", error: gone })).toBe(false)
    expect(
      interval({ status: "error", error: gone, data: { status: "running" } })
    ).toBe(false)
  })

  it("keeps polling through an error that is worth retrying", () => {
    expect(
      interval({
        status: "error",
        error: new ApiError(503, "agent_unavailable", "no agent"),
      })
    ).toBe(1000)
    expect(
      interval({ status: "error", error: new NetworkError("offline") })
    ).toBe(1000)
  })
})

describe("blockPreviewQuery", () => {
  const panel: DataPanel = {
    id: "panel",
    type: "timeseries",
    title: "Errors",
    datasource: "prom",
    gridPos: { x: 0, y: 0, w: 24, h: 8 },
    queries: [{ query: "up" }],
  }

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  async function sentFor(blockId: string | null) {
    const requests: Request[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        requests.push(request.clone())
        return Response.json({ frames: [] })
      })
    )
    await testQueryClient().fetchQuery(
      blockPreviewQuery({
        observationId: "o1",
        blockId,
        version: 3,
        panel,
        from: "2026-11-12T08:00:00Z",
        to: "2026-11-12T09:00:00Z",
        timezone: undefined,
        selections: { env: ["prod"] },
        maxDataPoints: 400,
      })
    )
    const [request] = requests
    return {
      path: new URL(request.url).pathname,
      body: (await request.json()) as Record<string, unknown>,
    }
  }

  it("posts the draft with the block it edits to the observation", async () => {
    const { path, body } = await sentFor("b1")
    expect(path).toBe("/api/v1/observations/o1/blocks/preview")
    expect(body).toEqual({
      block_id: "b1",
      panel: storedBlockPanel(panel),
      from: "2026-11-12T08:00:00Z",
      to: "2026-11-12T09:00:00Z",
      max_data_points: 400,
      variables: { env: ["prod"] },
    })
  })

  it("names no block for a new query", async () => {
    const { body } = await sentFor(null)
    expect(body).not.toHaveProperty("block_id")
  })
})

describe("storedBlockPanel", () => {
  it("drops the editor's id and grid position", () => {
    expect(
      storedBlockPanel({
        id: "panel",
        type: "stat",
        title: "Up",
        datasource: "prom",
        gridPos: { x: 0, y: 0, w: 24, h: 8 },
        queries: [{ query: "up" }],
      })
    ).toEqual({
      type: "stat",
      title: "Up",
      datasource: "prom",
      queries: [{ query: "up" }],
    })
  })
})
