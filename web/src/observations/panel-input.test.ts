import { describe, expect, it } from "vitest"

import type { DataPanel } from "@/spec"

import { blockPanelFrom, blockRange, panelUsesVariables } from "./panel-input"

function panel(overrides: Record<string, unknown> = {}): DataPanel {
  return {
    id: "latency",
    type: "timeseries",
    title: "Latency",
    datasource: "prom",
    gridPos: { x: 4, y: 9, w: 12, h: 6 },
    queries: [{ query: "rate(http_requests_total[5m])" }],
    ...overrides,
  } as DataPanel
}

describe("panelUsesVariables", () => {
  it.each([
    ["a $name reference", 'up{service="$service"}'],
    ["a ${name} reference", 'up{service="${service}"}'],
    ["a ${name:format} reference", 'up{service="${service:regex}"}'],
    ["an underscored name", "up{job=~$_job}"],
  ])("is true for %s in a query", (_, query) => {
    expect(panelUsesVariables(panel({ queries: [{ query }] }))).toBe(true)
  })

  it("is true when a later query of the panel references a variable", () => {
    const queries = [{ query: "up" }, { query: "up{job=$job}" }]
    expect(panelUsesVariables(panel({ queries }))).toBe(true)
  })

  it("is true when the panel's datasource names a variable", () => {
    expect(panelUsesVariables(panel({ datasource: "${ds}" }))).toBe(true)
  })

  it.each([
    ["a plain query", "sum(rate(http_requests_total[5m]))"],
    ["a dollar sign before a digit", "up{price=~'$5'}"],
    ["a lone dollar sign", "up{x='$'}"],
  ])("is false for %s", (_, query) => {
    expect(panelUsesVariables(panel({ queries: [{ query }] }))).toBe(false)
  })

  it("ignores variable references outside the queries and the datasource", () => {
    expect(panelUsesVariables(panel({ title: "$service latency" }))).toBe(false)
  })
})

describe("blockRange", () => {
  const now = Date.parse("2026-11-12T12:00:00Z")

  it("turns a relative range into absolute times", () => {
    expect(blockRange("now-6h", "now", now)).toEqual({
      from: "2026-11-12T06:00:00.000Z",
      to: "2026-11-12T12:00:00.000Z",
    })
  })

  it("keeps an absolute range", () => {
    expect(blockRange("1762912800000", "1762916400000", now)).toEqual({
      from: "2025-11-12T02:00:00.000Z",
      to: "2025-11-12T03:00:00.000Z",
    })
  })

  it.each([
    ["no range", undefined, undefined],
    ["only from", "now-1h", undefined],
    ["an unreadable bound", "yesterday", "now"],
    ["a range that ends before it starts", "now", "now-1h"],
  ])("gives no range for %s", (_, from, to) => {
    expect(blockRange(from, to, now)).toEqual({ from: null, to: null })
  })
})

describe("blockPanelFrom", () => {
  it("drops the repeat key and keeps the rest", () => {
    const source = panel({ repeat: "service" })
    const copy = blockPanelFrom(source)
    expect("repeat" in copy).toBe(false)
    expect(copy.queries).toEqual(source.queries)
    expect(copy.title).toBe("Latency")
  })

  it("does not change the panel it was given", () => {
    const source = panel({ repeat: "service" })
    blockPanelFrom(source)
    expect(source.repeat).toBe("service")
  })
})
