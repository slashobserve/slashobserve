import { describe, expect, it } from "vitest"

import {
  attributeKey,
  compareBody,
  compareDescription,
  compareNote,
  compareRows,
  percentText,
  shareText,
  type TraceCompare,
  type TraceDifference,
} from "./compare"
import type { TraceRow } from "./trace-rows"

function row(overrides: Partial<TraceRow> & { traceId: string }): TraceRow {
  return {
    start: null,
    service: "",
    span: "",
    durationMs: null,
    spans: null,
    errors: null,
    ...overrides,
  }
}

function difference(
  attribute: string,
  value: string,
  selected: number,
  baseline: number
): TraceDifference {
  return {
    attribute,
    value,
    selected_share: selected,
    baseline_share: baseline,
    selected_count: 0,
    baseline_count: 0,
  }
}

function result(overrides: Partial<TraceCompare> = {}): TraceCompare {
  return {
    selected: { requested: 48, read: 48 },
    baseline: { requested: 150, read: 150 },
    attributes: 31,
    partial: false,
    differences: [],
    truncated: false,
    ...overrides,
  }
}

describe("compareBody", () => {
  const range = { from: "now-1h", to: "now" }
  const rows = Array.from({ length: 300 }, (_, index) =>
    row({
      traceId: `t${index}`,
      start: 1_000_000 + index * 1000,
      durationMs: index < 60 ? 2000 : 100,
    })
  )
  const selection = { from: 0, to: 2_000_000, minMs: 1000, maxMs: null }

  it("sends the newest 50 selected and the newest 150 other trace ids", () => {
    const body = compareBody(rows, selection, range)
    expect(body.selected).toHaveLength(50)
    expect(body.selected[0]).toBe("t59")
    expect(body.selected[49]).toBe("t10")
    expect(body.baseline).toHaveLength(150)
    expect(body.baseline[0]).toBe("t299")
    expect(body.baseline[149]).toBe("t150")
    expect(body.selected.some((id) => body.baseline.includes(id))).toBe(false)
    expect(body.from).toBe("now-1h")
    expect(body.to).toBe("now")
  })

  it("leaves rows without a start at the end of the baseline", () => {
    const body = compareBody(
      [
        row({ traceId: "unknown", durationMs: 100 }),
        row({ traceId: "old", start: 1, durationMs: 100 }),
        row({ traceId: "picked", start: 2, durationMs: 2000 }),
      ],
      selection,
      range
    )
    expect(body.selected).toEqual(["picked"])
    expect(body.baseline).toEqual(["old", "unknown"])
  })
})

describe("compareRows", () => {
  it("splits the differences at a gap of 20 points", () => {
    const strong = difference("span.a", "1", 0.92, 0.11)
    const edge = difference("span.b", "2", 0.3, 0.1)
    const weak = difference("span.c", "3", 0.29, 0.1)
    const negative = difference("span.d", "4", 0.1, 0.4)
    expect(
      compareRows(result({ differences: [strong, edge, weak, negative] }))
    ).toEqual({ strong: [strong, edge, negative], weak: [weak] })
  })
})

describe("share texts", () => {
  it.each([
    [0.92, "92%"],
    [0.005, "1%"],
    [0, "0%"],
    [1, "100%"],
  ])("rounds %s to %s", (share, text) => {
    expect(percentText(share)).toBe(text)
  })

  it("reads both shares for a screen reader", () => {
    expect(
      shareText(difference("span.payments.provider", "acquirer-b", 0.92, 0.11))
    ).toBe("92 percent of selected traces, 11 percent of the rest")
  })

  it.each([
    ["span.payments.provider", "payments.provider"],
    ["resource.k8s.pod.name", "k8s.pod.name"],
    ["name", "name"],
    ["span.resource.x", "resource.x"],
  ])("strips the scope from %s", (attribute, key) => {
    expect(attributeKey(attribute)).toBe(key)
  })
})

describe("compareDescription", () => {
  it("counts the traces that were read in each set", () => {
    expect(
      compareDescription(
        result({
          selected: { requested: 50, read: 48 },
          baseline: { requested: 150, read: 1214 },
        })
      )
    ).toBe(
      "Attribute values that appear far more often in the 48 selected traces than in the other 1,214. Ranked by the size of the gap."
    )
  })
})

describe("compareNote", () => {
  it("says how many attributes were compared", () => {
    expect(compareNote(result(), 48)).toBe(
      "Compared 31 attributes across the spans of each trace. Attributes with one value everywhere are left out."
    )
  })

  it("says when only the newest selected traces were sent", () => {
    expect(
      compareNote(result({ selected: { requested: 50, read: 50 } }), 60)
    ).toBe(
      "Compared 31 attributes across the spans of each trace. Attributes with one value everywhere are left out. Compared the newest 50 of the 60 selected traces."
    )
  })

  it("counts the traces not read before the time limit", () => {
    expect(
      compareNote(
        result({
          selected: { requested: 48, read: 45 },
          baseline: { requested: 150, read: 140 },
          partial: true,
        }),
        48
      )
    ).toBe(
      "Compared 31 attributes across the spans of each trace. Attributes with one value everywhere are left out. 13 traces were not read before the time limit; the shares are from the rest."
    )
  })
})
