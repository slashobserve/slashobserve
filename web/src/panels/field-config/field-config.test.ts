import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { WireField } from "@/panels/frames"
import { prepareFrames, type PreparedFrame } from "@/panels/transforms"
import type { DataPanel } from "@/spec"

import { createFieldResolver } from "."

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "Panel",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "timeseries",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

const time: WireField = { name: "Time", type: "time", values: [1, 2] }
const value = (name = "Value"): WireField => ({
  name,
  type: "number",
  values: [1, 2],
})

function frame(
  fields: WireField[],
  extra: Partial<Omit<WireFrame, "fields">> = {}
): WireFrame {
  return { ref: "A", fields, ...extra }
}

describe("field config resolution", () => {
  it("falls back to the shared defaults when the panel sets nothing", () => {
    const f = frame([time, value()])
    const config = createFieldResolver(panel()).configFor(f, f.fields[1])
    expect(config).toEqual({
      thresholdMode: "absolute",
      colorBy: "last",
      style: "line",
      fill: 0,
      lineStyle: "solid",
      lineInterpolation: "linear",
      lineWidth: 2,
      points: "auto",
      pointSize: 4,
      pointShape: "circle",
      pointStrokeWidth: 0,
      spanNulls: false,
      stack: "none",
      axis: "left",
      hidden: false,
      scale: "linear",
      logBase: 10,
      axisCenteredZero: false,
      axisBorder: false,
      axisColor: "text",
      hideInLegend: false,
      hideInTooltip: false,
      align: "auto",
      filterable: false,
      inspect: false,
      hideHeader: false,
      stackGroup: "A",
      nullAsZero: false,
      barAlignment: "center",
      barWidthFactor: 0.6,
      barMaxWidth: 64,
      thresholdsStyle: "dashed",
      mappings: [],
      links: [],
    })
  })

  it("keeps the panel's own field props", () => {
    const f = frame([time, value()])
    const config = createFieldResolver(
      panel({ unit: "bytes", fill: 30, stack: "normal" })
    ).configFor(f, f.fields[1])
    expect(config).toMatchObject({ unit: "bytes", fill: 30, stack: "normal" })
    expect(config.style).toBe("line")
  })

  it("tells a written prop from a default, wherever it was written", () => {
    const a = frame([time, value()], { ref: "A" })
    const b = frame([time, value()], { ref: "B" })
    const resolver = createFieldResolver(
      panel({
        fill: 0,
        overrides: [{ match: { query: "B" }, lineWidth: 2 }],
      })
    )
    expect(resolver.isSet(a, a.fields[1], "fill")).toBe(true)
    expect(resolver.isSet(a, a.fields[1], "lineWidth")).toBe(false)
    expect(resolver.isSet(b, b.fields[1], "lineWidth")).toBe(true)
    expect(resolver.isSet(b, b.fields[1], "spanNulls")).toBe(false)
  })

  it("tells a written color mode and color-by from their defaults in the panel, the field and an override", () => {
    const a = frame([time, value()], { ref: "A" })
    const b = frame([time, value()], { ref: "B" })
    const c: PreparedFrame = {
      ref: "C",
      fields: [time, { ...value(), config: { colorBy: "max" } }],
    }
    const resolver = createFieldResolver(
      panel({
        colorMode: "paletteByName",
        overrides: [
          {
            match: { query: "B" },
            colorMode: "scheme",
            colorScheme: "blues",
            colorBy: "last",
          },
        ],
      })
    )
    const cases = [
      { subject: a, key: "colorMode", expected: true },
      { subject: a, key: "colorBy", expected: false },
      { subject: a, key: "colorScheme", expected: false },
      { subject: b, key: "colorScheme", expected: true },
      { subject: b, key: "colorBy", expected: true },
      { subject: c, key: "colorBy", expected: true },
      { subject: c, key: "colorScheme", expected: false },
    ] as const
    for (const { subject, key, expected } of cases) {
      expect(resolver.isSet(subject, subject.fields[1], key)).toBe(expected)
    }
    expect(resolver.configFor(b, b.fields[1])).toMatchObject({
      colorMode: "scheme",
      colorScheme: "blues",
      colorBy: "last",
    })
  })

  it("matches query by the frame ref", () => {
    const a = frame([time, value()], { ref: "A" })
    const b = frame([time, value()], { ref: "B" })
    const resolver = createFieldResolver(
      panel({ overrides: [{ match: { query: "B" }, unit: "ms" }] })
    )
    expect(resolver.configFor(a, a.fields[1]).unit).toBeUndefined()
    expect(resolver.configFor(b, b.fields[1]).unit).toBe("ms")
  })

  it("matches series by the series name", () => {
    const f = frame([time, value()], { name: "up", labels: { pod: "a" } })
    const resolver = createFieldResolver(
      panel({
        overrides: [{ match: { series: 'up{pod="a"}' }, color: "critical" }],
      })
    )
    expect(resolver.configFor(f, f.fields[1]).color).toBe("critical")
  })

  it("matches a series name stamped by the query legend", () => {
    const withLegend = panel({
      queries: [{ query: "up", legend: "{{pod}}" }],
      overrides: [{ match: { series: "web-1" }, unit: "percent" }],
    })
    const [prepared] = prepareFrames(withLegend, [
      frame([time, value()], { name: "up", labels: { pod: "web-1" } }),
    ])
    const resolver = createFieldResolver(withLegend)
    expect(resolver.seriesName(prepared, prepared.fields[1])).toBe("web-1")
    expect(resolver.configFor(prepared, prepared.fields[1]).unit).toBe(
      "percent"
    )
  })

  it("anchors seriesRegex unless written with slashes", () => {
    const f = frame([time, value()], { name: "cpu_usage" })
    const anchored = createFieldResolver(
      panel({ overrides: [{ match: { seriesRegex: "cpu" }, unit: "s" }] })
    )
    const slashed = createFieldResolver(
      panel({ overrides: [{ match: { seriesRegex: "/cpu/" }, unit: "s" }] })
    )
    expect(anchored.configFor(f, f.fields[1]).unit).toBeUndefined()
    expect(slashed.configFor(f, f.fields[1]).unit).toBe("s")
  })

  it("matches seriesRegex written with the sticky flag on every field", () => {
    const f = frame([time, value("a"), value("b")], { name: "cpu" })
    const resolver = createFieldResolver(
      panel({ overrides: [{ match: { seriesRegex: "/cpu/y" }, unit: "s" }] })
    )
    expect(resolver.configFor(f, f.fields[1]).unit).toBe("s")
    expect(resolver.configFor(f, f.fields[2]).unit).toBe("s")
  })

  it("never matches an invalid seriesRegex", () => {
    const f = frame([time, value()], { name: "cpu" })
    const resolver = createFieldResolver(
      panel({ overrides: [{ match: { seriesRegex: "(" }, unit: "s" }] })
    )
    expect(resolver.configFor(f, f.fields[1]).unit).toBeUndefined()
  })

  it("matches field by name or by series name", () => {
    const f = frame([time, value("Value")], {
      name: "up",
      labels: { pod: "a" },
    })
    const byName = createFieldResolver(
      panel({ overrides: [{ match: { field: "Value" }, unit: "ms" }] })
    )
    const bySeriesName = createFieldResolver(
      panel({ overrides: [{ match: { field: 'up{pod="a"}' }, unit: "ms" }] })
    )
    const miss = createFieldResolver(
      panel({ overrides: [{ match: { field: "Nope" }, unit: "ms" }] })
    )
    expect(byName.configFor(f, f.fields[1]).unit).toBe("ms")
    expect(bySeriesName.configFor(f, f.fields[1]).unit).toBe("ms")
    expect(miss.configFor(f, f.fields[1]).unit).toBeUndefined()
  })

  it("matches field before the panel display name renames it", () => {
    const f = frame([time, value()], { name: "up", labels: { pod: "a" } })
    const resolver = createFieldResolver(
      panel({
        displayName: "${__field.labels.pod}",
        overrides: [
          { match: { field: 'up{pod="a"}' }, unit: "ms" },
          { match: { field: "a" }, unit: "s" },
        ],
      })
    )
    expect(resolver.displayName(f, f.fields[1])).toBe("a")
    expect(resolver.configFor(f, f.fields[1]).unit).toBe("ms")
  })

  it("matches fieldType, mapping bool to boolean and never matching other", () => {
    const flag: WireField = { name: "up", type: "bool", values: [true] }
    const other: WireField = { name: "raw", type: "other", values: [{}] }
    const f = frame([time, value(), flag, other])
    const resolver = createFieldResolver(
      panel({
        overrides: [
          { match: { fieldType: "boolean" }, noValue: "bool" },
          { match: { fieldType: "number" }, noValue: "num" },
        ],
      })
    )
    expect(resolver.configFor(f, flag).noValue).toBe("bool")
    expect(resolver.configFor(f, f.fields[1]).noValue).toBe("num")
    expect(resolver.configFor(f, other).noValue).toBeUndefined()
    expect(resolver.configFor(f, f.fields[0]).noValue).toBeUndefined()
  })

  it("matches a number field by its reduced value", () => {
    const low: WireField = { name: "low", type: "number", values: [1, 2, 3] }
    const high: WireField = { name: "high", type: "number", values: [50, 60] }
    const empty: WireField = { name: "empty", type: "number", values: [null] }
    const label: WireField = { name: "text", type: "string", values: ["9"] }
    const f = frame([time, low, high, empty, label])
    const hit = (op: string, value: number, reducer = "max") =>
      createFieldResolver(
        panel({
          overrides: [
            {
              match: { value: { reducer, op, value } },
              noValue: "hit",
            },
          ],
        } as Partial<DataPanel>)
      )
    const matched = (op: string, value: number, reducer?: string) => {
      const resolver = hit(op, value, reducer)
      return [low, high, empty, label].filter(
        (field) => resolver.configFor(f, field).noValue === "hit"
      )
    }
    expect(matched("gt", 3)).toEqual([high])
    expect(matched("gte", 3)).toEqual([low, high])
    expect(matched("lt", 3)).toEqual([])
    expect(matched("lte", 3)).toEqual([low])
    expect(matched("eq", 3)).toEqual([low])
    expect(matched("neq", 3)).toEqual([high])
    expect(matched("gt", 50, "min")).toEqual([])
    expect(matched("eq", 3, "count")).toEqual([low])
  })

  it("resolves the series-drawing props with their defaults", () => {
    const f = frame([time, value()])
    const plain = createFieldResolver(panel()).configFor(f, f.fields[1])
    expect(plain).toMatchObject({
      hideInLegend: false,
      hideInTooltip: false,
      align: "auto",
      filterable: false,
      inspect: false,
      hideHeader: false,
      stackGroup: "A",
    })
    expect(plain.valueTransform).toBeUndefined()
    expect(plain.fillBelowTo).toBeUndefined()
    const resolver = createFieldResolver(
      panel({
        overrides: [
          {
            match: { fieldType: "number" },
            valueTransform: "negativeY",
            hideInLegend: true,
            hideInTooltip: true,
            stackGroup: "B",
            fillBelowTo: "other",
          },
        ],
      })
    )
    expect(resolver.configFor(f, f.fields[1])).toMatchObject({
      valueTransform: "negativeY",
      hideInLegend: true,
      hideInTooltip: true,
      stackGroup: "B",
      fillBelowTo: "other",
    })
  })

  it("carries color, cell and series-drawing props through one override", () => {
    const f = frame([time, value()])
    const resolver = createFieldResolver(
      panel({
        colorMode: "scheme",
        colorScheme: "blues",
        colorBy: "min",
        overrides: [
          {
            match: { fieldType: "number" },
            colorMode: "scheme",
            colorScheme: "reds",
            colorBy: "max",
            cell: { type: "background", wholeRow: true },
            valueTransform: "constant",
            hideInLegend: true,
            hideInTooltip: true,
            stackGroup: "B",
            fillBelowTo: "other",
          },
        ],
      })
    )
    expect(resolver.configFor(f, f.fields[1])).toMatchObject({
      colorMode: "scheme",
      colorScheme: "reds",
      colorBy: "max",
      cell: { type: "background", wholeRow: true },
      valueTransform: "constant",
      hideInLegend: true,
      hideInTooltip: true,
      stackGroup: "B",
      fillBelowTo: "other",
    })
  })

  it("applies overrides in order so the later one wins", () => {
    const f = frame([time, value()])
    const resolver = createFieldResolver(
      panel({
        unit: "bytes",
        overrides: [
          { match: { fieldType: "number" }, unit: "ms", decimals: 1 },
          { match: { query: "A" }, unit: "s" },
        ],
      })
    )
    expect(resolver.configFor(f, f.fields[1])).toMatchObject({
      unit: "s",
      decimals: 1,
    })
  })

  it("replaces arrays instead of merging them", () => {
    const f = frame([time, value()])
    const resolver = createFieldResolver(
      panel({
        thresholds: [{ color: "ok" }, { value: 80, color: "critical" }],
        mappings: [{ value: "0", text: "Down" }],
        links: [{ title: "A", url: "/a" }],
        overrides: [
          {
            match: { fieldType: "number" },
            thresholds: [{ color: "info" }],
            mappings: [{ value: "1", text: "Up" }],
            links: [{ title: "B", url: "/b" }],
          },
        ],
      })
    )
    expect(resolver.configFor(f, f.fields[1])).toMatchObject({
      thresholds: [{ color: "info" }],
      mappings: [{ value: "1", text: "Up" }],
      links: [{ title: "B", url: "/b" }],
    })
  })

  it("resolves the display name from the series name or the template", () => {
    const f = frame([time, value()], { name: "up", labels: { pod: "a" } })
    expect(createFieldResolver(panel()).displayName(f, f.fields[1])).toBe(
      'up{pod="a"}'
    )
    const resolver = createFieldResolver(
      panel({ displayName: "Pod ${__field.labels.pod} (${__field.name})" })
    )
    expect(resolver.displayName(f, f.fields[1])).toBe("Pod a (Value)")
  })

  it("interpolates all labels, the series name and keeps unknown tokens", () => {
    const f = frame([time, value()], {
      name: "up",
      labels: { pod: "a", app: "web" },
    })
    const resolver = createFieldResolver(
      panel({
        displayName:
          "${__field.labels} | ${__series.name} | ${__field.labels.x}",
      })
    )
    expect(resolver.displayName(f, f.fields[1])).toBe(
      'pod="a", app="web" | up{app="web", pod="a"} | ${__field.labels.x}'
    )
  })

  it("keeps a label token that names an inherited property as written", () => {
    const f = frame([time, value()], { labels: { pod: "a" } })
    const resolver = createFieldResolver(
      panel({ displayName: "${__field.labels.constructor}" })
    )
    expect(resolver.displayName(f, f.fields[1])).toBe(
      "${__field.labels.constructor}"
    )
  })

  it("writes all labels in insertion order without escaping quotes", () => {
    const f = frame([time, value()], { labels: { b: 'x"y', a: "1" } })
    const resolver = createFieldResolver(
      panel({ displayName: "${__field.labels}" })
    )
    expect(resolver.displayName(f, f.fields[1])).toBe('b="x"y", a="1"')
  })

  it("lets an override set the display name", () => {
    const f = frame([time, value()], { labels: { pod: "a" } })
    const resolver = createFieldResolver(
      panel({
        overrides: [
          { match: { fieldType: "number" }, displayName: "${__field.name}!" },
        ],
      })
    )
    expect(resolver.displayName(f, f.fields[1])).toBe("Value!")
  })

  it("applies config carried by the field under the panel's overrides", () => {
    const carrying: PreparedFrame = {
      ref: "A",
      fields: [
        time,
        { ...value(), config: { unit: "bytes", max: 80, min: 1 } },
      ],
    }
    const resolver = createFieldResolver(
      panel({
        max: 10,
        overrides: [{ match: { fieldType: "number" }, min: 5 }],
      })
    )
    const config = resolver.configFor(carrying, carrying.fields[1])
    expect(config).toMatchObject({ unit: "bytes", max: 80, min: 5 })
    expect(resolver.configFor(carrying, carrying.fields[0]).max).toBe(10)
  })

  it("returns the same config object for the same field", () => {
    const f = frame([time, value()])
    const resolver = createFieldResolver(panel())
    expect(resolver.configFor(f, f.fields[1])).toBe(
      resolver.configFor(f, f.fields[1])
    )
  })
})
