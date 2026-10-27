import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel } from "@/spec"

import { applyTransforms } from "."
import { stampSeriesNames, type DatasourceTypeOf } from "./series-names"

type Query = {
  query?: string
  format?: string
  legend?: string
  ref?: string
  datasource?: string
}

function panel(queries: Query[] = [{}]): DataPanel {
  return {
    id: "p",
    title: "p",
    type: "timeseries",
    gridPos: { x: 0, y: 0, w: 6, h: 4 },
    datasource: "ds",
    queries: queries.map((query) => ({ query: "q", ...query })),
  } as unknown as DataPanel
}

const time: WireFrame["fields"][number] = {
  name: "Time",
  type: "time",
  values: [1, 2],
}
const number = (name: string): WireFrame["fields"][number] => ({
  name,
  type: "number",
  values: [1, 2],
})

function frame(
  extra: Partial<Omit<WireFrame, "fields">>,
  ...names: string[]
): WireFrame {
  return { ref: "A", ...extra, fields: [time, ...names.map(number)] }
}

function names(
  queries: Query[],
  frames: WireFrame[],
  type?: string
): string[][] {
  const resolver: DatasourceTypeOf | undefined = type ? () => type : undefined
  return stampSeriesNames(panel(queries), frames, resolver).map((out) =>
    out.fields.filter((field) => field.seriesName).map((f) => f.seriesName!)
  )
}

const expr = "sum(rate(x[5m]))"
const pods = [
  frame({ labels: { pod: "checkout-0" } }, "Value"),
  frame({ labels: { pod: "checkout-1" } }, "Value"),
]

describe("Prometheus series names", () => {
  const bare = [frame({}, "Value")]

  it("names an unlabelled series after the query text", () => {
    for (const legend of [undefined, "__auto", "{}"]) {
      expect(names([{ query: expr, legend }], bare, "prometheus")).toEqual([
        [expr],
      ])
    }
  })

  it("falls through to Value when a template renders empty", () => {
    expect(names([{ legend: "{{pod}}" }], bare, "prometheus")).toEqual([
      ["Value"],
    ])
    expect(
      names([{ legend: "{{constructor}}" }], [pods[0]], "prometheus")
    ).toEqual([["checkout-0"]])
  })

  it("shows the verbose label set without a legend", () => {
    expect(names([{}], pods, "prometheus")).toEqual([
      ['{pod="checkout-0"}'],
      ['{pod="checkout-1"}'],
    ])
  })

  it("names Auto series by their unique label value", () => {
    expect(names([{ legend: "__auto" }], pods, "prometheus")).toEqual([
      ["checkout-0"],
      ["checkout-1"],
    ])
    const quoted = [
      frame({ labels: { path: 'a"b' } }, "Value"),
      frame({ labels: { path: "c" } }, "Value"),
    ]
    expect(names([{ legend: "__auto" }], quoted, "prometheus")[0]).toEqual([
      'a"b',
    ])
  })

  it("renders custom templates and empties missing labels", () => {
    expect(names([{ legend: "{{pod}}" }], pods, "prometheus")[0]).toEqual([
      "checkout-0",
    ])
    expect(
      names([{ legend: "pod {{ pod }} / {{missing}}" }], pods, "prometheus")[0]
    ).toEqual(["pod checkout-0 / "])
  })

  it("puts the metric name first and sorts the label set", () => {
    const up = [
      frame({ name: "up", labels: { job: "a", instance: "x" } }, "Value"),
    ]
    expect(names([{}], up, "prometheus")).toEqual([
      ['up{instance="x", job="a"}'],
    ])
    expect(names([{ legend: "__auto" }], up, "prometheus")).toEqual([
      ['{__name__="up", instance="x", job="a"}'],
    ])
    expect(
      names([{ legend: "{{__name__}} {{job}}" }], up, "prometheus")
    ).toEqual([["up a"]])
  })

  it("names a metric without labels after the metric", () => {
    const up = [frame({ name: "up" }, "Value")]
    expect(names([{}], up, "prometheus")).toEqual([["up"]])
    expect(names([{ legend: "__auto" }], up, "prometheus")).toEqual([["up"]])
  })

  it("quotes label values like Go and sorts the formatted pairs", () => {
    expect(
      names(
        [{}],
        [frame({ labels: { path: 'a"b\\c\n' } }, "Value")],
        "prometheus"
      )
    ).toEqual([['{path="a\\"b\\\\c\\n"}']])
    expect(
      names(
        [{}],
        [frame({ labels: { a: "1", a1: "2" } }, "Value")],
        "prometheus"
      )
    ).toEqual([['{a1="2", a="1"}']])
  })

  it("keeps the metric name before the label set for a metric series", () => {
    expect(
      names(
        [{}],
        [frame({ name: "checkout-0", labels: { pod: "checkout-0" } }, "Value")],
        "prometheus"
      )
    ).toEqual([['checkout-0{pod="checkout-0"}']])
  })
})

describe("SQL series names", () => {
  const byService = [
    frame({ labels: { service: "api" } }, "c"),
    frame({ labels: { service: "web" } }, "c"),
  ]

  it("names a series by its column and single label value", () => {
    expect(names([{}], byService, "clickhouse")).toEqual([["c api"], ["c web"]])
  })

  it("shows the label set when there are several label keys", () => {
    expect(
      names(
        [{}],
        [frame({ labels: { method: "GET", service: "api" } }, "c")],
        "clickhouse"
      )
    ).toEqual([['c {method="GET", service="api"}']])
  })

  it("names unlabelled columns by their own name and suffixes labelled ones", () => {
    expect(names([{}], [frame({}, "c", "p95")], "clickhouse")).toEqual([
      ["c", "p95"],
    ])
    expect(
      names(
        [{}],
        [frame({ labels: { service: "api" } }, "c", "p95")],
        "clickhouse"
      )
    ).toEqual([["c api", "p95 api"]])
  })

  it("names a lone Value column after its query ref", () => {
    expect(names([{}], [frame({}, "Value")], "clickhouse")).toEqual([["A"]])
  })

  it("qualifies with the ref when frame names differ", () => {
    const frames = [
      frame({ labels: { service: "api" } }, "c"),
      frame({ ref: "B", labels: { service: "api" } }, "c"),
    ]
    expect(names([{}, {}], frames, "clickhouse")).toEqual([
      ["A c api"],
      ["B c api"],
    ])
  })

  it("uses a legend template and the column suffix", () => {
    expect(
      names([{ legend: "{{service}}" }], [byService[0]], "clickhouse")
    ).toEqual([["api"]])
    expect(
      names(
        [{ legend: "{{service}}" }],
        [frame({ labels: { service: "api" } }, "c", "p95")],
        "clickhouse"
      )
    ).toEqual([["api c", "api p95"]])
  })

  it("skips an empty label value", () => {
    expect(
      names(
        [{}],
        [frame({ labels: { service: "" } }, "c"), byService[1]],
        "clickhouse"
      )
    ).toEqual([["c"], ["c web"]])
  })

  it("numbers duplicate column names", () => {
    expect(names([{}], [frame({}, "x", "x")], "clickhouse")).toEqual([
      ["x 1", "x 2"],
    ])
  })

  it("stamps only number fields of a raw table", () => {
    const raw: WireFrame = {
      ref: "A",
      fields: [
        { name: "service", type: "string", values: ["api"] },
        { name: "n", type: "number", values: [1] },
      ],
    }
    const [out] = stampSeriesNames(panel(), [raw], () => "clickhouse")
    expect(out.fields.map((field) => field.seriesName)).toEqual([
      undefined,
      "n",
    ])
  })
})

describe("mixed panels", () => {
  it("names each frame by its own datasource type", () => {
    const frames = [
      frame({ labels: { pod: "checkout-0" } }, "Value"),
      frame({ ref: "B", labels: { service: "api" } }, "c"),
    ]
    const out = stampSeriesNames(
      panel([{}, { datasource: "ch" }]),
      frames,
      (uid) => (uid === "ch" ? "clickhouse" : "prometheus")
    )
    expect(out.map((f) => f.fields[1].seriesName)).toEqual([
      '{pod="checkout-0"}',
      'B c {service="api"}',
    ])
  })

  it("ignores table and logs queries and leaves their fields unnamed", () => {
    const frames = [
      frame({ labels: { pod: "x" } }, "Value"),
      frame({ ref: "B", labels: { service: "api" } }, "c"),
      frame({ ref: "C", labels: { host: "h" } }, "c"),
    ]
    const out = stampSeriesNames(
      panel([{}, { format: "table" }, { format: "logs" }]),
      frames,
      () => "prometheus"
    )
    expect(out[0].fields[1].seriesName).toBe('{pod="x"}')
    expect(out[1].fields[1].seriesName).toBeUndefined()
    expect(out[2].fields[1].seriesName).toBeUndefined()
    const sql = stampSeriesNames(
      panel([{}, { format: "table" }]),
      [frame({ labels: { service: "api" } }, "c"), frames[1]],
      () => "clickhouse"
    )
    expect(sql[0].fields[1].seriesName).toBe("c api")
  })

  it("does not mutate its input", () => {
    const input = frame({ labels: { service: "api" } }, "c")
    const out = stampSeriesNames(panel(), [input], () => "clickhouse")
    expect(out[0]).not.toBe(input)
    expect(input.fields[1]).not.toHaveProperty("seriesName")
  })

  it("lets organize rename a Grafana series name", () => {
    const stamped = stampSeriesNames(
      panel(),
      [frame({ labels: { service: "api" } }, "c")],
      () => "clickhouse"
    )
    const [renamed] = applyTransforms(stamped, [
      { type: "organize", rename: { "c api": "API" } },
    ])
    expect(renamed.fields[1].seriesName).toBe("API")
  })
})

describe("without datasource types", () => {
  it("names a frame with a metric name or a Time and Value shape as Prometheus", () => {
    expect(names([{ query: expr }], [frame({}, "Value")])).toEqual([[expr]])
    expect(names([{}], [frame({ name: "up" }, "c")])).toEqual([["up"]])
  })

  it("names any other shape as SQL", () => {
    expect(names([{}], [frame({ labels: { service: "api" } }, "c")])).toEqual([
      ["c api"],
    ])
  })
})
