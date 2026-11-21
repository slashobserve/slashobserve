import { describe, expect, it } from "vitest"

import { describeRange, liveNowRefresh, refreshToMs } from "@/lib/time"
import type { Block } from "@/observations/queries"
import type { DataPanel } from "@/spec"

import {
  datasourceUidOf,
  frozenCaption,
  pageRangeFor,
  panelBlockBody,
  pinnedBody,
  resolveBlockRange,
  runBlockBody,
  snapshotIdOf,
} from "./block-data"

function blockWith(body: Record<string, unknown>): Block {
  return { id: "b1", body } as unknown as Block
}

describe("panelBlockBody", () => {
  it("reads the panel and the range and names the panel block", () => {
    const body = panelBlockBody(
      blockWith({
        panel: { type: "timeseries", title: "Error rate", queries: [] },
        from: "2026-11-06T10:00:00Z",
        to: "2026-11-06T11:00:00Z",
      })
    )
    expect(body).toEqual({
      panel: {
        id: "block",
        type: "timeseries",
        title: "Error rate",
        queries: [],
      },
      from: "2026-11-06T10:00:00Z",
      to: "2026-11-06T11:00:00Z",
    })
  })

  it("leaves the range out when the block has none", () => {
    const body = panelBlockBody(blockWith({ panel: { type: "stat" } }))
    expect(body).not.toBeNull()
    expect(body).not.toHaveProperty("from")
    expect(body).not.toHaveProperty("to")
  })

  it.each([
    ["no panel", {}],
    ["a panel that is not an object", { panel: "stat" }],
    ["a panel without a type", { panel: { title: "x" } }],
    ["a panel with a type that is not a string", { panel: { type: 3 } }],
    ["a from that is not a string", { panel: { type: "stat" }, from: 5 }],
    ["a to that is null", { panel: { type: "stat" }, to: null }],
  ])("rejects %s", (_name, body) => {
    expect(panelBlockBody(blockWith(body))).toBeNull()
  })
})

describe("snapshotIdOf", () => {
  it("reads the snapshot id", () => {
    expect(snapshotIdOf(blockWith({ snapshot_id: "s1" }))).toBe("s1")
  })

  it.each([
    ["missing", {}],
    ["a number", { snapshot_id: 1 }],
    ["empty", { snapshot_id: "" }],
  ])("rejects an id that is %s", (_name, body) => {
    expect(snapshotIdOf(blockWith(body))).toBeNull()
  })
})

describe("runBlockBody", () => {
  it("reads the run id and its cites", () => {
    expect(
      runBlockBody(blockWith({ run_id: "r1", cites: ["a", "b"] }))
    ).toEqual({ runId: "r1", cites: ["a", "b"] })
  })

  it("accepts a run that cites nothing yet", () => {
    expect(runBlockBody(blockWith({ run_id: "r1", cites: [] }))).toEqual({
      runId: "r1",
      cites: [],
    })
  })

  it.each([
    ["no run id", { cites: [] }],
    ["a null run id", { run_id: null, cites: [] }],
    ["a run id that is not a string", { run_id: 7, cites: [] }],
    ["no cites", { run_id: "r1" }],
    ["cites that are not an array", { run_id: "r1", cites: "a" }],
    ["a cite that is not a string", { run_id: "r1", cites: ["a", 2] }],
  ])("rejects %s", (_name, body) => {
    expect(runBlockBody(blockWith(body))).toBeNull()
  })
})

describe("datasourceUidOf", () => {
  function panel(extra: Record<string, unknown>): DataPanel {
    return { type: "timeseries", queries: [], ...extra } as unknown as DataPanel
  }

  it("prefers the panel's datasource", () => {
    expect(
      datasourceUidOf(
        panel({ datasource: "prom-prod", queries: [{ query: "up" }] })
      )
    ).toBe("prom-prod")
  })

  it("falls back to the first query that names one", () => {
    expect(
      datasourceUidOf(
        panel({
          queries: [
            { expression: {} },
            { query: "up" },
            { query: "up", datasource: "loki" },
          ],
        })
      )
    ).toBe("loki")
  })

  it("is undefined when nothing names a datasource", () => {
    expect(datasourceUidOf(panel({ queries: [{ query: "up" }] }))).toBe(
      undefined
    )
  })
})

describe("frozenCaption", () => {
  it("names the frozen range", () => {
    const from = new Date(2026, 10, 6, 10, 0).toISOString()
    const to = new Date(2026, 10, 6, 11, 30).toISOString()
    expect(frozenCaption(from, to)).toMatch(
      /^Frozen, 2026-11-06 10:00 to 2026-11-06 11:30 \S+$/
    )
  })
})

describe("resolveBlockRange", () => {
  const pageFrom = "2026-11-20T23:56:00Z"
  const pageTo = "2026-11-21T00:56:00Z"
  const pinFrom = "2026-11-20T10:00:00Z"
  const pinTo = "2026-11-20T11:00:00Z"

  it("shows the last hour live, refreshing as a live dashboard does, when neither the page nor the block has a range", () => {
    expect(resolveBlockRange({ from: null, to: null }, {})).toEqual({
      from: "now-1h",
      to: "now",
      pinned: false,
      caption: "Last hour, live",
      refreshMs: refreshToMs(liveNowRefresh),
    })
  })

  it("follows the page's range when the block has none", () => {
    expect(resolveBlockRange({ from: pageFrom, to: pageTo }, {})).toEqual({
      from: pageFrom,
      to: pageTo,
      pinned: false,
      caption: describeRange(pageFrom, pageTo),
      refreshMs: null,
    })
  })

  it("keeps a pinned block on its own window and says so", () => {
    expect(
      resolveBlockRange(
        { from: pageFrom, to: pageTo },
        { from: pinFrom, to: pinTo }
      )
    ).toEqual({
      from: pinFrom,
      to: pinTo,
      pinned: true,
      caption: `Pinned to ${describeRange(pinFrom, pinTo)}`,
      refreshMs: null,
    })
    expect(
      resolveBlockRange({ from: null, to: null }, { from: pinFrom, to: pinTo })
        .pinned
    ).toBe(true)
  })

  it("treats half a range on either side as no range", () => {
    expect(
      resolveBlockRange({ from: pageFrom, to: null }, { from: pinFrom })
    ).toMatchObject({ from: "now-1h", to: "now", pinned: false })
  })
})

describe("pageRangeFor", () => {
  const now = Date.parse("2026-11-20T19:26:00Z")

  it("clears the page's range for the last hour, so the page stays live", () => {
    expect(pageRangeFor({ from: "now-1h", to: "now" }, now)).toEqual({
      from: null,
      to: null,
    })
  })

  it("stores any other range as the absolute window it means now", () => {
    expect(pageRangeFor({ from: "now-6h", to: "now" }, now)).toEqual({
      from: "2026-11-20T13:26:00.000Z",
      to: "2026-11-20T19:26:00.000Z",
    })
  })
})

describe("pinnedBody", () => {
  const now = Date.parse("2026-11-20T19:26:00Z")
  const body = { panel: { type: "timeseries" }, variables: [] }

  it("keeps the panel and pins it to the absolute window", () => {
    expect(pinnedBody(body, "now-1h", "now", now)).toEqual({
      ...body,
      from: "2026-11-20T18:26:00.000Z",
      to: "2026-11-20T19:26:00.000Z",
    })
  })

  it("reads a dragged range in epoch milliseconds", () => {
    expect(
      pinnedBody(
        body,
        String(Date.parse("2026-11-20T10:00:00Z")),
        String(Date.parse("2026-11-20T10:30:00Z")),
        now
      )
    ).toEqual({
      ...body,
      from: "2026-11-20T10:00:00.000Z",
      to: "2026-11-20T10:30:00.000Z",
    })
  })

  it("pins nothing when the window ends before it starts", () => {
    expect(pinnedBody(body, "now", "now-1h", now)).toBeNull()
  })
})
