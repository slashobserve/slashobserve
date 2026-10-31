import { describe, expect, it } from "vitest"

import type { AlignedFrames } from "./align"
import {
  insertGaps,
  insertNullsThresholdMs,
  nullsAsZero,
  queryIntervalMs,
  shapeNulls,
  spanShortGaps,
} from "./gaps"

const times = [0, 10, 20, 30, 40, 50]

describe("spanShortGaps", () => {
  it("joins a gap shorter than the limit", () => {
    expect(spanShortGaps(times, [1, null, 3, 4, 5, 6], 30_000)).toEqual([
      1,
      undefined,
      3,
      4,
      5,
      6,
    ])
  })

  it("joins a gap exactly as long as the limit", () => {
    expect(spanShortGaps(times, [1, null, 3, 4, 5, 6], 20_000)[1]).toBe(
      undefined
    )
  })

  it("keeps a gap longer than the limit", () => {
    expect(spanShortGaps(times, [1, null, null, 4, 5, 6], 20_000)).toEqual([
      1,
      null,
      null,
      4,
      5,
      6,
    ])
  })

  it("keeps a gap at the start and at the end", () => {
    const values = [null, null, 3, 4, null, null]
    expect(spanShortGaps(times, values, 600_000)).toEqual(values)
  })

  it("counts missing samples inside a null run as part of the run", () => {
    const values = [1, undefined, null, undefined, 5, 6]
    expect(spanShortGaps(times, values, 40_000)).toEqual([
      1,
      undefined,
      undefined,
      undefined,
      5,
      6,
    ])
    expect(spanShortGaps(times, values, 39_999)).toEqual(values)
  })

  it("joins the short gaps of a series and keeps its long ones", () => {
    expect(spanShortGaps(times, [1, null, 3, null, null, 6], 20_000)).toEqual([
      1,
      undefined,
      3,
      null,
      null,
      6,
    ])
  })

  it("returns the same array when nothing changes", () => {
    const values = [1, 2, null, null, null, 6]
    expect(spanShortGaps(times, values, 10_000)).toBe(values)
  })
})

describe("insertGaps", () => {
  it("nulls the first aligned cell inside a gap longer than the threshold", () => {
    const grid = {
      xs: [0, 10, 20, 30],
      columns: [
        [1, undefined, undefined, 4],
        [5, 6, 7, 8],
      ],
    }
    expect(insertGaps(grid, 0, 25_000)).toEqual({
      xs: [0, 10, 20, 30],
      columns: [
        [1, null, undefined, 4],
        [5, 6, 7, 8],
      ],
    })
  })

  it("inserts an x at the midpoint when no aligned x lies inside the gap", () => {
    const grid = {
      xs: [0, 60, 70],
      columns: [
        [1, 2, 3],
        [4, 5, 6],
      ],
    }
    expect(insertGaps(grid, 0, 30_000)).toEqual({
      xs: [0, 30, 60, 70],
      columns: [
        [1, null, 2, 3],
        [4, undefined, 5, 6],
      ],
    })
  })

  it("leaves a gap exactly as long as the threshold alone", () => {
    const grid = { xs: [0, 30], columns: [[1, 2]] }
    expect(insertGaps(grid, 0, 30_000)).toBe(grid)
  })

  it("measures between present samples, skipping nulls", () => {
    const grid = {
      xs: [0, 10, 20, 100],
      columns: [[1, null, null, 4]],
    }
    expect(insertGaps(grid, 0, 50_000).columns[0]).toEqual([1, null, null, 4])
  })

  it("does not touch the other columns when it nulls a cell", () => {
    const other = [5, 6, 7]
    const grid = { xs: [0, 10, 50], columns: [[1, undefined, 3], other] }
    const result = insertGaps(grid, 0, 20_000)
    expect(result.columns[0]).toEqual([1, null, 3])
    expect(result.columns[1]).toEqual(other)
  })
})

describe("nullsAsZero", () => {
  it("turns null into zero and keeps missing samples missing", () => {
    expect(nullsAsZero([1, null, undefined, 0])).toEqual([1, 0, undefined, 0])
  })
})

describe("shapeNulls", () => {
  const off = { nullAsZero: false }
  const ref = "A"
  const meta = { step_ms: 30_000, queries: [] }

  function frames(): AlignedFrames {
    const series = (values: (number | null | undefined)[]) =>
      ({
        key: "k",
        label: "l",
        values,
        frame: { ref },
      }) as AlignedFrames["series"][number]
    return {
      xs: [0, 60, 70],
      series: [series([1, 2, null]), series([4, 5, 6])],
      ghosts: [[7, 8, 9]],
    }
  }

  it("returns the same frames when no series asks for it", () => {
    const aligned = frames()
    expect(shapeNulls(aligned, [off, off], meta)).toBe(aligned)
  })

  it("applies insertNulls to every column and nullAsZero to its own series", () => {
    const shaped = shapeNulls(
      frames(),
      [{ insertNulls: 30_000, nullAsZero: true }, off],
      meta
    )
    expect(shaped.xs).toEqual([0, 30, 60, 70])
    expect(shaped.series[0].values).toEqual([1, 0, 2, 0])
    expect(shaped.series[1].values).toEqual([4, undefined, 5, 6])
    expect(shaped.ghosts).toEqual([[7, undefined, 8, 9]])
  })
})

describe("queryIntervalMs", () => {
  const meta = {
    step_ms: 60_000,
    queries: [
      { ref: "A", kind: "range" as const, step_ms: 15_000 },
      { ref: "B", kind: "instant" as const, step_ms: 0 },
    ],
  }

  it("uses the step of the frame's own query", () => {
    expect(queryIntervalMs(meta, "A")).toBe(15_000)
  })

  it("falls back to the panel step for an unknown or instant query", () => {
    expect(queryIntervalMs(meta, "C")).toBe(60_000)
    expect(queryIntervalMs(meta, "B")).toBe(60_000)
  })
})

describe("insertNullsThresholdMs", () => {
  it.each<[number | true | undefined, number, number | undefined]>([
    [undefined, 15_000, undefined],
    [30_000, 15_000, 30_000],
    [true, 15_000, 15_000],
    [true, 0, undefined],
  ])("setting %s at interval %s gives %s", (setting, interval, expected) => {
    expect(insertNullsThresholdMs(setting, interval)).toBe(expected)
  })
})

describe("shapeNulls with insertNulls: true", () => {
  const series = (ref: string, values: (number | null)[]) =>
    ({
      key: ref,
      label: ref,
      values,
      frame: { ref },
    }) as AlignedFrames["series"][number]

  it("breaks each series at its own query interval", () => {
    const aligned: AlignedFrames = {
      xs: [0, 10, 20, 50],
      series: [series("A", [1, 2, 3, 4]), series("B", [5, 6, 7, 8])],
      ghosts: [],
    }
    const meta = {
      step_ms: 60_000,
      queries: [{ ref: "A", kind: "range" as const, step_ms: 10_000 }],
    }
    const shaped = shapeNulls(
      aligned,
      [
        { insertNulls: true, nullAsZero: false },
        { insertNulls: true, nullAsZero: false },
      ],
      meta
    )
    expect(shaped.xs).toEqual([0, 10, 20, 35, 50])
    expect(shaped.series[0].values).toEqual([1, 2, 3, null, 4])
    expect(shaped.series[1].values).toEqual([5, 6, 7, undefined, 8])
  })
})
