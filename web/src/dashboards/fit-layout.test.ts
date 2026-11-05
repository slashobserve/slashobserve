import { describe, expect, it } from "vitest"
import type { Layout } from "react-grid-layout"
import { collides } from "react-grid-layout/core"

import { columnsFor, fitLayout } from "./fit-layout"

type Box = readonly [string, number, number, number, number]

function layoutOf(boxes: readonly Box[]): Layout {
  return boxes.map(([i, x, y, w, h]) => ({ i, x, y, w, h }))
}

function strip(prefix: string, count: number, y: number, h: number): Box[] {
  return Array.from({ length: count }, (_, k) => [
    `${prefix}${k + 1}`,
    4 * k,
    y,
    4,
    h,
  ])
}

function boxesOf(layout: Layout): Box[] {
  return layout.map(({ i, x, y, w, h }) => [i, x, y, w, h])
}

function expectNoOverlap(layout: Layout) {
  for (const [index, a] of layout.entries()) {
    for (const b of layout.slice(index + 1)) {
      expect(collides(a, b), `${a.i} overlaps ${b.i}`).toBe(false)
    }
  }
}

const cases: {
  name: string
  input: Box[]
  minimum: Record<string, number>
  fixed?: string[]
  want: Box[]
}[] = [
  {
    name: "keeps a layout whose panels meet their minimums",
    input: [
      ["a", 0, 0, 12, 8],
      ["b", 12, 0, 12, 8],
    ],
    minimum: { a: 6, b: 6 },
    want: [
      ["a", 0, 0, 12, 8],
      ["b", 12, 0, 12, 8],
    ],
  },
  {
    name: "stacks every item when every minimum is the full width",
    input: [
      ["a", 0, 0, 12, 8],
      ["b", 12, 0, 12, 4],
      ["c", 12, 4, 12, 4],
      ["d", 0, 8, 24, 8],
    ],
    minimum: { a: 24, b: 24, c: 24, d: 24 },
    want: [
      ["a", 0, 0, 24, 8],
      ["b", 0, 8, 24, 4],
      ["c", 0, 12, 24, 4],
      ["d", 0, 16, 24, 8],
    ],
  },
  {
    name: "keeps the input order",
    input: [
      ["d", 0, 8, 24, 8],
      ["c", 12, 4, 12, 4],
      ["b", 12, 0, 12, 4],
      ["a", 0, 0, 12, 8],
    ],
    minimum: { a: 24, b: 24, c: 24, d: 24 },
    want: [
      ["d", 0, 16, 24, 8],
      ["c", 0, 12, 24, 4],
      ["b", 0, 8, 24, 4],
      ["a", 0, 0, 24, 8],
    ],
  },
  {
    name: "flows a strip of six into two even lines",
    input: [...strip("s", 6, 0, 4), ["c", 0, 4, 24, 8]],
    minimum: { s1: 6, s2: 6, s3: 6, s4: 6, s5: 6, s6: 6 },
    want: [
      ["s1", 0, 0, 8, 4],
      ["s2", 8, 0, 8, 4],
      ["s3", 16, 0, 8, 4],
      ["s4", 0, 4, 8, 4],
      ["s5", 8, 4, 8, 4],
      ["s6", 16, 4, 8, 4],
      ["c", 0, 8, 24, 8],
    ],
  },
  {
    name: "reflows a band of mixed heights",
    input: [
      ["big", 0, 0, 12, 8],
      ["g1", 12, 0, 6, 4],
      ["g2", 18, 0, 6, 4],
      ["g3", 12, 4, 12, 4],
    ],
    minimum: { big: 6, g1: 7, g2: 7 },
    want: [
      ["big", 0, 0, 16, 8],
      ["g1", 16, 0, 8, 4],
      ["g2", 0, 8, 9, 4],
      ["g3", 9, 8, 15, 4],
    ],
  },
  {
    name: "a row header below a reflowed band moves down with it",
    input: [
      ["r1", 0, 0, 24, 1],
      ...strip("s", 6, 1, 4),
      ["r2", 0, 5, 24, 1],
      ["t", 0, 6, 24, 8],
    ],
    minimum: { s1: 6, s2: 6, s3: 6, s4: 6, s5: 6, s6: 6 },
    want: [
      ["r1", 0, 0, 24, 1],
      ["s1", 0, 1, 8, 4],
      ["s2", 8, 1, 8, 4],
      ["s3", 16, 1, 8, 4],
      ["s4", 0, 5, 8, 4],
      ["s5", 8, 5, 8, 4],
      ["s6", 16, 5, 8, 4],
      ["r2", 0, 9, 24, 1],
      ["t", 0, 10, 24, 8],
    ],
  },
  {
    name: "a band that fits keeps its positions below a reflowed one",
    input: [...strip("s", 6, 0, 4), ["a", 0, 4, 12, 8], ["b", 12, 4, 12, 8]],
    minimum: { s1: 6, s2: 6, s3: 6, s4: 6, s5: 6, s6: 6, a: 6, b: 6 },
    want: [
      ["s1", 0, 0, 8, 4],
      ["s2", 8, 0, 8, 4],
      ["s3", 16, 0, 8, 4],
      ["s4", 0, 4, 8, 4],
      ["s5", 8, 4, 8, 4],
      ["s6", 16, 4, 8, 4],
      ["a", 0, 8, 12, 8],
      ["b", 12, 8, 12, 8],
    ],
  },
  {
    name: "a lone narrow panel widens to the full line",
    input: [["g", 0, 0, 3, 4]],
    minimum: { g: 5 },
    want: [["g", 0, 0, 24, 4]],
  },
  {
    name: "a narrow fixed panel never triggers a reflow",
    input: [
      ["f", 0, 0, 3, 4],
      ["t", 3, 0, 21, 4],
    ],
    minimum: { f: 5, t: 6 },
    fixed: ["f"],
    want: [
      ["f", 0, 0, 3, 4],
      ["t", 3, 0, 21, 4],
    ],
  },
  {
    name: "a fixed panel keeps its size in a reflowed band",
    input: [...strip("s", 5, 0, 4), ["f", 20, 0, 4, 4]],
    minimum: { s1: 6, s2: 6, s3: 6, s4: 6, s5: 6 },
    fixed: ["f"],
    want: [
      ["s1", 0, 0, 8, 4],
      ["s2", 8, 0, 8, 4],
      ["s3", 16, 0, 8, 4],
      ["s4", 0, 4, 10, 4],
      ["s5", 10, 4, 10, 4],
      ["f", 20, 4, 4, 4],
    ],
  },
  {
    name: "a line of only fixed panels is not widened",
    input: [
      ["r", 0, 0, 6, 4],
      ["f1", 6, 0, 9, 4],
      ["f2", 15, 0, 9, 4],
    ],
    minimum: { r: 12 },
    fixed: ["f1", "f2"],
    want: [
      ["r", 0, 0, 15, 4],
      ["f1", 15, 0, 9, 4],
      ["f2", 0, 4, 9, 4],
    ],
  },
]

describe("fitLayout", () => {
  it.each(cases)("$name", ({ input, minimum, fixed, want }) => {
    const result = fitLayout(
      layoutOf(input),
      new Map(Object.entries(minimum)),
      new Set(fixed ?? [])
    )
    expect(boxesOf(result)).toEqual(want)
    expectNoOverlap(result)
  })

  it("returns the same item when nothing changes", () => {
    const layout = layoutOf([["a", 0, 0, 12, 8]])
    expect(fitLayout(layout, new Map([["a", 6]]), new Set())[0]).toBe(layout[0])
  })
})

describe("columnsFor", () => {
  it.each([
    [undefined, 1024, 1],
    [160, 1024, 4],
    [160, 800, 6],
    [160, 1400, 3],
    [180, 1254, 4],
    [5000, 1024, 24],
  ])("a minimum of %s pixels at %s needs %s columns", (pixels, width, want) => {
    expect(columnsFor(pixels, width)).toBe(want)
  })
})
