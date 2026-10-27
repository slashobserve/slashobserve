import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import type { Panel } from "@/spec"

import { maxDataPointsFor, panelRetry } from "./use-panel-data"

const error = (status: number, code: string) =>
  new ApiError(status, code, "failed")

describe("panelRetry", () => {
  it("never retries a query timeout", () => {
    expect(panelRetry(0, error(504, "query_timeout"))).toBe(false)
  })

  it("retries an unavailable datasource once", () => {
    const unavailable = error(502, "datasource_unavailable")
    expect(panelRetry(0, unavailable)).toBe(true)
    expect(panelRetry(1, unavailable)).toBe(false)
  })

  it("stops other retryable errors at three retries", () => {
    const busy = error(503, "busy")
    expect(panelRetry(2, busy)).toBe(true)
    expect(panelRetry(3, busy)).toBe(false)
  })

  it("never retries a client error", () => {
    expect(panelRetry(0, error(404, "not_found"))).toBe(false)
  })
})

const panelOfType = (type: string) => ({ id: "p", type }) as Panel

describe("maxDataPointsFor", () => {
  it.each([
    ["timeseries", 640, 700],
    ["heatmap", 640, 700],
    ["state-timeline", 640, 700],
    ["histogram", 640, 700],
    ["stat", 640, 700],
    ["gauge", 640, 700],
    ["bargauge", 640, 700],
    ["piechart", 640, 700],
    ["timeseries", 100, 100],
    ["timeseries", 101, 200],
    ["timeseries", 0, 100],
    ["timeseries", 10_000, 1100],
    ["barchart", 640, 700],
    ["status-history", 640, 700],
    ["trend", 640, 700],
    ["xychart", 640, 700],
    ["candlestick", 640, 700],
    ["canvas", 640, 700],
    ["table", 640, undefined],
    ["nodeGraph", 640, undefined],
    ["traces", 640, undefined],
    ["flamegraph", 640, undefined],
    ["geomap", 640, undefined],
    ["alertlist", 640, undefined],
    ["dashlist", 640, undefined],
    ["logs", 640, undefined],
    ["text", 640, undefined],
    ["row", 640, undefined],
  ])("%s at %i pixels asks for %s points", (type, width, expected) => {
    expect(maxDataPointsFor(panelOfType(type), width)).toBe(expected)
  })
})
