import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel, Transform } from "@/spec"

import type { WireField } from "../frames"
import { applyTransforms, prepareFrames } from "."

function deepFreeze<T>(value: T): T {
  if (value !== null && typeof value === "object" && !Object.isFrozen(value)) {
    Object.freeze(value)
    for (const child of Object.values(value)) deepFreeze(child)
  }
  return value
}

const time = (values: unknown[], name = "Time"): WireField => ({
  name,
  type: "time",
  values,
})
const num = (name: string, values: unknown[]): WireField => ({
  name,
  type: "number",
  values,
})
const str = (name: string, values: unknown[]): WireField => ({
  name,
  type: "string",
  values,
})

function frame(
  fields: WireField[],
  extra: Partial<Omit<WireFrame, "fields">> = {}
): WireFrame {
  return deepFreeze({ ref: "A", ...extra, fields })
}

function panel(
  queries: { ref?: string; format?: string }[],
  transform?: Transform[]
): DataPanel {
  return deepFreeze({
    id: "p",
    title: "p",
    type: "table",
    gridPos: { x: 0, y: 0, w: 6, h: 4 },
    datasource: "prom",
    queries: queries.map((query) => ({ query: "q", ...query })),
    transform,
  }) as unknown as DataPanel
}

function run(frames: WireFrame[], ...transforms: Transform[]): WireFrame[] {
  return applyTransforms(frames, transforms)
}

function columns(out: WireFrame): Record<string, unknown[]> {
  return Object.fromEntries(out.fields.map((f) => [f.name, f.values]))
}

describe("query format table", () => {
  const first = frame([time([1, 2]), num("Value", [1, 2])], {
    labels: { pod: "a" },
  })
  const second = frame([time([1, 3]), num("Value", [5, null])], {
    labels: { pod: "b" },
  })

  it("turns the series of a query into one row per series", () => {
    const [table, ...rest] = prepareFrames(panel([{ format: "table" }]), [
      first,
      second,
    ])
    expect(rest).toEqual([])
    expect(table.ref).toBe("A")
    expect(table.fields.map((f) => [f.name, f.type])).toEqual([
      ["Time", "time"],
      ["pod", "string"],
      ["Value", "number"],
    ])
    expect(columns(table)).toEqual({
      Time: [2, 3],
      pod: ["a", "b"],
      Value: [2, 5],
    })
  })

  it("names values per query when the panel has several", () => {
    const b = frame([time([4]), num("Value", [9])], { ref: "B" })
    const out = prepareFrames(
      panel([{ format: "table" }, { ref: "B", format: "table" }]),
      [first, second, b]
    )
    expect(out).toHaveLength(2)
    expect(out[0].fields.map((f) => f.name)).toEqual([
      "Time",
      "pod",
      "Value #A",
    ])
    expect(out[1].ref).toBe("B")
    expect(columns(out[1])).toEqual({ Time: [4], "Value #B": [9] })
  })

  it("leaves other formats and other queries unchanged", () => {
    const b = frame([time([4]), num("Value", [9])], { ref: "B" })
    const out = prepareFrames(
      panel([{ format: "table" }, { format: "timeseries" }]),
      [first, b]
    )
    expect(out).toHaveLength(2)
    expect(out[1]).toEqual(b)
    expect(out[1]).not.toBe(b)
    expect(prepareFrames(panel([{ format: "heatmap" }]), [first])).toEqual([
      first,
    ])
    expect(prepareFrames(panel([{}]), [first])).toEqual([first])
  })

  it("keeps a frame that is already a table", () => {
    const rows = frame([str("host", ["a", "b", "c"]), num("n", [1, 2, 3])])
    expect(prepareFrames(panel([{ format: "table" }]), [rows])).toEqual([rows])
  })

  it("omits Time when no series has one", () => {
    const labeled = frame([num("Value", [1, 2])], { labels: { pod: "a" } })
    const [table] = prepareFrames(panel([{ format: "table" }]), [labeled])
    expect(table.fields.map((f) => f.name)).toEqual(["pod", "Value"])
  })
})

describe("merge", () => {
  it("combines rows with equal time and labels into one row", () => {
    const [out, ...rest] = run(
      [
        frame([time([1, 2]), num("Value", [10, 20])], {
          labels: { pod: "a" },
        }),
        frame([time([1, 2]), num("Errors", [1, 2])], { labels: { pod: "a" } }),
        frame([time([1]), num("Value", [5])], { labels: { pod: "b" } }),
      ],
      { type: "merge" }
    )
    expect(rest).toEqual([])
    expect(out.fields.map((f) => f.name)).toEqual([
      "Time",
      "pod",
      "Value",
      "Errors",
    ])
    expect(columns(out)).toEqual({
      Time: [1, 2, 1],
      pod: ["a", "a", "b"],
      Value: [10, 20, 5],
      Errors: [1, 2, null],
    })
  })

  it("shares a column between frames with the same field name", () => {
    const [out] = run(
      [
        frame([time([1]), num("Value", [1])], { labels: { pod: "a" } }),
        frame([time([2]), num("Value", [2])], { labels: { pod: "a" } }),
      ],
      { type: "merge" }
    )
    expect(columns(out)).toEqual({
      Time: [1, 2],
      pod: ["a", "a"],
      Value: [1, 2],
    })
  })

  it("is a no-op on no frames", () => {
    expect(run([], { type: "merge" })).toEqual([])
  })
})

describe("seriesToRows", () => {
  it("stacks single-number series newest first with a Metric column", () => {
    const [out, ...rest] = run(
      [
        frame([time([1, 3]), num("Value", [10, 30])], {
          name: "cpu",
          labels: { pod: "a" },
        }),
        frame([time([2]), num("Value", [20])], {
          name: "cpu",
          labels: { pod: "b" },
        }),
      ],
      { type: "seriesToRows" }
    )
    expect(rest).toEqual([])
    expect(columns(out)).toEqual({
      Time: [3, 2, 1],
      Metric: ['cpu{pod="a"}', 'cpu{pod="b"}', 'cpu{pod="a"}'],
      Value: [30, 20, 10],
    })
  })

  it("keeps frames that are not a single-number series", () => {
    const two = frame([time([1]), num("a", [1]), num("b", [2])])
    const out = run([two], { type: "seriesToRows" })
    expect(out).toEqual([two])
  })
})

describe("organize", () => {
  const input = frame([
    time([1]),
    num("Value", [1]),
    num("Errors", [2]),
    str("Extra", ["x"]),
  ])

  it("excludes, renames and orders fields", () => {
    const [out] = run([input], {
      type: "organize",
      exclude: ["Extra"],
      rename: { Errors: "Err" },
      order: ["Value"],
    })
    expect(out.fields.map((f) => f.name)).toEqual(["Value", "Time", "Err"])
  })

  it("matches fields by display name", () => {
    const labeled = frame([time([1]), num("Value", [1])], {
      name: "cpu",
      labels: { pod: "a" },
    })
    const [out] = run([labeled], {
      type: "organize",
      rename: { 'cpu{pod="a"}': "CPU" },
    })
    expect(out.fields.map((f) => f.name)).toEqual(["Time", "CPU"])
  })

  it("orders by the renamed name too", () => {
    const [out] = run([input], {
      type: "organize",
      rename: { Errors: "Err" },
      order: ["Err", "Value"],
    })
    expect(out.fields.map((f) => f.name)).toEqual([
      "Err",
      "Value",
      "Time",
      "Extra",
    ])
  })

  it("ignores names that match nothing", () => {
    const [out] = run([input], {
      type: "organize",
      exclude: ["nope"],
      order: ["nope"],
    })
    expect(out.fields.map((f) => f.name)).toEqual([
      "Time",
      "Value",
      "Errors",
      "Extra",
    ])
  })
})

describe("reduce", () => {
  const a = frame([time([1, 2, 3]), num("Value", [1, 2, 3])], {
    name: "cpu",
    labels: { pod: "a" },
  })
  const b = frame([time([1, 2, 3]), num("Value", [10, null, 20])], {
    name: "cpu",
    labels: { pod: "b" },
  })

  it("reduces every number field to a row by default", () => {
    const [out, ...rest] = run([a, b], {
      type: "reduce",
      reducers: ["max", "mean"],
    })
    expect(rest).toEqual([])
    expect(columns(out)).toEqual({
      Field: ['cpu{pod="a"}', 'cpu{pod="b"}'],
      Max: [3, 20],
      Mean: [2, 15],
    })
  })

  it("keeps labels and string fields with reduceFields", () => {
    const wide = frame(
      [
        time([1, 2, 3]),
        num("Value", [1, 2, 3]),
        num("Errors", [0, 1, 1]),
        str("host", ["x", "y", null]),
      ],
      { labels: { pod: "a" } }
    )
    const [single] = run([wide], {
      type: "reduce",
      mode: "reduceFields",
      reducers: ["max"],
    })
    expect(single.labels).toEqual({ pod: "a" })
    expect(columns(single)).toEqual({ Value: [3], Errors: [1], host: ["y"] })
    const [several] = run([wide], {
      type: "reduce",
      mode: "reduceFields",
      reducers: ["min", "max"],
    })
    expect(columns(several)).toEqual({
      "Value Min": [1],
      "Value Max": [3],
      "Errors Min": [0],
      "Errors Max": [1],
      host: ["y"],
    })
  })
})

describe("joinByField", () => {
  const cpu = frame([time([3, 1, 2]), num("Value", [30, 10, 20])], {
    name: "cpu",
  })
  const mem = frame([time([2, 3, 4]), num("Value", [200, 300, 400])], {
    name: "mem",
  })

  it("outer joins on the first time field and sorts by it", () => {
    const [out, ...rest] = run([cpu, mem], { type: "joinByField" })
    expect(rest).toEqual([])
    expect(columns(out)).toEqual({
      Time: [1, 2, 3, 4],
      cpu: [10, 20, 30, null],
      mem: [null, 200, 300, 400],
    })
  })

  it("inner joins keep only keys present in every frame", () => {
    const [out] = run([cpu, mem], { type: "joinByField", mode: "inner" })
    expect(columns(out)).toEqual({
      Time: [2, 3],
      cpu: [20, 30],
      mem: [200, 300],
    })
  })

  it("joins on a named field", () => {
    const left = frame([str("host", ["b", "a"]), num("cpu", [2, 1])])
    const right = frame([str("host", ["a", "c"]), num("mem", [10, 30])])
    const [out] = run([left, right], { type: "joinByField", field: "host" })
    expect(columns(out)).toEqual({
      host: ["a", "b", "c"],
      cpu: [1, 2, null],
      mem: [10, null, 30],
    })
  })

  it("keeps distinct names for fields that share a display name", () => {
    const x = frame([time([1]), num("Value", [1])])
    const y = frame([time([1]), num("Value", [2])])
    const [out] = run([x, y], { type: "joinByField" })
    expect(out.fields.map((f) => f.name)).toEqual(["Time", "Value", "Value#1"])
  })

  it("is a no-op when no frame has the field", () => {
    const out = run([cpu], { type: "joinByField", field: "missing" })
    expect(out).toEqual([cpu])
  })
})

describe("filterFieldsByName", () => {
  const input = frame([
    time([1]),
    num("Value", [1]),
    num("Errors", [2]),
    str("host", ["x"]),
  ])

  it("keeps the fields named in include", () => {
    const [out] = run([input], {
      type: "filterFieldsByName",
      include: ["Value", "host"],
    })
    expect(out.fields.map((f) => f.name)).toEqual(["Value", "host"])
  })

  it("keeps the fields matching includeRegex", () => {
    const [out] = run([input], {
      type: "filterFieldsByName",
      includeRegex: "^(Err|ho)",
    })
    expect(out.fields.map((f) => f.name)).toEqual(["Errors", "host"])
  })

  it("keeps a field matched by either option", () => {
    const [out] = run([input], {
      type: "filterFieldsByName",
      include: ["Time"],
      includeRegex: "^Err",
    })
    expect(out.fields.map((f) => f.name)).toEqual(["Time", "Errors"])
  })

  it("keeps every field for an invalid pattern", () => {
    const [out] = run([input], {
      type: "filterFieldsByName",
      includeRegex: "(",
    })
    expect(out.fields).toHaveLength(4)
  })

  it("drops frames left without fields", () => {
    const other = frame([num("x", [1])])
    const out = run([input, other], {
      type: "filterFieldsByName",
      include: ["Value"],
    })
    expect(out).toHaveLength(1)
  })

  it("matches display names", () => {
    const labeled = frame([time([1]), num("Value", [1])], {
      name: "cpu",
      labels: { pod: "a" },
    })
    const [out] = run([labeled], {
      type: "filterFieldsByName",
      include: ['cpu{pod="a"}'],
    })
    expect(out.fields.map((f) => f.name)).toEqual(["Value"])
  })
})

describe("calculateField", () => {
  const input = frame([
    time([1, 2, 3]),
    num("Value", [1, 2, null]),
    num("A", [10, 9, 8]),
    num("B", [2, 0, 4]),
  ])

  it("computes a field against a number literal", () => {
    const [out] = run([input], {
      type: "calculateField",
      alias: "pct",
      expr: "Value * 100",
    })
    expect(out.fields.at(-1)).toEqual({
      name: "pct",
      type: "number",
      values: [100, 200, null],
    })
  })

  it("computes between two fields and yields null for division by zero", () => {
    const [out] = run([input], {
      type: "calculateField",
      alias: "ratio",
      expr: "A / B",
    })
    expect(out.fields.at(-1)?.values).toEqual([5, null, 2])
  })

  it("supports all four operators with a literal on the left", () => {
    const calc = (expr: string) =>
      run([input], { type: "calculateField", alias: "r", expr })[0].fields.at(
        -1
      )?.values
    expect(calc("A + B")).toEqual([12, 9, 12])
    expect(calc("A - B")).toEqual([8, 9, 4])
    expect(calc("A*B")).toEqual([20, 0, 32])
    expect(calc("100 - A")).toEqual([90, 91, 92])
    expect(calc("A * -1")).toEqual([-10, -9, -8])
  })

  it("reduces the number fields of each row with reduceRow", () => {
    const [out] = run([input], {
      type: "calculateField",
      alias: "total",
      reduceRow: "sum",
    })
    expect(out.fields.at(-1)?.values).toEqual([13, 11, 12])
  })

  it("replaces a field with the alias's name", () => {
    const [out] = run([input], {
      type: "calculateField",
      alias: "A",
      expr: "A + B",
    })
    expect(out.fields.filter((f) => f.name === "A")).toHaveLength(1)
    expect(out.fields.at(-1)?.values).toEqual([12, 9, 12])
  })

  it("is a no-op without operands or options", () => {
    const missing = run([input], {
      type: "calculateField",
      alias: "x",
      expr: "Nope * 2",
    })
    expect(missing).toEqual([input])
    expect(run([input], { type: "calculateField", alias: "x" })).toEqual([
      input,
    ])
  })
})

describe("sortBy", () => {
  const input = frame([
    str("host", ["b", "a", "c", "d"]),
    num("Value", [2, null, 3, 1]),
  ])

  it("sorts numbers ascending with nulls last", () => {
    const [out] = run([input], { type: "sortBy", field: "Value" })
    expect(columns(out)).toEqual({
      host: ["d", "b", "c", "a"],
      Value: [1, 2, 3, null],
    })
  })

  it("sorts descending with nulls still last", () => {
    const [out] = run([input], { type: "sortBy", field: "Value", desc: true })
    expect(columns(out)).toEqual({
      host: ["c", "b", "d", "a"],
      Value: [3, 2, 1, null],
    })
  })

  it("sorts strings with localeCompare", () => {
    const [out] = run([input], { type: "sortBy", field: "host" })
    expect(columns(out).host).toEqual(["a", "b", "c", "d"])
  })

  it("sorts by time numerically", () => {
    const series = frame([time([10, 9, 100]), num("Value", [1, 2, 3])])
    const [out] = run([series], { type: "sortBy", field: "Time" })
    expect(columns(out).Time).toEqual([9, 10, 100])
  })

  it("is a no-op for an unknown field", () => {
    expect(run([input], { type: "sortBy", field: "nope" })).toEqual([input])
  })
})

describe("limit", () => {
  const input = frame([time([1, 2, 3]), num("Value", [1, 2, 3])])

  it("keeps the first rows of every frame", () => {
    const [out] = run([input, input], { type: "limit", count: 2 })
    expect(columns(out)).toEqual({ Time: [1, 2], Value: [1, 2] })
  })

  it("keeps nothing for zero and everything for a large count", () => {
    expect(
      run([input], { type: "limit", count: 0 })[0].fields[0].values
    ).toEqual([])
    expect(run([input], { type: "limit", count: 99 })).toEqual([input])
  })
})

describe("chaining", () => {
  it("applies transforms in order", () => {
    const input = frame([
      str("host", ["a", "b", "c", "d"]),
      num("Value", [3, 1, 4, 2]),
      str("noise", ["x", "x", "x", "x"]),
    ])
    const [out] = run(
      [input],
      { type: "organize", exclude: ["noise"], order: ["Value"] },
      { type: "sortBy", field: "Value", desc: true },
      { type: "limit", count: 2 }
    )
    expect(columns(out)).toEqual({ Value: [4, 3], host: ["c", "a"] })
  })

  it("prepares frames with the query format first", () => {
    const series = [
      frame([time([1]), num("Value", [5])], { labels: { pod: "a" } }),
      frame([time([1]), num("Value", [9])], { labels: { pod: "b" } }),
    ]
    const [out] = prepareFrames(
      panel(
        [{ format: "table" }],
        [
          { type: "sortBy", field: "Value", desc: true },
          { type: "limit", count: 1 },
        ]
      ),
      series
    )
    expect(columns(out)).toEqual({ Time: [1], pod: ["b"], Value: [9] })
  })

  it("returns the frames when there is nothing to apply", () => {
    const input = frame([num("Value", [1])])
    expect(applyTransforms([input], undefined)).toEqual([input])
    expect(applyTransforms([input], [])).toEqual([input])
  })
})

describe("empty input", () => {
  const all: Transform[] = [
    { type: "merge" },
    { type: "seriesToRows" },
    { type: "organize", exclude: ["a"] },
    { type: "reduce", reducers: ["last"] },
    { type: "joinByField" },
    { type: "filterFieldsByName", include: ["a"] },
    { type: "calculateField", alias: "x", expr: "a + 1" },
    { type: "sortBy", field: "a" },
    { type: "limit", count: 1 },
  ]

  it("never throws on no frames or frames without fields", () => {
    const empty = frame([])
    for (const transform of all) {
      expect(() => run([], transform)).not.toThrow()
      expect(() => run([empty], transform)).not.toThrow()
    }
  })
})
