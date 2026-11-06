import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"

import {
  clickhouseContextQueries,
  contextFrames,
  lokiAfterQuery,
  lokiBeforeQuery,
  lokiStreamSelector,
  nextAfterStep,
} from "./context-query"
import { rowCount, rowKey } from "./frames"

describe("lokiStreamSelector", () => {
  const labels = {
    service_name: "checkout",
    namespace: "shop",
    trace_id: "4bf92f3577b34da6a3ce929d0e0e4736",
    detected_level: "info",
  }

  it("keeps only the index labels when they are known", () => {
    expect(lokiStreamSelector(labels, ["service_name", "job"])).toBe(
      '{service_name="checkout"}'
    )
  })

  it("drops the per-line labels without an index list", () => {
    expect(lokiStreamSelector(labels, undefined)).toBe(
      '{namespace="shop", service_name="checkout"}'
    )
    expect(lokiStreamSelector(labels, [])).toBe(
      '{namespace="shop", service_name="checkout"}'
    )
  })

  it("escapes quotes and backslashes", () => {
    expect(lokiStreamSelector({ path: 'C:\\logs "a"' }, undefined)).toBe(
      '{path="C:\\\\logs \\"a\\""}'
    )
  })

  it("gives null when no label is left", () => {
    expect(lokiStreamSelector({ trace_id: "abc" }, undefined)).toBeNull()
    expect(lokiStreamSelector(labels, ["pod"])).toBeNull()
  })
})

describe("Loki context queries", () => {
  it("reads the 51 newest lines of the hour before the line", () => {
    expect(lokiBeforeQuery('{a="b"}', 1_760_000_000_000)).toEqual({
      query: '{a="b"}',
      kind: "instant",
      from: "1759996400000",
      to: "1760000000001",
      limit: 51,
    })
  })

  it("widens the window after the line step by step", () => {
    expect(lokiAfterQuery('{a="b"}', 1_760_000_000_000, 0)).toEqual({
      query: '{a="b"}',
      kind: "instant",
      from: "1760000000000",
      to: "1760000010000",
      limit: 1000,
    })
    expect(lokiAfterQuery('{a="b"}', 1_760_000_000_000, 3).to).toBe(
      "1760003600000"
    )
  })

  it.each([
    ["fewer than 50 lines after", 0, 10, false, 1],
    ["50 lines after", 0, 50, false, null],
    ["a saturated result", 0, 10, true, null],
    ["the last step", 3, 10, false, null],
  ])("for %s gives the next step %s", (_, step, after, saturated, next) => {
    expect(nextAfterStep(step, after, saturated)).toBe(next)
  })
})

describe("clickhouseContextQueries", () => {
  it("builds the queries before and after the line in the same service", () => {
    expect(clickhouseContextQueries("check'out", 1_760_000_000_000)).toEqual({
      before: {
        query: `SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes
FROM otel_logs
WHERE $__timeFilter(Timestamp) AND ServiceName = 'check\\'out'
ORDER BY Timestamp DESC
LIMIT 51`,
        kind: "instant",
        from: "1759996400000",
        to: "1760000000001",
      },
      after: {
        query: `SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes
FROM otel_logs
WHERE $__timeFilter(Timestamp) AND ServiceName = 'check\\'out'
ORDER BY Timestamp ASC
LIMIT 51`,
        kind: "instant",
        from: "1760000000000",
        to: "1760003600000",
      },
    })
  })
})

function frameOf(times: number[]): WireFrame {
  return {
    ref: "A",
    labels: { service_name: "checkout" },
    fields: [
      { name: "Time", type: "time", values: times },
      { name: "Body", type: "string", values: times.map((t) => `line ${t}`) },
    ],
  }
}

function keptTimes(frames: WireFrame[]): number[] {
  return frames
    .flatMap((frame) => frame.fields[0].values as number[])
    .sort((a, b) => a - b)
}

describe("contextFrames", () => {
  it("keeps 50 lines before, the line and 50 after out of 300", () => {
    const times = Array.from({ length: 300 }, (_, i) => 1_000 + i)
    const frame = frameOf(times)
    const kept = contextFrames([frame], {
      time: 1_150,
      key: rowKey(frame, 150),
    })
    expect(rowCount(kept)).toBe(101)
    expect(keptTimes(kept)[0]).toBe(1_100)
    expect(keptTimes(kept)[100]).toBe(1_200)
  })

  it("keeps what there is when one side has fewer than 50", () => {
    const frame = frameOf(Array.from({ length: 100 }, (_, i) => 1_000 + i))
    const kept = contextFrames([frame], { time: 1_010, key: null })
    expect(keptTimes(kept)[0]).toBe(1_000)
    expect(rowCount(kept)).toBe(61)
  })

  it("drops a row both queries returned", () => {
    const before = frameOf([1, 2, 3])
    const after = frameOf([3, 4])
    const kept = contextFrames([before, after], {
      time: 3,
      key: rowKey(before, 2),
    })
    expect(keptTimes(kept)).toEqual([1, 2, 3, 4])
  })

  it("orders lines at the anchor's millisecond by position when the key differs", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [
        { name: "Time", type: "time", values: [5, 5, 5, 6] },
        { name: "Body", type: "string", values: ["a", "b", "c", "d"] },
      ],
    }
    const kept = contextFrames([frame], { time: 5, key: rowKey(frame, 1) }, 1)
    expect(kept[0].fields[1].values).toEqual(["a", "b", "c"])
  })
})
