import { describe, expect, it } from "vitest"

import {
  candidateRows,
  tooltipRows,
  type StackEntry,
  type TooltipRow,
  type TooltipRowOptions,
} from "./tooltip-rows"

const plain: TooltipRowOptions = {
  sort: "none",
  hideZeros: false,
  cumulative: false,
  stack: [],
}

const rows: TooltipRow[] = [
  { index: 0, value: 5 },
  { index: 1, value: 0 },
  { index: 2, value: 9 },
  { index: 3, value: 5 },
]

const indexes = (result: { rows: TooltipRow[] }) =>
  result.rows.map((row) => row.index)

describe("tooltipRows", () => {
  it("keeps series order for none", () => {
    expect(indexes(tooltipRows(rows, plain, 10))).toEqual([0, 1, 2, 3])
  })

  it("sorts ascending and descending, ties in series order", () => {
    expect(indexes(tooltipRows(rows, { ...plain, sort: "asc" }, 10))).toEqual([
      1, 0, 3, 2,
    ])
    expect(indexes(tooltipRows(rows, { ...plain, sort: "desc" }, 10))).toEqual([
      2, 0, 3, 1,
    ])
  })

  it("matches the descending order the tooltip always had", () => {
    const old = [...rows].sort((a, b) => b.value - a.value).slice(0, 10)
    const options = { ...plain, sort: "desc" as const }
    expect(tooltipRows(rows, options, 10)).toEqual({
      rows: old,
      total: rows.length,
    })
  })

  it("does not change the rows it was given", () => {
    const before = structuredClone(rows)
    tooltipRows(rows, { ...plain, sort: "desc" }, 2)
    expect(rows).toEqual(before)
  })

  it("drops zero rows and counts only the rest", () => {
    const result = tooltipRows(rows, { ...plain, hideZeros: true }, 10)
    expect(indexes(result)).toEqual([0, 2, 3])
    expect(result.total).toBe(3)
  })

  it("limits the rows after sorting and reports the full count", () => {
    const result = tooltipRows(rows, { ...plain, sort: "desc" }, 2)
    expect(indexes(result)).toEqual([2, 0])
    expect(result.total).toBe(4)
  })

  it("shows running totals for stacked rows and leaves the others", () => {
    const stack: StackEntry[] = [
      { stacked: true, below: null, value: 5 },
      { stacked: true, below: 0, value: 0 },
      { stacked: false, below: null, value: 9 },
      { stacked: true, below: 1, value: 5 },
    ]
    const result = tooltipRows(rows, { ...plain, cumulative: true, stack }, 10)
    expect(result.rows).toEqual([
      { index: 0, value: 5 },
      { index: 1, value: 5 },
      { index: 2, value: 9 },
      { index: 3, value: 10 },
    ])
  })

  it("adds a stacked series with no value at the hovered time as zero", () => {
    const stack: StackEntry[] = [
      { stacked: true, below: null, value: 4 },
      { stacked: true, below: 0, value: 0 },
      { stacked: true, below: 1, value: 3 },
    ]
    const result = tooltipRows(
      [
        { index: 0, value: 4 },
        { index: 2, value: 3 },
      ],
      { ...plain, cumulative: true, stack },
      10
    )
    expect(result.rows).toEqual([
      { index: 0, value: 4 },
      { index: 2, value: 7 },
    ])
  })

  it("keeps separate stacks apart", () => {
    const stack: StackEntry[] = [
      { stacked: true, below: null, value: 2 },
      { stacked: true, below: null, value: 3 },
      { stacked: true, below: 0, value: 4 },
      { stacked: true, below: 1, value: 5 },
    ]
    const all = stack.map((entry, index) => ({ index, value: entry.value }))
    const result = tooltipRows(all, { ...plain, cumulative: true, stack }, 10)
    expect(result.rows.map((row) => row.value)).toEqual([2, 3, 6, 8])
  })

  it("combines hidden zeros, totals, sorting and the limit", () => {
    const stack: StackEntry[] = [
      { stacked: true, below: null, value: 5 },
      { stacked: true, below: 0, value: 0 },
      { stacked: false, below: null, value: 9 },
      { stacked: true, below: 1, value: 5 },
    ]
    const result = tooltipRows(
      rows,
      { sort: "desc", hideZeros: true, cumulative: true, stack },
      2
    )
    expect(result).toEqual({
      rows: [
        { index: 3, value: 10 },
        { index: 2, value: 9 },
      ],
      total: 3,
    })
  })
})

describe("candidateRows", () => {
  const entries = [
    { hideInTooltip: false, values: [1, 2] },
    { hideInTooltip: true, values: [3, 4] },
    { hideInTooltip: false, values: [null, 6] },
    { hideInTooltip: false, values: [7, undefined] },
  ]

  it("lists the numeric values at the index of the series that are shown", () => {
    expect(candidateRows(entries, 1, () => true)).toEqual([
      { index: 0, value: 2 },
      { index: 2, value: 6 },
    ])
  })

  it("skips gaps and series the plot has switched off", () => {
    expect(candidateRows(entries, 0, (index) => index !== 0)).toEqual([
      { index: 3, value: 7 },
    ])
  })
})
