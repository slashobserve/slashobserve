import { describe, expect, it } from "vitest"

import {
  durationBoundText,
  durationScale,
  durationTicks,
  durationTickText,
  nudgeSelection,
  scatterPoints,
  selectedRows,
  selectionFromDrag,
  selectionText,
  windowTicks,
  type Selection,
} from "./scatter"
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

describe("durationScale", () => {
  it("puts 1 ms at the bottom and the decade above the slowest trace at the top", () => {
    const scale = durationScale(2300)
    expect(scale.minMs).toBe(1)
    expect(scale.maxMs).toBe(10_000)
    expect(scale.y(1)).toBe(0)
    expect(scale.y(10_000)).toBe(1)
    expect(scale.y(100)).toBeCloseTo(0.5)
  })

  it("keeps a trace that is exactly a decade below the top edge", () => {
    expect(durationScale(1000).maxMs).toBe(10_000)
  })

  it("clamps durations below a millisecond and above the top", () => {
    const scale = durationScale(5)
    expect(scale.maxMs).toBe(10)
    expect(scale.y(0.2)).toBe(0)
    expect(scale.y(50)).toBe(1)
  })

  it("inverts a fraction back to the duration it was drawn at", () => {
    const scale = durationScale(2300)
    expect(scale.ms(0)).toBe(1)
    expect(scale.ms(1)).toBe(10_000)
    expect(scale.ms(0.5)).toBeCloseTo(100)
    for (const ms of [1, 7, 800, 2300, 10_000]) {
      expect(scale.ms(scale.y(ms))).toBeCloseTo(ms)
    }
    expect(scale.ms(-1)).toBe(1)
    expect(scale.ms(2)).toBe(10_000)
  })
})

describe("durationTicks", () => {
  it("lists the decades inside the scale", () => {
    expect(durationTicks(2300)).toEqual([10, 100, 1000, 10_000])
    expect(durationTicks(5)).toEqual([10])
  })

  it("keeps at most four ticks, always including the top", () => {
    expect(durationTicks(500_000)).toEqual([100, 10_000, 1_000_000])
  })
})

describe("durationTickText", () => {
  it("reads in milliseconds below a second and in seconds above", () => {
    expect(durationTickText(10)).toBe("10 ms")
    expect(durationTickText(1000)).toBe("1 s")
    expect(durationTickText(10_000)).toBe("10 s")
  })
})

describe("scatterPoints", () => {
  const range = { start: 1000, end: 3000 }

  it("maps start across the range and duration through the log scale", () => {
    const points = scatterPoints(
      [
        row({ traceId: "a", start: 2000, durationMs: 100, errors: 0 }),
        row({ traceId: "b", start: 3000, durationMs: 10_000, errors: 2 }),
      ],
      range,
      2300
    )
    expect(points).toEqual([
      { traceId: "a", x: 0.5, y: 0.5, error: false },
      { traceId: "b", x: 1, y: 1, error: true },
    ])
  })

  it("drops rows without a start or a duration", () => {
    const points = scatterPoints(
      [
        row({ traceId: "a", start: null, durationMs: 100 }),
        row({ traceId: "b", start: 2000, durationMs: null }),
        row({ traceId: "c", start: 2000, durationMs: 100 }),
      ],
      range,
      100
    )
    expect(points.map((point) => point.traceId)).toEqual(["c"])
  })

  it("keeps a trace outside the range on the edge", () => {
    const [point] = scatterPoints(
      [row({ traceId: "a", start: 500, durationMs: 1 })],
      range,
      1
    )
    expect(point?.x).toBe(0)
  })
})

describe("windowTicks", () => {
  it("picks the finest step that fits the tick budget, aligned to the step", () => {
    const start = Date.UTC(2026, 10, 7, 13, 17)
    const end = Date.UTC(2026, 10, 7, 13, 47)
    expect(windowTicks({ start, end }, 6)).toEqual([
      Date.UTC(2026, 10, 7, 13, 20),
      Date.UTC(2026, 10, 7, 13, 25),
      Date.UTC(2026, 10, 7, 13, 30),
      Date.UTC(2026, 10, 7, 13, 35),
      Date.UTC(2026, 10, 7, 13, 40),
      Date.UTC(2026, 10, 7, 13, 45),
    ])
  })

  it("is empty for an empty range", () => {
    expect(windowTicks({ start: 5, end: 5 }, 6)).toEqual([])
  })
})

describe("selectionFromDrag", () => {
  const plot = { width: 1000, height: 160 }
  const range = { start: 1000, end: 61_000 }
  const scale = durationScale(2300)

  it("selects the full range with no ceiling from bottom-left to top-right", () => {
    expect(
      selectionFromDrag({ x: 0, y: 160 }, { x: 1000, y: 0 }, plot, range, scale)
    ).toEqual({ from: 1000, to: 61_000, minMs: 1, maxMs: null })
  })

  it("orders the corners and keeps a ceiling drawn below the top edge", () => {
    const selection = selectionFromDrag(
      { x: 750, y: 40 },
      { x: 250, y: 120 },
      plot,
      range,
      scale
    )
    expect(selection?.from).toBe(16_000)
    expect(selection?.to).toBe(46_000)
    expect(selection?.minMs).toBeCloseTo(10)
    expect(selection?.maxMs).toBeCloseTo(1000)
  })

  it("snaps the ceiling off within two pixels of the top", () => {
    expect(
      selectionFromDrag({ x: 0, y: 100 }, { x: 100, y: 2 }, plot, range, scale)
        ?.maxMs
    ).toBeNull()
  })

  it.each([
    [
      { x: 10, y: 10 },
      { x: 13, y: 100 },
    ],
    [
      { x: 10, y: 10 },
      { x: 100, y: 13 },
    ],
    [
      { x: 10, y: 10 },
      { x: 10, y: 10 },
    ],
  ])("treats a drag under four pixels as a click", (a, b) => {
    expect(selectionFromDrag(a, b, plot, range, scale)).toBeNull()
  })

  it("clamps corners dragged outside the plot", () => {
    expect(
      selectionFromDrag(
        { x: -50, y: 300 },
        { x: 2000, y: -20 },
        plot,
        range,
        scale
      )
    ).toEqual({ from: 1000, to: 61_000, minMs: 1, maxMs: null })
  })

  it("is null for a plot with no size", () => {
    expect(
      selectionFromDrag(
        { x: 0, y: 0 },
        { x: 50, y: 50 },
        { width: 0, height: 0 },
        range,
        scale
      )
    ).toBeNull()
  })
})

describe("selectedRows", () => {
  const selection: Selection = { from: 2000, to: 3000, minMs: 800, maxMs: 2000 }
  const rows = [
    row({ traceId: "start-edge", start: 2000, durationMs: 800 }),
    row({ traceId: "end-edge", start: 3000, durationMs: 2000 }),
    row({ traceId: "inside", start: 2500, durationMs: 1000 }),
    row({ traceId: "early", start: 1999, durationMs: 1000 }),
    row({ traceId: "late", start: 3001, durationMs: 1000 }),
    row({ traceId: "fast", start: 2500, durationMs: 799 }),
    row({ traceId: "slow", start: 2500, durationMs: 2001 }),
    row({ traceId: "no-start", start: null, durationMs: 1000 }),
    row({ traceId: "no-duration", start: 2500, durationMs: null }),
  ]

  it("keeps rows inside the time and duration bounds, edges included", () => {
    expect(selectedRows(rows, selection).map((r) => r.traceId)).toEqual([
      "start-edge",
      "end-edge",
      "inside",
    ])
  })

  it("has no ceiling when maxMs is null", () => {
    expect(
      selectedRows(rows, { ...selection, maxMs: null }).map((r) => r.traceId)
    ).toEqual(["start-edge", "end-edge", "inside", "slow"])
  })
})

describe("selectionText", () => {
  const from = Date.UTC(2026, 10, 6, 14, 13)
  const to = Date.UTC(2026, 10, 6, 14, 17)

  it("reads slower than the floor when there is no ceiling", () => {
    expect(
      selectionText({ from, to, minMs: 800, maxMs: null }, 48, "UTC")
    ).toBe(
      "48 traces selected: 2026-11-06 14:13:00 to 2026-11-06 14:17:00, slower than 800 ms"
    )
  })

  it("reads between the floor and the ceiling", () => {
    expect(selectionText({ from, to, minMs: 800, maxMs: 2000 }, 1, "UTC")).toBe(
      "1 trace selected: 2026-11-06 14:13:00 to 2026-11-06 14:17:00, between 800 ms and 2 s"
    )
  })
})

describe("durationBoundText", () => {
  it.each([
    [1, "1 ms"],
    [7.46, "7.5 ms"],
    [812.3, "812 ms"],
    [1500, "1.5 s"],
    [2000, "2 s"],
    [12_345, "12.35 s"],
  ])("writes %s ms as %s", (ms, text) => {
    expect(durationBoundText(ms)).toBe(text)
  })
})

describe("nudgeSelection", () => {
  const range = { start: 0, end: 60_000 }
  const scale = durationScale(2300)
  const selection: Selection = {
    from: 10_000,
    to: 20_000,
    minMs: 100,
    maxMs: 1000,
  }

  it("moves the start and end by a sixtieth of the range", () => {
    expect(nudgeSelection(selection, "start", 1, range, scale).from).toBe(
      11_000
    )
    expect(nudgeSelection(selection, "end", -2, range, scale).to).toBe(18_000)
  })

  it("keeps the edges inside the range and never crosses them", () => {
    expect(nudgeSelection(selection, "start", -20, range, scale).from).toBe(0)
    expect(nudgeSelection(selection, "end", 100, range, scale).to).toBe(60_000)
    expect(nudgeSelection(selection, "start", 15, range, scale).from).toBe(
      20_000
    )
    expect(nudgeSelection(selection, "end", -15, range, scale).to).toBe(10_000)
  })

  it("moves the floor by a tenth of a decade within the scale and the ceiling", () => {
    expect(
      nudgeSelection(selection, "floor", 1, range, scale).minMs
    ).toBeCloseTo(125.89, 1)
    expect(nudgeSelection(selection, "floor", -30, range, scale).minMs).toBe(1)
    expect(nudgeSelection(selection, "floor", 30, range, scale).minMs).toBe(
      1000
    )
    expect(
      nudgeSelection({ ...selection, maxMs: null }, "floor", 30, range, scale)
        .minMs
    ).toBe(10_000)
  })
})
