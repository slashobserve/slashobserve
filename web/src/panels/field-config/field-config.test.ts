import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { WireField } from "@/panels/frames"
import { prepareFrames } from "@/panels/transforms"
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
      style: "line",
      fill: 0,
      lineStyle: "solid",
      stack: "none",
      axis: "left",
      hidden: false,
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
      'app="web", pod="a" | up{app="web", pod="a"} | ${__field.labels.x}'
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

  it("returns the same config object for the same field", () => {
    const f = frame([time, value()])
    const resolver = createFieldResolver(panel())
    expect(resolver.configFor(f, f.fields[1])).toBe(
      resolver.configFor(f, f.fields[1])
    )
  })
})
