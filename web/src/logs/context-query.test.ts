import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { LogDetail, LogEntry } from "@/panels/logs/logs-model"
import type { PreparedFrame } from "@/panels/transforms"

import {
  anchorFrames,
  anchorMatcher,
  anchorNs,
  clickhouseLogsTemplate,
  contextFrames,
  contextQueryFor,
  dateFieldNote,
  lokiStreamSelector,
  moreLines,
  type LogContext,
} from "./context-query"

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

function entryOf(
  details: LogDetail[],
  extra: Partial<LogEntry> = {}
): LogEntry {
  return {
    key: "k",
    time: null,
    level: null,
    message: "line",
    labels: {},
    repeats: 1,
    details,
    ...extra,
  }
}

function texts(pairs: [string, string][]): LogDetail[] {
  return pairs.map(([name, text]) => ({ name, text }))
}

describe("contextQueryFor", () => {
  const labels = { service_name: "checkout", trace_id: "abc" }

  it("gives the stream selector on Loki and nothing without a stream label", () => {
    expect(contextQueryFor("loki", entryOf([]), labels, undefined)).toBe(
      '{service_name="checkout"}'
    )
    expect(
      contextQueryFor("loki", entryOf([]), { trace_id: "abc" }, undefined)
    ).toBeNull()
  })

  it("narrows the ClickHouse template to the line's service before the ORDER BY", () => {
    const entry = entryOf(texts([["ServiceName", "check'out"]]))
    expect(contextQueryFor("clickhouse", entry, {}, undefined)).toBe(
      `SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes
FROM otel_logs
WHERE $__timeFilter(Timestamp) AND ServiceName = 'check\\'out'
ORDER BY Timestamp DESC
LIMIT 1000`
    )
  })

  it("keeps the ClickHouse template when the line has no service", () => {
    expect(contextQueryFor("clickhouse", entryOf([]), {}, undefined)).toBe(
      clickhouseLogsTemplate
    )
  })

  it.each(["elasticsearch", "opensearch"])(
    "matches every document on %s",
    (type) => {
      expect(contextQueryFor(type, entryOf([]), {}, undefined)).toBe("*")
    }
  )

  it("gives null for a datasource type with no lines", () => {
    expect(
      contextQueryFor("prometheus", entryOf([]), labels, undefined)
    ).toBeNull()
  })
})

describe("anchorNs", () => {
  it("reads the TimeNs detail", () => {
    const entry = entryOf(texts([["TimeNs", "1760000000123456789"]]), {
      time: 1_760_000_000_123,
    })
    expect(anchorNs(entry)).toBe("1760000000123456789")
  })

  it("falls back to the millisecond time", () => {
    expect(anchorNs(entryOf([], { time: 1_760_000_000_123 }))).toBe(
      "1760000000123000000"
    )
  })

  it("is null with neither", () => {
    expect(anchorNs(entryOf([]))).toBeNull()
    expect(anchorNs(entryOf(texts([["TimeNs", " "]])))).toBeNull()
  })
})

describe("anchorMatcher", () => {
  const anchor = entryOf(texts([["TimeNs", "100"]]), { message: "boom" })

  it.each([
    ["the same line", "100", "boom", true],
    ["another nanosecond", "101", "boom", false],
    ["another message", "100", "other", false],
  ])("for %s says %s", (_, ns, message, expected) => {
    const candidate = entryOf(texts([["TimeNs", ns]]), { message })
    expect(anchorMatcher(anchor)(candidate)).toBe(expected)
  })
})

function frameOf(ns: string[]): PreparedFrame {
  return {
    ref: "A",
    labels: { service_name: "checkout" },
    fields: [
      { name: "Time", type: "time", values: ns.map((n) => Number(n) / 1e6) },
      { name: "TimeNs", type: "string", values: ns },
      { name: "Body", type: "string", values: ns.map((n) => `line ${n}`) },
    ],
  }
}

function bodies(frames: readonly WireFrame[]): unknown[] {
  return frames.flatMap(
    (frame) => frame.fields.find((field) => field.name === "Body")?.values ?? []
  )
}

describe("anchorFrames", () => {
  it("keeps only the selected row of the frame it came from", () => {
    const frame = frameOf(["3000000", "2000000", "1000000"])
    const entry = entryOf(
      [{ name: "Body", field: frame.fields[2], frame, index: 1 }],
      { time: 2 }
    )
    const kept = anchorFrames(entry)
    expect(kept).toHaveLength(1)
    expect(bodies(kept)).toEqual(["line 2000000"])
    expect(kept[0].labels).toEqual({ service_name: "checkout" })
  })

  it("gives no frame for an entry made of text details", () => {
    expect(anchorFrames(entryOf(texts([["a", "b"]])))).toEqual([])
  })
})

describe("contextFrames", () => {
  it("lists the older lines oldest first, then the line, then the newer lines", () => {
    const context: LogContext = {
      anchor: "5000000",
      before: {
        frames: [frameOf(["4000000", "3000000"]), frameOf(["2500000"])],
        truncated: false,
      },
      after: { frames: [frameOf(["6000000", "7000000"])], truncated: true },
    }
    const frames = contextFrames(context, [frameOf(["5000000"])])
    expect(bodies(frames)).toEqual([
      "line 2500000",
      "line 3000000",
      "line 4000000",
      "line 5000000",
      "line 6000000",
      "line 7000000",
    ])
  })

  it("does not change the response it was given", () => {
    const before = frameOf(["2000000", "1000000"])
    contextFrames(
      {
        anchor: "3000000",
        before: { frames: [before], truncated: false },
        after: { frames: [], truncated: false },
      },
      []
    )
    expect(bodies([before])).toEqual(["line 2000000", "line 1000000"])
  })
})

describe("moreLines", () => {
  it.each([
    [50, 60],
    [490, 500],
    [495, 500],
    [500, 500],
  ])("takes %s lines to %s", (count, next) => {
    expect(moreLines(count)).toBe(next)
  })
})

describe("dateFieldNote", () => {
  const whole = frameOf(["1760000000123000000"])
  const fine = frameOf(["1760000000123456789"])

  it.each([
    ["elasticsearch", [whole], true],
    ["opensearch", [fine, whole], true],
    ["elasticsearch", [fine], false],
    ["elasticsearch", [], false],
    ["loki", [whole], false],
    ["clickhouse", [whole], false],
  ])("on %s with these lines says %s", (type, frames, expected) => {
    expect(dateFieldNote(type, frames)).toBe(expected)
  })
})
