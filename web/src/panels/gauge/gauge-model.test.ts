import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import { createFieldResolver } from "@/panels/field-config"
import { prepareFrames, type PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  arcPath,
  gaugeLinks,
  gaugeModel,
  valueFontSize,
  type Gauge,
} from "./gauge-model"

beforeEach(() => {
  vi.stubGlobal("location", { origin: "https://observe.example.com" })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "g1",
    title: "Gauge",
    gridPos: { x: 0, y: 0, w: 6, h: 6 },
    type: "gauge",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

const time: PreparedFrame["fields"][number] = {
  name: "Time",
  type: "time",
  values: [1, 2, 3],
}

const number = (values: (number | null)[], name = "Value") => ({
  name,
  type: "number" as const,
  values,
})

function frame(
  fields: PreparedFrame["fields"],
  extra: Partial<Omit<WireFrame, "fields">> = {}
): PreparedFrame {
  return { ref: "A", fields, ...extra }
}

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {}
): Gauge[] {
  return gaugeModel(
    frames,
    createFieldResolver(panel(extra)),
    displayWithDefaults("gauge", display)
  )
}

const one = (
  values: (number | null)[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {}
): Gauge => build([frame([time, number(values)])], extra, display)[0]

const steps = [
  { color: "ok" },
  { value: 50, color: "warning" },
  { value: 80, color: "critical" },
] as const

describe("gauge value", () => {
  it("reduces with the default last-not-null reducer", () => {
    expect(one([1, 2, 3, null]).value).toBe(3)
  })

  it("reduces with the panel's reducer", () => {
    expect(one([1, 2, 6], {}, { reduce: "mean" }).value).toBe(3)
    expect(one([1, 9, 2], {}, { reduce: "max" }).value).toBe(9)
  })

  it("leaves a series with no number without a value", () => {
    const gauge = one([null, null])
    expect(gauge.value).toBeNull()
    expect(gauge.fraction).toBe(0)
    expect(gauge.text).toBe("-")
  })
})

describe("gauge set", () => {
  it("draws one gauge per visible number field of every frame", () => {
    const gauges = build([
      frame([time, number([1], "a"), number([2], "b")], { ref: "A" }),
      frame([time, number([3])], { ref: "B", labels: { pod: "x" } }),
    ])
    expect(gauges.map((gauge) => gauge.value)).toEqual([1, 2, 3])
    expect(new Set(gauges.map((gauge) => gauge.key)).size).toBe(3)
  })

  it("skips hidden fields and fields that are not numbers", () => {
    const text = { name: "Pod", type: "string" as const, values: ["a"] }
    const gauges = build(
      [frame([time, text, number([1], "shown"), number([2], "hidden")])],
      { overrides: [{ match: { field: "hidden" }, hidden: true }] }
    )
    expect(gauges.map((gauge) => gauge.field.name)).toEqual(["shown"])
  })

  it("keeps the key of two identical series distinct", () => {
    const same = frame([time, number([1])])
    const gauges = build([same, same])
    expect(gauges[0].key).not.toBe(gauges[1].key)
  })

  it("names a gauge by its display name", () => {
    const named = build([frame([time, number([1])])], {
      displayName: "Busy ${__field.name}",
    })
    expect(named[0].name).toBe("Busy Value")
  })

  it("names a gauge by the query legend", () => {
    const withLegend = panel({ queries: [{ query: "up", legend: "{{pod}}" }] })
    const frames = prepareFrames(withLegend, [
      frame([time, number([1])], { labels: { pod: "web-1" } }),
    ])
    const [gauge] = gaugeModel(
      frames,
      createFieldResolver(withLegend),
      displayWithDefaults("gauge", {})
    )
    expect(gauge.name).toBe("web-1")
  })
})

describe("gauge range", () => {
  it("starts at zero and ends at the largest value of the data", () => {
    const gauge = one([10, 40, 25])
    expect([gauge.min, gauge.max]).toEqual([0, 40])
    expect(gauge.fraction).toBeCloseTo(25 / 40)
  })

  it("starts below zero when the data does", () => {
    const gauge = one([-20, 30, 10])
    expect([gauge.min, gauge.max]).toEqual([-20, 30])
    expect(gauge.fraction).toBeCloseTo(30 / 50)
  })

  it("spans every visible number field, not only the shown value", () => {
    const gauges = build([
      frame([time, number([5, 90, 6], "a")]),
      frame([time, number([7], "b")], { ref: "B" }),
    ])
    expect(gauges.map((gauge) => [gauge.min, gauge.max])).toEqual([
      [0, 90],
      [0, 90],
    ])
  })

  it("ignores the values of hidden fields", () => {
    const gauges = build(
      [frame([time, number([5], "shown"), number([500], "hidden")])],
      { overrides: [{ match: { field: "hidden" }, hidden: true }] }
    )
    expect(gauges[0].max).toBe(5)
  })

  it("takes the range of a very long series without spreading it", () => {
    const values = Array.from({ length: 300_000 }, (_, index) => index)
    const gauge = one(values)
    expect([gauge.min, gauge.max]).toEqual([0, 299_999])
  })

  it("uses the field min and max over the data", () => {
    const gauge = one([10, 40], { min: 5, max: 200 })
    expect([gauge.min, gauge.max]).toEqual([5, 200])
    expect(gauge.fraction).toBeCloseTo(35 / 195)
  })

  it("lets an override change one gauge's range", () => {
    const gauges = build(
      [frame([time, number([50], "a"), number([50], "b")])],
      { overrides: [{ match: { field: "b" }, min: 0, max: 100 }] }
    )
    expect(gauges.map((gauge) => gauge.max)).toEqual([50, 100])
  })

  it("widens an empty range to one", () => {
    expect([one([0, 0]).min, one([0, 0]).max]).toEqual([0, 1])
    expect(one([5], { min: 5, max: 5 }).max).toBe(6)
    expect(one([5], { min: 10, max: 2 }).max).toBe(11)
  })

  it("has a range of 0 to 1 without any data", () => {
    const gauge = one([null])
    expect([gauge.min, gauge.max]).toEqual([0, 1])
  })

  it("clamps the drawn fraction into the range", () => {
    expect(one([150], { min: 0, max: 100 }).fraction).toBe(1)
    expect(one([-30], { min: 0, max: 100 }).fraction).toBe(0)
    expect(one([150], { min: 0, max: 100 }).value).toBe(150)
  })
})

describe("gauge text and color", () => {
  it("shares one decimal count across gauges of the same unit", () => {
    const gauges = build(
      [frame([time, number([2.4], "a"), number([2.45], "b")])],
      { unit: "short" }
    )
    expect(gauges.map((gauge) => gauge.text)).toEqual(["2.40", "2.45"])
  })

  it("keeps a field's own decimals", () => {
    const gauges = build(
      [frame([time, number([2.4], "a"), number([2.45], "b")])],
      { overrides: [{ match: { field: "a" }, decimals: 1 }] }
    )
    expect(gauges.map((gauge) => gauge.text)).toEqual(["2.4", "2.45"])
  })

  it("colors by threshold with its state", () => {
    const gauge = one([60], { thresholds: [...steps], max: 100 })
    expect(gauge.color).toBe("var(--threshold-warning)")
    expect(gauge.threshold).toMatchObject({
      level: "warning",
      label: "Warning",
    })
  })

  it("colors without a threshold state when none is set", () => {
    const gauge = one([60], { color: "info" })
    expect(gauge.color).toBe("var(--threshold-info)")
    expect(gauge.threshold.level).toBe("base")
  })

  it("judges percentage thresholds against the drawn range", () => {
    const gauge = one([34], {
      thresholdMode: "percentage",
      thresholds: [...steps],
      min: 0,
      max: 40,
    })
    expect(gauge.threshold.level).toBe("critical")
  })

  it("shows a mapping's text and color instead of the threshold state", () => {
    const gauge = one([100], {
      thresholds: [...steps],
      mappings: [{ value: "100", text: "Full", color: "info" }],
    })
    expect(gauge.text).toBe("Full")
    expect(gauge.color).toBe("var(--threshold-info)")
    expect(gauge.threshold.level).toBe("base")
  })

  it("keeps the threshold state for a mapping without a color", () => {
    const gauge = one([100], {
      thresholds: [...steps],
      mappings: [{ value: "100", text: "Full" }],
    })
    expect(gauge.text).toBe("Full")
    expect(gauge.threshold.level).toBe("critical")
  })

  it("uses the field's no-value text when there is no value", () => {
    expect(one([null], { noValue: "n/a" }).text).toBe("n/a")
  })
})

describe("gauge markers", () => {
  it("splits the ring into one segment per threshold step", () => {
    const gauge = one([10], { thresholds: [...steps], min: 0, max: 100 })
    expect(gauge.markers).toEqual([
      { from: 0, to: 50, color: "var(--threshold-ok)" },
      { from: 50, to: 80, color: "var(--threshold-warning)" },
      { from: 80, to: 100, color: "var(--threshold-critical)" },
    ])
  })

  it("starts the base step at the range minimum", () => {
    const gauge = one([10], { thresholds: [...steps], min: -20, max: 100 })
    expect(gauge.markers[0]).toMatchObject({ from: -20, to: 50 })
  })

  it("maps percentage steps through the range", () => {
    const gauge = one([10], {
      thresholdMode: "percentage",
      thresholds: [...steps],
      min: 100,
      max: 300,
    })
    expect(gauge.markers.map(({ from, to }) => [from, to])).toEqual([
      [100, 200],
      [200, 260],
      [260, 300],
    ])
  })

  it("clips steps to the range and drops those outside it", () => {
    const gauge = one([10], {
      thresholds: [
        { color: "ok" },
        { value: -5, color: "info" },
        { value: 50, color: "warning" },
        { value: 500, color: "critical" },
      ],
      min: 0,
      max: 100,
    })
    expect(gauge.markers.map(({ from, to }) => [from, to])).toEqual([
      [0, 50],
      [50, 100],
    ])
  })

  it("draws no ring when thresholdMarkers is false", () => {
    const gauge = one(
      [10],
      { thresholds: [...steps] },
      { thresholdMarkers: false }
    )
    expect(gauge.markers).toEqual([])
  })

  it("draws no ring without thresholds", () => {
    expect(one([10]).markers).toEqual([])
  })

  it("keeps a custom step color as written", () => {
    const gauge = one([10], {
      thresholds: [{ color: "#112233" }, { value: 5, color: "#445566" }],
      min: 0,
      max: 10,
    })
    expect(gauge.markers.map((marker) => marker.color)).toEqual([
      "#112233",
      "#445566",
    ])
  })
})

describe("gauge links", () => {
  const dataLink = {
    title: "${__field.labels.pod} at ${__value.text}",
    url: "/pods?pod=${__field.labels.pod}&series=${__series.name}&last=${__data.fields.Value}",
  }

  function links(extra: Partial<DataPanel>) {
    const fields = createFieldResolver(panel(extra))
    const [gauge] = gaugeModel(
      [frame([time, number([4, 7])], { labels: { pod: "web-1" } })],
      fields,
      displayWithDefaults("gauge", {})
    )
    return gaugeLinks(gauge, fields, { vars: {}, fields })
  }

  it("resolves data links with the frame labels, the value and the last row", () => {
    expect(links({ links: [dataLink] })).toEqual([
      {
        title: "web-1 at 7",
        href: "/pods?pod=web-1&series=%7Bpod%3D%22web-1%22%7D&last=7",
        newTab: false,
      },
    ])
  })

  it("leaves links that do not use data tokens to the panel header", () => {
    expect(links({ links: [{ title: "Docs", url: "/docs" }] })).toEqual([])
  })

  it("takes links from the field's own config", () => {
    const fields = createFieldResolver(
      panel({
        overrides: [
          {
            match: { field: "b" },
            links: [{ title: "B ${__value.raw}", url: "/b" }],
          },
        ],
      })
    )
    const gauges = gaugeModel(
      [frame([time, number([1], "a"), number([2], "b")])],
      fields,
      displayWithDefaults("gauge", {})
    )
    const titles = gauges.map((gauge) =>
      gaugeLinks(gauge, fields, { vars: {}, fields }).map((link) => link.title)
    )
    expect(titles).toEqual([[], ["B 2"]])
  })
})

describe("arcPath", () => {
  it("sweeps 240 degrees from the lower left to the lower right", () => {
    expect(arcPath(0, 1, 40)).toBe("M 15.359 70 A 40 40 0 1 1 84.641 70")
  })

  it("marks an arc of more than half the sweep as a large arc", () => {
    expect(arcPath(0, 0.5, 40)).toBe("M 15.359 70 A 40 40 0 0 1 50 10")
    expect(arcPath(0, 0.75, 40)).toContain("A 40 40 0 0 1")
    expect(arcPath(0, 0.8, 40)).toContain("A 40 40 0 1 1")
  })

  it("starts a middle arc where the previous one ended", () => {
    const first = arcPath(0, 0.5, 40)
    const second = arcPath(0.5, 1, 40)
    expect(first.split(" ").slice(-2)).toEqual(second.split(" ").slice(1, 3))
  })

  it("draws nothing for an empty or reversed span", () => {
    expect(arcPath(0.4, 0.4, 40)).toBe("")
    expect(arcPath(0.8, 0.2, 40)).toBe("")
  })

  it("clamps fractions into the sweep", () => {
    expect(arcPath(-1, 2, 40)).toBe(arcPath(0, 1, 40))
  })
})

describe("valueFontSize", () => {
  it("shrinks as the text grows", () => {
    expect(valueFontSize("1234567890")).toBeLessThan(valueFontSize("1234"))
  })

  it("caps the size of a short text", () => {
    expect(valueFontSize("7")).toBe(valueFontSize("777"))
    expect(valueFontSize("7")).toBeLessThanOrEqual(24)
  })
})
