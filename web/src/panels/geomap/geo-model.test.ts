import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  geoModel,
  initialView,
  maxMarkers,
  noLocationReason,
  skippedText,
  viewOptions,
} from "./geo-model"
import { fitBounds, project, worldView } from "./projection"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "g1",
    title: "Geo",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "geomap",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

type Column = { name: string; type?: "number" | "string"; values: unknown[] }

function table(...columns: Column[]): PreparedFrame {
  return {
    ref: "A",
    labels: {},
    fields: columns.map(({ name, type, values }) => ({
      name,
      type: type ?? "number",
      values,
    })),
  } as PreparedFrame
}

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {}
) {
  return geoModel(
    frames,
    createFieldResolver(panel(extra)),
    displayWithDefaults("geomap", display)
  )
}

const cities = (extra: Column[] = []) =>
  table(
    { name: "lat", values: [48.85, 35.68, -33.87] },
    { name: "lon", values: [2.35, 139.69, 151.21] },
    ...extra
  )

describe("location fields", () => {
  it("finds lat and lon by name", () => {
    const model = build([cities()])
    expect(model.reason).toBeNull()
    expect(model.markers).toHaveLength(3)
    const paris = project(2.35, 48.85)
    const marker = model.markers.find((m) => m.label === "48.850, 2.350")
    expect(marker?.x).toBeCloseTo(paris.x, 10)
    expect(marker?.y).toBeCloseTo(paris.y, 10)
  })

  it("accepts latitude, lng and longitude spellings in any case", () => {
    const model = build([
      table({ name: "Latitude", values: [10] }, { name: "LNG", values: [20] }),
    ])
    expect(model.markers).toHaveLength(1)
  })

  it("reads numeric text such as Prometheus labels", () => {
    const model = build([
      table(
        { name: "lat", type: "string", values: ["10.5"] },
        { name: "lon", type: "string", values: ["20.25"] }
      ),
    ])
    expect(model.markers).toHaveLength(1)
  })

  it("decodes a geohash field", () => {
    const model = build([
      table({ name: "geohash", type: "string", values: ["u4pruydqqvj"] }),
    ])
    const point = project(10.40744, 57.64911)
    expect(model.markers[0].x).toBeCloseTo(point.x, 4)
    expect(model.markers[0].y).toBeCloseTo(point.y, 4)
  })

  it("prefers explicitly named fields over auto detection", () => {
    const frame = table(
      { name: "lat", values: [0] },
      { name: "lon", values: [0] },
      { name: "y", values: [50] },
      { name: "x", values: [10] }
    )
    const model = build([frame], {}, { latField: "y", lonField: "x" })
    expect(model.markers[0].label).toBe("50.000, 10.000")
  })

  it("uses the geohash field when only that is named", () => {
    const frame = table(
      { name: "lat", values: [0] },
      { name: "lon", values: [0] },
      { name: "cell", type: "string", values: ["ezs42"] }
    )
    const model = build([frame], {}, { geohashField: "cell" })
    expect(model.markers[0].label).toBe("42.605, -5.603")
  })

  it("follows the location option", () => {
    const frame = table(
      { name: "lat", values: [1] },
      { name: "lon", values: [2] },
      { name: "geohash", type: "string", values: ["ezs42"] }
    )
    expect(build([frame], {}, { location: "geohash" }).markers[0].label).toBe(
      "42.605, -5.603"
    )
    expect(build([frame], {}, { location: "coords" }).markers[0].label).toBe(
      "1.000, 2.000"
    )
  })

  it("explains what is missing when no location fields exist", () => {
    const model = build([table({ name: "value", values: [1, 2] })])
    expect(model.markers).toEqual([])
    expect(model.reason).toBe(noLocationReason)
  })

  it("explains it when a location mode finds nothing", () => {
    const model = build([cities()], {}, { location: "geohash" })
    expect(model.reason).toBe(noLocationReason)
  })
})

describe("skipped rows", () => {
  it("counts rows with missing, non-finite or out-of-range coordinates", () => {
    const model = build([
      table(
        { name: "lat", values: [1, null, NaN, 91, 5, "x"] },
        { name: "lon", values: [1, 2, 3, 4, 181, 6] }
      ),
    ])
    expect(model.markers).toHaveLength(1)
    expect(model.skipped).toBe(5)
  })

  it("counts an invalid geohash", () => {
    const model = build([
      table({ name: "geohash", type: "string", values: ["ezs42", "!!", 7] }),
    ])
    expect(model.markers).toHaveLength(1)
    expect(model.skipped).toBe(2)
  })

  it("words the count in singular and plural", () => {
    expect(skippedText(1)).toBe("1 row had no location.")
    expect(skippedText(12)).toBe("12 rows had no location.")
  })

  it("keeps at most 10,000 markers and counts the rest", () => {
    const total = maxMarkers + 5
    const model = build([
      table(
        { name: "lat", values: Array.from({ length: total }, () => 1) },
        { name: "lon", values: Array.from({ length: total }, () => 2) }
      ),
    ])
    expect(model.markers).toHaveLength(maxMarkers)
    expect(model.truncated).toBe(5)
  })
})

describe("marker size", () => {
  it("scales linearly from the small to the large size over the range", () => {
    const model = build(
      [cities([{ name: "value", values: [10, 20, 30] }])],
      {},
      { markerSize: 4, markerSizeMax: 12 }
    )
    const radii = model.markers.map((m) => m.radius).sort((a, b) => a - b)
    expect(radii).toEqual([4, 8, 12])
  })

  it("draws the largest markers first", () => {
    const model = build([cities([{ name: "value", values: [10, 30, 20] }])])
    const radii = model.markers.map((m) => m.radius)
    expect(radii).toEqual([...radii].sort((a, b) => b - a))
  })

  it("uses the named size field", () => {
    const model = build(
      [
        cities([
          { name: "first", values: [1, 1, 1] },
          { name: "count", values: [0, 5, 10] },
        ]),
      ],
      {},
      { sizeField: "count", markerSize: 2, markerSizeMax: 10 }
    )
    expect(model.markers.map((m) => m.radius).sort((a, b) => a - b)).toEqual([
      2, 6, 10,
    ])
  })

  it("uses the small size when every value is equal or there is no number field", () => {
    const equal = build([cities([{ name: "value", values: [3, 3, 3] }])])
    expect(equal.markers.every((m) => m.radius === 5)).toBe(true)
    const none = build([cities()])
    expect(none.markers.every((m) => m.radius === 5)).toBe(true)
  })

  it("never lets the large size fall below the small size", () => {
    const model = build(
      [cities([{ name: "value", values: [1, 2, 3] }])],
      {},
      { markerSize: 9, markerSizeMax: 3 }
    )
    expect(model.markers.every((m) => m.radius === 9)).toBe(true)
  })
})

describe("marker color", () => {
  const thresholds = [
    { color: "ok" as const },
    { value: 15, color: "warning" as const },
    { value: 25, color: "critical" as const },
  ]

  it("takes the threshold color of the color field and keeps its state", () => {
    const model = build([cities([{ name: "value", values: [10, 20, 30] }])], {
      thresholds,
    })
    const byRadius = [...model.markers].sort((a, b) => a.radius - b.radius)
    expect(byRadius.map((m) => m.color)).toEqual([
      "var(--threshold-ok)",
      "var(--threshold-warning)",
      "var(--threshold-critical)",
    ])
    expect(byRadius[2].state).toEqual({ level: "critical", word: "Critical" })
  })

  it("takes a value mapping color without a threshold state", () => {
    const model = build([cities([{ name: "value", values: [1, 1, 1] }])], {
      mappings: [{ value: "1", text: "Up", color: "ok" }],
    } as Partial<DataPanel>)
    expect(model.markers[0].color).toBe("var(--threshold-ok)")
    expect(model.markers[0].state).toBeNull()
  })

  it("falls back to the first series color", () => {
    const model = build([cities([{ name: "value", values: [1, 2, 3] }])])
    expect(new Set(model.markers.map((m) => m.color))).toEqual(
      new Set(["var(--series-1)"])
    )
    expect(model.markers[0].state).toBeNull()
  })
})

describe("tooltip details", () => {
  it("lists the other fields with formatted values", () => {
    const model = build(
      [
        cities([
          {
            name: "city",
            type: "string",
            values: ["Paris", "Tokyo", "Sydney"],
          },
          { name: "value", values: [1, 2, 3] },
        ]),
      ],
      { unit: "percent" } as Partial<DataPanel>
    )
    const paris = model.markers.find((m) => m.label === "48.850, 2.350")
    expect(paris?.details.map((d) => d.name)).toEqual(["city", "value"])
    expect(paris?.details[0].text).toBe("Paris")
    expect(paris?.details[1].text).toContain("1")
  })
})

describe("viewOptions", () => {
  it("defaults to fitting the data", () => {
    expect(viewOptions({}).mode).toBe("fit")
  })

  it("reads the coordinate view", () => {
    expect(viewOptions({ view: "coords", lat: 10, lon: 20, zoom: 5 })).toEqual({
      mode: "coords",
      lat: 10,
      lon: 20,
      zoom: 5,
    })
  })
})

describe("initialView", () => {
  const { markers } = build([cities()])
  const size = [600, 300] as const

  it("fits the markers by default", () => {
    expect(initialView(viewOptions({}), markers, ...size)).toEqual(
      fitBounds(markers, ...size)
    )
  })

  it("shows the world on request", () => {
    expect(
      initialView(viewOptions({ view: "world" }), markers, ...size)
    ).toEqual(worldView(...size))
  })

  it("centers on the given coordinates at the given zoom", () => {
    const view = initialView(
      viewOptions({ view: "coords", lat: 48, lon: 2, zoom: 7 }),
      markers,
      ...size
    )
    expect(view).toEqual({ center: project(2, 48), zoom: 7 })
  })

  it("keeps a coordinate view inside the zoom limits", () => {
    const view = initialView(
      viewOptions({ view: "coords", zoom: 99 }),
      markers,
      ...size
    )
    expect(view.zoom).toBe(18)
  })
})
