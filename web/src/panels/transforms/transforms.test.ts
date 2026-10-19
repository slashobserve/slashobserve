import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel, Transform } from "@/spec"

import type { WireField } from "../frames"
import {
  applyTransforms,
  prepareFrames,
  seriesName,
  type PreparedFrame,
} from "."

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
  queries: {
    ref?: string
    format?: string
    legend?: string
    hide?: boolean
  }[],
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

function run(frames: WireFrame[], ...transforms: Transform[]): PreparedFrame[] {
  return applyTransforms(frames, transforms)
}

function columns(out: PreparedFrame): Record<string, unknown[]> {
  return Object.fromEntries(out.fields.map((f) => [f.name, f.values]))
}

describe("query format table", () => {
  const tablePanel = panel([{ format: "table" }])

  it("passes a raw table through unchanged", () => {
    const raw = frame([
      str("service", ["api", "web", "db"]),
      time([1, 2, 3], "last_seen"),
      num("c", [10, 20, 30]),
    ])
    const out = prepareFrames(tablePanel, [raw])
    expect(out).toEqual([raw])
    expect(out[0]).not.toBe(raw)
  })

  it("turns instant series into one row each", () => {
    const series = (instance: string, value: number) =>
      frame([time([5]), num("Value", [value])], {
        name: "up",
        labels: { instance, job: "node" },
      })
    const [out, ...rest] = prepareFrames(tablePanel, [
      series("a:1", 1),
      series("b:1", 0),
    ])
    expect(rest).toEqual([])
    expect(out.ref).toBe("A")
    expect(out.fields.map((f) => [f.name, f.type])).toEqual([
      ["Time", "time"],
      ["__name__", "string"],
      ["instance", "string"],
      ["job", "string"],
      ["Value", "number"],
    ])
    expect(columns(out)).toEqual({
      Time: [5, 5],
      __name__: ["up", "up"],
      instance: ["a:1", "b:1"],
      job: ["node", "node"],
      Value: [1, 0],
    })
  })

  it("keeps every sample of range series in frame then sample order", () => {
    const [out] = prepareFrames(tablePanel, [
      frame([time([1, 2, 3]), num("Value", [1, 2, 3])], {
        labels: { pod: "a" },
      }),
      frame([time([1, 2, 3]), num("Value", [4, null, 6])], {
        labels: { pod: "b" },
      }),
    ])
    expect(columns(out)).toEqual({
      Time: [1, 2, 3, 1, 2, 3],
      pod: ["a", "a", "a", "b", "b", "b"],
      Value: [1, 2, 3, 4, null, 6],
    })
  })

  it("names values per query only when several table queries returned data", () => {
    const a = frame([time([1]), num("Value", [1])], { ref: "A" })
    const b = frame([time([4]), num("Value", [9])], { ref: "B" })
    const both = prepareFrames(
      panel([{ format: "table" }, { ref: "B", format: "table" }]),
      [a, b]
    )
    expect(both.map((f) => f.ref)).toEqual(["A", "B"])
    expect(both[0].fields.map((f) => f.name)).toEqual(["Time", "Value #A"])
    expect(columns(both[1])).toEqual({ Time: [4], "Value #B": [9] })

    const noData = panel([
      { format: "table" },
      { ref: "B", format: "table" },
      { ref: "C", format: "table", hide: true },
    ])
    expect(prepareFrames(noData, [a])[0].fields.map((f) => f.name)).toEqual([
      "Time",
      "Value",
    ])
  })

  it("adds no __name__ without a frame name and unions label keys", () => {
    const [plain] = prepareFrames(tablePanel, [
      frame([num("Value", [1])], { labels: { pod: "a" } }),
    ])
    expect(plain.fields.map((f) => f.name)).toEqual(["pod", "Value"])

    const [mixed] = prepareFrames(tablePanel, [
      frame([num("Value", [1])], { labels: { pod: "a" } }),
      frame([num("Value", [2])], { labels: { zone: "z" } }),
    ])
    expect(columns(mixed)).toEqual({
      pod: ["a", null],
      zone: [null, "z"],
      Value: [1, 2],
    })
  })

  it("flattens pivoted frames and keeps every row", () => {
    const [out] = prepareFrames(tablePanel, [
      frame([time([1, 2]), num("c", [1, 2]), num("p95", [3, 4])], {
        labels: { service: "api" },
      }),
      frame([time([1, 2]), num("c", [5, 6]), num("p95", [7, 8])], {
        labels: { service: "web" },
      }),
    ])
    expect(out.fields.map((f) => f.name)).toEqual([
      "Time",
      "service",
      "c",
      "p95",
    ])
    expect(columns(out).c).toEqual([1, 2, 5, 6])
    expect(columns(out).service).toEqual(["api", "api", "web", "web"])
  })

  it("keeps a second field of the same name instead of dropping it", () => {
    const [out] = prepareFrames(tablePanel, [
      frame([time([1]), num("c", [1]), num("c", [2])], {
        labels: { pod: "a" },
      }),
      frame([time([1]), num("c", [3]), num("c", [4])], {
        labels: { pod: "b" },
      }),
    ])
    expect(out.fields.map((f) => f.name)).toEqual(["Time", "pod", "c", "c#1"])
    expect(columns(out)).toEqual({
      Time: [1, 1],
      pod: ["a", "b"],
      c: [1, 3],
      "c#1": [2, 4],
    })
  })

  it("keeps every output column name unique when labels use reserved names", () => {
    const [out] = prepareFrames(tablePanel, [
      frame([time([5]), num("Value", [1])], {
        name: "up",
        labels: { Time: "t", Value: "v" },
      }),
    ])
    expect(out.fields.map((f) => [f.name, f.type])).toEqual([
      ["Time", "time"],
      ["Time#1", "string"],
      ["Value#1", "string"],
      ["__name__", "string"],
      ["Value", "number"],
    ])
    expect(columns(out)).toEqual({
      Time: [5],
      "Time#1": ["t"],
      "Value#1": ["v"],
      __name__: ["up"],
      Value: [1],
    })
  })

  it("names a later time field called Time apart from the time column", () => {
    const [out] = prepareFrames(tablePanel, [
      frame([time([5]), time([9], "Time"), num("Value", [1])], {
        labels: { pod: "a" },
      }),
    ])
    expect(out.fields.map((f) => [f.name, f.type])).toEqual([
      ["Time", "time"],
      ["pod", "string"],
      ["Time#1", "time"],
      ["Value", "number"],
    ])
    expect(columns(out).Time).toEqual([5])
    expect(columns(out)["Time#1"]).toEqual([9])
  })

  it("leaves other formats and other queries unchanged", () => {
    const a = frame([time([1, 2]), num("Value", [1, 2])], {
      labels: { pod: "a" },
    })
    const b = frame([time([4]), num("Value", [9])], { ref: "B" })
    const out = prepareFrames(
      panel([{ format: "table" }, { format: "timeseries" }]),
      [a, b]
    )
    expect(out).toHaveLength(2)
    expect(out[1]).toEqual(b)
    expect(out[1]).not.toBe(b)
    expect(prepareFrames(panel([{ format: "heatmap" }]), [a])).toEqual([a])
    expect(prepareFrames(panel([{}]), [a])).toEqual([a])
  })

  it("stamps a legend on timeseries queries but never on table queries", () => {
    const series = frame([time([1]), num("Value", [1])], {
      labels: { pod: "checkout-0" },
    })
    const [stamped] = prepareFrames(
      panel([{ format: "timeseries", legend: "{{pod}} pod" }]),
      [series]
    )
    expect(stamped.fields[1].seriesName).toBe("checkout-0 pod")
    expect(seriesName(stamped, stamped.fields[1])).toBe("checkout-0 pod")

    const [flat] = prepareFrames(
      panel([{ format: "table", legend: "{{pod}} pod" }]),
      [series]
    )
    expect(flat.fields.every((f) => f.seriesName === undefined)).toBe(true)
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

  it("keeps both values of same-labelled series that share a Value name", () => {
    const [out] = run(
      [
        frame([time([1, 2]), num("Value", [1, 2])], {
          ref: "A",
          labels: { job: "api" },
        }),
        frame([time([1, 2]), num("Value", [7, 8])], {
          ref: "B",
          labels: { job: "api" },
        }),
      ],
      { type: "merge" }
    )
    expect(columns(out)).toEqual({
      Time: [1, 2, 1, 2],
      job: ["api", "api", "api", "api"],
      Value: [1, 2, 7, 8],
    })
  })

  it("merges table-format frames with equal time and labels into one row", () => {
    const [out, ...rest] = run(
      [
        frame([
          time([1, 2]),
          str("job", ["api", "api"]),
          num("Value #A", [1, 2]),
        ]),
        frame(
          [time([1, 2]), str("job", ["api", "api"]), num("Value #B", [7, 8])],
          { ref: "B" }
        ),
      ],
      { type: "merge" }
    )
    expect(rest).toEqual([])
    expect(out.ref).toBe("A")
    expect(columns(out)).toEqual({
      Time: [1, 2],
      job: ["api", "api"],
      "Value #A": [1, 2],
      "Value #B": [7, 8],
    })
  })

  it("appends a new row when a shared non-key cell conflicts", () => {
    const [out] = run(
      [
        frame([time([1]), str("job", ["api"]), num("v", [5])]),
        frame([time([1]), str("job", ["api"]), num("v", [6])]),
        frame([time([1]), str("job", ["api"]), num("w", [9])]),
      ],
      { type: "merge" }
    )
    expect(columns(out)).toEqual({
      Time: [1, 1],
      job: ["api", "api"],
      v: [5, 6],
      w: [null, 9],
    })
  })

  it("returns the input when no field name is common to every frame", () => {
    const x = frame([time([1]), num("a", [1])])
    const y = frame([str("b", ["x"]), num("c", [2])])
    expect(run([x, y], { type: "merge" })).toEqual([x, y])
  })

  it("returns the input when fewer than two frames have fields", () => {
    const one = frame([time([1]), num("a", [1])], { labels: { pod: "a" } })
    const empty = frame([])
    expect(run([one, empty], { type: "merge" })).toEqual([one, empty])
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

describe("series names", () => {
  it("never repeats the field name for an unlabelled, unnamed frame", () => {
    const wide = frame([num("a", [1]), num("b", [2])])
    expect(wide.fields.map((f) => seriesName(wide, f))).toEqual(["a", "b"])
  })

  it("lets organize, sortBy and filterFieldsByName use the legend name", () => {
    const legendPanel = panel([{ legend: "{{pod}}" }])
    const frames = prepareFrames(legendPanel, [
      frame([time([1, 2]), num("Value", [5, 1])], {
        labels: { pod: "checkout-0" },
      }),
      frame([time([1, 2]), num("Value", [3, 9])], {
        labels: { pod: "checkout-1" },
      }),
    ])
    const [renamed] = applyTransforms(frames, [
      { type: "organize", rename: { "checkout-0": "Checkout" } },
    ])
    expect(renamed.fields.map((f) => f.name)).toEqual(["Time", "Checkout"])
    expect(seriesName(renamed, renamed.fields[1])).toBe("Checkout")

    const [sorted] = applyTransforms(frames, [
      { type: "sortBy", field: "checkout-0" },
    ])
    expect(columns(sorted).Value).toEqual([1, 5])

    const kept = applyTransforms(frames, [
      { type: "filterFieldsByName", include: ["checkout-1"] },
    ])
    expect(kept).toHaveLength(1)
    expect(kept[0].fields.map((f) => f.name)).toEqual(["Value"])
  })

  it("shows an organize rename on a labelled series through its series name", () => {
    const labeled = frame([time([1]), num("Value", [1])], {
      name: "cpu",
      labels: { pod: "a" },
    })
    const [out] = run([labeled], {
      type: "organize",
      rename: { Value: "CPU" },
    })
    expect(seriesName(out, out.fields[1])).toBe("CPU")
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

  it("names rows by series name and carries stamps through reduceFields", () => {
    const stamped = prepareFrames(panel([{ legend: "{{pod}}" }]), [a, b])
    const [rows] = applyTransforms(stamped, [
      { type: "reduce", reducers: ["max"] },
    ])
    expect(columns(rows).Field).toEqual(["a", "b"])
    const [one] = applyTransforms(stamped, [
      { type: "reduce", mode: "reduceFields", reducers: ["max"] },
    ])
    expect(one.fields[0].seriesName).toBe("a")
    const [two] = applyTransforms(stamped, [
      { type: "reduce", mode: "reduceFields", reducers: ["min", "max"] },
    ])
    expect(two.fields.map((f) => f.seriesName)).toEqual(["a Min", "a Max"])
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

  it("keeps joined names unique when a real field already carries the suffix", () => {
    const x = frame([time([1]), num("Value", [1])])
    const y = frame([time([1]), num("Value", [2])])
    const z = frame([time([1]), num("Value#1", [3])])
    const [out] = run([z, x, y], { type: "joinByField" })
    const names = out.fields.map((f) => f.name)
    expect(new Set(names).size).toBe(names.length)
    expect(names).toEqual(["Time", "Value#1", "Value", "Value#2"])
    expect(columns(out)).toEqual({
      Time: [1],
      "Value#1": [3],
      Value: [1],
      "Value#2": [2],
    })
  })

  it("keeps distinct names for fields that share a display name", () => {
    const x = frame([time([1]), num("Value", [1])])
    const y = frame([time([1]), num("Value", [2])])
    const [out] = run([x, y], { type: "joinByField" })
    expect(out.fields.map((f) => f.name)).toEqual(["Time", "Value", "Value#1"])
  })

  it("joins differently named time fields on each frame's own first time field", () => {
    const left = frame([time([1, 2]), num("x", [1, 2])])
    const right = frame([time([1, 2], "ts"), num("y", [7, 8])])
    const out = run([left, right], { type: "joinByField" })
    expect(out).toHaveLength(1)
    expect(columns(out[0])).toEqual({ Time: [1, 2], x: [1, 2], y: [7, 8] })
  })

  it("joins on a number field common to every frame when the first has no time", () => {
    const left = frame([num("id", [1, 2]), num("x", [10, 20])])
    const right = frame([num("id", [2, 3]), num("y", [200, 300])])
    const out = run([left, right], { type: "joinByField" })
    expect(columns(out[0])).toEqual({
      id: [1, 2, 3],
      x: [10, 20, null],
      y: [null, 200, 300],
    })
  })

  it("keeps a null key row after keyed rows in outer mode and drops it in inner mode", () => {
    const left = frame([num("k", [2, null, 1]), num("x", [20, 99, 10])])
    const right = frame([num("k", [1, 2]), num("y", [100, 200])])
    const [outer] = run([left, right], { type: "joinByField", field: "k" })
    expect(columns(outer)).toEqual({
      k: [1, 2, null],
      x: [10, 20, 99],
      y: [100, 200, null],
    })
    const [inner] = run([left, right], {
      type: "joinByField",
      field: "k",
      mode: "inner",
    })
    expect(columns(inner)).toEqual({ k: [1, 2], x: [10, 20], y: [100, 200] })
  })

  it("keeps the name and labels of a single member, sorts it and moves the key first", () => {
    const only = frame([num("v", [3, 1, 2]), time([30, 10, 20])], {
      name: "cpu",
      labels: { pod: "a" },
    })
    const other = frame([str("s", ["x"])])
    const out = run([only, other], { type: "joinByField" })
    expect(out).toHaveLength(2)
    expect(out[0].name).toBe("cpu")
    expect(out[0].labels).toEqual({ pod: "a" })
    expect(out[0].fields.map((f) => f.name)).toEqual(["Time", "v"])
    expect(columns(out[0])).toEqual({ Time: [10, 20, 30], v: [1, 2, 3] })
    expect(out[1]).toEqual(other)
  })

  it("names colliding labelled fields by series name and stamps every field", () => {
    const x = frame([time([1]), num("Value", [1])], {
      name: "cpu",
      labels: { pod: "a" },
    })
    const y = frame([time([1]), num("Value", [2])], {
      name: "cpu",
      labels: { pod: "b" },
    })
    const [out] = run([x, y], { type: "joinByField" })
    expect(out.fields.map((f) => f.name)).toEqual([
      "Time",
      'cpu{pod="a"}',
      'cpu{pod="b"}',
    ])
    expect(out.fields.slice(1).map((f) => f.seriesName)).toEqual([
      'cpu{pod="a"}',
      'cpu{pod="b"}',
    ])
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
      includeRegex: "(Err.*|host)",
    })
    expect(out.fields.map((f) => f.name)).toEqual(["Errors", "host"])
  })

  it("keeps a field matched by either option", () => {
    const [out] = run([input], {
      type: "filterFieldsByName",
      include: ["Time"],
      includeRegex: "Err.*",
    })
    expect(out.fields.map((f) => f.name)).toEqual(["Time", "Errors"])
  })

  it("anchors a plain pattern and honours /re/flags", () => {
    const names = frame([num("cpu", [1]), num("cpu_usage", [2])])
    const kept = (includeRegex: string) =>
      run([names], {
        type: "filterFieldsByName",
        includeRegex,
      })[0]?.fields.map((f) => f.name)
    expect(kept("cpu")).toEqual(["cpu"])
    expect(kept("/cpu/")).toEqual(["cpu", "cpu_usage"])
    expect(kept("/CPU/i")).toEqual(["cpu", "cpu_usage"])
    expect(kept("/^CPU$/i")).toEqual(["cpu"])
    expect(kept("/cpu/g")).toEqual(["cpu", "cpu_usage"])
  })

  it("matches nothing for an invalid pattern", () => {
    expect(
      run([input], { type: "filterFieldsByName", includeRegex: "(" })
    ).toEqual([])
    const [out] = run([input], {
      type: "filterFieldsByName",
      include: ["Value"],
      includeRegex: "(",
    })
    expect(out.fields.map((f) => f.name)).toEqual(["Value"])
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

  it("joins frames that share a time field name before computing", () => {
    const stamped = prepareFrames(
      panel([
        { ref: "A", legend: "errors" },
        { ref: "B", legend: "total" },
      ]),
      [
        frame([time([1, 2]), num("Value", [1, 3])], { ref: "A" }),
        frame([time([1, 2]), num("Value", [10, 12])], { ref: "B" }),
      ]
    )
    const out = applyTransforms(stamped, [
      { type: "calculateField", alias: "ratio", expr: "errors / total" },
    ])
    expect(out).toHaveLength(1)
    expect(columns(out[0])).toEqual({
      Time: [1, 2],
      errors: [1, 3],
      total: [10, 12],
      ratio: [0.1, 0.25],
    })
  })

  it("computes per frame when the time fields have different names", () => {
    const left = frame([time([1], "Time"), num("a", [1])])
    const right = frame([time([1], "ts"), num("a", [2])])
    const out = run([left, right], {
      type: "calculateField",
      alias: "d",
      expr: "a * 2",
    })
    expect(out).toHaveLength(2)
    expect(out.map((f) => f.fields.at(-1)?.values)).toEqual([[2], [4]])
  })

  it("computes per frame when the right operand is a fixed number", () => {
    const left = frame([time([1]), num("a", [1])])
    const right = frame([time([1]), num("a", [2])])
    const out = run([left, right], {
      type: "calculateField",
      alias: "d",
      expr: "a * 100",
    })
    expect(out).toHaveLength(2)
    expect(out.map((f) => f.fields.at(-1)?.values)).toEqual([[100], [200]])
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

  it("sorts strings with collation rather than code points", () => {
    const names = frame([str("host", ["Zebra", "apple", null, "Mango"])])
    const [asc] = run([names], { type: "sortBy", field: "host" })
    expect(columns(asc).host).toEqual(["apple", "Mango", "Zebra", null])
    const [desc] = run([names], { type: "sortBy", field: "host", desc: true })
    expect(columns(desc).host).toEqual(["Zebra", "Mango", "apple", null])
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
